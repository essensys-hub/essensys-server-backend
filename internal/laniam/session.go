package laniam

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

// RememberTTL est la durée fixe d'une session « Se souvenir de moi » : elle part de
// la saisie du mot de passe et n'est jamais prolongée par l'usage.
const RememberTTL = 30 * 24 * time.Hour

const maxUserAgentChars = 255

var ErrRememberUnavailable = errors.New("remember sessions unavailable")

type Session struct {
	ID           string
	UserID       int
	User         *models.LanUser
	CreatedAt    time.Time
	LastActivity time.Time
	ExpiresAt    time.Time
	Remembered   bool
}

// UserLoader relit un utilisateur LAN, pour restaurer une session mémorisée après redémarrage.
type UserLoader func(userID int) (*models.LanUser, error)

type SessionStore struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	ttl        time.Duration
	slidingMax time.Duration
	now        func() time.Time
	remember   RememberStore
	loadUser   UserLoader
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
		now:        time.Now,
	}
}

// SetClock remplace l'horloge (tests).
func (s *SessionStore) SetClock(now func() time.Time) {
	s.now = now
}

// SetRememberStore active les sessions mémorisées. À appeler avant de servir des requêtes.
func (s *SessionStore) SetRememberStore(rs RememberStore, load UserLoader) {
	s.remember = rs
	s.loadUser = load
}

func (s *SessionStore) RememberEnabled() bool {
	return s.remember != nil && s.loadUser != nil
}

func (s *SessionStore) CreateSession(user *models.LanUser) (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	now := s.now()
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

// CreateRememberedSession ouvre une session de 30 jours fixes, enregistrée en base avant
// d'être annoncée : si l'écriture échoue, la connexion échoue.
func (s *SessionStore) CreateRememberedSession(user *models.LanUser, userAgent string) (*Session, error) {
	if !s.RememberEnabled() {
		return nil, ErrRememberUnavailable
	}
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	now := s.now()
	sess := &Session{
		ID:           id,
		UserID:       user.ID,
		User:         user,
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now.Add(RememberTTL),
		Remembered:   true,
	}
	if err := s.remember.Create(HashSessionID(id), user.ID, sess.CreatedAt, sess.ExpiresAt, truncateChars(userAgent, maxUserAgentChars)); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *SessionStore) GetSession(sessionID string) (*Session, bool) {
	now := s.now()
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if ok {
		if sess.Remembered {
			if !now.Before(sess.ExpiresAt) {
				delete(s.sessions, sessionID)
				s.mu.Unlock()
				_ = s.deleteRemembered(sessionID)
				return nil, false
			}
			sess.LastActivity = now
			s.mu.Unlock()
			return sess, true
		}
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, sessionID)
			s.mu.Unlock()
			return nil, false
		}
		sess.LastActivity = now
		sess.ExpiresAt = now.Add(s.ttl)
		if sess.ExpiresAt.Sub(sess.CreatedAt) > s.slidingMax {
			sess.ExpiresAt = sess.CreatedAt.Add(s.slidingMax)
		}
		s.mu.Unlock()
		return sess, true
	}
	s.mu.Unlock()
	if !s.RememberEnabled() {
		return nil, false
	}
	return s.restoreRemembered(sessionID, now)
}

// restoreRemembered relit en base une session mémorisée inconnue de la mémoire (redémarrage).
// La date de fin vient de la base : l'usage ne la prolonge jamais.
func (s *SessionStore) restoreRemembered(sessionID string, now time.Time) (*Session, bool) {
	hash := HashSessionID(sessionID)
	rec, err := s.remember.FindByHash(hash)
	if err != nil {
		log.Printf("laniam: remembered session lookup failed: %v", err)
		return nil, false
	}
	if rec == nil {
		return nil, false
	}
	if !now.Before(rec.ExpiresAt) {
		_ = s.remember.DeleteByHash(hash)
		return nil, false
	}
	user, err := s.loadUser(rec.UserID)
	if err != nil {
		log.Printf("laniam: remembered session user load failed: %v", err)
		return nil, false
	}
	if user == nil || user.IsDisabled() {
		_ = s.remember.DeleteByHash(hash)
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[sessionID]; ok {
		return existing, true
	}
	sess := &Session{
		ID:           sessionID,
		UserID:       rec.UserID,
		User:         user,
		CreatedAt:    rec.CreatedAt,
		LastActivity: now,
		ExpiresAt:    rec.ExpiresAt,
		Remembered:   true,
	}
	s.sessions[sessionID] = sess
	return sess, true
}

// DeleteSession supprime la session en mémoire et sa ligne en base. L'erreur de base est
// renvoyée : une révocation qui échoue ne doit pas passer pour réussie.
func (s *SessionStore) DeleteSession(sessionID string) error {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	return s.deleteRemembered(sessionID)
}

func (s *SessionStore) DeleteSessionsForUser(userID int) error {
	s.mu.Lock()
	for id, sess := range s.sessions {
		if sess.UserID == userID {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
	if s.remember == nil {
		return nil
	}
	return s.remember.DeleteByUser(userID)
}

func (s *SessionStore) deleteRemembered(sessionID string) error {
	if s.remember == nil {
		return nil
	}
	return s.remember.DeleteByHash(HashSessionID(sessionID))
}

// PurgeExpiredRemembered supprime de la base les sessions mémorisées expirées.
func (s *SessionStore) PurgeExpiredRemembered() (int64, error) {
	if s.remember == nil {
		return 0, nil
	}
	return s.remember.PurgeExpired(s.now())
}

// RunRememberPurge purge au démarrage puis à chaque intervalle, jusqu'à l'annulation de ctx.
func (s *SessionStore) RunRememberPurge(ctx context.Context, interval time.Duration) {
	purge := func() {
		n, err := s.PurgeExpiredRemembered()
		if err != nil {
			log.Printf("laniam: remembered sessions purge failed: %v", err)
			return
		}
		if n > 0 {
			log.Printf("laniam: purged %d expired remembered sessions", n)
		}
	}
	purge()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purge()
		}
	}
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

func (s *SessionStore) RememberCookieMaxAgeSeconds() int {
	return int(RememberTTL.Seconds())
}

func truncateChars(v string, max int) string {
	r := []rune(v)
	if len(r) <= max {
		return v
	}
	return string(r[:max])
}
