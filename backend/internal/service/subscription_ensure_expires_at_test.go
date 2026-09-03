//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/dgraph-io/ristretto"
	"github.com/stretchr/testify/require"
)

type ensureSubRepoStub struct {
	userSubRepoNoop
	existing  *UserSubscription
	created   *UserSubscription
	getByID   *UserSubscription
	createErr error
	getErr    error
}

func (s *ensureSubRepoStub) GetByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.existing != nil && s.existing.UserID == userID && s.existing.GroupID == groupID {
		return s.existing, nil
	}
	return nil, ErrSubscriptionNotFound
}

func (s *ensureSubRepoStub) Create(_ context.Context, sub *UserSubscription) error {
	if s.createErr != nil {
		return s.createErr
	}
	sub.ID = 99
	copied := *sub
	s.created = &copied
	return nil
}

func (s *ensureSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if s.getByID != nil {
		return s.getByID, nil
	}
	if s.created != nil && s.created.ID == id {
		return s.created, nil
	}
	return nil, ErrSubscriptionNotFound
}

func TestEnsureSubscriptionWithExpiresAt_CreateActive(t *testing.T) {
	group := &Group{ID: 10, SubscriptionType: SubscriptionTypeSubscription}
	repo := &ensureSubRepoStub{}
	svc := &SubscriptionService{
		groupRepo:   &subscriptionGroupRepoStub{group: group},
		userSubRepo: repo,
	}

	expires := time.Now().Add(48 * time.Hour)
	sub, err := svc.EnsureSubscriptionWithExpiresAt(context.Background(), 1, 10, expires, "private provision", false)
	require.NoError(t, err)
	require.NotNil(t, sub)
	require.NotNil(t, repo.created)
	require.Equal(t, SubscriptionStatusActive, repo.created.Status)
	require.Equal(t, expires.Unix(), repo.created.ExpiresAt.Unix())
	require.Equal(t, "private provision", repo.created.Notes)
}

func TestEnsureSubscriptionWithExpiresAt_CreateExpiredWhenNotAfterNow(t *testing.T) {
	group := &Group{ID: 10, SubscriptionType: SubscriptionTypeSubscription}
	repo := &ensureSubRepoStub{}
	svc := &SubscriptionService{
		groupRepo:   &subscriptionGroupRepoStub{group: group},
		userSubRepo: repo,
	}

	// 过去时刻 → expired
	expires := time.Now().Add(-time.Minute)
	sub, err := svc.EnsureSubscriptionWithExpiresAt(context.Background(), 1, 10, expires, "", false)
	require.NoError(t, err)
	require.NotNil(t, sub)
	require.Equal(t, SubscriptionStatusExpired, repo.created.Status)

	// 恰好 now（!After）→ expired
	repo.created = nil
	now := time.Now()
	sub, err = svc.EnsureSubscriptionWithExpiresAt(context.Background(), 2, 10, now, "", false)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusExpired, repo.created.Status)
}

func TestEnsureSubscriptionWithExpiresAt_IdempotentNoChange(t *testing.T) {
	group := &Group{ID: 10, SubscriptionType: SubscriptionTypeSubscription}
	originalExpires := time.Now().Add(24 * time.Hour)
	existing := &UserSubscription{
		ID:        7,
		UserID:    1,
		GroupID:   10,
		ExpiresAt: originalExpires,
		Status:    SubscriptionStatusActive,
		Notes:     "original",
	}
	repo := &ensureSubRepoStub{existing: existing}
	svc := &SubscriptionService{
		groupRepo:   &subscriptionGroupRepoStub{group: group},
		userSubRepo: repo,
	}

	sub, err := svc.EnsureSubscriptionWithExpiresAt(context.Background(), 1, 10, time.Now().Add(365*24*time.Hour), "should-not-apply", false)
	require.NoError(t, err)
	require.Equal(t, existing.ID, sub.ID)
	require.Equal(t, originalExpires, sub.ExpiresAt)
	require.Equal(t, "original", sub.Notes)
	require.Nil(t, repo.created, "must not create a second subscription")
}

