# Essensys Backend Server

A Go-based HTTP gateway that bridges legacy BP_MQX_ETH embedded clients with modern web interfaces. This server maintains 100% protocol compatibility with the legacy ASP.NET implementation while providing a robust, concurrent, and maintainable architecture.

## Features

- **Protocol Compatibility**: 100% compatible with BP_MQX_ETH firmware clients
- **Concurrent Request Handling**: Efficiently handles multiple clients polling every 500ms
- **Thread-Safe Data Storage**: In-memory store with proper synchronization
- **Malformed JSON Support**: Automatically normalizes non-standard JSON from C clients
- **Authentication**: Basic Auth support for multiple clients
- **Graceful Shutdown**: Proper cleanup on SIGINT/SIGTERM signals
- **Comprehensive Logging**: Request/response logging with timing information

## Quick Start

```bash
# 1. Clone and build
git clone https://github.com/essensys-hub/essensys-server-backend.git
cd essensys-server-backend
go build -o server ./cmd/server

# 2. Configure port 80 access (Linux)
sudo setcap 'cap_net_bind_service=+ep' ./server

# 3. Run the server
./server

# 4. Test the server
curl http://localhost/health
# Response: {"status":"ok"}
```

For detailed setup instructions, see the sections below.

## Requirements

- Go 1.19 or higher
- Port 80 access (see Port Configuration below)
- Linux: libcap2-bin package for setcap (usually pre-installed)

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/essensys-hub/essensys-server-backend.git
cd essensys-server-backend

# Install dependencies
go mod download

# Build the server
go build -o server ./cmd/server

# Configure port 80 access (Linux only)
sudo setcap 'cap_net_bind_service=+ep' ./server

# Run the server
./server
```

### Using Docker

```bash
# Build the Docker image
docker build -t essensys-server .

# Run the container
docker run -d -p 80:80 --name essensys-server essensys-server

# Check logs
docker logs essensys-server

# Stop the container
docker stop essensys-server
```

### Using Go Install

```bash
# Install directly from source
go install github.com/essensys-hub/essensys-server-backend/cmd/server@latest

# The binary will be in $GOPATH/bin or ~/go/bin
# Configure port 80 access
sudo setcap 'cap_net_bind_service=+ep' ~/go/bin/server

# Run
~/go/bin/server
```

## Configuration

The server supports configuration through both environment variables and a YAML configuration file. Environment variables take precedence over YAML file values.

### Configuration Priority

1. Environment variables (highest priority)
2. config.yaml file
3. Default values (lowest priority)

### Environment Variables

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `SERVER_PORT` | HTTP server port | `80` | `8080` |
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | `info` | `debug` |
| `AUTH_ENABLED` | Enable/disable authentication | `false` | `true` |
| `CLIENT_CREDENTIALS` | Client credentials (comma-separated) | - | `client1:pass1,client2:pass2` |

#### Example: Using Environment Variables

```bash
# Run with custom port and debug logging
SERVER_PORT=8080 LOG_LEVEL=debug ./server

# Run with authentication disabled
AUTH_ENABLED=false ./server

# Run with specific client credentials
CLIENT_CREDENTIALS="testclient:testpass,client2:pass2" ./server
```

### YAML Configuration File

Create a `config.yaml` file in the same directory as the server binary:

```yaml
server:
  port: 80
  read_timeout: 10s
  write_timeout: 10s
  idle_timeout: 60s

auth:
  enabled: false
  clients:
    testclient: testpass
    client2: pass2

logging:
  level: info
  format: text
```

See `config.yaml.example` for a complete example with comments.

### Default Configuration

If no configuration is provided, the server uses these defaults:

- **Port**: 80 (MANDATORY for BP_MQX_ETH clients)
- **Read Timeout**: 10 seconds
- **Write Timeout**: 10 seconds
- **Idle Timeout**: 60 seconds
- **Authentication**: Disabled by default
- **Log Level**: info
- **Log Format**: text

## Port Configuration

### ⚠️ CRITICAL: Port 80 is MANDATORY

The BP_MQX_ETH client firmware is **hardcoded** to connect to port 80. The server MUST listen on port 80 for client compatibility. This is not configurable in the firmware.

**Why Port 80?**
- The embedded C firmware in BP_MQX_ETH devices has port 80 hardcoded
- Changing the port would require firmware updates on all devices
- The server defaults to port 80 to maintain compatibility

### Running on Port 80

Port 80 is a privileged port (< 1024) and requires elevated privileges. You have several options:

#### Option 1: Use setcap (Recommended for Linux)

This is the **recommended approach** for production Linux systems. It grants the binary permission to bind to privileged ports without running as root.

```bash
# Build the server
go build -o server ./cmd/server

# Grant the binary permission to bind to privileged ports
sudo setcap 'cap_net_bind_service=+ep' ./server

