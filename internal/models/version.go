package models

import (
	"time"
)

// Version représente une version de firmware disponible
type Version struct {
	ID         int    `db:"id" json:"id"`
	Descriptif string `db:"descriptif" json:"descriptif"`
	Size       int    `db:"size" json:"size"`
	Filename   string `db:"filename" json:"filename"`
}

// TableName retourne le nom de la table
func (Version) TableName() string {
	return "es_version"
}

// VersionMachine représente le suivi du téléchargement d'une version pour une machine
type VersionMachine struct {
	ID            int       `db:"id" json:"id"`
	MachineID     int       `db:"machine_id" json:"machine_id"`
	Version       string    `db:"version" json:"version"` // Ex: "V37"
	IsOk          bool      `db:"is_ok" json:"is_ok"`
	LastIndexCall int       `db:"last_index_call" json:"last_index_call"`
	DateAction    time.Time `db:"date_action" json:"date_action"`

	// Relations (chargées séparément)
	Machine *Machine `json:"machine,omitempty"`
}

// TableName retourne le nom de la table
func (VersionMachine) TableName() string {
	return "es_version_machine"
}

