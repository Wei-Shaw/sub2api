//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

type quotaWindowsRepoStub struct {
	userSubRepoNoop
	sub *UserSubscription
}

func (r *quotaWindowsRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *quotaWindowsRepoStub) SetQuotaWindows(_ context.Context, id int64, daily, weekly, monthly *time.Time) error {
	if r.sub == nil || r.sub.ID != id {
		return ErrSubscriptionNotFound
	}
	if daily != nil {
		r.sub.DailyWindowStart = daily
	}
	if weekly != nil {
		r.sub.WeeklyWindowStart = weekly
	}
	if monthly != nil {
		r.sub.MonthlyWindowStart = monthly
	}
	return nil
}

func TestAdminSetQuotaWindows_KeepsUsage(t *testing.T) {
	weekly := time.Date(2026, 9, 13, 21, 0, 58, 0, time.FixedZone("CST", 8*3600))
	sub := &UserSubscription{
		ID:                 2,
		UserID:             10,
		GroupID:            3,
		ExpiresAt:          time.Date(2026, 10, 13, 15, 56, 19, 0, time.FixedZone("CST", 8*3600)),
		DailyUsageUSD:      39.77,
		WeeklyUsageUSD:     169.40,
		MonthlyUsageUSD:    169.40,
		WeeklyWindowStart:  &weekly,
		MonthlyWindowStart: &weekly,
	}
	stub := &quotaWindowsRepoStub{sub: sub}
	svc := NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)

	aligned := time.Date(2026, 9, 12, 16, 32, 57, 0, time.FixedZone("CST", 8*3600))
	result, err := svc.AdminSetQuotaWindows(context.Background(), 2, &SetQuotaWindowsInput{
		WeeklyWindowStart:  &aligned,
		MonthlyWindowStart: &aligned,
	})
	require.NoError(t, err)
	require.Equal(t, aligned, *result.WeeklyWindowStart)
	require.Equal(t, aligned, *result.MonthlyWindowStart)
	require.Equal(t, 39.77, result.DailyUsageUSD)
	require.Equal(t, 169.40, result.WeeklyUsageUSD)
	require.Equal(t, 169.40, result.MonthlyUsageUSD)
	require.Equal(t, 39.77, stub.sub.DailyUsageUSD)
	require.Equal(t, 169.40, stub.sub.WeeklyUsageUSD)
}

func TestAdminSetQuotaWindows_SnapsDailyToStartOfDay(t *testing.T) {
	sub := &UserSubscription{
		ID:        5,
		UserID:    1,
		GroupID:   3,
		ExpiresAt: time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC),
	}
	stub := &quotaWindowsRepoStub{sub: sub}
	svc := NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
	input := time.Date(2026, 9, 17, 15, 17, 40, 0, time.UTC)

	result, err := svc.AdminSetQuotaWindows(context.Background(), 5, &SetQuotaWindowsInput{DailyWindowStart: &input})
	require.NoError(t, err)
	require.Equal(t, timezone.StartOfDay(input), *result.DailyWindowStart)
}

func TestAdminSetQuotaWindows_RequiresAtLeastOneWindow(t *testing.T) {
	stub := &quotaWindowsRepoStub{sub: &UserSubscription{ID: 1, ExpiresAt: time.Now().Add(24 * time.Hour)}}
	svc := NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
	_, err := svc.AdminSetQuotaWindows(context.Background(), 1, &SetQuotaWindowsInput{})
	require.ErrorIs(t, err, ErrInvalidQuotaWindows)
}

func TestAdminSetQuotaWindows_RejectsStartAfterExpiry(t *testing.T) {
	expires := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	stub := &quotaWindowsRepoStub{sub: &UserSubscription{ID: 1, ExpiresAt: expires}}
	svc := NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
	after := expires.Add(time.Hour)
	_, err := svc.AdminSetQuotaWindows(context.Background(), 1, &SetQuotaWindowsInput{WeeklyWindowStart: &after})
	require.Error(t, err)
}

type groupQuotaSubRepoStub struct {
	userSubRepoNoop
	subs map[int64]*UserSubscription
}

func (r *groupQuotaSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	sub := r.subs[id]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (r *groupQuotaSubRepoStub) List(_ context.Context, _ pagination.PaginationParams, _, groupID *int64, status, _, _, _ string) ([]UserSubscription, *pagination.PaginationResult, error) {
	var out []UserSubscription
	now := time.Now()
	for _, sub := range r.subs {
		if groupID != nil && sub.GroupID != *groupID {
			continue
		}
		if status == SubscriptionStatusActive && (sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now)) {
			continue
		}
		cp := *sub
		out = append(out, cp)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: 1, PageSize: groupSubscriptionScanPageSize, Pages: 1}, nil
}

func (r *groupQuotaSubRepoStub) SetQuotaWindows(_ context.Context, id int64, daily, weekly, monthly *time.Time) error {
	sub := r.subs[id]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	if daily != nil {
		sub.DailyWindowStart = daily
	}
	if weekly != nil {
		sub.WeeklyWindowStart = weekly
	}
	if monthly != nil {
		sub.MonthlyWindowStart = monthly
	}
	return nil
}

