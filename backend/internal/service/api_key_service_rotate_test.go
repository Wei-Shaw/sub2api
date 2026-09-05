//go:build unit

package service

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type rotateAPIKeyRepoStub struct {
	*apiKeyRepoStub
	expectedOldKey string
	rotatedTo      string
	rotateErr      error
}

func rotationTestConfig() *config.Config {
	return &config.Config{}
}

func (s *rotateAPIKeyRepoStub) RotateKey(_ context.Context, id int64, expectedKey, newKey string, rotatedAt time.Time) error {
	if s.rotateErr != nil {
		return s.rotateErr
	}
	if s.apiKey == nil || s.apiKey.ID != id || s.apiKey.Key != expectedKey {
		return ErrAPIKeyRotateConflict
	}
	s.expectedOldKey = expectedKey
	s.rotatedTo = newKey
	s.apiKey.Key = newKey
	s.apiKey.LastRotatedAt = &rotatedAt
	return nil
}

func TestAPIKeyServiceRotatePreservesRecordAndInvalidatesBothCredentials(t *testing.T) {
	groupID := int64(9)
	repo := &rotateAPIKeyRepoStub{apiKeyRepoStub: &apiKeyRepoStub{apiKey: &APIKey{
		ID: 42, UserID: 7, Key: "sk-old-credential", Name: "production",
		GroupID: &groupID, Status: StatusActive, Quota: 100, QuotaUsed: 12.5,
		RateLimit5h: 8, Usage5h: 2,
	}}}
	cache := &apiKeyCacheStub{}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, rotationTestConfig())

	rotated, err := svc.Rotate(context.Background(), 42, 7)

	require.NoError(t, err)
	require.Equal(t, int64(42), rotated.ID)
	require.Equal(t, "production", rotated.Name)
	require.Equal(t, &groupID, rotated.GroupID)
	require.Equal(t, 100.0, rotated.Quota)
	require.Equal(t, 12.5, rotated.QuotaUsed)
	require.Equal(t, 8.0, rotated.RateLimit5h)
	require.Equal(t, 2.0, rotated.Usage5h)
	require.Equal(t, "sk-old-credential", repo.expectedOldKey)
	require.NotEqual(t, repo.expectedOldKey, repo.rotatedTo)
	require.Equal(t, repo.rotatedTo, rotated.Key)
	require.NotNil(t, rotated.LastRotatedAt)
	require.Len(t, cache.deleteAuthKeys, 2)
	require.Equal(t, svc.authCacheKey("sk-old-credential"), cache.deleteAuthKeys[0])
	require.Equal(t, svc.authCacheKey(rotated.Key), cache.deleteAuthKeys[1])
	require.Equal(t, cache.deleteAuthKeys[0], cache.publishedAuthKeys[0])
	require.Equal(t, cache.deleteAuthKeys[1], cache.publishedAuthKeys[1])
}

func TestAPIKeyServiceRotateUsesAutomaticCreationCredentialFormat(t *testing.T) {
	repo := &rotateAPIKeyRepoStub{apiKeyRepoStub: &apiKeyRepoStub{apiKey: &APIKey{
		ID: 42, UserID: 7, Key: "custom-existing-credential",
	}}}
	cfg := rotationTestConfig()
	cfg.Default.APIKeyPrefix = "rk_"
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	rotated, err := svc.Rotate(context.Background(), 42, 7)

	require.NoError(t, err)
	require.Len(t, rotated.Key, len("rk_")+64)
	require.Equal(t, "rk_", rotated.Key[:len("rk_")])
	_, err = hex.DecodeString(rotated.Key[len("rk_"):])
	require.NoError(t, err)
}

func TestAPIKeyServiceRotateRejectsNonOwner(t *testing.T) {
	repo := &rotateAPIKeyRepoStub{apiKeyRepoStub: &apiKeyRepoStub{apiKey: &APIKey{
		ID: 42, UserID: 7, Key: "sk-old-credential",
	}}}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, rotationTestConfig())

	_, err := svc.Rotate(context.Background(), 42, 8)

	require.ErrorIs(t, err, ErrInsufficientPerms)
	require.Empty(t, repo.rotatedTo)
}

func TestAPIKeyServiceRotateReturnsConcurrentRotationConflict(t *testing.T) {
	repo := &rotateAPIKeyRepoStub{
		apiKeyRepoStub: &apiKeyRepoStub{apiKey: &APIKey{ID: 42, UserID: 7, Key: "sk-old-credential"}},
		rotateErr:      ErrAPIKeyRotateConflict,
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, rotationTestConfig())

	_, err := svc.Rotate(context.Background(), 42, 7)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrAPIKeyRotateConflict))
}

func TestAPIKeyServiceRotateRejectsRepositoryWithoutAtomicRotation(t *testing.T) {
	repo := &apiKeyRepoStub{apiKey: &APIKey{ID: 42, UserID: 7, Key: "sk-old-credential"}}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, rotationTestConfig())

	_, err := svc.Rotate(context.Background(), 42, 7)

	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support atomic rotation")
	require.Equal(t, "sk-old-credential", repo.apiKey.Key)
}
