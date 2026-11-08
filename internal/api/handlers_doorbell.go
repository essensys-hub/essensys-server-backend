package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"reflect"
	"strconv"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/api/testmode"
	"github.com/essensys-hub/essensys-server-backend/internal/audit"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/unifi"
)

// Sonnette UniFi (sonnette-unifi-2026-11-007) : état et vidéo (utilisateur authentifié),
// réglages et essai (lan_admin). La clé d'API et l'adresse de la console ne quittent jamais la
// gateway : la vidéo est une suite d'instantanés servis par la gateway.

const (
	doorbellMaxBody = 4 * 1024
	// doorbellStreamInterval : un instantané par seconde environ.
	doorbellStreamInterval = time.Second
	// doorbellStreamMax : durée maximale d'un flux (le portail le rouvre au besoin).
	doorbellStreamMax = 10 * time.Minute
)

// DoorbellHandler porte les routes de la sonnette.
type DoorbellHandler struct {
	Service *core.DoorbellService
	Store   *core.LightNameStore
	// Cameras liste les caméras de la console (nil : UniFi désactivé).
	Cameras func() ([]unifi.Camera, error)
	// Snapshot renvoie un instantané JPEG de la caméra (nil : UniFi désactivé).
	Snapshot func(cameraID string) ([]byte, error)
	// OnSettingsChanged est appelé après un enregistrement (relance de l'écoute).
	OnSettingsChanged func()
	Audit             *audit.Service
	// StreamInterval et StreamMax sont réglables pour les tests.
	StreamInterval time.Duration
	StreamMax      time.Duration
}

// NewDoorbellHandler crée le handler.
func NewDoorbellHandler(svc *core.DoorbellService, store *core.LightNameStore) *DoorbellHandler {
	return &DoorbellHandler{Service: svc, Store: store, StreamInterval: doorbellStreamInterval, StreamMax: doorbellStreamMax}
}

// GetStatus handles GET /api/doorbell/status.
func (h *DoorbellHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(h.Service.Status())
}

// GetStream handles GET /api/doorbell/stream : instantanés en multipart/x-mixed-replace.
func (h *DoorbellHandler) GetStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings := h.Store.Doorbell()
	if h.Snapshot == nil || !settings.Enabled || settings.CameraID == "" {
		writeLightTimerError(w, http.StatusServiceUnavailable, "doorbell_unavailable")
		return
	}
	frame, err := h.Snapshot(settings.CameraID)
	if err != nil || len(frame) == 0 {
		// Ni l'adresse ni l'erreur de la console ne sont journalisées ni renvoyées.
		writeLightTimerError(w, http.StatusServiceUnavailable, "doorbell_unavailable")
		return
	}
	mw := multipart.NewWriter(w)
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+mw.Boundary())
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	interval := h.StreamInterval
	if interval <= 0 {
		interval = doorbellStreamInterval
	}
	maxDur := h.StreamMax
	if maxDur <= 0 {
		maxDur = doorbellStreamMax
	}
	deadline := time.NewTimer(maxDur)
	defer deadline.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		part, perr := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":   {"image/jpeg"},
			"Content-Length": {strconv.Itoa(len(frame))},
		})
		if perr != nil {
			return
		}
		if _, werr := part.Write(frame); werr != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			_ = mw.Close()
			return
		case <-tick.C:
		}
		next, err := h.Snapshot(settings.CameraID)
		if err != nil || len(next) == 0 {
			log.Printf("Sonnette : instantané indisponible, fin du flux")
			_ = mw.Close()
			return
		}
		frame = next
	}
}

// GetAdminSettings handles GET /api/admin/doorbell/settings.
func (h *DoorbellHandler) GetAdminSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.Store.Doorbell())
}

type doorbellSaved struct {
	core.DoorbellSettings
	Status string `json:"status"`
}

var errDoorbellSettings = errors.New("invalid_settings")

