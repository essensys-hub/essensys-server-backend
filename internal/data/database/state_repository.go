package database

import (
	"database/sql"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
)

// StateRepository handles database operations for states
type StateRepository struct {
	db *sqlx.DB
}

// NewStateRepository creates a new StateRepository
func NewStateRepository(db *sqlx.DB) *StateRepository {
	return &StateRepository{db: db}
}

// Create creates a new state with its indexes
func (r *StateRepository) Create(state *models.State, indexes []models.StateIndex) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert state
	stateQuery := `INSERT INTO es_state (
		machine_id, version, completed, state_date
	) VALUES (
		:machine_id, :version, :completed, :state_date
	) RETURNING id`

	rows, err := tx.NamedQuery(stateQuery, state)
	if err != nil {
		return err
	}
	defer rows.Close()

	if !rows.Next() {
		return sql.ErrNoRows
	}
	if err := rows.Scan(&state.ID); err != nil {
		return err
	}

	// Insert state indexes
	for i := range indexes {
		indexes[i].StateID = state.ID
		indexQuery := `INSERT INTO es_state_index (state_id, index_id, value, updated_at)
			VALUES (:state_id, :index_id, :value, :updated_at)
			ON CONFLICT (state_id, index_id) 
			DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`
		if _, err := tx.NamedExec(indexQuery, indexes[i]); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetLatestByMachineID retrieves the latest state for a machine
func (r *StateRepository) GetLatestByMachineID(machineID int) (*models.State, error) {
	var state models.State
	query := `SELECT * FROM es_state 
		WHERE machine_id = $1 
		ORDER BY state_date DESC 
		LIMIT 1`
	err := r.db.Get(&state, query, machineID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// GetByMachineIDAfter retrieves states after a specific date
func (r *StateRepository) GetByMachineIDAfter(machineID int, after time.Time) ([]*models.State, error) {
	var states []*models.State
	query := `SELECT * FROM es_state 
		WHERE machine_id = $1 AND state_date > $2
		ORDER BY state_date ASC`
	err := r.db.Select(&states, query, machineID, after)
	if err != nil {
		return nil, err
	}
	return states, nil
}

// GetIndexesByStateID retrieves all indexes for a state
func (r *StateRepository) GetIndexesByStateID(stateID int) ([]models.StateIndex, error) {
	var indexes []models.StateIndex
	query := `SELECT si.*, di.index_key 
		FROM es_state_index si
		INNER JOIN es_data_index di ON si.index_id = di.id
		WHERE si.state_id = $1`
	err := r.db.Select(&indexes, query, stateID)
	if err != nil {
		return nil, err
	}
	return indexes, nil
}

// GetLastCallTime retrieves the last call time for a machine
func (r *StateRepository) GetLastCallTime(machineID int) (time.Time, error) {
	var lastCall sql.NullTime
	query := `SELECT MAX(state_date) FROM es_state WHERE machine_id = $1`
	err := r.db.Get(&lastCall, query, machineID)
	if err == sql.ErrNoRows || !lastCall.Valid {
		// Return zero time if no state exists
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return lastCall.Time, nil
}

// HasRefreshed checks if a machine has refreshed since a specific time
func (r *StateRepository) HasRefreshed(machineID int, since time.Time) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM es_state 
		WHERE machine_id = $1 AND state_date > $2`
	err := r.db.Get(&count, query, machineID, since)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

