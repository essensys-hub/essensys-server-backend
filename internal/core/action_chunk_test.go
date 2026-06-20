package core

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func TestAddActions_splitsBeyondFirmwareLimit(t *testing.T) {
	store := data.NewMemoryStore()
	svc := NewActionService(store)
	params := make([]protocol.ExchangeKV, 77)
	for i := range params {
		params[i] = protocol.ExchangeKV{K: 13 + i, V: "0"}
	}
	guids, err := svc.AddActions("default", params)
	if err != nil {
		t.Fatal(err)
	}
	if len(guids) != 3 {
		t.Fatalf("expected 3 chunked actions, got %d", len(guids))
	}
	queued := store.DequeueActions("default")
	if len(queued) != 3 {
		t.Fatalf("expected 3 queued actions, got %d", len(queued))
	}
	for _, act := range queued {
		if len(act.Params) > protocol.MaxParamsPerFirmwareAction {
			t.Fatalf("chunk too large: %d params", len(act.Params))
		}
	}
}
