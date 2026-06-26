package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type LanIAMHandler struct {
	svc          *laniam.Service
	secureCookie bool
}

func NewLanIAMHandler(svc *laniam.Service, secureCookie bool) *LanIAMHandler {
	return &LanIAMHandler{svc: svc, secureCookie: secureCookie}
}

func (h *LanIAMHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	user, err := h.svc.Login(req.Email, req.Password)
	if err != nil {
		if err.Error() == "account_disabled" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "account_disabled"})
			return
		}
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	sess, err := h.svc.Sessions().CreateSession(user)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	middleware.SetLanSessionCookie(w, sess.ID, h.svc.Sessions().CookieMaxAgeSeconds(), h.secureCookie)
	writeJSON(w, http.StatusOK, map[string]interface{}{"user": publicLanUser(user)})
}

func (h *LanIAMHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c, err := r.Cookie(middleware.LanSessionCookie); err == nil {
		h.svc.Sessions().DeleteSession(c.Value)
	}
	middleware.ClearLanSessionCookie(w, h.secureCookie)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *LanIAMHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetLanUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"user": publicLanUser(user)})
}

func (h *LanIAMHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := middleware.GetLanUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if err := h.svc.ChangePassword(user.ID, req.CurrentPassword, req.NewPassword); err != nil {
		if err.Error() == "invalid credentials" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *LanIAMHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetLanUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	users, err := h.svc.ListUsers(user)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	out := make([]map[string]interface{}, 0, len(users))
	for i := range users {
		out = append(out, publicLanUser(&users[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": out})
}

func (h *LanIAMHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	admin, ok := middleware.GetLanUser(r)
	if !ok || !admin.CanManageLanUsers() {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		Role        string `json:"role"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = models.LanRoleUser
	}
	u, err := h.svc.CreateUser(req.Email, req.Password, req.Role, req.DisplayName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"user": publicLanUser(u)})
}

func (h *LanIAMHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email          string `json:"email"`
		Password       string `json:"password"`
		BootstrapToken string `json:"bootstrap_token"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	u, err := h.svc.BootstrapAdmin(req.Email, req.Password, req.BootstrapToken)
	if err != nil {
		if err.Error() == "admin already exists" {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"user": publicLanUser(u)})
}

func (h *LanIAMHandler) RegisterClosed(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Forbidden", http.StatusForbidden)
}

func (h *LanIAMHandler) HandleLanUserSubresource(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.GetLanUser(r)
	if !ok || !admin.CanManageLanUsers() {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/lan-users/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "reset-password" && r.Method == http.MethodPost:
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if err := h.svc.ResetPassword(admin, id, req.Password); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case action == "disable" && r.Method == http.MethodPost:
		if err := h.svc.DisableUser(admin, id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.NotFound(w, r)
	}
}

func publicLanUser(u *models.LanUser) map[string]interface{} {
	return map[string]interface{}{
		"id":           u.ID,
		"email":        u.Email,
		"role":         u.Role,
		"display_name": u.DisplayName,
		"disabled_at":  u.DisabledAt,
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
