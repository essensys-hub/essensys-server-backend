package core

import (
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
)

const DefaultArmoireOfflineThreshold = 6 * time.Second

// IsClientConnectedByPoll reports whether the client posted mystatus within threshold.
func IsClientConnectedByPoll(store data.Store, clientID string, threshold time.Duration) bool {
	if threshold <= 0 {
		threshold = DefaultArmoireOfflineThreshold
	}
	lastPoll, ok := store.GetClientLastPoll(clientID)
	if !ok {
		return false
	}
	return time.Since(lastPoll) <= threshold
}
