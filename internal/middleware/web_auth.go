package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/auth"
)

const sessionIDKey contextKey = "sessionID"
const userIDKey contextKey = "userID"

// WebAuth middleware for web endpoints (sessions-based)
func WebAuth(sessionStore *auth.SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get session ID from cookie
			cookie, err := r.Cookie("session_id")
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			sessionID := cookie.Value
			session, exists := sessionStore.GetSession(sessionID)
			if !exists {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add session info to context
			ctx := context.WithValue(r.Context(), sessionIDKey, sessionID)
			ctx = context.WithValue(ctx, userIDKey, session.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetSessionID retrieves the session ID from context
func GetSessionID(r *http.Request) (string, bool) {
	sessionID, ok := r.Context().Value(sessionIDKey).(string)
	return sessionID, ok
}

// GetUserID retrieves the user ID from context
func GetUserID(r *http.Request) (int, bool) {
	userID, ok := r.Context().Value(userIDKey).(int)
	return userID, ok
}

// SetSessionCookie sets the session cookie
func SetSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24 hours
	})
}

// ClearSessionCookie clears the session cookie
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// CORS middleware for web frontend
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		
		// Allow specific origins (configure as needed)
		allowedOrigins := []string{
			"http://localhost:5173", // Vite dev server
			"http://localhost:3000", // Alternative dev server
		}
		
		allowed := false
		for _, allowedOrigin := range allowedOrigins {
			if origin == allowedOrigin {
				allowed = true
				break
			}
		}
		
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		
		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// ExtractBearerToken extracts Bearer token from Authorization header
func ExtractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}
	
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	
	return parts[1]
}

