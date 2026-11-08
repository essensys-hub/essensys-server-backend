package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/unifi"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// Service « sonnette » (sonnette-unifi-2026-11-007) : mémorise la sonnerie en cours, fait
// clignoter les lumières choisies (elles finissent allumées). Les écritures dans la
// table d'échange passent par DoorbellWriter (le chemin des commandes de lumières) ; les
// essais automatiques injectent un faux écrivain, jamais l'armoire.

const (
	// DoorbellRingingDuration : durée pendant laquelle ringing reste vrai après la pression.
	DoorbellRingingDuration = 60 * time.Second
	// DoorbellBurstWindow : une pression dans cette fenêtre après le début d'une séquence ne
	// relance pas de clignotement.
	DoorbellBurstWindow = 30 * time.Second
	// DoorbellPhase : durée d'une phase allumée ou éteinte (l'armoire lit les ordres à son
	// rythme ; à ajuster après l'essai 1.1).
	DoorbellPhase = 2 * time.Second
)

// ErrDoorbellWrite : l'écriture d'un ordre a échoué (armoire inconnue, file refusée).
var ErrDoorbellWrite = errors.New("doorbell_write_failed")

// lightCommand décrit la commande d'une lumière dans la table d'échange : ordre d'allumage,
// ordre d'extinction (indices 605 à 616) et masque binaire. Copie de IndexTable (apps) et de
// LightingPage (portail) : NE PAS modifier sans modifier les deux.
type lightCommand struct {
	On, Off int
	Mask    int
}

var lightCommands = map[string]lightCommand{
	"terrasse": {616, 610, 4}, "entree": {611, 605, 1}, "escalier": {613, 607, 1},
	"deg1": {616, 610, 1}, "deg2": {616, 610, 2}, "pieceserv": {615, 609, 128},
	"ann1": {616, 610, 8}, "ann2": {616, 610, 16}, "salon": {612, 606, 128},
	"sam": {612, 606, 64}, "cuisine": {615, 609, 1}, "sdb1": {616, 610, 128},
	"sdb2": {615, 609, 8}, "wc1": {615, 609, 32}, "wc2": {615, 609, 64},
	"bureau": {612, 606, 32}, "gdchamb": {614, 608, 128}, "ptchamb1": {614, 608, 64},
	"ptchamb2": {614, 608, 32}, "ptchamb3": {614, 608, 16}, "dressing": {611, 605, 8},
	"isalonind": {611, 605, 2}, "isalonind2": {611, 605, 4}, "icuisine": {615, 609, 2},
	"isdb1": {615, 609, 4}, "isdb2": {615, 609, 16}, "igdchamb1": {613, 607, 2},
	"igdchamb2": {613, 607, 4}, "iptchamb1": {613, 607, 8}, "iptchamb2": {613, 607, 16},
	"iptchamb22": {613, 607, 32}, "iptchamb3": {613, 607, 64}, "idressing": {611, 605, 16},
}

// DoorbellWriter écrit des ordres dans la table d'échange (un bloc par appel).
type DoorbellWriter interface {
	Write(params []protocol.ExchangeKV) error
}

// DoorbellStatus est la réponse de GET /api/doorbell/status.
type DoorbellStatus struct {
	Enabled      bool    `json:"enabled"`
	Available    bool    `json:"available"`
	Ringing      bool    `json:"ringing"`
	RingingUntil *string `json:"ringing_until"`
	LastRingAt   *string `json:"last_ring_at"`
	// InWindow : la case horaire de l'instant courant est active (sonnette-plages-push).
	InWindow bool `json:"in_window"`
}

// DoorbellRingResult décrit ce qu'une pression a déclenché.
type DoorbellRingResult struct {
	// Sequence : une séquence de clignotement a été lancée (ou, à sec, simulée).
	Sequence bool
	// Lights : nombre de lumières concernées par la séquence.
	Lights int
	DryRun bool
}

