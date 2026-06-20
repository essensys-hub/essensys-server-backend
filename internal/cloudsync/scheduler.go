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

func (a *Agent) runScheduledSync(ctx context.Context, clientID string, cfg *cloudSyncConfigResponse) {
	if cfg == nil {
		return
	}
	if !a.syncRunning.CompareAndSwap(false, true) {
		return
	}
	defer a.syncRunning.Store(false)

	for _, run := range cfg.PendingRuns {
		if ctx.Err() != nil {
			return
		}
		if run.Status != syncRunStatusPending {
			continue
		}
		profile := findProfile(cfg.Profiles, run.ProfileID)
		if profile == nil {
			log.Printf("[cloudsync] sync run %s: profile %s not found", run.ID, run.ProfileID)
			continue
		}
		a.executeSyncRun(ctx, clientID, run, *profile)
	}

	now := time.Now()
	for _, profile := range cfg.Profiles {
		if ctx.Err() != nil {
			return
		}
		if !profileDue(profile, now) {
			continue
		}
		if pendingRunForProfile(cfg.PendingRuns, profile.ID) != nil {
			continue
		}
		run, err := a.createScheduledRun(ctx, profile.ID)
		if err != nil {
			log.Printf("[cloudsync] scheduled run create %s: %v", profile.Name, err)
			continue
		}
		a.executeSyncRun(ctx, clientID, run, profile)
		break // one due profile per poll tick — avoids pull lock contention
	}
}

func pendingRunForProfile(runs []cloudSyncRun, profileID string) *cloudSyncRun {
	for i := range runs {
		if runs[i].ProfileID == profileID {
			return &runs[i]
		}
	}
	return nil
}

func (a *Agent) fetchSyncConfig(ctx context.Context) (*cloudSyncConfigResponse, error) {
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/sync-config"
	req, err := httpNewRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	a.setGatewayHeaders(req)
	res, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized")
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var out cloudSyncConfigResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	a.cacheSyncProfiles(out.Profiles)
	return &out, nil
}

func (a *Agent) cacheSyncProfiles(profiles []cloudSyncProfile) {
	a.profileMu.Lock()
	defer a.profileMu.Unlock()
	a.cachedProfiles = profiles
}

func (a *Agent) createScheduledRun(ctx context.Context, profileID string) (cloudSyncRun, error) {
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/sync-runs"
	body, _ := json.Marshal(cloudCreateRunBody{ProfileID: profileID, Source: "scheduled"})
	req, err := httpNewRequest(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return cloudSyncRun{}, err
	}
	a.setGatewayHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return cloudSyncRun{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		return cloudSyncRun{}, fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out cloudCreateRunResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return cloudSyncRun{}, err
	}
	return out.Run, nil
}

func (a *Agent) executeSyncRun(ctx context.Context, clientID string, run cloudSyncRun, profile cloudSyncProfile) {
	log.Printf("[cloudsync] sync run %s — profile %q", run.ID, profile.Name)
	_ = a.postSyncRunStart(ctx, run.ID)

	indices := core.FlattenIndexRanges(profile.IndexRanges)
	expected := len(indices)
	if expected == 0 {
		a.reportSyncProgress(ctx, run.ID, cloudSyncProgressBody{
			Status: syncRunFailed, Phase: "done", Message: "no indices in profile",
			ErrorMessage: "empty index_ranges",
		})
		return
	}

	chunks := chunkCount(expected)
	received := core.CountReceived(a.store, clientID, indices)
	if profile.PullFromArmoire {
		a.reportSyncProgress(ctx, run.ID, cloudSyncProgressBody{
			Phase: "pull", ChunkTotal: chunks, Message: fmt.Sprintf("Pull armoire — %s (%d octets, %d cycles)", profile.Name, expected, chunks),
		})
		if a.pullScheduler == nil {
			a.reportSyncProgress(ctx, run.ID, cloudSyncProgressBody{
				Status: syncRunFailed, Phase: "done", ErrorMessage: "pull scheduler unavailable",
			})
			return
		}
		okChunks, ok := a.pullScheduler.TryStartIndices(indices)
		if !ok {
			a.reportSyncProgress(ctx, run.ID, cloudSyncProgressBody{
				Status: syncRunFailed, Phase: "done",
				Message:      "Pull armoire occupé (sync manuelle ou autre profil en cours)",
				ErrorMessage: "pull busy",
			})
			return
		}
		chunks = okChunks
		timeout := pullWaitTimeout(chunks, expected)
		received = a.waitForPull(ctx, clientID, run.ID, indices, chunks, timeout)
	}

	pushed := 0
	if profile.PushToCloud && received > 0 {
		a.reportSyncProgress(ctx, run.ID, cloudSyncProgressBody{
			Phase: "push", ReceivedCount: received, ChunkTotal: chunks,
			Message: fmt.Sprintf("Push cloud — %d octets", received),
		})
		pushKeys := filterExcludedIndices(indices, profile.ExcludeIndices)
		pushed = a.pushExchangeKeys(ctx, clientID, pushKeys)
	}

	status := syncRunSuccess
	if received < expected {
		status = syncRunPartial
	}
	if received == 0 && profile.PullFromArmoire {
		status = syncRunFailed
	}

	a.reportSyncProgress(ctx, run.ID, cloudSyncProgressBody{
		Status: status, Phase: "done", ReceivedCount: received, PushedCount: pushed,
		ChunkTotal: chunks, ChunkIndex: chunks,
		Message: fmt.Sprintf("Terminé — %d/%d octets reçus, %d poussés cloud", received, expected, pushed),
	})
	log.Printf("[cloudsync] sync run %s done — %s (%d/%d)", run.ID, status, received, expected)
	a.recordScheduledRun(profile.Name, status)
}

