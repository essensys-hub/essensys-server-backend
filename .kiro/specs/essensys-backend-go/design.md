# Design Document

## Overview

The Essensys Backend Server is a Go-based HTTP gateway that bridges legacy BP_MQX_ETH embedded clients with modern web interfaces. The architecture follows Go's standard project layout with clear separation between HTTP handlers, business logic, and data storage. The server must handle high-frequency polling (every 500ms) from multiple clients while maintaining strict protocol compatibility with the legacy ASP.NET implementation.

## Architecture

### High-Level Architecture

```
┌─────────────────┐         ┌──────────────────────────────────┐
│  BP_MQX_ETH     │◄───────►│   Essensys Backend Server (Go)   │
│  Client (C)     │  HTTP   │                                  │
│  Polling 500ms  │         │  ┌────────────────────────────┐  │
└─────────────────┘         │  │   HTTP Layer (Handlers)    │  │
                            │  └────────────┬───────────────┘  │
┌─────────────────┐         │               │                  │
│  Web Interface  │◄───────►│  ┌────────────▼───────────────┐  │
│  (Future)       │ WebSocket│  │   Business Logic (Core)    │  │
└─────────────────┘         │  └────────────┬───────────────┘  │
                            │               │                  │
                            │  ┌────────────▼───────────────┐  │
                            │  │   Data Layer (Store)       │  │
                            │  │   - Exchange Table         │  │
                            │  │   - Action Queue           │  │
                            │  └────────────────────────────┘  │
                            └──────────────────────────────────┘
```

### Project Structure

```
essensys-server-backend/
├── cmd/
│   └── server/
│       └── main.go                 # Entry point, server initialization
├── internal/
│   ├── api/                        # HTTP layer
│   │   ├── handlers.go             # HTTP request handlers
│   │   ├── router.go               # Route definitions
│   │   └── json_normalizer.go     # Malformed JSON fixer
│   ├── core/                       # Business logic
│   │   ├── action_service.go      # Action queue & bitwise fusion
│   │   └── status_service.go      # Exchange table operations
│   ├── data/                       # Data access
│   │   └── memory_store.go        # Thread-safe in-memory storage
│   └── middleware/                 # HTTP middleware
│       ├── auth.go                 # Basic authentication
│       ├── logging.go              # Request/response logging
│       └── recovery.go             # Panic recovery
├── pkg/
│   └── protocol/                   # Shared types
│       ├── types.go                # JSON structs
│       └── constants.go            # Protocol constants
├── go.mod
└── README.md
```

## Components and Interfaces

### 1. Protocol Package (`pkg/protocol`)

Defines all data structures and constants used across the application.

**Constants:**
```go
const (
    IndexScenario     = 590  // Scenario trigger index
    IndexLightStart   = 605  // First light/shutter index
    IndexLightEnd     = 622  // Last light/shutter index
    MaxExchangeIndex  = 999  // Maximum valid index
)
```

**Types:**
```go
// ServerInfoResponse - Response for GET /api/serverinfos
type ServerInfoResponse struct {
    IsConnected bool   `json:"isconnected"`
    Infos       []int  `json:"infos"`
    NewVersion  string `json:"newversion"`
}

// StatusRequest - Request body for POST /api/mystatus
type StatusRequest struct {
    Version string       `json:"version"`
    EK      []ExchangeKV `json:"ek"`
}

// ExchangeKV - Key-value pair in exchange table
type ExchangeKV struct {
    K int    `json:"k"` // Index
    V string `json:"v"` // Value (always string)
}

// ActionsResponse - Response for GET /api/myactions
type ActionsResponse struct {
    De67f   *AlarmCommand `json:"_de67f"` // Must be first field
    Actions []Action      `json:"actions"`
}

// Action - Single action to execute
type Action struct {
    GUID   string       `json:"guid"`
    Params []ExchangeKV `json:"params"`
}

// AlarmCommand - Encrypted alarm command (optional)
type AlarmCommand struct {
    GUID string `json:"guid"`
    OBL  string `json:"obl"` // AES encrypted data
}
```

### 2. Data Layer (`internal/data`)

**MemoryStore Interface:**
```go
type Store interface {
    // Exchange Table operations
    GetValue(clientID string, index int) (string, bool)
    SetValue(clientID string, index int, value string)
    GetAllValues(clientID string, indices []int) []ExchangeKV
    
    // Action Queue operations
    EnqueueAction(clientID string, action Action)
    DequeueActions(clientID string) []Action
    AcknowledgeAction(clientID string, guid string) bool
    
    // Client management
    IsClientConnected(clientID string) bool
    SetClientConnected(clientID string, connected bool)
}
```

