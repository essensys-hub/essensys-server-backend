package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecovery_CatchesPanic(t *testing.T) {
	// Capture log output
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer log.SetOutput(nil)

	// Create a handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Wrap with recovery middleware
	handler := Recovery(panicHandler)

	// Create test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Execute request - should not panic
	handler.ServeHTTP(rec, req)

	// Verify HTTP 500 was returned
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	// Verify response contains error message
	body := rec.Body.String()
	if body == "" {
		t.Error("Expected error message in response body, got empty string")
	}
	if !strings.Contains(body, "test panic") {
		t.Errorf("Expected error message to contain 'test panic', got '%s'", body)
	}

	// Verify panic was logged
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "[PANIC]") {
		t.Error("Expected [PANIC] log entry")
	}
	if !strings.Contains(logOutput, "test panic") {
		t.Error("Expected panic message in log")
	}
	if !strings.Contains(logOutput, "Stack trace") {
		t.Error("Expected stack trace in log")
	}
}

func TestRecovery_NormalHandlerWorks(t *testing.T) {
	// Capture log output
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer log.SetOutput(nil)

	// Create a normal handler that doesn't panic
	normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Wrap with recovery middleware
	handler := Recovery(normalHandler)

	// Create test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rec, req)

	// Verify normal response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()
	if body != "success" {
		t.Errorf("Expected body 'success', got '%s'", body)
	}

	// Verify no panic was logged
	logOutput := logBuffer.String()
	if strings.Contains(logOutput, "[PANIC]") {
		t.Error("Did not expect [PANIC] log entry for normal handler")
	}
}
