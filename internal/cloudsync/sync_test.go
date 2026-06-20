package cloudsync

import "testing"

func TestValidateHubURL_requiresHTTPS(t *testing.T) {
	a := &Agent{cfg: Config{
		Enabled:      true,
		HubURL:       "http://mon.essensys.fr",
		GatewayID:    "gw-1",
		GatewayToken: "secret",
	}}
	if err := a.validateHubURL(); err == nil {
		t.Fatal("expected error for http hub url")
	}
}

func TestValidateHubURL_acceptsHTTPS(t *testing.T) {
	a := &Agent{cfg: Config{
		Enabled:      true,
		HubURL:       "https://mon.essensys.fr",
		GatewayID:    "gw-1",
		GatewayToken: "secret",
	}}
	if err := a.validateHubURL(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestExchangePushIndices_includesShutterTimes(t *testing.T) {
	indices := exchangePushIndices()
	want := map[int]bool{
		590: true, 566: true, 572: true, 574: true, 578: true, 582: true, 585: true,
		605: true, 622: true,
		13: true, 348: true, 349: true, 352: true,
	}
	for k := range want {
		found := false
		for _, i := range indices {
			if i == k {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing index %d in %v", k, indices)
		}
	}
	if len(indices) != 375 {
		t.Fatalf("expected 375 indices, got %d", len(indices))
	}
}
