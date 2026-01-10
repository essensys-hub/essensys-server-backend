package database

import (
	"database/sql"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
)

// UserRepository handles database operations for users
type UserRepository struct {
	db *sqlx.DB
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(id int) (*models.User, error) {
	var user models.User
	query := `SELECT * FROM es_user WHERE id = $1`
	err := r.db.Get(&user, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByEmail retrieves a user by email
func (r *UserRepository) GetByEmail(email string) (*models.User, error) {
	var user models.User
	query := `SELECT * FROM es_user WHERE mail = $1`
	err := r.db.Get(&user, query, email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByGuid retrieves a user by GUID and email
func (r *UserRepository) GetByGuid(guid, email string) (*models.User, error) {
	var user models.User
	query := `SELECT * FROM es_user WHERE guid = $1 AND mail = $2`
	err := r.db.Get(&user, query, guid, email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Create creates a new user
func (r *UserRepository) Create(user *models.User) error {
	query := `INSERT INTO es_user (
		mail, password, nom, prenom, adr1, adr2, cp, ville, phone,
		question, reponse, isvalid, send_infos, obsolete,
		date_creation, guid, machine_id
	) VALUES (
		:mail, :password, :nom, :prenom, :adr1, :adr2, :cp, :ville, :phone,
		:question, :reponse, :isvalid, :send_infos, :obsolete,
		:date_creation, :guid, :machine_id
	) RETURNING id`

	rows, err := r.db.NamedQuery(query, user)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		return rows.Scan(&user.ID)
	}
	return sql.ErrNoRows
}

// Update updates an existing user
func (r *UserRepository) Update(user *models.User) error {
	query := `UPDATE es_user SET
		mail = :mail,
		password = :password,
		nom = :nom,
		prenom = :prenom,
		adr1 = :adr1,
		adr2 = :adr2,
		cp = :cp,
		ville = :ville,
		phone = :phone,
		question = :question,
		reponse = :reponse,
		isvalid = :isvalid,
		send_infos = :send_infos,
		obsolete = :obsolete,
		date_cloture = :date_cloture,
		last_access = :last_access,
		guid = :guid,
		machine_id = :machine_id
	WHERE id = :id`

	_, err := r.db.NamedExec(query, user)
	return err
}

// UpdateLastAccess updates the last access timestamp
func (r *UserRepository) UpdateLastAccess(userID int) error {
	query := `UPDATE es_user SET last_access = $1 WHERE id = $2`
	_, err := r.db.Exec(query, time.Now(), userID)
	return err
}

// Delete soft-deletes a user (sets obsolete = true)
func (r *UserRepository) Delete(id int) error {
	now := time.Now()
	query := `UPDATE es_user SET obsolete = true, date_cloture = $1 WHERE id = $2`
	_, err := r.db.Exec(query, now, id)
	return err
}

// CheckEmailExists checks if an email already exists
func (r *UserRepository) CheckEmailExists(email string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM es_user WHERE mail = $1`
	err := r.db.Get(&count, query, email)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CheckNoSerieExists checks if a serial number is already associated with a machine
func (r *UserRepository) CheckNoSerieExists(noSerie string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM es_cle_machine WHERE cle = $1 AND machine_id IS NULL`
	err := r.db.Get(&count, query, noSerie)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}


