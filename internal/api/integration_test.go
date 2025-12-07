package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// TestFullClientPollingCycle tests the complete sequence: serverinfos → mystatus → myactions → done
func TestFullClientPollingCycle(t *testing.T) {
	// Setup server
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	validCredentials := map[string]string{
		"client1": "password1",
	}
	router := NewRouter(handler, validCredentials, true)
	server := httptest.NewServer(router)
	defer server.Close()

	// Create HTTP client
	client := &http.Client{}

	// Helper function to create authenticated request
	makeAuthRequest := func(method, path string, body []byte) *http.Request {
		var req *http.Request
		var err error
		if body != nil {
			req, err = http.NewRequest(method, server.URL+path, bytes.NewReader(body))
		} else {
			req, err = http.NewRequest(method, server.URL+path, nil)
		}
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		// Add Basic Auth header
		auth := base64.StdEncoding.EncodeToString([]byte("client1:password1"))
		req.Header.Set("Authorization", "Basic "+auth)
		return req
	}

	// Step 1: GET /api/serverinfos
	t.Run("Step1_ServerInfos", func(t *testing.T) {
		req := makeAuthRequest(http.MethodGet, "/api/serverinfos", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var serverInfo protocol.ServerInfoResponse
		if err := json.NewDecoder(resp.Body).Decode(&serverInfo); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify response structure
		if serverInfo.Infos == nil {
			t.Error("Expected Infos to be non-nil")
		}
	})

	// Step 2: POST /api/mystatus
	t.Run("Step2_MyStatus", func(t *testing.T) {
		statusReq := protocol.StatusRequest{
			Version: "1.0",
			EK: []protocol.ExchangeKV{
				{K: 100, V: "42"},
				{K: 200, V: "test-value"},
			},
		}
		body, _ := json.Marshal(statusReq)
		req := makeAuthRequest(http.MethodPost, "/api/mystatus", body)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		// Verify data was stored
		value, exists := store.GetValue("client1", 100)
		if !exists || value != "42" {
			t.Errorf("Expected value '42', got '%s' (exists: %v)", value, exists)
		}
	})

	// Step 3: Add an action to the queue (simulating web interface)
	action := protocol.Action{
		GUID: "test-action-guid-123",
		Params: []protocol.ExchangeKV{
			{K: 590, V: "1"},
			{K: 605, V: "64"},
		},
	}
	store.EnqueueAction("client1", action)

	// Step 4: GET /api/myactions
	var retrievedGUID string
	t.Run("Step3_MyActions", func(t *testing.T) {
		req := makeAuthRequest(http.MethodGet, "/api/myactions", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var actionsResp protocol.ActionsResponse
		if err := json.NewDecoder(resp.Body).Decode(&actionsResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(actionsResp.Actions) != 1 {
			t.Fatalf("Expected 1 action, got %d", len(actionsResp.Actions))
		}

		retrievedGUID = actionsResp.Actions[0].GUID
		if retrievedGUID != "test-action-guid-123" {
			t.Errorf("Expected GUID 'test-action-guid-123', got '%s'", retrievedGUID)
		}
	})

	// Step 5: POST /api/done/{guid}
	t.Run("Step4_Done", func(t *testing.T) {
		req := makeAuthRequest(http.MethodPost, "/api/done/"+retrievedGUID, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		// Verify action was removed
		actions := store.DequeueActions("client1")
		if len(actions) != 0 {
			t.Errorf("Expected action to be removed, but %d actions remain", len(actions))
		}
	})

	// Step 6: Verify empty actions after acknowledgment
	t.Run("Step5_EmptyActionsAfterDone", func(t *testing.T) {
		req := makeAuthRequest(http.MethodGet, "/api/myactions", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		var actionsResp protocol.ActionsResponse
		if err := json.NewDecoder(resp.Body).Decode(&actionsResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(actionsResp.Actions) != 0 {
			t.Errorf("Expected 0 actions after acknowledgment, got %d", len(actionsResp.Actions))
		}
	})
}

// TestMultipleConcurrentClients tests multiple clients polling simultaneously
func TestMultipleConcurrentClients(t *testing.T) {
	// Setup server
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	validCredentials := map[string]string{
		"client1": "password1",
		"client2": "password2",
		"client3": "password3",
	}
	router := NewRouter(handler, validCredentials, true)
	server := httptest.NewServer(router)
	defer server.Close()

	// Number of concurrent clients
	numClients := 3
	numRequests := 10

	var wg sync.WaitGroup
	errors := make(chan error, numClients*numRequests)

	// Simulate multiple clients polling
	for i := 1; i <= numClients; i++ {
		clientID := fmt.Sprintf("client%d", i)
		password := fmt.Sprintf("password%d", i)

		wg.Add(1)
		go func(cid, pwd string) {
			defer wg.Done()

			client := &http.Client{}
			auth := base64.StdEncoding.EncodeToString([]byte(cid + ":" + pwd))

			for j := 0; j < numRequests; j++ {
				// GET /api/serverinfos
				req, err := http.NewRequest(http.MethodGet, server.URL+"/api/serverinfos", nil)
				if err != nil {
					errors <- fmt.Errorf("client %s: failed to create request: %v", cid, err)
					continue
				}
				req.Header.Set("Authorization", "Basic "+auth)
				resp, err := client.Do(req)
				if err != nil {
					errors <- fmt.Errorf("client %s: serverinfos failed: %v", cid, err)
					continue
				}
				resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					errors <- fmt.Errorf("client %s: expected status 200, got %d", cid, resp.StatusCode)
				}

				// POST /api/mystatus
				statusReq := protocol.StatusRequest{
					Version: "1.0",
					EK: []protocol.ExchangeKV{
						{K: j * 10, V: fmt.Sprintf("value-%d", j)},
					},
				}
				body, _ := json.Marshal(statusReq)
				req, err = http.NewRequest(http.MethodPost, server.URL+"/api/mystatus", bytes.NewReader(body))
				if err != nil {
					errors <- fmt.Errorf("client %s: failed to create request: %v", cid, err)
					continue
				}
				req.Header.Set("Authorization", "Basic "+auth)
				req.Header.Set("Content-Type", "application/json")
				resp, err = client.Do(req)
				if err != nil {
					errors <- fmt.Errorf("client %s: mystatus failed: %v", cid, err)
					continue
				}
				resp.Body.Close()

				if resp.StatusCode != http.StatusCreated {
					errors <- fmt.Errorf("client %s: expected status 201, got %d", cid, resp.StatusCode)
				}

				// GET /api/myactions
				req, err = http.NewRequest(http.MethodGet, server.URL+"/api/myactions", nil)
				if err != nil {
					errors <- fmt.Errorf("client %s: failed to create request: %v", cid, err)
					continue
				}
				req.Header.Set("Authorization", "Basic "+auth)
				resp, err = client.Do(req)
				if err != nil {
					errors <- fmt.Errorf("client %s: myactions failed: %v", cid, err)
					continue
				}
				resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					errors <- fmt.Errorf("client %s: expected status 200, got %d", cid, resp.StatusCode)
				}
			}
		}(clientID, password)
	}

	// Wait for all clients to finish
	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Error(err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Encountered %d errors during concurrent client testing", errorCount)
	}

	// Verify data isolation - each client should have their own data
	for i := 1; i <= numClients; i++ {
		clientID := fmt.Sprintf("client%d", i)
		// Check that at least some values were stored for this client
		value, exists := store.GetValue(clientID, 0)
		if !exists {
			t.Errorf("Expected client %s to have stored values", clientID)
		}
		if exists && value != "value-0" {
			t.Errorf("Client %s: expected value 'value-0', got '%s'", clientID, value)
		}
	}
}

// TestActionQueueWithMultiplePendingActions tests handling of multiple pending actions
func TestActionQueueWithMultiplePendingActions(t *testing.T) {
	// Setup server
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	validCredentials := map[string]string{
		"client1": "password1",
	}
	router := NewRouter(handler, validCredentials, true)
	server := httptest.NewServer(router)
	defer server.Close()

	client := &http.Client{}
	auth := base64.StdEncoding.EncodeToString([]byte("client1:password1"))

	// Add multiple actions to the queue
	numActions := 5
	expectedGUIDs := make([]string, numActions)
	for i := 0; i < numActions; i++ {
		guid := fmt.Sprintf("action-guid-%d", i)
		expectedGUIDs[i] = guid
		action := protocol.Action{
			GUID: guid,
			Params: []protocol.ExchangeKV{
				{K: 590, V: "1"},
				{K: 605 + i, V: fmt.Sprintf("%d", i*10)},
			},
		}
		store.EnqueueAction("client1", action)
	}

	// GET /api/myactions - should return all actions
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/myactions", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Basic "+auth)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var actionsResp protocol.ActionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&actionsResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(actionsResp.Actions) != numActions {
		t.Fatalf("Expected %d actions, got %d", numActions, len(actionsResp.Actions))
	}

	// Verify FIFO order
	for i, action := range actionsResp.Actions {
		if action.GUID != expectedGUIDs[i] {
			t.Errorf("Expected GUID '%s' at position %d, got '%s'", expectedGUIDs[i], i, action.GUID)
		}
	}

	// Acknowledge actions one by one
	for i, guid := range expectedGUIDs {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/done/"+guid, nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Basic "+auth)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to acknowledge action %s: %v", guid, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201 for action %s, got %d", guid, resp.StatusCode)
		}

		// Verify remaining actions
		remainingActions := store.DequeueActions("client1")
		expectedRemaining := numActions - i - 1
		if len(remainingActions) != expectedRemaining {
			t.Errorf("After acknowledging %d actions, expected %d remaining, got %d", i+1, expectedRemaining, len(remainingActions))
		}
	}

	// Verify all actions are removed
	finalActions := store.DequeueActions("client1")
	if len(finalActions) != 0 {
		t.Errorf("Expected all actions to be removed, but %d remain", len(finalActions))
	}
}

