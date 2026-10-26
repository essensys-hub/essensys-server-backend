package main

import "testing"

// security-outlets-2026-10-011 (#10): the MCP "couper / remettre prises
// sécurité" and "machines" commands go through the block complement instead of
// sending 623 or 624 alone.
func TestExpandLegacyScenarioBlock_prises_de_securite(t *testing.T) {
	for _, in := range []exchangeKV{{K: 623, V: "1"}, {K: 623, V: "2"}, {K: 624, V: "1"}} {
		got := map[int]string{}
		for _, p := range expandLegacyScenarioBlock([]exchangeKV{in}) {
			got[p.K] = p.V
		}
		if got[590] != "1" {
			t.Fatalf("%d=%s: expected 590=1, got %q", in.K, in.V, got[590])
		}
		for i := 605; i <= 622; i++ {
			if got[i] != "0" {
				t.Fatalf("%d=%s: expected %d=0, got %q", in.K, in.V, i, got[i])
			}
		}
		if got[in.K] != in.V {
			t.Fatalf("expected %d=%s, got %q", in.K, in.V, got[in.K])
		}
		if in.K != 623 && got[623] != "0" {
			t.Fatalf("%d=%s: expected 623=0, got %q", in.K, in.V, got[623])
		}
	}
}

func TestExpandLegacyScenarioBlock_eclairage_623_a_0(t *testing.T) {
	got := map[int]string{}
	for _, p := range expandLegacyScenarioBlock([]exchangeKV{{K: 612, V: "128"}}) {
		got[p.K] = p.V
	}
	if got[623] != "0" || got[612] != "128" {
		t.Fatalf("expected 612=128 and 623=0, got 612=%q 623=%q", got[612], got[623])
	}
}
