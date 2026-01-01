package models

import (
	"time"
)

// Machine représente un boîtier Essensys (terminal)
type Machine struct {
	ID             int       `db:"id" json:"id"`
	NoSerie        string    `db:"no_serie" json:"no_serie"`
	Version        string    `db:"version" json:"version,omitempty"`
	Pkey           string    `db:"pkey" json:"-"` // Clé privée (ne jamais exposer)
	HashedPkey     string    `db:"hashed_pkey" json:"-"` // Clé hashée
	AutoriseAlarme bool      `db:"autorise_alarme" json:"autorise_alarme"`
	IsActive       bool      `db:"is_active" json:"is_active"`
	DateCreation   time.Time `db:"date_creation" json:"date_creation"`
	DateModification time.Time `db:"date_modification" json:"date_modification"`

	// Relations (chargées séparément)
	Users []User `json:"users,omitempty"`
}

// TableName retourne le nom de la table
func (Machine) TableName() string {
	return "es_machine"
}

