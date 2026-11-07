package core

import (
	"errors"
	"time"
)

// Noms personnalisés des volets et stores (noms-volets-2026-11-005).
// Même fichier et même magasin que les lumières (clé light_names.path), section « shutters ».

// ErrUnknownShutter : identifiant hors catalogue des volets.
var ErrUnknownShutter = errors.New("unknown_shutter")

// ShutterNameInfo est une entrée du catalogue : identifiant client et nom d'origine.
type ShutterNameInfo struct {
	ID      string
	Default string
}

var shutterNameCatalogue = []ShutterNameInfo{
	{"volet1salon", "Volet 1 Salon"}, {"volet2salon", "Volet 2 Salon"}, {"volet3salon", "Volet 3 Salon"},
	{"volet1salleamanger", "Volet 1 Salle à Manger"}, {"volet2salleamanger", "Volet 2 Salle à Manger"},
	{"volet1cuisine", "Volet 1 Cuisine"}, {"volet2cuisine", "Volet 2 Cuisine"},
	{"voletsdb", "Volet Salle de Bain 1"},
	{"volet1gdchamb", "Volet 1 Grande Chambre"}, {"volet2gdchamb", "Volet 2 Grande Chambre"},
	{"volet1ptchamb", "Volet Petite Chambre 1"}, {"volet2ptchamb", "Volet Petite Chambre 2"},
	{"volet3ptchamb", "Volet Petite Chambre 3"}, {"voletbureau", "Volet Bureau"},
	{"voletstore", `Volet "Store"`}, {"store", "Store (banne)"},
}

// ShutterNames renvoie une copie du catalogue des 16 volets et stores, dans l'ordre d'affichage.
func ShutterNames() []ShutterNameInfo {
	out := make([]ShutterNameInfo, len(shutterNameCatalogue))
	copy(out, shutterNameCatalogue)
	return out
}

// ShutterNameDefault renvoie le nom d'origine d'un volet.
func ShutterNameDefault(id string) (string, bool) {
	info, ok := shutterNameInfo(id)
	return info.Default, ok
}

func shutterNameInfo(id string) (ShutterNameInfo, bool) {
	for _, s := range shutterNameCatalogue {
		if s.ID == id {
			return s, true
		}
	}
	return ShutterNameInfo{}, false
}

// ValidateShutterName applique la règle du nom, identique à celle des lumières.
func ValidateShutterName(raw string) (*string, error) { return ValidateLightName(raw) }

// ShutterNameView est l'objet commun ShutterName du contrat d'API (même forme que LightName).
type ShutterNameView struct {
	ID        string  `json:"id"`
	Default   string  `json:"default"`
	Name      *string `json:"name"`
	Version   int     `json:"version"`
	UpdatedAt *string `json:"updated_at"`
}

// ShutterNameSyncItem est une entrée envoyée au cloud (POST /api/gateway/shutters/names/sync).
type ShutterNameSyncItem struct {
	ID        string  `json:"id"`
	Name      *string `json:"name"`
	Version   int     `json:"version"`
	UpdatedAt *string `json:"updated_at"`
}

func shutterNameView(info ShutterNameInfo, e lightNameEntry, known bool) ShutterNameView {
	v := lightNameView(LightNameInfo{ID: info.ID, Default: info.Default}, e, known)
	return ShutterNameView{ID: v.ID, Default: v.Default, Name: v.Name, Version: v.Version, UpdatedAt: v.UpdatedAt}
}

// ShutterList renvoie les 16 volets dans l'ordre du catalogue. N'échoue jamais : sans stockage
// lisible, les noms d'origine sont renvoyés.
func (s *LightNameStore) ShutterList() []ShutterNameView {
	var shutters map[string]lightNameEntry
	if s != nil {
		s.mu.Lock()
		_, shutters, _ = s.load(namesSectionShutters)
		s.mu.Unlock()
	}
	out := make([]ShutterNameView, 0, len(shutterNameCatalogue))
	for _, info := range shutterNameCatalogue {
		e, ok := shutters[info.ID]
		out = append(out, shutterNameView(info, e, ok))
	}
	return out
}

// GetShutter renvoie un volet.
func (s *LightNameStore) GetShutter(id string) (ShutterNameView, bool) {
	for _, v := range s.ShutterList() {
		if v.ID == id {
			return v, true
		}
	}
	return ShutterNameView{}, false
}

// SetShutter enregistre un nom (nil = retour au nom d'origine), incrémente la version et pose
// updated_at (UTC) et updated_by. Le nom doit déjà être validé. Les autres sections du fichier
// sont réécrites telles quelles.
func (s *LightNameStore) SetShutter(id string, name *string, by string, now time.Time) (ShutterNameView, error) {
	info, ok := shutterNameInfo(id)
	if !ok {
		return ShutterNameView{}, ErrUnknownShutter
	}
	if s == nil {
		return ShutterNameView{}, ErrLightNamesUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, shutters, err := s.load(namesSectionShutters)
	if err != nil {
		return ShutterNameView{}, err
	}
	e := shutters[id]
	e.Name = name
	e.Version++
	e.UpdatedAt = now.UTC().Format(lightNameTimeLayout)
	e.UpdatedBy = by
	shutters[id] = e
	if err := s.save(doc, namesSectionShutters, shutters); err != nil {
		return ShutterNameView{}, err
	}
	return shutterNameView(info, e, true), nil
}

// ShutterSyncPayload renvoie les 16 entrées à envoyer au cloud.
func (s *LightNameStore) ShutterSyncPayload() []ShutterNameSyncItem {
	list := s.ShutterList()
	out := make([]ShutterNameSyncItem, 0, len(list))
	for _, v := range list {
		out = append(out, ShutterNameSyncItem{ID: v.ID, Name: v.Name, Version: v.Version, UpdatedAt: v.UpdatedAt})
	}
	return out
}

// MergeShutters adopte la réponse du cloud, avec la même règle que les lumières : une entrée
// distante remplace l'entrée locale seulement si elle est strictement plus récente selon
// (updated_at, version, updated_by). Les entrées inconnues, vierges ou au nom invalide sont
// ignorées. Renvoie true si le fichier a changé.
func (s *LightNameStore) MergeShutters(remote []ShutterNameView) (bool, error) {
	if s == nil {
		return false, ErrLightNamesUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, shutters, err := s.load(namesSectionShutters)
	if err != nil {
		return false, err
	}
	changed := false
	for _, r := range remote {
		if _, ok := shutterNameInfo(r.ID); !ok || r.UpdatedAt == nil || r.Version <= 0 {
			continue
		}
		var name *string
		if r.Name != nil {
			n, verr := ValidateShutterName(*r.Name)
			if verr != nil {
				continue
			}
			name = n
		}
		in := lightNameEntry{Name: name, Version: r.Version, UpdatedAt: *r.UpdatedAt}
		if _, perr := time.Parse(time.RFC3339Nano, in.UpdatedAt); perr != nil {
			continue
		}
		if cur, has := shutters[r.ID]; has && !lightNameNewer(in, cur) {
			continue
		}
		in.UpdatedBy = lightNameCloudActor
		shutters[r.ID] = in
		changed = true
	}
	if !changed {
		return false, nil
	}
	return true, s.save(doc, namesSectionShutters, shutters)
}
