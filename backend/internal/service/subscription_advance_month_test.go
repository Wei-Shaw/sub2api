//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/dgraph-io/ristretto"
	"github.com/stretchr/testify/require"
)

func monthlyAdvanceFixture() (*UserSubscription, *Group, time.Time) {
	start := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	week := start.Add(28 * 24 * time.Hour)
	limit := 120.0
	return &UserSubscription{
		ID: 10, UserID: 20, GroupID: 30, Status: SubscriptionStatusActive,
		StartsAt: start, ExpiresAt: start.Add(60 * 24 * time.Hour),
		MonthlyWindowStart: &start, WeeklyWindowStart: &week, DailyWindowStart: &week,
		MonthlyUsageUSD: 120, WeeklyUsageUSD: 12, DailyUsageUSD: 4, AutoAdvanceWeek: true,
	}, &Group{ID: 30, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, MonthlyLimitUSD: &limit}, week
}

func TestMonthlyAdvanceCalculation(t *testing.T) {
	sub, group, now := monthlyAdvanceFixture()
	preview, err := monthlyAdvancePreview(sub, group, now)
	require.NoError(t, err)
	require.Equal(t, int64(2*24*3600), preview.DeductSeconds)
	require.Equal(t, sub.ExpiresAt.Add(-2*24*time.Hour), preview.NewExpiresAt)
	preview, err = monthlyAdvancePreview(sub, group, now.Add(90*time.Minute+100*time.Millisecond))
	require.NoError(t, err)
	require.Equal(t, int64(2*24*3600-90*60), preview.DeductSeconds)
	require.Equal(t, sub.ExpiresAt.Add(-2*24*time.Hour+90*time.Minute+100*time.Millisecond), preview.NewExpiresAt)
	// Legacy midnight windows use the same corrected reset time as the UI.
	legacy := startOfDay(sub.StartsAt)
	sub.MonthlyWindowStart = &legacy
	preview, err = monthlyAdvancePreview(sub, group, now)
	require.NoError(t, err)
	require.Equal(t, int64(2*24*3600), preview.DeductSeconds)
}

func TestMonthlyAdvanceEligibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*UserSubscription, *Group, time.Time)
	}{
		{"unused", func(s *UserSubscription, _ *Group, _ time.Time) { s.MonthlyUsageUSD = 0 }},
		{"inactive", func(s *UserSubscription, _ *Group, _ time.Time) { s.Status = "suspended" }},
		{"not started", func(s *UserSubscription, _ *Group, n time.Time) { s.StartsAt = n.Add(time.Hour) }},
		{"expired", func(s *UserSubscription, _ *Group, n time.Time) { s.ExpiresAt = n }},
		{"missing monthly anchor", func(s *UserSubscription, _ *Group, _ time.Time) { s.MonthlyWindowStart = nil }},
		{"future monthly anchor", func(s *UserSubscription, _ *Group, n time.Time) { v := n.Add(time.Hour); s.MonthlyWindowStart = &v }},
		{"already due", func(s *UserSubscription, _ *Group, n time.Time) {
			v := n.Add(-30 * 24 * time.Hour)
			s.MonthlyWindowStart = &v
			s.StartsAt = v
		}},
		{"final month", func(s *UserSubscription, _ *Group, _ time.Time) { s.ExpiresAt = *s.MonthlyResetTime() }},
		{"partial final month", func(s *UserSubscription, _ *Group, n time.Time) { s.ExpiresAt = n.Add(time.Hour) }},
		{"disabled group", func(_ *UserSubscription, g *Group, _ time.Time) { g.Status = "disabled" }},
		{"balance group", func(_ *UserSubscription, g *Group, _ time.Time) { g.SubscriptionType = "standard" }},
		{"no limit", func(_ *UserSubscription, g *Group, _ time.Time) { g.MonthlyLimitUSD = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub, group, now := monthlyAdvanceFixture()
			tc.mutate(sub, group, now)
			_, err := monthlyAdvancePreview(sub, group, now)
			require.ErrorIs(t, err, ErrMonthlyAdvanceUnavailable)
		})
	}
	sub, _, now := monthlyAdvanceFixture()
	_, err := monthlyAdvancePreview(sub, nil, now)
	require.ErrorIs(t, err, ErrMonthlyAdvanceUnavailable)
}