func (r *groupQuotaSubRepoStub) ResetUsageWindows(_ context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error {
	sub := r.subs[id]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	if resetDaily {
		sub.DailyUsageUSD = 0
		sub.DailyWindowStart = &dailyStart
	}
	if resetWeekly {
		sub.WeeklyUsageUSD = 0
		sub.WeeklyWindowStart = &periodicStart
	}
	if resetMonthly {
		sub.MonthlyUsageUSD = 0
		sub.MonthlyWindowStart = &periodicStart
	}
	return nil
}

func TestAdminGroupSetQuotaWindows_RejectsStandardGroup(t *testing.T) {
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: 4, SubscriptionType: SubscriptionTypeStandard}}, &groupQuotaSubRepoStub{}, nil, nil, nil)
	start := time.Now().Add(-time.Hour)
	_, err := svc.AdminGroupSetQuotaWindows(context.Background(), 4, &SetQuotaWindowsInput{WeeklyWindowStart: &start})
	require.ErrorIs(t, err, ErrGroupNotSubscriptionType)
}

func TestAdminGroupSetQuotaWindows_SkipsExpired(t *testing.T) {
	now := time.Now()
	activeStart := now.Add(-48 * time.Hour)
	repo := &groupQuotaSubRepoStub{subs: map[int64]*UserSubscription{
		1: {ID: 1, GroupID: 3, Status: SubscriptionStatusActive, ExpiresAt: now.Add(24 * time.Hour), WeeklyUsageUSD: 10, WeeklyWindowStart: &activeStart},
		2: {ID: 2, GroupID: 3, Status: SubscriptionStatusExpired, ExpiresAt: now.Add(-time.Hour), WeeklyUsageUSD: 99, WeeklyWindowStart: &activeStart},
		3: {ID: 3, GroupID: 3, Status: SubscriptionStatusActive, ExpiresAt: now.Add(-time.Minute), WeeklyUsageUSD: 50, WeeklyWindowStart: &activeStart},
	}}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: 3, SubscriptionType: SubscriptionTypeSubscription}}, repo, nil, nil, nil)
	aligned := now.Add(-24 * time.Hour)
	result, err := svc.AdminGroupSetQuotaWindows(context.Background(), 3, &SetQuotaWindowsInput{WeeklyWindowStart: &aligned})
	require.NoError(t, err)
	require.Equal(t, 1, result.Total)
	require.Equal(t, 1, result.Success)
	require.Equal(t, aligned, *repo.subs[1].WeeklyWindowStart)
	require.Equal(t, 10.0, repo.subs[1].WeeklyUsageUSD)
	require.Equal(t, activeStart, *repo.subs[2].WeeklyWindowStart)
	require.Equal(t, 99.0, repo.subs[2].WeeklyUsageUSD)
}

func TestAdminGroupResetQuota_ZerosUsageAndRestartsWindows(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	repo := &groupQuotaSubRepoStub{subs: map[int64]*UserSubscription{
		1: {ID: 1, UserID: 8, GroupID: 3, Status: SubscriptionStatusActive, ExpiresAt: now.Add(24 * time.Hour), DailyUsageUSD: 5, WeeklyUsageUSD: 20, MonthlyUsageUSD: 20, DailyWindowStart: &old, WeeklyWindowStart: &old, MonthlyWindowStart: &old},
		2: {ID: 2, UserID: 9, GroupID: 3, Status: SubscriptionStatusExpired, ExpiresAt: now.Add(-time.Hour), WeeklyUsageUSD: 99, WeeklyWindowStart: &old},
	}}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: 3, SubscriptionType: SubscriptionTypeSubscription}}, repo, nil, nil, nil)
	resetAt := time.Date(2026, 9, 17, 16, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return resetAt }

	result, err := svc.AdminGroupResetQuota(context.Background(), 3, true, true, true)
	require.NoError(t, err)
	require.Equal(t, 1, result.Total)
	require.Equal(t, 1, result.Success)
	require.Equal(t, 0.0, repo.subs[1].DailyUsageUSD)
	require.Equal(t, 0.0, repo.subs[1].WeeklyUsageUSD)
	require.Equal(t, timezone.StartOfDay(resetAt), *repo.subs[1].DailyWindowStart)
	require.Equal(t, resetAt, *repo.subs[1].WeeklyWindowStart)
	require.Equal(t, 99.0, repo.subs[2].WeeklyUsageUSD)
}

func TestAdminGroupResetQuota_RejectsStandardGroup(t *testing.T) {
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: 4, SubscriptionType: SubscriptionTypeStandard}}, &groupQuotaSubRepoStub{}, nil, nil, nil)
	_, err := svc.AdminGroupResetQuota(context.Background(), 4, true, true, true)
	require.ErrorIs(t, err, ErrGroupNotSubscriptionType)
}
