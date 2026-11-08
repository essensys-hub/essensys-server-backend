package unifi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/config"
	"github.com/gorilla/websocket"
)

// Sonnette UniFi (sonnette-unifi-2026-11-007) : détection de la pression de la sonnette.
// Deux sources derrière une même interface : l'écoute des événements de la console
// (WebSocket de l'API d'intégration) et, en repli, l'interrogation périodique de la caméra.
// Aucune adresse de console, clé d'API ni identifiant n'est jamais journalisé : les erreurs
// réseau de net/http et de gorilla contiennent l'adresse, on ne journalise donc que des
// messages fixes.

const (
	// eventsPath : flux d'événements de l'API d'intégration UniFi Protect (WebSocket).
	eventsPath = "/proxy/protect/integration/v1/subscribe/events"
	// cameraPollPath : état d'une caméra (champ lastRing, en millisecondes depuis 1970).
	cameraPollPath = "/proxy/protect/api/cameras/"

	// DefaultPollInterval : cadence du repli par interrogation (décision 1 du design).
	DefaultPollInterval = 2 * time.Second
	// BackoffMin et BackoffMax encadrent l'attente croissante de reconnexion (décision 8).
	BackoffMin = 1 * time.Second
	BackoffMax = 60 * time.Second
)

// RingEvent est une pression de la sonnette.
type RingEvent struct {
	CameraID string
	At       time.Time
}

// EventSource est une source d'événements de sonnerie. Run bloque jusqu'à l'annulation de ctx ;
// il appelle onRing pour chaque sonnerie de la caméra cameraID (jamais pour une autre caméra)
// et onState(true/false) quand la console devient joignable ou injoignable.
type EventSource interface {
	Run(ctx context.Context, cameraID string, onRing func(RingEvent), onState func(bool))
}

// Backoff est l'attente croissante entre deux tentatives (doublée à chaque échec, plafonnée).
type Backoff struct {
	Min, Max time.Duration
	cur      time.Duration
}

// Next renvoie l'attente avant la prochaine tentative.
func (b *Backoff) Next() time.Duration {
	min, max := b.Min, b.Max
	if min <= 0 {
		min = BackoffMin
	}
	if max <= 0 {
		max = BackoffMax
	}
	if b.cur == 0 {
		b.cur = min
	} else {
		b.cur *= 2
	}
	if b.cur > max {
		b.cur = max
	}
	return b.cur
}

// Reset repart de l'attente minimale (après une connexion réussie).
func (b *Backoff) Reset() { b.cur = 0 }

