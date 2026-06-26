package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
)

// APIKey represents an API key with metadata
type APIKey struct {
	Key       string    `json:"key"`
	UserID    string    `json:"userId"`
	Created   time.Time `json:"created"`
	Expires   time.Time `json:"expires"`
	LastUsed  time.Time `json:"lastUsed"`
	Rotated   bool      `json:"rotated"`
	Active    bool      `json:"active"`
}

// APIKeyManager handles API key generation, rotation, and validation
type APIKeyManager struct {
	mu       sync.RWMutex
	keys     map[string]*APIKey
	rotation map[string]string // maps old keys to new keys
}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager() *APIKeyManager {
	return &APIKeyManager{
		keys:     make(map[string]*APIKey),
		rotation: make(map[string]string),
	}
}

// GenerateKey generates a new API key
func (m *APIKeyManager) GenerateKey(ctx context.Context, userID string, validityDays int) (*APIKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate random key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, i18n.WrapError(ctx, err, i18n.MsgFailedToGenerateKey)
	}
	key := base64.URLEncoding.EncodeToString(keyBytes)

	// Create API key
	now := time.Now()
	apiKey := &APIKey{
		Key:       key,
		UserID:    userID,
		Created:   now,
		Expires:   now.AddDate(0, 0, validityDays),
		LastUsed:  now,
		Active:    true,
	}

	// Store key
	m.keys[key] = apiKey

	return apiKey, nil
}

// ValidateKey validates an API key
func (m *APIKeyManager) ValidateKey(ctx context.Context, key string) (*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	apiKey, exists := m.keys[key]
	if !exists {
		return nil, i18n.NewError(ctx, i18n.MsgInvalidAPIKey)
	}

	// Check if key is active
	if !apiKey.Active {
		return nil, i18n.NewError(ctx, i18n.MsgInactiveAPIKey)
	}

	// Check if key is expired
	if time.Now().After(apiKey.Expires) {
		return nil, i18n.NewError(ctx, i18n.MsgExpiredAPIKey)
	}

	// Update last used time
	apiKey.LastUsed = time.Now()

	return apiKey, nil
}

// RotateKey rotates an API key
func (m *APIKeyManager) RotateKey(ctx context.Context, oldKey string, validityDays int) (*APIKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate old key
	oldAPIKey, exists := m.keys[oldKey]
	if !exists {
		return nil, i18n.NewError(ctx, i18n.MsgInvalidAPIKey)
	}

	// Generate new key
	newKeyBytes := make([]byte, 32)
	if _, err := rand.Read(newKeyBytes); err != nil {
		return nil, i18n.WrapError(ctx, err, i18n.MsgFailedToGenerateKey)
	}
	newKey := base64.URLEncoding.EncodeToString(newKeyBytes)

	// Create new API key
	now := time.Now()
	newAPIKey := &APIKey{
		Key:       newKey,
		UserID:    oldAPIKey.UserID,
		Created:   now,
		Expires:   now.AddDate(0, 0, validityDays),
		LastUsed:  now,
		Active:    true,
	}

	// Mark old key as rotated
	oldAPIKey.Rotated = true
	oldAPIKey.Active = false

	// Store new key and rotation mapping
	m.keys[newKey] = newAPIKey
	m.rotation[oldKey] = newKey

	return newAPIKey, nil
}

// RevokeKey revokes an API key
func (m *APIKeyManager) RevokeKey(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	apiKey, exists := m.keys[key]
	if !exists {
		return i18n.NewError(ctx, i18n.MsgInvalidAPIKey)
	}

	apiKey.Active = false
	return nil
}

// GetUserKeys returns all API keys for a user
func (m *APIKeyManager) GetUserKeys(ctx context.Context, userID string) ([]*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var userKeys []*APIKey
	for _, key := range m.keys {
		if key.UserID == userID {
			userKeys = append(userKeys, key)
		}
	}

	return userKeys, nil
}

// CleanupExpiredKeys removes expired keys
func (m *APIKeyManager) CleanupExpiredKeys(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, apiKey := range m.keys {
		if now.After(apiKey.Expires) {
			delete(m.keys, key)
			delete(m.rotation, key)
		}
	}

	return nil
} 