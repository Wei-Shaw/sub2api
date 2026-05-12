package service

import (
	"context"
	"time"

	"github.com/stretchr/testify/require"
	"testing"
)

type statefulIPLockCacheStub struct {
	locks map[int64]string
}

func (s *statefulIPLockCacheStub) GetCreateAttemptCount(context.Context, int64) (int, error) {
	return 0, nil
}

func (s *statefulIPLockCacheStub) IncrementCreateAttemptCount(context.Context, int64) error {
	return nil
}

func (s *statefulIPLockCacheStub) DeleteCreateAttemptCount(context.Context, int64) error {
	return nil
}

func (s *statefulIPLockCacheStub) IncrementDailyUsage(context.Context, string) error {
	return nil
}

func (s *statefulIPLockCacheStub) SetDailyUsageExpiry(context.Context, string, time.Duration) error {
	return nil
}

func (s *statefulIPLockCacheStub) GetAuthCache(context.Context, string) (*APIKeyAuthCacheEntry, error) {
	return nil, nil
}

func (s *statefulIPLockCacheStub) SetAuthCache(context.Context, string, *APIKeyAuthCacheEntry, time.Duration) error {
	return nil
}

func (s *statefulIPLockCacheStub) DeleteAuthCache(context.Context, string) error {
	return nil
}

func (s *statefulIPLockCacheStub) PublishAuthCacheInvalidation(context.Context, string) error {
	return nil
}

func (s *statefulIPLockCacheStub) SubscribeAuthCacheInvalidation(context.Context, func(string)) error {
	return nil
}

func (s *statefulIPLockCacheStub) BindAPIKeyIPLock(_ context.Context, keyID int64, clientIP string, _ time.Duration) (string, error) {
	if s.locks == nil {
		s.locks = map[int64]string{}
	}
	if lockedIP := s.locks[keyID]; lockedIP != "" {
		return lockedIP, nil
	}
	s.locks[keyID] = clientIP
	return clientIP, nil
}

func (s *statefulIPLockCacheStub) GetAPIKeyIPLock(_ context.Context, keyID int64) (string, error) {
	if s.locks == nil {
		return "", nil
	}
	return s.locks[keyID], nil
}

func (s *statefulIPLockCacheStub) RefreshAPIKeyIPLock(context.Context, int64, time.Duration) error {
	return nil
}

func (s *statefulIPLockCacheStub) ResetAPIKeyIPLock(_ context.Context, keyID int64) error {
	delete(s.locks, keyID)
	return nil
}

func TestAPIKeyServiceEnforceIPLockAutoSingleIP(t *testing.T) {
	cache := &statefulIPLockCacheStub{locks: map[int64]string{}}
	svc := &APIKeyService{cache: cache}
	apiKey := &APIKey{ID: 42, IPLockMode: IPLockModeAutoSingleIP}

	require.NoError(t, svc.EnforceIPLock(context.Background(), apiKey, "203.0.113.10"))
	lockedIP, err := svc.GetIPLock(context.Background(), apiKey.ID)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.10", lockedIP)

	require.NoError(t, svc.EnforceIPLock(context.Background(), apiKey, "203.0.113.10"))
	require.ErrorIs(t, svc.EnforceIPLock(context.Background(), apiKey, "203.0.113.11"), ErrAPIKeyIPLocked)

	require.NoError(t, svc.ResetIPLock(context.Background(), apiKey.ID))
	require.NoError(t, svc.EnforceIPLock(context.Background(), apiKey, "203.0.113.11"))
	lockedIP, err = svc.GetIPLock(context.Background(), apiKey.ID)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.11", lockedIP)
}
