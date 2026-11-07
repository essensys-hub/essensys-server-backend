package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Noms personnalisés des lumières (noms-lumieres-2026-11-004).
// La gateway garde une réplique dans un fichier JSON ; le cloud arbitre.

const (
	// LightNameMaxRunes est la longueur maximale d'un nom, après rognage.
	LightNameMaxRunes = 40
	// lightNameTimeLayout : UTC, RFC 3339, millisecondes (contrat figé).
	lightNameTimeLayout   = "2006-01-02T15:04:05.000Z"
	lightNamesFileVersion = 1
	// lightNameCloudActor marque une entrée adoptée depuis le cloud.
	lightNameCloudActor = "cloud"
)

var (
	// ErrUnknownLight : identifiant hors catalogue.
	ErrUnknownLight = errors.New("unknown_light")
	// ErrInvalidLightName : nom refusé par la règle du nom.
	ErrInvalidLightName = errors.New("invalid_name")
	// ErrLightNamesUnavailable : pas de stockage valide (chemin absent, non inscriptible, fichier illisible).
	ErrLightNamesUnavailable = errors.New("names_unavailable")
)

// LightNameInfo est une entrée du catalogue : identifiant client et nom d'origine.
type LightNameInfo struct {
	ID      string
	Default string
}

var lightNameCatalogue = []LightNameInfo{
	{"terrasse", "Terrasse"}, {"entree", "Entrée"}, {"escalier", "Escalier"},
	{"deg1", "Dégagement 1"}, {"deg2", "Dégagement 2"}, {"pieceserv", "Pièce de service"},
	{"ann1", "Annexe 1"}, {"ann2", "Annexe 2"}, {"salon", "Salon"}, {"sam", "Salle à Manger"},
	{"cuisine", "Cuisine"}, {"sdb1", "Salle de Bain 1"}, {"sdb2", "Salle de Bain 2"},
	{"wc1", "WC 1"}, {"wc2", "WC 2"}, {"bureau", "Bureau"}, {"gdchamb", "Grande Chambre"},
	{"ptchamb1", "Petite Chambre 1"}, {"ptchamb2", "Petite Chambre 2"}, {"ptchamb3", "Petite Chambre 3"},
	{"dressing", "Dressing"},
	{"isalonind", "Salon (indirect 1)"}, {"isalonind2", "Salon (indirect 2)"},
	{"icuisine", "Cuisine (plans de travail)"}, {"isdb1", "Salle de Bain 1 (miroir)"},
	{"isdb2", "Salle de Bain 2 (miroir)"}, {"igdchamb1", "Grande Chambre (chevet 1)"},
	{"igdchamb2", "Grande Chambre (chevet 2)"}, {"iptchamb1", "Petite Chambre 1 (chevet 1)"},
	{"iptchamb2", "Petite Chambre 1 (chevet 2)"}, {"iptchamb22", "Petite Chambre 2 (chevet)"},
	{"iptchamb3", "Petite Chambre 3 (chevet)"}, {"idressing", "Dressing (placards)"},
}

// LightNames renvoie une copie du catalogue des 33 lumières, dans l'ordre d'affichage.
func LightNames() []LightNameInfo {
	out := make([]LightNameInfo, len(lightNameCatalogue))
	copy(out, lightNameCatalogue)
	return out
}

// LightNameDefault renvoie le nom d'origine d'une lumière.
func LightNameDefault(id string) (string, bool) {
	for _, l := range lightNameCatalogue {
		if l.ID == id {
			return l.Default, true
		}
	}
	return "", false
}

// ValidateLightName applique la règle du nom : après rognage, 1 à 40 caractères Unicode,
// sans caractère de contrôle (U+0000-U+001F, U+007F), sans '<' ni '>'.
// Un nom vide donne (nil, nil) : retour au nom d'origine.
func ValidateLightName(raw string) (*string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(name) > LightNameMaxRunes || !utf8.ValidString(name) {
		return nil, ErrInvalidLightName
	}
	for _, r := range name {
		if r <= 0x1F || r == 0x7F || r == '<' || r == '>' {
			return nil, ErrInvalidLightName
		}
	}
	return &name, nil
}

// LightNameView est l'objet commun LightName du contrat d'API.
type LightNameView struct {
	ID        string  `json:"id"`
	Default   string  `json:"default"`
	Name      *string `json:"name"`
	Version   int     `json:"version"`
	UpdatedAt *string `json:"updated_at"`
}

