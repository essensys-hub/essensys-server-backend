package armoire

import (
	"fmt"
	"strings"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
)

// Snapshot is the JSON payload for GET /api/admin/armoire/snapshot.
type Snapshot struct {
	Connected    bool          `json:"connected"`
	LastPollAt   string        `json:"last_poll_at,omitempty"`
	ClientID     string        `json:"client_id"`
	StaleSeconds *int          `json:"stale_seconds,omitempty"`
	Identity     IdentityBlock `json:"identity"`
	System       SystemBlock   `json:"system"`
	Alarm        AlarmBlock    `json:"alarm"`
	Comfort      ComfortBlock  `json:"comfort"`
	Energy       EnergyBlock   `json:"energy"`
	RawMissing   []int         `json:"raw_missing"`
}

type IdentityBlock struct {
	FirmwareEmbedded string `json:"firmware_embedded,omitempty"`
	FirmwareWeb      string `json:"firmware_web,omitempty"`
	RTC              string `json:"rtc,omitempty"`
	Ethernet         string `json:"ethernet,omitempty"`
	MAC              string `json:"mac,omitempty"`
}

type SystemBlock struct {
	HeuresCreuses bool   `json:"heures_creuses"`
	Delestage     bool   `json:"delestage"`
	Secouru       bool   `json:"secouru"`
	CommFaults    string `json:"comm_faults,omitempty"`
}

type AlarmBlock struct {
	Mode       string `json:"mode,omitempty"`
	Step       string `json:"step,omitempty"`
	Armed      bool   `json:"armed"`
	Triggered  bool   `json:"triggered"`
	WaterLeak  bool   `json:"water_leak"`
	Detection  string `json:"detection,omitempty"`
	Fraud      string `json:"fraud,omitempty"`
	BPState    string `json:"bp_state,omitempty"`
}

type HeatingZone struct {
	Consigne string `json:"consigne"`
	Mode     string `json:"mode"`
}

type ComfortBlock struct {
	Heating   map[string]HeatingZone `json:"heating,omitempty"`
	Cumulus   string                 `json:"cumulus,omitempty"`
	Sprinkler string                 `json:"sprinkler,omitempty"`
	Scenario  string                 `json:"scenario,omitempty"`
}

type EnergyBlock struct {
	Tariff          string `json:"tariff,omitempty"`
	Period          string `json:"period,omitempty"`
	ApparentPowerVA *int   `json:"apparent_power_va,omitempty"`
	WindSpeedKmh    *int   `json:"wind_speed_kmh,omitempty"`
}

// SnapshotOptions configures snapshot assembly.
type SnapshotOptions struct {
	ClientID         string
	OfflineThreshold time.Duration
}

var heatingZoneKeys = []struct {
	key   string
	index string
}{
	{"zone_jour", "349"},
	{"zone_nuit", "350"},
	{"zone_sdb1", "351"},
	{"zone_sdb2", "352"},
}

// BuildSnapshot reads exchange values and decodes dashboard fields.
func BuildSnapshot(store data.Store, opts SnapshotOptions) Snapshot {
	clientID := opts.ClientID
	if clientID == "" {
		if id, ok := store.GetLastPolledClientID(); ok {
			clientID = id
		} else {
			clientID = "default"
		}
	}

	threshold := opts.OfflineThreshold
	if threshold <= 0 {
		threshold = core.DefaultArmoireOfflineThreshold
	}

	now := time.Now()
	lastPoll, hasPoll := store.GetClientLastPoll(clientID)
	connected := core.IsClientConnectedByPoll(store, clientID, threshold)

	snap := Snapshot{
		ClientID:   clientID,
		Connected:  connected,
		RawMissing: []int{},
	}
	if hasPoll {
		snap.LastPollAt = lastPoll.UTC().Format(time.RFC3339)
		if !connected {
			sec := int(now.Sub(lastPoll).Seconds())
			snap.StaleSeconds = &sec
		}
	}

	vals := make(map[string]string, len(SnapshotIndices))
	for _, k := range SnapshotIndices {
		key := fmt.Sprintf("%d", k)
		if v, ok := store.GetValue(clientID, k); ok && strings.TrimSpace(v) != "" {
			vals[key] = v
		} else {
			snap.RawMissing = append(snap.RawMissing, k)
		}
	}

	snap.Identity = decodeIdentity(vals)
	snap.System = decodeSystem(vals)
	snap.Alarm = decodeAlarm(vals)
	snap.Comfort = decodeComfort(vals)
	snap.Energy = decodeEnergy(vals)
	return snap
}

