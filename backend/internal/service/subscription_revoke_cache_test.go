//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type revokeCacheUserSubRepoStub struct {
	userSubRepoNoop

	sub            *UserSubscription
	deleted        bool
	getActiveCalls int
}

func (r *revokeCacheUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id || r.deleted {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *revokeCacheUserSubRepoStub) Delete(_ context.Context, id int64) error {
	if r.sub == nil || r.sub.ID != id || r.deleted {
		return ErrSubscriptionNotFound
	}
	r.deleted = true
	return nil
}

func (r *revokeCacheUserSubRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	r.getActiveCalls++
	if r.deleted || r.sub == nil || r.sub.UserID != userID || r.sub.GroupID != groupID {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func TestRevokeSubscription_InvalidatesL1CacheSynchronously(t *testing.T) {
	repo := &revokeCacheUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{
			L1Size:       16,
			L1TTLSeconds: 60,
		},
	})
	t.Cleanup(svc.Stop)

	_, err := svc.GetActiveSubscription(context.Background(), 10, 20)
	require.NoError(t, err)
	svc.subCacheL1.Wait()
	require.Equal(t, 1, repo.getActiveCalls)

	err = svc.RevokeSubscription(context.Background(), 1)
	require.NoError(t, err)

	_, err = svc.GetActiveSubscription(context.Background(), 10, 20)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.Equal(t, 2, repo.getActiveCalls, "撤销后应回源确认订阅已不存在，不能命中旧 L1")
}

type restoreUserSubRepoStub struct {
	userSubRepoNoop

	sub            *UserSubscription
	existsActive   bool
	restoreCalls   int
	restoredStatus string
}

func (r *restoreUserSubRepoStub) GetByIDIncludeDeleted(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *restoreUserSubRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return r.existsActive, nil
}

func (r *restoreUserSubRepoStub) Restore(_ context.Context, id int64, restoredStatus string) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	r.restoreCalls++
	r.restoredStatus = restoredStatus
	cp := *r.sub
	cp.Status = restoredStatus
	cp.DeletedAt = nil
	r.sub = &cp
	return &cp, nil
}

func TestRestoreSubscription_ExpiredActiveRestoresAsExpired(t *testing.T) {
	deletedAt := time.Now().Add(-time.Hour)
	repo := &restoreUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(-time.Minute),
			DeletedAt: &deletedAt,
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	restored, err := svc.RestoreSubscription(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, repo.restoreCalls)
	require.Equal(t, SubscriptionStatusExpired, repo.restoredStatus)
	require.Equal(t, SubscriptionStatusExpired, restored.Status)
	require.Nil(t, restored.DeletedAt)
}

func TestRestoreSubscription_NotRevokedReturnsConflict(t *testing.T) {
	repo := &restoreUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	_, err := svc.RestoreSubscription(context.Background(), 1)
	require.ErrorIs(t, err, ErrSubscriptionNotRevoked)
	require.Zero(t, repo.restoreCalls)
}

func TestRestoreSubscription_LiveSubscriptionConflict(t *testing.T) {
	deletedAt := time.Now().Add(-time.Hour)
	repo := &restoreUserSubRepoStub{
		existsActive: true,
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusExpired,
			ExpiresAt: time.Now().Add(-time.Hour),
			DeletedAt: &deletedAt,
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	_, err := svc.RestoreSubscription(context.Background(), 1)
	require.ErrorIs(t, err, ErrSubscriptionRestoreConflict)
	require.Zero(t, repo.restoreCalls)
}
