package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// Signalement d'un appui de sonnette au cloud (sonnette-plages-push-2026-11-008) :
// POST /api/gateway/doorbell/ring, même authentification que les autres appels de la gateway.

var (
	// doorbellRingWaits : attentes avant la 2e et la 3e tentative.
	doorbellRingWaits = []time.Duration{1 * time.Second, 3 * time.Second}
	// doorbellRingDeadline : au-delà, l'appui est abandonné (une sonnerie ancienne ne sert plus).
	doorbellRingDeadline = 60 * time.Second
)

const doorbellRingTimeLayout = "2006-01-02T15:04:05.000Z"

// NotifyDoorbellRing envoie l'appui au cloud, en 3 tentatives au plus. Il bloque : l'appelant
// le lance en tâche de fond. Cloud non configuré : aucun appel, aucune erreur. Ni le jeton ni
// l'adresse ne sont journalisés.
func (a *Agent) NotifyDoorbellRing(ringID string, at time.Time) {
	if !a.cfg.Enabled || a.cfg.GatewayToken == "" || a.cfg.GatewayID == "" || a.validateHubURL() != nil {
		return
	}
	body, _ := json.Marshal(map[string]string{"ring_id": ringID, "at": at.UTC().Format(doorbellRingTimeLayout)})
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/doorbell/ring"
	ctx, cancel := context.WithTimeout(context.Background(), doorbellRingDeadline)
	defer cancel()

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			t := time.NewTimer(doorbellRingWaits[attempt-1])
			select {
			case <-ctx.Done():
				t.Stop()
				log.Printf("Sonnette : signalement au cloud abandonné")
				return
			case <-t.C:
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		a.setGatewayHeaders(req)
		req.Header.Set("Content-Type", "application/json")
		res, err := a.client.Do(req)
		if err == nil {
			res.Body.Close()
			if res.StatusCode >= 200 && res.StatusCode < 300 {
				return
			}
			if res.StatusCode >= 400 && res.StatusCode < 500 {
				// Jeton refusé, limite atteinte… : insister ne servirait à rien.
				log.Printf("Sonnette : signalement au cloud refusé (%d)", res.StatusCode)
				return
			}
		}
	}
	log.Printf("Sonnette : signalement au cloud abandonné après 3 tentatives")
}
