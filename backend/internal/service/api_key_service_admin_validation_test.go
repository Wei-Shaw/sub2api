package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyServiceCreateRejectsConflictingExpirationsBeforeRepositoryAccess(t *testing.T) {
	days := 30
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err := (&APIKeyService{}).Create(context.Background(), 1, CreateAPIKeyRequest{
		ExpiresInDays: &days,
		ExpiresAt:     &expiresAt,
	})
	require.ErrorIs(t, err, ErrAPIKeyExpiryConflict)
}

func TestAPIKeyServiceCreateRejectsNegativeLimitsBeforeRepositoryAccess(t *testing.T) {
	_, err := (&APIKeyService{}).Create(context.Background(), 1, CreateAPIKeyRequest{Quota: -1})
	require.ErrorIs(t, err, ErrAPIKeyInvalidLimit)
}

func TestAPIKeyServiceUpdateRejectsNegativeLimitsBeforeRepositoryAccess(t *testing.T) {
	negative := -1.0
	_, err := (&APIKeyService{}).Update(context.Background(), 1, 1, UpdateAPIKeyRequest{RateLimit5h: &negative})
	require.ErrorIs(t, err, ErrAPIKeyInvalidLimit)
}

func TestAPIKeyServiceValidateCustomKeyPrefixUsesDefaultPrefix(t *testing.T) {
	svc := &APIKeyService{}
	require.NoError(t, svc.ValidateCustomKeyPrefix("sk-custom-1234567890"))
	require.ErrorIs(t, svc.ValidateCustomKeyPrefix("custom-1234567890"), ErrAPIKeyInvalidPrefix)
}