func decodeIdentity(vals map[string]string) IdentityBlock {
	var b IdentityBlock
	if v, ok := vals["0"]; ok {
		b.FirmwareEmbedded = strings.TrimSpace(v)
	}
	if v, ok := vals["1"]; ok {
		b.FirmwareWeb = strings.TrimSpace(v)
	}
	if mins, okM := vals["5"]; okM {
		if h, okH := vals["6"]; okH {
			if j, okJ := vals["7"]; okJ {
				if mo, okMo := vals["8"]; okMo {
					if y, okY := vals["9"]; okY {
						b.RTC = fmt.Sprintf("%s/%s/%s %s:%s", j, mo, y, h, mins)
					}
				}
			}
		}
	}
	if v, ok := vals["945"]; ok {
		b.Ethernet = decodeEthernetState(v)
	}
	b.MAC = formatMAC(vals["947"], vals["948"], vals["949"], vals["950"], vals["951"], vals["952"])
	return b
}

func decodeSystem(vals map[string]string) SystemBlock {
	var b SystemBlock
	if v, ok := vals["10"]; ok {
		if flags, ok := decodeStatusByte(v); ok {
			b.HeuresCreuses = flags.HeuresCreuses
			b.Delestage = flags.Delestage
			b.Secouru = flags.Secouru
		}
	}
	if v, ok := vals["12"]; ok {
		if faults := decodeInformationFaults(v); faults != "" {
			b.CommFaults = faults
		}
	}
	return b
}

func decodeAlarm(vals map[string]string) AlarmBlock {
	var b AlarmBlock
	if v, ok := vals["408"]; ok {
		b.Mode = alarmModeLabel(v)
		b.Armed = v != "0" && v != ""
	}
	if v, ok := vals["413"]; ok {
		b.Step = alarmStepLabel(v)
	}
	if v, ok := vals["414"]; ok {
		b.Detection = decodeAlarmDetection(v)
	}
	if v, ok := vals["415"]; ok {
		b.Fraud = decodeAlarmFraud(v)
	}
	if v, ok := vals["11"]; ok {
		if flags, ok := decodeAlerteByte(v); ok {
			b.Triggered = flags.Alarme
			b.WaterLeak = flags.FuiteLL || flags.FuiteLV
		}
	}
	if v, ok := vals["920"]; ok {
		b.BPState = decodeBPState(v)
	}
	return b
}

func decodeComfort(vals map[string]string) ComfortBlock {
	var b ComfortBlock
	heating := make(map[string]HeatingZone)
	for _, z := range heatingZoneKeys {
		if v, ok := vals[z.index]; ok {
			consigne, mode := heatingModeLabel(v)
			heating[z.key] = HeatingZone{Consigne: consigne, Mode: mode}
		}
	}
	if len(heating) > 0 {
		b.Heating = heating
	}
	if v, ok := vals["353"]; ok {
		b.Cumulus = cumulusLabel(v)
	}
	if v, ok := vals["363"]; ok {
		b.Sprinkler = sprinklerLabel(v)
	}
	if v, ok := vals["591"]; ok {
		b.Scenario = scenarioLabel(v)
	}
	return b
}

func decodeEnergy(vals map[string]string) EnergyBlock {
	var b EnergyBlock
	if v, ok := vals["460"]; ok {
		b.Tariff = linkyTariffLabel(v)
	}
	if v, ok := vals["461"]; ok {
		b.Period = linkyPeriodLabel(v)
	}
	b.ApparentPowerVA = apparentPowerVA(vals["463"], vals["464"])
	if v, ok := vals["940"]; ok {
		if n, err := parseIntDecimal(v); err == nil {
			b.WindSpeedKmh = &n
		}
	}
	return b
}
