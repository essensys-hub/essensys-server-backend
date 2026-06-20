package cloudsync

import (
	"sync"
	"time"
)

// LANStatus is exposed read-only on GET /api/admin/cloudsync/status.
type LANStatus struct {
	Enabled              bool       `json:"enabled"`
	ScheduledSyncEnabled   bool       `json:"scheduled_sync_enabled"`
	HubURL               string     `json:"hub_url,omitempty"`
	GatewayID            string     `json:"gateway_id,omitempty"`
	LastPollAt           *time.Time `json:"last_poll_at,omitempty"`
	LastConfigError      string     `json:"last_config_error,omitempty"`
	PullActive           bool       `json:"pull_active"`
	SchedulerBusy        bool       `json:"scheduler_busy"`
	CachedProfileCount   int        `json:"cached_profile_count"`
	PendingRunsCount     int        `json:"pending_runs_count"`
	LastScheduledRunAt   *time.Time `json:"last_scheduled_run_at,omitempty"`
	LastScheduledRunName string     `json:"last_scheduled_run_name,omitempty"`
	LastScheduledStatus  string     `json:"last_scheduled_status,omitempty"`
}

type statusState struct {
	mu                   sync.RWMutex
	lastPollAt           time.Time
	lastConfigError      string
	pendingRunsCount     int
	lastScheduledRunAt   time.Time
	lastScheduledRunName string
	lastScheduledStatus  string
	hasLastRun           bool
}

func (a *Agent) Status() LANStatus {
	st := LANStatus{
		Enabled:            a.cfg.Enabled,
		ScheduledSyncEnabled: a.cfg.ScheduledSyncEnabled,
		HubURL:             a.cfg.HubURL,
		GatewayID:          a.cfg.GatewayID,
		SchedulerBusy:      a.syncRunning.Load(),
	}
	if a.pullScheduler != nil {
		st.PullActive = a.pullScheduler.IsActive()
	}
	a.profileMu.Lock()
	st.CachedProfileCount = len(a.cachedProfiles)
	a.profileMu.Unlock()

	a.status.mu.RLock()
	defer a.status.mu.RUnlock()
	if !a.status.lastPollAt.IsZero() {
		t := a.status.lastPollAt
		st.LastPollAt = &t
	}
	st.LastConfigError = a.status.lastConfigError
	st.PendingRunsCount = a.status.pendingRunsCount
	if a.status.hasLastRun {
		t := a.status.lastScheduledRunAt
		st.LastScheduledRunAt = &t
		st.LastScheduledRunName = a.status.lastScheduledRunName
		st.LastScheduledStatus = a.status.lastScheduledStatus
	}
	return st
}

func (a *Agent) recordPoll(cfg *cloudSyncConfigResponse, cfgErr error) {
	a.status.mu.Lock()
	defer a.status.mu.Unlock()
	a.status.lastPollAt = time.Now()
	if cfgErr != nil {
		a.status.lastConfigError = cfgErr.Error()
		return
	}
	a.status.lastConfigError = ""
	if cfg != nil {
		a.status.pendingRunsCount = len(cfg.PendingRuns)
	}
}

func (a *Agent) recordScheduledRun(name, status string) {
	a.status.mu.Lock()
	defer a.status.mu.Unlock()
	a.status.lastScheduledRunAt = time.Now()
	a.status.lastScheduledRunName = name
	a.status.lastScheduledStatus = status
	a.status.hasLastRun = true
}
