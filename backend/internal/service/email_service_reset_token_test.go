//go:build unit

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

type resetTokenCacheStub struct {
	emailCacheStub
	stored       *PasswordResetTokenData
	consumedHash string
}

func (s *resetTokenCacheStub) GetPasswordResetToken(context.Context, string) (*PasswordResetTokenData, error) {
	return s.stored, nil
}

func (s *resetTokenCacheStub) ConsumePasswordResetToken(_ context.Context, _ string, tokenHash string) (bool, error) {
	s.consumedHash = tokenHash
	if s.stored == nil || s.stored.Token != tokenHash {
		return false, nil
	}
	s.stored = nil
	return true, nil
}

func TestConsumePasswordResetToken_ComparesHashNotPlaintext(t *testing.T) {
	token := "deadbeef"
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	require.Equal(t, hash, hashPasswordResetToken(token))
	require.NotEqual(t, token, hashPasswordResetToken(token))

	cache := &resetTokenCacheStub{stored: &PasswordResetTokenData{Token: hash}}
	svc := NewEmailService(nil, cache)

	require.NoError(t, svc.ConsumePasswordResetToken(context.Background(), "a@b.c", token))
	require.Equal(t, hash, cache.consumedHash)
	require.ErrorIs(t, svc.ConsumePasswordResetToken(context.Background(), "a@b.c", token), ErrInvalidResetToken)

	// A legacy plaintext value (issued before upgrade) no longer validates.
	cache.stored = &PasswordResetTokenData{Token: token}
	require.ErrorIs(t, svc.ConsumePasswordResetToken(context.Background(), "a@b.c", token), ErrInvalidResetToken)
}