// DoorbellService est le service de la sonnette.
type DoorbellService struct {
	store  *LightNameStore
	writer DoorbellWriter

	// Injectables pour les tests.
	Now   func() time.Time
	Sleep func(time.Duration)
	Phase time.Duration
	// Notifier signale un appui valide (dans la plage) au cloud ; appelé en tâche de fond, il
	// ne retarde jamais le clignotement. nil : aucun signalement.
	Notifier func(ringID string, at time.Time)

	mu           sync.Mutex
	available    bool
	ringingUntil time.Time
	lastRing     time.Time
	running      bool
	lastSeqStart time.Time
	lastNotify   time.Time
	notifyWG     sync.WaitGroup
	seqLights    map[string]bool // lumières de la séquence en cours
	excluded     map[string]bool // lumières commandées à la main pendant la séquence
	wg           sync.WaitGroup
	stop         chan struct{}
	stopOnce     sync.Once
}

// NewDoorbellService crée le service. writer peut être nil (aucune écriture : la
// sonnerie est signalée mais rien ne clignote).
func NewDoorbellService(store *LightNameStore, writer DoorbellWriter) *DoorbellService {
	s := &DoorbellService{
		store: store, writer: writer,
		Now: time.Now, Phase: DoorbellPhase,
		stop: make(chan struct{}),
	}
	s.Sleep = func(d time.Duration) {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
		case <-s.stop:
		}
	}
	return s
}

// Close interrompt les séquences en cours (les lumières choisies sont laissées allumées).
func (s *DoorbellService) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.wg.Wait()
}

// WaitIdle attend la fin de la séquence en cours (tests).
func (s *DoorbellService) WaitIdle() { s.wg.Wait(); s.notifyWG.Wait() }

// SetAvailable est appelé par la source d'événements : la console est joignable ou non.
func (s *DoorbellService) SetAvailable(ok bool) {
	s.mu.Lock()
	s.available = ok
	s.mu.Unlock()
}

// Status renvoie l'état pour le portail.
func (s *DoorbellService) Status() DoorbellStatus {
	settings := s.store.Doorbell()
	now := s.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := DoorbellStatus{Enabled: settings.Enabled, Available: settings.Enabled && s.available,
		InWindow: settings.Schedule.ActiveAt(now)}
	if !s.lastRing.IsZero() {
		at := s.lastRing.UTC().Format(lightNameTimeLayout)
		st.LastRingAt = &at
	}
	if now.Before(s.ringingUntil) {
		st.Ringing = true
		until := s.ringingUntil.UTC().Format(lightNameTimeLayout)
		st.RingingUntil = &until
	}
	return st
}

// HandleRing traite une sonnerie venue de la console : ignorée si la fonction est désactivée
// ou si la caméra n'est pas celle des réglages.
func (s *DoorbellService) HandleRing(ev unifi.RingEvent) {
	settings := s.store.Doorbell()
	if !settings.Enabled || settings.CameraID == "" || ev.CameraID != settings.CameraID {
		return
	}
	at := ev.At
	if at.IsZero() {
		at = s.Now()
	}
	id := ev.ID
	if id == "" {
		id = newRingID()
	}
	s.press(true, false, at, id)
}

func newRingID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// Ring enregistre une pression (vraie ou essai) et lance la séquence de clignotement si
// aucune n'est en cours ni récente. dryRun : aucune écriture dans la table d'échange.
// Cet appel est l'« Essayer » : il ignore la plage horaire et ne prévient pas le cloud.
func (s *DoorbellService) Ring(dryRun bool) DoorbellRingResult {
	return s.press(false, dryRun, time.Time{}, "")
}