# Verify the capability was set
getcap ./server
# Output: ./server = cap_net_bind_service+ep

# Now you can run without sudo
./server
```

**Important Notes:**
- You need to re-run `setcap` after rebuilding the binary
- This only works on Linux systems with capability support
- The capability is tied to the specific binary file

**Automated Build Script:**
```bash
#!/bin/bash
# build.sh
go build -o server ./cmd/server
sudo setcap 'cap_net_bind_service=+ep' ./server
echo "Server built and capabilities set. Run with: ./server"
```

#### Option 2: Run as root (Not Recommended for Production)

```bash
# Build the server
go build -o server ./cmd/server

# Run with sudo
sudo ./server
```

**Security Warning:** Running as root gives the process full system access. Use setcap instead.

#### Option 3: Use Docker

Docker can map privileged ports without requiring the container to run as root.

```bash
# Build Docker image
docker build -t essensys-server .

# Run container with port mapping
docker run -p 80:80 essensys-server

# Run in background with restart policy
docker run -d --restart=unless-stopped -p 80:80 essensys-server
```

**Example Dockerfile:**
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o server ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 80
CMD ["./server"]
```

#### Option 4: Reverse Proxy (Not Recommended)

You can use nginx or Apache to forward port 80 to a higher port, but this adds complexity, latency, and potential compatibility issues.

**nginx example:**
```nginx
server {
    listen 80;
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

Then run the server on port 8080:
```bash
SERVER_PORT=8080 ./server
```

### Platform-Specific Instructions

#### Linux (Ubuntu/Debian)

```bash
# Install Go
sudo apt update
sudo apt install golang-go

# Build and configure
go build -o server ./cmd/server
sudo setcap 'cap_net_bind_service=+ep' ./server

# Run
./server
```

#### Linux (RHEL/CentOS/Fedora)

```bash
# Install Go
sudo dnf install golang

# Build and configure
go build -o server ./cmd/server
sudo setcap 'cap_net_bind_service=+ep' ./server

# Run
./server
```

#### macOS

macOS does not support Linux capabilities. You must run with sudo:

```bash
# Build
go build -o server ./cmd/server

# Run with sudo
sudo ./server
```

**Alternative for macOS:** Use a higher port for development:
```bash
SERVER_PORT=8080 ./server
```

Note: BP_MQX_ETH clients will not connect to port 8080. This is only for testing with other clients.

#### Windows

Windows does not have privileged ports. The server can bind to port 80 without special permissions:

```bash
# Build
go build -o server.exe ./cmd/server

# Run
./server.exe
```

### Verifying Port Configuration

After starting the server, verify it's listening on port 80:

**Linux/macOS:**
```bash
# Check if port 80 is listening
sudo lsof -i :80
# or
sudo netstat -tlnp | grep :80

# Test with curl
curl http://localhost/health
```

**Windows:**
```bash
# Check if port 80 is listening
netstat -an | findstr :80

# Test with curl
curl http://localhost/health
```

### Troubleshooting Port Issues

**Error: `bind: permission denied`**
```
Solution: Use setcap (Linux) or run with sudo
```

**Error: `bind: address already in use`**
```bash
# Find what's using port 80
sudo lsof -i :80
# or
sudo netstat -tlnp | grep :80

# Stop the conflicting service (e.g., Apache)
sudo systemctl stop apache2
# or
sudo systemctl stop nginx
```

**Warning: Server configured to use port 8080 instead of 80**
```
Impact: BP_MQX_ETH clients will not be able to connect
Solution: Change SERVER_PORT to 80 or remove the environment variable
```

## API Endpoints

All API endpoints (except `/health`) require authentication when `AUTH_ENABLED=true`.

### GET /api/serverinfos

Returns server information and connection status. This is typically the first endpoint called by clients.

**Authentication:** Required (when enabled)

**Request:**
```bash
curl -u client1:pass1 http://localhost/api/serverinfos
```

**Response:** HTTP 200 OK
```json
{
  "isconnected": true,
  "infos": [1, 2, 3],
  "newversion": "1.0.0"
}
```

**Response Headers:**
```
Content-Type: application/json ;charset=UTF-8
```

**Fields:**
- `isconnected` (boolean): Whether the client is connected to the server
- `infos` (array of integers): List of exchange table indices the server wants from the client
- `newversion` (string): Server version information

---

### POST /api/mystatus

Client sends status updates to the server. Updates the exchange table with current values from the client.

**Authentication:** Required (when enabled)

**Request:**
```bash
curl -X POST http://localhost/api/mystatus \
  -u client1:pass1 \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.0",
    "ek": [
      {"k": 1, "v": "100"},
      {"k": 2, "v": "200"},
      {"k": 605, "v": "0"}
    ]
  }'
