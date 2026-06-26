package middleware

import (
	"context"
	"net/http"

	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type lanContextKey string

const (
	lanUserKey      lanContextKey = "lanUser"
	lanSessionIDKey lanContextKey = "lanSessionID"
	LanSessionCookie              = "essensys_lan_session"
)

func GetLanUser(r *http.Request) (*models.LanUser, bool) {
	u, ok := r.Context().Value(lanUserKey).(*models.LanUser)
	return u, ok
}

func SetLanSessionCookie(w http.ResponseWriter, sessionID string, maxAgeSec int, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     LanSessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAgeSec,
	})
}

func ClearLanSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     LanSessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func LanRequireSession(store *laniam.SessionStore, secureCookie bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(LanSessionCookie)
			if err != nil || cookie.Value == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			sess, ok := store.GetSession(cookie.Value)
			if !ok || sess.User == nil {
				ClearLanSessionCookie(w, secureCookie)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if sess.User.IsDisabled() {
				store.DeleteSession(cookie.Value)
				ClearLanSessionCookie(w, secureCookie)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"account_disabled"}`))
				return
			}
			ctx := context.WithValue(r.Context(), lanSessionIDKey, sess.ID)
			ctx = context.WithValue(ctx, lanUserKey, sess.User)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func LanRequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetLanUser(r)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if _, ok := allowed[user.Role]; !ok {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
