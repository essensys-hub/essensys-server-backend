package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/api"
	"github.com/essensys-hub/essensys-server-backend/internal/auth"
	"github.com/essensys-hub/essensys-server-backend/internal/cloudsync"
	"github.com/essensys-hub/essensys-server-backend/internal/config"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/metrics"
	"github.com/essensys-hub/essensys-server-backend/internal/mqtt"
	"github.com/essensys-hub/essensys-server-backend/internal/server"
	"github.com/essensys-hub/essensys-server-backend/internal/unifi"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func main() {
	configFile := flag.String("config", "", "Path to config file (default: config.yaml in working directory)")
	flag.Parse()

	// Load configuration
	var cfg *config.Config
	var err error
	if *configFile != "" {
		cfg, err = config.LoadFromFile(*configFile)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize Prometheus metrics
	metrics.Init("1.3.0")

	// Log configuration
	cfg.LogConfig()

	// Initialize store
	// Initialize store
	// store := data.NewMemoryStore()
	
	// Initialize Redis store
	store := data.NewRedisStore(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	log.Printf("Initialized Redis data store (%s)", cfg.Redis.Addr)

	// Initialize Database (for Archiver)
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User, cfg.Database.Password, cfg.Database.Name)
	db, err := sqlx.Connect(cfg.Database.Driver, connStr)
	if err != nil {
		log.Printf("WARNING: Failed to connect to database: %v. Archiver disabled.", err)
	} else {
		log.Println("Connected to database")
	}

	// Initialize services
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	log.Println("Initialized action and status services")
	
	// Initialize MQTT client (if enabled)
	var mqttClient *mqtt.Client
	if cfg.MQTT.Enabled {
		mqttClient = mqtt.NewClient(cfg.MQTT)
		if err := mqttClient.Connect(); err != nil {
			log.Printf("WARNING: MQTT connection failed: %v. MQTT features disabled.", err)
			mqttClient = nil
		} else {
			log.Println("Connected to MQTT broker")
			
			// Publish MQTT Discovery configs
			if err := mqttClient.PublishDiscoveryConfigs(); err != nil {
				log.Printf("WARNING: Failed to publish MQTT Discovery configs: %v", err)
			}
			
			// Subscribe to command topics
			entityMapping := mqttClient.GetEntityMapping()
			commandHandler := mqtt.NewCommandHandler(actionService, entityMapping)
			if err := mqttClient.SubscribeToCommands(commandHandler); err != nil {
				log.Printf("WARNING: Failed to subscribe to MQTT command topics: %v", err)
			}
			
			// Set up entity mapping for ActionService to publish states
			// Build reverse mapping: index -> entity info
			indexToEntity := make(map[int]struct {
				EntityType string
				EntityID   string
			})
			for key, cmd := range entityMapping {
				parts := strings.Split(key, "/")
				if len(parts) == 2 {
					indexToEntity[cmd.Index] = struct {
						EntityType string
						EntityID   string
					}{
						EntityType: parts[0],
						EntityID:   parts[1],
					}
				}
			}
			actionService.SetMQTTPublisher(mqttClient, indexToEntity)
		}
	}
	
	// Initialize Archiver
	if db != nil {
		archiver := core.NewArchiverService(store, db)
		archiver.Start(10 * time.Minute)
		log.Println("Initialized Archiver Service (10m interval)")
	}

	// Initialize shared exchange pull scheduler (LAN manual sync + cloud scheduled sync).
	pullScheduler := core.NewExchangePullScheduler()

	cloudAgent := cloudsync.NewAgent(cloudsync.Config{
		Enabled:              cfg.Cloud.Enabled,
		HubURL:               cfg.Cloud.HubURL,
		GatewayID:            cfg.Cloud.GatewayID,
		GatewayToken:         cfg.Cloud.GatewayToken,
		PollIntervalSeconds:  cfg.Cloud.PollIntervalSeconds,
		ClientID:             cfg.Cloud.ClientID,
		Eth0MAC:              cfg.Cloud.Eth0MAC,
		Eth1MAC:              cfg.Cloud.Eth1MAC,
		ScheduledSyncEnabled: cfg.Cloud.ScheduledSyncEnabled,
	}, actionService, store, pullScheduler)

	// Initialize handler
	handler := api.NewHandler(actionService, statusService, store, pullScheduler, cloudAgent)

	// Cloud hub sync (optional — outbound HTTPS to mon.essensys.fr)
	cloudCtx, cloudCancel := context.WithCancel(context.Background())
	cloudAgent.Start(cloudCtx)

    // Initialize SessionStore
    sessionStore := auth.NewSessionStore()

    // Initialize web handler (if db is connected)
    var webHandler *api.WebHandler
    if db != nil {
        webHandler = api.NewWebHandler(db, sessionStore, actionService, store)
        log.Println("Initialized Web Handler (Authentication enabled)")
    } else {
        log.Println("WARNING: Database not connected. Web Handler (Auth) disabled.")
    }

	// Initialize UniFi Protect client (if enabled)
	var unifiHandler *api.UniFiHandler
	if cfg.UniFi.Enabled {
		if cfg.UniFi.APIKey == "" {
			log.Printf("WARNING: UniFi Protect enabled but API key not configured. UniFi features disabled.")
			unifiHandler = nil
		} else {
			unifiClient := unifi.NewClient(cfg.UniFi)
			unifiHandler = api.NewUniFiHandler(unifiClient)
			log.Println("Initialized UniFi Protect Handler (using API key)")
		}
	}

	// Setup router with middleware chain
	router := api.NewRouter(handler, webHandler, unifiHandler, cfg.Auth.Clients, cfg.Auth.Enabled, store)
	if cfg.Auth.Enabled {
		log.Println("Configured HTTP router with middleware chain (Recovery → Logging → CaptureAuth)")
	} else {
		log.Println("Configured HTTP router with middleware chain (Recovery → Logging) - Authentication DISABLED")
	}

	// Configure server address
	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	// Log server startup
	log.Printf("Server starting on %s", addr)
	log.Printf("Health check available at: http://localhost%s/health", addr)
	log.Printf("===========================================")

	// Channel to listen for errors from the server
	serverErrors := make(chan error, 1)

	// Create a TCP listener with logging
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to create listener: %v", err)
	}

	// Wrap with logging listener to see all TCP connections
	loggingListener := server.NewLoggingListener(listener)

	// Create legacy HTTP server that tolerates non-standard HTTP from BP_MQX_ETH clients
	legacyServer := server.NewLegacyHTTPServer(router)

	// Start server in a goroutine
	go func() {
		log.Println("HTTP server listening (legacy-compatible mode)...")
		log.Printf("Waiting for connections on %s...", addr)
		log.Println("NOTE: Server configured to accept non-standard HTTP from BP_MQX_ETH clients")
		log.Println("  - Tolerates trailing spaces in request line")
		log.Println("  - Accepts HTTP/1.0 and HTTP/1.1")
		serverErrors <- legacyServer.Serve(loggingListener)
	}()

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive a signal or an error
	select {
	case err := <-serverErrors:
		log.Fatalf("Server error: %v", err)

	case sig := <-shutdown:
		log.Printf("Received shutdown signal: %v", sig)
		log.Println("Starting graceful shutdown...")
		cloudCancel()

		// Close the listener to stop accepting new connections
		if err := listener.Close(); err != nil {
			log.Printf("Error closing listener: %v", err)
		}

		// Disconnect MQTT client
		if mqttClient != nil {
			mqttClient.Disconnect()
		}

		// Give existing connections time to finish
		time.Sleep(2 * time.Second)

		log.Println("Server stopped gracefully")
	}
}
