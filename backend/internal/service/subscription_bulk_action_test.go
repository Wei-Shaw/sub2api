package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type bulkActionSubscriptionRepo struct {
	UserSubscriptionRepository
	subscriptions map[int64]*UserSubscription
	mutations     []int64
	afterMutation func()
}

func (r *bulkActionSubscriptionRepo) GetByIDIncludeDeleted(_ context.Context, id int64) (*UserSubscription, error) {
	sub := r.subscriptions[id]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	copy := *sub
	return &copy, nil
}

func (r *bulkActionSubscriptionRepo) GetByID(ctx context.Context, id int64) (*UserSubscription, error) {
	sub, err := r.GetByIDIncludeDeleted(ctx, id)
	if err != nil {
		return nil, err
	}
	if sub.DeletedAt != nil {
		return nil, ErrSubscriptionNotFound
	}
	return sub, nil
}

func (r *bulkActionSubscriptionRepo) GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error) {
	return r.GetByID(ctx, id)
}

func (r *bulkActionSubscriptionRepo) mutated(id int64) {
	r.mutations = append(r.mutations, id)
	if r.afterMutation != nil {
		r.afterMutation()
	}
}

func (r *bulkActionSubscriptionRepo) ExtendExpiry(_ context.Context, id int64, expiresAt time.Time) error {
	r.subscriptions[id].ExpiresAt = expiresAt
	r.mutated(id)
	return nil
}

func (r *bulkActionSubscriptionRepo) ResetUsageWindows(_ context.Context, id int64, daily, weekly, monthly bool, dailyStart, periodicStart time.Time) error {
	sub := r.subscriptions[id]
	if daily {
		sub.DailyUsageUSD, sub.DailyWindowStart = 0, &dailyStart
	}
	if weekly {
		sub.WeeklyUsageUSD, sub.WeeklyWindowStart = 0, &periodicStart
	}
	if monthly {
		sub.MonthlyUsageUSD, sub.MonthlyWindowStart = 0, &periodicStart
	}
	r.mutated(id)
	return nil
}

func (r *bulkActionSubscriptionRepo) Delete(_ context.Context, id int64) error {
	now := time.Now()
	r.subscriptions[id].DeletedAt = &now
	r.mutated(id)
	return nil
}

func (r *bulkActionSubscriptionRepo) ExistsActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (bool, error) {
	for _, sub := range r.subscriptions {
		if sub.UserID == userID && sub.GroupID == groupID && sub.DeletedAt == nil {
			return true, nil
		}
	}
	return false, nil
}

func (r *bulkActionSubscriptionRepo) Restore(_ context.Context, id int64, status string) (*UserSubscription, error) {
	sub := r.subscriptions[id]
	sub.DeletedAt, sub.Status = nil, status
	r.mutated(id)
	copy := *sub
	return &copy, nil
}

func TestBulkSubscriptionAction_PartialSuccessAndDeduplication(t *testing.T) {
	for _, action := range []string{"extend", "reset_quota", "revoke", "restore"} {
		t.Run(action, func(t *testing.T) {
			expiresAt := time.Now().AddDate(0, 0, 30)
			repo := &bulkActionSubscriptionRepo{subscriptions: map[int64]*UserSubscription{}}
			for _, id := range []int64{1, 2} {
				sub := &UserSubscription{ID: id, UserID: id, GroupID: 10, Status: SubscriptionStatusActive, ExpiresAt: expiresAt, DailyUsageUSD: 2, WeeklyUsageUSD: 5, MonthlyUsageUSD: 8}
				if action == "restore" {
					deletedAt := time.Now().Add(-time.Hour)
					sub.DeletedAt = &deletedAt
				}
				repo.subscriptions[id] = sub
			}
			svc := NewSubscriptionService(nil, repo, nil, nil, nil)
			t.Cleanup(svc.Stop)

			result, err := svc.BulkSubscriptionAction(context.Background(), &BulkSubscriptionActionInput{
				SubscriptionIDs: []int64{2, 404, 1, 2, 404}, Action: action, Days: 7, Daily: true, Weekly: true,
			})
			require.NoError(t, err)
			require.Equal(t, 2, result.SuccessCount)
			require.Equal(t, 1, result.FailedCount)
			require.Equal(t, []BulkSubscriptionActionItemResult{
				{SubscriptionID: 2, Success: true},
				{SubscriptionID: 404, Error: "subscription not found"},
				{SubscriptionID: 1, Success: true},
			}, result.Results)
			require.Equal(t, []int64{2, 1}, repo.mutations)
			for _, sub := range repo.subscriptions {
				switch action {
				case "extend":
					require.Equal(t, expiresAt.AddDate(0, 0, 7), sub.ExpiresAt)
				case "reset_quota":
					require.Zero(t, sub.DailyUsageUSD)
					require.Zero(t, sub.WeeklyUsageUSD)
					require.Equal(t, float64(8), sub.MonthlyUsageUSD)
				case "revoke":
					require.NotNil(t, sub.DeletedAt)
				case "restore":
					require.Nil(t, sub.DeletedAt)
					require.Equal(t, SubscriptionStatusActive, sub.Status)
				}
			}
		})
	}
}

