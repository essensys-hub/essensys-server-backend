package fleet

import (
	"fmt"
	"log"
	"simulation/internal/client"
	"sync"
	"time"
)

type Manager struct {
	mu      sync.RWMutex
	Clients map[string]*client.Emulator
}

func NewManager() *Manager {
	return &Manager{
		Clients: make(map[string]*client.Emulator),
	}
}

func (m *Manager) AddClient(id, serial, serverURL string) *client.Emulator {
	m.mu.Lock()
	defer m.mu.Unlock()

	emu := client.NewEmulator(id, serial, serverURL)
	m.Clients[id] = emu
	return emu
}

func (m *Manager) StartRampUp(count int, serverURL string, startupScenario []client.ScenarioStep) {
	log.Printf("[Manager] StartRampUp requested for %d clients targeting %s", count, serverURL)
	go func() {
		for i := 0; i < count; i++ {
			id := fmt.Sprintf("client-%d", i)
			serial := fmt.Sprintf("%032x", i)

			log.Printf("[Manager] Creating client %s", id)
			emu := m.AddClient(id, serial, serverURL)

			emu.Start()

			if len(startupScenario) > 0 {
				log.Printf("[Manager] Applying startup scenario to client %s", id)
				emu.ExecuteScenario(startupScenario)
			}

			if (i+1)%5 == 0 {
				time.Sleep(1 * time.Second)
			}
		}
		log.Printf("[Manager] Ramp-up complete")
	}()
}

func (m *Manager) GetAllClients() []*client.Emulator {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*client.Emulator, 0, len(m.Clients))
	for _, c := range m.Clients {
		list = append(list, c)
	}
	return list
}

func (m *Manager) StopClient(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if emu, ok := m.Clients[id]; ok {
		emu.Stop()
		delete(m.Clients, id)
	}
}

func (m *Manager) StopAllClients() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, emu := range m.Clients {
		emu.Stop()
	}
	// Clear the map
	m.Clients = make(map[string]*client.Emulator)
}

func (m *Manager) GetClient(id string) (*client.Emulator, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.Clients[id]
	return c, ok
}
