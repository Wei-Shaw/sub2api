package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type subscriptionRestoreRepoStub struct {
	userSubRepoNoop

	nextID   int64
	created  *UserSubscription
	restored *UserSubscription
}

func (r *subscriptionRestoreRepoStub) RestoreSnapshot(_ context.Context, sub *UserSubscription) error {
	if sub == nil {
		return ErrSubscriptionNilInput
	}
	cp := *sub
	r.restored = &cp
	return nil
}

func newSubscriptionRestoreRepoStub() *subscriptionRestoreRepoStub {
	return &subscriptionRestoreRepoStub{nextID: 1}
}

func (r *subscriptionRestoreRepoStub) Create(_ context.Context, sub *UserSubscription) error {
	if sub == nil {
		return ErrSubscriptionNilInput
	}
	cp := *sub
	if cp.ID == 0 {
		cp.ID = r.nextID
		r.nextID++
	}
	sub.ID = cp.ID
	r.created = &cp
	return nil
}

func TestRestoreSubscriptionSnapshotCreatesExactSubscription(t *testing.T) {
	repo := newSubscriptionRestoreRepoStub()
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	startsAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)
	dailyWindow := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	weeklyWindow := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	monthlyWindow := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	assignedBy := int64(99)
	assignedAt := time.Date(2026, 5, 1, 10, 1, 0, 0, time.UTC)

	sub, err := svc.RestoreSubscriptionSnapshot(context.Background(), RestoreSubscriptionSnapshotInput{
		UserID:             31,
		GroupID:            7,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		Status:             SubscriptionStatusActive,
		DailyWindowStart:   &dailyWindow,
		WeeklyWindowStart:  &weeklyWindow,
		MonthlyWindowStart: &monthlyWindow,
		DailyUsageUSD:      1.25,
		WeeklyUsageUSD:     2.5,
		MonthlyUsageUSD:    8.75,
		AssignedBy:         &assignedBy,
		AssignedAt:         assignedAt,
		Notes:              "restore by sales-man after-sales operation op-1",
	})

	require.NoError(t, err)
	require.NotNil(t, sub)
	require.Equal(t, int64(31), sub.UserID)
	require.Equal(t, int64(7), sub.GroupID)
	require.Equal(t, startsAt, sub.StartsAt)
	require.Equal(t, expiresAt, sub.ExpiresAt)
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	require.NotNil(t, sub.DailyWindowStart)
	require.Equal(t, dailyWindow, *sub.DailyWindowStart)
	require.NotNil(t, sub.WeeklyWindowStart)
	require.Equal(t, weeklyWindow, *sub.WeeklyWindowStart)
	require.NotNil(t, sub.MonthlyWindowStart)
	require.Equal(t, monthlyWindow, *sub.MonthlyWindowStart)
	require.Equal(t, 1.25, sub.DailyUsageUSD)
	require.Equal(t, 2.5, sub.WeeklyUsageUSD)
	require.Equal(t, 8.75, sub.MonthlyUsageUSD)
	require.Equal(t, &assignedBy, sub.AssignedBy)
	require.Equal(t, assignedAt, sub.AssignedAt)
	require.Equal(t, "restore by sales-man after-sales operation op-1", sub.Notes)
}

func TestRestoreSubscriptionSnapshotDefaultsStatusAndAssignedAt(t *testing.T) {
	repo := newSubscriptionRestoreRepoStub()
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	startsAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := startsAt.AddDate(0, 0, 30)

	sub, err := svc.RestoreSubscriptionSnapshot(context.Background(), RestoreSubscriptionSnapshotInput{
		UserID:    41,
		GroupID:   9,
		StartsAt:  startsAt,
		ExpiresAt: expiresAt,
	})

	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	require.False(t, sub.AssignedAt.IsZero())
	require.Equal(t, startsAt, sub.StartsAt)
	require.Equal(t, expiresAt, sub.ExpiresAt)
}

func TestRestoreSubscriptionSnapshotRequiresUserAndGroup(t *testing.T) {
	repo := newSubscriptionRestoreRepoStub()
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	_, err := svc.RestoreSubscriptionSnapshot(context.Background(), RestoreSubscriptionSnapshotInput{
		UserID:  0,
		GroupID: 7,
	})

	require.Error(t, err)
	require.Nil(t, repo.created)
}

func TestRestoreSubscriptionSnapshotRestoresSoftDeletedID(t *testing.T) {
	repo := newSubscriptionRestoreRepoStub()
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	startsAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := startsAt.AddDate(0, 0, 30)

	sub, err := svc.RestoreSubscriptionSnapshot(context.Background(), RestoreSubscriptionSnapshotInput{
		SubscriptionID: 99,
		UserID:         41,
		GroupID:        9,
		StartsAt:       startsAt,
		ExpiresAt:      expiresAt,
	})

	require.NoError(t, err)
	require.Equal(t, int64(99), sub.ID)
	require.NotNil(t, repo.restored)
	require.Equal(t, int64(99), repo.restored.ID)
	require.Equal(t, int64(41), repo.restored.UserID)
	require.Equal(t, int64(9), repo.restored.GroupID)
}
