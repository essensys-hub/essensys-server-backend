package core

import (
	"sync"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/unifi"
)

// sonnette-plages-push-2026-11-008 : évaluation de la plage à l'appui (horloge simulée).

type notifySpy struct {
	mu    sync.Mutex
	calls []string
	ats   []time.Time
}

func (n *notifySpy) notify(id string, at time.Time) {
	n.mu.Lock()
	n.calls = append(n.calls, id)
	n.ats = append(n.ats, at)
	n.mu.Unlock()
}

func (n *notifySpy) count() int { n.mu.Lock(); defer n.mu.Unlock(); return len(n.calls) }

// windowRig : mardi 14 h-14 h 59 heure de Paris seulement.
func windowRig(t *testing.T) (*rig, *notifySpy, *time.Location) {
	t.Helper()
	r := newDoorbellRig(t, []string{"salon"}, 1)
	sc := DoorbellSchedule{Mode: "custom", Timezone: "Europe/Paris", Grid: customGrid(map[int]string{1: "000000000000001000000000"})}
	if _, err := r.st.SetDoorbellSchedule(true, testDoorbellCam, []string{"salon"}, 1, &sc, "admin@example.org", time.Now()); err != nil {
		t.Fatal(err)
	}
	spy := &notifySpy{}
	r.svc.Notifier = spy.notify
	paris, _ := time.LoadLocation("Europe/Paris")
	return r, spy, paris
}

func TestDoorbellWindow_dansLaPlage(t *testing.T) {
	r, spy, paris := windowRig(t)
	at := time.Date(2026, 11, 10, 14, 5, 0, 0, paris) // mardi
	r.clk.t = at
	r.svc.HandleRing(unifi.RingEvent{ID: "evt-1", CameraID: testDoorbellCam, At: at})
	r.svc.WaitIdle()
	if len(r.w.all()) == 0 {
		t.Fatal("les lumières doivent clignoter")
	}
	if spy.count() != 1 || spy.calls[0] != "evt-1" || !spy.ats[0].Equal(at) {
		t.Fatalf("le cloud doit être prévenu une fois avec l'id de l'événement : %+v", spy.calls)
	}
	if !r.svc.Status().InWindow {
		t.Fatal("in_window attendu")
	}
}

func TestDoorbellWindow_horsPlage(t *testing.T) {
	r, spy, paris := windowRig(t)
	at := time.Date(2026, 11, 10, 3, 10, 0, 0, paris)
	r.clk.t = at
	r.svc.HandleRing(unifi.RingEvent{ID: "evt-2", CameraID: testDoorbellCam, At: at})
	r.svc.WaitIdle()
	if len(r.w.all()) != 0 || spy.count() != 0 {
		t.Fatal("hors plage : ni clignotement ni signalement")
	}
	st := r.svc.Status()
	if !st.Ringing || st.InWindow || st.LastRingAt == nil {
		t.Fatalf("ringing conservé, in_window=false : %+v", st)
	}
	// l'anti-rafale n'est pas armé : une pression dans la plage 5 s plus tard est prise en compte
	in := time.Date(2026, 11, 10, 14, 0, 5, 0, paris)
	r.clk.t = in
	r.svc.HandleRing(unifi.RingEvent{ID: "evt-3", CameraID: testDoorbellCam, At: in})
	r.svc.WaitIdle()
	if len(r.w.all()) == 0 || spy.count() != 1 {
		t.Fatal("la pression hors plage ne doit pas armer l'anti-rafale")
	}
}

func TestDoorbellWindow_heureDeLEvenementEtNonDeLArrivee(t *testing.T) {
	r, spy, paris := windowRig(t)
	r.clk.t = time.Date(2026, 11, 10, 3, 0, 0, 0, paris) // la gateway reçoit hors plage...
	at := time.Date(2026, 11, 10, 14, 30, 0, 0, paris)   // ...un événement daté dans la plage
	r.svc.HandleRing(unifi.RingEvent{ID: "e", CameraID: testDoorbellCam, At: at})
	r.svc.WaitIdle()
	if spy.count() != 1 {
		t.Fatal("c'est l'heure de l'événement qui compte")
	}
}

