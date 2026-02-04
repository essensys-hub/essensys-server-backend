package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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

// Redis Client
var rdb *redis.Client

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
		mcp.WithString("client_id", mcp.Description("Client ID to read from (default: 'default')"), mcp.Required(false)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		clientID := "default"
		if cid, ok := request.Params.Arguments["client_id"].(string); ok && cid != "" {
			clientID = cid
		}

		key := fmt.Sprintf("essensys:client:%s:exchange", clientID)
		vals, err := rdb.HGetAll(ctx, key).Result()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Redis error: %v", err)), nil
		}

		jsonBytes, _ := json.Marshal(vals)
		return mcp.NewToolResultText(string(jsonBytes)), nil
	})

	// Tool: read_exchange_value
	s.AddTool(mcp.NewTool("read_exchange_value",
		mcp.WithDescription("Read a specific value from the exchange table by index."),
		mcp.WithString("client_id", mcp.Description("Client ID (default: 'default')"), mcp.Required(false)),
		mcp.WithNumber("index", mcp.Description("Index to read"), mcp.Required(true)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		clientID := "default"
		if cid, ok := request.Params.Arguments["client_id"].(string); ok && cid != "" {
			clientID = cid
		}

		idxVal, ok := request.Params.Arguments["index"].(float64)
		if !ok {
			return mcp.NewToolResultError("Invalid index format"), nil
		}
		index := int(idxVal)

		key := fmt.Sprintf("essensys:client:%s:exchange", clientID)
		val, err := rdb.HGet(ctx, key, strconv.Itoa(index)).Result()
		if err == redis.Nil {
			return mcp.NewToolResultText(""), nil
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Redis error: %v", err)), nil
		}

		return mcp.NewToolResultText(val), nil
	})

	// Tool: set_exchange_value
	s.AddTool(mcp.NewTool("set_exchange_value",
		mcp.WithDescription("Directly set a value in the exchange table (Warning: Bypasses order queue logic usually)."),
		mcp.WithString("client_id", mcp.Description("Client ID (default: 'default')"), mcp.Required(false)),
		mcp.WithNumber("index", mcp.Description("Index to set"), mcp.Required(true)),
		mcp.WithString("value", mcp.Description("Value to set"), mcp.Required(true)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		clientID := "default"
		if cid, ok := request.Params.Arguments["client_id"].(string); ok && cid != "" {
			clientID = cid
		}

		idxVal, ok := request.Params.Arguments["index"].(float64)
		if !ok {
			return mcp.NewToolResultError("Invalid index format"), nil
		}
		index := int(idxVal)

		value, ok := request.Params.Arguments["value"].(string)
		if !ok {
			return mcp.NewToolResultError("Invalid value format"), nil
		}

		key := fmt.Sprintf("essensys:client:%s:exchange", clientID)
		err := rdb.HSet(ctx, key, strconv.Itoa(index), value).Err()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Redis error: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Set index %d to '%s' for client %s", index, value, clientID)), nil
	})

	// Tool: send_order
	s.AddTool(mcp.NewTool("send_order",
		mcp.WithDescription("Send an order (action) to the backend via the global action queue."),
		mcp.WithString("guid", mcp.Description("Unique ID for the action (optional, auto-generated if empty)"), mcp.Required(false)),
		mcp.WithString("params_json", mcp.Description("JSON string representing the parameters (ExchangeKV list) e.g. '[{\"k\":1,\"v\":\"1\"}]'"), mcp.Required(true)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		guid := ""
		if g, ok := request.Params.Arguments["guid"].(string); ok {
			guid = g
		}
		if guid == "" {
			guid = fmt.Sprintf("mcp-%d", time.Now().UnixNano())
		}

		paramsStr, ok := request.Params.Arguments["params_json"].(string)
		if !ok {
			return mcp.NewToolResultError("Invalid params_json format"), nil
		}

		var rawParams interface{}
		if err := json.Unmarshal([]byte(paramsStr), &rawParams); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid JSON in params_json: %v", err)), nil
		}

		actionJSON := fmt.Sprintf(`{"guid":"%s","params":%s}`, guid, paramsStr)
		key := "essensys:global:actions"
		err := rdb.RPush(ctx, key, actionJSON).Err()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Redis error: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Order sent with GUID %s", guid)), nil
	})

	if *mode == "stdio" {
		if err := server.ServeStdio(s); err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	} else {
		// SSE Mode
		sseServer := server.NewSSEServer(s, "http://localhost:"+*port)
		
		// Setup Mux
		mux := http.NewServeMux()
		mux.Handle("/sse", sseServer)
		mux.Handle("/messages", sseServer)

		log.Printf("Starting MCP SSE server on port %s (Private IPs only)...", *port)
        
        // Chain middlewares: IP Check -> Auth Check -> Handler
        handler := authMiddleware(sseServer, *token)
        handler = privateIPMiddleware(handler)

		if err := http.ListenAndServe(":"+*port, handler); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}
