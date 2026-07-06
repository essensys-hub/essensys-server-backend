package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/audit"
	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
)

type AuditHandler struct {
	authorizer *audit.Authorizer
	events     *audit.EventRepository
	charter    *audit.CharterRepository
	audit      *audit.Service
	machineID  int
}

func NewAuditHandler(auth *audit.Authorizer, events *audit.EventRepository, charter *audit.CharterRepository, machineID int, auditSvc *audit.Service) *AuditHandler {
	return &AuditHandler{
		authorizer: auth,
		events:     events,
		charter:    charter,
		audit:      auditSvc,
		machineID:  machineID,
	}
}

func (h *AuditHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := audit.UserFromRequest(r)
	if !ok || !h.authorizer.CanReadAudit(user.Role) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	accepted, err := h.authorizer.HasAcceptedCharter(user.ID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if !accepted {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "charter_required"})
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	filter := audit.ListFilter{
		MachineID: h.machineID,
		Query:     strings.TrimSpace(q.Get("q")),
		EventType: strings.TrimSpace(q.Get("event_type")),
		Limit:     limit,
		Offset:    offset,
	}
	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			filter.From = &t
		}
	}
	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			filter.To = &t
		}
	}

	rows, err := h.events.List(r.Context(), filter)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":     maskEventIPs(rows),
		"machine_id": h.machineID,
	})
}

func maskEventIPs(rows []audit.EventRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m := map[string]any{
			"event_id":     row.EventID,
			"machine_id":   row.MachineID,
			"occurred_at":  row.OccurredAt,
			"ingested_at":  row.IngestedAt,
			"event_type":   row.EventType,
			"actor_type":   row.ActorType,
			"subject_type": row.SubjectType,
			"subject_key":  row.SubjectKey,
			"source":       row.Source,
			"pending_sync": row.PendingSync,
			"event_hash":   row.EventHash,
		}
		if row.ActorID.Valid {
			m["actor_id"] = row.ActorID.String
		}
		if row.OldValue.Valid {
			m["old_value"] = row.OldValue.String
		}
		if row.NewValue.Valid {
			m["new_value"] = row.NewValue.String
		}
		if row.Details.Valid && row.Details.String != "" {
			m["details"] = json.RawMessage(row.Details.String)
		}
		out = append(out, m)
	}
	return out
}

func (h *AuditHandler) CharterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := audit.UserFromRequest(r)
	if !ok || !h.authorizer.CanReadAudit(user.Role) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	accepted, err := h.authorizer.HasAcceptedCharter(user.ID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"charter_version": audit.CharterVersion,
		"accepted":        accepted,
		"text":            audit.CharterText(),
	})
}

func (h *AuditHandler) AcceptCharter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := audit.UserFromRequest(r)
	if !ok || !h.authorizer.CanReadAudit(user.Role) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if err := h.charter.Accept(user.ID, audit.CharterVersion, laniam.ClientIPFromRequest(r)); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if h.audit != nil && h.audit.Enabled() {
		_ = h.audit.EmitAuthEvent(r.Context(), user.Email, "audit_charter", audit.CharterVersion)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuditHandler) ExportEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := audit.UserFromRequest(r)
	if !ok || !h.authorizer.CanReadAudit(user.Role) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	accepted, _ := h.authorizer.HasAcceptedCharter(user.ID)
	if !accepted {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "json"
	}
	rows, err := h.events.List(r.Context(), audit.ListFilter{MachineID: h.machineID, Limit: 5000})
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=audit-export.csv")
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"occurred_at", "event_type", "actor_id", "subject_key", "new_value", "event_hash"})
		for _, row := range rows {
			actor := ""
			if row.ActorID.Valid {
				actor = row.ActorID.String
			}
			newVal := ""
			if row.NewValue.Valid {
				newVal = row.NewValue.String
			}
			_ = cw.Write([]string{
				row.OccurredAt.Format(time.RFC3339),
				row.EventType,
				actor,
				row.SubjectKey,
				newVal,
				row.EventHash,
			})
		}
		cw.Flush()
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"events": maskEventIPs(rows)})
	}
}

func (h *AuditHandler) Integrity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := audit.UserFromRequest(r)
	if !ok || !h.authorizer.CanAdminAudit(user.Role) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	count, err := h.events.Count(r.Context(), h.machineID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"machine_id":  h.machineID,
		"event_count": count,
		"message":     fmt.Sprintf("projection contains %d events", count),
	})
}
