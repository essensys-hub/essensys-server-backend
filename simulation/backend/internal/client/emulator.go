package client

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Emulator simulates a single BP_MQX_ETH client.
type Emulator struct {
	ID            string // Internal ID (UUID)
	Serial        string // Serial Number (Source for Matricule)
	Matricule     string // Generated Auth Token (Base64(MD5(Serial)))
	ServerURL     string
	TargetIndices []int // Indices to monitor (from serverinfos)

	// State
	mu      sync.RWMutex
	Values  map[int]string // Current values of the exchange table
	History []string       // Log of last 20 events/values
	Active  bool
	Client  *http.Client
}

func NewEmulator(id, serial, serverURL string) *Emulator {
	e := &Emulator{
		ID:        id,
		Serial:    serial,
		ServerURL: serverURL,
		Values:    make(map[int]string),
		History:   make([]string, 0),
		Client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
	e.Matricule = e.GenerateAuth()
	return e
}

func (e *Emulator) Start() {
	e.Active = true
	go e.loop()
}

func (e *Emulator) Stop() {
	e.Active = false
}

func (e *Emulator) loop() {
	// Initial connection: Get Server Infos
	e.getServerInfos()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for e.Active {
		select {
		case <-ticker.C:
			e.poll()
		}
	}
}

func (e *Emulator) poll() {
	// 1. Send Status (POST /api/mystatus)
	e.postMyStatus()

	// 2. Get Actions (GET /api/myactions)
	e.getMyActions()
}

func (e *Emulator) getServerInfos() {
	msg := "GET /api/serverinfos"
	req, _ := http.NewRequest("GET", e.ServerURL+"/api/serverinfos", nil)
	req.Header.Set("Connection", "close")

	resp, err := e.Client.Do(req)
	if err != nil {
		e.logHistory(fmt.Sprintf("Error %s: %v", msg, err))
		return
	}
	defer resp.Body.Close()
	// e.logHistory(fmt.Sprintf("Success %s: %d", msg, resp.StatusCode))
}

func (e *Emulator) postMyStatus() {
	// Construct the bad JSON manually
	var ekParts []string

	e.mu.RLock()
	for k, v := range e.Values {
		ekParts = append(ekParts, fmt.Sprintf("{k:%d,v:\"%s\"}", k, v))
	}
	e.mu.RUnlock()

	if len(ekParts) == 0 {
		ekParts = append(ekParts, "{k:605,v:\"0\"}")
	}

	bodyStr := fmt.Sprintf(`{version:"1.0",ek:[%s]}`, strings.Join(ekParts, ","))

	req, _ := http.NewRequest("POST", e.ServerURL+"/api/mystatus", strings.NewReader(bodyStr))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "close")
	req.Header.Set("Authorization", "Basic "+e.Matricule)

	resp, err := e.Client.Do(req)
	if err != nil {
		e.logHistory(fmt.Sprintf("Error POST /api/mystatus: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		e.logHistory(fmt.Sprintf("Warning POST /api/mystatus: Status %d", resp.StatusCode))
	}
}

func (e *Emulator) getMyActions() {
	req, _ := http.NewRequest("GET", e.ServerURL+"/api/myactions", nil)
	req.Header.Set("Connection", "close")
	req.Header.Set("Authorization", "Basic "+e.Matricule)

	resp, err := e.Client.Do(req)
	if err != nil {
		e.logHistory(fmt.Sprintf("Error GET /api/myactions: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		e.logHistory(fmt.Sprintf("Warning GET /api/myactions: Status %d", resp.StatusCode))
		return
	}

	// Parse Actions
	var response struct {
		Actions []struct {
			Index int    `json:"index"`
			Value string `json:"value"`
		} `json:"actions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		e.logHistory(fmt.Sprintf("Error decoding actions: %v", err))
		return
	}

	if len(response.Actions) > 0 {
		e.mu.Lock()
		for _, action := range response.Actions {
			e.Values[action.Index] = action.Value
			e.logHistory(fmt.Sprintf("ACTION RECEIVED: Set [%d]=%s", action.Index, action.Value))
		}
		e.mu.Unlock()

		// Acknowledge? The legacy protocol usually has a separate ACK mechanism or just assumes done.
		// The current backend dequeue implementation assumes it is sent and done.
	}
}

func (e *Emulator) logHistory(msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	e.History = append(e.History, entry)
	if len(e.History) > 20 {
		e.History = e.History[1:]
	}
}

// Custom JSON marshaler to safely exclude Client field
func (e *Emulator) MarshalJSON() ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Explicitly list fields to marshal to avoid any reflection issues with http.Client
	return json.Marshal(&struct {
		ID            string         `json:"ID"`
		Serial        string         `json:"Serial"`
		Matricule     string         `json:"Matricule"`
		ServerURL     string         `json:"ServerURL"`
		TargetIndices []int          `json:"TargetIndices"`
		Values        map[int]string `json:"Values"`
		History       []string       `json:"History"`
		Active        bool           `json:"Active"`
	}{
		ID:            e.ID,
		Serial:        e.Serial,
		Matricule:     e.Matricule,
		ServerURL:     e.ServerURL,
		TargetIndices: e.TargetIndices,
		Values:        e.Values,
		History:       e.History,
		Active:        e.Active,
	})
}

// ScenarioJob defines a single action
type ScenarioJob struct {
	Index int    `json:"index"`
	Value string `json:"value"`
}

// ScenarioStep defines a group of actions with a delay
type ScenarioStep struct {
	Jobs  []ScenarioJob `json:"jobs"`
	Delay int           `json:"delay"`
}

func (e *Emulator) ExecuteScenario(steps []ScenarioStep) {
	go func() {
		e.logHistory(fmt.Sprintf("Starting scenario with %d steps", len(steps)))

		for i, step := range steps {
			for _, job := range step.Jobs {
				e.mu.Lock()
				e.Values[job.Index] = job.Value
				e.mu.Unlock()
				e.logHistory(fmt.Sprintf("Scenario Step %d: Set [%d]=%s", i+1, job.Index, job.Value))
			}

			// Force immediate update to server
			e.postMyStatus()

			if step.Delay > 0 {
				e.logHistory(fmt.Sprintf("Waiting %dms...", step.Delay))
				time.Sleep(time.Duration(step.Delay) * time.Millisecond)
			}
		}
		e.logHistory("Scenario complete")
	}()
}

func (e *Emulator) GenerateAuth() string {
	keyBytes, err := hex.DecodeString(e.Serial)
	if err != nil {
		keyHash := md5.Sum([]byte(e.Serial))
		keyBytes = keyHash[:]
	}

	weirdBuffer := make([]byte, 32)
	for i, b := range keyBytes {
		if i >= 16 {
			break
		}
		weirdBuffer[i*2] = (b & 0x0F) + '0'
		weirdBuffer[i*2+1] = ((b >> 4) & 0x0F) + '0'
	}

	hasher := md5.New()
	hasher.Write(weirdBuffer)
	sum := hasher.Sum(nil)

	hexStr := hex.EncodeToString(sum)
	firstHalf := hexStr[:16]
	secondHalf := hexStr[16:]
	combined := firstHalf + ":" + secondHalf

	return base64.StdEncoding.EncodeToString([]byte(combined))
}