// LightNameSyncItem est une entrée envoyée au cloud (POST /api/gateway/lights/names/sync).
type LightNameSyncItem struct {
	ID        string  `json:"id"`
	Name      *string `json:"name"`
	Version   int     `json:"version"`
	UpdatedAt *string `json:"updated_at"`
}

type lightNameEntry struct {
	Name      *string `json:"name"`
	Version   int     `json:"version"`
	UpdatedAt string  `json:"updated_at"`
	UpdatedBy string  `json:"updated_by"`
}

// namesDoc est le contenu brut du fichier : chaque section reste en JSON brut tant qu'elle
// n'est pas modifiée, de sorte qu'une écriture ne perde ni l'autre section (« lights » ou
// « shutters ») ni une section inconnue ajoutée à la main (noms-volets-2026-11-005).
type namesDoc map[string]json.RawMessage

const (
	namesSectionLights   = "lights"
	namesSectionShutters = "shutters"
)

// section décode une section ; une section absente est vide.
func (d namesDoc) section(name string) (map[string]lightNameEntry, error) {
	out := map[string]lightNameEntry{}
	raw, ok := d[name]
	if !ok || string(raw) == "null" {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, ErrLightNamesUnavailable
	}
	if out == nil {
		out = map[string]lightNameEntry{}
	}
	return out, nil
}

// setSection remplace une section et laisse toutes les autres telles quelles.
func (d namesDoc) setSection(name string, entries map[string]lightNameEntry) error {
	raw, err := json.Marshal(entries)
	if err != nil {
		return ErrLightNamesUnavailable
	}
	d[name] = raw
	return nil
}

// LightNameStore est le magasin de fichier JSON unique (sections « lights » et « shutters »),
// écrit de façon atomique sous un seul verrou.
type LightNameStore struct {
	path string
	mu   sync.Mutex
}

// NewLightNameStore crée un magasin. Un chemin vide donne un magasin indisponible
// (lecture : noms d'origine ; écriture : ErrLightNamesUnavailable).
func NewLightNameStore(path string) *LightNameStore {
	return &LightNameStore{path: strings.TrimSpace(path)}
}

// Available indique si un chemin est configuré (l'inscriptibilité se vérifie à l'écriture).
func (s *LightNameStore) Available() bool { return s != nil && s.path != "" }

// loadDoc lit le fichier. Fichier absent : document vide. Fichier illisible : erreur.
func (s *LightNameStore) loadDoc() (namesDoc, error) {
	if !s.Available() {
		return nil, ErrLightNamesUnavailable
	}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return namesDoc{}, nil
	}
	if err != nil {
		return nil, ErrLightNamesUnavailable
	}
	var d namesDoc
	if err := json.Unmarshal(raw, &d); err != nil || d == nil {
		return nil, ErrLightNamesUnavailable
	}
	var version int
	if v, ok := d["version"]; !ok || json.Unmarshal(v, &version) != nil || version != lightNamesFileVersion {
		return nil, ErrLightNamesUnavailable
	}
	return d, nil
}

// load lit une section du fichier.
func (s *LightNameStore) load(section string) (namesDoc, map[string]lightNameEntry, error) {
	d, err := s.loadDoc()
	if err != nil {
		return nil, nil, err
	}
	entries, err := d.section(section)
	if err != nil {
		return nil, nil, err
	}
	return d, entries, nil
}

// save écrit le document de façon atomique : fichier temporaire dans le même dossier, puis renommage.
func (s *LightNameStore) save(d namesDoc, section string, entries map[string]lightNameEntry) error {
	if err := d.setSection(section, entries); err != nil {
		return err
	}
	return s.writeDoc(d)
}

// writeDoc écrit le document entier (toutes les sections) de façon atomique. Appelé sous le verrou.
func (s *LightNameStore) writeDoc(d namesDoc) error {
	d["version"] = json.RawMessage("1")
	raw, err := json.Marshal(d)
	if err != nil {
		return ErrLightNamesUnavailable
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".light-names-*.tmp")
	if err != nil {
		return ErrLightNamesUnavailable
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(raw)
	serr := tmp.Sync()
	cerr := tmp.Close()
	if werr != nil || serr != nil || cerr != nil {
		_ = os.Remove(tmpName)
		return ErrLightNamesUnavailable
	}
	_ = os.Chmod(tmpName, 0o644)
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return ErrLightNamesUnavailable
	}
	return nil
}

func lightNameView(info LightNameInfo, e lightNameEntry, known bool) LightNameView {
	v := LightNameView{ID: info.ID, Default: info.Default}
	if known {
		v.Name = e.Name
		v.Version = e.Version
		if e.UpdatedAt != "" {
			at := e.UpdatedAt
			v.UpdatedAt = &at
		}
	}
	return v
}

