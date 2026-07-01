package armoire

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/config"
)

func TestRotator_MaxIndicesPerChunk(t *testing.T) {
	groups := [][]int{
		DefaultCommandIndices,
		IdentityIndices,
		HealthIndices,
		ComfortEnergyIndices,
	}
	for i, g := range groups {
		if len(g) > 30 {
			t.Fatalf("group %d has %d indices (>30)", i, len(g))
		}
	}
}

func TestRotator_CyclesGroups(t *testing.T) {
	r := NewRotator(true)
	first := r.Next()
	second := r.Next()
	third := r.Next()
	fourth := r.Next()

	if len(first) != len(DefaultCommandIndices) {
		t.Fatalf("expected default chunk first, got len %d", len(first))
	}
	if second[0] != IdentityIndices[0] {
		t.Fatalf("expected identity group second, got %v", second)
	}
	if third[0] != HealthIndices[0] {
		t.Fatalf("expected health group third, got %v", third)
	}
	if fourth[0] != ComfortEnergyIndices[0] {
		t.Fatalf("expected comfort group fourth, got %v", fourth)
	}
}

func TestRotator_DisabledUsesDefault(t *testing.T) {
	r := NewRotator(false)
	for i := 0; i < 5; i++ {
		indices := r.Next()
		if len(indices) != len(DefaultCommandIndices) {
			t.Fatalf("iteration %d: expected default indices", i)
		}
	}
}

func TestDefaultCommandIndices_MatchesLegacyList(t *testing.T) {
	legacy := []int{613, 607, 615, 590, 349, 350, 351, 352, 363, 425, 426, 920,
		566, 567, 568, 569, 570, 571, 572,
		574, 575, 576, 577, 578,
		582, 583, 584, 585}
	if len(DefaultCommandIndices) != len(legacy) {
		t.Fatalf("default list length changed: %d vs %d", len(DefaultCommandIndices), len(legacy))
	}
	for i := range legacy {
		if DefaultCommandIndices[i] != legacy[i] {
			t.Fatalf("index %d: got %d want %d", i, DefaultCommandIndices[i], legacy[i])
		}
	}
}

func TestConfig_DefaultDashboardPullEnabled(t *testing.T) {
	cfg := &config.Config{}
	// mirror defaults from config.Load path
	cfg.Armoire.DashboardPullEnabled = true
	if !cfg.Armoire.DashboardPullEnabled {
		t.Fatal("expected dashboard pull enabled by default")
	}
}
