package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
)

// noms-volets-2026-11-005 : synchronisation des noms personnalisés des volets et stores,
// dans les deux sens, par un appel distinct de celui des lumières. Le magasin est le même
// fichier ; le cloud fusionne et la gateway applique la même règle (updated_at, version,
// updated_by) pour ne jamais écraser un nom local plus récent.

// SetShutterNames branche le magasin des noms (nil : synchronisation désactivée).
func (a *Agent) SetShutterNames(store *core.LightNameStore) {
	a.shutterNamesMu.Lock()
	a.shutterNames = store
	a.shutterNamesMu.Unlock()
}

// syncNames lance les deux synchronisations de noms l'une après l'autre ; l'échec de l'une
// (hors ligne, 404 d'un cloud pas encore déployé, 5xx) ne bloque pas l'autre.
func (a *Agent) syncNames(ctx context.Context) {
	if err := a.syncLightNames(ctx); err != nil {
		log.Printf("[cloudsync] light names sync: %v", err)
	}
	if err := a.syncShutterNames(ctx); err != nil {
		log.Printf("[cloudsync] shutter names sync: %v", err)
	}
}

// syncShutterNames envoie les 16 entrées au cloud et adopte l'état fusionné renvoyé. En cas
// d'échec la réplique locale est conservée ; le cycle suivant renvoie tout.
func (a *Agent) syncShutterNames(ctx context.Context) error {
	a.shutterNamesMu.Lock()
	defer a.shutterNamesMu.Unlock()
	store := a.shutterNames
	if store == nil || !store.Available() {
		return nil
	}
	body, err := json.Marshal(map[string]any{"names": store.ShutterSyncPayload()})
	if err != nil {
		return err
	}
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/shutters/names/sync"
	req, err := httpNewRequest(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	a.setGatewayHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Shutters []core.ShutterNameView `json:"shutters"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return err
	}
	if _, err := store.MergeShutters(out.Shutters); err != nil {
		return err
	}
	return nil
}

// TriggerShutterNamesSync tente une synchronisation immédiate après une écriture locale.
// Ne bloque jamais l'appelant : l'envoi part dans une goroutine et un échec est seulement
// journalisé (le prochain cycle reprendra).
func (a *Agent) TriggerShutterNamesSync() {
	if a == nil || !a.cfg.Enabled || a.validateHubURL() != nil {
		return
	}
	a.shutterNamesMu.Lock()
	has := a.shutterNames != nil
	a.shutterNamesMu.Unlock()
	if !has {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), lightNamesSyncTimeout)
		defer cancel()
		if err := a.syncShutterNames(ctx); err != nil {
			log.Printf("[cloudsync] shutter names sync (immediate): %v", err)
		}
	}()
}
