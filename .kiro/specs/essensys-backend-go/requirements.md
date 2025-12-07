# Requirements Document

## Introduction

This document specifies the requirements for the Essensys Backend Server implemented in Go. The server acts as a gateway between BP_MQX_ETH firmware clients (embedded C devices) and modern web interfaces. It must maintain 100% protocol compatibility with the legacy ASP.NET server while providing a robust, concurrent, and maintainable architecture.

## Glossary

- **BP_MQX_ETH**: Embedded firmware client running on home automation controllers
- **Table d'Échange**: Exchange table - a key-value store mapping indices (0-999) to string values
- **Action Queue**: FIFO queue storing pending actions to be executed by clients
- **Scenario Index**: Index 590 in the Table d'Échange, must be set to "1" to trigger action execution
- **Light/Shutter Range**: Indices 605-622 in the Table d'Échange controlling lights and shutters
- **GUID**: Globally Unique Identifier used for action acknowledgment
- **Bitwise Fusion**: Binary OR operation applied when merging multiple actions on the same index
- **Malformed JSON**: Non-standard JSON sent by C client with unquoted keys (e.g., `{k:1,v:"0"}`)

## Requirements

### Requirement 1: HTTP API Compatibility

**User Story:** As a BP_MQX_ETH client, I want to communicate with the server using the exact same protocol as the legacy ASP.NET server, so that I can operate without firmware modifications.

#### Acceptance Criteria

1. WHEN the server starts THEN the Server SHALL listen on port 80 (standard HTTP port)
2. WHEN a client sends GET /api/serverinfos THEN the Server SHALL respond with HTTP 200 and JSON containing fields "isconnected" (boolean), "infos" (array of integers), and "newversion" (string)
3. WHEN a client sends POST /api/mystatus with malformed JSON THEN the Server SHALL normalize the JSON by adding quotes to unquoted keys before parsing
4. WHEN a client sends POST /api/mystatus with valid status data THEN the Server SHALL respond with HTTP 201 Created
5. WHEN a client sends GET /api/myactions THEN the Server SHALL respond with HTTP 200 and JSON where field "_de67f" appears before field "actions"
6. WHEN a client sends POST /api/done/{guid} THEN the Server SHALL remove the action with matching GUID from the Action Queue and respond with HTTP 201 Created

### Requirement 2: Exchange Table Management

**User Story:** As a system administrator, I want the server to maintain a thread-safe exchange table for each client, so that concurrent requests do not corrupt data.

#### Acceptance Criteria

1. WHEN multiple goroutines read from the Table d'Échange simultaneously THEN the Server SHALL use sync.RWMutex to prevent race conditions
2. WHEN a client updates an index via POST /api/mystatus THEN the Server SHALL store the value as a string in the Table d'Échange
3. WHEN the server receives a status update for an existing index THEN the Server SHALL overwrite the previous value
4. WHEN the server queries a non-existent index THEN the Server SHALL return an empty string or default value
5. WHEN the server stores values THEN the Server SHALL support indices from 0 to 999

### Requirement 3: Action Queue Management

**User Story:** As a web interface, I want to queue actions for clients, so that I can remotely control devices.

#### Acceptance Criteria

1. WHEN an action is added to the Action Queue THEN the Server SHALL assign a unique GUID to that action
2. WHEN a client requests GET /api/myactions THEN the Server SHALL return all pending actions in FIFO order
3. WHEN a client acknowledges an action via POST /api/done/{guid} THEN the Server SHALL remove only the action with the matching GUID
4. WHEN multiple actions target the same index THEN the Server SHALL apply bitwise OR fusion to merge the values before queuing
5. WHEN an action targets indices in the Light/Shutter Range (605-622) THEN the Server SHALL automatically generate a complete parameter block including all indices from 605 to 622 with default value "0" for missing indices

### Requirement 4: Complete Action Block Generation

**User Story:** As a BP_MQX_ETH client, I want to receive complete action blocks with all required indices, so that I can execute actions without parsing errors.

**Context:** The BP_MQX_ETH firmware uses a manual string-based JSON parser that expects ALL indices from 605 to 622 to be present in the params array. This is not optional - if even one index is missing, the client will ignore the entire action. For example, to turn on bedroom 3 light (index 613 = "64"), you cannot send just `[{"k":613,"v":"64"}]`. You MUST send the complete block with all 18 indices (605-622) plus the scenario trigger (590).

#### Acceptance Criteria

