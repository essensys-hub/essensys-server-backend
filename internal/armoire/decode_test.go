package armoire

import (
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
)

func TestDecodeStatus_Secouru(t *testing.T) {
	flags, ok := decodeStatusByte("4")
	if !ok {
		t.Fatal("expected decode ok")
	}
	if !flags.Secouru {
		t.Fatal("expected secouru true for k=10 value 4")
	}
	if flags.HeuresCreuses || flags.Delestage {
		t.Fatal("unexpected other flags")
	}
}

func TestDecodeEthernet_FaultBits(t *testing.T) {
	if got := decodeEthernetState("0"); got != "câble OK, IP OK, DNS OK, serveur joignable" {
		t.Fatalf("all OK: %q", got)
	}
	if got := decodeEthernetState("1"); got != "câble déconnecté" {
		t.Fatalf("cable fault: %q", got)
	}
	if got := decodeEthernetState("8"); got != "serveur HS" {
		t.Fatalf("server fault: %q", got)
	}
}

func TestDecodeHeatingZone_ConfortForce(t *testing.T) {
	consigne, mode := heatingModeLabel("17")
	if consigne != "CONFORT" {
		t.Fatalf("consigne=%s", consigne)
	}
	if mode != "forcé" {
		t.Fatalf("mode=%s", mode)
	}
}

func TestBuildSnapshot_SystemAndAlarm(t *testing.T) {
	store := data.NewMemoryStore()
	clientID := "dev1"
	store.SetValue(clientID, 10, "4")
	store.SetValue(clientID, 408, "4")
	store.SetValue(clientID, 413, "4")
	store.SetValue(clientID, 349, "17")
	store.RecordClientPoll(clientID, time.Now())

	snap := BuildSnapshot(store, SnapshotOptions{ClientID: clientID})
	if !snap.System.Secouru {
		t.Fatal("expected secouru")
	}
	if snap.Alarm.Step != "croisière" {
		t.Fatalf("alarm step=%s", snap.Alarm.Step)
	}
	zone, ok := snap.Comfort.Heating["zone_jour"]
	if !ok {
		t.Fatal("missing zone_jour")
	}
	if zone.Consigne != "CONFORT" || zone.Mode != "forcé" {
		t.Fatalf("zone=%+v", zone)
	}
	if !snap.Connected {
		t.Fatal("expected connected with recent poll")
	}
}

func TestBuildSnapshot_ExcludesAlarmCodes(t *testing.T) {
	for _, k := range SnapshotIndices {
		if k == 417 || k == 418 {
			t.Fatalf("snapshot must not include alarm user codes, found %d", k)
		}
	}
}

// NR: NR-server-backend-24 essensys-hub/essensys-server-backend#11
// security-outlets-2026-10-011: the security outlet state (440) is polled with
// the health indices and decoded in the snapshot alarm block.
func Test_NR_server_backend_24_etat_des_prises_440(t *testing.T) {
	for v, want := range map[string]string{"1": "cut", "0": "piloted", "": "unknown", "7": "unknown"} {
		if got := decodeSecurityOutlets(v); got != want {
			t.Fatalf("440=%q: got %q, want %q", v, got, want)
		}
	}
	polled := false
	for _, k := range HealthIndices {
		polled = polled || k == 440
	}
	if !polled {
		t.Fatal("440 must be part of the serverinfos health rotation")
	}

	store := data.NewMemoryStore()
	store.RecordClientPoll("dev1", time.Now())
	if got := BuildSnapshot(store, SnapshotOptions{ClientID: "dev1"}).Alarm.SecurityOutlets; got != "unknown" {
		t.Fatalf("440 never read: got %q, want unknown", got)
	}
	store.SetValue("dev1", 440, "1")
	if got := BuildSnapshot(store, SnapshotOptions{ClientID: "dev1"}).Alarm.SecurityOutlets; got != "cut" {
		t.Fatalf("440=1: got %q, want cut", got)
	}
}
