package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/essensys-hub/essensys-server-backend/internal/auth"
    "github.com/essensys-hub/essensys-server-backend/internal/core"
    "github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/data/database"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/essensys-hub/essensys-server-backend/internal/services"
    "github.com/essensys-hub/essensys-server-backend/pkg/protocol"
	"github.com/jmoiron/sqlx"
)

// WebHandler contains HTTP request handlers for web endpoints (React frontend)
type WebHandler struct {
	userService  *services.UserService
	sessionStore *auth.SessionStore
	userRepo     *database.UserRepository
    actionService *core.ActionService
    store        data.Store
}

// NewWebHandler creates a new WebHandler instance
func NewWebHandler(db *sqlx.DB, sessionStore *auth.SessionStore, actionService *core.ActionService, store data.Store) *WebHandler {
	return &WebHandler{
		userService:  services.NewUserService(db),
		sessionStore: sessionStore,
		userRepo:     database.NewUserRepository(db),
        actionService: actionService,
        store:        store,
	}
}

// Login handles POST /api/auth/login
func (h *WebHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate credentials
	user, err := h.userService.Login(req.Email, req.Password)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Create session
	session, err := h.sessionStore.CreateSession(user)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	middleware.SetSessionCookie(w, session.ID)

	// Return user info (without sensitive data)
	response := map[string]interface{}{
		"user": map[string]interface{}{
			"id":      user.ID,
			"email":   user.Mail,
			"nom":     user.Nom,
			"prenom":  user.Prenom,
			"machine_id": user.MachineID,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Logout handles POST /api/auth/logout
func (h *WebHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get session ID from cookie
	sessionID, ok := middleware.GetSessionID(r)
	if ok {
		h.sessionStore.DeleteSession(sessionID)
	}

	// Clear session cookie
	middleware.ClearSessionCookie(w)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Register handles POST /api/auth/register
func (h *WebHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Email     string `json:"email"`
		Password  string `json:"password"`
		Nom       string `json:"nom"`
		Prenom    string `json:"prenom"`
		Adr1      string `json:"adr1"`
		Adr2      string `json:"adr2"`
		Cp        string `json:"cp"`
		Ville     string `json:"ville"`
		Phone     string `json:"phone"`
		Question  string `json:"question"`
		Reponse   string `json:"reponse"`
		NoSerie   string `json:"no_serie"`
		SendInfos bool   `json:"send_infos"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create user model
	user := &models.User{
		Mail:      req.Email,
		Password:  req.Password, // Will be hashed in Register
		Nom:       req.Nom,
		Prenom:    req.Prenom,
		Adr1:      req.Adr1,
		Adr2:      req.Adr2,
		Cp:        req.Cp,
		Ville:     req.Ville,
		Phone:     req.Phone,
		Question:  req.Question,
		Reponse:   req.Reponse, // Will be hashed in Register
		SendInfos: req.SendInfos,
	}

	// Register user
	if err := h.userService.Register(user, req.NoSerie); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return success
	response := map[string]interface{}{
		"status": "ok",
		"guid":   user.Guid,
		"message": "Account created. Please check your email to validate your account.",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// ValidateAccount handles GET /api/auth/validate
func (h *WebHandler) ValidateAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	guid := r.URL.Query().Get("guid")
	email := r.URL.Query().Get("email")

	if guid == "" || email == "" {
		http.Error(w, "guid and email parameters required", http.StatusBadRequest)
		return
	}

	// Validate account and generate code
	pkey, err := h.userService.ValidateAccount(guid, email, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Format code for display (like legacy: "1234  5678  9012  3456...")
	formattedCode := formatActivationCode(pkey)

	response := map[string]interface{}{
		"status": "ok",
		"code":   formattedCode,
		"message": "Account validated. Use this code on your Essensys control panel.",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// ForgotPassword handles POST /api/auth/forgot-password
func (h *WebHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Email string `json:"email"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Generate new password
	newPassword, err := h.userService.ForgotPassword(req.Email)
	if err != nil {
		// Don't reveal if email exists or not (security)
		response := map[string]string{
			"status": "ok",
			"message": "If the email exists, a new password has been sent.",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
		return
	}

	// TODO: Send email with new password
	// For now, log it (in production, only send by email)
	log.Printf("[FORGOT_PASSWORD] New password for %s: %s", req.Email, newPassword)

	response := map[string]string{
		"status": "ok",
		"message": "A new password has been sent to your email.",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetCurrentUser handles GET /api/user/me
func (h *WebHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user ID from context (set by WebAuth middleware)
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get user from database
	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Return user info (without sensitive data)
	response := map[string]interface{}{
		"user": map[string]interface{}{
			"id":        user.ID,
			"email":     user.Mail,
			"nom":       user.Nom,
			"prenom":    user.Prenom,
			"adr1":      user.Adr1,
			"adr2":      user.Adr2,
			"cp":        user.Cp,
			"ville":     user.Ville,
			"phone":     user.Phone,
			"question":  user.Question,
			"send_infos": user.SendInfos,
			"machine_id": user.MachineID,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// UpdateCurrentUser handles PUT /api/user/me
func (h *WebHandler) UpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user ID from context
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Nom              string `json:"nom"`
		Prenom           string `json:"prenom"`
		Adr1             string `json:"adr1"`
		Adr2             string `json:"adr2"`
		Cp               string `json:"cp"`
		Ville            string `json:"ville"`
		Phone            string `json:"phone"`
		Question         string `json:"question"`
		Reponse          string `json:"reponse"`
		CurrentPassword  string `json:"current_password"`
		NewPassword      string `json:"new_password"`
		CurrentResponse  string `json:"current_response"`
		SendInfos        bool   `json:"send_infos"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Get existing user
	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Update user fields
	user.Nom = req.Nom
	user.Prenom = req.Prenom
	user.Adr1 = req.Adr1
	user.Adr2 = req.Adr2
	user.Cp = req.Cp
	user.Ville = req.Ville
	user.Phone = req.Phone
	user.SendInfos = req.SendInfos

	// Update user
	if err := h.userService.UpdateUser(user, req.CurrentPassword, req.NewPassword, req.Question, req.Reponse, req.CurrentResponse); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := map[string]string{
		"status": "ok",
		"message": "User updated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// CloseAccount handles POST /api/user/close-account
func (h *WebHandler) CloseAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user ID from context
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Close account
	if err := h.userService.CloseAccount(userID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete session
	sessionID, _ := middleware.GetSessionID(r)
	if sessionID != "" {
		h.sessionStore.DeleteSession(sessionID)
	}

	// Clear session cookie
	middleware.ClearSessionCookie(w)

	response := map[string]string{
		"status": "ok",
		"message": "Account closed successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// TestQuestion handles POST /api/user/test-question
func (h *WebHandler) TestQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user ID from context
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Response string `json:"response"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Test question
	valid, err := h.userService.TestQuestion(userID, req.Response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"response_is_ok": valid,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// formatActivationCode formats the activation code like legacy: "1234  5678  9012..."
func formatActivationCode(code string) string {
	if len(code) != 32 {
		return code
	}
	
	// Format as: "1234  5678  9012  3456  7890  1234  5678  9012"
	formatted := ""
	for i := 0; i < len(code); i += 4 {
		if i > 0 {
			formatted += "  "
		}
		formatted += code[i : i+4]
	}
	return formatted
}

// PostWebActions handles POST /api/web/actions
// This endpoint receives actions from the modern web frontend
// It performs logic (splitting alarm code, queuing actions) before sending to the board
func (h *WebHandler) PostWebActions(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Get user ID from context
    userID, ok := middleware.GetUserID(r)
    if !ok {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    // Get user to find machine ID (or client ID)
    user, err := h.userRepo.GetByID(userID)
    if err != nil {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }
    
    // Use "default" as Safe Default for now.
    clientID := "default"
    _ = user // suppress unused check if we don't use it for clientID right now

    // Parse request body
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Failed to read request body", http.StatusBadRequest)
        return
    }
    defer r.Body.Close()

    var req struct {
        Alarme     string `json:"alarme"`     // "on" or "off"
        CodeAlarme string `json:"codealarme"` // 4 digit code
    }

    if err := json.Unmarshal(body, &req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Process Alarm Action
    if req.Alarme != "" && req.CodeAlarme != "" && len(req.CodeAlarme) == 4 {
        log.Printf("[WEB] Received Alarm Action: %s with code %s", req.Alarme, req.CodeAlarme)

        // Split Code into LSB (1st & 2nd digits) and MSB (3rd & 4th digits)
        lsb := req.CodeAlarme[0:2]
        msb := req.CodeAlarme[2:4]

        // Command value: "1" for ON, "0" for OFF
        cmd := "0"
        if req.Alarme == "on" {
            cmd = "1"
        }

        params := []protocol.ExchangeKV{
            {K: 409, V: cmd},
            {K: 410, V: lsb},
            {K: 411, V: msb},
            // Reset Authorization status to 0 (Pending)
            {K: 307, V: "0"}, 
        }

        guid, err := h.actionService.AddAction(clientID, params)
        if err != nil {
            http.Error(w, "Failed to queue action", http.StatusInternalServerError)
            return
        }
        
        log.Printf("[WEB] Queued Alarm Action GUID: %s", guid)
    }

    response := map[string]string{
        "status": "ok",
        "message": "Action sent to queue",
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}




