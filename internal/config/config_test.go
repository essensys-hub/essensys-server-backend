package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any environment variables
	os.Clearenv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check default values
	if cfg.Server.Port != 80 {
		t.Errorf("Expected default port 80, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 10*time.Second {
		t.Errorf("Expected default read timeout 10s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 10*time.Second {
		t.Errorf("Expected default write timeout 10s, got %v", cfg.Server.WriteTimeout)
	}
	if cfg.Server.IdleTimeout != 60*time.Second {
		t.Errorf("Expected default idle timeout 60s, got %v", cfg.Server.IdleTimeout)
	}
	if cfg.Auth.Enabled {
		t.Error("Expected auth to be disabled by default")
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default log level 'info', got '%s'", cfg.Logging.Level)
	}
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	// Set environment variables
	os.Setenv("SERVER_PORT", "8080")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("AUTH_ENABLED", "true")
	os.Setenv("CLIENT_CREDENTIALS", "client1:pass1,client2:pass2")
	defer os.Clearenv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check environment variable overrides
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080 from env, got %d", cfg.Server.Port)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug' from env, got '%s'", cfg.Logging.Level)
	}
	if !cfg.Auth.Enabled {
		t.Error("Expected auth to be enabled from env")
	}
	if len(cfg.Auth.Clients) != 2 {
		t.Errorf("Expected 2 clients from env, got %d", len(cfg.Auth.Clients))
	}
	if cfg.Auth.Clients["client1"] != "pass1" {
		t.Errorf("Expected client1:pass1, got client1:%s", cfg.Auth.Clients["client1"])
	}
	if cfg.Auth.Clients["client2"] != "pass2" {
		t.Errorf("Expected client2:pass2, got client2:%s", cfg.Auth.Clients["client2"])
	}
}

func TestParseClientCredentials(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "single client",
			input: "client1:pass1",
			expected: map[string]string{
				"client1": "pass1",
			},
		},
		{
			name:  "multiple clients",
			input: "client1:pass1,client2:pass2,client3:pass3",
			expected: map[string]string{
				"client1": "pass1",
				"client2": "pass2",
				"client3": "pass3",
			},
		},
		{
			name:  "with whitespace",
			input: " client1 : pass1 , client2 : pass2 ",
			expected: map[string]string{
				"client1": "pass1",
				"client2": "pass2",
			},
		},
		{
			name:     "invalid format - no colon",
			input:    "client1pass1",
			expected: map[string]string{},
		},
		{
			name:  "mixed valid and invalid",
			input: "client1:pass1,invalid,client2:pass2",
			expected: map[string]string{
				"client1": "pass1",
				"client2": "pass2",
			},
		},
		{
			name:     "empty string",
			input:    "",
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseClientCredentials(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d clients, got %d", len(tt.expected), len(result))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("Expected %s:%s, got %s:%s", k, v, k, result[k])
				}
			}
		})
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         0,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Auth: AuthConfig{
			Enabled: true,
			Clients: map[string]string{"test": "pass"},
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid port, got nil")
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         80,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Auth: AuthConfig{
			Enabled: true,
			Clients: map[string]string{"test": "pass"},
		},
		Logging: LoggingConfig{
			Level: "invalid",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid log level, got nil")
	}
}

func TestValidate_InvalidTimeout(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         80,
			ReadTimeout:  -1 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Auth: AuthConfig{
			Enabled: true,
			Clients: map[string]string{"test": "pass"},
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid timeout, got nil")
	}
}

func TestLoadFromYAML(t *testing.T) {
	// Create a temporary YAML file
	yamlContent := `
server:
  port: 8080
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 120s

auth:
  enabled: true
  clients:
    testclient: testpass
    client2: pass2

logging:
  level: debug
  format: json
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	cfg := &Config{
		Auth: AuthConfig{
			Clients: make(map[string]string),
		},
	}
	err = loadFromYAML(cfg, tmpfile.Name())
	if err != nil {
		t.Fatalf("loadFromYAML() failed: %v", err)
	}

	// Check loaded values
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080 from YAML, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Errorf("Expected read timeout 15s from YAML, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug' from YAML, got '%s'", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Expected log format 'json' from YAML, got '%s'", cfg.Logging.Format)
	}
	if len(cfg.Auth.Clients) != 2 {
		t.Errorf("Expected 2 clients from YAML, got %d", len(cfg.Auth.Clients))
	}
}
