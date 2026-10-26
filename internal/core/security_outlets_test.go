package core

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func blockValues(params []protocol.ExchangeKV) map[int]string {
	m := make(map[int]string, len(params))
	for _, p := range params {
		m[p.K] = p.V
	}
	return m
}

// NR: NR-server-backend-22 essensys-hub/essensys-server-backend#10
// security-outlets-2026-10-011: a lone cut or restore of the security outlets
// is completed into a full scenario 1 block, otherwise the BP ignores it.
func Test_NR_server_backend_22_coupure_seule_complete_590_et_605_a_622(t *testing.T) {
	service := NewActionService(data.NewMemoryStore())
	for _, v := range []string{"1", "2"} {
		got := blockValues(service.GenerateCompleteBlock([]protocol.ExchangeKV{{K: protocol.IndexSecurityOutlets, V: v}}))
		if got[protocol.IndexScenario] != "1" {
			t.Fatalf("623=%s: expected 590=1, got %q", v, got[protocol.IndexScenario])
		}
		for i := protocol.IndexLightStart; i <= protocol.IndexLightEnd; i++ {
			if got[i] != "0" {
				t.Fatalf("623=%s: expected %d=0, got %q", v, i, got[i])
			}
		}
		if got[protocol.IndexSecurityOutlets] != v {
			t.Fatalf("expected 623=%s, got %q", v, got[protocol.IndexSecurityOutlets])
		}
	}
}

// NR: NR-server-backend-23 essensys-hub/essensys-server-backend#10
// security-outlets-2026-10-011: a light command carries 623=0 and cannot
// replay a cut; a mixed batch keeps both values.
func Test_NR_server_backend_23_eclairage_ajoute_623_a_0(t *testing.T) {
	service := NewActionService(data.NewMemoryStore())
	light := blockValues(service.GenerateCompleteBlock([]protocol.ExchangeKV{{K: 612, V: "128"}}))
	if light[612] != "128" || light[protocol.IndexSecurityOutlets] != "0" {
		t.Fatalf("light command: expected 612=128 and 623=0, got 612=%q 623=%q", light[612], light[protocol.IndexSecurityOutlets])
	}
	mixed := blockValues(service.GenerateCompleteBlock([]protocol.ExchangeKV{{K: 612, V: "128"}, {K: protocol.IndexSecurityOutlets, V: "1"}}))
	if mixed[612] != "128" || mixed[protocol.IndexSecurityOutlets] != "1" || mixed[protocol.IndexScenario] != "1" {
		t.Fatalf("mixed batch: got 590=%q 612=%q 623=%q", mixed[protocol.IndexScenario], mixed[612], mixed[protocol.IndexSecurityOutlets])
	}
}

// security-outlets-2026-10-011 (#10): the gateway queue never merges actions,
// so a cut between two light commands stays a separate action, in order.
func TestAddActions_coupure_jamais_fusionnee_avec_eclairage(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)
	for _, p := range [][]protocol.ExchangeKV{
		{{K: 612, V: "128"}},
		{{K: protocol.IndexSecurityOutlets, V: "1"}},
		{{K: 613, V: "64"}},
	} {
		if _, err := service.AddActions("dev1", p); err != nil {
			t.Fatal(err)
		}
	}
	queued := store.DequeueActions("dev1")
	if len(queued) != 3 {
		t.Fatalf("expected 3 queued actions, got %d", len(queued))
	}
	want := []string{"0", "1", "0"}
	for i, a := range queued {
		if got := blockValues(a.Params)[protocol.IndexSecurityOutlets]; got != want[i] {
			t.Fatalf("action %d: expected 623=%s, got %q", i, want[i], got)
		}
	}
}
