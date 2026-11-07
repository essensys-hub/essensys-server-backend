package core

import (
	"encoding/json"
	"errors"
	"time"
)

// Disposition des objets (affichage-objets-2026-11-006) : activation/désactivation et ordre
// d'affichage des lumières et des volets. Un document par genre, stocké dans le même fichier
// que les noms (clé light_names.path), sections « lights_layout » et « shutters_layout ».
// Ces sections sont des OBJETS, pas des tables d'entrées : elles ne passent jamais par
// namesDoc.section().

// ErrInvalidLayout : corps illisible, clé absente, valeur qui n'est pas une liste de chaînes, doublon.
var ErrInvalidLayout = errors.New("invalid_layout")

// LayoutKind est le genre d'objets d'une disposition.
type LayoutKind string

const (
	LayoutLights   LayoutKind = "lights"
	LayoutShutters LayoutKind = "shutters"

	namesSectionLightsLayout   = "lights_layout"
	namesSectionShuttersLayout = "shutters_layout"
)

func (k LayoutKind) section() string {
	if k == LayoutShutters {
		return namesSectionShuttersLayout
	}
	return namesSectionLightsLayout
}

// Valid indique si le genre est connu.
func (k LayoutKind) Valid() bool { return k == LayoutLights || k == LayoutShutters }

