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
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
)

// noms-lumieres-2026-11-004 : synchronisation des noms personnalisés des lumières,
// dans les deux sens. Le cloud fusionne ; la gateway adopte sa réponse en appliquant la
// même règle (updated_at, version, updated_by) pour ne jamais écraser un nom local plus récent.

const lightNamesSyncTimeout = 30 * time.Second

// SetLightNames branche le magasin des noms (nil : synchronisation désactivée).
func (a *Agent) SetLightNames(store *core.LightNameStore) {
	a.lightNamesMu.Lock()
	a.lightNames = store
	a.lightNamesMu.Unlock()
}

// syncLightNames envoie les 33 entrées au cloud et adopte l'état fusionné renvoyé.
// En cas d'échec (hors ligne, 401, 5xx) la réplique locale est conservée ; le cycle
// suivant renvoie tout, ce qui assure la reprise sans perdre une modification locale.
func (a *Agent) syncLightNames(ctx context.Context) error {
	a.lightNamesMu.Lock()
	defer a.lightNamesMu.Unlock()
	store := a.lightNames
	if store == nil || !store.Available() {
		return nil
	}
	body, err := json.Marshal(map[string]any{"names": store.SyncPayload()})
	if err != nil {
		return err
	}
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/lights/names/sync"
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
		Lights []core.LightNameView `json:"lights"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return err
	}
	if _, err := store.Merge(out.Lights); err != nil {
		return err
	}
	return nil
}

// TriggerLightNamesSync tente une synchronisation immédiate après une écriture locale.
// Ne bloque jamais l'appelant : l'envoi part dans une goroutine et un échec est seulement
// journalisé (le prochain cycle reprendra).
func (a *Agent) TriggerLightNamesSync() {
	if a == nil || !a.cfg.Enabled || a.validateHubURL() != nil {
		return
	}
	a.lightNamesMu.Lock()
	has := a.lightNames != nil
	a.lightNamesMu.Unlock()
	if !has {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), lightNamesSyncTimeout)
		defer cancel()
		if err := a.syncLightNames(ctx); err != nil {
			log.Printf("[cloudsync] light names sync (immediate): %v", err)
		}
	}()
}