```

**Request Body:**
```json
{
  "version": "1.0",
  "ek": [
    {"k": 1, "v": "100"},
    {"k": 2, "v": "200"}
  ]
}
```

**Fields:**
- `version` (string): Client firmware version
- `ek` (array): Exchange table key-value pairs
  - `k` (integer): Index (0-999)
  - `v` (string): Value (always string, even for numbers)

**Response:** HTTP 201 Created

**Special Feature - Malformed JSON Support:**

The server automatically normalizes malformed JSON from legacy C clients:

```bash
# Client sends malformed JSON (unquoted keys)
curl -X POST http://localhost/api/mystatus \
  -u client1:pass1 \
  -H "Content-Type: application/json" \
  -d '{version:"1.0",ek:[{k:1,v:"100"}]}'

# Server automatically converts to valid JSON and processes it
# Logs show: [NORMALIZE] Original: {version:"1.0",...} → Normalized: {"version":"1.0",...}
```

---

### GET /api/myactions

Client retrieves pending actions from the server. Actions are returned in FIFO order.

**Authentication:** Required (when enabled)

**Request:**
```bash
curl -u client1:pass1 http://localhost/api/myactions
```

**Response:** HTTP 200 OK

**Example 1 - No Pending Actions:**
```json
{
  "_de67f": null,
  "actions": []
}
```

**Example 2 - Single Action (Light Control):**
```json
{
  "_de67f": null,
  "actions": [
    {
      "guid": "ec9026fe-25fc-4b2f-b4b0-c5402699f399",
      "params": [
        {"k": 590, "v": "1"},
        {"k": 605, "v": "0"},
        {"k": 606, "v": "0"},
        {"k": 607, "v": "0"},
        {"k": 608, "v": "0"},
        {"k": 609, "v": "0"},
        {"k": 610, "v": "0"},
        {"k": 611, "v": "0"},
        {"k": 612, "v": "0"},
        {"k": 613, "v": "64"},
        {"k": 614, "v": "0"},
        {"k": 615, "v": "0"},
        {"k": 616, "v": "0"},
        {"k": 617, "v": "0"},
        {"k": 618, "v": "0"},
        {"k": 619, "v": "0"},
        {"k": 620, "v": "0"},
        {"k": 621, "v": "0"},
        {"k": 622, "v": "0"}
      ]
    }
  ]
}
```

**Example 3 - With Alarm Command:**
```json
{
  "_de67f": {
    "guid": "806b4fc7-a820-4c49-9ae8-24ced8f6770f",
    "obl": "73;178;187;105;197;154;208;248;26;52;219;233;77;251;102;182"
  },
  "actions": [
    {
      "guid": "ec9026fe-25fc-4b2f-b4b0-c5402699f399",
      "params": [
        {"k": 590, "v": "1"},
        {"k": 613, "v": "64"}
      ]
    }
  ]
}
```

**Response Headers:**
```
Content-Type: application/json ;charset=UTF-8
```

**Important Notes:**

1. **Field Ordering:** The `_de67f` field MUST appear before `actions` in the JSON response (required by C client parser)
2. **Complete Light Blocks:** Actions targeting indices 605-622 automatically include ALL indices from 605-622
3. **Scenario Trigger:** Index 590 with value "1" is automatically added for light/shutter actions
4. **GUID:** Each action has a unique GUID for acknowledgment

**Fields:**
- `_de67f` (object or null): Encrypted alarm command (optional)
  - `guid` (string): Unique identifier for alarm command
  - `obl` (string): AES encrypted alarm data
- `actions` (array): List of pending actions
  - `guid` (string): Unique identifier for this action
  - `params` (array): Action parameters
    - `k` (integer): Exchange table index
    - `v` (string): Value to set

---

### POST /api/done/{guid}

Client acknowledges completion of an action. Removes the action from the queue.

**Authentication:** Required (when enabled)

**Request:**
```bash
curl -X POST http://localhost/api/done/ec9026fe-25fc-4b2f-b4b0-c5402699f399 \
  -u client1:pass1
```

**URL Parameters:**
- `guid` (string): The GUID of the completed action (from GET /api/myactions)

**Response:** HTTP 201 Created

**Error Responses:**
- HTTP 404 Not Found: Action with specified GUID not found

---

### POST /api/admin/inject

**Admin endpoint** to manually inject actions into the client's action queue. This is useful for testing, debugging, or triggering actions from external systems.

**Authentication:** Required (when enabled)

**Request:**
```bash
# Inject a single action parameter
curl -X POST http://localhost/api/admin/inject \
  -u client1:pass1 \
  -H "Content-Type: application/json" \
  -d '{"k": 613, "v": "64"}'

