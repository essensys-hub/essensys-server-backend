package core

import "testing"

func TestExchangePullScheduler_StartAndChunks(t *testing.T) {
	m := NewExchangePullScheduler()
	chunks := m.Start(181, 84)
	if chunks != 3 {
		t.Fatalf("expected 3 chunks, got %d", chunks)
	}

	first, ok := m.CurrentChunk()
	if !ok || len(first) != 30 || first[0] != 181 || first[29] != 210 {
		t.Fatalf("unexpected first chunk: ok=%v len=%d first=%v", ok, len(first), first)
	}
	m.Advance()

	second, ok := m.CurrentChunk()
	if !ok || len(second) != 30 || second[0] != 211 {
		t.Fatalf("unexpected second chunk: ok=%v first=%v", ok, second)
	}
	m.Advance()

	third, ok := m.CurrentChunk()
	if !ok || len(third) != 24 || third[0] != 241 || third[23] != 264 {
		t.Fatalf("unexpected third chunk: ok=%v len=%d", ok, len(third))
	}
	m.Advance()

	if _, ok := m.CurrentChunk(); ok {
		t.Fatal("expected sync inactive after all chunks")
	}
}

func TestExchangePullScheduler_TryStartExclusive(t *testing.T) {
	m := NewExchangePullScheduler()
	if _, ok := m.TryStart(181, 84); !ok {
		t.Fatal("first TryStart should succeed")
	}
	if _, ok := m.TryStart(13, 84); ok {
		t.Fatal("second TryStart should fail while active")
	}
	m.Advance()
	m.Advance()
	m.Advance()
	if _, ok := m.TryStart(13, 84); !ok {
		t.Fatal("TryStart should succeed after pull completes")
	}
}
