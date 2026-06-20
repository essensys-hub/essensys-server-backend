package scenario

import (
	"fmt"
	"strconv"
)

// BitmaskOffsets are enumScenario fields encoded as 0–255 bitmasks (offsets within each slot).
var BitmaskOffsets = map[int]struct{}{
	13: {}, 14: {}, 15: {}, 16: {}, 17: {}, 18: {}, // éteindre PDV/CHB/PDE
	19: {}, 20: {}, 21: {}, 22: {}, 23: {}, 24: {}, // allumer PDV/CHB/PDE
	25: {}, 26: {}, 27: {}, 28: {}, 29: {},         // ouvrir volets
	30: {}, 31: {}, 32: {},                         // fermer volets
}

// ValidateDefinition checks params for a slot (1–8). Slot 1 editable only with admin flag in API layer.
func ValidateDefinition(slot int, params map[int]string) error {
	start, end, err := SlotRange(slot)
	if err != nil {
		return err
	}
	for k, v := range params {
		if k < start || k > end {
			return fmt.Errorf("scenario: index %d outside slot %d range [%d-%d]", k, slot, start, end)
		}
		if n, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("scenario: index %d value not numeric: %q", k, v)
		} else if n < 0 || n > 255 {
			return fmt.Errorf("scenario: index %d value out of byte range: %d", k, n)
		}
	}
	return nil
}
