//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

// resetQuotaUserSubRepoStub 支持 GetByID、ResetUsageWindows，
// 其余方法继承 userSubRepoNoop（panic）。
type resetQuotaUserSubRepoStub struct {
	userSubRepoNoop

	sub                *UserSubscription
	subs               map[int64]*UserSubscription
	groupSubscriptions []UserSubscription
	resetErrors        map[int64]error
	resetIDs           []int64

	resetDailyCalled   bool
	resetWeeklyCalled  bool
	resetMonthlyCalled bool
	resetDailyErr      error
	resetWeeklyErr     error
	resetMonthlyErr    error
	dailyStart         time.Time
	periodicStart      time.Time
}

func (r *resetQuotaUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub != nil && r.sub.ID == id {
		cp := *r.sub
		return &cp, nil
	}
	if sub := r.subs[id]; sub != nil {
		cp := *sub
		return &cp, nil
	}
	return nil, ErrSubscriptionNotFound
}

func (r *resetQuotaUserSubRepoStub) ListByGroupID(_ context.Context, groupID int64, params pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	matching := make([]UserSubscription, 0, len(r.groupSubscriptions))
	for i := range r.groupSubscriptions {
		if r.groupSubscriptions[i].GroupID == groupID {
			matching = append(matching, r.groupSubscriptions[i])
		}
	}
	start := params.Offset()
	if start > len(matching) {
		start = len(matching)
	}
	end := start + params.Limit()
	if end > len(matching) {
		end = len(matching)
	}
	pages := 0
	if len(matching) > 0 {
		pages = (len(matching) + params.Limit() - 1) / params.Limit()
	}
	return matching[start:end], &pagination.PaginationResult{
		Total:    int64(len(matching)),
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}, nil
}

func (r *resetQuotaUserSubRepoStub) ResetUsageWindows(_ context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error {
	r.resetDailyCalled = resetDaily
	r.resetWeeklyCalled = resetWeekly
	r.resetMonthlyCalled = resetMonthly
	r.dailyStart = dailyStart
	r.periodicStart = periodicStart
	r.resetIDs = append(r.resetIDs, id)
	if err := r.resetErrors[id]; err != nil {
		return err
	}
	if resetDaily && r.resetDailyErr != nil {
		return r.resetDailyErr
	}
	if resetWeekly && r.resetWeeklyErr != nil {
		return r.resetWeeklyErr
	}
	if resetMonthly && r.resetMonthlyErr != nil {
		return r.resetMonthlyErr
	}
	target := r.sub
	if sub := r.subs[id]; sub != nil {
		target = sub
	}
	if target == nil {
		return nil
	}
	if resetDaily {
		target.DailyUsageUSD = 0
		target.DailyWindowStart = &dailyStart
	}
	if resetWeekly {
		target.WeeklyUsageUSD = 0
		target.WeeklyWindowStart = &periodicStart
	}
	if resetMonthly {
		target.MonthlyUsageUSD = 0
		target.MonthlyWindowStart = &periodicStart
	}
	return nil
}

func (r *resetQuotaUserSubRepoStub) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetDailyCalled = true
	if r.resetDailyErr == nil && r.sub != nil {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &windowStart
	}
	return r.resetDailyErr
}

func (r *resetQuotaUserSubRepoStub) ResetWeeklyUsage(_ context.Context, _ int64, _ *time.Time, _ time.Time) error {
	r.resetWeeklyCalled = true
	return r.resetWeeklyErr
}

func (r *resetQuotaUserSubRepoStub) ResetMonthlyUsage(_ context.Context, _ int64, _ *time.Time, _ time.Time) error {
	r.resetMonthlyCalled = true
	return r.resetMonthlyErr
}

func newResetQuotaSvc(stub *resetQuotaUserSubRepoStub) *SubscriptionService {
	return NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
}

type resetQuotaGroupRepoStub struct {
	groupRepoNoop
	group *Group
	err   error
}

