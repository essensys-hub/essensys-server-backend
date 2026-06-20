package scenario

import (
	"fmt"
	"sort"

	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// WriteDefinitionChunks splits slot definition into firmware-sized action chunks (max 30 params).
// Does not include index 590 (definition write, not launch).
func WriteDefinitionChunks(slot int, params map[int]string) ([][]protocol.ExchangeKV, error) {
	if err := ValidateDefinition(slot, params); err != nil {
		return nil, err
	}
	if slot == 1 {
		return nil, ErrSlot1ServerReserved
	}

	start, end, err := SlotRange(slot)
	if err != nil {
		return nil, err
	}

	kvs := make([]protocol.ExchangeKV, 0, ParamCount)
	for i := start; i <= end; i++ {
		v, ok := params[i]
		if !ok {
			v = "0"
		}
		kvs = append(kvs, protocol.ExchangeKV{K: i, V: v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].K < kvs[j].K })

	return chunkParams(kvs, protocol.MaxParamsPerFirmwareAction), nil
}

// RestorePresetParams returns params to trigger firmware preset init via Scenario_Efface.
func RestorePresetParams(slot int) ([]protocol.ExchangeKV, error) {
	if slot < 2 || slot > 6 {
		return nil, fmt.Errorf("scenario: restore preset supported for slots 2–6, got %d", slot)
	}
	efface, ok := PresetEffaceValue[slot]
	if !ok {
		return nil, fmt.Errorf("scenario: no preset efface value for slot %d", slot)
	}
	idx, err := AbsoluteIndex(slot, OffsetEfface)
	if err != nil {
		return nil, err
	}
	return []protocol.ExchangeKV{{K: idx, V: efface}}, nil
}

func chunkParams(params []protocol.ExchangeKV, max int) [][]protocol.ExchangeKV {
	if len(params) == 0 {
		return nil
	}
	if max <= 0 || len(params) <= max {
		return [][]protocol.ExchangeKV{params}
	}
	out := make([][]protocol.ExchangeKV, 0, (len(params)+max-1)/max)
	for i := 0; i < len(params); i += max {
		end := i + max
		if end > len(params) {
			end = len(params)
		}
		out = append(out, params[i:end])
	}
	return out
}