// TestMalformedJSONHandlingEndToEnd tests malformed JSON handling in the full flow
func TestMalformedJSONHandlingEndToEnd(t *testing.T) {
	// Setup server
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	validCredentials := map[string]string{
		"client1": "password1",
	}
	router := NewRouter(handler, validCredentials, true)
	server := httptest.NewServer(router)
	defer server.Close()

	client := &http.Client{}
	auth := base64.StdEncoding.EncodeToString([]byte("client1:password1"))

	testCases := []struct {
		name           string
		malformedJSON  string
		expectedStatus int
		shouldStore    bool
		checkIndex     int
		expectedValue  string
	}{
		{
			name:           "Unquoted keys",
			malformedJSON:  `{"version":"1.0","ek":[{k:100,v:"test-value"}]}`,
			expectedStatus: http.StatusCreated,
			shouldStore:    true,
			checkIndex:     100,
			expectedValue:  "test-value",
		},
		{
			name:           "Multiple unquoted keys",
			malformedJSON:  `{"version":"1.0","ek":[{k:200,v:"value1"},{k:201,v:"value2"}]}`,
			expectedStatus: http.StatusCreated,
			shouldStore:    true,
			checkIndex:     200,
			expectedValue:  "value1",
		},
		{
			name:           "Numeric values",
			malformedJSON:  `{"version":"1.0","ek":[{k:300,v:"42"}]}`,
			expectedStatus: http.StatusCreated,
			shouldStore:    true,
			checkIndex:     300,
			expectedValue:  "42",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL+"/api/mystatus", bytes.NewReader([]byte(tc.malformedJSON)))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", "Basic "+auth)
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			if tc.shouldStore {
				value, exists := store.GetValue("client1", tc.checkIndex)
				if !exists {
					t.Errorf("Expected value to be stored at index %d", tc.checkIndex)
				}
				if value != tc.expectedValue {
					t.Errorf("Expected value '%s', got '%s'", tc.expectedValue, value)
				}
			}
		})
	}
}

