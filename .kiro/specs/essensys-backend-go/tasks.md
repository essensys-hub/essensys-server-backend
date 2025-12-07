# Implementation Plan

- [x] 1. Initialize Go project structure
  - Create directory structure following Go standard layout
  - Initialize go.mod with module path `github.com/essensys-hub/essensys-server-backend`
  - Create placeholder files for all packages (protocol, data, core, api, middleware)
  - _Requirements: 1.1_

- [x] 2. Implement protocol package types and constants
  - Define all JSON struct types (ServerInfoResponse, StatusRequest, ActionsResponse, Action, ExchangeKV, AlarmCommand)
  - Define protocol constants (IndexScenario=590, IndexLightStart=605, IndexLightEnd=622, MaxExchangeIndex=999)
  - Add JSON struct tags with proper field ordering for ActionsResponse (_de67f before actions)
  - _Requirements: 1.2, 1.4, 4.1, 4.2_

- [ ]* 2.1 Write property test for JSON struct serialization
  - **Property 3: MyActions Field Ordering**
  - **Validates: Requirements 1.4**

- [ ]* 2.2 Write property test for parameter format
  - **Property 16: Parameter Format Consistency**
  - **Validates: Requirements 5.3**

- [x] 3. Implement data layer (MemoryStore)
  - Create Store interface with methods for exchange table and action queue operations
  - Implement ClientData struct with ExchangeTable, ActionQueue, and connection status
  - Implement thread-safe ExchangeTable with sync.RWMutex
  - Implement thread-safe ActionQueue with sync.Mutex and FIFO behavior
  - Implement GetValue, SetValue, GetAllValues for exchange table
  - Implement EnqueueAction, DequeueActions, AcknowledgeAction for action queue
  - _Requirements: 2.1, 2.2, 2.3, 2.5, 3.1, 3.2, 3.3_

- [ ]* 3.1 Write property test for exchange table thread safety
  - **Property 5: Exchange Table Thread Safety**
  - **Validates: Requirements 2.1**

- [ ]* 3.2 Write property test for exchange table updates
  - **Property 6: Exchange Table Update Overwrites**
  - **Validates: Requirements 2.2, 2.3**

- [ ]* 3.3 Write property test for valid index range
  - **Property 7: Valid Index Range**
  - **Validates: Requirements 2.5**

- [ ]* 3.4 Write property test for GUID uniqueness
  - **Property 8: GUID Uniqueness**
  - **Validates: Requirements 3.1**

- [ ]* 3.5 Write property test for FIFO queue ordering
  - **Property 9: FIFO Action Queue Ordering**
  - **Validates: Requirements 3.2**

- [ ]* 3.6 Write property test for action acknowledgment
  - **Property 4: Action Acknowledgment Removes Correct Action**
  - **Validates: Requirements 3.3**

- [x] 4. Implement JSON normalization for malformed client JSON
  - Create NormalizeJSON function in api/json_normalizer.go
  - Implement regex-based transformation: `{k:` → `{"k":`, `,v:` → `,"v":`
  - Preserve quoted strings and numeric values during normalization
  - Handle nested structures and arrays
  - Return error for JSON that cannot be normalized
  - _Requirements: 1.2, 6.1, 6.2, 6.3, 6.4_

- [ ]* 4.1 Write property test for JSON normalization
  - **Property 2: JSON Normalization Correctness**
  - **Validates: Requirements 1.2, 6.1**

- [ ]* 4.2 Write property test for numeric value preservation
  - **Property 17: Numeric Value Preservation in Normalization**
  - **Validates: Requirements 6.2**

- [ ]* 4.3 Write property test for string value preservation
  - **Property 18: String Value Preservation in Normalization**
  - **Validates: Requirements 6.3**

- [x] 5. Implement action processing logic (ActionService)
  - Create ActionService struct with Store dependency
  - Implement GenerateCompleteBlock function to ensure all indices 605-622 are present
  - Implement automatic addition of index 590 with value "1" for light/shutter actions
  - Implement parameter ordering by ascending index number
  - Preserve explicit values when generating complete block
  - _Requirements: 3.5, 4.1, 4.2, 4.4, 4.5_

- [ ]* 5.1 Write property test for complete block generation
  - **Property 11: Complete Light Block Generation**
  - **Validates: Requirements 3.5, 4.1**

- [ ]* 5.2 Write property test for scenario index auto-addition
  - **Property 12: Scenario Index Auto-Addition**
  - **Validates: Requirements 4.2**

- [ ]* 5.3 Write property test for explicit value preservation
  - **Property 13: Explicit Values Preserved in Complete Block**
  - **Validates: Requirements 4.4**

- [ ]* 5.4 Write property test for parameter ordering
  - **Property 14: Parameter Ordering by Index**
  - **Validates: Requirements 4.5**

- [x] 6. Implement bitwise OR fusion logic
  - Implement BitwiseFusion function in ActionService
  - Parse string values as integers, apply bitwise OR, convert back to string
  - Implement exception for index 590 (no fusion)
  - Implement fallback to most recent value for non-numeric values
  - Handle single action case (no fusion needed)
  - _Requirements: 3.4, 10.1, 10.2, 10.3, 10.4, 10.5_

- [ ]* 6.1 Write property test for bitwise OR fusion
  - **Property 10: Bitwise OR Fusion**
  - **Validates: Requirements 3.4, 10.1, 10.2**

