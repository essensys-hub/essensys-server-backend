package armoire

import "sync"

// Rotator cycles serverinfos index lists when dashboard pull is enabled.
type Rotator struct {
	mu      sync.Mutex
	enabled bool
	slot    int
	slots   [][]int
}

// NewRotator builds a rotator with default + dashboard groups.
func NewRotator(enabled bool) *Rotator {
	return &Rotator{
		enabled: enabled,
		slots: [][]int{
			DefaultCommandIndices,
			IdentityIndices,
			HealthIndices,
			ComfortEnergyIndices,
		},
	}
}

func (r *Rotator) Enabled() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enabled
}

func (r *Rotator) SetEnabled(enabled bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
}

// Next returns the indices for the current poll and advances the rotation.
func (r *Rotator) Next() []int {
	if r == nil || !r.Enabled() {
		out := make([]int, len(DefaultCommandIndices))
		copy(out, DefaultCommandIndices)
		return out
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.slots) == 0 {
		return nil
	}
	indices := r.slots[r.slot%len(r.slots)]
	r.slot++
	out := make([]int, len(indices))
	copy(out, indices)
	return out
}
