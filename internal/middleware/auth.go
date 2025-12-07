package middleware

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// ClientIDKey is the context key for storing client ID
	ClientIDKey contextKey = "clientID"
)

// GetClientID extracts the client ID from the request context
func GetClientID(r *http.Request) (string, bool) {
	clientID, ok := r.Context().Value(ClientIDKey).(string)
	return clientID, ok
}

// BasicAuth middleware validates Basic Authentication credentials
// validCredentials is a map of username:password pairs
func BasicAuth(validCredentials map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Check if it's Basic auth
			if !strings.HasPrefix(authHeader, "Basic ") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Extract the base64 encoded credentials
			encodedCredentials := strings.TrimPrefix(authHeader, "Basic ")

			// Decode Base64
			decodedBytes, err := base64.StdEncoding.DecodeString(encodedCredentials)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Parse username:password
			credentials := string(decodedBytes)
			parts := strings.SplitN(credentials, ":", 2)
			if len(parts) != 2 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			username := parts[0]
			password := parts[1]

			// Validate credentials
			expectedPassword, exists := validCredentials[username]
			if !exists || expectedPassword != password {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Set clientID in context (using username as clientID)
			ctx := context.WithValue(r.Context(), ClientIDKey, username)
			r = r.WithContext(ctx)

			// Authentication successful, proceed to next handler
			next.ServeHTTP(w, r)
		})
	}
}
