//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type concurrencyLimitRepo struct {
	APIKeyRepository
	key           APIKey
	writes, reads int
}

func (r *concurrencyLimitRepo) Create(_ context.Context, key *APIKey) error {
	key.ID = 1
	r.key = *key
	r.key.User = &User{ID: key.UserID, Status: StatusActive}
	r.writes++
	return nil
}
func (r *concurrencyLimitRepo) GetByID(context.Context, int64) (*APIKey, error) {
	key := r.key
	return &key, nil
}
func (r *concurrencyLimitRepo) GetByKeyForAuth(context.Context, string) (*APIKey, error) {
	r.reads++
	key := r.key
	return &key, nil
}
func (r *concurrencyLimitRepo) Update(_ context.Context, key *APIKey, _ APIKeyUpdateFields) error {
	r.key = *key
	r.writes++
	return nil
}

type concurrencyLimitUserRepo struct{ UserRepository }

func (*concurrencyLimitUserRepo) GetByID(_ context.Context, id int64) (*User, error) {
	return &User{ID: id}, nil
}

// Serialize L2 entries to exercise the same snapshot boundary as Redis.
type concurrencyLimitCache struct {
	authCacheStub
	data      []byte
	published []string
}

func (c *concurrencyLimitCache) GetAuthCache(context.Context, string) (*APIKeyAuthCacheEntry, error) {
	if c.data == nil {
		return nil, nil
	}
	var entry APIKeyAuthCacheEntry
	err := json.Unmarshal(c.data, &entry)
	return &entry, err
}
func (c *concurrencyLimitCache) SetAuthCache(_ context.Context, _ string, entry *APIKeyAuthCacheEntry, _ time.Duration) error {
	var err error
	c.data, err = json.Marshal(entry)
	return err
}
func (c *concurrencyLimitCache) DeleteAuthCache(_ context.Context, key string) error {
	c.data = nil
	c.deleteAuthKeys = append(c.deleteAuthKeys, key)
	return nil
}
func (c *concurrencyLimitCache) PublishAuthCacheInvalidation(_ context.Context, key string) error {
	c.published = append(c.published, key)
	return nil
}

func TestAPIKeyConcurrencyLimitCreate(t *testing.T) {
	for _, limit := range []int{0, 7, -1} {
		repo := &concurrencyLimitRepo{}
		svc := NewAPIKeyService(repo, &concurrencyLimitUserRepo{}, nil, nil, nil, nil, &config.Config{})
		key, err := svc.Create(context.Background(), 2, CreateAPIKeyRequest{Name: "test", ConcurrencyLimit: limit})
		if limit < 0 {
			require.Error(t, err)
			require.Zero(t, repo.writes)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, limit, key.ConcurrencyLimit)
		require.Equal(t, limit, repo.key.ConcurrencyLimit)
	}
}

func TestAPIKeyConcurrencyLimitUpdateAndAuthCache(t *testing.T) {
	ctx := context.Background()
	repo := &concurrencyLimitRepo{key: APIKey{ID: 1, UserID: 2, Key: "sk-limit", Status: StatusActive, ConcurrencyLimit: 5, User: &User{ID: 2}}}
	cache := &concurrencyLimitCache{}
	cfg := &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{L1Size: 100, L1TTLSeconds: 60, L2TTLSeconds: 60}}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)
	t.Cleanup(svc.authCacheL1.Close)
	for _, tc := range []struct {
		name  string
		limit *int
		want  int
	}{
		{"omitted", nil, 5}, {"positive", concurrencyLimitPtr(9), 9}, {"zero", concurrencyLimitPtr(0), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.GetByKey(ctx, repo.key.Key)
			require.NoError(t, err)
			svc.authCacheL1.Wait()
			_, ok := svc.authCacheL1.Get(svc.authCacheKey(repo.key.Key))
			require.True(t, ok)
			key, err := svc.Update(ctx, 1, 2, UpdateAPIKeyRequest{ConcurrencyLimit: tc.limit})
			require.NoError(t, err)
			require.Equal(t, tc.want, key.ConcurrencyLimit)
			require.Nil(t, cache.data)
			require.Equal(t, svc.authCacheKey(repo.key.Key), cache.published[len(cache.published)-1])
			svc.authCacheL1.Wait()
			_, ok = svc.authCacheL1.Get(svc.authCacheKey(repo.key.Key))
			require.False(t, ok)
			key, err = svc.GetByKey(ctx, repo.key.Key)
			require.NoError(t, err)
			require.Equal(t, tc.want, key.ConcurrencyLimit)
			svc.authCacheL1.Wait()
			reads := repo.reads
			key, err = svc.GetByKey(ctx, repo.key.Key)
			require.NoError(t, err)
			require.Equal(t, tc.want, key.ConcurrencyLimit)
			require.Equal(t, reads, repo.reads, "L1 hit")
			svc.authCacheL1.Clear()
			key, err = svc.GetByKey(ctx, repo.key.Key)
			require.NoError(t, err)
			require.Equal(t, tc.want, key.ConcurrencyLimit)
			require.Equal(t, reads, repo.reads, "serialized L2 hit")
		})
	}
	writes := repo.writes
	_, err := svc.Update(ctx, 1, 2, UpdateAPIKeyRequest{ConcurrencyLimit: concurrencyLimitPtr(-1)})
	require.Error(t, err)
	require.Equal(t, writes, repo.writes)
	// An upstream v24 entry must reload the configured value instead of implying unlimited.
	svc.authCacheL1.Clear()
	repo.key.ConcurrencyLimit = 12
	cache.data = []byte(`{"snapshot":{"version":24,"api_key_id":1}}`)
	key, err := svc.GetByKey(ctx, repo.key.Key)
	require.NoError(t, err)
	require.Equal(t, 12, key.ConcurrencyLimit)
}

func concurrencyLimitPtr(v int) *int { return &v }
