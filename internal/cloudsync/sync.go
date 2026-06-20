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
	"sync"
	"sync/atomic"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

type Config struct {
	Enabled              bool
	HubURL               string
	GatewayID            string
	GatewayToken         string
	PollIntervalSeconds  int
	ClientID             string
	Eth0MAC              string
	Eth1MAC              string
	ScheduledSyncEnabled bool
}

type cloudAction struct {
	GUID   string `json:"guid"`
	Params []struct {
		K int    `json:"k"`
		V string `json:"v"`
	} `json:"params"`
}

type Agent struct {
	cfg            Config
	client         *http.Client
	actionService  *core.ActionService
	store          data.Store
	pullScheduler  *core.ExchangePullScheduler
	syncRunning    atomic.Bool
	profileMu      sync.Mutex
	cachedProfiles []cloudSyncProfile
	status         statusState
}

func NewAgent(cfg Config, actionService *core.ActionService, store data.Store, pull *core.ExchangePullScheduler) *Agent {
	return &Agent{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		actionService: actionService,
		store:         store,
		pullScheduler: pull,
	}
}

func (a *Agent) Start(ctx context.Context) {
	if !a.cfg.Enabled {
		return
	}
	if err := a.validateHubURL(); err != nil {
		log.Printf("[cloudsync] disabled: %v", err)
		return
	}
	interval := time.Duration(a.cfg.PollIntervalSeconds) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	clientID := a.cfg.ClientID
	if clientID == "" {
		clientID = "default"
	}
	log.Printf("[cloudsync] started hub=%s gateway=%s scheduled_sync=%v interval=%s",
		a.cfg.HubURL, a.cfg.GatewayID, a.cfg.ScheduledSyncEnabled, interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		a.pollCycle(ctx, clientID)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.pollCycle(ctx, clientID)
			}
		}
	}()
}

func (a *Agent) validateHubURL() error {
	if !strings.HasPrefix(strings.ToLower(a.cfg.HubURL), "https://") {
		return fmt.Errorf("hub_url must use HTTPS, got %q", a.cfg.HubURL)
	}
	if a.cfg.GatewayID == "" || a.cfg.GatewayToken == "" {
		return fmt.Errorf("gateway_id and gateway_token required")
	}
	return nil
}

func (a *Agent) pollCycle(ctx context.Context, clientID string) {
	a.pollAndApply(ctx, clientID)
	cfg, err := a.fetchSyncConfig(ctx)
	a.recordPoll(cfg, err)
	if err != nil {
		log.Printf("[cloudsync] sync-config: %v", err)
	} else if cfg != nil && a.cfg.ScheduledSyncEnabled {
		a.runScheduledSync(ctx, clientID, cfg)
	}
	a.pushExchangeProfileAware(ctx, clientID)
	a.heartbeat(ctx)
}

func (a *Agent) pollAndApply(ctx context.Context, clientID string) {
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/pending-actions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	a.setGatewayHeaders(req)

	res, err := a.client.Do(req)
	if err != nil {
		log.Printf("[cloudsync] poll error: %v", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		log.Printf("[cloudsync] poll unauthorized — check gateway token registration")
		return
	}
	if res.StatusCode != http.StatusOK {
		log.Printf("[cloudsync] poll HTTP %d", res.StatusCode)
		return
	}

	body, _ := io.ReadAll(res.Body)
	var actions []cloudAction
	if err := json.Unmarshal(body, &actions); err != nil {
		log.Printf("[cloudsync] decode error: %v", err)
		return
	}

	for _, act := range actions {
		params := make([]protocol.ExchangeKV, len(act.Params))
		for i, p := range act.Params {
			params[i] = protocol.ExchangeKV{K: p.K, V: p.V}
		}
		if _, err := a.actionService.AddAction(clientID, params); err != nil {
			log.Printf("[cloudsync] apply %s: %v", act.GUID, err)
			continue
		}
		a.ackDone(ctx, act.GUID)
	}
}

func (a *Agent) ackDone(ctx context.Context, guid string) {
	url := fmt.Sprintf("%s/api/gateway/actions/%s/done", strings.TrimRight(a.cfg.HubURL, "/"), guid)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	a.setGatewayHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		log.Printf("[cloudsync] ack %s: %v", guid, err)
		return
	}
	res.Body.Close()
}

func (a *Agent) heartbeat(ctx context.Context) {
	url := strings.TrimRight(a.cfg.HubURL, "/") + "/api/gateway/heartbeat"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	a.setGatewayHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		log.Printf("[cloudsync] heartbeat: %v", err)
		return
	}
	res.Body.Close()
}

// exchangePushIndices lists exchange table keys synced to the cloud hub.
// Includes scenario block (590, 605-622), shutter travel times (566-585),
// heating schedule (13-348) and immediate modes (349-352).
func exchangePushIndices() []int {
	indices := []int{590}
	for i := 605; i <= 622; i++ {
		indices = append(indices, i)
	}
	for i := 566; i <= 572; i++ {
		indices = append(indices, i)
	}
	for i := 574; i <= 578; i++ {
		indices = append(indices, i)
	}
	for i := 582; i <= 585; i++ {
		indices = append(indices, i)
	}
	for i := 13; i <= 348; i++ {
		indices = append(indices, i)
	}
	for i := 349; i <= 352; i++ {
		indices = append(indices, i)
	}
	return indices
}

func (a *Agent) setGatewayHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.cfg.GatewayToken)
	req.Header.Set("X-Gateway-ID", a.cfg.GatewayID)
	if a.cfg.Eth0MAC != "" {
		req.Header.Set("X-Gateway-Eth0-MAC", a.cfg.Eth0MAC)
	}
	if a.cfg.Eth1MAC != "" {
		req.Header.Set("X-Gateway-Eth1-MAC", a.cfg.Eth1MAC)
	}
}
