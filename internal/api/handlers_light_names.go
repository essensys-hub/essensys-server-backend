package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/api/testmode"
	"github.com/essensys-hub/essensys-server-backend/internal/audit"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
)

// noms-lumieres-2026-11-004 : noms personnalisés des lumières (réplique de la gateway).

const lightNamesAdminPath = "/api/admin/lights/names"

// SetLightNames branche le magasin des noms et le déclencheur de synchronisation
// immédiate avec le cloud (appelé sans bloquer, peut être nil).
func (h *Handler) SetLightNames(store *core.LightNameStore, notify func()) {
	h.lightNames = store
	h.lightNamesNotify = notify
}

// GetLightNames handles GET /api/lights/names (tout utilisateur authentifié).
func (h *Handler) GetLightNames(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lights": h.lightNames.List()})
}

type lightNameSaved struct {
	core.LightNameView
	Status string `json:"status"`
}

// PutAdminLightName handles PUT /api/admin/lights/names/{id} with {"name": "…"} or {"name": null}.
func (h *Handler) PutAdminLightName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, lightNamesAdminPath+"/")
	if _, known := core.LightNameDefault(id); !known {
		writeLightTimerError(w, http.StatusBadRequest, "unknown_light")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	var in map[string]json.RawMessage
	rawName, present := json.RawMessage(nil), false
	if json.Unmarshal(body, &in) == nil {
		rawName, present = in["name"]
	}
	if !present {
		writeLightTimerError(w, http.StatusBadRequest, "invalid_name")
		return
	}
	var name *string
	if string(rawName) != "null" {
		var s string
		if json.Unmarshal(rawName, &s) != nil {
			writeLightTimerError(w, http.StatusBadRequest, "invalid_name")
			return
		}
		if name, err = core.ValidateLightName(s); err != nil {
			writeLightTimerError(w, http.StatusBadRequest, "invalid_name")
			return
		}
	}
	if !h.lightNames.Available() {
		writeLightTimerError(w, http.StatusServiceUnavailable, "names_unavailable")
		return
	}
	previous, _ := h.lightNames.Get(id)

	if testmode.IsDryRun(r) {
		preview := previous
		preview.Name = name
		preview.Version++
		writeLightNameJSON(w, lightNameSaved{LightNameView: preview, Status: "dry_run"})
		return
	}

	actor := audit.ActorIDFromRequest(r, "lan")
	saved, err := h.lightNames.Set(id, name, actor, time.Now().UTC())
	if err != nil {
		if errors.Is(err, core.ErrLightNamesUnavailable) {
			writeLightTimerError(w, http.StatusServiceUnavailable, "names_unavailable")
		} else {
			writeLightTimerError(w, http.StatusBadRequest, "unknown_light")
		}
		return
	}
	addDebugLog("Light name %s set", id)

	if h.audit != nil && h.audit.Enabled() {
		details := map[string]any{
			"event":    "light_name",
			"light_id": id,
			"version":  saved.Version,
		}
		details["previous_name"] = nameOrNil(previous.Name)
		details["new_name"] = nameOrNil(saved.Name)
		if u, ok := audit.UserFromRequest(r); ok {
			details["lan_user_id"] = u.ID
		}
		newValue := ""
		if saved.Name != nil {
			newValue = *saved.Name
		}
		_ = h.audit.EmitUserAction(r.Context(), "lan_user", actor, fmt.Sprintf("light_name:%s", id), newValue, details)
	}

	if h.lightNamesNotify != nil {
		h.lightNamesNotify()
	}
	writeLightNameJSON(w, lightNameSaved{LightNameView: saved, Status: "saved"})
}

func nameOrNil(n *string) any {
	if n == nil {
		return nil
	}
	return *n
}

func writeLightNameJSON(w http.ResponseWriter, v lightNameSaved) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