**Implementation Details:**
- Uses `sync.RWMutex` for exchange table (many readers, few writers)
- Uses `sync.Mutex` for action queue (FIFO operations)
- Stores data per client using `map[string]*ClientData`
- ClientData contains: exchange table (map[int]string), action queue ([]Action), connection status

### 3. Business Logic (`internal/core`)

**ActionService:**
```go
type ActionService struct {
    store Store
}

// AddAction adds an action to the queue with proper processing
func (s *ActionService) AddAction(clientID string, params []ExchangeKV) (string, error)

// ProcessAction applies bitwise fusion and generates complete blocks
func (s *ActionService) ProcessAction(params []ExchangeKV) []ExchangeKV

// BitwiseFusion merges multiple values for the same index using OR
func (s *ActionService) BitwiseFusion(existing, new string) string

// GenerateCompleteBlock ensures all indices 605-622 are present
// CRITICAL: This function MUST generate ALL indices from 605 to 622
// Example: To change index 613 to "64" (bedroom 3 light), you MUST provide:
//   - Index 590 = "1" (scenario trigger)
//   - Index 605-612 = "0" (other lights/shutters)
//   - Index 613 = "64" (bedroom 3 light - your target)
//   - Index 614-622 = "0" (other lights/shutters)
// The client will IGNORE the action if ANY index from 605-622 is missing!
func (s *ActionService) GenerateCompleteBlock(params []ExchangeKV) []ExchangeKV
```

**StatusService:**
```go
type StatusService struct {
    store Store
}

// UpdateStatus processes status updates from client
func (s *StatusService) UpdateStatus(clientID string, status StatusRequest) error

// GetRequestedIndices returns indices the server wants from client
func (s *StatusService) GetRequestedIndices(clientID string) []int
```

### 4. HTTP Layer (`internal/api`)

**Handlers:**
```go
type Handler struct {
    actionService *core.ActionService
    statusService *core.StatusService
    store         data.Store
}

func (h *Handler) GetServerInfos(w http.ResponseWriter, r *http.Request)
func (h *Handler) PostMyStatus(w http.ResponseWriter, r *http.Request)
func (h *Handler) GetMyActions(w http.ResponseWriter, r *http.Request)
func (h *Handler) PostDone(w http.ResponseWriter, r *http.Request)
```

**JSON Normalizer:**
```go
// NormalizeJSON converts malformed JSON to valid JSON
// Input:  {k:1,v:"0"}
// Output: {"k":1,"v":"0"}
func NormalizeJSON(input []byte) ([]byte, error)
```

Uses regex patterns:
- `{k:` → `{"k":`
- `,v:` → `,"v":`
- Preserves quoted strings
- Handles nested structures

### 5. Middleware (`internal/middleware`)

**Authentication:**
```go
func BasicAuth(next http.Handler) http.Handler
```
- Extracts Authorization header
- Decodes Base64 credentials
- Validates against configured matricules
- Sets clientID in request context

**Logging:**
```go
func RequestLogger(next http.Handler) http.Handler
```
- Logs: timestamp, method, path, client IP, status code, duration
- Logs malformed JSON normalization

**Recovery:**
```go
func Recovery(next http.Handler) http.Handler
```
- Catches panics in handlers
- Logs stack trace
- Returns HTTP 500 without crashing server

## Data Models

### Exchange Table Structure

```go
type ExchangeTable struct {
    mu     sync.RWMutex
    values map[int]string // index -> value
}
```

**Characteristics:**
- Indices: 0-999
- Values: Always stored as strings
- Thread-safe: RWMutex allows concurrent reads
- Default: Empty string for non-existent indices

### Action Queue Structure

```go
type ActionQueue struct {
    mu      sync.Mutex
    actions []Action // FIFO queue
}
```

**Characteristics:**
- FIFO ordering
- Each action has unique GUID
- Actions removed only on acknowledgment
- Thread-safe: Mutex for queue operations

### Client Data Structure

```go
type ClientData struct {
    ExchangeTable *ExchangeTable
    ActionQueue   *ActionQueue
    IsConnected   bool
    LastSeen      time.Time
}
```

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system-essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: ServerInfo Response Structure
*For any* server state, when responding to GET /api/serverinfos, the response SHALL contain exactly three fields: "isconnected" (boolean), "infos" (array of integers), and "newversion" (string).
**Validates: Requirements 1.1**