// List renvoie les 33 lumières dans l'ordre du catalogue. N'échoue jamais : sans stockage
// lisible, les noms d'origine sont renvoyés.
func (s *LightNameStore) List() []LightNameView {
	var lights map[string]lightNameEntry
	if s != nil {
		s.mu.Lock()
		_, lights, _ = s.load(namesSectionLights)
		s.mu.Unlock()
	}
	out := make([]LightNameView, 0, len(lightNameCatalogue))
	for _, info := range lightNameCatalogue {
		e, ok := lights[info.ID]
		out = append(out, lightNameView(info, e, ok))
	}
	return out
}

// Get renvoie une lumière.
func (s *LightNameStore) Get(id string) (LightNameView, bool) {
	for _, v := range s.List() {
		if v.ID == id {
			return v, true
		}
	}
	return LightNameView{}, false
}

// Set enregistre un nom (nil = retour au nom d'origine), incrémente la version et pose
// updated_at (UTC) et updated_by. Le nom doit déjà être validé.
func (s *LightNameStore) Set(id string, name *string, by string, now time.Time) (LightNameView, error) {
	info, ok := lightNameInfo(id)
	if !ok {
		return LightNameView{}, ErrUnknownLight
	}
	if s == nil {
		return LightNameView{}, ErrLightNamesUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, lights, err := s.load(namesSectionLights)
	if err != nil {
		return LightNameView{}, err
	}
	e := lights[id]
	e.Name = name
	e.Version++
	e.UpdatedAt = now.UTC().Format(lightNameTimeLayout)
	e.UpdatedBy = by
	lights[id] = e
	if err := s.save(doc, namesSectionLights, lights); err != nil {
		return LightNameView{}, err
	}
	return lightNameView(info, e, true), nil
}

// SyncPayload renvoie les 33 entrées à envoyer au cloud.
func (s *LightNameStore) SyncPayload() []LightNameSyncItem {
	list := s.List()
	out := make([]LightNameSyncItem, 0, len(list))
	for _, v := range list {
		out = append(out, LightNameSyncItem{ID: v.ID, Name: v.Name, Version: v.Version, UpdatedAt: v.UpdatedAt})
	}
	return out
}

// Merge adopte la réponse du cloud : une entrée distante remplace l'entrée locale seulement
// si elle est strictement plus récente selon (updated_at, version, updated_by). Le cloud ne
// renvoie pas updated_by : ce dernier critère ne départage que si les deux côtés le portent.
// Les entrées inconnues, vierges ou au nom invalide sont ignorées. Renvoie true si le fichier a changé.
func (s *LightNameStore) Merge(remote []LightNameView) (bool, error) {
	if s == nil {
		return false, ErrLightNamesUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, lights, err := s.load(namesSectionLights)
	if err != nil {
		return false, err
	}
	changed := false
	for _, r := range remote {
		if _, ok := lightNameInfo(r.ID); !ok || r.UpdatedAt == nil || r.Version <= 0 {
			continue
		}
		var name *string
		if r.Name != nil {
			n, verr := ValidateLightName(*r.Name)
			if verr != nil {
				continue
			}
			name = n
		}
		in := lightNameEntry{Name: name, Version: r.Version, UpdatedAt: *r.UpdatedAt}
		if _, perr := time.Parse(time.RFC3339Nano, in.UpdatedAt); perr != nil {
			continue
		}
		if cur, has := lights[r.ID]; has && !lightNameNewer(in, cur) {
			continue
		}
		in.UpdatedBy = lightNameCloudActor
		lights[r.ID] = in
		changed = true
	}
	if !changed {
		return false, nil
	}
	return true, s.save(doc, namesSectionLights, lights)
}

// lightNameNewer : a est strictement plus récent que b selon (updated_at, version, updated_by).
func lightNameNewer(a, b lightNameEntry) bool {
	ta, tb := parseLightNameTime(a.UpdatedAt), parseLightNameTime(b.UpdatedAt)
	if !ta.Equal(tb) {
		return ta.After(tb)
	}
	if a.Version != b.Version {
		return a.Version > b.Version
	}
	if a.UpdatedBy != "" && b.UpdatedBy != "" {
		return a.UpdatedBy > b.UpdatedBy
	}
	return false
}

func parseLightNameTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func lightNameInfo(id string) (LightNameInfo, bool) {
	for _, l := range lightNameCatalogue {
		if l.ID == id {
			return l, true
		}
	}
	return LightNameInfo{}, false
}
