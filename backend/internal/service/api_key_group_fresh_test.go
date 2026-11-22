package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type keyGroupCurrentRepository struct {
	APIKeyRepository
	keys      map[string]*APIKey
	authReads map[string]int
}

func (r *keyGroupCurrentRepository) GetByKey(_ context.Context, key string) (*APIKey, error) {
	current := r.keys[key]
	if current == nil {
		return nil, ErrAPIKeyNotFound
	}
	copy := *current
	return &copy, nil
}

func (r *keyGroupCurrentRepository) GetByKeyForAuth(ctx context.Context, key string) (*APIKey, error) {
	r.authReads[key]++
	return r.GetByKey(ctx, key)
}

func newKeyGroupCachedService(t *testing.T) (*APIKeyService, *keyGroupCurrentRepository) {
	t.Helper()
	groupID := int64(8)
	group := &Group{ID: groupID, Name: "old-group", Platform: PlatformOpenAI, Hydrated: true, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1}
	user := &User{ID: 7, Role: RoleUser, Status: StatusActive, Balance: 10}
	repo := &keyGroupCurrentRepository{keys: map[string]*APIKey{}, authReads: map[string]int{}}
	for i, key := range []string{"changing-key", "other-key"} {
		repo.keys[key] = &APIKey{ID: int64(100 + i), Key: key, UserID: user.ID, User: user, Status: StatusActive, GroupID: &groupID, Group: group}
	}
	// Ristretto includes entry overhead in MaxCost; leave room for both keys.
	cfg := &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{L1Size: 4096, L1TTLSeconds: 3600}}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	require.NotNil(t, svc.authCacheL1)
	t.Cleanup(func() { svc.authCacheL1.Close() })
	return svc, repo
}

func TestAPIKeyGroupDiscoveryRefreshesOnlyItsKeysRealAuthCache(t *testing.T) {
	svc, repo := newKeyGroupCachedService(t)
	ctx := context.Background()
	for _, key := range []string{"changing-key", "other-key"} {
		_, err := svc.GetByKey(ctx, key)
		require.NoError(t, err)
	}
	svc.authCacheL1.Wait()
	newGroupID := int64(9)
	repo.keys["changing-key"].GroupID = &newGroupID
	repo.keys["changing-key"].Group = &Group{ID: 9, Name: "new-group", Platform: PlatformOpenAI, Hydrated: true, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1}
	stale, err := svc.GetByKey(ctx, "changing-key")
	require.NoError(t, err)
	require.Equal(t, int64(8), stale.Group.ID)
	require.Equal(t, 1, repo.authReads["changing-key"])

	metadataKey, err := svc.GetByKeyUncached(ctx, "changing-key")
	require.NoError(t, err)
	require.Equal(t, int64(9), metadataKey.Group.ID)
	nextAuth, err := svc.GetByKey(ctx, "changing-key")
	require.NoError(t, err)
	require.Equal(t, int64(9), nextAuth.Group.ID)
	require.Equal(t, 2, repo.authReads["changing-key"])
	other, err := svc.GetByKey(ctx, "other-key")
	require.NoError(t, err)
	require.Equal(t, int64(8), other.Group.ID)
	require.Equal(t, 1, repo.authReads["other-key"])
}

func TestAPIKeyGroupDiscoveryClearsRevokedAndDeletedKeysOldAuthCache(t *testing.T) {
	for _, change := range []string{"revoked", "deleted"} {
		t.Run(change, func(t *testing.T) {
			svc, repo := newKeyGroupCachedService(t)
			ctx := context.Background()
			_, err := svc.GetByKey(ctx, "changing-key")
			require.NoError(t, err)
			svc.authCacheL1.Wait()
			if change == "revoked" {
				repo.keys["changing-key"].Status = StatusDisabled
			} else {
				delete(repo.keys, "changing-key")
			}

			_, metadataErr := svc.GetByKeyUncached(ctx, "changing-key")
			nextAuth, authErr := svc.GetByKey(ctx, "changing-key")
			if change == "revoked" {
				require.NoError(t, metadataErr)
				require.NoError(t, authErr)
				require.Equal(t, StatusDisabled, nextAuth.Status)
			} else {
				require.True(t, errors.Is(metadataErr, ErrAPIKeyNotFound))
				require.True(t, errors.Is(authErr, ErrAPIKeyNotFound))
			}
			require.Equal(t, 2, repo.authReads["changing-key"])
		})
	}
}