### Property 2: JSON Normalization Correctness
*For any* malformed JSON with unquoted keys (e.g., `{k:1,v:"0"}`), the normalization function SHALL produce valid JSON with quoted keys (e.g., `{"k":1,"v":"0"}`) that can be successfully unmarshaled.
**Validates: Requirements 1.2, 6.1**

### Property 3: MyActions Field Ordering
*For any* actions response, the serialized JSON string SHALL contain the substring `"_de67f"` before the substring `"actions"`.
**Validates: Requirements 1.4, 5.1**

### Property 4: Action Acknowledgment Removes Correct Action
*For any* action queue with multiple actions, when acknowledging an action by GUID, only the action with the matching GUID SHALL be removed from the queue.
**Validates: Requirements 1.5, 3.3**

### Property 5: Exchange Table Thread Safety
*For any* sequence of concurrent read and write operations on the exchange table, the Go race detector SHALL not report any data races.
**Validates: Requirements 2.1, 7.2**

### Property 6: Exchange Table Update Overwrites
*For any* index and two different values, when setting the index to the first value then setting it to the second value, retrieving the index SHALL return the second value.
**Validates: Requirements 2.2, 2.3**

### Property 7: Valid Index Range
*For any* index value, the server SHALL accept indices from 0 to 999 and reject indices outside this range.
**Validates: Requirements 2.5**

### Property 8: GUID Uniqueness
*For any* sequence of action additions, all generated GUIDs SHALL be unique.
**Validates: Requirements 3.1**

### Property 9: FIFO Action Queue Ordering
*For any* sequence of actions added to the queue, when retrieving actions, they SHALL be returned in the same order they were added.
**Validates: Requirements 3.2**

### Property 10: Bitwise OR Fusion
*For any* two numeric string values targeting the same index, the merged value SHALL equal the bitwise OR of the two values converted to integers.
**Validates: Requirements 3.4, 10.1, 10.2**

### Property 11: Complete Light Block Generation
*For any* action containing at least one index in the range 605-622, the processed action SHALL contain all indices from 605 to 622.

**Critical Implementation Note:** The BP_MQX_ETH client uses a manual string parser that expects a complete block. If you want to change only index 613 to "64" (bedroom 3 light), the server MUST generate a params array with:
- Index 590 = "1" (scenario trigger - required to execute)
- Indices 605-612 = "0" (all other lights before target)
- Index 613 = "64" (your target value)
- Indices 614-622 = "0" (all other lights after target)

Missing even one index will cause the client to ignore the entire action.

**Complete Example Response:**
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

**Validates: Requirements 3.5, 4.1**

### Property 12: Scenario Index Auto-Addition
*For any* action containing indices in the range 605-622, the processed action SHALL include index 590 with value "1".
**Validates: Requirements 4.2**

### Property 13: Explicit Values Preserved in Complete Block
*For any* action with explicit values for indices in range 605-622, those explicit values SHALL appear unchanged in the complete block.
**Validates: Requirements 4.4**

### Property 14: Parameter Ordering by Index
*For any* action parameters, the output SHALL be ordered by ascending index number.
**Validates: Requirements 4.5**

### Property 15: Null Field Serialization
*For any* actions response where _de67f is null, the serialized JSON SHALL contain the literal string `"_de67f":null`.
**Validates: Requirements 5.2**

### Property 16: Parameter Format Consistency
*For any* action parameter, the serialized JSON SHALL match the pattern `{"k":<integer>,"v":"<string>"}`.
**Validates: Requirements 5.3**

### Property 17: Numeric Value Preservation in Normalization
*For any* malformed JSON containing unquoted numeric values, normalization SHALL preserve the numeric format without adding quotes.
**Validates: Requirements 6.2**

### Property 18: String Value Preservation in Normalization
*For any* malformed JSON containing quoted string values, normalization SHALL preserve the quotes around strings.
**Validates: Requirements 6.3**

### Property 19: Server Resilience Under Load
*For any* sequence of concurrent requests from multiple clients, the server SHALL not deadlock or crash.
**Validates: Requirements 7.4**

### Property 20: Panic Recovery
*For any* handler that panics, the server SHALL recover, log the error, and continue serving subsequent requests.
**Validates: Requirements 7.5**

### Property 21: Valid Authentication Success
*For any* request with valid Basic Auth credentials, the server SHALL process the request and return a success status code (200 or 201).
**Validates: Requirements 8.2**