func TestDoorbellWindow_essaiIgnoreLaPlage(t *testing.T) {
	r, spy, paris := windowRig(t)
	r.clk.t = time.Date(2026, 11, 10, 3, 10, 0, 0, paris)
	res := r.svc.Ring(false)
	r.svc.WaitIdle()
	if !res.Sequence || len(r.w.all()) == 0 {
		t.Fatalf("« Essayer » ignore la plage : %+v", res)
	}
	if spy.count() != 0 {
		t.Fatal("un essai ne prévient pas le cloud")
	}
}

func TestDoorbellWindow_modeToujoursIgnoreLaGrille(t *testing.T) {
	r, spy, paris := windowRig(t)
	sc := DoorbellSchedule{Mode: "always", Timezone: "Europe/Paris", Grid: customGrid(nil)}
	if _, err := r.st.SetDoorbellSchedule(true, testDoorbellCam, []string{"salon"}, 1, &sc, "x", time.Now()); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 10, 3, 10, 0, 0, paris)
	r.clk.t = at
	r.svc.HandleRing(unifi.RingEvent{CameraID: testDoorbellCam, At: at})
	r.svc.WaitIdle()
	if spy.count() != 1 || len(r.w.all()) == 0 {
		t.Fatal("mode always : la grille est ignorée")
	}
	if len(spy.calls[0]) != 36 {
		t.Fatalf("un ring_id est généré quand l'événement n'en porte pas : %q", spy.calls[0])
	}
}

func TestDoorbellWindow_rafaleUnSeulSignalement(t *testing.T) {
	r, spy, paris := windowRig(t)
	at := time.Date(2026, 11, 10, 14, 5, 0, 0, paris)
	r.clk.t = at
	r.svc.HandleRing(unifi.RingEvent{ID: "a", CameraID: testDoorbellCam, At: at})
	r.svc.WaitIdle()
	r.clk.add(10 * time.Second)
	r.svc.HandleRing(unifi.RingEvent{ID: "b", CameraID: testDoorbellCam, At: r.clk.now()})
	r.svc.WaitIdle()
	if spy.count() != 1 {
		t.Fatalf("rafale : %d signalements", spy.count())
	}
	r.clk.add(30 * time.Second)
	r.svc.HandleRing(unifi.RingEvent{ID: "c", CameraID: testDoorbellCam, At: r.clk.now()})
	r.svc.WaitIdle()
	if spy.count() != 2 {
		t.Fatalf("après 30 s : %d signalements", spy.count())
	}
}

func TestDoorbellWindow_signalementMemeSansLumiereChoisie(t *testing.T) {
	r := newDoorbellRig(t, []string{}, 3)
	spy := &notifySpy{}
	r.svc.Notifier = spy.notify
	r.svc.HandleRing(unifi.RingEvent{ID: "x", CameraID: testDoorbellCam, At: r.clk.now()})
	r.svc.WaitIdle()
	if spy.count() != 1 {
		t.Fatal("le cloud est prévenu même sans lumière choisie")
	}
}

func TestDoorbellWindow_notifierBloquantNeRetardePasLeClignotement(t *testing.T) {
	r := newDoorbellRig(t, []string{"salon"}, 1)
	release := make(chan struct{})
	r.svc.Notifier = func(string, time.Time) { <-release }
	done := make(chan struct{})
	go func() {
		r.svc.HandleRing(unifi.RingEvent{ID: "x", CameraID: testDoorbellCam, At: r.clk.now()})
		r.svc.wg.Wait() // séquence de clignotement seule
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("le signalement bloqué retarde le clignotement")
	}
	if len(r.w.all()) == 0 {
		t.Fatal("la séquence doit avoir écrit")
	}
	close(release)
	r.svc.WaitIdle()
}
