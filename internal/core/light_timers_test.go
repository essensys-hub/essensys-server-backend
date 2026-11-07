package core

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func TestLightTimers_catalogue(t *testing.T) {
	want := map[string]int{"dressing": 521, "wc1": 555, "wc2": 556, "pieceserv": 557, "ann1": 561, "ann2": 562}
	got := LightTimers()
	if len(got) != len(want) {
		t.Fatalf("expected %d lights, got %d", len(want), len(got))
	}
	for _, l := range got {
		if want[l.ID] != l.Key || l.Label == "" {
			t.Fatalf("unexpected entry %+v", l)
		}
		if l.Key >= 605 && l.Key <= 623 {
			t.Fatalf("%s key %d is inside the scenario block", l.ID, l.Key)
		}
	}
	got[0].Key = 1
	if LightTimers()[0].Key != 521 {
		t.Fatal("catalogue must not be mutable by callers")
	}
}

func TestLightTimerParam(t *testing.T) {
	for _, m := range []int{0, 1, 15, 255} {
		p, err := LightTimerParam("dressing", m)
		if err != nil || p.K != 521 {
			t.Fatalf("%d: %+v %v", m, p, err)
		}
	}
	p, _ := LightTimerParam("wc2", 60)
	if p.K != 556 || p.V != "60" {
		t.Fatalf("got %+v", p)
	}
	for _, m := range []int{-1, 256, 300} {
		if _, err := LightTimerParam("dressing", m); err == nil {
			t.Fatalf("%d must be refused", m)
		}
	}
	if _, err := LightTimerParam("salon", 10); err == nil {
		t.Fatal("unknown id must be refused")
	}
}

// A timer write is a single key outside 605..623: no scenario block completion.
func TestLightTimerParam_noBlockCompletion(t *testing.T) {
	p, _ := LightTimerParam("ann1", 30)
	if out := (&ActionService{}).GenerateCompleteBlock([]protocol.ExchangeKV{p}); len(out) != 1 || out[0] != p {
		t.Fatalf("expected the single write untouched, got %+v", out)
	}
}

func TestParseLightTimerValue(t *testing.T) {
	for v, want := range map[string]int{"0": 0, "60": 60, " 255 ": 255} {
		if got := ParseLightTimerValue(v); got == nil || *got != want {
			t.Fatalf("%q: got %v", v, got)
		}
	}
	for _, v := range []string{"", "abc", "256", "-1", "7.5"} {
		if ParseLightTimerValue(v) != nil {
			t.Fatalf("%q must be unknown", v)
		}
	}
}
