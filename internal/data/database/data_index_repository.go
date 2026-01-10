package database

import (
	"database/sql"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
)

// DataIndexRepository handles database operations for data indexes
type DataIndexRepository struct {
	db *sqlx.DB
}

// NewDataIndexRepository creates a new DataIndexRepository
func NewDataIndexRepository(db *sqlx.DB) *DataIndexRepository {
	return &DataIndexRepository{db: db}
}

// GetByKey retrieves a data index by its key (e.g., "605", "613")
func (r *DataIndexRepository) GetByKey(key string) (*models.DataIndex, error) {
	var index models.DataIndex
	query := `SELECT * FROM es_data_index WHERE index_key = $1`
	err := r.db.Get(&index, query, key)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &index, nil
}

// GetByID retrieves a data index by ID
func (r *DataIndexRepository) GetByID(id int) (*models.DataIndex, error) {
	var index models.DataIndex
	query := `SELECT * FROM es_data_index WHERE id = $1`
	err := r.db.Get(&index, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &index, nil
}

// GetOrCreateByKey retrieves an index by key, creating it if it doesn't exist
func (r *DataIndexRepository) GetOrCreateByKey(key string) (*models.DataIndex, error) {
	index, err := r.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if index != nil {
		return index, nil
	}

	// Create new index
	newIndex := &models.DataIndex{
		IndexKey: key,
		IsActive: true,
	}
	query := `INSERT INTO es_data_index (index_key, is_active) 
		VALUES (:index_key, :is_active) RETURNING id`
	rows, err := r.db.NamedQuery(query, newIndex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&newIndex.ID); err != nil {
			return nil, err
		}
	}
	return newIndex, nil
}

// GetAllActive retrieves all active data indexes
func (r *DataIndexRepository) GetAllActive() ([]models.DataIndex, error) {
	var indexes []models.DataIndex
	query := `SELECT * FROM es_data_index WHERE is_active = true ORDER BY index_key`
	err := r.db.Select(&indexes, query)
	if err != nil {
		return nil, err
	}
	return indexes, nil
}


