package core

import (
	"errors"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // le conteneur est en UTC et sans base de fuseaux : elle est embarquée
)

// Plages horaires de la sonnette (sonnette-plages-push-2026-11-008) : section
// « doorbell.schedule » du même fichier que les réglages (aucune nouvelle clé de chemin).

const (
	DoorbellScheduleAlways = "always"
	DoorbellScheduleCustom = "custom"
	// DoorbellDefaultTimezone : fuseau par défaut.
	DoorbellDefaultTimezone = "Europe/Paris"
)

// ErrInvalidSchedule : mode, fuseau ou grille invalide.
var ErrInvalidSchedule = errors.New("invalid_schedule")

// DoorbellSchedule : mode, fuseau IANA et grille de 7 chaînes de 24 caractères 0/1
// (grid[0] = lundi ; le caractère h = heure locale h à h+1 ; 1 = active).
type DoorbellSchedule struct {
	Mode     string   `json:"mode"`
	Timezone string   `json:"timezone"`
	Grid     []string `json:"grid"`
}

// DefaultDoorbellSchedule : toujours actif, Europe/Paris, grille entièrement active.
func DefaultDoorbellSchedule() DoorbellSchedule {
	return DoorbellSchedule{Mode: DoorbellScheduleAlways, Timezone: DoorbellDefaultTimezone, Grid: fullGrid()}
}

func fullGrid() []string {
	g := make([]string, 7)
	for i := range g {
		g[i] = strings.Repeat("1", 24)
	}
	return g
}

var tzCache sync.Map // nom -> *time.Location

func loadZone(name string) (*time.Location, error) {
	if v, ok := tzCache.Load(name); ok {
		return v.(*time.Location), nil
	}
	if name == "" || name == "Local" {
		return nil, ErrInvalidSchedule
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, ErrInvalidSchedule
	}
	tzCache.Store(name, loc)
	return loc, nil
}

// Normalize complète les valeurs absentes (mode always, fuseau par défaut, grille pleine pour
// always) puis contrôle le tout. Une grille mal formée est toujours refusée.
func (s DoorbellSchedule) Normalize() (DoorbellSchedule, error) {
	switch s.Mode {
	case DoorbellScheduleAlways, DoorbellScheduleCustom:
	default:
		return DoorbellSchedule{}, ErrInvalidSchedule
	}
	if s.Timezone == "" {
		s.Timezone = DoorbellDefaultTimezone
	}
	if _, err := loadZone(s.Timezone); err != nil {
		return DoorbellSchedule{}, err
	}
	if len(s.Grid) == 0 && s.Mode == DoorbellScheduleAlways {
		s.Grid = fullGrid()
	}
	if len(s.Grid) != 7 {
		return DoorbellSchedule{}, ErrInvalidSchedule
	}
	for _, row := range s.Grid {
		if len(row) != 24 || strings.Trim(row, "01") != "" {
			return DoorbellSchedule{}, ErrInvalidSchedule
		}
	}
	s.Grid = append([]string{}, s.Grid...)
	return s, nil
}

// ActiveAt indique si la case de l'instant t (converti dans le fuseau réglé) est active.
// Réglage illisible : actif (comportement de #58).
func (s DoorbellSchedule) ActiveAt(t time.Time) bool {
	if s.Mode != DoorbellScheduleCustom {
		return true
	}
	loc, err := loadZone(s.Timezone)
	if err != nil || len(s.Grid) != 7 {
		return true
	}
	l := t.In(loc)
	day := (int(l.Weekday()) + 6) % 7
	row := s.Grid[day]
	h := l.Hour()
	if len(row) != 24 {
		return true
	}
	return row[h] == '1'
}
