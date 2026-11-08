//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type createLimitAPIKeyRepoStub struct {
	*apiKeyRepoStub
	activeCount int64
	countErr    error
	created     []*APIKey
}

func (s *createLimitAPIKeyRepoStub) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	return s.activeCount, s.countErr
}

func (s *createLimitAPIKeyRepoStub) ExistsByKey(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (s *createLimitAPIKeyRepoStub) Create(ctx context.Context, key *APIKey) error {
	s.created = append(s.created, key)
	return nil
}

type createLimitCacheStub struct {
	*apiKeyCacheStub
	createCounts map[int64]int64
	incrErr      error
	windows      []time.Duration
}

func (s *createLimitCacheStub) IncrementCreateCount(ctx context.Context, userID int64, window time.Duration) (int64, error) {
	s.windows = append(s.windows, window)
	if s.incrErr != nil {
		return 0, s.incrErr
	}
	s.createCounts[userID]++
	return s.createCounts[userID], nil
}

func newCreateLimitService(repo *createLimitAPIKeyRepoStub, cache *createLimitCacheStub, maxActive, maxPerHour int) *APIKeyService {
	cfg := &config.Config{}
	cfg.APIKeyCreate.MaxActivePerUser = maxActive
	cfg.APIKeyCreate.MaxPerUserPerHour = maxPerHour
	return &APIKeyService{
		apiKeyRepo: repo,
		userRepo:   &userRepoStub{user: &User{ID: 7}},
		cache:      cache,
		cfg:        cfg,
	}
}

func newCreateLimitStubs() (*createLimitAPIKeyRepoStub, *createLimitCacheStub) {
	return &createLimitAPIKeyRepoStub{apiKeyRepoStub: &apiKeyRepoStub{}},
		&createLimitCacheStub{apiKeyCacheStub: &apiKeyCacheStub{}, createCounts: map[int64]int64{}}
}

func TestAPIKeyServiceCreate_RateLimitsGeneratedKeys(t *testing.T) {
	repo, cache := newCreateLimitStubs()
	svc := newCreateLimitService(repo, cache, 0, 3)

	for i := 0; i < 3; i++ {
		_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
		require.NoError(t, err)
	}
	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
	require.ErrorIs(t, err, ErrAPIKeyCreateLimited)
	require.Len(t, repo.created, 3)
	require.Equal(t, apiKeyCreateCountWindow, cache.windows[0])
}

func TestAPIKeyServiceCreate_RateLimitsCustomKeys(t *testing.T) {
	repo, cache := newCreateLimitStubs()
	svc := newCreateLimitService(repo, cache, 0, 1)
	custom := "sk-custom-key-0123456789"

	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k", CustomKey: &custom})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k", CustomKey: &custom})
	require.ErrorIs(t, err, ErrAPIKeyCreateLimited)
	require.Len(t, repo.created, 1)
}

// 删除 Key 释放数量名额，但不返还创建次数，删建循环仍受每小时次数限制。
func TestAPIKeyServiceCreate_DeleteDoesNotRefundCreateCount(t *testing.T) {
	repo, cache := newCreateLimitStubs()
	repo.apiKey = &APIKey{ID: 1, UserID: 7, Key: "k"}
	svc := newCreateLimitService(repo, cache, 1, 2)

	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
	require.NoError(t, err)
	repo.activeCount = 1
	_, err = svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
	require.ErrorIs(t, err, ErrAPIKeyCountExceeded)

	require.NoError(t, svc.Delete(context.Background(), 1, 7))
	repo.activeCount = 0
	_, err = svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
	require.NoError(t, err)

	repo.activeCount = 0
	_, err = svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
	require.ErrorIs(t, err, ErrAPIKeyCreateLimited)
	require.Len(t, repo.created, 2)
}

func TestAPIKeyServiceCreate_ActiveLimitDoesNotConsumeCreateCount(t *testing.T) {
	repo, cache := newCreateLimitStubs()
	repo.activeCount = 5
	svc := newCreateLimitService(repo, cache, 5, 10)

	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
	require.ErrorIs(t, err, ErrAPIKeyCountExceeded)
	require.Zero(t, cache.createCounts[7])
}

func TestAPIKeyServiceCreate_ZeroLimitsDisableChecks(t *testing.T) {
	repo, cache := newCreateLimitStubs()
	repo.countErr = errors.New("must not count")
	svc := newCreateLimitService(repo, cache, 0, 0)

	for i := 0; i < 5; i++ {
		_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
		require.NoError(t, err)
	}
	require.Empty(t, cache.windows)
}

func TestAPIKeyServiceCreate_CountErrorFailsClosed(t *testing.T) {
	repo, cache := newCreateLimitStubs()
	repo.countErr = errors.New("db down")
	svc := newCreateLimitService(repo, cache, 5, 0)

	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
	require.Error(t, err)
	require.Empty(t, repo.created)
}

func TestAPIKeyServiceCreate_RedisErrorFailsOpen(t *testing.T) {
	repo, cache := newCreateLimitStubs()
	cache.incrErr = errors.New("redis down")
	svc := newCreateLimitService(repo, cache, 0, 1)

	for i := 0; i < 3; i++ {
		_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
		require.NoError(t, err)
	}
}
