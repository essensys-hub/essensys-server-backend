package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/api/testmode"
	"github.com/essensys-hub/essensys-server-backend/internal/audit"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
)

// affichage-objets-2026-11-006 : activation/désactivation et ordre d'affichage des lumières et
// des volets (réplique de la gateway). Un document par genre, mêmes routes jumelles, même
// fichier que les noms (sections lights_layout et shutters_layout).

// layoutMaxBody est la taille maximale du corps d'une écriture (16 Ko).
const layoutMaxBody = 16 * 1024

// SetObjectLayout branche le magasin (le même que celui des noms) et les déclencheurs de
// synchronisation immédiate avec le cloud, un par genre (appelés sans bloquer, peuvent être nil).
func (h *Handler) SetObjectLayout(store *core.LightNameStore, notifyLights, notifyShutters func()) {
	h.objectLayout = store
	h.objectLayoutNotify = map[core.LayoutKind]func(){core.LayoutLights: notifyLights, core.LayoutShutters: notifyShutters}
}

type objectLayoutSaved struct {
	core.ObjectLayout
	Status string `json:"status"`
}

// GetLightsLayout handles GET /api/lights/layout (tout utilisateur authentifié).
func (h *Handler) GetLightsLayout(w http.ResponseWriter, r *http.Request) {
	h.getLayout(w, r, core.LayoutLights)
}

// GetShuttersLayout handles GET /api/shutters/layout (tout utilisateur authentifié).
func (h *Handler) GetShuttersLayout(w http.ResponseWriter, r *http.Request) {
	h.getLayout(w, r, core.LayoutShutters)
}

// PutAdminLightsLayout handles PUT /api/admin/lights/layout.
func (h *Handler) PutAdminLightsLayout(w http.ResponseWriter, r *http.Request) {
	h.putLayout(w, r, core.LayoutLights)
}

// PutAdminShuttersLayout handles PUT /api/admin/shutters/layout.
func (h *Handler) PutAdminShuttersLayout(w http.ResponseWriter, r *http.Request) {
	h.putLayout(w, r, core.LayoutShutters)
}

func (h *Handler) getLayout(w http.ResponseWriter, r *http.Request, kind core.LayoutKind) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeLayoutJSON(w, h.objectLayout.Layout(kind))
}

// parseLayoutBody lit {"order":[…],"disabled":[…]} ou {"reset":true}. Toute anomalie de forme
// donne ErrInvalidLayout (corps illisible ou trop gros, clé absente, valeur qui n'est pas une
// liste de chaînes).
func parseLayoutBody(r *http.Request) (order, disabled []string, reset bool, err error) {
	body, rerr := io.ReadAll(io.LimitReader(r.Body, layoutMaxBody+1))
	defer r.Body.Close()
	if rerr != nil || len(body) > layoutMaxBody {
		return nil, nil, false, core.ErrInvalidLayout
	}
	var in map[string]json.RawMessage
	if json.Unmarshal(body, &in) != nil || in == nil {
		return nil, nil, false, core.ErrInvalidLayout
	}
	if raw, ok := in["reset"]; ok {
		var b bool
		if json.Unmarshal(raw, &b) != nil {
			return nil, nil, false, core.ErrInvalidLayout
		}
		if b {
			return []string{}, []string{}, true, nil
		}
	}
	list := func(key string) ([]string, bool) {
		raw, ok := in[key]
		if !ok || string(raw) == "null" {
			return nil, false
		}
		var out []string
		if json.Unmarshal(raw, &out) != nil || out == nil {
			return nil, false
		}
		return out, true
	}
	var ok1, ok2 bool
	if order, ok1 = list("order"); !ok1 {
		return nil, nil, false, core.ErrInvalidLayout
	}
	if disabled, ok2 = list("disabled"); !ok2 {
		return nil, nil, false, core.ErrInvalidLayout
	}
	return order, disabled, false, nil
}

func layoutErrorKey(err error) (int, string) {
	switch {
	case errors.Is(err, core.ErrUnknownLight):
		return http.StatusBadRequest, "unknown_light"
	case errors.Is(err, core.ErrUnknownShutter):
		return http.StatusBadRequest, "unknown_shutter"
	case errors.Is(err, core.ErrLightNamesUnavailable):
		return http.StatusServiceUnavailable, "names_unavailable"
	default:
		return http.StatusBadRequest, "invalid_layout"
	}
}

func (h *Handler) putLayout(w http.ResponseWriter, r *http.Request, kind core.LayoutKind) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	order, disabled, reset, err := parseLayoutBody(r)
	if err == nil {
		err = core.ValidateLayout(kind, order, disabled)
	}
	if err != nil {
		code, key := layoutErrorKey(err)
		writeLightTimerError(w, code, key)
		return
	}
	if !h.objectLayout.Available() {
		writeLightTimerError(w, http.StatusServiceUnavailable, "names_unavailable")
		return
	}
	previous := h.objectLayout.Layout(kind)

	if testmode.IsDryRun(r) {
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		writeLayoutSavedJSON(w, objectLayoutSaved{
			ObjectLayout: core.ObjectLayout{Order: order, Disabled: disabled, Version: previous.Version + 1, UpdatedAt: &now},
			Status:       "dry_run",
		})
		return
	}

	actor := audit.ActorIDFromRequest(r, "lan")
	saved, err := h.objectLayout.SetLayout(kind, order, disabled, reset, actor, time.Now().UTC())
	if err != nil {
		code, key := layoutErrorKey(err)
		writeLightTimerError(w, code, key)
		return
	}
	addDebugLog("Object layout %s saved (v%d)", kind, saved.Version)

	if h.audit != nil && h.audit.Enabled() {
		details := objectLayoutAuditDetails(kind, previous, saved)
		if u, ok := audit.UserFromRequest(r); ok {
			details["lan_user_id"] = u.ID
		}
		newDoc, _ := json.Marshal(saved)
		_ = h.audit.EmitUserAction(r.Context(), "lan_user", actor, fmt.Sprintf("object_layout:%s", kind), string(newDoc), details)
	}

	if notify := h.objectLayoutNotify[kind]; notify != nil {
		notify()
	}
	writeLayoutSavedJSON(w, objectLayoutSaved{ObjectLayout: saved, Status: "saved"})
}

// objectLayoutAuditDetails construit les détails de l'événement d'audit « object_layout »
// (qui est porté par l'acteur ; quand, ancien et nouveau document).
func objectLayoutAuditDetails(kind core.LayoutKind, previous, next core.ObjectLayout) map[string]any {
	return map[string]any{
		"event":    "object_layout",
		"kind":     string(kind),
		"version":  next.Version,
		"previous": previous,
		"new":      next,
	}
}

func writeLayoutJSON(w http.ResponseWriter, v core.ObjectLayout) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeLayoutSavedJSON(w http.ResponseWriter, v objectLayoutSaved) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