// press traite une pression. real : vraie pression de la sonnette (plage évaluée à l'heure at,
// signalement au cloud) ; sinon essai.
func (s *DoorbellService) press(real, dryRun bool, at time.Time, ringID string) DoorbellRingResult {
	settings := s.store.Doorbell()
	now := s.Now()
	inWindow := !real || settings.Schedule.ActiveAt(at)

	s.mu.Lock()
	s.lastRing = now
	s.ringingUntil = now.Add(DoorbellRingingDuration)
	res := DoorbellRingResult{DryRun: dryRun}
	if !inWindow {
		// Hors plage : ni clignotement, ni signalement, anti-rafale non armé ; ringing reste vrai.
		s.mu.Unlock()
		return res
	}
	if real && s.Notifier != nil && (s.lastNotify.IsZero() || now.Sub(s.lastNotify) >= DoorbellBurstWindow) {
		s.lastNotify = now
		notify := s.Notifier
		s.notifyWG.Add(1)
		defer func() {
			go func() {
				defer s.notifyWG.Done()
				notify(ringID, at)
			}()
		}()
	}
	var ids []string
	for _, id := range settings.Lights {
		if _, ok := lightCommands[id]; ok {
			ids = append(ids, id)
		}
	}
	burst := s.running || (!s.lastSeqStart.IsZero() && now.Sub(s.lastSeqStart) < DoorbellBurstWindow)
	if len(ids) == 0 || burst || (!dryRun && (s.writer == nil)) {
		s.mu.Unlock()
		return res
	}
	res.Sequence, res.Lights = true, len(ids)
	if dryRun {
		// Simulation : l'état de la séquence (anti-rafale) n'est pas armé par un essai à sec.
		s.mu.Unlock()
		return res
	}
	s.running = true
	s.lastSeqStart = now
	s.seqLights = map[string]bool{}
	for _, id := range ids {
		s.seqLights[id] = true
	}
	s.excluded = map[string]bool{}
	s.wg.Add(1)
	s.mu.Unlock()

	go s.sequence(ids, settings.Blinks)
	return res
}

// NoteCommand est appelé pour chaque ordre venu d'un utilisateur ou d'un automatisme (pas pour
// ceux de la séquence) : une lumière de la séquence qui est commandée sort de la séquence.
func (s *DoorbellService) NoteCommand(params []protocol.ExchangeKV) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	for _, p := range params {
		v, err := strconv.Atoi(p.V)
		if err != nil || v == 0 {
			continue
		}
		for id := range s.seqLights {
			c := lightCommands[id]
			if (p.K == c.On || p.K == c.Off) && v&c.Mask != 0 {
				s.excluded[id] = true
			}
		}
	}
}

func (s *DoorbellService) active(ids []string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, id := range ids {
		if !s.excluded[id] {
			out = append(out, id)
		}
	}
	return out
}

// buildParams regroupe les lumières par indice de commande (masques cumulés par OU).
func buildDoorbellParams(want map[string]bool) []protocol.ExchangeKV {
	byIdx := map[int]int{}
	for id, on := range want {
		c := lightCommands[id]
		idx := c.Off
		if on {
			idx = c.On
		}
		byIdx[idx] |= c.Mask
	}
	idxs := make([]int, 0, len(byIdx))
	for k := range byIdx {
		idxs = append(idxs, k)
	}
	sort.Ints(idxs)
	out := make([]protocol.ExchangeKV, 0, len(idxs))
	for _, k := range idxs {
		out = append(out, protocol.ExchangeKV{K: k, V: strconv.Itoa(byIdx[k])})
	}
	return out
}

func (s *DoorbellService) put(want map[string]bool) bool {
	if len(want) == 0 {
		return true
	}
	if err := s.writer.Write(buildDoorbellParams(want)); err != nil {
		log.Printf("Sonnette : écriture d'un ordre de lumière impossible")
		return false
	}
	return true
}

