package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

// Session represents a user session
type Session struct {
	ID        string
	UserID    int
	User      *models.User
	MachineID *int
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionStore manages user sessions in memory
// In production, consider using Redis or database-backed sessions
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionStore creates a new session store
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session for a user
func (s *SessionStore) CreateSession(user *models.User) (*Session, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	session := &Session{
		ID:        sessionID,
		UserID:    user.ID,
		User:      user,
		MachineID: user.MachineID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24 hours expiration
	}

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	// Cleanup expired sessions in background
	go s.cleanupExpired()

	return session, nil
}

// GetSession retrieves a session by ID
func (s *SessionStore) GetSession(sessionID string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(session.ExpiresAt) {
		// Delete expired session
		s.mu.RUnlock()
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		s.mu.RLock()
		return nil, false
	}

	return session, true
}

// DeleteSession removes a session
func (s *SessionStore) DeleteSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// cleanupExpired removes expired sessions
func (s *SessionStore) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}

// generateSessionID generates a secure random session ID
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ExtendSession extends the expiration time of a session
func (s *SessionStore) ExtendSession(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return false
	}

	session.ExpiresAt = time.Now().Add(24 * time.Hour)
	return true
}

