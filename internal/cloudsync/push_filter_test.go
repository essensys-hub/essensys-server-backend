package cloudsync

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
)

func TestFilterExcludedIndices(t *testing.T) {
	in := []int{590, 591, 592, 605}
	got := filterExcludedIndices(in, []int{590})
	if len(got) != 3 || got[0] != 591 {
		t.Fatalf("filterExcludedIndices=%v want [591 592 605]", got)
	}
}

func TestPushIndicesFromProfilesList_excludes590(t *testing.T) {
	profiles := []cloudSyncProfile{{
		Name: "Scénarios", Enabled: true, PushToCloud: true,
		IndexRanges:    [][2]int{{591, 595}},
		ExcludeIndices: []int{590},
	}}
	got := pushIndicesFromProfilesList(profiles)
	for _, k := range got {
		if k == 590 {
			t.Fatal("590 must not be in push keys")
		}
	}
	if len(got) != 5 {
		t.Fatalf("len=%d want 5 (591-595)", len(got))
	}
}

func TestPushIndicesFromProfilesList_disabledProfile(t *testing.T) {
	profiles := []cloudSyncProfile{{
		Name: "Scénarios", Enabled: false, PushToCloud: true,
		IndexRanges: [][2]int{{592, 919}},
	}}
	got := pushIndicesFromProfilesList(profiles)
	if len(got) != 0 {
		t.Fatalf("disabled profile must contribute no keys, got %d", len(got))
	}
}

func TestPushIndicesFromProfilesList_union(t *testing.T) {
	profiles := []cloudSyncProfile{
		{Name: "A", Enabled: true, PushToCloud: true, IndexRanges: [][2]int{{1, 2}}},
		{Name: "B", Enabled: true, PushToCloud: true, IndexRanges: [][2]int{{2, 3}}},
	}
	got := pushIndicesFromProfilesList(profiles)
	want := len(core.FlattenIndexRanges([][2]int{{1, 3}}))
	if len(got) != want {
		t.Fatalf("union len=%d want %d", len(got), want)
	}
}

func TestFindScenariosProfile(t *testing.T) {
	profiles := []cloudSyncProfile{
		{Name: "Chauffage", ID: "a"},
		{Name: scenariosProfileName, ID: "b", Enabled: true},
	}
	p := findScenariosProfile(profiles)
	if p == nil || p.ID != "b" {
		t.Fatal("expected Scénarios profile")
	}
}
