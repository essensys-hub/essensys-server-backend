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

// noms-volets-2026-11-005 : noms personnalisés des volets et stores (réplique de la gateway).
// Routes jumelles de celles des lumières ; même fichier, même magasin.

const shutterNamesAdminPath = "/api/admin/shutters/names"

// SetShutterNames branche le magasin des noms (le même que celui des lumières) et le
// déclencheur de synchronisation immédiate avec le cloud (appelé sans bloquer, peut être nil).
func (h *Handler) SetShutterNames(store *core.LightNameStore, notify func()) {
	h.shutterNames = store
	h.shutterNamesNotify = notify
}

// GetShutterNames handles GET /api/shutters/names (tout utilisateur authentifié).
func (h *Handler) GetShutterNames(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"shutters": h.shutterNames.ShutterList()})
}

type shutterNameSaved struct {
	core.ShutterNameView
	Status string `json:"status"`
}

// PutAdminShutterName handles PUT /api/admin/shutters/names/{id} with {"name": "…"} or {"name": null}.
func (h *Handler) PutAdminShutterName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, shutterNamesAdminPath+"/")
	if _, known := core.ShutterNameDefault(id); !known {
		writeLightTimerError(w, http.StatusBadRequest, "unknown_shutter")
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
		if name, err = core.ValidateShutterName(s); err != nil {
			writeLightTimerError(w, http.StatusBadRequest, "invalid_name")
			return
		}
	}
	if !h.shutterNames.Available() {
		writeLightTimerError(w, http.StatusServiceUnavailable, "names_unavailable")
		return
	}
	previous, _ := h.shutterNames.GetShutter(id)

	if testmode.IsDryRun(r) {
		preview := previous
		preview.Name = name
		preview.Version++
		writeShutterNameJSON(w, shutterNameSaved{ShutterNameView: preview, Status: "dry_run"})
		return
	}

	actor := audit.ActorIDFromRequest(r, "lan")
	saved, err := h.shutterNames.SetShutter(id, name, actor, time.Now().UTC())
	if err != nil {
		if errors.Is(err, core.ErrLightNamesUnavailable) {
			writeLightTimerError(w, http.StatusServiceUnavailable, "names_unavailable")
		} else {
			writeLightTimerError(w, http.StatusBadRequest, "unknown_shutter")
		}
		return
	}
	addDebugLog("Shutter name %s set", id)

	if h.audit != nil && h.audit.Enabled() {
		details := map[string]any{
			"event":      "shutter_name",
			"shutter_id": id,
			"version":    saved.Version,
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
		_ = h.audit.EmitUserAction(r.Context(), "lan_user", actor, fmt.Sprintf("shutter_name:%s", id), newValue, details)
	}

	if h.shutterNamesNotify != nil {
		h.shutterNamesNotify()
	}
	writeShutterNameJSON(w, shutterNameSaved{ShutterNameView: saved, Status: "saved"})
}

func writeShutterNameJSON(w http.ResponseWriter, v shutterNameSaved) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
