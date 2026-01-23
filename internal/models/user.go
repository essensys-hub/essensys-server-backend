package models

import (
	"time"
)

// User représente un utilisateur du système Essensys
type User struct {
	ID           int       `db:"id" json:"id"`
	Mail         string    `db:"mail" json:"mail"`
	Password     string    `db:"password" json:"-"` // Hash SHA1 (legacy)
	Nom          string    `db:"nom" json:"nom"`
	Prenom       string    `db:"prenom" json:"prenom"`
	Adr1         string    `db:"adr1" json:"adr1,omitempty"`
	Adr2         string    `db:"adr2" json:"adr2,omitempty"`
	Cp           string    `db:"cp" json:"cp,omitempty"`
	Ville        string    `db:"ville" json:"ville,omitempty"`
	Phone        string    `db:"phone" json:"phone,omitempty"`
	Question     string    `db:"question" json:"question"`
	Reponse      string    `db:"reponse" json:"-"` // Hash SHA1
	IsValid      bool      `db:"isvalid" json:"is_valid"`
	SendInfos    bool      `db:"send_infos" json:"send_infos"`
	Obsolete     bool      `db:"obsolete" json:"obsolete"`
	DateCreation time.Time `db:"date_creation" json:"date_creation"`
	DateCloture  *time.Time `db:"date_cloture" json:"date_cloture,omitempty"`
	LastAccess   *time.Time `db:"last_access" json:"last_access,omitempty"`
	Guid         string    `db:"guid" json:"guid,omitempty"`
	MachineID    *int      `db:"machine_id" json:"machine_id,omitempty"`

	// Relations (chargées séparément)
	Machine *Machine `json:"machine,omitempty"`
	Phones  []Phone  `json:"phones,omitempty"`
}

// TableName retourne le nom de la table
func (User) TableName() string {
	return "es_user"
}




