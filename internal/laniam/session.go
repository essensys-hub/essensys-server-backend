package laniam

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type Session struct {
	ID           string
	UserID       int
	User         *models.LanUser
	CreatedAt    time.Time
	LastActivity time.Time
	ExpiresAt    time.Time
}

type SessionStore struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	ttl        time.Duration
	slidingMax time.Duration
}

func NewSessionStore(ttlHours int) *SessionStore {
	if ttlHours <= 0 {
		ttlHours = 168
	}
	ttl := time.Duration(ttlHours) * time.Hour
	return &SessionStore{
		sessions:   make(map[string]*Session),
		ttl:        ttl,
		slidingMax: ttl,
	}
}

func (s *SessionStore) CreateSession(user *models.LanUser) (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &Session{
		ID:           id,
		UserID:       user.ID,
		User:         user,
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now.Add(s.ttl),
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *SessionStore) GetSession(sessionID string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, false
	}
	now := time.Now()
	if now.After(sess.ExpiresAt) {
		delete(s.sessions, sessionID)
		return nil, false
	}
	sess.LastActivity = now
	sess.ExpiresAt = now.Add(s.ttl)
	if sess.ExpiresAt.Sub(sess.CreatedAt) > s.slidingMax {
		sess.ExpiresAt = sess.CreatedAt.Add(s.slidingMax)
	}
	return sess, true
}

func (s *SessionStore) DeleteSession(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

func (s *SessionStore) DeleteSessionsForUser(userID int) {
	s.mu.Lock()
	for id, sess := range s.sessions {
		if sess.UserID == userID {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (s *SessionStore) CookieMaxAgeSeconds() int {
	return int(s.ttl.Seconds())
}