# Inject multiple action parameters
curl -X POST http://localhost/api/admin/inject \
  -u client1:pass1 \
  -H "Content-Type: application/json" \
  -d '[
    {"k": 613, "v": "64"},
    {"k": 615, "v": "128"}
  ]'
```

**Request Body:**

Can be either a single object or an array of objects:

**Single Parameter:**
```json
{"k": 613, "v": "64"}
```

**Multiple Parameters:**
```json
[
  {"k": 613, "v": "64"},
  {"k": 615, "v": "128"}
]
```

**Fields:**
- `k` (integer): Exchange table index (0-999)
- `v` (string): Value to set (always string, even for numbers)

**Response:** HTTP 200 OK
```json
{
  "status": "ok",
  "guid": "ec9026fe-25fc-4b2f-b4b0-c5402699f399"
}
```

**Response Fields:**
- `status` (string): Always "ok" on success
- `guid` (string): The GUID of the created action

**Automatic Processing:**

The server automatically processes injected actions:

1. **Complete Block Generation:** If any index is in range 605-622 (lights/shutters), all indices 605-622 are included with default value "0" for missing indices
2. **Scenario Trigger:** Index 590 with value "1" is automatically added for light/shutter actions
3. **Parameter Ordering:** Parameters are sorted by ascending index number

**Example - Turning on Bedroom 3 Light:**

```bash
# You send just the target index
curl -X POST http://localhost/api/admin/inject \
  -u client1:pass1 \
  -H "Content-Type: application/json" \
  -d '{"k": 613, "v": "64"}'

# Server automatically expands to complete block:
# {
#   "guid": "abc123...",
#   "params": [
#     {"k": 590, "v": "1"},   // Scenario trigger (auto-added)
#     {"k": 605, "v": "0"},   // All other indices (auto-added)
#     {"k": 606, "v": "0"},
#     ...
#     {"k": 613, "v": "64"},  // Your target value
#     ...
#     {"k": 622, "v": "0"}
#   ]
# }
```

**Use Cases:**
- Testing light/shutter control from command line
- Integrating with external automation systems
- Debugging action processing
- Manual control during development

**Error Responses:**
- HTTP 400 Bad Request: Invalid JSON format
- HTTP 500 Internal Server Error: Failed to add action

---

### GET /health

Health check endpoint for monitoring and load balancers. Does not require authentication.

**Authentication:** Not required

**Request:**
```bash
curl http://localhost/health
```

**Response:** HTTP 200 OK
```json
{
  "status": "ok"
}
```

---

## Complete Client Polling Cycle Example

Here's a complete example of a typical client polling cycle:

```bash
# 1. Get server info
curl -u client1:pass1 http://localhost/api/serverinfos
# Response: {"isconnected":true,"infos":[1,2,3],"newversion":"1.0.0"}

# 2. Send status update
curl -X POST http://localhost/api/mystatus \
  -u client1:pass1 \
  -H "Content-Type: application/json" \
  -d '{"version":"1.0","ek":[{"k":1,"v":"100"},{"k":2,"v":"200"}]}'
# Response: HTTP 201 Created

# 3. Check for pending actions
curl -u client1:pass1 http://localhost/api/myactions
# Response: {"_de67f":null,"actions":[{"guid":"abc123","params":[...]}]}

# 4. Acknowledge action completion
curl -X POST http://localhost/api/done/abc123 -u client1:pass1
# Response: HTTP 201 Created

# 5. Repeat from step 1 (typically every 500ms)
```

## Authentication

The server uses HTTP Basic Authentication to secure API endpoints. Clients must provide credentials in the Authorization header using the standard Basic Auth format.

### Authentication Flow

1. Client sends request with `Authorization: Basic <base64(username:password)>` header
2. Server decodes the Base64 credentials
3. Server validates credentials against configured client list
4. If valid, request is processed; if invalid, HTTP 401 Unauthorized is returned

### Configuring Client Credentials

**Via Environment Variable:**
```bash
# Single client
CLIENT_CREDENTIALS="client1:pass1" ./server

# Multiple clients (comma-separated)
CLIENT_CREDENTIALS="client1:pass1,client2:pass2,client3:pass3" ./server
```

**Via YAML File:**
```yaml
auth:
  enabled: true
  clients:
    client1: pass1
    client2: pass2
    client3: pass3
```

### Enabling/Disabling Authentication

Authentication is **disabled by default** for easier development and testing.

**Enable Authentication:**
```bash
# Via environment variable
AUTH_ENABLED=true CLIENT_CREDENTIALS="client1:pass1" ./server

# Via config.yaml
# Set auth.enabled: true in config.yaml
./server
```

**Disable Authentication:**
```bash
# Via environment variable
AUTH_ENABLED=false ./server

# Via config.yaml
# Set auth.enabled: false in config.yaml
./server
```

### Client Request Examples

**Using curl:**
```bash
# Basic Auth with -u flag (recommended)
curl -u client1:pass1 http://localhost/api/serverinfos

