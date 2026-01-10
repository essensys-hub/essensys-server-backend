package main

import (
	"fmt"
	"log"
	"os"

	"github.com/essensys-hub/essensys-server-backend/internal/config"
	"gopkg.in/yaml.v3"
)

func main() {
	// Test direct du parsing YAML
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("Error reading config.yaml: %v", err)
	}

	var testCfg struct {
		Database struct {
			Enabled  bool   `yaml:"enabled"`
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			DBName   string `yaml:"dbname"`
			SSLMode  string `yaml:"sslmode"`
		} `yaml:"database"`
	}

	if err := yaml.Unmarshal(data, &testCfg); err != nil {
		log.Fatalf("Error parsing YAML: %v", err)
	}

	fmt.Printf("Direct YAML parsing:\n")
	fmt.Printf("  Enabled: %v\n", testCfg.Database.Enabled)
	fmt.Printf("  Host: %s\n", testCfg.Database.Host)
	fmt.Printf("  Port: %d\n", testCfg.Database.Port)
	fmt.Printf("  User: %s\n", testCfg.Database.User)
	fmt.Printf("  DBName: '%s'\n", testCfg.Database.DBName)
	fmt.Printf("  SSLMode: %s\n", testCfg.Database.SSLMode)
	fmt.Printf("\n")

	// Test avec la fonction Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	fmt.Printf("Config.Load() result:\n")
	fmt.Printf("  Enabled: %v\n", cfg.Database.Enabled)
	fmt.Printf("  Host: %s\n", cfg.Database.Host)
	fmt.Printf("  Port: %d\n", cfg.Database.Port)
	fmt.Printf("  User: %s\n", cfg.Database.User)
	fmt.Printf("  DBName: '%s'\n", cfg.Database.DBName)
	fmt.Printf("  SSLMode: %s\n", cfg.Database.SSLMode)
}


