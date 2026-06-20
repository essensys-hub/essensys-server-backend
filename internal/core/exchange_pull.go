package core

import (
	"sync"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
)

// MaxServerInfoIndices matches firmware BP_MQX_ETH (Json.c / ERREUR_INFOS_NB_VALEURS_MAX).
const MaxServerInfoIndices = 30

// ExchangePullScheduler rotates exchange indices through GET /api/serverinfos so the
// armoire reports them via POST /api/mystatus (firmware limit: 30 indices per cycle).
type ExchangePullScheduler struct {
	mu        sync.Mutex
	target    []int
	chunkIdx  int
	active    bool
	startedAt time.Time
}

// HeatingSyncManager is kept as an alias for backward compatibility.
type HeatingSyncManager = ExchangePullScheduler

// HeatingSyncStatus is returned to the UI while polling exchange after a sync request.
type HeatingSyncStatus struct {
	Active          bool      `json:"active"`
	StartIndex      int       `json:"startIndex"`
	ByteCount       int       `json:"byteCount"`
	Received        int       `json:"received"`
	Total           int       `json:"total"`
	ChunksTotal     int       `json:"chunksTotal"`
	ChunksCompleted int       `json:"chunksCompleted"`
	StartedAt       time.Time `json:"startedAt,omitempty"`
}

func NewExchangePullScheduler() *ExchangePullScheduler {
	return &ExchangePullScheduler{}
}

func NewHeatingSyncManager() *ExchangePullScheduler {
	return NewExchangePullScheduler()
}

func (m *ExchangePullScheduler) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

func (m *ExchangePullScheduler) Start(startIndex, byteCount int) int {
	indices := make([]int, byteCount)
	for i := 0; i < byteCount; i++ {
		indices[i] = startIndex + i
	}
	m.startIndices(indices)
	return chunkCount(byteCount)
}

// TryStart returns false if a pull is already in progress.
func (m *ExchangePullScheduler) TryStart(startIndex, byteCount int) (chunks int, ok bool) {
	indices := make([]int, byteCount)
	for i := 0; i < byteCount; i++ {
		indices[i] = startIndex + i
	}
	return m.tryStartIndices(indices)
}

func (m *ExchangePullScheduler) StartIndices(indices []int) int {
	m.startIndices(indices)
	return chunkCount(len(indices))
}

func (m *ExchangePullScheduler) TryStartIndices(indices []int) (chunks int, ok bool) {
	return m.tryStartIndices(indices)
}

func (m *ExchangePullScheduler) startIndices(indices []int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.target = append([]int(nil), indices...)
	m.chunkIdx = 0
	m.active = len(indices) > 0
	m.startedAt = time.Now()
}

func (m *ExchangePullScheduler) tryStartIndices(indices []int) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return 0, false
	}
	m.target = append([]int(nil), indices...)
	m.chunkIdx = 0
	m.active = len(indices) > 0
	m.startedAt = time.Now()
	return chunkCount(len(indices)), true
}

func chunkCount(byteCount int) int {
	if byteCount <= 0 {
		return 0
	}
	return (byteCount + MaxServerInfoIndices - 1) / MaxServerInfoIndices
}

func (m *ExchangePullScheduler) CurrentChunk() ([]int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || len(m.target) == 0 {
		return nil, false
	}
	start := m.chunkIdx * MaxServerInfoIndices
	if start >= len(m.target) {
		m.active = false
		return nil, false
	}
	end := start + MaxServerInfoIndices
	if end > len(m.target) {
		end = len(m.target)
	}
	out := make([]int, end-start)
	copy(out, m.target[start:end])
	return out, true
}

func (m *ExchangePullScheduler) Advance() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		return
	}
	m.chunkIdx++
	if m.chunkIdx*MaxServerInfoIndices >= len(m.target) {
		m.active = false
	}
}

func (m *ExchangePullScheduler) Status(store data.Store, clientID string, startIndex, byteCount int) HeatingSyncStatus {
	m.mu.Lock()
	active := m.active
	chunkIdx := m.chunkIdx
	startedAt := m.startedAt
	idxStart := startIndex
	total := byteCount
	if len(m.target) > 0 {
		idxStart = m.target[0]
		total = len(m.target)
	}
	m.mu.Unlock()

	chunksTotal := chunkCount(total)
	chunksCompleted := chunkIdx
	if !active && chunksTotal > 0 {
		chunksCompleted = chunksTotal
	}

	received := countReceived(store, clientID, idxStart, total)

	return HeatingSyncStatus{
		Active:          active,
		StartIndex:      idxStart,
		ByteCount:       total,
		Received:        received,
		Total:           total,
		ChunksTotal:     chunksTotal,
		ChunksCompleted: chunksCompleted,
		StartedAt:       startedAt,
	}
}

func countReceived(store data.Store, clientID string, startIndex, total int) int {
	if store == nil || total <= 0 {
		return 0
	}
	keys := make([]int, total)
	for i := 0; i < total; i++ {
		keys[i] = startIndex + i
	}
	return len(store.GetAllValues(clientID, keys))
}

// CountReceived returns how many of the given indices are present in exchange Redis.
func CountReceived(store data.Store, clientID string, indices []int) int {
	if store == nil || len(indices) == 0 {
		return 0
	}
	return len(store.GetAllValues(clientID, indices))
}

// FlattenIndexRanges expands inclusive [start,end] pairs into a sorted index list.
func FlattenIndexRanges(ranges [][2]int) []int {
	var out []int
	for _, rg := range ranges {
		if rg[0] > rg[1] {
			continue
		}
		for k := rg[0]; k <= rg[1]; k++ {
			out = append(out, k)
		}
	}
	return out
}