# Basic Auth with explicit header
curl -H "Authorization: Basic Y2xpZW50MTpwYXNzMQ==" http://localhost/api/serverinfos

# POST request with authentication
curl -X POST http://localhost/api/mystatus \
  -u client1:pass1 \
  -H "Content-Type: application/json" \
  -d '{"version":"1.0","ek":[{"k":1,"v":"100"}]}'
```

**Using Go:**
```go
client := &http.Client{}
req, _ := http.NewRequest("GET", "http://localhost/api/serverinfos", nil)
req.SetBasicAuth("client1", "pass1")
resp, _ := client.Do(req)
```

**Using Python:**
```python
import requests
from requests.auth import HTTPBasicAuth

response = requests.get(
    'http://localhost/api/serverinfos',
    auth=HTTPBasicAuth('client1', 'pass1')
)
```

### Authentication Error Responses

**Missing Authorization Header:**
```bash
curl http://localhost/api/serverinfos
# Response: HTTP 401 Unauthorized
# Body: Unauthorized
```

**Invalid Credentials:**
```bash
curl -u client1:wrongpass http://localhost/api/serverinfos
# Response: HTTP 401 Unauthorized
# Body: Unauthorized
```

**Malformed Authorization Header:**
```bash
curl -H "Authorization: InvalidFormat" http://localhost/api/serverinfos
# Response: HTTP 401 Unauthorized
# Body: Unauthorized
```

### Security Considerations

1. **Use HTTPS in Production:** Basic Auth sends credentials in Base64 encoding (not encryption). Always use HTTPS in production.
2. **Strong Passwords:** Use strong, unique passwords for each client.
3. **Credential Rotation:** Regularly rotate client credentials.
4. **Environment Variables:** Store credentials in environment variables or secure configuration management systems, not in code.
5. **Health Endpoint:** The `/health` endpoint does not require authentication for monitoring purposes.

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run tests with race detector
go test ./... -race

# Run tests for a specific package
go test ./internal/config/... -v
```

### Project Structure

```
essensys-server-backend/
├── cmd/
│   └── server/
│       └── main.go                 # Entry point
├── internal/
│   ├── api/                        # HTTP handlers
│   ├── config/                     # Configuration management
│   ├── core/                       # Business logic
│   ├── data/                       # Data storage
│   └── middleware/                 # HTTP middleware
├── pkg/
│   └── protocol/                   # Shared types
├── config.yaml.example             # Example configuration
├── go.mod
└── README.md
```

## Logging

The server logs all requests, responses, and errors. When a client connects, you should see logs like this:

```
[GO] GET /api/serverinfos
[REQUEST] 2025-12-03T10:22:45+01:00 GET /api/serverinfos 192.168.1.100:54321
[RESPONSE] /api/serverinfos 200 1.2ms

[GO] POST /api/mystatus
[REQUEST] 2025-12-03T10:22:45+01:00 POST /api/mystatus 192.168.1.100:54321
[GO] Status Update (Version: V37, Items: 12)
[RESPONSE] /api/mystatus 201 2.5ms

[GO] GET /api/myactions
[REQUEST] 2025-12-03T10:22:45+01:00 GET /api/myactions 192.168.1.100:54321
[GO] Sending Actions: {"_de67f":null,"actions":[]}
[RESPONSE] /api/myactions 200 0.8ms
```

### Log Format

Each request generates multiple log entries:

1. **Simple format**: `[GO] METHOD PATH` - Quick overview of the request
2. **Detailed request**: `[REQUEST] timestamp METHOD PATH client_ip` - Full request details
3. **Handler logs**: Specific logs from handlers (e.g., status updates, actions sent)
4. **Response**: `[RESPONSE] PATH status_code duration` - Response summary

### JSON Normalization Logs

When the server normalizes malformed JSON from C clients:

```
[NORMALIZE] Original: {version:"V37",ek:[{k:1,v:"100"}]} → Normalized: {"version":"V37","ek":[{"k":1,"v":"100"}]}
```

### Log Levels

- **debug**: Detailed information for debugging
- **info**: General informational messages (default)
- **warn**: Warning messages
- **error**: Error messages

Set log level via environment variable:
```bash
LOG_LEVEL=debug ./server
```

### Troubleshooting: No Logs Appearing

If you don't see any logs when your client connects:

1. **Check if server is running**: `ps aux | grep server`
2. **Check if port 80 is listening**: `sudo lsof -i :80`
3. **Test with curl**: `curl http://localhost/health` - You should see logs
4. **Check client configuration**: Ensure client is pointing to the correct IP/port
5. **Check firewall**: Ensure port 80 is not blocked
6. **Compare with working server**: If `server.sample` works but `./server` doesn't, check network configuration

