package cloudsync

import (
	"testing"
	"time"
)

func TestProfileDue(t *testing.T) {
	last := time.Now().Add(-4 * time.Hour)
	p := cloudSyncProfile{
		Enabled: true, PullFromArmoire: true, IntervalHours: 3, LastRunAt: &last,
	}
	if !profileDue(p, time.Now()) {
		t.Fatal("expected profile due after 4h with interval 3h")
	}
	recent := time.Now().Add(-1 * time.Hour)
	p.LastRunAt = &recent
	if profileDue(p, time.Now()) {
		t.Fatal("expected profile not due after 1h with interval 3h")
	}
	p.Enabled = false
	if profileDue(p, time.Now()) {
		t.Fatal("disabled profile must not be due")
	}
}

func TestPushIndicesFromProfilesFallback(t *testing.T) {
	a := &Agent{}
	got := a.pushIndicesFromProfiles("default")
	legacy := exchangePushIndices()
	if len(got) != len(legacy) {
		t.Fatalf("fallback len=%d want %d", len(got), len(legacy))
	}
}
