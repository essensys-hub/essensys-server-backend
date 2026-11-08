package core

import (
	"encoding/json"
	"errors"
	"time"
)

// Réglages de la sonnette UniFi (sonnette-unifi-2026-11-007) : section « doorbell » du même
// fichier que les noms et les dispositions (clé light_names.path, aucune nouvelle clé). Comme
// les dispositions, c'est un OBJET : il ne passe jamais par namesDoc.section().

const (
	namesSectionDoorbell = "doorbell"

	// DoorbellDefaultBlinks : nombre de clignotements par défaut.
	DoorbellDefaultBlinks = 5
	// DoorbellMinBlinks et DoorbellMaxBlinks bornent blinks.
	DoorbellMinBlinks = 1
	DoorbellMaxBlinks = 10
)

var (
	// ErrInvalidBlinks : blinks hors de 1 à 10.
	ErrInvalidBlinks = errors.New("invalid_blinks")
	// ErrUnknownCamera : caméra absente de la console (ou vide alors que la fonction est activée).
	ErrUnknownCamera = errors.New("unknown_camera")
)

// DoorbellSettings est le document des réglages (contrat d'API).
type DoorbellSettings struct {
	Enabled   bool             `json:"enabled"`
	CameraID  string           `json:"camera_id"`
	Lights    []string         `json:"lights"`
	Blinks    int              `json:"blinks"`
	Schedule  DoorbellSchedule `json:"schedule"`
	Version   int              `json:"version"`
	UpdatedAt *string          `json:"updated_at"`
	UpdatedBy string           `json:"updated_by"`
}

// doorbellDoc est la section du fichier.
type doorbellDoc struct {
	Enabled   bool              `json:"enabled"`
	CameraID  string            `json:"camera_id"`
	Lights    []string          `json:"lights"`
	Blinks    int               `json:"blinks"`
	Schedule  *DoorbellSchedule `json:"schedule,omitempty"`
	Version   int               `json:"version"`
	UpdatedAt string            `json:"updated_at"`
	UpdatedBy string            `json:"updated_by"`
}

func (d doorbellDoc) normalized() doorbellDoc {
	if d.Lights == nil {
		d.Lights = []string{}
	}
	if d.Blinks < DoorbellMinBlinks || d.Blinks > DoorbellMaxBlinks {
		d.Blinks = DoorbellDefaultBlinks
	}
	return d
}

// schedule renvoie la plage normalisée ; absente ou illisible : toujours actif.
func (d doorbellDoc) schedule() DoorbellSchedule {
	if d.Schedule == nil {
		return DefaultDoorbellSchedule()
	}
	n, err := d.Schedule.Normalize()
	if err != nil {
		return DefaultDoorbellSchedule()
	}
	return n
}

func (d doorbellDoc) view() DoorbellSettings {
	d = d.normalized()
	v := DoorbellSettings{
		Enabled: d.Enabled, CameraID: d.CameraID, Lights: append([]string{}, d.Lights...),
		Blinks: d.Blinks, Schedule: d.schedule(), Version: d.Version, UpdatedBy: d.UpdatedBy,
	}
	if d.UpdatedAt != "" {
		at := d.UpdatedAt
		v.UpdatedAt = &at
	}
	return v
}

// ValidateDoorbellLights contrôle les identifiants de lumières (catalogue, sans doublon).
func ValidateDoorbellLights(lights []string) error {
	if lights == nil {
		return ErrUnknownLight
	}
	seen := map[string]bool{}
	for _, id := range lights {
		if !LayoutLights.known(id) {
			return ErrUnknownLight
		}
		if seen[id] {
			return ErrUnknownLight
		}
		seen[id] = true
	}
	return nil
}

// ValidateDoorbellBlinks contrôle blinks (1 à 10).
func ValidateDoorbellBlinks(n int) error {
	if n < DoorbellMinBlinks || n > DoorbellMaxBlinks {
		return ErrInvalidBlinks
	}
	return nil
}

// doorbell décode la section. Absente ou null : réglages par défaut. Une section qui n'est pas
// un objet est illisible (ErrLightNamesUnavailable) : on ne l'écrase pas.
func (d namesDoc) doorbell() (doorbellDoc, error) {
	raw, ok := d[namesSectionDoorbell]
	if !ok || string(raw) == "null" {
		return doorbellDoc{}.normalized(), nil
	}
	var v doorbellDoc
	if err := json.Unmarshal(raw, &v); err != nil {
		return doorbellDoc{}, ErrLightNamesUnavailable
	}
	return v.normalized(), nil
}

// Doorbell renvoie les réglages. N'échoue jamais : sans stockage lisible, les valeurs par
// défaut (désactivé, aucune lumière, 5 clignotements) sont renvoyées.
func (s *LightNameStore) Doorbell() DoorbellSettings {
	if s != nil {
		s.mu.Lock()
		doc, err := s.loadDoc()
		var v doorbellDoc
		if err == nil {
			v, err = doc.doorbell()
		}
		s.mu.Unlock()
		if err == nil {
			return v.view()
		}
	}
	return doorbellDoc{}.view()
}

// SetDoorbell remplace les réglages à la version n+1 et pose updated_at (UTC) et updated_by.
// Les valeurs doivent avoir été validées par l'appelant (ValidateDoorbell*) ; toutes les autres
// sections du fichier sont réécrites telles quelles.
func (s *LightNameStore) SetDoorbell(enabled bool, cameraID string, lights []string, blinks int, by string, now time.Time) (DoorbellSettings, error) {
	return s.SetDoorbellSchedule(enabled, cameraID, lights, blinks, nil, by, now)
}

// SetDoorbellSchedule est SetDoorbell avec la plage horaire ; schedule nil : la plage actuelle
// est conservée.
func (s *LightNameStore) SetDoorbellSchedule(enabled bool, cameraID string, lights []string, blinks int, schedule *DoorbellSchedule, by string, now time.Time) (DoorbellSettings, error) {
	var sched *DoorbellSchedule
	if schedule != nil {
		n, err := schedule.Normalize()
		if err != nil {
			return DoorbellSettings{}, err
		}
		sched = &n
	}
	if err := ValidateDoorbellLights(lights); err != nil {
		return DoorbellSettings{}, err
	}
	if err := ValidateDoorbellBlinks(blinks); err != nil {
		return DoorbellSettings{}, err
	}
	if !s.Available() {
		return DoorbellSettings{}, ErrLightNamesUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc()
	if err != nil {
		return DoorbellSettings{}, err
	}
	cur, err := doc.doorbell()
	if err != nil {
		return DoorbellSettings{}, err
	}
	if sched == nil {
		sched = cur.Schedule
	}
	next := doorbellDoc{
		Schedule:  sched,
		Enabled:   enabled,
		CameraID:  cameraID,
		Lights:    append([]string{}, lights...),
		Blinks:    blinks,
		Version:   cur.Version + 1,
		UpdatedAt: now.UTC().Format(lightNameTimeLayout),
		UpdatedBy: by,
	}
	if err := doc.setRaw(namesSectionDoorbell, next); err != nil {
		return DoorbellSettings{}, err
	}
	if err := s.writeDoc(doc); err != nil {
		return DoorbellSettings{}, err
	}
	return next.view(), nil
}