func TestBulkSubscriptionAction_ValidatesBeforeAnyRepositoryAccess(t *testing.T) {
	tooMany := make([]int64, MaxBulkSubscriptionActions+1)
	for i := range tooMany {
		tooMany[i] = 1
	}
	for name, input := range map[string]*BulkSubscriptionActionInput{
		"nil":              nil,
		"empty IDs":        {Action: "revoke"},
		"too many IDs":     {SubscriptionIDs: tooMany, Action: "revoke"},
		"invalid later ID": {SubscriptionIDs: []int64{1, 0}, Action: "revoke"},
		"negative ID":      {SubscriptionIDs: []int64{1, -1}, Action: "revoke"},
		"unknown action":   {SubscriptionIDs: []int64{1}, Action: "delete"},
		"missing action":   {SubscriptionIDs: []int64{1}},
		"zero adjustment":  {SubscriptionIDs: []int64{1}, Action: "extend"},
		"large adjustment": {SubscriptionIDs: []int64{1}, Action: "extend", Days: MaxValidityDays + 1},
		"small adjustment": {SubscriptionIDs: []int64{1}, Action: "extend", Days: -MaxValidityDays - 1},
		"no reset windows": {SubscriptionIDs: []int64{1}, Action: "reset_quota"},
	} {
		t.Run(name, func(t *testing.T) {
			// A nil repository would panic if validation allowed any execution.
			svc := &SubscriptionService{}
			result, err := svc.BulkSubscriptionAction(context.Background(), input)
			require.Error(t, err)
			require.Equal(t, 400, infraerrors.Code(err))
			require.Nil(t, result)
		})
	}
	for _, days := range []int{-MaxValidityDays, -1, 1, MaxValidityDays} {
		input := BulkSubscriptionActionInput{SubscriptionIDs: []int64{1}, Action: "extend", Days: days}
		require.NoError(t, input.Validate())
	}
}

func TestBulkSubscriptionAction_CancellationPreservesCompletedResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &bulkActionSubscriptionRepo{
		subscriptions: map[int64]*UserSubscription{1: {ID: 1, UserID: 1, GroupID: 10}},
		afterMutation: cancel,
	}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)
	result, err := svc.BulkSubscriptionAction(ctx, &BulkSubscriptionActionInput{SubscriptionIDs: []int64{1, 2, 3}, Action: "revoke"})
	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessCount)
	require.Equal(t, 2, result.FailedCount)
	require.Equal(t, []int64{1}, repo.mutations)
	require.True(t, result.Results[0].Success)
	for _, item := range result.Results[1:] {
		require.False(t, item.Success)
		require.Equal(t, context.Canceled.Error(), item.Error)
	}
}

type failingBulkActionSubscriptionRepo struct {
	*bulkActionSubscriptionRepo
	err error
}

func (r failingBulkActionSubscriptionRepo) Delete(context.Context, int64) error {
	return r.err
}

func TestBulkSubscriptionAction_DoesNotExposeInternalErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "internal failure", err: errors.New("postgres: internal connection details"), want: "internal error"},
		{name: "wrapped cancellation", err: fmt.Errorf("postgres: internal connection details: %w", context.Canceled), want: context.Canceled.Error()},
		{name: "wrapped deadline", err: fmt.Errorf("postgres: internal connection details: %w", context.DeadlineExceeded), want: context.DeadlineExceeded.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := failingBulkActionSubscriptionRepo{
				bulkActionSubscriptionRepo: &bulkActionSubscriptionRepo{subscriptions: map[int64]*UserSubscription{1: {ID: 1}}},
				err:                        tc.err,
			}
			svc := NewSubscriptionService(nil, repo, nil, nil, nil)
			t.Cleanup(svc.Stop)
			result, err := svc.BulkSubscriptionAction(context.Background(), &BulkSubscriptionActionInput{SubscriptionIDs: []int64{1}, Action: "revoke"})
			require.NoError(t, err)
			require.Equal(t, 1, result.FailedCount)
			require.Equal(t, tc.want, result.Results[0].Error)
		})
	}
}
