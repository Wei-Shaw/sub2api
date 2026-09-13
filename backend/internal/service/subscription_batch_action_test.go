//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type subscriptionBatchRepoStub struct {
	userSubRepoNoop

	subs            map[int64]*UserSubscription
	deleteCalls     []int64
	hardDeleteCalls []int64
}

func newSubscriptionBatchRepoStub(subs ...*UserSubscription) *subscriptionBatchRepoStub {
	repo := &subscriptionBatchRepoStub{subs: make(map[int64]*UserSubscription, len(subs))}
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		cp := *sub
		repo.subs[sub.ID] = &cp
	}
	return repo
}

func (r *subscriptionBatchRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	sub := r.subs[id]
	if sub == nil || sub.DeletedAt != nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (r *subscriptionBatchRepoStub) GetByIDIncludeDeleted(_ context.Context, id int64) (*UserSubscription, error) {
	sub := r.subs[id]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (r *subscriptionBatchRepoStub) Delete(_ context.Context, id int64) error {
	sub := r.subs[id]
	if sub == nil || sub.DeletedAt != nil {
		return ErrSubscriptionNotFound
	}
	now := time.Now()
	sub.DeletedAt = &now
	r.deleteCalls = append(r.deleteCalls, id)
	return nil
}

func (r *subscriptionBatchRepoStub) HardDelete(_ context.Context, id int64) error {
	if r.subs[id] == nil {
		return ErrSubscriptionNotFound
	}
	delete(r.subs, id)
	r.hardDeleteCalls = append(r.hardDeleteCalls, id)
	return nil
}

func TestSubscriptionBatchAction_RevokeReportsMixedResultsAndDeduplicates(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Hour)
	repo := newSubscriptionBatchRepoStub(
		&UserSubscription{ID: 1, UserID: 10, GroupID: 20, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour)},
		&UserSubscription{ID: 2, UserID: 11, GroupID: 20, Status: SubscriptionStatusExpired, ExpiresAt: now.Add(-time.Hour)},
		&UserSubscription{ID: 3, UserID: 12, GroupID: 20, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DeletedAt: &revokedAt},
	)
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	t.Cleanup(svc.Stop)

	result, err := svc.BatchAction(context.Background(), &SubscriptionBatchActionInput{
		SubscriptionIDs: []int64{1, 1, 2, 3, 999},
		Action:          SubscriptionBatchActionRevoke,
	})

	require.NoError(t, err)
	require.Equal(t, 4, result.TotalCount)
	require.Equal(t, 1, result.SucceededCount)
	require.Equal(t, 2, result.SkippedCount)
	require.Equal(t, 1, result.FailedCount)
	require.Equal(t, []int64{1}, repo.deleteCalls)
	require.Equal(t, []SubscriptionBatchActionItem{
		{SubscriptionID: 1, Status: SubscriptionBatchItemSucceeded},
		{SubscriptionID: 2, Status: SubscriptionBatchItemSkipped, Reason: infraerrors.Reason(ErrSubscriptionBatchIneligible), Message: infraerrors.Message(ErrSubscriptionBatchIneligible)},
		{SubscriptionID: 3, Status: SubscriptionBatchItemSkipped, Reason: infraerrors.Reason(ErrSubscriptionBatchIneligible), Message: infraerrors.Message(ErrSubscriptionBatchIneligible)},
		{SubscriptionID: 999, Status: SubscriptionBatchItemFailed, Reason: infraerrors.Reason(ErrSubscriptionNotFound), Message: infraerrors.Message(ErrSubscriptionNotFound)},
	}, result.Items)
}

func TestSubscriptionBatchAction_ValidatesUniqueSelectionLimitAndOptions(t *testing.T) {
	duplicateIDs := make([]int64, MaxSubscriptionBatchSize+50)
	for i := range duplicateIDs {
		duplicateIDs[i] = int64(i%MaxSubscriptionBatchSize + 1)
	}
	ids, err := validateSubscriptionBatchAction(&SubscriptionBatchActionInput{
		SubscriptionIDs: duplicateIDs,
		Action:          SubscriptionBatchActionRevoke,
	})
	require.NoError(t, err)
	require.Len(t, ids, MaxSubscriptionBatchSize)

	tooManyIDs := make([]int64, MaxSubscriptionBatchSize+1)
	for i := range tooManyIDs {
		tooManyIDs[i] = int64(i + 1)
	}
	_, err = validateSubscriptionBatchAction(&SubscriptionBatchActionInput{
		SubscriptionIDs: tooManyIDs,
		Action:          SubscriptionBatchActionRevoke,
	})
	require.ErrorIs(t, err, ErrInvalidSubscriptionBatch)

	_, err = validateSubscriptionBatchAction(&SubscriptionBatchActionInput{
		SubscriptionIDs: []int64{1},
		Action:          SubscriptionBatchActionAdjust,
	})
	require.ErrorIs(t, err, ErrInvalidSubscriptionBatch)

	_, err = validateSubscriptionBatchAction(&SubscriptionBatchActionInput{
		SubscriptionIDs: []int64{1},
		Action:          SubscriptionBatchActionResetQuota,
	})
	require.ErrorIs(t, err, ErrInvalidSubscriptionBatch)
}

func TestPermanentlyDeleteSubscription_RequiresRevokedSubscription(t *testing.T) {
	now := time.Now()
	revokedAt := now.Add(-time.Hour)
	repo := newSubscriptionBatchRepoStub(
		&UserSubscription{ID: 1, UserID: 10, GroupID: 20, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour)},
		&UserSubscription{ID: 2, UserID: 11, GroupID: 20, Status: SubscriptionStatusExpired, ExpiresAt: now.Add(-time.Hour), DeletedAt: &revokedAt},
	)
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	err := svc.PermanentlyDeleteSubscription(context.Background(), 1)
	require.ErrorIs(t, err, ErrSubscriptionNotRevoked)
	require.Empty(t, repo.hardDeleteCalls)

	err = svc.PermanentlyDeleteSubscription(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, []int64{2}, repo.hardDeleteCalls)
	require.NotContains(t, repo.subs, int64(2))
}
