package models

import (
	"time"
)

// DataIndex représente un index de données (référentiel des indices)
// Les indices sont des clés numériques (ex: "605", "613") qui correspondent
// à des paramètres du système (lumières, volets, chauffage, etc.)
type DataIndex struct {
	ID             int       `db:"id" json:"id"`
	IndexKey       string    `db:"index_key" json:"index_key"` // Ex: "605", "613"
	IsActive       bool      `db:"is_active" json:"is_active"`
	DateCreation   time.Time `db:"date_creation" json:"date_creation"`
	DateModification time.Time `db:"date_modification" json:"date_modification"`
}

// TableName retourne le nom de la table
func (DataIndex) TableName() string {
	return "es_data_index"
}




