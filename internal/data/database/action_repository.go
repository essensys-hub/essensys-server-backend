package database

import (
	"database/sql"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
)

// ActionRepository handles database operations for actions
type ActionRepository struct {
	db *sqlx.DB
}

// NewActionRepository creates a new ActionRepository
func NewActionRepository(db *sqlx.DB) *ActionRepository {
	return &ActionRepository{db: db}
}

// Create creates a new action with its indexes
func (r *ActionRepository) Create(action *models.Action, indexes []models.ActionIndex) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert action
	actionQuery := `INSERT INTO es_action (
		machine_id, guid, action_type, action_info, is_done, date_creation
	) VALUES (
		:machine_id, :guid, :action_type, :action_info, :is_done, :date_creation
	) RETURNING id`

	rows, err := tx.NamedQuery(actionQuery, action)
	if err != nil {
		return err
	}
	defer rows.Close()

	if !rows.Next() {
		return sql.ErrNoRows
	}
	if err := rows.Scan(&action.ID); err != nil {
		return err
	}

	// Insert action indexes
	for i := range indexes {
		indexes[i].ActionID = action.ID
		indexQuery := `INSERT INTO es_action_index (action_id, index_id, value)
			VALUES (:action_id, :index_id, :value)`
		if _, err := tx.NamedExec(indexQuery, indexes[i]); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetByGUID retrieves an action by GUID
func (r *ActionRepository) GetByGUID(guid string) (*models.Action, error) {
	var action models.Action
	query := `SELECT * FROM es_action WHERE guid = $1`
	err := r.db.Get(&action, query, guid)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &action, nil
}

// GetPendingByMachineID retrieves all pending actions for a machine
func (r *ActionRepository) GetPendingByMachineID(machineID int) ([]*models.Action, error) {
	var actions []*models.Action
	query := `SELECT * FROM es_action 
		WHERE machine_id = $1 AND is_done = false 
		ORDER BY date_creation ASC`
	err := r.db.Select(&actions, query, machineID)
	if err != nil {
		return nil, err
	}
	return actions, nil
}

// GetIndexesByActionID retrieves all indexes for an action
func (r *ActionRepository) GetIndexesByActionID(actionID int) ([]models.ActionIndex, error) {
	var indexes []models.ActionIndex
	query := `SELECT * FROM es_action_index WHERE action_id = $1`
	err := r.db.Select(&indexes, query, actionID)
	if err != nil {
		return nil, err
	}
	return indexes, nil
}

// MarkDone marks an action as done
func (r *ActionRepository) MarkDone(guid string) error {
	query := `UPDATE es_action SET is_done = true WHERE guid = $1`
	result, err := r.db.Exec(query, guid)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteByMachineID deletes all actions for a machine (purge)
func (r *ActionRepository) DeleteByMachineID(machineID int) error {
	query := `DELETE FROM es_action WHERE machine_id = $1 AND is_done = false`
	_, err := r.db.Exec(query, machineID)
	return err
}

// GetByMachineIDAndDateRange retrieves actions in a date range
func (r *ActionRepository) GetByMachineIDAndDateRange(machineID int, start, end time.Time) ([]*models.Action, error) {
	var actions []*models.Action
	query := `SELECT * FROM es_action 
		WHERE machine_id = $1 AND date_creation BETWEEN $2 AND $3
		ORDER BY date_creation ASC`
	err := r.db.Select(&actions, query, machineID, start, end)
	if err != nil {
		return nil, err
	}
	return actions, nil
}