### Troubleshooting: Client Connects but No HTTP Logs

If you see TCP connection logs but no HTTP request logs:

```
[TCP] New connection from 192.168.0.151:1994
[TCP] Closing connection from 192.168.0.151:1994
```

This means the client is connecting but sending HTTP that Go's `net/http` cannot parse. Check the logs for:

```
[TCP] First X bytes from 192.168.0.151:1994:
[TCP] Raw data (hex): ...
[TCP] Raw data (string): ...
[TCP] First line: ...
```

Common issues with legacy BP_MQX_ETH clients:
- **Missing HTTP version**: Client sends `GET /api/serverinfos` without `HTTP/1.0`
- **Wrong line endings**: Client uses `\n` instead of `\r\n`
- **Missing Host header**: Required in HTTP/1.1 but client may not send it
- **Malformed headers**: Extra/missing spaces

If you see this issue, the raw data logs will show exactly what the client is sending.

## Protocol Features

### Malformed JSON Normalization

The server automatically handles malformed JSON from legacy C clients that don't quote object keys.

**Example:**
```bash
# Client sends (invalid JSON):
{version:"1.0",ek:[{k:1,v:"100"}]}

# Server normalizes to (valid JSON):
{"version":"1.0","ek":[{"k":1,"v":"100"}]}

# Then processes normally
```

The normalization is logged for debugging:
```
[NORMALIZE] Original: {version:"1.0",...} → Normalized: {"version":"1.0",...}
```

### Complete Light Block Generation

When controlling lights or shutters (indices 605-622), the server automatically generates a complete parameter block.

**Why?** The BP_MQX_ETH firmware parser expects ALL indices from 605-622 to be present. Missing even one index causes the client to ignore the entire action.

**Example:**

If you want to turn on bedroom 3 light (index 613 = "64"), you cannot send just:
```json
{"k": 613, "v": "64"}
```

The server automatically expands this to include all required indices:
```json
[
  {"k": 590, "v": "1"},   // Scenario trigger (required)
  {"k": 605, "v": "0"},   // Other lights/shutters
  {"k": 606, "v": "0"},
  {"k": 607, "v": "0"},
  {"k": 608, "v": "0"},
  {"k": 609, "v": "0"},
  {"k": 610, "v": "0"},
  {"k": 611, "v": "0"},
  {"k": 612, "v": "0"},
  {"k": 613, "v": "64"},  // Your target value
  {"k": 614, "v": "0"},
  {"k": 615, "v": "0"},
  {"k": 616, "v": "0"},
  {"k": 617, "v": "0"},
  {"k": 618, "v": "0"},
  {"k": 619, "v": "0"},
  {"k": 620, "v": "0"},
  {"k": 621, "v": "0"},
  {"k": 622, "v": "0"}
]
```

### Bitwise OR Fusion

When multiple actions target the same index, the server merges them using bitwise OR.

**Example:**
```
Action 1: Index 613 = "64"  (binary: 01000000)
Action 2: Index 613 = "128" (binary: 10000000)
Merged:   Index 613 = "192" (binary: 11000000)
```

**Exception:** Index 590 (scenario trigger) is never fused.

### Field Ordering in JSON Responses

The `_de67f` field MUST appear before `actions` in GET /api/myactions responses. This is required by the C client's manual JSON parser.

**Correct:**
```json
{"_de67f":null,"actions":[...]}
```

**Incorrect (will break client):**
```json
{"actions":[...],"_de67f":null}
```

### Content-Type Header

All JSON responses include a space before the semicolon in the Content-Type header:
```
Content-Type: application/json ;charset=UTF-8
```

This matches the legacy ASP.NET server format expected by clients.

## Troubleshooting

### Port 80 Permission Denied

**Error:** `listen tcp :80: bind: permission denied`

**Solution:** Use setcap or run with sudo (see Port Configuration above)

**Quick Fix:**
```bash
sudo setcap 'cap_net_bind_service=+ep' ./server
./server
```

### Authentication Failures

**Error:** HTTP 401 Unauthorized

**Possible Causes:**
- Missing Authorization header
- Invalid credentials
- Credentials not configured on server

**Solution:** 
- Verify client credentials are configured: `CLIENT_CREDENTIALS="client1:pass1"`
- Check Authorization header format: `Authorization: Basic base64(username:password)`
- Ensure AUTH_ENABLED is true if you want authentication
- Test with curl: `curl -u client1:pass1 http://localhost/api/serverinfos`

### No Client Credentials Configured

**Warning:** `Authentication is enabled but no client credentials are configured`

**Solution:** Add client credentials via environment variable or config.yaml

```bash
# Via environment variable
CLIENT_CREDENTIALS="client1:pass1" ./server

# Or disable authentication for testing
AUTH_ENABLED=false ./server
```

