package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/api/testmode"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/scenario"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func (h *Handler) scenarios() *scenario.Service {
	return scenario.NewService(h.store, h.actionService)
}

// HandleScenarios routes /api/scenarios and subpaths.
func (h *Handler) HandleScenarios(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/scenarios")
	path = strings.Trim(path, "/")

	if path == "" || path == "/" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		h.listScenarios(w, r)
		return
	}

	parts := strings.Split(path, "/")
	switch parts[0] {
	case "meta":
		if len(parts) == 2 && parts[1] == "bitmasks" && r.Method == http.MethodGet {
			writeScenarioJSON(w, http.StatusOK, map[string]any{
				"fields": scenario.BitmaskMeta(),
			})
			return
		}
	case "restore":
		// POST /api/scenarios/{slot}/restore handled below via slot parsing
	}

	if len(parts) >= 1 {
		slot, err := strconv.Atoi(parts[0])
		if err != nil || slot < 1 || slot > scenario.SlotCount {
			http.Error(w, "Invalid slot", http.StatusBadRequest)
			return
		}

		if len(parts) == 1 {
			switch r.Method {
			case http.MethodGet:
				h.getScenario(w, r, slot)
			case http.MethodPut:
				h.putScenario(w, r, slot)
			default:
				methodNotAllowed(w)
			}
			return
		}

		if len(parts) == 2 {
			switch parts[1] {
			case "launch":
				if r.Method == http.MethodPost {
					h.launchScenario(w, r, slot)
					return
				}
			case "restore":
				if r.Method == http.MethodPost {
					h.restoreScenario(w, r, slot)
					return
				}
			}
		}
	}

	http.NotFound(w, r)
}

func (h *Handler) listScenarios(w http.ResponseWriter, r *http.Request) {
	clientID := scenarioClientID(r)
	slots, err := h.scenarios().List(clientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeScenarioJSON(w, http.StatusOK, map[string]any{"slots": slots})
}

func (h *Handler) getScenario(w http.ResponseWriter, r *http.Request, slot int) {
	clientID := scenarioClientID(r)
	detail, err := h.scenarios().Get(clientID, slot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeScenarioJSON(w, http.StatusOK, detail)
}

func (h *Handler) putScenario(w http.ResponseWriter, r *http.Request, slot int) {
	clientID := scenarioClientID(r)

	var body struct {
		Params map[int]string `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(body.Params) == 0 {
		http.Error(w, "params required", http.StatusBadRequest)
		return
	}

	if testmode.IsDryRun(r) {
		chunks, err := scenario.WriteDefinitionChunks(slot, body.Params)
		if err != nil {
			if err == scenario.ErrSlot1ServerReserved {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			testmode.WriteFailed(w, err.Error())
			return
		}
		flat := flattenScenarioChunks(chunks)
		snap := testmode.ExchangeSnapshot(h.store, clientID, flat)
		testmode.WriteOK(w, flat, snap, "")
		return
	}

	guids, err := h.scenarios().Put(clientID, slot, body.Params)
	if err != nil {
		if err == scenario.ErrSlot1ServerReserved {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeScenarioJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"guids":  guids,
	})
}

func (h *Handler) launchScenario(w http.ResponseWriter, r *http.Request, slot int) {
	clientID := scenarioClientID(r)
	params, paramErr := scenario.LaunchParams(slot)
	if testmode.IsDryRun(r) {
		if paramErr != nil {
			testmode.WriteFailed(w, paramErr.Error())
			return
		}
		snap := testmode.ExchangeSnapshot(h.store, clientID, params)
		testmode.WriteOK(w, params, snap, "")
		return
	}
	guid, err := h.scenarios().Launch(clientID, slot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeScenarioJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"guid":   guid,
		"slot":   slot,
	})
}

func (h *Handler) restoreScenario(w http.ResponseWriter, r *http.Request, slot int) {
	clientID := scenarioClientID(r)
	params, paramErr := scenario.RestorePresetParams(slot)
	if testmode.IsDryRun(r) {
		if paramErr != nil {
			testmode.WriteFailed(w, paramErr.Error())
			return
		}
		snap := testmode.ExchangeSnapshot(h.store, clientID, params)
		testmode.WriteOK(w, params, snap, "")
		return
	}
	guid, err := h.scenarios().Restore(clientID, slot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeScenarioJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"guid":   guid,
		"slot":   slot,
	})
}

func scenarioClientID(r *http.Request) string {
	if id, ok := middleware.GetClientID(r); ok {
		return id
	}
	return "default"
}

func writeScenarioJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func flattenScenarioChunks(chunks [][]protocol.ExchangeKV) []protocol.ExchangeKV {
	out := make([]protocol.ExchangeKV, 0)
	for _, c := range chunks {
		out = append(out, c...)
	}
	return out
}
