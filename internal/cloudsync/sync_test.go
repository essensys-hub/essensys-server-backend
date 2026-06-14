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
