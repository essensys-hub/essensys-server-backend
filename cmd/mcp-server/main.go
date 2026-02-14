package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Version variables - set at build time via ldflags
var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// Redis Client
var rdb *redis.Client

// responseLogger wraps http.ResponseWriter to capture status code
type responseLogger struct {
	http.ResponseWriter
	statusCode int
}

func (rl *responseLogger) WriteHeader(code int) {
	rl.statusCode = code
	rl.ResponseWriter.WriteHeader(code)
}

// IP Whitelist Middleware
func privateIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// If we can't parse remote addr, assume it's just IP
			host = r.RemoteAddr
		}

		// Handle X-Forwarded-For if behind a proxy (like Caddy/Nginx)
		// WARNING: Only trust this if the proxy is trusted. 
		// For stricter security, configure the proxy to deny external access to this path.
		// However, the requirement is "impossible to access from other subnet than private".
		// We will validate the direct connection or the forwarded IP.
		
		forwarded := r.Header.Get("X-Forwarded-For")
		if forwarded != "" {
			// Get the first IP in the list
			parts := strings.Split(forwarded, ",")
			host = strings.TrimSpace(parts[0])
		}

		if !isPrivateIP(host) {
			log.Printf("Blocked access from non-private IP: %s", host)
			http.Error(w, "Forbidden: Access restricted to private subnets.", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false // Invalid IP
	}

	// 127.0.0.0/8
	if ip.IsLoopback() {
		return true
	}

	// 10.0.0.0/8
	// 172.16.0.0/12
	// 192.168.0.0/16
	privateBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, block := range privateBlocks {
		_, subnet, _ := net.ParseCIDR(block)
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

// Auth Middleware
func authMiddleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: Missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Expect "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] != token {
			// Also check query param "token" for simple SSE test clients if needed, 
			// but standard is Header. For stricter security, only Header.
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	port := flag.String("port", "8080", "Port to listen on for SSE (default: 8080)")
	mode := flag.String("mode", "sse", "Mode: 'stdio' or 'sse' (default: sse)")
	token := flag.String("token", "", "Security token for SSE access (required in sse mode)")
	flag.Parse()

    if *mode == "sse" && *token == "" {
        // Warning or Fatal? Let's Log fatal to force security.
        log.Fatal("Error: -token is required in SSE mode for security.")
    }

	// Initialize Redis connection
	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// Create MCP Server
	s := server.NewMCPServer(
		"Essensys MCP",
		"1.0.0",
		server.WithLogging(),
	)

	// Tool: read_exchange_table
	s.AddTool(mcp.NewTool("read_exchange_table",
		mcp.WithDescription("Read all values from the exchange table. Returns a map of index to value."),
		mcp.WithString("client_id", mcp.Description("Client ID to read from (default: 'default')")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		clientID := "default"
		// Handle Arguments which can be map[string]interface{} or other types
		if args, ok := request.Params.Arguments.(map[string]interface{}); ok {
			if cid, ok := args["client_id"].(string); ok && cid != "" {
				clientID = cid
			}
		}

		log.Printf("[MCP TOOL] read_exchange_table called with client_id=%s", clientID)
		
		key := fmt.Sprintf("essensys:client:%s:exchange", clientID)
		log.Printf("[MCP TOOL] Reading Redis key: %s", key)
		
		vals, err := rdb.HGetAll(ctx, key).Result()
		if err != nil {
			log.Printf("[MCP TOOL] Redis error reading %s: %v", key, err)
			return mcp.NewToolResultError(fmt.Sprintf("Redis error: %v", err)), nil
		}

		log.Printf("[MCP TOOL] Retrieved %d values from exchange table for client %s", len(vals), clientID)
		jsonBytes, _ := json.Marshal(vals)
		result := string(jsonBytes)
		log.Printf("[MCP TOOL] read_exchange_table result: %s", result)
		return mcp.NewToolResultText(result), nil
	})

	// Tool: read_exchange_value
	s.AddTool(mcp.NewTool("read_exchange_value",
		mcp.WithDescription("Read a specific value from the exchange table by index."),
		mcp.WithString("client_id", mcp.Description("Client ID (default: 'default')")),
		mcp.WithNumber("index", mcp.Required(), mcp.Description("Index to read")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		clientID := "default"
		// Handle Arguments which can be map[string]interface{} or other types
		var args map[string]interface{}
		switch v := request.Params.Arguments.(type) {
		case map[string]interface{}:
			args = v
		default:
			log.Printf("[MCP TOOL] read_exchange_value: Invalid arguments format")
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		if cid, ok := args["client_id"].(string); ok && cid != "" {
			clientID = cid
		}

		idxVal, ok := args["index"].(float64)
		if !ok {
			log.Printf("[MCP TOOL] read_exchange_value: Invalid index format")
			return mcp.NewToolResultError("Invalid index format"), nil
		}
		index := int(idxVal)

		log.Printf("[MCP TOOL] read_exchange_value called with client_id=%s, index=%d", clientID, index)

		key := fmt.Sprintf("essensys:client:%s:exchange", clientID)
		field := strconv.Itoa(index)
		log.Printf("[MCP TOOL] Reading Redis key: %s, field: %s", key, field)
		
		val, err := rdb.HGet(ctx, key, field).Result()
		if err == redis.Nil {
			log.Printf("[MCP TOOL] Value not found at index %d for client %s", index, clientID)
			return mcp.NewToolResultText(""), nil
		}
		if err != nil {
			log.Printf("[MCP TOOL] Redis error reading %s[%s]: %v", key, field, err)
			return mcp.NewToolResultError(fmt.Sprintf("Redis error: %v", err)), nil
		}

		log.Printf("[MCP TOOL] read_exchange_value result: index=%d, value=%s", index, val)
		return mcp.NewToolResultText(val), nil
	})

	// Tool: set_exchange_value
	s.AddTool(mcp.NewTool("set_exchange_value",
		mcp.WithDescription("Directly set a value in the exchange table (Warning: Bypasses order queue logic usually)."),
		mcp.WithString("client_id", mcp.Description("Client ID (default: 'default')")),
		mcp.WithNumber("index", mcp.Required(), mcp.Description("Index to set")),
		mcp.WithString("value", mcp.Required(), mcp.Description("Value to set")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		clientID := "default"
		// Handle Arguments which can be map[string]interface{} or other types
		var args map[string]interface{}
		switch v := request.Params.Arguments.(type) {
		case map[string]interface{}:
			args = v
		default:
			log.Printf("[MCP TOOL] set_exchange_value: Invalid arguments format")
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		if cid, ok := args["client_id"].(string); ok && cid != "" {
			clientID = cid
		}

		idxVal, ok := args["index"].(float64)
		if !ok {
			log.Printf("[MCP TOOL] set_exchange_value: Invalid index format")
			return mcp.NewToolResultError("Invalid index format"), nil
		}
		index := int(idxVal)

		value, ok := args["value"].(string)
		if !ok {
			log.Printf("[MCP TOOL] set_exchange_value: Invalid value format")
			return mcp.NewToolResultError("Invalid value format"), nil
		}

		log.Printf("[MCP TOOL] set_exchange_value called with client_id=%s, index=%d, value=%s", clientID, index, value)

		key := fmt.Sprintf("essensys:client:%s:exchange", clientID)
		field := strconv.Itoa(index)
		log.Printf("[MCP TOOL] Setting Redis key: %s, field: %s, value: %s", key, field, value)
		
		err := rdb.HSet(ctx, key, field, value).Err()
		if err != nil {
			log.Printf("[MCP TOOL] Redis error setting %s[%s]=%s: %v", key, field, value, err)
			return mcp.NewToolResultError(fmt.Sprintf("Redis error: %v", err)), nil
		}

		result := fmt.Sprintf("Set index %d to '%s' for client %s", index, value, clientID)
		log.Printf("[MCP TOOL] set_exchange_value success: %s", result)
		return mcp.NewToolResultText(result), nil
	})

	// Tool: send_order
	s.AddTool(mcp.NewTool("send_order",
		mcp.WithDescription("Send an order (action) to the backend via the global action queue."),
		mcp.WithString("guid", mcp.Description("Unique ID for the action (optional, auto-generated if empty)")),
		mcp.WithString("params_json", mcp.Required(), mcp.Description("JSON string representing the parameters (ExchangeKV list) e.g. '[{\"k\":1,\"v\":\"1\"}]'")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		guid := ""
		// Handle Arguments which can be map[string]interface{} or other types
		var args map[string]interface{}
		switch v := request.Params.Arguments.(type) {
		case map[string]interface{}:
			args = v
		default:
			log.Printf("[MCP TOOL] send_order: Invalid arguments format")
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		if g, ok := args["guid"].(string); ok {
			guid = g
		}
		if guid == "" {
			guid = fmt.Sprintf("mcp-%d", time.Now().UnixNano())
		}

		paramsStr, ok := args["params_json"].(string)
		if !ok {
			log.Printf("[MCP TOOL] send_order: Invalid params_json format")
			return mcp.NewToolResultError("Invalid params_json format"), nil
		}

		log.Printf("[MCP TOOL] send_order called with guid=%s, params_json=%s", guid, paramsStr)

		var rawParams interface{}
		if err := json.Unmarshal([]byte(paramsStr), &rawParams); err != nil {
			log.Printf("[MCP TOOL] send_order: Invalid JSON in params_json: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Invalid JSON in params_json: %v", err)), nil
		}

		actionJSON := fmt.Sprintf(`{"guid":"%s","params":%s}`, guid, paramsStr)
		log.Printf("[MCP TOOL] Generated action JSON: %s", actionJSON)
		
		key := "essensys:global:actions"
		log.Printf("[MCP TOOL] Pushing to Redis queue: %s", key)
		
		err := rdb.RPush(ctx, key, actionJSON).Err()
		if err != nil {
			log.Printf("[MCP TOOL] Redis error pushing to %s: %v", key, err)
			return mcp.NewToolResultError(fmt.Sprintf("Redis error: %v", err)), nil
		}

		result := fmt.Sprintf("Order sent with GUID %s", guid)
		log.Printf("[MCP TOOL] send_order success: %s", result)
		return mcp.NewToolResultText(result), nil
	})

	if *mode == "stdio" {
		if err := server.ServeStdio(s); err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	} else {
		// SSE Mode
		baseURL := fmt.Sprintf("http://localhost:%s", *port)
		// NewSSEServer returns a handler that implements the MCP Streamable HTTP spec
		// The handler should handle both GET (SSE) and POST (messages) requests
		sseServer := server.NewSSEServer(s, 
			server.WithBaseURL(baseURL),
			server.WithSSEEndpoint("/sse"),
			server.WithMessageEndpoint("/messages"),
		)
		
		// Setup Mux - mount the SSE server handler
		// The handler returned by NewSSEServer should handle routing internally
		// We mount it on both paths as configured with WithSSEEndpoint and WithMessageEndpoint
		mux := http.NewServeMux()
		
		// Wrap handler to log all requests with detailed information
		loggingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract client IP
			clientIP := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				parts := strings.Split(forwarded, ",")
				clientIP = strings.TrimSpace(parts[0])
			}
			
			// Log request details
			log.Printf("[MCP HTTP] %s %s from %s", r.Method, r.URL.Path, clientIP)
			log.Printf("[MCP HTTP] Headers: User-Agent=%s, Content-Type=%s, Content-Length=%s", 
				r.Header.Get("User-Agent"), r.Header.Get("Content-Type"), r.Header.Get("Content-Length"))
			
			// For POST requests, log the body
			if r.Method == "POST" && r.Body != nil {
				bodyBytes, err := io.ReadAll(r.Body)
				if err == nil && len(bodyBytes) > 0 {
					// Restore body for handler
					r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
					
					// Try to parse as JSON for better logging
					var jsonData interface{}
					if err := json.Unmarshal(bodyBytes, &jsonData); err == nil {
						jsonPretty, _ := json.MarshalIndent(jsonData, "", "  ")
						log.Printf("[MCP HTTP] Request body (JSON):\n%s", string(jsonPretty))
					} else {
						log.Printf("[MCP HTTP] Request body (raw): %s", string(bodyBytes))
					}
				}
			}
			
			// Wrap response writer to capture status code and response
			responseWriter := &responseLogger{
				ResponseWriter: w,
				statusCode:     200,
			}
			
			startTime := time.Now()
			sseServer.ServeHTTP(responseWriter, r)
			duration := time.Since(startTime)
			
			log.Printf("[MCP HTTP] Response: %s %s -> %d (%v)", r.Method, r.URL.Path, responseWriter.statusCode, duration)
		})
		
		mux.Handle("/sse", loggingHandler)
		mux.Handle("/messages", loggingHandler)
		
		// Add a catch-all handler to log unhandled requests for debugging
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Unhandled request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			http.NotFound(w, r)
		})

		log.Printf("===========================================")
		log.Printf("Essensys MCP Server")
		log.Printf("Version: %s", version)
		log.Printf("Build time: %s", buildTime)
		log.Printf("Git commit: %s", gitCommit)
		log.Printf("===========================================")
		log.Printf("Starting MCP SSE server on port %s (Private IPs only)...", *port)
		log.Printf("SSE endpoint: http://localhost:%s/sse (GET)", *port)
		log.Printf("Messages endpoint: http://localhost:%s/messages (POST)", *port)
        
        // Chain middlewares: IP Check -> Auth Check -> Handler
        handler := authMiddleware(mux, *token)
        handler = privateIPMiddleware(handler)

		if err := http.ListenAndServe(":"+*port, handler); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}
