package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyConcurrencyLimitRepository(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	u := mustCreateAPIKeyRepoUser(t, ctx, client, "concurrency-limit@test.com")
	g, err := client.Group.Create().SetName("limit-group").Save(ctx)
	require.NoError(t, err)
	// The Ent default applies even when a caller does not set the field.
	entity, err := client.APIKey.Create().SetUserID(u.ID).SetKey("sk-default-limit").SetName("default").Save(ctx)
	require.NoError(t, err)
	require.Zero(t, entity.ConcurrencyLimit)
	key := &service.APIKey{UserID: u.ID, GroupID: &g.ID, Key: "sk-concurrency-limit", Name: "limited", Status: service.StatusActive, ConcurrencyLimit: 7}
	require.NoError(t, repo.Create(ctx, key))
	for _, limit := range []int{7, 11, 0} {
		key.ConcurrencyLimit = limit
		if limit != 7 {
			require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{ConcurrencyLimit: true}))
		}
		for _, get := range []func() (*service.APIKey, error){
			func() (*service.APIKey, error) { return repo.GetByID(ctx, key.ID) },
			func() (*service.APIKey, error) { return repo.GetByKey(ctx, key.Key) },
			func() (*service.APIKey, error) { return repo.GetByKeyForAuth(ctx, key.Key) },
		} {
			got, err := get()
			require.NoError(t, err)
			require.Equal(t, limit, got.ConcurrencyLimit)
		}
		params := pagination.PaginationParams{Page: 1, PageSize: 20}
		byUser, _, err := repo.ListByUserID(ctx, u.ID, params, service.APIKeyListFilters{Search: "limited"})
		require.NoError(t, err)
		byGroup, _, err := repo.ListByGroupID(ctx, g.ID, params)
		require.NoError(t, err)
		search, err := repo.SearchAPIKeys(ctx, u.ID, "limited", 20)
		require.NoError(t, err)
		for _, keys := range [][]service.APIKey{byUser, byGroup, search} {
			require.Len(t, keys, 1)
			require.Equal(t, limit, keys[0].ConcurrencyLimit)
		}
	}
	// A stale edit must not overwrite columns owned by another update or billing.
	require.NoError(t, client.APIKey.UpdateOneID(key.ID).SetConcurrencyLimit(8).SetQuotaUsed(23).Exec(ctx))
	key.Name = "renamed"
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{Name: true}))
	saved, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, 8, saved.ConcurrencyLimit)
	key.ConcurrencyLimit = 4
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{ConcurrencyLimit: true}))
	saved, err = repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, 4, saved.ConcurrencyLimit)
	require.Equal(t, 23.0, saved.QuotaUsed)
	require.Equal(t, "renamed", saved.Name)
	key.ConcurrencyLimit = -1
	require.Error(t, repo.Update(ctx, key, service.APIKeyUpdateFields{ConcurrencyLimit: true}))
	key.Key = "sk-negative-limit"
	require.Error(t, repo.Create(ctx, key))
}

func TestAPIKeyConcurrencyLimitRedisSerialization(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &apiKeyCache{rdb: rdb}
	ctx := context.Background()
	for _, limit := range []int{7, 0} {
		entry := &service.APIKeyAuthCacheEntry{Snapshot: &service.APIKeyAuthSnapshot{ConcurrencyLimit: limit}}
		require.NoError(t, cache.SetAuthCache(ctx, "limit", entry, time.Minute))
		got, err := cache.GetAuthCache(ctx, "limit")
		require.NoError(t, err)
		require.Equal(t, limit, got.Snapshot.ConcurrencyLimit)
		require.NoError(t, cache.DeleteAuthCache(ctx, "limit"))
		_, err = cache.GetAuthCache(ctx, "limit")
		require.ErrorIs(t, err, redis.Nil)
	}
}