// parseDoorbellBody lit {"enabled","camera_id","lights","blinks"} (les quatre clés sont
// obligatoires) et, en option, "schedule" (absent : la plage actuelle est conservée).
func parseDoorbellBody(r *http.Request) (enabled bool, camera string, lights []string, blinks int, schedule *core.DoorbellSchedule, err error) {
	body, rerr := io.ReadAll(io.LimitReader(r.Body, doorbellMaxBody+1))
	defer r.Body.Close()
	if rerr != nil || len(body) > doorbellMaxBody {
		return false, "", nil, 0, nil, errDoorbellSettings
	}
	var in map[string]json.RawMessage
	if json.Unmarshal(body, &in) != nil || in == nil {
		return false, "", nil, 0, nil, errDoorbellSettings
	}
	get := func(key string, dst any) bool {
		raw, ok := in[key]
		return ok && string(raw) != "null" && json.Unmarshal(raw, dst) == nil
	}
	if !get("enabled", &enabled) || !get("camera_id", &camera) || !get("lights", &lights) || lights == nil {
		return false, "", nil, 0, nil, errDoorbellSettings
	}
	if !get("blinks", &blinks) {
		return false, "", nil, 0, nil, core.ErrInvalidBlinks
	}
	if raw, ok := in["schedule"]; ok {
		var sc core.DoorbellSchedule
		if string(raw) == "null" || json.Unmarshal(raw, &sc) != nil {
			return false, "", nil, 0, nil, core.ErrInvalidSchedule
		}
		schedule = &sc
	}
	return enabled, camera, lights, blinks, schedule, nil
}

func doorbellErrorKey(err error) (int, string) {
	switch {
	case errors.Is(err, core.ErrUnknownLight):
		return http.StatusBadRequest, "unknown_light"
	case errors.Is(err, core.ErrInvalidBlinks):
		return http.StatusBadRequest, "invalid_blinks"
	case errors.Is(err, core.ErrInvalidSchedule):
		return http.StatusBadRequest, "invalid_schedule"
	case errors.Is(err, core.ErrUnknownCamera):
		return http.StatusBadRequest, "unknown_camera"
	case errors.Is(err, core.ErrLightNamesUnavailable):
		return http.StatusServiceUnavailable, "names_unavailable"
	default:
		return http.StatusBadRequest, "invalid_settings"
	}
}

// checkCamera valide camera_id contre les caméras de la console.
// Retour : nil, ErrUnknownCamera, ou errDoorbellUnavailable (console injoignable ou UniFi désactivé).
var errDoorbellUnavailable = errors.New("doorbell_unavailable")

func (h *DoorbellHandler) checkCamera(enabled bool, id string) error {
	if id == "" {
		if enabled {
			return core.ErrUnknownCamera
		}
		return nil
	}
	if h.Cameras == nil {
		return errDoorbellUnavailable
	}
	cams, err := h.Cameras()
	if err != nil {
		return errDoorbellUnavailable
	}
	for _, c := range cams {
		if c.ID == id {
			return nil
		}
	}
	return core.ErrUnknownCamera
}