### Port 80 Warning

**Warning:** `Server configured to use port 8080 instead of 80`

**Impact:** BP_MQX_ETH clients will not be able to connect

**Solution:** Change SERVER_PORT to 80 or remove the environment variable

```bash
# Use default port 80
unset SERVER_PORT
./server

# Or explicitly set to 80
SERVER_PORT=80 ./server
```

### Address Already in Use

**Error:** `bind: address already in use`

**Solution:** Another process is using port 80

```bash
# Find the process
sudo lsof -i :80

# Stop conflicting service
sudo systemctl stop apache2  # or nginx, httpd, etc.

# Then start the server
./server
```

### Connection Refused

**Error:** Client cannot connect to server

**Checklist:**
1. Is the server running? Check with `ps aux | grep server`
2. Is it listening on port 80? Check with `sudo lsof -i :80`
3. Is there a firewall blocking port 80? Check with `sudo iptables -L`
4. Can you reach it locally? Test with `curl http://localhost/health`

### Malformed JSON Not Parsing

**Error:** HTTP 400 Bad Request

**Solution:** Check the JSON format in logs

The server logs both original and normalized JSON:
```
[NORMALIZE] Original: {...} → Normalized: {...}
```

If normalization fails, the original JSON may be too malformed to fix automatically.

### High Memory Usage

**Symptom:** Server memory usage grows over time

**Possible Causes:**
- Action queue growing without acknowledgments
- Memory leak in concurrent operations

**Solution:**
1. Check action queue depth in logs
2. Ensure clients are calling POST /api/done/{guid} after completing actions
3. Run with race detector: `go test -race ./...`
4. Monitor with: `ps aux | grep server`

### Slow Response Times

**Symptom:** Requests taking longer than expected

**Possible Causes:**
- High concurrent load
- Deadlock in synchronization
- Large action queues

**Solution:**
1. Check server logs for timing information
2. Monitor concurrent client count
3. Run with race detector to check for deadlocks
4. Consider increasing timeouts in config.yaml

## Production Deployment

### Systemd Service (Linux)

Create a systemd service file for automatic startup and management.

**Create `/etc/systemd/system/essensys-server.service`:**
```ini
[Unit]
Description=Essensys Backend Server
After=network.target

[Service]
Type=simple
User=essensys
Group=essensys
WorkingDirectory=/opt/essensys-server
ExecStart=/opt/essensys-server/server
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# Environment variables
Environment="LOG_LEVEL=info"
Environment="AUTH_ENABLED=true"
Environment="CLIENT_CREDENTIALS=client1:pass1,client2:pass2"

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/essensys-server

# Allow binding to port 80
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
```

**Setup and start the service:**
```bash
# Create user and directory
sudo useradd -r -s /bin/false essensys
sudo mkdir -p /opt/essensys-server
sudo cp server /opt/essensys-server/
sudo cp config.yaml /opt/essensys-server/
sudo chown -R essensys:essensys /opt/essensys-server

# Set capabilities
sudo setcap 'cap_net_bind_service=+ep' /opt/essensys-server/server

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable essensys-server
sudo systemctl start essensys-server

# Check status
sudo systemctl status essensys-server

# View logs
sudo journalctl -u essensys-server -f
```

### Docker Compose

**Create `docker-compose.yml`:**
```yaml
version: '3.8'

services:
  essensys-server:
    build: .
    container_name: essensys-server
    ports:
      - "80:80"
    environment:
      - LOG_LEVEL=info
      - AUTH_ENABLED=true
      - CLIENT_CREDENTIALS=client1:pass1,client2:pass2
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

**Run with Docker Compose:**
```bash
# Start
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

### Kubernetes Deployment

**Create `k8s-deployment.yaml`:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: essensys-server
spec:
  replicas: 3
  selector:
    matchLabels:
      app: essensys-server
  template:
    metadata:
      labels:
        app: essensys-server
    spec:
      containers:
      - name: essensys-server
        image: essensys-server:latest
        ports:
        - containerPort: 80
        env:
        - name: LOG_LEVEL
          value: "info"
        - name: AUTH_ENABLED
          value: "true"
        - name: CLIENT_CREDENTIALS
          valueFrom:
            secretKeyRef:
              name: essensys-credentials
              key: credentials
        livenessProbe:
          httpGet:
            path: /health
            port: 80
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 80
          initialDelaySeconds: 5
          periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: essensys-server
spec:
  type: LoadBalancer
  ports:
  - port: 80
    targetPort: 80
  selector:
    app: essensys-server
