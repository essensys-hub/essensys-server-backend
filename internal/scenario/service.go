package scenario

import (
	"fmt"
	"sort"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// Service coordinates scenario reads and action enqueue.
type Service struct {
	store   data.Store
	actions *core.ActionService
}

func NewService(store data.Store, actions *core.ActionService) *Service {
	return &Service{store: store, actions: actions}
}

// SlotSummary describes one scenario slot for list API.
type SlotSummary struct {
	SlotNumber   int    `json:"slot_number"`
	Label        string `json:"label"`
	BaseIndex    int    `json:"base_index"`
	EndIndex     int    `json:"end_index"`
	Editable     bool   `json:"editable"`
	LastLaunched *int   `json:"last_launched,omitempty"`
}

// SlotDetail includes parameter values from exchange cache.
type SlotDetail struct {
	SlotSummary
	Params []protocol.ExchangeKV `json:"params"`
}

// List returns all scenario slots with optional last-launched from index 591.
func (s *Service) List(clientID string) ([]SlotSummary, error) {
	var last *int
	if v, ok := s.store.GetValue(clientID, IndexLastLaunched); ok {
		if n, err := parseByte(v); err == nil {
			last = &n
		}
	}

	out := make([]SlotSummary, 0, SlotCount)
	for slot := 1; slot <= SlotCount; slot++ {
		sum, err := s.summaryForSlot(slot, last)
		if err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return out, nil
}

func (s *Service) summaryForSlot(slot int, lastLaunched *int) (SlotSummary, error) {
	base, err := SlotBaseIndex(slot)
	if err != nil {
		return SlotSummary{}, err
	}
	label := DefaultSlotLabels[slot]
	sum := SlotSummary{
		SlotNumber: slot,
		Label:      label,
		BaseIndex:  base,
		EndIndex:   base + ParamCount - 1,
		Editable:   slot >= 2,
	}
	if lastLaunched != nil && *lastLaunched == slot {
		sum.LastLaunched = lastLaunched
	}
	return sum, nil
}

// Get reads slot parameters from the exchange store.
func (s *Service) Get(clientID string, slot int) (*SlotDetail, error) {
	if slot < 1 || slot > SlotCount {
		return nil, fmt.Errorf("scenario: invalid slot %d", slot)
	}
	start, end, err := SlotRange(slot)
	if err != nil {
		return nil, err
	}
	indices := make([]int, 0, ParamCount)
	for i := start; i <= end; i++ {
		indices = append(indices, i)
	}
	params := s.store.GetAllValues(clientID, indices)
	if params == nil {
		params = []protocol.ExchangeKV{}
	}
	sort.Slice(params, func(i, j int) bool { return params[i].K < params[j].K })

	var last *int
	if v, ok := s.store.GetValue(clientID, IndexLastLaunched); ok {
		if n, err := parseByte(v); err == nil {
			last = &n
		}
	}
	sum, err := s.summaryForSlot(slot, last)
	if err != nil {
		return nil, err
	}
	return &SlotDetail{SlotSummary: sum, Params: params}, nil
}

// Put validates and enqueues definition writes (slots 2–8).
func (s *Service) Put(clientID string, slot int, params map[int]string) ([]string, error) {
	chunks, err := WriteDefinitionChunks(slot, params)
	if err != nil {
		return nil, err
	}
	guids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunkGuids, err := s.actions.AddActions(clientID, chunk)
		if err != nil {
			return guids, err
		}
		guids = append(guids, chunkGuids...)
	}
	return guids, nil
}

// Launch enqueues Mode A params for slots 2–8.
func (s *Service) Launch(clientID string, slot int) (string, error) {
	params, err := LaunchParams(slot)
	if err != nil {
		return "", err
	}
	return s.actions.AddAction(clientID, params)
}

// Restore enqueues firmware preset init via Scenario_Efface.
func (s *Service) Restore(clientID string, slot int) (string, error) {
	params, err := RestorePresetParams(slot)
	if err != nil {
		return "", err
	}
	return s.actions.AddAction(clientID, params)
}

func parseByte(v string) (int, error) {
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	return n, err
}
