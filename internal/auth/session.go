package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
)

// Session represents a user session
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Created   time.Time `json:"created"`
	Expires   time.Time `json:"expires"`
	LastUsed  time.Time `json:"lastUsed"`
	IPAddress string    `json:"ipAddress"`
	UserAgent string    `json:"userAgent"`
	Active    bool      `json:"active"`
}

// SessionManager handles session creation, validation, and cleanup
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session for a user
func (m *SessionManager) CreateSession(ctx context.Context, userID string, ipAddress string, userAgent string, validityHours int) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate session ID
	sessionBytes := make([]byte, 32)
	if _, err := rand.Read(sessionBytes); err != nil {
		return nil, i18n.WrapError(ctx, err, i18n.MsgFailedToGenerateSession)
	}
	sessionID := base64.URLEncoding.EncodeToString(sessionBytes)

	// Create session
	now := time.Now()
	session := &Session{
		ID:        sessionID,
		UserID:    userID,
		Created:   now,
		Expires:   now.Add(time.Duration(validityHours) * time.Hour),
		LastUsed:  now,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Active:    true,
	}

	// Store session
	m.sessions[sessionID] = session

	return session, nil
}

// ValidateSession validates a session
func (m *SessionManager) ValidateSession(ctx context.Context, sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, i18n.NewError(ctx, i18n.MsgInvalidSession)
	}

	// Check if session is active
	if !session.Active {
		return nil, i18n.NewError(ctx, i18n.MsgInactiveSession)
	}

	// Check if session is expired
	if time.Now().After(session.Expires) {
		return nil, i18n.NewError(ctx, i18n.MsgExpiredSession)
	}

	// Update last used time
	session.LastUsed = time.Now()

	return session, nil
}

// RevokeSession revokes a session
func (m *SessionManager) RevokeSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return i18n.NewError(ctx, i18n.MsgInvalidSession)
	}

	session.Active = false
	return nil
}

// RevokeUserSessions revokes all sessions for a user
func (m *SessionManager) RevokeUserSessions(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, session := range m.sessions {
		if session.UserID == userID {
			session.Active = false
		}
	}

	return nil
}

// GetUserSessions returns all active sessions for a user
func (m *SessionManager) GetUserSessions(ctx context.Context, userID string) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var userSessions []*Session
	for _, session := range m.sessions {
		if session.UserID == userID && session.Active {
			userSessions = append(userSessions, session)
		}
	}

	return userSessions, nil
}

// CleanupExpiredSessions removes expired sessions
func (m *SessionManager) CleanupExpiredSessions(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for sessionID, session := range m.sessions {
		if now.After(session.Expires) {
			delete(m.sessions, sessionID)
		}
	}

	return nil
}

// GetSessionByID returns a session by its ID
func (m *SessionManager) GetSessionByID(ctx context.Context, sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, i18n.NewError(ctx, i18n.MsgInvalidSession)
	}

	return session, nil
}

// UpdateSessionIP updates the IP address of a session
func (m *SessionManager) UpdateSessionIP(ctx context.Context, sessionID string, newIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return i18n.NewError(ctx, i18n.MsgInvalidSession)
	}

	session.IPAddress = newIP
	return nil
} 