func (r resetQuotaGroupRepoStub) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.group == nil || r.group.ID != id {
		return nil, ErrGroupNotFound
	}
	cp := *r.group
	return &cp, nil
}

func newBulkResetQuotaSvc(groupRepo GroupRepository, stub *resetQuotaUserSubRepoStub) *SubscriptionService {
	return NewSubscriptionService(groupRepo, stub, nil, nil, nil)
}

func TestAdminResetQuota_ResetBoth(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 1, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)
	resetAt := time.Date(2026, 7, 1, 10, 37, 42, 123, time.UTC)
	svc.now = func() time.Time { return resetAt }

	result, err := svc.AdminResetQuota(context.Background(), 1, true, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
	// 手动重置后日窗口锚定当天 0 点（保持 0 点刷新节奏），周窗口锚定重置时刻。
	require.Equal(t, timezone.StartOfDay(resetAt), stub.dailyStart)
	require.Equal(t, resetAt, stub.periodicStart)
	require.Equal(t, timezone.StartOfDay(resetAt), *result.DailyWindowStart)
	require.Equal(t, resetAt, *result.WeeklyWindowStart)
}

func TestAdminResetQuota_ResetDailyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 2, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 2, true, false, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.False(t, stub.resetWeeklyCalled, "不应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_ResetWeeklyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 3, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 3, false, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, stub.resetDailyCalled, "不应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_BothFalseReturnsError(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 7, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 7, false, false, false)

	require.ErrorIs(t, err, ErrInvalidInput)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_SubscriptionNotFound(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: nil}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 999, true, true, true)

	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_ResetDailyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:           &UserSubscription{ID: 4, UserID: 10, GroupID: 20},
		resetDailyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 4, true, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetDailyCalled)
	require.True(t, stub.resetWeeklyCalled, "原子重置应在一次调用中提交所选窗口")
}

func TestAdminResetQuota_ResetWeeklyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:            &UserSubscription{ID: 5, UserID: 10, GroupID: 20},
		resetWeeklyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 5, false, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetWeeklyCalled)
}

func TestAdminResetQuota_ResetMonthlyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 8, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 8, false, false, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, stub.resetDailyCalled, "不应调用 ResetDailyUsage")
	require.False(t, stub.resetWeeklyCalled, "不应调用 ResetWeeklyUsage")
	require.True(t, stub.resetMonthlyCalled, "应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_BeforeStartsAtSameDayPreservesAutomaticBoundary(t *testing.T) {
	startsAt := time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC)
	resetAt := time.Date(2026, 7, 1, 10, 37, 42, 123, time.UTC)
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{
			ID:        10,
			UserID:    10,
			GroupID:   20,
			StartsAt:  startsAt,
			ExpiresAt: startsAt.Add(45 * 24 * time.Hour),
		},
	}
	svc := newResetQuotaSvc(stub)
	svc.now = func() time.Time { return resetAt }

	result, err := svc.AdminResetQuota(context.Background(), 10, false, false, true)

	require.NoError(t, err)
	require.Equal(t, resetAt, *result.MonthlyWindowStart)
	boundary, ok := result.automaticWindowStartAt(result.MonthlyWindowStart, 30*24*time.Hour, resetAt.Add(30*24*time.Hour))
	require.True(t, ok)
	require.Equal(t, resetAt.Add(30*24*time.Hour), boundary)
}

func TestAdminResetQuota_ResetMonthlyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:             &UserSubscription{ID: 9, UserID: 10, GroupID: 20},
		resetMonthlyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 9, false, false, true)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_ReturnsRefreshedSub(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{
			ID:            6,
			UserID:        10,
			GroupID:       20,
			DailyUsageUSD: 99.9,
		},
	}

	svc := newResetQuotaSvc(stub)
	result, err := svc.AdminResetQuota(context.Background(), 6, true, false, false)

	require.NoError(t, err)
	// ResetUsageWindows stub 会将 sub.DailyUsageUSD 归零，
	// 服务应返回第二次 GetByID 的刷新值而非初始的 99.9
	require.Equal(t, float64(0), result.DailyUsageUSD, "返回的订阅应反映已归零的用量")
	require.True(t, stub.resetDailyCalled)
}

