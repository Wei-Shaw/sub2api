package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	systemTokenPrefix    = "sat_"
	systemTokenHexLen    = 64 // 32 bytes → hex
	systemTokenTotalLen  = len(systemTokenPrefix) + systemTokenHexLen
)

var ErrSystemTokenNotFound = errors.New("system token not found")

// SystemTokenService manages long-lived system access tokens (系统访问令牌)
// for programmatic access to the management API without JWT login.
type SystemTokenService struct {
	userRepo UserRepository
}

func NewSystemTokenService(userRepo UserRepository) *SystemTokenService {
	return &SystemTokenService{userRepo: userRepo}
}

// IsSystemToken returns true if the token string has the sat_ prefix.
func IsSystemToken(token string) bool {
	return len(token) > len(systemTokenPrefix) && token[:len(systemTokenPrefix)] == systemTokenPrefix
}

// IsValidSystemTokenFormat performs strict format check: sat_ + 64 lowercase hex chars.
// Use before any DB lookup to prevent unauthenticated DB amplification.
func IsValidSystemTokenFormat(token string) bool {
	if len(token) != systemTokenTotalLen {
		return false
	}
	if token[:len(systemTokenPrefix)] != systemTokenPrefix {
		return false
	}
	for _, c := range token[len(systemTokenPrefix):] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// Generate creates a new system access token for the user, replacing any existing one.
// Returns the cleartext token (shown once).
func (s *SystemTokenService) Generate(ctx context.Context, userID int64) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	token := systemTokenPrefix + hex.EncodeToString(raw)
	hash := sha256Hex(token)

	if err := s.userRepo.SetSystemTokenHash(ctx, userID, &hash); err != nil {
		return "", fmt.Errorf("save system token: %w", err)
	}
	return token, nil
}

// Revoke clears the system access token for the user.
func (s *SystemTokenService) Revoke(ctx context.Context, userID int64) error {
	return s.userRepo.SetSystemTokenHash(ctx, userID, nil)
}

// HasToken reports whether the user has a system access token set.
func (s *SystemTokenService) HasToken(ctx context.Context, userID int64) (bool, error) {
	return s.userRepo.HasSystemToken(ctx, userID)
}

// GetUserIDByToken validates a system access token and returns the owner's user ID.
func (s *SystemTokenService) GetUserIDByToken(ctx context.Context, token string) (int64, error) {
	hash := sha256Hex(token)
	user, err := s.userRepo.GetUserBySystemTokenHash(ctx, hash)
	if err != nil {
		return 0, ErrSystemTokenNotFound
	}
	return user.ID, nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