### Property 22: Base64 Credential Decoding
*For any* valid Base64-encoded Authorization header, the server SHALL successfully decode it to extract client credentials.
**Validates: Requirements 8.4**

### Property 23: Multi-Client Authentication Support
*For any* set of valid client matricules, the server SHALL successfully authenticate requests from all clients.
**Validates: Requirements 8.5**

### Property 24: Request Logging Completeness
*For any* HTTP request, the log entry SHALL contain method, path, client IP, and timestamp.
**Validates: Requirements 9.1**

### Property 25: Response Logging Completeness
*For any* HTTP response, the log entry SHALL contain status code and response time.
**Validates: Requirements 9.2**

### Property 26: JSON Normalization Logging
*For any* malformed JSON that is normalized, the log SHALL contain both the original and normalized JSON.
**Validates: Requirements 9.3**

### Property 27: Error Logging Completeness
*For any* error that occurs, the log entry SHALL contain the error message and stack trace.
**Validates: Requirements 9.4**

### Property 28: Scenario Index Fusion Exception
*For any* actions targeting index 590 (Scenario), bitwise fusion SHALL not be applied.
**Validates: Requirements 10.3**

### Property 29: Fusion Fallback for Non-Numeric Values
*For any* two non-numeric string values targeting the same index, the merged value SHALL be the most recent value.
**Validates: Requirements 10.4**

### Property 30: Single Action No Fusion
*For any* single action targeting an index, the value SHALL remain unchanged (no fusion applied).
**Validates: Requirements 10.5**



## Error Handling

### HTTP Error Responses

**400 Bad Request:**
- Malformed JSON that cannot be normalized
- Invalid request body structure
- Missing required fields

**401 Unauthorized:**
- Missing Authorization header
- Invalid Basic Auth credentials
- Malformed Authorization header

**404 Not Found:**
- Invalid endpoint path
- Action GUID not found in POST /api/done/{guid}

**500 Internal Server Error:**
- Unhandled panic (after recovery)
- Database/storage errors
- Unexpected system errors

### Error Logging Strategy

All errors are logged with:
- Timestamp (RFC3339 format)
- Error level (ERROR, WARN, INFO)
- Request context (method, path, client ID)
- Error message and stack trace
- Original request data (for debugging)

### Graceful Degradation

**Storage Failures:**
- Return empty data rather than crashing
- Log error and continue serving other clients

**JSON Parsing Failures:**
- Attempt normalization first
- Return 400 with descriptive error message
- Log original and attempted normalized JSON

**Concurrent Access:**
- Use timeouts on mutex acquisition
- Detect and log potential deadlocks
- Implement circuit breaker for repeated failures

## Testing Strategy

### Unit Testing

**Framework:** Go's built-in `testing` package

**Coverage Areas:**
- JSON normalization logic (malformed → valid)
- Bitwise OR fusion calculations
- Complete block generation (605-622 + 590)
- GUID generation and uniqueness
- Exchange table operations
- Action queue FIFO behavior

**Example Unit Tests:**
```go
func TestNormalizeJSON_UnquotedKeys(t *testing.T)
func TestBitwiseFusion_TwoValues(t *testing.T)
func TestGenerateCompleteBlock_PartialInput(t *testing.T)
func TestActionQueue_FIFOOrder(t *testing.T)
```

### Property-Based Testing

