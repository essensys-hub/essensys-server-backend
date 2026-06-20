package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
)

func TestHandleScenarios_launchJeSors(t *testing.T) {
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	h := NewHandler(actionService, core.NewStatusService(store), store, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/scenarios/2/launch", nil)
	rec := httptest.NewRecorder()
	h.HandleScenarios(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["slot"].(float64) != 2 {
		t.Fatalf("resp %+v", resp)
	}

	actions := store.DequeueActions("default")
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if len(actions[0].Params) != 1 || actions[0].Params[0].K != 590 || actions[0].Params[0].V != "2" {
		t.Fatalf("params %+v", actions[0].Params)
	}
}

func TestHandleScenarios_list(t *testing.T) {
	store := data.NewMemoryStore()
	store.SetValue("default", 591, "2")
	actionService := core.NewActionService(store)
	h := NewHandler(actionService, core.NewStatusService(store), store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/scenarios", nil)
	rec := httptest.NewRecorder()
	h.HandleScenarios(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandleScenarios_bitmasks(t *testing.T) {
	store := data.NewMemoryStore()
	h := NewHandler(core.NewActionService(store), core.NewStatusService(store), store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/scenarios/meta/bitmasks", nil)
	rec := httptest.NewRecorder()
	h.HandleScenarios(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandleScenarios_launchSlot1Rejected(t *testing.T) {
	store := data.NewMemoryStore()
	h := NewHandler(core.NewActionService(store), core.NewStatusService(store), store, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/scenarios/1/launch", nil)
	rec := httptest.NewRecorder()
	h.HandleScenarios(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}