1. WHEN an action contains any index between 605 and 622 THEN the Server SHALL include all indices from 605 to 622 in the params array
2. WHEN an action contains indices in the Light/Shutter Range THEN the Server SHALL include index 590 with value "1" in the params array
3. WHEN an action specifies index 613 with value "64" THEN the Server SHALL include indices 605-612 and 614-622 with value "0"
4. WHEN generating the complete block THEN the Server SHALL preserve the original values for explicitly specified indices
5. WHEN generating the complete block THEN the Server SHALL order parameters by ascending index number

### Requirement 5: JSON Response Format Compliance

**User Story:** As a BP_MQX_ETH client with a manual JSON parser, I want responses in a strict format, so that my string-based parser does not crash.

#### Acceptance Criteria

1. WHEN the server responds to GET /api/myactions THEN the Server SHALL place the "_de67f" field before the "actions" field in the JSON output
2. WHEN the "_de67f" field is null THEN the Server SHALL serialize it as `"_de67f":null` not as an omitted field
3. WHEN serializing action parameters THEN the Server SHALL format each parameter as `{"k":<int>,"v":"<string>"}`
4. WHEN the actions array is empty THEN the Server SHALL return `{"_de67f":null,"actions":[]}`
5. WHEN responding to POST /api/mystatus THEN the Server SHALL set Content-Type header to "application/json ;charset=UTF-8" with space before semicolon

### Requirement 6: Malformed JSON Normalization

**User Story:** As a server, I want to accept malformed JSON from legacy C clients, so that I maintain backward compatibility.

#### Acceptance Criteria

1. WHEN the server receives JSON with unquoted keys like `{k:1,v:"0"}` THEN the Server SHALL transform it to `{"k":1,"v":"0"}` before unmarshaling
2. WHEN the server receives JSON with unquoted numeric values THEN the Server SHALL preserve the numeric format during normalization
3. WHEN the server receives JSON with quoted string values THEN the Server SHALL preserve the quotes during normalization
4. WHEN normalization fails THEN the Server SHALL return HTTP 400 Bad Request
5. WHEN normalization succeeds THEN the Server SHALL process the request normally and return HTTP 201 Created

### Requirement 7: Concurrent Request Handling

**User Story:** As a system with multiple clients polling every 500ms, I want the server to handle concurrent requests efficiently, so that response times remain low.

#### Acceptance Criteria

1. WHEN multiple clients send requests simultaneously THEN the Server SHALL handle each request in a separate goroutine
2. WHEN accessing shared data structures THEN the Server SHALL use appropriate synchronization primitives (sync.RWMutex, sync.Mutex)
3. WHEN a client polls GET /api/myactions THEN the Server SHALL respond within 100ms under normal load
4. WHEN the server experiences high load THEN the Server SHALL not crash or deadlock
5. WHEN a goroutine panics THEN the Server SHALL recover and log the error without terminating the server process

### Requirement 8: Authentication

**User Story:** As a security-conscious administrator, I want clients to authenticate using Basic Auth, so that unauthorized devices cannot access the system.

#### Acceptance Criteria

1. WHEN a client sends a request without an Authorization header THEN the Server SHALL respond with HTTP 401 Unauthorized
2. WHEN a client sends a request with valid Basic Auth credentials THEN the Server SHALL process the request normally
3. WHEN a client sends a request with invalid credentials THEN the Server SHALL respond with HTTP 401 Unauthorized
4. WHEN the server validates credentials THEN the Server SHALL decode the Base64 Authorization header
5. WHEN the server stores credentials THEN the Server SHALL support multiple client matricules (MAC address + server key)

### Requirement 9: Logging and Observability

**User Story:** As a developer debugging protocol issues, I want detailed request/response logs, so that I can trace client behavior.

#### Acceptance Criteria

1. WHEN the server receives any HTTP request THEN the Server SHALL log the method, path, client IP, and timestamp
2. WHEN the server sends an HTTP response THEN the Server SHALL log the status code and response time
3. WHEN the server normalizes malformed JSON THEN the Server SHALL log both the original and normalized JSON
4. WHEN an error occurs THEN the Server SHALL log the error message and stack trace
5. WHEN the server starts THEN the Server SHALL log the listening port and configuration

### Requirement 10: Bitwise Action Fusion

**User Story:** As a system handling multiple simultaneous light commands, I want actions on the same index to be merged using bitwise OR, so that multiple commands combine correctly.

#### Acceptance Criteria

1. WHEN two actions target the same index with values "64" and "128" THEN the Server SHALL merge them to "192" using bitwise OR
2. WHEN merging actions THEN the Server SHALL parse string values as integers, apply OR operation, and convert back to string
3. WHEN an action targets index 590 (Scenario) THEN the Server SHALL not apply bitwise fusion
4. WHEN merging fails due to non-numeric values THEN the Server SHALL use the most recent value
5. WHEN only one action targets an index THEN the Server SHALL use that value without modification
