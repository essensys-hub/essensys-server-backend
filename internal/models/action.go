package models

import (
	"time"
)

// Action représente une action à exécuter par une machine
type Action struct {
	ID          int       `db:"id" json:"id"`
	MachineID   int       `db:"machine_id" json:"machine_id"`
	Guid        string    `db:"guid" json:"guid"`
	ActionType  string    `db:"action_type" json:"action_type"`
	ActionInfo  string    `db:"action_info" json:"action_info,omitempty"`
	IsDone      bool      `db:"is_done" json:"is_done"`
	DateCreation time.Time `db:"date_creation" json:"date_creation"`

	// Relations (chargées séparément)
	Machine *Machine       `json:"machine,omitempty"`
	Indexes []ActionIndex  `json:"indexes,omitempty"`
}

// TableName retourne le nom de la table
func (Action) TableName() string {
	return "es_action"
}

// ActionIndex représente un paramètre (index/valeur) d'une action
type ActionIndex struct {
	ID      int    `db:"id" json:"id"`
	ActionID int   `db:"action_id" json:"action_id"`
	IndexID  int   `db:"index_id" json:"index_id"`
	Value    string `db:"value" json:"value"`

	// Relations (chargées séparément)
	Action *Action    `json:"action,omitempty"`
	Index  *DataIndex `json:"index,omitempty"`
}

// TableName retourne le nom de la table
func (ActionIndex) TableName() string {
	return "es_action_index"
}

