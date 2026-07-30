package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

const systemTokenPrefix = "sat_"

var ErrSystemTokenNotFound = errors.New("system token not found")

// SystemTokenService manages long-lived system access tokens (系统访问令牌)
// for programmatic access to the management API without JWT login.
type SystemTokenService struct {
	db *sql.DB
}

func NewSystemTokenService(db *sql.DB) *SystemTokenService {
	return &SystemTokenService{db: db}
}

// IsSystemToken returns true if the token string looks like a system access token.
func IsSystemToken(token string) bool {
	return len(token) > len(systemTokenPrefix) && token[:len(systemTokenPrefix)] == systemTokenPrefix
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

	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET system_token_hash = $1 WHERE id = $2 AND deleted_at IS NULL`,
		hash, userID)
	if err != nil {
		return "", fmt.Errorf("save system token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", errors.New("user not found")
	}
	return token, nil
}

// Revoke clears the system access token for the user.
func (s *SystemTokenService) Revoke(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET system_token_hash = NULL WHERE id = $1 AND deleted_at IS NULL`,
		userID)
	return err
}

// HasToken reports whether the user has a system access token set.
func (s *SystemTokenService) HasToken(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT system_token_hash IS NOT NULL FROM users WHERE id = $1 AND deleted_at IS NULL`,
		userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return exists, err
}

// GetUserIDByToken validates a system access token and returns the owner's user ID.
func (s *SystemTokenService) GetUserIDByToken(ctx context.Context, token string) (int64, error) {
	hash := sha256Hex(token)
	var userID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE system_token_hash = $1 AND deleted_at IS NULL`,
		hash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrSystemTokenNotFound
	}
	if err != nil {
		return 0, err
	}
	return userID, nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