func TestAdminResetGroupQuota_ResetsOnlyActiveSubscriptions(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	activeOne := &UserSubscription{ID: 1, UserID: 10, GroupID: 20, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DailyUsageUSD: 3}
	activeTwo := &UserSubscription{ID: 2, UserID: 11, GroupID: 20, Status: SubscriptionStatusActive, ExpiresAt: now.Add(2 * time.Hour), DailyUsageUSD: 4}
	expiredStatus := &UserSubscription{ID: 3, UserID: 12, GroupID: 20, Status: SubscriptionStatusExpired, ExpiresAt: now.Add(time.Hour), DailyUsageUSD: 5}
	expiredTime := &UserSubscription{ID: 4, UserID: 13, GroupID: 20, Status: SubscriptionStatusActive, ExpiresAt: now.Add(-time.Second), DailyUsageUSD: 6}
	stub := &resetQuotaUserSubRepoStub{
		subs: map[int64]*UserSubscription{
			1: activeOne,
			2: activeTwo,
			3: expiredStatus,
			4: expiredTime,
		},
		groupSubscriptions: []UserSubscription{*activeOne, *activeTwo, *expiredStatus, *expiredTime},
	}
	svc := newBulkResetQuotaSvc(resetQuotaGroupRepoStub{
		group: &Group{ID: 20, SubscriptionType: SubscriptionTypeSubscription},
	}, stub)
	svc.now = func() time.Time { return now }

	result, err := svc.AdminResetGroupQuota(context.Background(), 20, true, true, true)

	require.NoError(t, err)
	require.Equal(t, 2, result.TotalCount)
	require.Equal(t, 2, result.SuccessCount)
	require.Zero(t, result.FailedCount)
	require.Equal(t, []int64{1, 2}, stub.resetIDs)
	require.Zero(t, activeOne.DailyUsageUSD)
	require.Zero(t, activeTwo.DailyUsageUSD)
	require.Equal(t, float64(5), expiredStatus.DailyUsageUSD)
	require.Equal(t, float64(6), expiredTime.DailyUsageUSD)
}

func TestAdminResetGroupQuota_ReportsPartialFailures(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	first := &UserSubscription{ID: 1, UserID: 10, GroupID: 20, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour)}
	second := &UserSubscription{ID: 2, UserID: 11, GroupID: 20, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour)}
	stub := &resetQuotaUserSubRepoStub{
		subs:               map[int64]*UserSubscription{1: first, 2: second},
		groupSubscriptions: []UserSubscription{*first, *second},
		resetErrors:        map[int64]error{1: errors.New("reset failed")},
	}
	svc := newBulkResetQuotaSvc(resetQuotaGroupRepoStub{
		group: &Group{ID: 20, SubscriptionType: SubscriptionTypeSubscription},
	}, stub)
	svc.now = func() time.Time { return now }

	result, err := svc.AdminResetGroupQuota(context.Background(), 20, true, true, true)

	require.NoError(t, err)
	require.Equal(t, 2, result.TotalCount)
	require.Equal(t, 1, result.SuccessCount)
	require.Equal(t, 1, result.FailedCount)
	require.Equal(t, []int64{1}, result.FailedSubscriptionIDs)
	require.Len(t, result.Errors, 1)
	require.Equal(t, []int64{1, 2}, stub.resetIDs)
}

func TestAdminResetGroupQuota_RejectsStandardGroup(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{}
	svc := newBulkResetQuotaSvc(resetQuotaGroupRepoStub{
		group: &Group{ID: 20, SubscriptionType: SubscriptionTypeStandard},
	}, stub)

	result, err := svc.AdminResetGroupQuota(context.Background(), 20, true, true, true)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrGroupNotSubscriptionType)
	require.Empty(t, stub.resetIDs)
}
