package scenario

import (
	"fmt"
	"strconv"

	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// IsMemorizedLaunch reports Mode A: launch stored scenario via index 590 only.
func IsMemorizedLaunch(params []protocol.ExchangeKV) bool {
	if len(params) != 1 {
		return false
	}
	p := params[0]
	if p.K != IndexTrigger {
		return false
	}
	n, err := strconv.Atoi(p.V)
	return err == nil && n >= 2 && n <= SlotCount
}

// LaunchParams returns inject params for launching a memorized scenario (Mode A).
// Slot 1 is server-reserved; use Mode B inject instead.
func LaunchParams(slot int) ([]protocol.ExchangeKV, error) {
	if slot < 2 || slot > SlotCount {
		return nil, fmt.Errorf("scenario: launch slot must be 2–8, got %d", slot)
	}
	return []protocol.ExchangeKV{{
		K: IndexTrigger,
		V: strconv.Itoa(slot),
	}}, nil
}
