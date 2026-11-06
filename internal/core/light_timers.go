package core

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// LightTimer is one light auto-off timer cell of the exchange table
// (lumieres-temporisees-2026-11-003): value in minutes, 0..255, 0 = never.
type LightTimer struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Key   int    `json:"key"`
}

const (
	// LightTimerMinMinutes is the smallest value (0 = the light is never switched off).
	LightTimerMinMinutes = 0
	// LightTimerMaxMinutes is the largest value (the cell is an unsigned byte).
	LightTimerMaxMinutes = 255
)

var lightTimers = []LightTimer{
	{ID: "dressing", Label: "Dressing", Key: protocol.IndexLightTimerDressing},
	{ID: "wc1", Label: "WC 1", Key: protocol.IndexLightTimerWC1},
	{ID: "wc2", Label: "WC 2", Key: protocol.IndexLightTimerWC2},
	{ID: "pieceserv", Label: "Service", Key: protocol.IndexLightTimerPieceServ},
	{ID: "ann1", Label: "Annexe 1", Key: protocol.IndexLightTimerAnnexe1},
	{ID: "ann2", Label: "Annexe 2", Key: protocol.IndexLightTimerAnnexe2},
}

// LightTimers returns a copy of the catalogue, in display order.
func LightTimers() []LightTimer {
	out := make([]LightTimer, len(lightTimers))
	copy(out, lightTimers)
	return out
}

// LightTimerByID looks a light up by its identifier.
func LightTimerByID(id string) (LightTimer, bool) {
	for _, t := range lightTimers {
		if t.ID == id {
			return t, true
		}
	}
	return LightTimer{}, false
}

// ValidateLightTimerMinutes rejects values outside 0..255.
func ValidateLightTimerMinutes(minutes int) error {
	if minutes < LightTimerMinMinutes || minutes > LightTimerMaxMinutes {
		return fmt.Errorf("minutes must be between %d and %d", LightTimerMinMinutes, LightTimerMaxMinutes)
	}
	return nil
}

// LightTimerParam builds the single exchange write for a light (decimal string value).
func LightTimerParam(id string, minutes int) (protocol.ExchangeKV, error) {
	t, ok := LightTimerByID(id)
	if !ok {
		return protocol.ExchangeKV{}, fmt.Errorf("unknown light %q", id)
	}
	if err := ValidateLightTimerMinutes(minutes); err != nil {
		return protocol.ExchangeKV{}, err
	}
	return protocol.ExchangeKV{K: t.Key, V: fmt.Sprintf("%d", minutes)}, nil
}

// ParseLightTimerValue decodes a stored cell. An empty, non numeric or out of
// range value is unknown (nil): a key never read must never show as 0.
func ParseLightTimerValue(v string) *int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || ValidateLightTimerMinutes(n) != nil {
		return nil
	}
	return &n
}
