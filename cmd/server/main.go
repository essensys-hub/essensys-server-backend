package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/api"
	"github.com/essensys-hub/essensys-server-backend/internal/config"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/server"
)

func main() {
	// Load configuration from environment variables and config.yaml
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Log configuration
	cfg.LogConfig()

	// Initialize store
	store := data.NewMemoryStore()
	log.Println("Initialized in-memory data store")

	// Initialize services
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	log.Println("Initialized action and status services")

	// Initialize handler
	handler := api.NewHandler(actionService, statusService, store)

	// Setup router with middleware chain
	router := api.NewRouter(handler, cfg.Auth.Clients, cfg.Auth.Enabled)
	if cfg.Auth.Enabled {
		log.Println("Configured HTTP router with middleware chain (Recovery → Logging → BasicAuth)")
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

		// Close the listener to stop accepting new connections
		if err := listener.Close(); err != nil {
			log.Printf("Error closing listener: %v", err)
		}

		// Give existing connections time to finish
		time.Sleep(2 * time.Second)

		log.Println("Server stopped gracefully")
	}
}
