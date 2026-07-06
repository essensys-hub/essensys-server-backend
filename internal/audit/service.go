package audit

import (
	"context"
	"encoding/json"
	"time"

	auditcollector "github.com/essensys-hub/essensys-audit-collector"
	"github.com/google/uuid"
)

// Service wraps the audit collector for server-backend producers.
type Service struct {
	collector *auditcollector.Collector
	machineID int
	gatewayID string
	dedup     *auditcollector.Dedup
}

type Config struct {
	ServiceURL string
	APIToken   string
	MachineID  int
	GatewayID  string
}

func NewService(cfg Config, outbox *OutboxRepository) *Service {
	client := auditcollector.NewClient(cfg.ServiceURL, cfg.APIToken, nil)
	var ob auditcollector.OutboxStore
	if outbox != nil {
		ob = outbox
	}
	return &Service{
		collector: auditcollector.NewCollector(client, ob, auditcollector.NewDedup()),
		machineID: cfg.MachineID,
		gatewayID: cfg.GatewayID,
	}
}

func (s *Service) Enabled() bool {
	return s != nil && s.collector != nil && s.machineID > 0
}

func (s *Service) Emit(ctx context.Context, ev auditcollector.AuditEvent) error {
	if !s.Enabled() {
		return nil
	}
	if ev.EventID == uuid.Nil {
		ev.EventID = uuid.New()
	}
	if ev.MachineID == 0 {
		ev.MachineID = s.machineID
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	if ev.Source == "" {
		ev.Source = "gateway"
	}
	return s.collector.Emit(ctx, ev)
}

func (s *Service) EmitUserAction(ctx context.Context, actorType, actorID, subjectKey, newValue string, details map[string]any) error {
	var raw json.RawMessage
	if details != nil {
		b, _ := json.Marshal(details)
		raw = b
	}
	return s.Emit(ctx, auditcollector.AuditEvent{
		EventType:   "USER_ACTION",
		ActorType:   actorType,
		ActorID:     actorID,
		SubjectType: "exchange",
		SubjectKey:  subjectKey,
		NewValue:    newValue,
		Details:     raw,
	})
}

func (s *Service) EmitAuthEvent(ctx context.Context, actorID, subjectKey, detail string) error {
	return s.Emit(ctx, auditcollector.AuditEvent{
		EventType:   "AUTH_EVENT",
		ActorType:   "lan_user",
		ActorID:     actorID,
		SubjectType: "auth",
		SubjectKey:  subjectKey,
		NewValue:    detail,
	})
}

func (s *Service) EmitStateChange(ctx context.Context, subjectKey, oldVal, newVal string, details map[string]any) error {
	var raw json.RawMessage
	if details != nil {
		b, _ := json.Marshal(details)
		raw = b
	}
	return s.Emit(ctx, auditcollector.AuditEvent{
		EventType:   "STATE_CHANGE",
		ActorType:   "system",
		ActorID:     "exchange",
		SubjectType: "exchange",
		SubjectKey:  subjectKey,
		OldValue:    oldVal,
		NewValue:    newVal,
		Details:     raw,
	})
}

// EmitFirmwareAck records firmware confirmation of an injected action (POST /api/done/{guid}).
func (s *Service) EmitFirmwareAck(ctx context.Context, clientID, guid, subjectKey, previousVal, receivedVal string) error {
	details := map[string]any{
		"guid":             guid,
		"client_id":        clientID,
		"lifecycle_status": "acknowledged",
		"ack_status":       "received",
		"received_value":   receivedVal,
	}
	if previousVal != "" {
		details["previous_value"] = previousVal
	}
	raw, _ := json.Marshal(details)
	return s.Emit(ctx, auditcollector.AuditEvent{
		EventType:   "USER_ACTION",
		ActorType:   "firmware",
		ActorID:     clientID,
		SubjectType: "exchange",
		SubjectKey:  subjectKey,
		NewValue:    receivedVal,
		OldValue:    previousVal,
		Details:     raw,
	})
}
