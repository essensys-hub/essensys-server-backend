package audit

// AuditableExchangeIndices lists table d'échange indices that may produce STATE_CHANGE
// audit events (OpenSpec 2026-07-034 D8). Validate against SC944D TableEchange.h before extending.
var AuditableExchangeIndices = map[int]struct{}{
	590: {}, // scenario trigger
	409: {}, // alarm state
	410: {}, // alarm control
	411: {}, // alarm
}

func init() {
	for i := 605; i <= 622; i++ {
		AuditableExchangeIndices[i] = struct{}{}
	}
}

// IsAuditableExchangeIndex reports whether a mystatus/exchange index should emit STATE_CHANGE.
func IsAuditableExchangeIndex(k int) bool {
	_, ok := AuditableExchangeIndices[k]
	return ok
}
