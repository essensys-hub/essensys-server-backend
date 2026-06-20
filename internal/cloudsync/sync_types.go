package cloudsync

import (
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// cloudSyncProfile mirrors essensys-user-portal-backend/domain.SyncProfile (gateway JSON).
type cloudSyncProfile struct {
	ID              string        `json:"id"`
	GatewayID       string        `json:"gateway_id"`
	Name            string        `json:"name"`
	IndexRanges     [][2]int      `json:"index_ranges"`
	IntervalHours   int           `json:"interval_hours"`
	CronExpression  string        `json:"cron_expression,omitempty"`
	PullFromArmoire bool          `json:"pull_from_armoire"`
	PushToCloud     bool          `json:"push_to_cloud"`
	Enabled         bool          `json:"enabled"`
	ExcludeIndices  []int         `json:"exclude_indices"`
	LastRunAt       *time.Time    `json:"last_run_at"`
}

type cloudSyncRun struct {
	ID            string `json:"id"`
	ProfileID     string `json:"profile_id"`
	GatewayID     string `json:"gateway_id"`
	Status        string `json:"status"`
	ExpectedCount int    `json:"expected_count"`
	ReceivedCount int    `json:"received_count"`
	PushedCount   int    `json:"pushed_count"`
}

type cloudSyncConfigResponse struct {
	Profiles    []cloudSyncProfile `json:"profiles"`
	PendingRuns []cloudSyncRun     `json:"pending_runs"`
}

type cloudSyncProgressBody struct {
	ReceivedCount int    `json:"received_count"`
	PushedCount   int    `json:"pushed_count"`
	ChunkIndex    int    `json:"chunk_index"`
	ChunkTotal    int    `json:"chunk_total"`
	Phase         string `json:"phase"`
	Message       string `json:"message"`
	Status        string `json:"status"`
	ErrorMessage  string `json:"error_message"`
}

type cloudCreateRunBody struct {
	ProfileID string `json:"profile_id"`
	Source    string `json:"source"`
}

type cloudCreateRunResponse struct {
	Run cloudSyncRun `json:"run"`
}

const (
	syncRunStatusPending = "pending"
	syncRunSuccess       = "success"
	syncRunPartial       = "partial"
	syncRunFailed        = "failed"
)

func profileDue(p cloudSyncProfile, now time.Time) bool {
	if !p.Enabled || !p.PullFromArmoire {
		return false
	}
	if p.CronExpression != "" {
		sched, err := cron.ParseStandard(p.CronExpression)
		if err != nil {
			log.Printf("[cloudsync] invalid cron %q on profile %q: %v", p.CronExpression, p.Name, err)
		} else {
			from := now.Add(-365 * 24 * time.Hour)
			if p.LastRunAt != nil {
				from = *p.LastRunAt
			}
			return !sched.Next(from).After(now)
		}
	}
	hours := p.IntervalHours
	if hours <= 0 {
		hours = 3
	}
	if p.LastRunAt == nil {
		return true
	}
	return now.Sub(*p.LastRunAt) >= time.Duration(hours)*time.Hour
}

func findProfile(profiles []cloudSyncProfile, id string) *cloudSyncProfile {
	for i := range profiles {
		if profiles[i].ID == id {
			return &profiles[i]
		}
	}
	return nil
}