**Framework:** [gopter](https://github.com/leanovate/gopter) - Go property testing library

**Configuration:**
- Minimum 100 iterations per property test
- Use seed for reproducibility
- Shrinking enabled for counterexample minimization

**Property Test Tagging:**
Each property-based test MUST include a comment with this format:
```go
// **Feature: essensys-backend-go, Property X: [property description]**
```

**Key Properties to Test:**

1. **JSON Normalization Round-Trip:**
   - Generate malformed JSON → normalize → unmarshal → verify structure

2. **Bitwise Fusion Commutativity:**
   - For any two values A and B: fusion(A, B) = fusion(B, A)

3. **Complete Block Invariant:**
   - For any input with indices in 605-622: output contains all indices 605-622

4. **FIFO Ordering:**
   - For any sequence of actions: output order = input order

5. **Concurrent Safety:**
   - For any concurrent read/write operations: no race conditions detected

6. **Authentication Idempotence:**
   - For any valid credentials: authenticate(creds) always succeeds

**Generator Strategies:**

```go
// Generate malformed JSON with unquoted keys
func GenMalformedJSON() gopter.Gen

// Generate valid exchange indices (0-999)
func GenExchangeIndex() gopter.Gen

// Generate action parameters with random indices
func GenActionParams() gopter.Gen

// Generate concurrent operation sequences
func GenConcurrentOps() gopter.Gen
```

### Integration Testing

**Test Server Setup:**
- Start HTTP server on random port
- Use httptest.Server for isolated testing
- Mock authentication for test clients

**Test Scenarios:**
1. Full client polling cycle (serverinfos → mystatus → myactions → done)
2. Multiple clients polling simultaneously
3. Action queue with multiple pending actions
4. Malformed JSON handling end-to-end
5. Authentication failure scenarios

### Concurrency Testing

**Race Detector:**
```bash
go test -race ./...
```

**Stress Testing:**
- 100 concurrent clients polling every 10ms
- Verify no deadlocks after 10,000 requests
- Monitor memory usage for leaks

### Manual Testing with Legacy Client

**Test Script:** `test_chb3.py`
- Validates protocol compliance
- Tests malformed JSON acceptance
- Verifies field ordering in responses
- Checks complete block generation

## Performance Considerations

### Optimization Strategies

**1. Read-Heavy Workload:**
- Use `sync.RWMutex` for exchange table
- Multiple clients can read simultaneously
- Writes are infrequent (only on status updates)

**2. Memory Efficiency:**
- Store values as strings (no conversion overhead)
- Reuse buffers for JSON marshaling
- Limit action queue size per client (max 100 actions)

**3. Response Time:**
- Pre-compute complete blocks when action is queued
- Cache serialized JSON for empty action responses
- Use connection pooling for future database integration

### Scalability Targets

- **Concurrent Clients:** 100+ clients
- **Polling Frequency:** 500ms per client (2 req/sec/client)
- **Total Throughput:** 200+ req/sec
- **Response Time:** < 10ms (p95) under normal load
- **Memory Usage:** < 100MB for 100 clients

## Deployment Considerations

### Port Configuration

**CRITICAL REQUIREMENT: Port 80 is MANDATORY**

The BP_MQX_ETH client firmware is hardcoded to connect to port 80. The server MUST listen on port 80 - this is not configurable.

**Port 80 Setup:**
- **Linux/macOS:** Requires root privileges or capability configuration
- **Run as root:** `sudo ./server` (not recommended for production)
- **Use setcap (recommended):**
  ```bash
  sudo setcap 'cap_net_bind_service=+ep' /path/to/server
  ./server  # Can now bind to port 80 without root
  ```
- **Docker:** Map container port 80 to host port 80
  ```bash
  docker run -p 80:80 essensys-server
  ```

**Alternative (NOT recommended):** Use a reverse proxy (nginx/Apache) to forward port 80 to a higher port, but this adds complexity and potential latency.

### Configuration

**Environment Variables:**
```bash
SERVER_PORT=80
LOG_LEVEL=info
AUTH_ENABLED=true
CLIENT_CREDENTIALS=client1:pass1,client2:pass2
```

**Configuration File:** `config.yaml`
```yaml
server:
  port: 80  # MANDATORY - Client firmware is hardcoded to port 80
  read_timeout: 10s
  write_timeout: 10s
  
logging:
  level: info
  format: json
  
auth:
  enabled: true
  clients:
    - matricule: "123456789abcdef"
      key: "fedcba0987654321"
```

### Monitoring

**Metrics to Track:**
- Request rate per endpoint
- Response time percentiles (p50, p95, p99)
- Error rate by status code
- Active client count
- Action queue depth per client
- Memory and CPU usage

**Health Check Endpoint:**
```
GET /health
Response: {"status": "ok", "uptime": "24h", "clients": 5}
```

## Future Enhancements

### Phase 2: Database Persistence
- Replace in-memory store with SQLite/PostgreSQL
- Persist exchange table and action queue
- Support server restarts without data loss

### Phase 3: WebSocket Support
- Real-time notifications to web interface
- Bidirectional communication
- Reduce polling overhead

### Phase 4: Firmware Update Support
- Implement `/api/getversioncontent/{index}` endpoint
- Implement `/api/endversioncontent` endpoint
- Support chunked firmware downloads
- Version management and rollback

### Phase 5: Alarm Command Support
- Implement AES decryption for `_de67f.obl` field
- Support ALARMEON/ALARMEOFF commands
- Secure key storage and rotation

### Phase 6: Advanced Features
- Multi-tenancy support
- Role-based access control
- Audit logging
- Metrics dashboard
- Alerting and notifications
