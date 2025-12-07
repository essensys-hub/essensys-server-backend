package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the server
type Config struct {
	Server ServerConfig `yaml:"server"`
	Auth   AuthConfig   `yaml:"auth"`
	Logging LoggingConfig `yaml:"logging"`
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Enabled bool              `yaml:"enabled"`
	Clients map[string]string `yaml:"clients"` // matricule -> key
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Load loads configuration from environment variables and optionally a YAML file
// Environment variables take precedence over YAML file values
func Load() (*Config, error) {
	// Start with default configuration
	cfg := &Config{
		Server: ServerConfig{
			Port:         80, // MANDATORY for BP_MQX_ETH client compatibility
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Auth: AuthConfig{
			Enabled: false, // Disabled by default
			Clients: make(map[string]string),
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	// Try to load from config.yaml if it exists
	if err := loadFromYAML(cfg, "config.yaml"); err != nil {
		// Log but don't fail if config file doesn't exist
		if !os.IsNotExist(err) {
			log.Printf("Warning: error loading config.yaml: %v", err)
		}
	}

	// Override with environment variables
	loadFromEnv(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// loadFromYAML loads configuration from a YAML file
func loadFromYAML(cfg *Config, filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	return nil
}

// loadFromEnv loads configuration from environment variables
// Environment variables take precedence over YAML values
func loadFromEnv(cfg *Config) {
	// SERVER_PORT
	if portStr := os.Getenv("SERVER_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			cfg.Server.Port = port
		} else {
			log.Printf("Warning: invalid SERVER_PORT value '%s', using default", portStr)
		}
	}

	// LOG_LEVEL
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		cfg.Logging.Level = logLevel
	}

	// AUTH_ENABLED
	if authEnabledStr := os.Getenv("AUTH_ENABLED"); authEnabledStr != "" {
		if authEnabled, err := strconv.ParseBool(authEnabledStr); err == nil {
			cfg.Auth.Enabled = authEnabled
		} else {
			log.Printf("Warning: invalid AUTH_ENABLED value '%s', using default", authEnabledStr)
		}
	}

	// CLIENT_CREDENTIALS (format: "client1:pass1,client2:pass2")
	if clientCreds := os.Getenv("CLIENT_CREDENTIALS"); clientCreds != "" {
		clients := parseClientCredentials(clientCreds)
		if len(clients) > 0 {
			cfg.Auth.Clients = clients
		}
	}
}

// parseClientCredentials parses the CLIENT_CREDENTIALS environment variable
// Format: "client1:pass1,client2:pass2"
func parseClientCredentials(creds string) map[string]string {
	clients := make(map[string]string)
	
	pairs := strings.Split(creds, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			matricule := strings.TrimSpace(parts[0])
			key := strings.TrimSpace(parts[1])
			if matricule != "" && key != "" {
				clients[matricule] = key
			}
		} else {
			log.Printf("Warning: invalid client credential format '%s', expected 'matricule:key'", pair)
		}
	}
	
	return clients
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate port
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be between 1 and 65535)", c.Server.Port)
	}

	// Warn if not using port 80
	if c.Server.Port != 80 {
		log.Printf("WARNING: Server configured to use port %d instead of 80", c.Server.Port)
		log.Printf("WARNING: BP_MQX_ETH clients are hardcoded to connect to port 80")
		log.Printf("WARNING: This configuration may not work with legacy clients")
	}

	// Validate timeouts
	if c.Server.ReadTimeout <= 0 {
		return fmt.Errorf("invalid read timeout: %v (must be positive)", c.Server.ReadTimeout)
	}
	if c.Server.WriteTimeout <= 0 {
		return fmt.Errorf("invalid write timeout: %v (must be positive)", c.Server.WriteTimeout)
	}
	if c.Server.IdleTimeout <= 0 {
		return fmt.Errorf("invalid idle timeout: %v (must be positive)", c.Server.IdleTimeout)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}

	// Validate authentication
	if c.Auth.Enabled {
		if len(c.Auth.Clients) == 0 {
			log.Printf("WARNING: Authentication is enabled but no client credentials are configured")
			log.Printf("WARNING: All requests will be rejected with 401 Unauthorized")
		}
	} else {
		log.Printf("INFO: Authentication is disabled - all requests will be accepted without credentials")
	}

	return nil
}

// LogConfig logs the current configuration (without sensitive data)
func (c *Config) LogConfig() {
	log.Printf("===========================================")
	log.Printf("Essensys Backend Server Configuration")
	log.Printf("===========================================")
	log.Printf("Server:")
	log.Printf("  Port: %d %s", c.Server.Port, c.portWarning())
	log.Printf("  Read Timeout: %v", c.Server.ReadTimeout)
	log.Printf("  Write Timeout: %v", c.Server.WriteTimeout)
	log.Printf("  Idle Timeout: %v", c.Server.IdleTimeout)
	log.Printf("Authentication:")
	log.Printf("  Enabled: %v", c.Auth.Enabled)
	log.Printf("  Configured Clients: %d", len(c.Auth.Clients))
	log.Printf("Logging:")
	log.Printf("  Level: %s", c.Logging.Level)
	log.Printf("  Format: %s", c.Logging.Format)
	log.Printf("===========================================")
}

// portWarning returns a warning message if not using port 80
func (c *Config) portWarning() string {
	if c.Server.Port != 80 {
		return "(WARNING: BP_MQX_ETH clients require port 80)"
	}
	return "(MANDATORY for BP_MQX_ETH clients)"
}
