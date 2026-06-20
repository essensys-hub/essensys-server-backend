package scenario

import (
	"sort"

	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// ExpandModeB fills 590=1 and missing indices in range with "0".
// full=false: 605–622 (legacy light/shutter block).
// full=true: 592–632 (complete Scenario1 parameters).
func ExpandModeB(params []protocol.ExchangeKV, full bool) []protocol.ExchangeKV {
	start, end := protocol.IndexLightStart, protocol.IndexLightEnd
	if full {
		start, end = IndexFullBlockStart, IndexFullBlockEnd
	}

	touchesRange := false
	for _, p := range params {
		if p.K >= start && p.K <= end {
			touchesRange = true
			break
		}
	}
	if !touchesRange {
		return params
	}

	byIndex := make(map[int]string, end-start+2)
	for _, p := range params {
		byIndex[p.K] = p.V
	}
	if _, ok := byIndex[IndexTrigger]; !ok {
		byIndex[IndexTrigger] = "1"
	}
	for i := start; i <= end; i++ {
		if _, ok := byIndex[i]; !ok {
			byIndex[i] = "0"
		}
	}

	out := make([]protocol.ExchangeKV, 0, len(byIndex))
	for k, v := range byIndex {
		out = append(out, protocol.ExchangeKV{K: k, V: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out
}
