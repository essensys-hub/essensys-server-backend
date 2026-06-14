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
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

type Config struct {
	Enabled             bool
	HubURL              string
	GatewayID           string
	GatewayToken        string
	PollIntervalSeconds int
	ClientID            string
	Eth0MAC             string
	Eth1MAC             string
}

type cloudAction struct {
	GUID   string `json:"guid"`
	Params []struct {
		K int    `json:"k"`
		V string `json:"v"`
	} `json:"params"`
}

type Agent struct {
	cfg           Config
	client        *http.Client
	actionService *core.ActionService
}

func NewAgent(cfg Config, actionService *core.ActionService) *Agent {
	return &Agent{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		actionService: actionService,
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
	log.Printf("[cloudsync] started hub=%s gateway=%s eth0=%s eth1=%s interval=%s",
		a.cfg.HubURL, a.cfg.GatewayID, a.cfg.Eth0MAC, a.cfg.Eth1MAC, interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		a.heartbeat(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.pollAndApply(ctx, clientID)
				a.heartbeat(ctx)
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
