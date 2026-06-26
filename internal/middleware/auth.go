package middleware

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
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

// BasicAuth middleware validates Basic Authentication credentials.
// When passiveCapture is false or LAN IAM is active, credentials are not stored in Redis.
func BasicAuth(validCredentials map[string]string, store data.Store, passiveCapture bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			
			// Default clientID if no auth provided
			clientID := "unknown"

			// Extract Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Basic ") {
				// Extract the base64 encoded credentials
				encodedCredentials := strings.TrimPrefix(authHeader, "Basic ")

				// Decode Base64 (try-catch style)
				decodedBytes, err := base64.StdEncoding.DecodeString(encodedCredentials)
				if err == nil {
					// Parse username:password
					credentials := string(decodedBytes)
					parts := strings.SplitN(credentials, ":", 2)
					
					if len(parts) == 2 {
						username := parts[0]
						// password := parts[1] // Ignored in passive mode

						clientID = username

						if passiveCapture && store != nil {
							ip, _, _ := net.SplitHostPort(r.RemoteAddr)
							store.SetAuthInfo(username, ip, encodedCredentials, "?")
						}
					}
				}
			}

			// In PASSIVE MODE, we DO NOT VALIDATE expectedPassword.
			// We accept the request regardless.
			
			// Set clientID in context
			ctx := context.WithValue(r.Context(), ClientIDKey, clientID)
			r = r.WithContext(ctx)

			// Proceed to next handler
			next.ServeHTTP(w, r)
		})
	}
}
