//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func (s *UserSubscriptionRepoSuite) TestApplyMonthlyAdvancePreservesOtherFields() {
	user := s.mustCreateUser("monthly-advance@test.com", service.RoleUser)
	group := s.mustCreateGroup("monthly-advance")
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	oldStart := now.Add(-28 * 24 * time.Hour)
	sub := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetWeeklyWindowStart(now).SetMonthlyWindowStart(oldStart).
			SetDailyWindowStart(now).SetWeeklyUsageUsd(12).SetMonthlyUsageUsd(120).
			SetDailyUsageUsd(4).SetAutoAdvanceWeek(true).SetNotes("keep me")
	})
	week := now.Add(-2 * 24 * time.Hour)
	expiry := now.Add(30 * 24 * time.Hour)
	s.Require().NoError(s.repo.ApplyMonthlyAdvance(s.ctx, sub.ID, now, expiry, &week, 12))
	got, err := s.repo.GetByID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().True(got.MonthlyWindowStart.Equal(now))
	s.Require().True(got.WeeklyWindowStart.Equal(week))
	s.Require().True(got.ExpiresAt.Equal(expiry))
	s.Require().Zero(got.MonthlyUsageUSD)
	s.Require().Equal(float64(12), got.WeeklyUsageUSD)
	s.Require().Equal(float64(4), got.DailyUsageUSD)
	s.Require().True(got.DailyWindowStart.Equal(now))
	s.Require().True(got.AutoAdvanceWeek)
	s.Require().Equal("keep me", got.Notes)
}

func TestMonthlyAdvanceConcurrentConfirmation(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user, err := client.User.Create().SetEmail("month-advance-" + suffix + "@test.com").SetPasswordHash("test").Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().SetName("month-advance-" + suffix).
		SetStatus(service.StatusActive).SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetMonthlyLimitUsd(120).SetWeeklyLimitUsd(30).Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Microsecond)
	month := now.Add(-28 * 24 * time.Hour)
	week := now.Add(-6 * 24 * time.Hour)
	expiry := now.Add(32 * 24 * time.Hour)
	sub, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(group.ID).
		SetStartsAt(month).SetExpiresAt(expiry).SetStatus(service.SubscriptionStatusActive).
		SetMonthlyWindowStart(month).SetWeeklyWindowStart(week).SetDailyWindowStart(now).
		SetMonthlyUsageUsd(120).SetWeeklyUsageUsd(30).SetDailyUsageUsd(4).Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.UserSubscription.DeleteOneID(sub.ID).Exec(ctx)
		_ = client.Group.DeleteOneID(group.ID).Exec(ctx)
		_ = client.User.DeleteOneID(user.ID).Exec(ctx)
	})
	repo := NewUserSubscriptionRepository(client)
	svc := service.NewSubscriptionService(NewGroupRepository(client, integrationDB), repo, nil, client, nil)
	t.Cleanup(svc.Stop)
	preview, err := svc.PreviewAdvanceMonth(ctx, user.ID, sub.ID)
	require.NoError(t, err)
	const attempts = 8
	results := make(chan error, attempts)
	ready := make(chan struct{})
	for i := 0; i < attempts; i++ {
		go func() {
			<-ready
			_, err := svc.AdvanceMonth(ctx, user.ID, sub.ID, preview.MonthlyWindowStart, preview.WeeklyWindowStart, preview.ExpiresAt)
			results <- err
		}()
	}
	close(ready)
	succeeded, stale := 0, 0
	for i := 0; i < attempts; i++ {
		err := <-results
		if err == nil {
			succeeded++
		} else if errors.Is(err, service.ErrMonthlyAdvanceStale) {
			stale++
		} else {
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, attempts-1, stale)
	got, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Zero(t, got.MonthlyUsageUSD)
	require.Zero(t, got.WeeklyUsageUSD)
	require.Equal(t, float64(4), got.DailyUsageUSD)
	require.True(t, got.DailyWindowStart.Equal(now))
	// Advancing both clocks by the same exact duration leaves one full month,
	// regardless of how long the confirmation requests took to reach the lock.
	require.Equal(t, 30*24*time.Hour, got.ExpiresAt.Sub(*got.MonthlyWindowStart))
	require.Equal(t, 24*time.Hour, got.MonthlyWindowStart.Sub(*got.WeeklyWindowStart))
}
