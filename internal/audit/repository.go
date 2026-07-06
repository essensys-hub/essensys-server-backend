package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	auditcollector "github.com/essensys-hub/essensys-audit-collector"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// OutboxRepository persists events when audit-service is unreachable.
type OutboxRepository struct {
	db *sqlx.DB
}

func NewOutboxRepository(db *sqlx.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) Insert(event auditcollector.AuditEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		INSERT INTO audit_outbox (event_id, payload_json, sync_status)
		VALUES ($1, $2::jsonb, 'pending')
		ON CONFLICT (event_id) DO NOTHING`,
		event.EventID, string(payload))
	return err
}

// EventRepository reads projected audit events from PostgreSQL.
type EventRepository struct {
	db *sqlx.DB
}

func NewEventRepository(db *sqlx.DB) *EventRepository {
	return &EventRepository{db: db}
}

type EventRow struct {
	EventID     uuid.UUID       `db:"event_id" json:"event_id"`
	MachineID   int             `db:"machine_id" json:"machine_id"`
	OccurredAt  time.Time       `db:"occurred_at" json:"occurred_at"`
	IngestedAt  time.Time       `db:"ingested_at" json:"ingested_at"`
	EventType   string          `db:"event_type" json:"event_type"`
	ActorType   string          `db:"actor_type" json:"actor_type"`
	ActorID     sql.NullString  `db:"actor_id" json:"actor_id"`
	SubjectType string          `db:"subject_type" json:"subject_type"`
	SubjectKey  string          `db:"subject_key" json:"subject_key"`
	OldValue    sql.NullString  `db:"old_value" json:"old_value"`
	NewValue    sql.NullString  `db:"new_value" json:"new_value"`
	Details     sql.NullString  `db:"details" json:"details,omitempty"`
	Source      string          `db:"source" json:"source"`
	PendingSync bool            `db:"pending_sync" json:"pending_sync"`
	EventHash   string          `db:"event_hash" json:"event_hash"`
}

type ListFilter struct {
	MachineID int
	Query     string
	EventType string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
}

func (r *EventRepository) List(ctx context.Context, f ListFilter) ([]EventRow, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	q := `
		SELECT event_id, machine_id, occurred_at, ingested_at, event_type, actor_type, actor_id,
		       subject_type, subject_key, old_value, new_value, details, source, pending_sync, event_hash
		FROM armoire_audit_events
		WHERE machine_id = $1`
	args := []any{f.MachineID}
	n := 2
	if f.EventType != "" {
		q += fmt.Sprintf(" AND event_type = $%d", n)
		args = append(args, f.EventType)
		n++
	}
	if f.Query != "" {
		q += fmt.Sprintf(` AND (
			COALESCE(actor_id,'') ILIKE $%d OR subject_key ILIKE $%d OR
			COALESCE(new_value,'') ILIKE $%d OR COALESCE(old_value,'') ILIKE $%d)`, n, n, n, n)
		args = append(args, "%"+f.Query+"%")
		n++
	}
	if f.From != nil {
		q += fmt.Sprintf(" AND occurred_at >= $%d", n)
		args = append(args, *f.From)
		n++
	}
	if f.To != nil {
		q += fmt.Sprintf(" AND occurred_at <= $%d", n)
		args = append(args, *f.To)
		n++
	}
	q += fmt.Sprintf(" ORDER BY occurred_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, f.Limit, f.Offset)

	var rows []EventRow
	err := r.db.SelectContext(ctx, &rows, q, args...)
	return rows, err
}

func (r *EventRepository) Count(ctx context.Context, machineID int) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM armoire_audit_events WHERE machine_id = $1`, machineID)
	return n, err
}

// CharterRepository tracks LAN charter acceptances.
type CharterRepository struct {
	db *sqlx.DB
}

func NewCharterRepository(db *sqlx.DB) *CharterRepository {
	return &CharterRepository{db: db}
}

func (r *CharterRepository) HasAccepted(lanUserID int, version string) (bool, error) {
	var n int
	err := r.db.Get(&n, `
		SELECT COUNT(*) FROM lan_audit_charter_acceptances
		WHERE lan_user_id = $1 AND charter_version = $2`, lanUserID, version)
	return n > 0, err
}

func (r *CharterRepository) Accept(lanUserID int, version, ip string) error {
	_, err := r.db.Exec(`
		INSERT INTO lan_audit_charter_acceptances (lan_user_id, charter_version, ip_address)
		VALUES ($1, $2, NULLIF($3,'')::inet)
		ON CONFLICT (lan_user_id, charter_version) DO NOTHING`,
		lanUserID, version, ip)
	return err
}

func (r *CharterRepository) CurrentVersion() string {
	return CharterVersion
}

// CharterText returns the RGPD charter v1 text shown in the UI modal.
func CharterText() string {
	return `Charte d'utilisation du journal d'activité Essensys (v1)

Le journal enregistre les actions domotiques et changements d'état de votre installation
(scénarios, éclairage, alarme, authentifications LAN) à des fins de sécurité et de transparence
au sein du foyer.

Base légale : intérêt légitime (sécurité) et exécution du contrat de service.
Conservation : 24 mois sur la copie consultable ; preuve immuable conservée sur le registre local.
Vos adresses IP sont stockées mais affichées partiellement (/24).

En acceptant, vous confirmez avoir pris connaissance de cette charte et autorisez la consultation
du journal partagé par les utilisateurs autorisés de cette gateway.`
}
