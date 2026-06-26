package coreconfig

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hyperledger/firefly-common/pkg/i18n"
)

// SecretsManager handles secure storage and retrieval of sensitive data
type SecretsManager struct {
	mu       sync.RWMutex
	secrets  map[string][]byte
	basePath string
	key      []byte
}

// NewSecretsManager creates a new secrets manager
func NewSecretsManager(basePath string, key []byte) (*SecretsManager, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes")
	}
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create secrets directory: %v", err)
	}
	return &SecretsManager{
		secrets:  make(map[string][]byte),
		basePath: basePath,
		key:      key,
	}, nil
}

// StoreSecret securely stores a secret
func (sm *SecretsManager) StoreSecret(ctx context.Context, key string, value []byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Encrypt the secret before storing
	encrypted, err := sm.encrypt(value)
	if err != nil {
		return i18n.WrapError(ctx, err, i18n.MsgFailedToEncryptSecret)
	}

	// Store in memory
	sm.secrets[key] = encrypted

	// Store on disk
	path := filepath.Join(sm.basePath, key)
	if err := os.WriteFile(path, encrypted, 0600); err != nil {
		return i18n.WrapError(ctx, err, i18n.MsgFailedToStoreSecret)
	}

	return nil
}

// GetSecret retrieves a secret
func (sm *SecretsManager) GetSecret(ctx context.Context, key string) ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Try memory first
	if secret, exists := sm.secrets[key]; exists {
		return sm.decrypt(secret)
	}

	// Try disk
	path := filepath.Join(sm.basePath, key)
	encrypted, err := os.ReadFile(path)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, i18n.MsgFailedToReadSecret)
	}

	// Store in memory for future use
	sm.secrets[key] = encrypted

	return sm.decrypt(encrypted)
}

// DeleteSecret removes a secret
func (sm *SecretsManager) DeleteSecret(ctx context.Context, key string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Remove from memory
	delete(sm.secrets, key)

	// Remove from disk
	path := filepath.Join(sm.basePath, key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return i18n.WrapError(ctx, err, i18n.MsgFailedToDeleteSecret)
	}

	return nil
}

// encrypt encrypts data using AES-GCM
func (sm *SecretsManager) encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(sm.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	// Seal the data with the nonce
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-GCM
func (sm *SecretsManager) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(sm.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := data[:gcm.NonceSize()]
	ciphertext := data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
} 