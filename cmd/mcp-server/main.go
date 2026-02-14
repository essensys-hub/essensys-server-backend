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
	"os"
	"path/filepath"
	"sort"
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

type exchangeKV struct {
	K int    `json:"k"`
	V string `json:"v"`
}

func expandLegacyScenarioBlock(params []exchangeKV) []exchangeKV {
	hasLightShutterIndex := false
	for _, p := range params {
		if p.K >= 605 && p.K <= 622 {
			hasLightShutterIndex = true
			break
		}
	}
	if !hasLightShutterIndex {
		return params
	}

	byIndex := make(map[int]string)
	for _, p := range params {
		byIndex[p.K] = p.V
	}

	// BP_MQX_ETH expects full scenario trigger + complete 605..622 block.
	if _, exists := byIndex[590]; !exists {
		byIndex[590] = "1"
	}
	for i := 605; i <= 622; i++ {
		if _, exists := byIndex[i]; !exists {
			byIndex[i] = "0"
		}
	}

	out := make([]exchangeKV, 0, len(byIndex))
	for k, v := range byIndex {
		out = append(out, exchangeKV{K: k, V: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out
}

func normalizeActionName(action string) string {
	a := strings.ToLower(strings.TrimSpace(action))
	a = strings.ReplaceAll(a, "é", "e")
	a = strings.ReplaceAll(a, "è", "e")
	a = strings.ReplaceAll(a, "ê", "e")
	a = strings.ReplaceAll(a, "à", "a")
	a = strings.ReplaceAll(a, "ù", "u")
	return a
}

func deriveOppositeAction(index int, value, action, category string) (int, string, string, bool) {
	switch strings.ToLower(category) {
	case "light":
		switch normalizeActionName(action) {
		case "allumer":
			if index >= 611 && index <= 616 {
				return index - 6, value, "eteindre", true
			}
		case "eteindre":
			if index >= 605 && index <= 610 {
				return index + 6, value, "allumer", true
			}
		}
	case "shutter":
		switch normalizeActionName(action) {
		case "ouvrir":
			if index >= 617 && index <= 619 {
				return index + 3, value, "fermer", true
			}
		case "fermer":
			if index >= 620 && index <= 622 {
				return index - 3, value, "ouvrir", true
			}
		}
	}
	return 0, "", "", false
}

const essensysSkillMarkdown = `---
name: essensys-quick-commands
version: 2026.01.30.1
description: Explains Essensys backend flow and fast command patterns to avoid repeated long exchanges about the reference table. Use when the user asks to control lights/shutters/scenarios and when ON/OFF may use different indices.
---

# Essensys Quick Commands

## Goal

Respond fast with minimal tool calls by using direct command patterns for common actions.

## Fast flow

1. Use find_device_index with device_name (+ category/action when possible).
2. Use send_order with returned index/value.
3. Let send_order auto-expand legacy block (590 + 605..622) when needed.

## Important rule

For lights and shutters, ON/OFF or OPEN/CLOSE can use different indices.

Example:
- allumer chevet petite chambre 3 -> index 613, value 64
- eteindre chevet petite chambre 3 -> index 607, value 64

## Response style

- Keep answers short.
- Return: Cause, Technical proof (index/value), and exact command to run.
`

const essensysSkillReferenceMarkdown = `# Essensys quick reference

## Backend path

MCP/API -> Redis list essensys:global:actions -> /api/myactions -> /api/done/{guid}

## Legacy scenario block

When payload contains light/shutter indices (605..622), include full block:
- 590 trigger
- 605..622 complete (missing values set to 0)

## Action symmetry

Lights:
- allumer indexes: 611..616
- eteindre indexes: 605..610 (often ON index - 6)

Shutters:
- ouvrir indexes: 617..619
- fermer indexes: 620..622 (often OPEN index + 3)
`

const essensysSkillPackVersion = "2026.01.30.1"

func installEssensysSkillPack(targetDir string) (string, string, string, error) {
	cleanDir := filepath.Clean(strings.TrimSpace(targetDir))
	if cleanDir == "" || cleanDir == "." {
		cleanDir = ".cursor/skills/essensys-quick-commands"
	}

	if err := os.MkdirAll(cleanDir, 0o755); err != nil {
		return "", "", "", fmt.Errorf("failed to create skill directory: %w", err)
	}

	skillPath := filepath.Join(cleanDir, "SKILL.md")
	refPath := filepath.Join(cleanDir, "reference.md")
	manifestPath := filepath.Join(cleanDir, "skill-manifest.json")

	if err := os.WriteFile(skillPath, []byte(essensysSkillMarkdown), 0o644); err != nil {
		return "", "", "", fmt.Errorf("failed to write SKILL.md: %w", err)
	}
	if err := os.WriteFile(refPath, []byte(essensysSkillReferenceMarkdown), 0o644); err != nil {
		return "", "", "", fmt.Errorf("failed to write reference.md: %w", err)
	}

	manifestBytes, err := json.MarshalIndent(map[string]string{
		"name":         "essensys-quick-commands",
		"version":      essensysSkillPackVersion,
		"updated_at":   time.Now().UTC().Format(time.RFC3339),
		"generated_by": "download_essensys_skill",
	}, "", "  ")
	if err != nil {
		return "", "", "", fmt.Errorf("failed to build skill manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return "", "", "", fmt.Errorf("failed to write skill-manifest.json: %w", err)
	}

	return skillPath, refPath, manifestPath, nil
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
		mcp.WithDescription("Read all values from the exchange table. Returns a map of index to value. Use find_device_index to search for specific devices by name."),
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

	// Tool: find_device_index
	s.AddTool(mcp.NewTool("find_device_index",
		mcp.WithDescription("Find exchange table index/value by device name. Supports partial matching, category filter, and action filter. Important: ON/OFF (or open/close) may use different indices. Use action='allumer' or action='eteindre' for lights, and action='ouvrir' or action='fermer' for shutters."),
		mcp.WithString("device_name", mcp.Required(), mcp.Description("Device name to search for (partial match supported, e.g., 'chevet chambre petit 3', 'lampe salon', 'volet cuisine')")),
		mcp.WithString("category", mcp.Description("Optional category filter: 'light', 'shutter', 'scenario', 'security', 'heating', 'irrigation'")),
		mcp.WithString("action", mcp.Description("Optional action filter: e.g., 'allumer', 'eteindre', 'ouvrir', 'fermer'")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]interface{}
		switch v := request.Params.Arguments.(type) {
		case map[string]interface{}:
			args = v
		default:
			log.Printf("[MCP TOOL] find_device_index: Invalid arguments format")
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		deviceName, ok := args["device_name"].(string)
		if !ok || deviceName == "" {
			log.Printf("[MCP TOOL] find_device_index: Invalid device_name format")
			return mcp.NewToolResultError("device_name is required"), nil
		}

		category := ""
		if cat, ok := args["category"].(string); ok {
			category = strings.ToLower(cat)
		}
		actionFilter := ""
		if action, ok := args["action"].(string); ok {
			actionFilter = normalizeActionName(action)
		}

		log.Printf("[MCP TOOL] find_device_index called with device_name=%s, category=%s, action=%s", deviceName, category, actionFilter)

		// Complete device mapping based on debug.md documentation
		deviceMap := map[string][]map[string]interface{}{
			// Éclairage - ALLUMER PDV (Pièces De Vie)
			"lampe entrée":             {{"index": 611, "value": "1", "action": "allumer", "category": "light", "description": "Lampe Entrée"}},
			"entrée":                   {{"index": 611, "value": "1", "action": "allumer", "category": "light", "description": "Lampe Entrée"}},
			"lampe salon 1":            {{"index": 611, "value": "2", "action": "allumer", "category": "light", "description": "Lampe Salon 1"}},
			"lampe salon":              {{"index": 611, "value": "2", "action": "allumer", "category": "light", "description": "Lampe Salon 1"}},
			"salon lampe":              {{"index": 611, "value": "2", "action": "allumer", "category": "light", "description": "Lampe Salon 1"}},
			"lampe salon 2":            {{"index": 611, "value": "4", "action": "allumer", "category": "light", "description": "Lampe Salon 2"}},
			"lampe dressing 1":         {{"index": 611, "value": "8", "action": "allumer", "category": "light", "description": "Lampe Dressing 1"}},
			"dressing 1":               {{"index": 611, "value": "8", "action": "allumer", "category": "light", "description": "Lampe Dressing 1"}},
			"lampe dressing 2":         {{"index": 611, "value": "16", "action": "allumer", "category": "light", "description": "Lampe Dressing 2"}},
			"dressing 2":               {{"index": 611, "value": "16", "action": "allumer", "category": "light", "description": "Lampe Dressing 2"}},
			"variateur bureau":         {{"index": 612, "value": "32", "action": "allumer", "category": "light", "description": "Variateur Bureau"}},
			"bureau variateur":         {{"index": 612, "value": "32", "action": "allumer", "category": "light", "description": "Variateur Bureau"}},
			"variateur sam":            {{"index": 612, "value": "64", "action": "allumer", "category": "light", "description": "Variateur Salle à Manger"}},
			"variateur salle à manger": {{"index": 612, "value": "64", "action": "allumer", "category": "light", "description": "Variateur Salle à Manger"}},
			"variateur salon":          {{"index": 612, "value": "128", "action": "allumer", "category": "light", "description": "Variateur Salon"}},

			// Éclairage - ALLUMER CHB (Chambres)
			"lampe escalier":             {{"index": 613, "value": "1", "action": "allumer", "category": "light", "description": "Lampe Escalier"}},
			"escalier":                   {{"index": 613, "value": "1", "action": "allumer", "category": "light", "description": "Lampe Escalier"}},
			"lampe grande chambre 1":     {{"index": 613, "value": "2", "action": "allumer", "category": "light", "description": "Lampe Grande Chambre 1"}},
			"grande chambre 1":           {{"index": 613, "value": "2", "action": "allumer", "category": "light", "description": "Lampe Grande Chambre 1"}},
			"lampe grande chambre 2":     {{"index": 613, "value": "4", "action": "allumer", "category": "light", "description": "Lampe Grande Chambre 2"}},
			"grande chambre 2":           {{"index": 613, "value": "4", "action": "allumer", "category": "light", "description": "Lampe Grande Chambre 2"}},
			"lampe petite chambre 1":     {{"index": 613, "value": "8", "action": "allumer", "category": "light", "description": "Lampe Petite Chambre 1 (1)"}},
			"petite chambre 1":           {{"index": 613, "value": "8", "action": "allumer", "category": "light", "description": "Lampe Petite Chambre 1 (1)"}},
			"lampe petite chambre 1 2":   {{"index": 613, "value": "16", "action": "allumer", "category": "light", "description": "Lampe Petite Chambre 1 (2)"}},
			"lampe petite chambre 2":     {{"index": 613, "value": "32", "action": "allumer", "category": "light", "description": "Lampe Petite Chambre 2"}},
			"petite chambre 2":           {{"index": 613, "value": "32", "action": "allumer", "category": "light", "description": "Lampe Petite Chambre 2"}},
			"lampe petite chambre 3":     {{"index": 613, "value": "64", "action": "allumer", "category": "light", "description": "Lampe Petite Chambre 3"}},
			"petite chambre 3":           {{"index": 613, "value": "64", "action": "allumer", "category": "light", "description": "Lampe Petite Chambre 3"}},
			"chevet chambre petit 3":     {{"index": 613, "value": "64", "action": "allumer", "category": "light", "description": "Chevet Petite Chambre 3"}},
			"chevet petite chambre 3":    {{"index": 613, "value": "64", "action": "allumer", "category": "light", "description": "Chevet Petite Chambre 3"}},
			"petite chambre 3 chevet":    {{"index": 613, "value": "64", "action": "allumer", "category": "light", "description": "Chevet Petite Chambre 3"}},
			"variateur petite chambre 3": {{"index": 614, "value": "16", "action": "allumer", "category": "light", "description": "Variateur Petite Chambre 3"}},
			"variateur petite chambre 2": {{"index": 614, "value": "32", "action": "allumer", "category": "light", "description": "Variateur Petite Chambre 2"}},
			"variateur petite chambre 1": {{"index": 614, "value": "64", "action": "allumer", "category": "light", "description": "Variateur Petite Chambre 1"}},
			"variateur grande chambre":   {{"index": 614, "value": "128", "action": "allumer", "category": "light", "description": "Variateur Grande Chambre"}},

			// Éclairage - ALLUMER PDE (Pièces d'Eau)
			"lampe cuisine 1":    {{"index": 615, "value": "1", "action": "allumer", "category": "light", "description": "Lampe Cuisine 1"}},
			"cuisine 1":          {{"index": 615, "value": "1", "action": "allumer", "category": "light", "description": "Lampe Cuisine 1"}},
			"lampe cuisine 2":    {{"index": 615, "value": "2", "action": "allumer", "category": "light", "description": "Lampe Cuisine 2"}},
			"cuisine 2":          {{"index": 615, "value": "2", "action": "allumer", "category": "light", "description": "Lampe Cuisine 2"}},
			"lampe sdb 1":        {{"index": 615, "value": "4", "action": "allumer", "category": "light", "description": "Lampe SDB 1"}},
			"salle de bain 1":    {{"index": 615, "value": "4", "action": "allumer", "category": "light", "description": "Lampe SDB 1"}},
			"lampe sdb 2":        {{"index": 615, "value": "8", "action": "allumer", "category": "light", "description": "Lampe SDB 2 (1)"}},
			"salle de bain 2":    {{"index": 615, "value": "8", "action": "allumer", "category": "light", "description": "Lampe SDB 2 (1)"}},
			"lampe sdb 2 2":      {{"index": 615, "value": "16", "action": "allumer", "category": "light", "description": "Lampe SDB 2 (2)"}},
			"lampe wc 1":         {{"index": 615, "value": "32", "action": "allumer", "category": "light", "description": "Lampe WC 1"}},
			"wc 1":               {{"index": 615, "value": "32", "action": "allumer", "category": "light", "description": "Lampe WC 1"}},
			"lampe wc 2":         {{"index": 615, "value": "64", "action": "allumer", "category": "light", "description": "Lampe WC 2"}},
			"wc 2":               {{"index": 615, "value": "64", "action": "allumer", "category": "light", "description": "Lampe WC 2"}},
			"lampe service":      {{"index": 615, "value": "128", "action": "allumer", "category": "light", "description": "Lampe Service"}},
			"service":            {{"index": 615, "value": "128", "action": "allumer", "category": "light", "description": "Lampe Service"}},
			"lampe dégagement 1": {{"index": 616, "value": "1", "action": "allumer", "category": "light", "description": "Lampe Dégagement 1"}},
			"dégagement 1":       {{"index": 616, "value": "1", "action": "allumer", "category": "light", "description": "Lampe Dégagement 1"}},
			"lampe dégagement 2": {{"index": 616, "value": "2", "action": "allumer", "category": "light", "description": "Lampe Dégagement 2"}},
			"dégagement 2":       {{"index": 616, "value": "2", "action": "allumer", "category": "light", "description": "Lampe Dégagement 2"}},
			"lampe terrasse":     {{"index": 616, "value": "4", "action": "allumer", "category": "light", "description": "Lampe Terrasse"}},
			"terrasse":           {{"index": 616, "value": "4", "action": "allumer", "category": "light", "description": "Lampe Terrasse"}},
			"lampe annexe 1":     {{"index": 616, "value": "8", "action": "allumer", "category": "light", "description": "Lampe Annexe 1"}},
			"annexe 1":           {{"index": 616, "value": "8", "action": "allumer", "category": "light", "description": "Lampe Annexe 1"}},
			"lampe annexe 2":     {{"index": 616, "value": "16", "action": "allumer", "category": "light", "description": "Lampe Annexe 2"}},
			"annexe 2":           {{"index": 616, "value": "16", "action": "allumer", "category": "light", "description": "Lampe Annexe 2"}},
			"variateur sdb 1":    {{"index": 616, "value": "128", "action": "allumer", "category": "light", "description": "Variateur SDB 1"}},

			// Éclairage - ÉTEINDRE (mêmes indices mais action différente)
			"éteindre entrée":   {{"index": 605, "value": "1", "action": "éteindre", "category": "light", "description": "Éteindre Entrée"}},
			"éteindre salon":    {{"index": 605, "value": "2", "action": "éteindre", "category": "light", "description": "Éteindre Salon 1"}},
			"éteindre chambres": {{"index": 607, "value": "64", "action": "éteindre", "category": "light", "description": "Éteindre Petite Chambre 3"}},

			// Volets - OUVRIR PDV
			"volet salon 1":          {{"index": 617, "value": "1", "action": "ouvrir", "category": "shutter", "description": "Volet Salon 1"}},
			"ouvrir volet salon 1":   {{"index": 617, "value": "1", "action": "ouvrir", "category": "shutter", "description": "Volet Salon 1"}},
			"volet salon 2":          {{"index": 617, "value": "2", "action": "ouvrir", "category": "shutter", "description": "Volet Salon 2"}},
			"ouvrir volet salon 2":   {{"index": 617, "value": "2", "action": "ouvrir", "category": "shutter", "description": "Volet Salon 2"}},
			"volet salon 3":          {{"index": 617, "value": "4", "action": "ouvrir", "category": "shutter", "description": "Volet Salon 3"}},
			"ouvrir volet salon 3":   {{"index": 617, "value": "4", "action": "ouvrir", "category": "shutter", "description": "Volet Salon 3"}},
			"volet sam 1":            {{"index": 617, "value": "8", "action": "ouvrir", "category": "shutter", "description": "Volet SAM 1"}},
			"volet salle à manger 1": {{"index": 617, "value": "8", "action": "ouvrir", "category": "shutter", "description": "Volet SAM 1"}},
			"volet sam 2":            {{"index": 617, "value": "16", "action": "ouvrir", "category": "shutter", "description": "Volet SAM 2"}},
			"volet salle à manger 2": {{"index": 617, "value": "16", "action": "ouvrir", "category": "shutter", "description": "Volet SAM 2"}},
			"volet bureau":           {{"index": 617, "value": "32", "action": "ouvrir", "category": "shutter", "description": "Volet Bureau"}},
			"ouvrir volet bureau":    {{"index": 617, "value": "32", "action": "ouvrir", "category": "shutter", "description": "Volet Bureau"}},

			// Volets - OUVRIR CHB
			"volet grande chambre 1":        {{"index": 618, "value": "1", "action": "ouvrir", "category": "shutter", "description": "Volet Grande Chambre 1"}},
			"ouvrir volet grande chambre 1": {{"index": 618, "value": "1", "action": "ouvrir", "category": "shutter", "description": "Volet Grande Chambre 1"}},
			"volet grande chambre 2":        {{"index": 618, "value": "2", "action": "ouvrir", "category": "shutter", "description": "Volet Grande Chambre 2"}},
			"ouvrir volet grande chambre 2": {{"index": 618, "value": "2", "action": "ouvrir", "category": "shutter", "description": "Volet Grande Chambre 2"}},
			"volet petite chambre 1":        {{"index": 618, "value": "4", "action": "ouvrir", "category": "shutter", "description": "Volet Petite Chambre 1"}},
			"ouvrir volet petite chambre 1": {{"index": 618, "value": "4", "action": "ouvrir", "category": "shutter", "description": "Volet Petite Chambre 1"}},
			"volet petite chambre 2":        {{"index": 618, "value": "8", "action": "ouvrir", "category": "shutter", "description": "Volet Petite Chambre 2"}},
			"ouvrir volet petite chambre 2": {{"index": 618, "value": "8", "action": "ouvrir", "category": "shutter", "description": "Volet Petite Chambre 2"}},
			"volet petite chambre 3":        {{"index": 618, "value": "16", "action": "ouvrir", "category": "shutter", "description": "Volet Petite Chambre 3"}},
			"ouvrir volet petite chambre 3": {{"index": 618, "value": "16", "action": "ouvrir", "category": "shutter", "description": "Volet Petite Chambre 3"}},

			// Volets - OUVRIR PDE
			"volet cuisine 1":         {{"index": 619, "value": "1", "action": "ouvrir", "category": "shutter", "description": "Volet Cuisine 1"}},
			"ouvrir volet cuisine 1":  {{"index": 619, "value": "1", "action": "ouvrir", "category": "shutter", "description": "Volet Cuisine 1"}},
			"volet cuisine 2":         {{"index": 619, "value": "2", "action": "ouvrir", "category": "shutter", "description": "Volet Cuisine 2"}},
			"ouvrir volet cuisine 2":  {{"index": 619, "value": "2", "action": "ouvrir", "category": "shutter", "description": "Volet Cuisine 2"}},
			"volet sdb 1":             {{"index": 619, "value": "4", "action": "ouvrir", "category": "shutter", "description": "Volet SDB 1"}},
			"ouvrir volet sdb 1":      {{"index": 619, "value": "4", "action": "ouvrir", "category": "shutter", "description": "Volet SDB 1"}},
			"store terrasse":          {{"index": 619, "value": "8", "action": "ouvrir", "category": "shutter", "description": "Remonter Store Terrasse"}},
			"remonter store terrasse": {{"index": 619, "value": "8", "action": "ouvrir", "category": "shutter", "description": "Remonter Store Terrasse"}},

			// Volets - FERMER (mêmes valeurs mais index différent)
			"fermer volet salon 1":          {{"index": 620, "value": "1", "action": "fermer", "category": "shutter", "description": "Fermer Volet Salon 1"}},
			"fermer volet grande chambre 1": {{"index": 621, "value": "1", "action": "fermer", "category": "shutter", "description": "Fermer Volet Grande Chambre 1"}},
			"fermer volet petite chambre 3": {{"index": 621, "value": "16", "action": "fermer", "category": "shutter", "description": "Fermer Volet Petite Chambre 3"}},
			"fermer volet cuisine 1":        {{"index": 622, "value": "1", "action": "fermer", "category": "shutter", "description": "Fermer Volet Cuisine 1"}},

			// Scénarios
			"scénario 1":         {{"index": 590, "value": "1", "action": "scénario", "category": "scenario", "description": "Scénario 1 (Réservé Internet)"}},
			"scénario 2":         {{"index": 590, "value": "2", "action": "scénario", "category": "scenario", "description": "Scénario 2 (Je sors)"}},
			"scénario je sors":   {{"index": 590, "value": "2", "action": "scénario", "category": "scenario", "description": "Scénario 2 (Je sors)"}},
			"scénario 3":         {{"index": 590, "value": "3", "action": "scénario", "category": "scenario", "description": "Scénario 3 (Je pars en vacances)"}},
			"scénario vacances":  {{"index": 590, "value": "3", "action": "scénario", "category": "scenario", "description": "Scénario 3 (Je pars en vacances)"}},
			"scénario 4":         {{"index": 590, "value": "4", "action": "scénario", "category": "scenario", "description": "Scénario 4 (Je rentre)"}},
			"scénario je rentre": {{"index": 590, "value": "4", "action": "scénario", "category": "scenario", "description": "Scénario 4 (Je rentre)"}},
			"scénario 5":         {{"index": 590, "value": "5", "action": "scénario", "category": "scenario", "description": "Scénario 5 (Je vais me coucher)"}},
			"scénario coucher":   {{"index": 590, "value": "5", "action": "scénario", "category": "scenario", "description": "Scénario 5 (Je vais me coucher)"}},
			"scénario 6":         {{"index": 590, "value": "6", "action": "scénario", "category": "scenario", "description": "Scénario 6 (Je me lève)"}},
			"scénario lever":     {{"index": 590, "value": "6", "action": "scénario", "category": "scenario", "description": "Scénario 6 (Je me lève)"}},
			"scénario 7":         {{"index": 590, "value": "7", "action": "scénario", "category": "scenario", "description": "Scénario 7 (Personnalisé 1)"}},
			"scénario 8":         {{"index": 590, "value": "8", "action": "scénario", "category": "scenario", "description": "Scénario 8 (Personnalisé 2)"}},

			// Sécurité
			"alarme":                   {{"index": 593, "value": "1", "action": "activer", "category": "security", "description": "Mettre l'alarme"}},
			"mettre alarme":            {{"index": 593, "value": "1", "action": "activer", "category": "security", "description": "Mettre l'alarme"}},
			"enlever alarme":           {{"index": 593, "value": "2", "action": "désactiver", "category": "security", "description": "Enlever l'alarme"}},
			"couper prises sécurité":   {{"index": 623, "value": "1", "action": "couper", "category": "security", "description": "Couper prises sécurité"}},
			"remettre prises sécurité": {{"index": 623, "value": "2", "action": "rétablir", "category": "security", "description": "Remettre prises sécurité"}},
			"couper machines":          {{"index": 624, "value": "1", "action": "couper", "category": "security", "description": "Couper machines à laver"}},
			"remettre machines":        {{"index": 624, "value": "2", "action": "rétablir", "category": "security", "description": "Remettre machines à laver"}},

			// Arrosage
			"arrosage":             {{"index": 363, "value": "255", "action": "automatique", "category": "irrigation", "description": "Arrosage automatique"}},
			"arrosage automatique": {{"index": 363, "value": "255", "action": "automatique", "category": "irrigation", "description": "Arrosage automatique"}},
		}

		// Search for matches (case-insensitive, partial)
		deviceNameLower := strings.ToLower(deviceName)
		var matches []map[string]interface{}

		for key, devices := range deviceMap {
			keyLower := strings.ToLower(key)
			if strings.Contains(keyLower, deviceNameLower) || strings.Contains(deviceNameLower, keyLower) {
				// Filter by category if specified
				for _, device := range devices {
					deviceActionNorm := normalizeActionName(fmt.Sprintf("%v", device["action"]))
					if (category == "" || device["category"] == category) && (actionFilter == "" || deviceActionNorm == actionFilter) {
						matches = append(matches, device)
					}
				}
			}
		}

		if len(matches) == 0 {
			categoryHint := ""
			if category != "" {
				categoryHint = fmt.Sprintf(" dans la catégorie '%s'", category)
			}
			result := fmt.Sprintf("Aucun équipement trouvé pour '%s'%s.\n\nCatégories disponibles: 'light' (éclairage), 'shutter' (volets), 'scenario' (scénarios), 'security' (sécurité), 'heating' (chauffage), 'irrigation' (arrosage).\n\nExemples de recherche: 'chevet', 'lampe', 'volet', 'chambre', 'salon', 'scénario', 'alarme', etc.", deviceName, categoryHint)
			log.Printf("[MCP TOOL] find_device_index: No matches found for '%s' (category: %s)", deviceName, category)
			return mcp.NewToolResultText(result), nil
		}

		// Format results
		var resultParts []string
		resultParts = append(resultParts, fmt.Sprintf("Équipements trouvés pour '%s':", deviceName))
		if category != "" {
			resultParts = append(resultParts, fmt.Sprintf("(Filtré par catégorie: %s)", category))
		}
		for i, match := range matches {
			resultParts = append(resultParts, fmt.Sprintf("\n%d. %s", i+1, match["description"]))
			resultParts = append(resultParts, fmt.Sprintf("   Index: %d", match["index"]))
			resultParts = append(resultParts, fmt.Sprintf("   Valeur: %s", match["value"]))
			resultParts = append(resultParts, fmt.Sprintf("   Action: %s", match["action"]))
			resultParts = append(resultParts, fmt.Sprintf("   Catégorie: %s", match["category"]))
			resultParts = append(resultParts, fmt.Sprintf("   Commande MCP: send_order avec params_json='[{\"k\":%d,\"v\":\"%s\"}]'", match["index"], match["value"]))
			matchIndex, idxOK := match["index"].(int)
			matchValue, valOK := match["value"].(string)
			matchAction, actionOK := match["action"].(string)
			matchCategory, categoryOK := match["category"].(string)
			if idxOK && valOK && actionOK && categoryOK {
				if oppositeIndex, oppositeValue, oppositeAction, ok := deriveOppositeAction(matchIndex, matchValue, matchAction, matchCategory); ok {
					resultParts = append(resultParts, fmt.Sprintf("   Action opposée (%s): index=%d, valeur=%s", oppositeAction, oppositeIndex, oppositeValue))
					resultParts = append(resultParts, fmt.Sprintf("   Commande MCP (%s): send_order avec params_json='[{\"k\":%d,\"v\":\"%s\"}]'", oppositeAction, oppositeIndex, oppositeValue))
				}
			}
		}

		result := strings.Join(resultParts, "\n")
		log.Printf("[MCP TOOL] find_device_index result: %s", result)
		return mcp.NewToolResultText(result), nil
	})

	// Tool: download_essensys_skill
	s.AddTool(mcp.NewTool("download_essensys_skill",
		mcp.WithDescription("Create a compact Essensys skill pack (SKILL.md + reference.md) to reduce repeated long exchanges about tools and reference table. Use this to speed up AI control workflows."),
		mcp.WithString("target_dir", mcp.Description("Optional directory where the skill will be generated. Default: .cursor/skills/essensys-quick-commands")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]interface{}
		switch v := request.Params.Arguments.(type) {
		case map[string]interface{}:
			args = v
		default:
			log.Printf("[MCP TOOL] download_essensys_skill: Invalid arguments format")
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		targetDir := ".cursor/skills/essensys-quick-commands"
		if raw, ok := args["target_dir"].(string); ok && strings.TrimSpace(raw) != "" {
			targetDir = raw
		}

		log.Printf("[MCP TOOL] download_essensys_skill called with target_dir=%s", targetDir)
		skillPath, refPath, manifestPath, err := installEssensysSkillPack(targetDir)
		if err != nil {
			log.Printf("[MCP TOOL] download_essensys_skill error: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Failed to generate Essensys skill pack: %v", err)), nil
		}

		result := fmt.Sprintf("Essensys skill pack generated.\n- Version: %s\n- SKILL.md: %s\n- reference.md: %s\n- skill-manifest.json: %s\n\nNext step: load this skill in your agent environment to reduce repeated tool calls and long reference-table explanations.", essensysSkillPackVersion, skillPath, refPath, manifestPath)
		log.Printf("[MCP TOOL] download_essensys_skill success: %s", result)
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

		var params []exchangeKV
		if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
			log.Printf("[MCP TOOL] send_order: params_json must be an array of {k,v}: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("params_json must be an array of {k,v}: %v", err)), nil
		}

		expanded := expandLegacyScenarioBlock(params)
		if len(expanded) != len(params) {
			expandedJSON, _ := json.Marshal(expanded)
			paramsStr = string(expandedJSON)
			log.Printf("[MCP TOOL] send_order expanded legacy block to: %s", paramsStr)
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

		// IMPORTANT: keep /sse unwrapped for streaming (ResponseWriter must support Flusher).
		sseLoggingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract client IP
			clientIP := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				parts := strings.Split(forwarded, ",")
				clientIP = strings.TrimSpace(parts[0])
			}

			// Log request details (no body read, no writer wrapper on SSE endpoint)
			log.Printf("[MCP HTTP] %s %s from %s", r.Method, r.URL.Path, clientIP)
			log.Printf("[MCP HTTP] Headers: User-Agent=%s, Accept=%s", r.Header.Get("User-Agent"), r.Header.Get("Accept"))
			startTime := time.Now()
			sseServer.ServeHTTP(w, r)
			duration := time.Since(startTime)
			log.Printf("[MCP HTTP] Response: %s %s -> stream/open (%v)", r.Method, r.URL.Path, duration)
		})

		// Detailed logging for /messages (safe to read body and wrap status code)
		messagesLoggingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		mux.Handle("/sse", sseLoggingHandler)
		mux.Handle("/messages", messagesLoggingHandler)

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
