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

// affichage-objets-2026-11-006 : synchronisation des dispositions (activation et ordre des
// lumières et des volets), dans les deux sens, par un appel par genre distinct de ceux des
// noms. Le magasin est le même fichier ; le cloud fusionne le document entier et la gateway
// applique la même règle (updated_at, version, updated_by) pour ne jamais écraser un document
// local plus récent.

var layoutKinds = []core.LayoutKind{core.LayoutLights, core.LayoutShutters}

// SetObjectLayout branche le magasin (le même que celui des noms ; nil : synchronisation désactivée).
func (a *Agent) SetObjectLayout(store *core.LightNameStore) {
	a.layoutMu.Lock()
	a.layoutStore = store
	a.layoutMu.Unlock()
}

// syncLayouts lance les deux synchronisations de dispositions l'une après l'autre ; l'échec
// de l'une (hors ligne, 404 d'un cloud pas encore déployé, 5xx) ne bloque pas l'autre.
func (a *Agent) syncLayouts(ctx context.Context) {
	for _, kind := range layoutKinds {
		if err := a.syncLayout(ctx, kind); err != nil {
			log.Printf("[cloudsync] %s layout sync: %v", kind, err)
		}
	}
}

func layoutKindIndex(kind core.LayoutKind) int {
	if kind == core.LayoutShutters {
		return 1
	}
	return 0
}

// syncLayout envoie le document du genre au cloud et adopte l'état fusionné renvoyé (seulement
// s'il est plus récent que le local). En cas d'échec la réplique locale est conservée ; le
// cycle suivant renvoie tout.
func (a *Agent) syncLayout(ctx context.Context, kind core.LayoutKind) error {
	a.layoutKindMu[layoutKindIndex(kind)].Lock()
	defer a.layoutKindMu[layoutKindIndex(kind)].Unlock()
	a.layoutMu.Lock()
	store := a.layoutStore
	a.layoutMu.Unlock()
	if store == nil || !store.Available() {
		return nil
	}
	body, err := json.Marshal(store.LayoutSyncPayload(kind))
	if err != nil {
		return err
	}
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/" + string(kind) + "/layout/sync"
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
	var out core.LayoutSyncDoc
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return err
	}
	_, err = store.MergeLayout(kind, out)
	return err
}

// TriggerLightsLayoutSync et TriggerShuttersLayoutSync tentent une synchronisation immédiate
// du genre après une écriture locale.
func (a *Agent) TriggerLightsLayoutSync()   { a.triggerLayoutSync(core.LayoutLights) }
func (a *Agent) TriggerShuttersLayoutSync() { a.triggerLayoutSync(core.LayoutShutters) }

// triggerLayoutSync ne bloque jamais l'appelant : l'envoi part dans une goroutine et un échec
// est seulement journalisé (le prochain cycle reprendra).
func (a *Agent) triggerLayoutSync(kind core.LayoutKind) {
	if a == nil || !a.cfg.Enabled || a.validateHubURL() != nil {
		return
	}
	a.layoutMu.Lock()
	has := a.layoutStore != nil
	a.layoutMu.Unlock()
	if !has {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), lightNamesSyncTimeout)
		defer cancel()
		if err := a.syncLayout(ctx, kind); err != nil {
			log.Printf("[cloudsync] %s layout sync (immediate): %v", kind, err)
		}
	}()
}