func (s *DoorbellService) sequence(ids []string, blinks int) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.seqLights, s.excluded = nil, nil
		s.mu.Unlock()
	}()

	phase := s.Phase
	if phase <= 0 {
		phase = DoorbellPhase
	}
	step := func(on bool) bool {
		want := map[string]bool{}
		for _, id := range s.active(ids) {
			want[id] = on
		}
		if len(want) == 0 {
			return false
		}
		if !s.put(want) {
			return false
		}
		s.Sleep(phase)
		select {
		case <-s.stop:
			return false
		default:
		}
		return true
	}
	// blinks cycles éteint/allumé ; le dernier ordre allume : les lumières choisies finissent
	// allumées (décision de l'utilisateur), sauf celles commandées à la main (exclues).
	for i := 0; i < blinks; i++ {
		if !step(false) || !step(true) {
			break
		}
	}
	// Arrêt en cours de séquence : on laisse les lumières allumées.
	if len(s.active(ids)) > 0 {
		select {
		case <-s.stop:
			s.put(allOn(s.active(ids)))
		default:
		}
	}
}

func allOn(ids []string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// ---------------------------------------------------------------------------------------------
// Supervision de la source d'événements

// DoorbellSupervisor lance la source d'événements pour la caméra des réglages et la relance
// quand les réglages changent (activation, autre caméra).
type DoorbellSupervisor struct {
	svc    *DoorbellService
	source unifi.EventSource

	mu     sync.Mutex
	parent context.Context
	cancel context.CancelFunc
	camera string
	done   chan struct{}
}

// NewDoorbellSupervisor crée le superviseur. source nil (UniFi désactivé) : aucune écoute.
func NewDoorbellSupervisor(svc *DoorbellService, source unifi.EventSource) *DoorbellSupervisor {
	return &DoorbellSupervisor{svc: svc, source: source}
}

// Start démarre l'écoute selon les réglages courants.
func (d *DoorbellSupervisor) Start(ctx context.Context) {
	d.mu.Lock()
	d.parent = ctx
	d.mu.Unlock()
	d.Reconfigure()
}

// Reconfigure relit les réglages : écoute la caméra choisie si la fonction est activée.
func (d *DoorbellSupervisor) Reconfigure() {
	if d == nil || d.source == nil {
		return
	}
	settings := d.svc.store.Doorbell()
	want := ""
	if settings.Enabled {
		want = settings.CameraID
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.parent == nil || want == d.camera {
		return
	}
	if d.cancel != nil {
		d.cancel()
		<-d.done
		d.cancel, d.done = nil, nil
		d.svc.SetAvailable(false)
	}
	d.camera = want
	if want == "" {
		return
	}
	ctx, cancel := context.WithCancel(d.parent)
	done := make(chan struct{})
	d.cancel, d.done = cancel, done
	go func() {
		defer close(done)
		d.source.Run(ctx, want, d.svc.HandleRing, d.svc.SetAvailable)
	}()
}

// Stop arrête l'écoute.
func (d *DoorbellSupervisor) Stop() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		d.cancel()
		<-d.done
		d.cancel, d.done = nil, nil
	}
}

// ---------------------------------------------------------------------------------------------
// Branchements de production sur la table d'échange

// ExchangeDoorbellWriter écrit par le chemin des commandes de lumières (ActionService), sans
// prévenir l'observateur de commandes manuelles. L'armoire visée est la dernière qui a interrogé
// la gateway (repli : « default »).
type ExchangeDoorbellWriter struct {
	Actions *ActionService
	Store   interface {
		GetLastPolledClientID() (string, bool)
	}
}

// Write implémente DoorbellWriter.
func (w ExchangeDoorbellWriter) Write(params []protocol.ExchangeKV) error {
	clientID := "default"
	if w.Store != nil {
		if id, ok := w.Store.GetLastPolledClientID(); ok && id != "" {
			clientID = id
		}
	}
	if _, err := w.Actions.AddActionsUnobserved(clientID, params); err != nil {
		return ErrDoorbellWrite
	}
	return nil
}