// PutAdminSettings handles PUT /api/admin/doorbell/settings.
func (h *DoorbellHandler) PutAdminSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	enabled, camera, lights, blinks, schedule, err := parseDoorbellBody(r)
	if err == nil {
		err = core.ValidateDoorbellBlinks(blinks)
	}
	if err == nil && schedule != nil {
		var n core.DoorbellSchedule
		if n, err = schedule.Normalize(); err == nil {
			schedule = &n
		}
	}
	if err == nil {
		err = core.ValidateDoorbellLights(lights)
	}
	if err == nil && !h.Store.Available() {
		err = core.ErrLightNamesUnavailable
	}
	if err == nil {
		err = h.checkCamera(enabled, camera)
	}
	if err != nil {
		if errors.Is(err, errDoorbellUnavailable) {
			writeLightTimerError(w, http.StatusServiceUnavailable, "doorbell_unavailable")
			return
		}
		code, key := doorbellErrorKey(err)
		writeLightTimerError(w, code, key)
		return
	}
	previous := h.Store.Doorbell()

	if testmode.IsDryRun(r) {
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		sched := previous.Schedule
		if schedule != nil {
			sched = *schedule
		}
		writeDoorbellSaved(w, doorbellSaved{
			DoorbellSettings: core.DoorbellSettings{Enabled: enabled, CameraID: camera, Lights: lights, Blinks: blinks, Schedule: sched, Version: previous.Version + 1, UpdatedAt: &now},
			Status:           "dry_run",
		})
		return
	}

	actor := audit.ActorIDFromRequest(r, "lan")
	saved, err := h.Store.SetDoorbellSchedule(enabled, camera, lights, blinks, schedule, actor, time.Now().UTC())
	if err != nil {
		code, key := doorbellErrorKey(err)
		writeLightTimerError(w, code, key)
		return
	}
	addDebugLog("Doorbell settings saved (v%d)", saved.Version)

	if h.Audit != nil && h.Audit.Enabled() {
		details := map[string]any{
			"event":    "doorbell_settings",
			"version":  saved.Version,
			"previous": doorbellAuditView(previous),
			"new":      doorbellAuditView(saved),
		}
		if u, ok := audit.UserFromRequest(r); ok {
			details["lan_user_id"] = u.ID
		}
		newDoc, _ := json.Marshal(doorbellAuditView(saved))
		_ = h.Audit.EmitUserAction(r.Context(), "lan_user", actor, "doorbell_settings", string(newDoc), details)
		if sd := doorbellScheduleAuditDetails(previous, saved); sd != nil {
			if u, ok := audit.UserFromRequest(r); ok {
				sd["lan_user_id"] = u.ID
			}
			newSched, _ := json.Marshal(saved.Schedule)
			_ = h.Audit.EmitUserAction(r.Context(), "lan_user", actor, "doorbell_schedule", string(newSched), sd)
		}
	}
	if h.OnSettingsChanged != nil {
		h.OnSettingsChanged()
	}
	writeDoorbellSaved(w, doorbellSaved{DoorbellSettings: saved, Status: "saved"})
}

// doorbellAuditView : l'identifiant de caméra n'est pas écrit dans le journal d'audit.
func doorbellAuditView(s core.DoorbellSettings) map[string]any {
	return map[string]any{"enabled": s.Enabled, "lights": s.Lights, "blinks": s.Blinks, "version": s.Version}
}

// doorbellScheduleAuditDetails décrit un changement de plage horaire (événement
// doorbell_schedule) ; nil quand la plage n'a pas changé.
func doorbellScheduleAuditDetails(previous, saved core.DoorbellSettings) map[string]any {
	if reflect.DeepEqual(previous.Schedule, saved.Schedule) {
		return nil
	}
	return map[string]any{
		"event":    "doorbell_schedule",
		"version":  saved.Version,
		"previous": previous.Schedule,
		"new":      saved.Schedule,
	}
}

// PostAdminTest handles POST /api/admin/doorbell/test : joue la séquence comme une vraie
// pression. En-tête X-Essensys-Test-Mode: dry-run : aucune écriture dans la table d'échange.
func (h *DoorbellHandler) PostAdminTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dry := testmode.IsDryRun(r)
	res := h.Service.Ring(dry)
	st := h.Service.Status()
	status := "started"
	if dry {
		status = "dry_run"
	}
	if h.Audit != nil && h.Audit.Enabled() && !dry {
		actor := audit.ActorIDFromRequest(r, "lan")
		_ = h.Audit.EmitUserAction(r.Context(), "lan_user", actor, "doorbell_test", fmt.Sprintf(`{"lights":%d}`, res.Lights), map[string]any{"event": "doorbell_test", "lights": res.Lights})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":        status,
		"ringing":       st.Ringing,
		"ringing_until": st.RingingUntil,
		"sequence":      res.Sequence,
		"lights":        res.Lights,
	})
}

func writeDoorbellSaved(w http.ResponseWriter, v doorbellSaved) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