func chunkCount(n int) int {
	if n <= 0 {
		return 0
	}
	return (n + core.MaxServerInfoIndices - 1) / core.MaxServerInfoIndices
}

func pullWaitTimeout(chunks, expected int) time.Duration {
	sec := chunks*25 + 30
	if sec < 60 {
		sec = 60
	}
	max := expected / 10
	if max > sec {
		sec = max
	}
	if sec > 600 {
		sec = 600
	}
	return time.Duration(sec) * time.Second
}

func (a *Agent) waitForPull(ctx context.Context, clientID, runID string, indices []int, chunks int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	lastReceived := -1
	lastReport := time.Time{}
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			break
		}
		received := core.CountReceived(a.store, clientID, indices)
		if received != lastReceived || time.Since(lastReport) > 10*time.Second {
			chunkIdx := received / core.MaxServerInfoIndices
			if chunkIdx > chunks {
				chunkIdx = chunks
			}
			a.reportSyncProgress(ctx, runID, cloudSyncProgressBody{
				Phase: "pull", ReceivedCount: received, ChunkIndex: chunkIdx, ChunkTotal: chunks,
				Message: fmt.Sprintf("Attente armoire… %d/%d octets", received, len(indices)),
			})
			lastReceived = received
			lastReport = time.Now()
		}
		if received >= len(indices) && !a.pullScheduler.IsActive() {
			return received
		}
		select {
		case <-ctx.Done():
			return received
		case <-time.After(3 * time.Second):
		}
	}
	return core.CountReceived(a.store, clientID, indices)
}

func (a *Agent) postSyncRunStart(ctx context.Context, runID string) error {
	url := fmt.Sprintf("%s/api/gateway/sync-runs/%s/start", strings.TrimRight(a.cfg.HubURL, "/"), runID)
	req, err := httpNewRequest(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	a.setGatewayHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}

func (a *Agent) reportSyncProgress(ctx context.Context, runID string, body cloudSyncProgressBody) {
	url := fmt.Sprintf("%s/api/gateway/sync-runs/%s/progress", strings.TrimRight(a.cfg.HubURL, "/"), runID)
	raw, _ := json.Marshal(body)
	req, err := httpNewRequest(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return
	}
	a.setGatewayHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		log.Printf("[cloudsync] progress %s: %v", runID, err)
		return
	}
	res.Body.Close()
}

func (a *Agent) pushExchangeKeys(ctx context.Context, clientID string, keys []int) int {
	if a.store == nil || len(keys) == 0 {
		return 0
	}
	vals := a.store.GetAllValues(clientID, keys)
	if len(vals) == 0 {
		return 0
	}
	body, err := json.Marshal(map[string]any{"keys": vals})
	if err != nil {
		return 0
	}
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/exchange"
	req, err := httpNewRequest(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0
	}
	a.setGatewayHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		log.Printf("[cloudsync] exchange push: %v", err)
		return 0
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Printf("[cloudsync] exchange push HTTP %d", res.StatusCode)
		return 0
	}
	return len(vals)
}

// pushIndicesFromProfiles returns union of indices from cached sync profiles, or fallback list.
func (a *Agent) pushIndicesFromProfiles(clientID string) []int {
	a.profileMu.Lock()
	profiles := a.cachedProfiles
	a.profileMu.Unlock()

	if len(profiles) == 0 {
		return exchangePushIndices()
	}

	out := pushIndicesFromProfilesList(profiles)
	if len(out) == 0 {
		return exchangePushIndices()
	}
	return out
}

func (a *Agent) pushExchangeProfileAware(ctx context.Context, clientID string) {
	if a.store == nil {
		return
	}
	keys := a.pushIndicesFromProfiles(clientID)
	vals := a.store.GetAllValues(clientID, keys)
	if len(vals) == 0 {
		return
	}
	body, err := json.Marshal(map[string]any{"keys": vals})
	if err != nil {
		return
	}
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/exchange"
	req, err := httpNewRequest(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	a.setGatewayHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		log.Printf("[cloudsync] exchange push: %v", err)
		return
	}
	res.Body.Close()
}

// httpNewRequest wraps http.NewRequestWithContext for testability.
var httpNewRequest = func(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, url, body)
}