func TestMonthlyAdvanceWeeklyClock(t *testing.T) {
	for _, tc := range []struct {
		name            string
		weeklyAge       time.Duration
		deduct          time.Duration
		wantAge         time.Duration
		wantWeeklyUsage float64
	}{
		{"same week keeps usage", 0, 2 * 24 * time.Hour, 2 * 24 * time.Hour, 12},
		{"cross week resets usage", 6 * 24 * time.Hour, 2 * 24 * time.Hour, 24 * time.Hour, 0},
		{"exact boundary resets usage", 5 * 24 * time.Hour, 2 * 24 * time.Hour, 0, 0},
		{"multiple crossed weeks", 6 * 24 * time.Hour, 20 * 24 * time.Hour, 5 * 24 * time.Hour, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub, _, now := monthlyAdvanceFixture()
			week := now.Add(-tc.weeklyAge)
			sub.WeeklyWindowStart = &week
			start, usage := weeklyWindowAfterMonthlyAdvance(sub, sub.ExpiresAt.Add(-tc.deduct), now)
			require.Equal(t, now.Add(-tc.wantAge), *start)
			require.Equal(t, tc.wantWeeklyUsage, usage)
		})
	}
	sub, _, now := monthlyAdvanceFixture()
	sub.WeeklyWindowStart = nil
	start, usage := weeklyWindowAfterMonthlyAdvance(sub, sub.ExpiresAt.Add(-48*time.Hour), now)
	require.Nil(t, start)
	require.Equal(t, sub.WeeklyUsageUSD, usage)
}

type monthlyAdvanceRepo struct {
	UserSubscriptionRepository
	sub        *UserSubscription
	applyErr   error
	applyCalls int
}

func (r *monthlyAdvanceRepo) GetByID(context.Context, int64) (*UserSubscription, error) {
	cp := *r.sub
	return &cp, nil
}

func (r *monthlyAdvanceRepo) GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error) {
	return r.GetByID(ctx, id)
}

func (r *monthlyAdvanceRepo) ApplyMonthlyAdvance(_ context.Context, _ int64, start, expiry time.Time, weeklyStart *time.Time, weeklyUsage float64) error {
	r.applyCalls++
	if r.applyErr != nil {
		return r.applyErr
	}
	r.sub.MonthlyWindowStart = &start
	r.sub.MonthlyUsageUSD = 0
	r.sub.ExpiresAt = expiry
	r.sub.WeeklyWindowStart = weeklyStart
	r.sub.WeeklyUsageUSD = weeklyUsage
	return nil
}

type monthlyAdvanceGroupRepo struct {
	GroupRepository
	group *Group
}

func (r *monthlyAdvanceGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	return r.group, nil
}

func newMonthlyAdvanceTestService(sub *UserSubscription, group *Group, now time.Time) (*SubscriptionService, *monthlyAdvanceRepo) {
	repo := &monthlyAdvanceRepo{sub: sub}
	return &SubscriptionService{userSubRepo: repo, groupRepo: &monthlyAdvanceGroupRepo{group: group}, now: func() time.Time { return now }}, repo
}

func TestMonthlyAdvanceOwnershipAndReplay(t *testing.T) {
	sub, group, now := monthlyAdvanceFixture()
	svc, repo := newMonthlyAdvanceTestService(sub, group, now)
	month, week, expiry, daily := *sub.MonthlyWindowStart, sub.WeeklyWindowStart, sub.ExpiresAt, *sub.DailyWindowStart
	_, err := svc.PreviewAdvanceMonth(context.Background(), 999, sub.ID)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	_, err = svc.AdvanceMonth(context.Background(), 999, sub.ID, month, week, expiry)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.Zero(t, repo.applyCalls)
	preview, err := svc.AdvanceMonth(context.Background(), sub.UserID, sub.ID, month, week, expiry)
	require.NoError(t, err)
	require.Equal(t, expiry.Add(-48*time.Hour), preview.NewExpiresAt)
	require.Equal(t, preview.NewExpiresAt, sub.ExpiresAt)
	require.Equal(t, now, *sub.MonthlyWindowStart)
	require.Zero(t, sub.MonthlyUsageUSD)
	require.Equal(t, now.Add(-48*time.Hour), *sub.WeeklyWindowStart)
	require.Equal(t, float64(12), sub.WeeklyUsageUSD)
	require.Equal(t, float64(4), sub.DailyUsageUSD)
	require.Equal(t, daily, *sub.DailyWindowStart)
	require.True(t, sub.AutoAdvanceWeek)
	_, err = svc.AdvanceMonth(context.Background(), sub.UserID, sub.ID, month, week, expiry)
	require.ErrorIs(t, err, ErrMonthlyAdvanceStale)
	require.Equal(t, 1, repo.applyCalls)
	// After new usage, another monthly reset cannot consume the final month.
	sub.MonthlyUsageUSD = 1
	_, err = svc.PreviewAdvanceMonth(context.Background(), sub.UserID, sub.ID)
	require.ErrorIs(t, err, ErrMonthlyAdvanceUnavailable)
}

