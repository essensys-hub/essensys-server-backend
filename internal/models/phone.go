package models

import (
	"time"
)

// Phone représente un numéro de téléphone associé à un utilisateur
type Phone struct {
	ID              int       `db:"id" json:"id"`
	Phone           string    `db:"phone" json:"phone"`
	Nom             string    `db:"nom" json:"nom,omitempty"`
	SendMail        bool      `db:"send_mail" json:"send_mail"`
	AlerteAlarmeSent bool     `db:"alerte_alarme_sent" json:"alerte_alarme_sent"`
	AlerteLvSent    bool      `db:"alerte_lv_sent" json:"alerte_lv_sent"`
	AlerteLlSent    bool      `db:"alerte_ll_sent" json:"alerte_ll_sent"`
	AlerteNoSync    bool      `db:"alerte_no_sync" json:"alerte_no_sync"`
	DateCreation    time.Time `db:"date_creation" json:"date_creation"`
	DateModification time.Time `db:"date_modification" json:"date_modification"`
	UserID          int       `db:"user_id" json:"user_id"`

	// Relations (chargées séparément)
	User *User `json:"user,omitempty"`
}

// TableName retourne le nom de la table
func (Phone) TableName() string {
	return "es_phone"
}

// Smssend représente un SMS envoyé
type Smssend struct {
	ID       int       `db:"id" json:"id"`
	Phone    string    `db:"phone" json:"phone"`
	Message  string    `db:"message" json:"message"`
	DateSent time.Time `db:"date_sent" json:"date_sent"`
	UserID   *int      `db:"user_id" json:"user_id,omitempty"`

	// Relations (chargées séparément)
	User *User `json:"user,omitempty"`
}

// TableName retourne le nom de la table
func (Smssend) TableName() string {
	return "es_sms_send"
}




