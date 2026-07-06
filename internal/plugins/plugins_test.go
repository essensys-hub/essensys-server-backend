package plugins

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"

	plugin "github.com/essensys-hub/essensys-plugin-framework/go"
	sungrow "github.com/essensys-hub/essensys-plugin-sungrow/adapter"
)

func TestMapRole(t *testing.T) {
	cases := []struct {
		role     string
		wantOK   bool
		wantRole plugin.Role
	}{
		{models.LanRoleAdmin, true, plugin.RoleLANAdmin},
		{models.LanRoleUser, true, plugin.RoleLANUser},
		{models.LanRoleGuest, false, ""},
		{"", false, ""},
	}
	for _, c := range cases {
		id, ok := mapRole(c.role)
		if ok != c.wantOK {
			t.Fatalf("role %q: ok=%v attendu %v", c.role, ok, c.wantOK)
		}
		if ok && id.Roles[0] != c.wantRole {
			t.Fatalf("role %q: mappé %v attendu %v", c.role, id.Roles[0], c.wantRole)
		}
	}
}

func TestNewWiresSungrow(t *testing.T) {
	store := plugin.NewMemStore(90 * time.Second)
	reg, handler, err := New(Deps{Store: store, Sink: plugin.NewMemSink(), Bus: nil})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !reg.Enabled(sungrow.ID) {
		t.Fatal("sungrow-solar devrait être activé")
	}

	// Ingestion via le registre -> disponible sur /current.
	reg.Ingest(sungrow.New(), plugin.BusMessage{
		Topic: plugin.Topic(sungrow.ID, "A254", "pv_power"), MachineID: "A254",
		Payload: []byte(`{"value":5.81,"unit":"kW","ts":1783351102}`),
	})
	if cur := store.Current(sungrow.ID); len(cur.Samples) != 1 || cur.Samples[0].Value != 5.81 {
		t.Fatalf("current inattendu: %+v", cur)
	}

	// Non authentifié (aucun user LAN dans le contexte) -> 403.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/plugins/sungrow-solar/current", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("attendu 403 sans auth, got %d", rec.Code)
	}
}