func insecureTransport() *http.Transport {
	// La console UniFi utilise un certificat auto-signé (comme le client existant).
	return &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// ---------------------------------------------------------------------------------------------
// Source 1 : écoute des événements (WebSocket)

// StreamSource écoute le flux d'événements de la console.
type StreamSource struct {
	cfg     config.UniFiConfig
	Backoff Backoff
	// Fallback prend le relais si la console n'expose pas le flux d'événements (réponse 404,
	// 405 ou 501 à l'ouverture) : interrogation périodique (décision 1 du design).
	Fallback EventSource
}

// NewStreamSource crée la source par écoute d'événements.
func NewStreamSource(cfg config.UniFiConfig) *StreamSource { return &StreamSource{cfg: cfg} }

func (s *StreamSource) wsURL() string {
	u, err := url.Parse(s.cfg.BaseURL)
	if err != nil {
		return ""
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + eventsPath
	return u.String()
}

// ringFromMessage extrait la sonnerie d'un message du flux (nil si ce n'est pas une sonnerie
// de la caméra voulue). Les messages « add » et « update » d'un même événement portent le même
// identifiant : l'appelant dédoublonne.
func ringFromMessage(raw []byte, cameraID string) (id string, ev *RingEvent) {
	var m struct {
		Type string `json:"type"`
		Item struct {
			ID       string `json:"id"`
			ModelKey string `json:"modelKey"`
			Type     string `json:"type"`
			Device   string `json:"device"`
			Start    int64  `json:"start"`
		} `json:"item"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return "", nil
	}
	if m.Item.Type != "ring" || (m.Item.ModelKey != "" && m.Item.ModelKey != "event") {
		return "", nil
	}
	if m.Type != "add" && m.Type != "update" {
		return "", nil
	}
	if cameraID == "" || m.Item.Device != cameraID {
		return "", nil
	}
	at := time.Now()
	if m.Item.Start > 0 {
		at = time.UnixMilli(m.Item.Start)
	}
	return m.Item.ID, &RingEvent{CameraID: cameraID, At: at}
}

// Run implémente EventSource.
func (s *StreamSource) Run(ctx context.Context, cameraID string, onRing func(RingEvent), onState func(bool)) {
	dialer := websocket.Dialer{
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
		HandshakeTimeout: 15 * time.Second,
	}
	target := s.wsURL()
	seen := map[string]bool{}
	for ctx.Err() == nil {
		hdr := http.Header{}
		if s.cfg.APIKey != "" {
			hdr.Set("X-API-Key", s.cfg.APIKey)
		}
		conn, resp, err := dialer.DialContext(ctx, target, hdr)
		if err != nil {
			if resp != nil && s.Fallback != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
				log.Printf("Sonnette : flux d'événements UniFi non exposé, repli par interrogation")
				s.Fallback.Run(ctx, cameraID, onRing, onState)
				return
			}
			onState(false)
			if ctx.Err() != nil {
				return
			}
			wait := s.Backoff.Next()
			log.Printf("Sonnette : console UniFi injoignable, nouvelle tentative dans %s", wait)
			if !sleepCtx(ctx, wait) {
				return
			}
			continue
		}
		s.Backoff.Reset()
		onState(true)
		log.Printf("Sonnette : écoute des événements UniFi établie")

		stop := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-stop:
			}
		}()
		for {
			_, raw, rerr := conn.ReadMessage()
			if rerr != nil {
				break
			}
			id, ev := ringFromMessage(raw, cameraID)
			if ev == nil {
				continue
			}
			if id != "" {
				if seen[id] {
					continue
				}
				if len(seen) > 256 {
					seen = map[string]bool{}
				}
				seen[id] = true
			}
			onRing(*ev)
		}
		close(stop)
		_ = conn.Close()
		onState(false)
		if ctx.Err() != nil {
			return
		}
		wait := s.Backoff.Next()
		log.Printf("Sonnette : écoute des événements UniFi interrompue, nouvelle tentative dans %s", wait)
		if !sleepCtx(ctx, wait) {
			return
		}
	}
}

// ---------------------------------------------------------------------------------------------
// Source 2 : interrogation périodique (repli)

// PollSource interroge la caméra et détecte une nouvelle sonnerie quand lastRing augmente.
type PollSource struct {
	cfg      config.UniFiConfig
	client   *http.Client
	Interval time.Duration
	Backoff  Backoff
}

// NewPollSource crée la source par interrogation.
func NewPollSource(cfg config.UniFiConfig) *PollSource {
	return &PollSource{
		cfg:      cfg,
		client:   &http.Client{Transport: insecureTransport(), Timeout: 10 * time.Second},
		Interval: DefaultPollInterval,
	}
}

// lastRing lit l'horodatage de la dernière sonnerie (millisecondes ; 0 si jamais).
func (p *PollSource) lastRing(ctx context.Context, cameraID string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.cfg.BaseURL, "/")+cameraPollPath+url.PathEscape(cameraID), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("X-API-Key", p.cfg.APIKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, io.ErrUnexpectedEOF
	}
	var cam struct {
		LastRing int64 `json:"lastRing"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cam); err != nil {
		return 0, err
	}
	return cam.LastRing, nil
}

// Run implémente EventSource. La première interrogation fixe la référence : une sonnerie
// antérieure au démarrage n'est pas rejouée.
func (p *PollSource) Run(ctx context.Context, cameraID string, onRing func(RingEvent), onState func(bool)) {
	interval := p.Interval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	var last int64
	haveBase := false
	online := false
	for ctx.Err() == nil {
		v, err := p.lastRing(ctx, cameraID)
		wait := interval
		if err != nil {
			if online || !haveBase {
				onState(false)
			}
			online = false
			if ctx.Err() != nil {
				return
			}
			wait = p.Backoff.Next()
			log.Printf("Sonnette : console UniFi injoignable (interrogation), nouvelle tentative dans %s", wait)
		} else {
			p.Backoff.Reset()
			if !online {
				onState(true)
				online = true
			}
			if !haveBase {
				last, haveBase = v, true
			} else if v > last {
				last = v
				onRing(RingEvent{CameraID: cameraID, At: time.UnixMilli(v)})
			}
		}
		if !sleepCtx(ctx, wait) {
			return
		}
	}
}
