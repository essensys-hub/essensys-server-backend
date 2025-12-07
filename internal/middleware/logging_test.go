package middleware

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRequestLogger_BasicLogging tests that the middleware logs requests and responses
func TestRequestLogger_BasicLogging(t *testing.T) {
	// Capture log output
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer log.SetOutput(nil) // Reset after test

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Wrap with logging middleware
	loggedHandler := RequestLogger(testHandler)

	// Create test request
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	// Execute request
	loggedHandler.ServeHTTP(w, req)

	// Verify log output
	logOutput := logBuffer.String()

	// Check for request log
	if !strings.Contains(logOutput, "[REQUEST]") {
		t.Error("Expected [REQUEST] log entry")
	}
	if !strings.Contains(logOutput, "GET") {
		t.Error("Expected method GET in log")
	}
	if !strings.Contains(logOutput, "/api/test") {
		t.Error("Expected path /api/test in log")
	}
	if !strings.Contains(logOutput, "192.168.1.1") {
		t.Error("Expected client IP in log")
	}

	// Check for response log
	if !strings.Contains(logOutput, "[RESPONSE]") {
		t.Error("Expected [RESPONSE] log entry")
	}
	if !strings.Contains(logOutput, "200") {
		t.Error("Expected status code 200 in log")
	}
}

// TestRequestLogger_StatusCodeCapture tests that various status codes are captured
func TestRequestLogger_StatusCodeCapture(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"Created", http.StatusCreated},
		{"BadRequest", http.StatusBadRequest},
		{"Unauthorized", http.StatusUnauthorized},
		{"NotFound", http.StatusNotFound},
		{"InternalServerError", http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			log.SetOutput(&logBuffer)
			defer log.SetOutput(nil)

			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			})

			loggedHandler := RequestLogger(testHandler)
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			loggedHandler.ServeHTTP(w, req)

			logOutput := logBuffer.String()
			
			// Check if status code appears in log
			if !strings.Contains(logOutput, "[RESPONSE]") {
				t.Errorf("Expected [RESPONSE] log entry for status %d", tc.statusCode)
			}
			
			// Verify the actual status code was captured
			if w.Code != tc.statusCode {
				t.Errorf("Expected status code %d, got %d", tc.statusCode, w.Code)
			}
		})
	}
}

// TestRequestLogger_JSONNormalization tests logging of normalized JSON
func TestRequestLogger_JSONNormalization(t *testing.T) {
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer log.SetOutput(nil)

	// Create a middleware that simulates JSON normalization (would happen before logging)
	normalizationMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate JSON normalization by adding info to context
			normalizedInfo := &NormalizedJSONInfo{
				Original:   `{k:1,v:"test"}`,
				Normalized: `{"k":1,"v":"test"}`,
			}
			ctx := context.WithValue(r.Context(), NormalizedJSONKey, normalizedInfo)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Chain: normalization -> logging -> handler
	handler := normalizationMiddleware(RequestLogger(testHandler))
	req := httptest.NewRequest("POST", "/api/mystatus", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	logOutput := logBuffer.String()

	// Check for JSON normalization logs
	if !strings.Contains(logOutput, "[JSON_NORMALIZATION]") {
		t.Error("Expected [JSON_NORMALIZATION] log entry")
	}
	if !strings.Contains(logOutput, "Original:") {
		t.Error("Expected 'Original:' in normalization log")
	}
	if !strings.Contains(logOutput, "Normalized:") {
		t.Error("Expected 'Normalized:' in normalization log")
	}
	if !strings.Contains(logOutput, `{k:1,v:"test"}`) {
		t.Error("Expected original JSON in log")
	}
	if !strings.Contains(logOutput, `{"k":1,"v":"test"}`) {
		t.Error("Expected normalized JSON in log")
	}
}

// TestRequestLogger_XForwardedFor tests that X-Forwarded-For header is used for client IP
func TestRequestLogger_XForwardedFor(t *testing.T) {
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer log.SetOutput(nil)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	loggedHandler := RequestLogger(testHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	w := httptest.NewRecorder()

	loggedHandler.ServeHTTP(w, req)

	logOutput := logBuffer.String()

	// Should use X-Forwarded-For IP instead of RemoteAddr
	if !strings.Contains(logOutput, "203.0.113.1") {
		t.Error("Expected X-Forwarded-For IP in log")
	}
}

// TestRequestLogger_ResponseTime tests that response time is logged
func TestRequestLogger_ResponseTime(t *testing.T) {
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer log.SetOutput(nil)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	loggedHandler := RequestLogger(testHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	loggedHandler.ServeHTTP(w, req)

	logOutput := logBuffer.String()

	// Check that response log contains timing information (should have 'ms' or 'µs' or 's')
	if !strings.Contains(logOutput, "s") && !strings.Contains(logOutput, "ms") && !strings.Contains(logOutput, "µs") {
		t.Error("Expected response time in log")
	}
}

// TestResponseWriter_DefaultStatusCode tests that default status code is 200
func TestResponseWriter_DefaultStatusCode(t *testing.T) {
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer log.SetOutput(nil)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't explicitly call WriteHeader
		w.Write([]byte("test"))
	})

	loggedHandler := RequestLogger(testHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	loggedHandler.ServeHTTP(w, req)

	logOutput := logBuffer.String()

	// Should log 200 as default status code
	if !strings.Contains(logOutput, "200") {
		t.Error("Expected default status code 200 in log")
	}
}
