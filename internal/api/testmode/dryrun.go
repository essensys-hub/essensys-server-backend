package testmode

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

const (
	Header      = "X-Essensys-Test-Mode"
	QueryParam  = "test_mode"
	ValueDryRun = "dry-run"
	StatusOK    = "test_ok"
	StatusFail  = "test_failed"
)

// IsDryRun reports whether the request is a validation-only test (no firmware enqueue).
func IsDryRun(r *http.Request) bool {
	if strings.EqualFold(strings.TrimSpace(r.Header.Get(Header)), ValueDryRun) {
		return true
	}
	return r.URL.Query().Get(QueryParam) == "dry_run"
}

// ExchangeSnapshot returns current store values for the requested indices.
func ExchangeSnapshot(store data.Store, clientID string, params []protocol.ExchangeKV) []protocol.ExchangeKV {
	seen := make(map[int]struct{}, len(params))
	out := make([]protocol.ExchangeKV, 0, len(params))
	for _, p := range params {
		if _, ok := seen[p.K]; ok {
			continue
		}
		seen[p.K] = struct{}{}
		if v, ok := store.GetValue(clientID, p.K); ok {
			out = append(out, protocol.ExchangeKV{K: p.K, V: v})
		}
	}
	return out
}

// WriteOK sends a dry-run success JSON response.
func WriteOK(w http.ResponseWriter, validated []protocol.ExchangeKV, snapshot []protocol.ExchangeKV, message string) {
	writeDryRun(w, http.StatusOK, StatusOK, validated, snapshot, message)
}

// WriteFailed sends a dry-run validation failure.
func WriteFailed(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  StatusFail,
		"dry_run": true,
		"message": message,
	})
}

func writeDryRun(w http.ResponseWriter, code int, status string, validated, snapshot []protocol.ExchangeKV, message string) {
	if message == "" {
		message = "Validation OK — non envoyé à l'armoire"
	}
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":            status,
		"dry_run":           true,
		"validated_params":  validated,
		"exchange_snapshot": snapshot,
		"message":           message,
	})
}