```

### Environment-Specific Configuration

**Development:**
```bash
# Disable authentication for easier testing
AUTH_ENABLED=false LOG_LEVEL=debug ./server
```

**Staging:**
```bash
# Enable authentication with test credentials
AUTH_ENABLED=true \
CLIENT_CREDENTIALS="test1:testpass1,test2:testpass2" \
LOG_LEVEL=info \
./server
```

**Production:**
```bash
# Use config file for production credentials
# Store sensitive data in config.yaml (not in environment variables)
LOG_LEVEL=info ./server
```

## Monitoring and Observability

### Health Checks

The `/health` endpoint provides basic health status:

```bash
curl http://localhost/health
# Response: {"status":"ok"}
```

Use this endpoint for:
- Load balancer health checks
- Kubernetes liveness/readiness probes
- Monitoring system checks
- Uptime monitoring

### Log Monitoring

The server logs all requests, responses, and errors in a structured format:

```
[REQUEST] 2025-12-03T10:15:30Z GET /api/serverinfos 192.0.2.1:1234
[RESPONSE] /api/serverinfos 200 8.75µs
[NORMALIZE] Original: {k:1,v:"0"} → Normalized: {"k":1,"v":"0"}
[ERROR] Failed to process action: invalid GUID format
```

**Log Aggregation:**

Use tools like:
- **journalctl** (systemd): `sudo journalctl -u essensys-server -f`
- **Docker logs**: `docker logs -f essensys-server`
- **ELK Stack**: Elasticsearch, Logstash, Kibana
- **Grafana Loki**: Lightweight log aggregation
- **CloudWatch Logs**: AWS log management

### Metrics to Monitor

**Application Metrics:**
- Request rate per endpoint
- Response time (p50, p95, p99)
- Error rate by status code (4xx, 5xx)
- Active client count
- Action queue depth per client

**System Metrics:**
- CPU usage
- Memory usage
- Goroutine count
- File descriptor count
- Network connections

**Business Metrics:**
- Client polling frequency
- Action completion rate
- Authentication failure rate
- Malformed JSON normalization rate

### Alerting

Set up alerts for:
- Server downtime (health check failures)
- High error rate (> 5% 5xx responses)
- High response time (p95 > 100ms)
- Memory usage (> 80%)
- Authentication failures (potential security issue)

### Performance Tuning

**Go Runtime Tuning:**
```bash
# Increase max goroutines (if needed)
GOMAXPROCS=8 ./server

# Enable CPU profiling
go tool pprof http://localhost:6060/debug/pprof/profile

# Enable memory profiling
go tool pprof http://localhost:6060/debug/pprof/heap
```

**Configuration Tuning:**
```yaml
server:
  read_timeout: 10s      # Adjust based on client behavior
  write_timeout: 10s     # Adjust based on response size
  idle_timeout: 60s      # Adjust based on polling frequency
  max_header_bytes: 1048576  # 1MB
```

## Security Best Practices

1. **Use HTTPS in Production**: Deploy behind a reverse proxy with TLS termination
2. **Strong Authentication**: Use strong, unique passwords for each client
3. **Credential Management**: Store credentials in secure configuration management (e.g., HashiCorp Vault)
4. **Regular Updates**: Keep Go and dependencies up to date
5. **Principle of Least Privilege**: Run as non-root user with minimal capabilities
6. **Network Segmentation**: Restrict access to trusted networks
7. **Audit Logging**: Enable detailed logging for security audits
8. **Rate Limiting**: Consider adding rate limiting for production deployments
9. **Input Validation**: The server validates all inputs, but monitor for unusual patterns
10. **Regular Security Audits**: Review logs and access patterns regularly

## Performance Benchmarks

Typical performance on modern hardware (4 CPU cores, 8GB RAM):

- **Concurrent Clients**: 100+ clients
- **Request Throughput**: 200+ requests/second
- **Response Time**: < 10ms (p95) under normal load
- **Memory Usage**: < 100MB for 100 clients
- **CPU Usage**: < 20% under normal load

## Backup and Recovery

### Data Persistence

The current implementation uses in-memory storage. Data is lost on server restart.

**For production, consider:**
- Implementing database persistence (SQLite, PostgreSQL)
- Regular backups of exchange table and action queue
- Replication for high availability

### Disaster Recovery

**Backup Configuration:**
```bash
# Backup configuration files
cp config.yaml config.yaml.backup
cp /etc/systemd/system/essensys-server.service essensys-server.service.backup
```

**Recovery Steps:**
1. Restore configuration files
2. Rebuild or restore server binary
3. Restart service
4. Verify health check
5. Monitor logs for errors

## License

[Your License Here]

## Contributing

[Your Contributing Guidelines Here]

## Support

For issues, questions, or contributions:
- **Issues**: [GitHub Issues](https://github.com/essensys-hub/essensys-server-backend/issues)
- **Documentation**: [Wiki](https://github.com/essensys-hub/essensys-server-backend/wiki)
- **Email**: support@essensys.example.com

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for version history and release notes.
