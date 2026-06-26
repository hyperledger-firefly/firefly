package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/i18n"
)

const (
	// PasswordMinLength is the minimum length for passwords
	PasswordMinLength = 8
	// PasswordMaxLength is the maximum length for passwords
	PasswordMaxLength = 72
	// DefaultCost is the default bcrypt cost
	DefaultCost = 12
)

// PasswordManager handles password hashing and validation
type PasswordManager struct {
	cost int
}

// NewPasswordManager creates a new password manager
func NewPasswordManager(cost int) *PasswordManager {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = DefaultCost
	}
	return &PasswordManager{
		cost: cost,
	}
}

// HashPassword hashes a password using bcrypt
func (m *PasswordManager) HashPassword(ctx context.Context, password string) (string, error) {
	if err := m.validatePassword(ctx, password); err != nil {
		return "", err
	}

	// Generate a random salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", i18n.WrapError(ctx, err, i18n.MsgFailedToGenerateSalt)
	}

	// Hash the password with the salt
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), m.cost)
	if err != nil {
		return "", i18n.WrapError(ctx, err, i18n.MsgFailedToHashPassword)
	}

	return string(hashedBytes), nil
}

// ComparePasswords compares a password with its hash
func (m *PasswordManager) ComparePasswords(ctx context.Context, password, hash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return i18n.NewError(ctx, i18n.MsgInvalidPassword)
	}
	return nil
}

// ValidatePassword validates a password against security requirements
func (m *PasswordManager) ValidatePassword(ctx context.Context, password string) error {
	// Check length
	if len(password) < PasswordMinLength {
		return i18n.NewError(ctx, i18n.MsgPasswordTooShort, PasswordMinLength)
	}
	if len(password) > PasswordMaxLength {
		return i18n.NewError(ctx, i18n.MsgPasswordTooLong, PasswordMaxLength)
	}

	// Check for common passwords
	if m.isCommonPassword(password) {
		return i18n.NewError(ctx, i18n.MsgCommonPassword)
	}

	// Check for character types
	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasNumber = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?", char):
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber || !hasSpecial {
		return i18n.NewError(ctx, i18n.MsgPasswordRequirements)
	}

	return nil
}

// GenerateSecurePassword generates a secure random password
func (m *PasswordManager) GenerateSecurePassword(ctx context.Context) (string, error) {
	// Define character sets
	const (
		upperChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lowerChars = "abcdefghijklmnopqrstuvwxyz"
		numberChars = "0123456789"
		specialChars = "!@#$%^&*()_+-=[]{}|;:,.<>?"
	)

	// Generate random bytes for each character type
	password := make([]byte, PasswordMinLength)
	
	// Ensure at least one of each character type
	password[0] = upperChars[randInt(len(upperChars))]
	password[1] = lowerChars[randInt(len(lowerChars))]
	password[2] = numberChars[randInt(len(numberChars))]
	password[3] = specialChars[randInt(len(specialChars))]

	// Fill the rest with random characters from all sets
	allChars := upperChars + lowerChars + numberChars + specialChars
	for i := 4; i < PasswordMinLength; i++ {
		password[i] = allChars[randInt(len(allChars))]
	}

	// Shuffle the password
	for i := len(password) - 1; i > 0; i-- {
		j := randInt(i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password), nil
}

// isCommonPassword checks if a password is in a list of common passwords
func (m *PasswordManager) isCommonPassword(password string) bool {
	commonPasswords := []string{
		"password", "123456", "12345678", "qwerty", "abc123",
		"monkey", "letmein", "dragon", "111111", "baseball",
		"iloveyou", "trustno1", "sunshine", "master", "welcome",
		"shadow", "ashley", "football", "jesus", "michael",
		"ninja", "mustang", "password1", "123456789", "password123",
	}

	for _, common := range commonPasswords {
		if strings.ToLower(password) == common {
			return true
		}
	}
	return false
}

// randInt generates a random integer between 0 and max-1
func randInt(max int) int {
	b := make([]byte, 8)
	rand.Read(b)
	return int(b[0]) % max
} 