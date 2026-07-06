package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/audit"
	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type LanIAMHandler struct {
	svc          *laniam.Service
	secureCookie bool
	audit        *audit.Service
}

func NewLanIAMHandler(svc *laniam.Service, secureCookie bool) *LanIAMHandler {
	return &LanIAMHandler{svc: svc, secureCookie: secureCookie}
}

func (h *LanIAMHandler) SetAuditService(svc *audit.Service) {
	h.audit = svc
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
		switch err.Error() {
		case "account_disabled":
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "account_disabled"})
		default:
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}
		return
	}
	h.svc.RecordLoginClient(user.ID, laniam.ClientIPFromRequest(r))
	h.establishSession(w, user, http.StatusOK)
	if h.audit != nil && h.audit.Enabled() {
		_ = h.audit.EmitAuthEvent(r.Context(), user.Email, "auth:login", "success")
	}
}

func (h *LanIAMHandler) AutoLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, _, err := h.svc.AutoLogin(laniam.ClientIPFromRequest(r))
	if err != nil {
		if err.Error() == "ambiguous_trusted_device" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.establishSession(w, user, http.StatusOK)
}

func (h *LanIAMHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var logoutEmail string
	if c, err := r.Cookie(middleware.LanSessionCookie); err == nil {
		if sess, ok := h.svc.Sessions().GetSession(c.Value); ok && sess.User != nil {
			logoutEmail = sess.User.Email
		}
		h.svc.Sessions().DeleteSession(c.Value)
	}
	middleware.ClearLanSessionCookie(w, h.secureCookie)
	if h.audit != nil && h.audit.Enabled() && logoutEmail != "" {
		_ = h.audit.EmitAuthEvent(r.Context(), logoutEmail, "auth:logout", "success")
	}
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

func (h *LanIAMHandler) ListTrustedDevices(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetLanUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	devices, err := h.svc.ListTrustedDevices(user)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
}

func (h *LanIAMHandler) ListTrustedDeviceCandidates(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetLanUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	candidates, err := h.svc.ListTrustedDeviceCandidates(user, laniam.ClientIPFromRequest(r))
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"candidates": candidates})
}

func (h *LanIAMHandler) CreateTrustedDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := middleware.GetLanUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	candidate, err := decodeCandidate(r)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	device, err := h.svc.TrustCurrentDevice(user, candidate)
	if err != nil {
		h.writeTrustedDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"device": device})
}

func (h *LanIAMHandler) DeleteTrustedDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := middleware.GetLanUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	id, ok := parseTailID(r.URL.Path, "/api/user/me/trusted-devices/")
	if !ok {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	if err := h.svc.RevokeTrustedDevice(user, id); err != nil {
		h.writeTrustedDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *LanIAMHandler) ListAdminTrustedDevices(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.GetLanUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	devices, err := h.svc.ListAdminTrustedDevices(admin)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
}

func (h *LanIAMHandler) ListAdminTrustedDeviceCandidates(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.GetLanUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	candidates, err := h.svc.ListAdminTrustedDeviceCandidates(admin)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"candidates": candidates})
}

func (h *LanIAMHandler) CreateAdminTrustedDevice(w http.ResponseWriter, r *http.Request) {
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
		LanUserID   int    `json:"lan_user_id"`
		MacAddress  string `json:"mac_address"`
		DeviceLabel string `json:"device_label"`
		SourceIP    string `json:"source_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	device, err := h.svc.CreateAdminTrustedDevice(admin, req.LanUserID, models.TrustedDeviceCandidate{
		MacAddress:  req.MacAddress,
		DeviceLabel: req.DeviceLabel,
		SourceIP:    req.SourceIP,
	})
	if err != nil {
		h.writeTrustedDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"device": device})
}

func (h *LanIAMHandler) HandleAdminTrustedDeviceSubresource(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.GetLanUser(r)
	if !ok || !admin.CanManageLanUsers() {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/trusted-devices/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	switch {
	case parts[1] == "promote-permanent" && r.Method == http.MethodPost:
		if err := h.svc.PromoteTrustedDevice(admin, id); err != nil {
			h.writeTrustedDeviceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case parts[1] == "revoke" && r.Method == http.MethodPost:
		if err := h.svc.RevokeTrustedDeviceAdmin(admin, id); err != nil {
			h.writeTrustedDeviceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.NotFound(w, r)
	}
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
	case action == "enable" && r.Method == http.MethodPost:
		if err := h.svc.EnableUser(admin, id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.NotFound(w, r)
	}
}

func (h *LanIAMHandler) establishSession(w http.ResponseWriter, user *models.LanUser, status int) {
	sess, err := h.svc.Sessions().CreateSession(user)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	middleware.SetLanSessionCookie(w, sess.ID, h.svc.Sessions().CookieMaxAgeSeconds(), h.secureCookie)
	writeJSON(w, status, map[string]interface{}{"user": publicLanUser(user)})
}

func (h *LanIAMHandler) writeTrustedDeviceError(w http.ResponseWriter, err error) {
	switch err.Error() {
	case "forbidden", "admin_trusted_device_forbidden":
		http.Error(w, err.Error(), http.StatusForbidden)
	case "invalid_mac_address", "trusted_device_not_found", "user not found":
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func decodeCandidate(r *http.Request) (models.TrustedDeviceCandidate, error) {
	var req struct {
		MacAddress  string `json:"mac_address"`
		DeviceLabel string `json:"device_label"`
		SourceIP    string `json:"source_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return models.TrustedDeviceCandidate{}, err
	}
	return models.TrustedDeviceCandidate{MacAddress: req.MacAddress, DeviceLabel: req.DeviceLabel, SourceIP: req.SourceIP}, nil
}

func parseTailID(path, prefix string) (int, bool) {
	value := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if value == "" || strings.Contains(value, "/") {
		return 0, false
	}
	id, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return id, true
}

func publicLanUser(u *models.LanUser) map[string]interface{} {
	return map[string]interface{}{
		"id":                      u.ID,
		"email":                   u.Email,
		"role":                    u.Role,
		"display_name":            u.DisplayName,
		"disabled_at":             u.DisabledAt,
		"can_use_trusted_devices": u.CanUseTrustedDevices(),
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
