package models

import (
	"time"
)

// CleMachine représente une clé d'activation pour une machine
type CleMachine struct {
	ID            int       `db:"id" json:"id"`
	Cle           string    `db:"cle" json:"cle"`
	DateGeneration time.Time `db:"date_generation" json:"date_generation"`
	DateActivation *time.Time `db:"date_activation" json:"date_activation,omitempty"`
	MachineID     *int      `db:"machine_id" json:"machine_id,omitempty"`

	// Relations (chargées séparément)
	Machine *Machine `json:"machine,omitempty"`
}

// TableName retourne le nom de la table
func (CleMachine) TableName() string {
	return "es_cle_machine"
}