func TestEnsureSubscriptionWithExpiresAt_RejectsNonSubscriptionGroup(t *testing.T) {
	group := &Group{ID: 10, SubscriptionType: SubscriptionTypeStandard}
	svc := &SubscriptionService{
		groupRepo:   &subscriptionGroupRepoStub{group: group},
		userSubRepo: &ensureSubRepoStub{},
	}

	_, err := svc.EnsureSubscriptionWithExpiresAt(context.Background(), 1, 10, time.Now().Add(time.Hour), "", false)
	require.Error(t, err)
	require.Equal(t, infraerrors.Code(ErrGroupNotSubscriptionType), infraerrors.Code(err))
}

func TestEnsureSubscriptionWithExpiresAt_PropagatesGetErrors(t *testing.T) {
	group := &Group{ID: 10, SubscriptionType: SubscriptionTypeSubscription}
	repo := &ensureSubRepoStub{getErr: errors.New("db unavailable")}
	svc := &SubscriptionService{
		groupRepo:   &subscriptionGroupRepoStub{group: group},
		userSubRepo: repo,
	}

	_, err := svc.EnsureSubscriptionWithExpiresAt(context.Background(), 1, 10, time.Now().Add(time.Hour), "", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "db unavailable")
	require.Nil(t, repo.created, "must not create when Get fails with non-not-found")
}

func TestEnsureSubscriptionWithExpiresAt_ClampsMaxExpiresAt(t *testing.T) {
	group := &Group{ID: 10, SubscriptionType: SubscriptionTypeSubscription}
	repo := &ensureSubRepoStub{}
	svc := &SubscriptionService{
		groupRepo:   &subscriptionGroupRepoStub{group: group},
		userSubRepo: repo,
	}

	farFuture := time.Date(2100, 6, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.EnsureSubscriptionWithExpiresAt(context.Background(), 1, 10, farFuture, "", false)
	require.NoError(t, err)
	require.True(t, repo.created.ExpiresAt.Equal(MaxExpiresAt))
	require.False(t, repo.created.ExpiresAt.After(MaxExpiresAt))
}

func TestEnsureSubscriptionWithExpiresAt_InvalidatesCache(t *testing.T) {
	cache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 1_000, MaxCost: 100, BufferItems: 64})
	require.NoError(t, err)
	t.Cleanup(cache.Close)

	group := &Group{ID: 10, SubscriptionType: SubscriptionTypeSubscription}
	repo := &ensureSubRepoStub{}
	svc := &SubscriptionService{
		groupRepo:   &subscriptionGroupRepoStub{group: group},
		userSubRepo: repo,
		subCacheL1:  cache,
	}

	key := subCacheKey(1, 10)
	require.True(t, cache.Set(key, &UserSubscription{ID: 1}, 1))
	cache.Wait()

	_, err = svc.EnsureSubscriptionWithExpiresAt(context.Background(), 1, 10, time.Now().Add(time.Hour), "", false)
	require.NoError(t, err)
	cache.Wait()
	_, stillCached := cache.Get(key)
	require.False(t, stillCached, "assignment cache must be invalidated on create")
}

func TestEnsureSubscriptionWithExpiresAt_DeferCacheInvalidation(t *testing.T) {
	cache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 1_000, MaxCost: 100, BufferItems: 64})
	require.NoError(t, err)
	t.Cleanup(cache.Close)

	group := &Group{ID: 10, SubscriptionType: SubscriptionTypeSubscription}
	repo := &ensureSubRepoStub{}
	svc := &SubscriptionService{
		groupRepo:   &subscriptionGroupRepoStub{group: group},
		userSubRepo: repo,
		subCacheL1:  cache,
	}

	key := subCacheKey(1, 10)
	require.True(t, cache.Set(key, &UserSubscription{ID: 1}, 1))
	cache.Wait()

	_, err = svc.EnsureSubscriptionWithExpiresAt(context.Background(), 1, 10, time.Now().Add(time.Hour), "", true)
	require.NoError(t, err)
	cache.Wait()
	_, stillCached := cache.Get(key)
	require.True(t, stillCached, "deferred invalidation must retain cache until outer owner commits")
}