- [ ]* 6.2 Write property test for scenario index fusion exception
  - **Property 28: Scenario Index Fusion Exception**
  - **Validates: Requirements 10.3**

- [ ]* 6.3 Write property test for fusion fallback
  - **Property 29: Fusion Fallback for Non-Numeric Values**
  - **Validates: Requirements 10.4**

- [ ]* 6.4 Write property test for single action no fusion
  - **Property 30: Single Action No Fusion**
  - **Validates: Requirements 10.5**

- [x] 7. Implement status service (StatusService)
  - Create StatusService struct with Store dependency
  - Implement UpdateStatus function to process client status updates
  - Implement GetRequestedIndices function to return indices server wants from client
  - Store status values in exchange table
  - _Requirements: 2.2, 2.3_

- [x] 8. Implement authentication middleware
  - Create BasicAuth middleware function
  - Extract and decode Authorization header (Base64)
  - Validate credentials against configured client matricules
  - Set clientID in request context on successful authentication
  - Return HTTP 401 for missing or invalid credentials
  - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5_

- [ ]* 8.1 Write property test for valid authentication
  - **Property 21: Valid Authentication Success**
  - **Validates: Requirements 8.2**

- [ ]* 8.2 Write property test for Base64 decoding
  - **Property 22: Base64 Credential Decoding**
  - **Validates: Requirements 8.4**

- [ ]* 8.3 Write property test for multi-client authentication
  - **Property 23: Multi-Client Authentication Support**
  - **Validates: Requirements 8.5**

- [x] 9. Implement logging middleware
  - Create RequestLogger middleware function
  - Log method, path, client IP, timestamp for each request
  - Log status code and response time for each response
  - Log original and normalized JSON for malformed JSON normalization
  - _Requirements: 9.1, 9.2, 9.3_

- [ ]* 9.1 Write property test for request logging
  - **Property 24: Request Logging Completeness**
  - **Validates: Requirements 9.1**

- [ ]* 9.2 Write property test for response logging
  - **Property 25: Response Logging Completeness**
  - **Validates: Requirements 9.2**

- [ ]* 9.3 Write property test for normalization logging
  - **Property 26: JSON Normalization Logging**
  - **Validates: Requirements 9.3**

- [x] 10. Implement recovery middleware
  - Create Recovery middleware function
  - Catch panics in handlers using defer/recover
  - Log error message and stack trace
  - Return HTTP 500 without crashing server
  - _Requirements: 7.5_

- [ ]* 10.1 Write property test for panic recovery
  - **Property 20: Panic Recovery**
  - **Validates: Requirements 7.5**

- [x] 11. Implement HTTP handlers
  - Create Handler struct with ActionService, StatusService, and Store dependencies
  - Implement GetServerInfos handler (GET /api/serverinfos)
  - Implement PostMyStatus handler (POST /api/mystatus) with JSON normalization
  - Implement GetMyActions handler (GET /api/myactions) with proper field ordering
  - Implement PostDone handler (POST /api/done/{guid})
  - Set Content-Type header to "application/json ;charset=UTF-8" (with space before semicolon)
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 5.5_

- [ ]* 11.1 Write property test for ServerInfo response structure
  - **Property 1: ServerInfo Response Structure**
  - **Validates: Requirements 1.1**

- [ ]* 11.2 Write property test for null field serialization
  - **Property 15: Null Field Serialization**
  - **Validates: Requirements 5.2**

- [x] 12. Implement router and wire up middleware
  - Create router.go with route definitions
  - Wire up middleware chain: Recovery → Logging → BasicAuth → Handlers
  - Configure routes: GET /api/serverinfos, POST /api/mystatus, GET /api/myactions, POST /api/done/{guid}
  - Add health check endpoint GET /health
  - _Requirements: 7.5, 8.1, 9.1_

- [x] 13. Implement main server entry point
  - Create cmd/server/main.go
  - Initialize Store, ActionService, StatusService, Handler
  - Configure HTTP server to listen on port 80 (MANDATORY)
  - Set read and write timeouts
  - Log server startup with port and configuration
  - Handle graceful shutdown on SIGINT/SIGTERM
  - _Requirements: 1.1, 9.5_

- [x] 14. Add configuration management
  - Support environment variables (SERVER_PORT, LOG_LEVEL, AUTH_ENABLED, CLIENT_CREDENTIALS)
  - Support config.yaml file for structured configuration
  - Default to port 80 (mandatory for client compatibility)
  - Load client credentials from configuration
  - _Requirements: 1.1, 8.5_

- [ ] 15. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 16. Write integration tests for full client polling cycle
  - Test sequence: serverinfos → mystatus → myactions → done
  - Test multiple concurrent clients
  - Test action queue with multiple pending actions
  - Test malformed JSON handling end-to-end
  - Test authentication failure scenarios

- [ ]* 17. Write concurrency stress tests
  - Run 100 concurrent clients polling every 10ms
  - Verify no deadlocks after 10,000 requests
  - Run with Go race detector enabled
  - Monitor memory usage for leaks

- [x] 18. Create README with setup instructions
  - Document port 80 requirement and setcap setup
  - Document configuration options
  - Document API endpoints
  - Document authentication setup
  - Include example requests and responses
  - _Requirements: 1.1_

- [ ] 19. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.
