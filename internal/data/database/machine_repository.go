package database

import (
	"database/sql"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
)

// MachineRepository handles database operations for machines
type MachineRepository struct {
	db *sqlx.DB
}

// NewMachineRepository creates a new MachineRepository
func NewMachineRepository(db *sqlx.DB) *MachineRepository {
	return &MachineRepository{db: db}
}

// GetByID retrieves a machine by ID
func (r *MachineRepository) GetByID(id int) (*models.Machine, error) {
	var machine models.Machine
	query := `SELECT * FROM es_machine WHERE id = $1`
	err := r.db.Get(&machine, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &machine, nil
}

// GetByNoSerie retrieves a machine by serial number
func (r *MachineRepository) GetByNoSerie(noSerie string) (*models.Machine, error) {
	var machine models.Machine
	query := `SELECT * FROM es_machine WHERE no_serie = $1`
	err := r.db.Get(&machine, query, noSerie)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &machine, nil
}

// Create creates a new machine
func (r *MachineRepository) Create(machine *models.Machine) error {
	query := `INSERT INTO es_machine (
		no_serie, version, pkey, hashed_pkey, autorise_alarme,
		is_active, date_creation, date_modification
	) VALUES (
		:no_serie, :version, :pkey, :hashed_pkey, :autorise_alarme,
		:is_active, :date_creation, :date_modification
	) RETURNING id`

	rows, err := r.db.NamedQuery(query, machine)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		return rows.Scan(&machine.ID)
	}
	return sql.ErrNoRows
}

// Update updates an existing machine
func (r *MachineRepository) Update(machine *models.Machine) error {
	machine.DateModification = time.Now()
	query := `UPDATE es_machine SET
		no_serie = :no_serie,
		version = :version,
		pkey = :pkey,
		hashed_pkey = :hashed_pkey,
		autorise_alarme = :autorise_alarme,
		is_active = :is_active,
		date_modification = :date_modification
	WHERE id = :id`

	_, err := r.db.NamedExec(query, machine)
	return err
}

// GetByUserID retrieves the machine associated with a user
func (r *MachineRepository) GetByUserID(userID int) (*models.Machine, error) {
	var machine models.Machine
	query := `SELECT m.* FROM es_machine m
		INNER JOIN es_user u ON u.machine_id = m.id
		WHERE u.id = $1`
	err := r.db.Get(&machine, query, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &machine, nil
}

// GetByHashedPkey retrieves a machine by its hashed activation key
// This is used for legacy IoT client authentication (Basic Auth)
// The client sends: Base64(MD5(cle_serveur_hex)[0:16]:MD5(cle_serveur_hex)[16:32])
// The server decodes Base64, concatenates username+password, and searches for matching HashedPkey
func (r *MachineRepository) GetByHashedPkey(hashedPkey string) (*models.Machine, error) {
	var machine models.Machine
	query := `SELECT * FROM es_machine WHERE hashed_pkey = $1 AND is_active = true`
	err := r.db.Get(&machine, query, hashedPkey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &machine, nil
}