// TestAuthenticationFailureScenarios tests various authentication failure cases
func TestAuthenticationFailureScenarios(t *testing.T) {
	// Setup server
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	validCredentials := map[string]string{
		"client1": "password1",
	}
	router := NewRouter(handler, validCredentials, true)
	server := httptest.NewServer(router)
	defer server.Close()

	client := &http.Client{}

	testCases := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Missing Authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid credentials",
			authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("client1:wrongpassword")),
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Non-existent user",
			authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("nonexistent:password")),
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Malformed Basic Auth (no colon)",
			authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("client1password1")),
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid Base64",
			authHeader:     "Basic invalid-base64!!!",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Wrong auth scheme",
			authHeader:     "Bearer some-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Valid credentials",
			authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("client1:password1")),
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+"/api/serverinfos", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}
		})
	}
}

// TestConcurrentActionQueueOperations tests thread safety of action queue operations
func TestConcurrentActionQueueOperations(t *testing.T) {
	// Setup server
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	validCredentials := map[string]string{
		"client1": "password1",
	}
	router := NewRouter(handler, validCredentials, true)
	server := httptest.NewServer(router)
	defer server.Close()

	client := &http.Client{}
	auth := base64.StdEncoding.EncodeToString([]byte("client1:password1"))

	// Add multiple actions
	numActions := 20
	for i := 0; i < numActions; i++ {
		action := protocol.Action{
			GUID: fmt.Sprintf("concurrent-action-%d", i),
			Params: []protocol.ExchangeKV{
				{K: 590, V: "1"},
			},
		}
		store.EnqueueAction("client1", action)
	}

	// Concurrently read and acknowledge actions
	var wg sync.WaitGroup
	errors := make(chan error, numActions)

	// Multiple goroutines reading actions
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req, err := http.NewRequest(http.MethodGet, server.URL+"/api/myactions", nil)
			if err != nil {
				errors <- fmt.Errorf("failed to create request: %v", err)
				return
			}
			req.Header.Set("Authorization", "Basic "+auth)
			resp, err := client.Do(req)
			if err != nil {
				errors <- fmt.Errorf("failed to get actions: %v", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("expected status 200, got %d", resp.StatusCode)
			}
		}()
	}

	// Multiple goroutines acknowledging actions
	for i := 0; i < numActions; i++ {
		wg.Add(1)
		guid := fmt.Sprintf("concurrent-action-%d", i)
		go func(g string) {
			defer wg.Done()

			req, err := http.NewRequest(http.MethodPost, server.URL+"/api/done/"+g, nil)
			if err != nil {
				errors <- fmt.Errorf("failed to create request: %v", err)
				return
			}
			req.Header.Set("Authorization", "Basic "+auth)
			resp, err := client.Do(req)
			if err != nil {
				errors <- fmt.Errorf("failed to acknowledge action %s: %v", g, err)
				return
			}
			defer resp.Body.Close()

			// Either 201 (success) or 404 (already removed) is acceptable in concurrent scenario
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNotFound {
				errors <- fmt.Errorf("expected status 201 or 404, got %d", resp.StatusCode)
			}
		}(guid)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Error(err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Encountered %d errors during concurrent operations", errorCount)
	}

	// Verify all actions are eventually removed
	finalActions := store.DequeueActions("client1")
	if len(finalActions) != 0 {
		t.Errorf("Expected all actions to be removed, but %d remain", len(finalActions))
	}
}
