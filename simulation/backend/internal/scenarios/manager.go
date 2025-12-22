package scenarios

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"simulation/internal/client"
)

const ScenarioDir = "scenarios_data"

type Manager struct {
	mu sync.RWMutex
}

func NewManager() *Manager {
	// Ensure directory exists
	if _, err := os.Stat(ScenarioDir); os.IsNotExist(err) {
		os.Mkdir(ScenarioDir, 0755)
	}
	return &Manager{}
}

type ScenarioWrapper struct {
	Name  string                `json:"name"`
	Steps []client.ScenarioStep `json:"steps"`
}

func (m *Manager) ListScenarios() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, err := ioutil.ReadDir(ScenarioDir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
			names = append(names, strings.TrimSuffix(f.Name(), ".json"))
		}
	}
	return names, nil
}

func (m *Manager) SaveScenario(name string, steps []client.ScenarioStep) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(ScenarioDir, name+".json")
	data, err := json.MarshalIndent(steps, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, data, 0644)
}

func (m *Manager) LoadScenario(name string) ([]client.ScenarioStep, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	path := filepath.Join(ScenarioDir, name+".json")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var steps []client.ScenarioStep
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, err
	}

	return steps, nil
}