func TestMonthlyAdvanceRejectsChangedConfirmation(t *testing.T) {
	for _, field := range []string{"month", "week", "week to nil", "nil to week", "expiry"} {
		t.Run(field, func(t *testing.T) {
			sub, group, now := monthlyAdvanceFixture()
			if field == "nil to week" {
				sub.WeeklyWindowStart = nil
			}
			svc, repo := newMonthlyAdvanceTestService(sub, group, now)
			preview, err := svc.PreviewAdvanceMonth(context.Background(), sub.UserID, sub.ID)
			require.NoError(t, err)
			changed := now.Add(time.Hour)
			switch field {
			case "month":
				sub.MonthlyWindowStart = &changed
			case "week", "nil to week":
				sub.WeeklyWindowStart = &changed
			case "week to nil":
				sub.WeeklyWindowStart = nil
			case "expiry":
				sub.ExpiresAt = sub.ExpiresAt.Add(time.Hour)
			}
			_, err = svc.AdvanceMonth(context.Background(), sub.UserID, sub.ID, preview.MonthlyWindowStart, preview.WeeklyWindowStart, preview.ExpiresAt)
			require.ErrorIs(t, err, ErrMonthlyAdvanceStale)
			require.Zero(t, repo.applyCalls)
		})
	}
}

func TestMonthlyAdvanceCacheInvalidationAndPersistenceFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			sub, group, now := monthlyAdvanceFixture()
			svc, repo := newMonthlyAdvanceTestService(sub, group, now)
			cache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 1000, MaxCost: 100, BufferItems: 64})
			require.NoError(t, err)
			t.Cleanup(cache.Close)
			svc.subCacheL1 = cache
			key := subCacheKey(sub.UserID, sub.GroupID)
			require.True(t, cache.Set(key, "stale", 1))
			cache.Wait()
			if fail {
				repo.applyErr = errors.New("write failed")
			}
			_, err = svc.AdvanceMonth(context.Background(), sub.UserID, sub.ID, *sub.MonthlyWindowStart, sub.WeeklyWindowStart, sub.ExpiresAt)
			cache.Wait()
			_, cached := cache.Get(key)
			if fail {
				require.ErrorIs(t, err, repo.applyErr)
				require.True(t, cached)
				require.Equal(t, float64(120), sub.MonthlyUsageUSD)
			} else {
				require.NoError(t, err)
				require.False(t, cached)
			}
		})
	}
}

func TestMonthlyAdvanceCacheWaitsForOuterCommit(t *testing.T) {
	for _, commit := range []bool{true, false} {
		t.Run(map[bool]string{true: "commit", false: "rollback"}[commit], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			mock.ExpectBegin()
			tx, err := client.Tx(context.Background())
			require.NoError(t, err)
			sub, group, now := monthlyAdvanceFixture()
			svc, _ := newMonthlyAdvanceTestService(sub, group, now)
			cache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 1000, MaxCost: 100, BufferItems: 64})
			require.NoError(t, err)
			t.Cleanup(cache.Close)
			svc.subCacheL1 = cache
			key := subCacheKey(sub.UserID, sub.GroupID)
			require.True(t, cache.Set(key, "before commit", 1))
			cache.Wait()
			_, err = svc.AdvanceMonth(dbent.NewTxContext(context.Background(), tx), sub.UserID, sub.ID, *sub.MonthlyWindowStart, sub.WeeklyWindowStart, sub.ExpiresAt)
			require.NoError(t, err)
			_, cached := cache.Get(key)
			require.True(t, cached, "changes are not committed yet")
			if commit {
				mock.ExpectCommit()
				require.NoError(t, tx.Commit())
			} else {
				mock.ExpectRollback()
				require.NoError(t, tx.Rollback())
			}
			cache.Wait()
			_, cached = cache.Get(key)
			require.Equal(t, !commit, cached)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
