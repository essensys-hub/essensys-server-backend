package middleware

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// contextKey for storing normalized JSON info
const (
	NormalizedJSONKey contextKey = "normalizedJSON"
)

// NormalizedJSONInfo stores information about JSON normalization
type NormalizedJSONInfo struct {
	Original   string
	Normalized string
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// newResponseWriter creates a new responseWriter
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default status code
		written:        false,
	}
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(statusCode int) {
	if !rw.written {
		rw.statusCode = statusCode
		rw.written = true
		rw.ResponseWriter.WriteHeader(statusCode)
	}
}

// Write ensures status code is captured even if WriteHeader isn't called explicitly
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// RequestLogger middleware logs HTTP requests and responses
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract client IP (remove port)
		clientIP := r.RemoteAddr
		if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
			clientIP = clientIP[:idx]
		}
		if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
			clientIP = forwardedFor
		}

		// Log incoming request with timestamp and client IP
		// Format: [GO] DD/MM/YYYY HH:MM:SS METHOD PATH (IP)
		timestamp := time.Now().Format("02/01/2006 15:04:05")
		log.Printf("[GO] %s %s %s (%s)", timestamp, r.Method, r.URL.Path, clientIP)

		// Wrap response writer to capture status code
		wrappedWriter := newResponseWriter(w)

		// Call next handler
		next.ServeHTTP(wrappedWriter, r)

		// Log JSON normalization if it occurred (only in debug mode)
		if normalizedInfo, ok := r.Context().Value(NormalizedJSONKey).(*NormalizedJSONInfo); ok {
			log.Printf("[DEBUG] JSON normalized for %s", r.URL.Path)
			log.Printf("[DEBUG] Original: %s", normalizedInfo.Original)
			log.Printf("[DEBUG] Normalized: %s", normalizedInfo.Normalized)
		}
	})
}
