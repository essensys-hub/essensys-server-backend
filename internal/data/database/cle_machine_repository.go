package database

import (
	"database/sql"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
)

// CleMachineRepository handles database operations for cle_machine
type CleMachineRepository struct {
	db *sqlx.DB
}

// NewCleMachineRepository creates a new CleMachineRepository
func NewCleMachineRepository(db *sqlx.DB) *CleMachineRepository {
	return &CleMachineRepository{db: db}
}

// GetByCle retrieves a cle_machine by its cle (serial number)
func (r *CleMachineRepository) GetByCle(cle string) (*models.CleMachine, error) {
	var cleMachine models.CleMachine
	query := `SELECT * FROM es_cle_machine WHERE cle = $1`
	err := r.db.Get(&cleMachine, query, cle)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cleMachine, nil
}

// GetByID retrieves a cle_machine by ID
func (r *CleMachineRepository) GetByID(id int) (*models.CleMachine, error) {
	var cleMachine models.CleMachine
	query := `SELECT * FROM es_cle_machine WHERE id = $1`
	err := r.db.Get(&cleMachine, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cleMachine, nil
}

// Create creates a new cle_machine
func (r *CleMachineRepository) Create(cleMachine *models.CleMachine) error {
	query := `INSERT INTO es_cle_machine (cle, date_generation, date_activation, machine_id)
		VALUES (:cle, :date_generation, :date_activation, :machine_id) RETURNING id`

	rows, err := r.db.NamedQuery(query, cleMachine)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		return rows.Scan(&cleMachine.ID)
	}
	return sql.ErrNoRows
}

// Update updates an existing cle_machine
func (r *CleMachineRepository) Update(cleMachine *models.CleMachine) error {
	query := `UPDATE es_cle_machine SET
		cle = :cle,
		date_generation = :date_generation,
		date_activation = :date_activation,
		machine_id = :machine_id
	WHERE id = :id`

	_, err := r.db.NamedExec(query, cleMachine)
	return err
}

// CountByCleAndMachineNull counts cle_machine with specific cle and null machine_id
func (r *CleMachineRepository) CountByCleAndMachineNull(cle string) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM es_cle_machine WHERE cle = $1 AND machine_id IS NULL`
	err := r.db.Get(&count, query, cle)
	return count, err
}

