package models

import (
	"time"
)

// State représente un snapshot de l'état d'une machine à un moment donné
type State struct {
	ID          int       `db:"id" json:"id"`
	MachineID   int       `db:"machine_id" json:"machine_id"`
	Version     string    `db:"version" json:"version,omitempty"`
	Completed   bool      `db:"completed" json:"completed"`
	StateDate   time.Time `db:"state_date" json:"state_date"`

	// Relations (chargées séparément)
	Machine *Machine      `json:"machine,omitempty"`
	Indexes []StateIndex   `json:"indexes,omitempty"`
}

// TableName retourne le nom de la table
func (State) TableName() string {
	return "es_state"
}

// StateIndex représente la valeur d'un index dans un état
type StateIndex struct {
	ID        int       `db:"id" json:"id"`
	StateID   int       `db:"state_id" json:"state_id"`
	IndexID   int       `db:"index_id" json:"index_id"`
	Value     string    `db:"value" json:"value"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	// Relations (chargées séparément)
	State *State     `json:"state,omitempty"`
	Index *DataIndex `json:"index,omitempty"`
}

// TableName retourne le nom de la table
func (StateIndex) TableName() string {
	return "es_state_index"
}


