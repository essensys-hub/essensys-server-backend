package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ScenariosSyncLAN is returned by GET /api/admin/scenarios/sync.
type ScenariosSyncLAN struct {
	Enabled   bool   `json:"enabled"`
	ProfileID string `json:"profile_id,omitempty"`
	Found     bool   `json:"found"`
	Message   string `json:"message,omitempty"`
}

func (a *Agent) ScenariosSyncStatus() ScenariosSyncLAN {
	if a == nil || !a.cfg.Enabled {
		return ScenariosSyncLAN{Message: "cloud sync agent not configured"}
	}
	a.profileMu.Lock()
	defer a.profileMu.Unlock()
	p := findScenariosProfile(a.cachedProfiles)
	if p == nil {
		return ScenariosSyncLAN{Found: false, Message: "profil Scénarios absent (sync-config)"}
	}
	return ScenariosSyncLAN{
		Enabled:   p.Enabled,
		ProfileID: p.ID,
		Found:     true,
	}
}

func (a *Agent) SetScenariosSyncEnabled(ctx context.Context, enabled bool) error {
	if a == nil || !a.cfg.Enabled {
		return fmt.Errorf("cloud sync agent not configured")
	}
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/sync-profiles/scenarios"
	body, _ := json.Marshal(map[string]bool{"enabled": enabled})
	req, err := httpNewRequest(ctx, http.MethodPatch, url, bytes.NewReader(body))
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
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Profile cloudSyncProfile `json:"profile"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return err
	}
	a.profileMu.Lock()
	for i := range a.cachedProfiles {
		if a.cachedProfiles[i].ID == out.Profile.ID {
			a.cachedProfiles[i].Enabled = out.Profile.Enabled
			break
		}
	}
	a.profileMu.Unlock()
	return nil
}