// Catalogue renvoie les identifiants du genre dans l'ordre d'origine.
func (k LayoutKind) Catalogue() []string {
	var ids []string
	switch k {
	case LayoutLights:
		for _, l := range lightNameCatalogue {
			ids = append(ids, l.ID)
		}
	case LayoutShutters:
		for _, s := range shutterNameCatalogue {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

func (k LayoutKind) known(id string) bool {
	for _, c := range k.Catalogue() {
		if c == id {
			return true
		}
	}
	return false
}

// unknownErr est l'erreur d'identifiant hors catalogue du genre.
func (k LayoutKind) unknownErr() error {
	if k == LayoutShutters {
		return ErrUnknownShutter
	}
	return ErrUnknownLight
}

// ObjectLayout est le document commun du contrat d'API (sans updated_by).
type ObjectLayout struct {
	Order     []string `json:"order"`
	Disabled  []string `json:"disabled"`
	Version   int      `json:"version"`
	UpdatedAt *string  `json:"updated_at"`
}

// LayoutSyncDoc est le document échangé avec le cloud (POST /api/gateway/{genre}/layout/sync) :
// il porte updated_by, que la fusion compare.
type LayoutSyncDoc struct {
	Order     []string `json:"order"`
	Disabled  []string `json:"disabled"`
	Version   int      `json:"version"`
	UpdatedAt *string  `json:"updated_at"`
	UpdatedBy string   `json:"updated_by"`
}

// layoutDoc est la section du fichier.
type layoutDoc struct {
	Order     []string `json:"order"`
	Disabled  []string `json:"disabled"`
	Version   int      `json:"version"`
	UpdatedAt string   `json:"updated_at"`
	UpdatedBy string   `json:"updated_by"`
}

func (l layoutDoc) normalized() layoutDoc {
	if l.Order == nil {
		l.Order = []string{}
	}
	if l.Disabled == nil {
		l.Disabled = []string{}
	}
	return l
}

func (l layoutDoc) view() ObjectLayout {
	l = l.normalized()
	v := ObjectLayout{Order: append([]string{}, l.Order...), Disabled: append([]string{}, l.Disabled...), Version: l.Version}
	if l.UpdatedAt != "" {
		at := l.UpdatedAt
		v.UpdatedAt = &at
	}
	return v
}

func (l layoutDoc) syncDoc() LayoutSyncDoc {
	v := l.view()
	return LayoutSyncDoc{Order: v.Order, Disabled: v.Disabled, Version: v.Version, UpdatedAt: v.UpdatedAt, UpdatedBy: l.UpdatedBy}
}

// ValidateLayout contrôle une disposition : identifiants du catalogue du genre (y compris un
// identifiant de l'autre genre refusé), sans doublon dans order ni dans disabled. Un order
// incomplet et un disabled complet sont acceptés. Les listes nil sont refusées (clés obligatoires).
func ValidateLayout(kind LayoutKind, order, disabled []string) error {
	if !kind.Valid() {
		return ErrInvalidLayout
	}
	if order == nil || disabled == nil {
		return ErrInvalidLayout
	}
	for _, list := range [][]string{order, disabled} {
		for _, id := range list {
			if !kind.known(id) {
				return kind.unknownErr()
			}
		}
	}
	for _, list := range [][]string{order, disabled} {
		seen := map[string]bool{}
		for _, id := range list {
			if seen[id] {
				return ErrInvalidLayout
			}
			seen[id] = true
		}
	}
	return nil
}

// EffectiveLayout calcule l'ordre effectif (decision 2 du design) : les identifiants de order
// du catalogue (premier exemplaire), puis les identifiants du catalogue absents de order dans
// l'ordre du catalogue ; disabled est l'intersection avec le catalogue.
func EffectiveLayout(kind LayoutKind, l ObjectLayout) (ordered []string, disabled map[string]bool) {
	seen := map[string]bool{}
	ordered = []string{}
	for _, id := range l.Order {
		if kind.known(id) && !seen[id] {
			seen[id] = true
			ordered = append(ordered, id)
		}
	}
	for _, id := range kind.Catalogue() {
		if !seen[id] {
			ordered = append(ordered, id)
		}
	}
	disabled = map[string]bool{}
	for _, id := range l.Disabled {
		if kind.known(id) {
			disabled[id] = true
		}
	}
	return ordered, disabled
}

// layout décode une section de disposition (objet). Absente ou null : disposition par défaut.
// Une section qui n'est pas un objet est illisible (ErrLightNamesUnavailable) : on ne l'écrase pas.
func (d namesDoc) layout(name string) (layoutDoc, error) {
	raw, ok := d[name]
	if !ok || string(raw) == "null" {
		return layoutDoc{}.normalized(), nil
	}
	var l layoutDoc
	if err := json.Unmarshal(raw, &l); err != nil {
		return layoutDoc{}, ErrLightNamesUnavailable
	}
	return l.normalized(), nil
}

// setRaw remplace une section par une valeur quelconque et laisse toutes les autres telles quelles.
func (d namesDoc) setRaw(name string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return ErrLightNamesUnavailable
	}
	d[name] = raw
	return nil
}

func (s *LightNameStore) loadLayout(kind LayoutKind) (namesDoc, layoutDoc, error) {
	d, err := s.loadDoc()
	if err != nil {
		return nil, layoutDoc{}, err
	}
	l, err := d.layout(kind.section())
	if err != nil {
		return nil, layoutDoc{}, err
	}
	return d, l, nil
}

func (s *LightNameStore) saveLayout(d namesDoc, kind LayoutKind, l layoutDoc) error {
	if err := d.setRaw(kind.section(), l.normalized()); err != nil {
		return err
	}
	return s.writeDoc(d)
}

// Layout renvoie la disposition du genre. N'échoue jamais : sans stockage lisible, la
// disposition par défaut (version 0, updated_at nul) est renvoyée.
func (s *LightNameStore) Layout(kind LayoutKind) ObjectLayout {
	if s != nil && kind.Valid() {
		s.mu.Lock()
		_, l, err := s.loadLayout(kind)
		s.mu.Unlock()
		if err == nil {
			return l.view()
		}
	}
	return layoutDoc{}.view()
}

// SetLayout remplace le document entier à la version n+1 (reset : disposition par défaut, même
// version n+1) et pose updated_at (UTC) et updated_by. Valide d'abord (ValidateLayout) ; les
// autres sections du fichier sont réécrites telles quelles.
func (s *LightNameStore) SetLayout(kind LayoutKind, order, disabled []string, reset bool, by string, now time.Time) (ObjectLayout, error) {
	if !kind.Valid() {
		return ObjectLayout{}, ErrInvalidLayout
	}
	if reset {
		order, disabled = []string{}, []string{}
	}
	if err := ValidateLayout(kind, order, disabled); err != nil {
		return ObjectLayout{}, err
	}
	if s == nil {
		return ObjectLayout{}, ErrLightNamesUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, cur, err := s.loadLayout(kind)
	if err != nil {
		return ObjectLayout{}, err
	}
	next := layoutDoc{
		Order:     append([]string{}, order...),
		Disabled:  append([]string{}, disabled...),
		Version:   cur.Version + 1,
		UpdatedAt: now.UTC().Format(lightNameTimeLayout),
		UpdatedBy: by,
	}
	if err := s.saveLayout(doc, kind, next); err != nil {
		return ObjectLayout{}, err
	}
	return next.view(), nil
}

// LayoutSyncPayload renvoie le document à envoyer au cloud (version 0 : disposition par défaut).
func (s *LightNameStore) LayoutSyncPayload(kind LayoutKind) LayoutSyncDoc {
	if s != nil && kind.Valid() {
		s.mu.Lock()
		_, l, err := s.loadLayout(kind)
		s.mu.Unlock()
		if err == nil {
			return l.syncDoc()
		}
	}
	return layoutDoc{}.syncDoc()
}

// MergeLayout adopte le document renvoyé par le cloud seulement s'il est strictement plus
// récent que la section locale selon (updated_at, version, updated_by). Identifiants inconnus
// ignorés, doublons retirés ; document de version 0, sans updated_at ou illisible ignoré.
// Renvoie true si le fichier a changé.
func (s *LightNameStore) MergeLayout(kind LayoutKind, remote LayoutSyncDoc) (bool, error) {
	if s == nil || !kind.Valid() {
		return false, ErrLightNamesUnavailable
	}
	if remote.Version <= 0 || remote.UpdatedAt == nil {
		return false, nil
	}
	if _, err := time.Parse(time.RFC3339Nano, *remote.UpdatedAt); err != nil {
		return false, nil
	}
	in := layoutDoc{
		Order:     cleanLayoutIDs(kind, remote.Order),
		Disabled:  cleanLayoutIDs(kind, remote.Disabled),
		Version:   remote.Version,
		UpdatedAt: *remote.UpdatedAt,
		UpdatedBy: remote.UpdatedBy,
	}
	if in.UpdatedBy == "" {
		in.UpdatedBy = lightNameCloudActor
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, cur, err := s.loadLayout(kind)
	if err != nil {
		return false, err
	}
	if cur.Version > 0 && !layoutNewer(in, cur) {
		return false, nil
	}
	return true, s.saveLayout(doc, kind, in)
}

// cleanLayoutIDs retire les identifiants inconnus et les doublons, en gardant l'ordre.
func cleanLayoutIDs(kind LayoutKind, ids []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if kind.known(id) && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// layoutNewer : a est strictement plus récent que b selon (updated_at, version, updated_by).
func layoutNewer(a, b layoutDoc) bool {
	ta, tb := parseLightNameTime(a.UpdatedAt), parseLightNameTime(b.UpdatedAt)
	if !ta.Equal(tb) {
		return ta.After(tb)
	}
	if a.Version != b.Version {
		return a.Version > b.Version
	}
	return a.UpdatedBy > b.UpdatedBy
}
