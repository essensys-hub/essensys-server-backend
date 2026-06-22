package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
)

func TestPostAdminInject_dryRun(t *testing.T) {
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	h := NewHandler(actionService, core.NewStatusService(store), store, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/inject?test_mode=dry_run", strings.NewReader(`{"k":590,"v":"2"}`))
	rec := httptest.NewRecorder()
	h.PostAdminInject(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["dry_run"] != true || resp["status"] != "test_ok" {
		t.Fatalf("resp %+v", resp)
	}
	if len(store.DequeueActions("default")) != 0 {
		t.Fatal("expected no enqueued actions")
	}
}

func TestHandleScenarios_launchDryRun(t *testing.T) {
	store := data.NewMemoryStore()
	h := NewHandler(core.NewActionService(store), core.NewStatusService(store), store, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/scenarios/2/launch?test_mode=dry_run", nil)
	rec := httptest.NewRecorder()
	h.HandleScenarios(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if len(store.DequeueActions("default")) != 0 {
		t.Fatal("expected no enqueued actions")
	}
}
