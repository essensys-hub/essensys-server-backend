package laniam

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

func TestParseIPNeighOutput(t *testing.T) {
	output := []byte("192.168.0.42 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE\n")
	candidates := parseIPNeighOutput(output)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].MacAddress != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("unexpected mac: %s", candidates[0].MacAddress)
	}
}

func TestNormalizeCandidateRejectsIncompleteMAC(t *testing.T) {
	_, ok := NormalizeCandidate(models.TrustedDeviceCandidate{
		MacAddress: "00:00:00:00:00:00",
		SourceIP:   "192.168.0.10",
	})
	if ok {
		t.Fatal("expected incomplete MAC to be rejected")
	}
}

func TestNormalizeCandidateAcceptsUnicastMAC(t *testing.T) {
	candidate, ok := NormalizeCandidate(models.TrustedDeviceCandidate{
		MacAddress: "aa:bb:cc:dd:ee:ff",
		SourceIP:   "192.168.0.42",
	})
	if !ok {
		t.Fatal("expected valid MAC")
	}
	if candidate.MacAddress != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("unexpected normalized mac: %s", candidate.MacAddress)
	}
}
