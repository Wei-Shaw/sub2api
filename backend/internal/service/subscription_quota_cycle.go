package service

import (
	"context"
	"fmt"
	"log"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const groupSubscriptionScanPageSize = 100

// SetQuotaWindowsInput updates quota window starts without changing used amounts.
type SetQuotaWindowsInput struct {
	DailyWindowStart   *time.Time `json:"daily_window_start"`
	WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
	MonthlyWindowStart *time.Time `json:"monthly_window_start"`
}

func (input *SetQuotaWindowsInput) hasAny() bool {
	return input != nil && (input.DailyWindowStart != nil || input.WeeklyWindowStart != nil || input.MonthlyWindowStart != nil)
}

// GroupSubscriptionBatchResult reports a group-scoped subscription mutation that
// may succeed for some rows and fail for others.
type GroupSubscriptionBatchResult struct {
	Total                 int      `json:"total"`
	Success               int      `json:"success"`
	Failed                int      `json:"failed"`
	FailedSubscriptionIDs []int64  `json:"failed_subscription_ids"`
	Errors                []string `json:"errors"`
}

func validatePeriodicWindowStart(start, expiresAt time.Time) error {
	if start.IsZero() {
		return infraerrors.BadRequest("INVALID_QUOTA_WINDOW_START", "quota window start is invalid")
	}
	if !start.Before(expiresAt) {
		return infraerrors.BadRequest("INVALID_QUOTA_WINDOW_START", "quota window start must be before subscription expiration")
	}
	return nil
}

func (s *SubscriptionService) normalizeQuotaWindows(sub *UserSubscription, input *SetQuotaWindowsInput) (daily, weekly, monthly *time.Time, err error) {
	if !input.hasAny() {
		return nil, nil, nil, ErrInvalidQuotaWindows
	}
	if input.DailyWindowStart != nil {
		if input.DailyWindowStart.IsZero() {
			return nil, nil, nil, infraerrors.BadRequest("INVALID_QUOTA_WINDOW_START", "daily_window_start is invalid")
		}
		start := timezone.StartOfDay(*input.DailyWindowStart)
		if !start.Before(sub.ExpiresAt) {
			return nil, nil, nil, infraerrors.BadRequest("INVALID_QUOTA_WINDOW_START", "quota window start must be before subscription expiration")
		}
		daily = &start
	}
	if input.WeeklyWindowStart != nil {
		if err := validatePeriodicWindowStart(*input.WeeklyWindowStart, sub.ExpiresAt); err != nil {
			return nil, nil, nil, err
		}
		start := *input.WeeklyWindowStart
		weekly = &start
	}
	if input.MonthlyWindowStart != nil {
		if err := validatePeriodicWindowStart(*input.MonthlyWindowStart, sub.ExpiresAt); err != nil {
			return nil, nil, nil, err
		}
		start := *input.MonthlyWindowStart
		monthly = &start
	}
	return daily, weekly, monthly, nil
}

func (s *SubscriptionService) invalidateQuotaCaches(ctx context.Context, userID, groupID int64) {
	s.InvalidateSubCacheSync(userID, groupID)
	if s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateSubscription(ctx, userID, groupID)
	}
}

// AdminSetQuotaWindows moves selected quota window starts and leaves used amounts unchanged.
func (s *SubscriptionService) AdminSetQuotaWindows(ctx context.Context, subscriptionID int64, input *SetQuotaWindowsInput) (*UserSubscription, error) {
	if !input.hasAny() {
		return nil, ErrInvalidQuotaWindows
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	daily, weekly, monthly, err := s.normalizeQuotaWindows(sub, input)
	if err != nil {
		return nil, err
	}
	if err := s.userSubRepo.SetQuotaWindows(ctx, sub.ID, daily, weekly, monthly); err != nil {
		return nil, err
	}
	s.invalidateQuotaCaches(ctx, sub.UserID, sub.GroupID)
	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

func (s *SubscriptionService) requireSubscriptionGroup(ctx context.Context, groupID int64) (*Group, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if !group.IsSubscriptionType() {
		return nil, ErrGroupNotSubscriptionType
	}
	return group, nil
}

func (s *SubscriptionService) listActiveGroupSubscriptions(ctx context.Context, groupID int64) ([]UserSubscription, error) {
	var all []UserSubscription
	page := 1
	for {
		subs, pag, err := s.userSubRepo.List(ctx, pagination.PaginationParams{
			Page:      page,
			PageSize:  groupSubscriptionScanPageSize,
			SortBy:    "id",
			SortOrder: pagination.SortOrderAsc,
		}, nil, &groupID, SubscriptionStatusActive, "", "id", pagination.SortOrderAsc)
		if err != nil {
			return nil, err
		}
		all = append(all, subs...)
		if pag == nil || page >= pag.Pages || len(subs) == 0 {
			break
		}
		page++
	}
	return all, nil
}

func newGroupSubscriptionBatchResult() *GroupSubscriptionBatchResult {
	return &GroupSubscriptionBatchResult{
		FailedSubscriptionIDs: []int64{},
		Errors:                []string{},
	}
}

func (result *GroupSubscriptionBatchResult) record(id int64, err error) {
	result.Total++
	if err == nil {
		result.Success++
		return
	}
	result.Failed++
	result.FailedSubscriptionIDs = append(result.FailedSubscriptionIDs, id)
	result.Errors = append(result.Errors, fmt.Sprintf("subscription %d: %s", id, infraerrors.Message(err)))
	if infraerrors.Code(err) >= 500 {
		log.Printf("[GroupSubscriptionBatch] subscription_id=%d error=%s", id, logredact.RedactText(err.Error()))
	}
}

// AdminGroupSetQuotaWindows applies the same window starts to every active subscription in a group.
func (s *SubscriptionService) AdminGroupSetQuotaWindows(ctx context.Context, groupID int64, input *SetQuotaWindowsInput) (*GroupSubscriptionBatchResult, error) {
	if !input.hasAny() {
		return nil, ErrInvalidQuotaWindows
	}
	if _, err := s.requireSubscriptionGroup(ctx, groupID); err != nil {
		return nil, err
	}
	subs, err := s.listActiveGroupSubscriptions(ctx, groupID)
	if err != nil {
		return nil, err
	}
	result := newGroupSubscriptionBatchResult()
	for i := range subs {
		sub := &subs[i]
		if err := ctx.Err(); err != nil {
			result.record(sub.ID, err)
			continue
		}
		_, setErr := s.AdminSetQuotaWindows(ctx, sub.ID, input)
		result.record(sub.ID, setErr)
	}
	return result, nil
}

// AdminGroupResetQuota resets quota the same way as the subscription-page reset:
// zero selected usage and restart those windows (daily at midnight, weekly/monthly at now).
func (s *SubscriptionService) AdminGroupResetQuota(ctx context.Context, groupID int64, resetDaily, resetWeekly, resetMonthly bool) (*GroupSubscriptionBatchResult, error) {
	if !resetDaily && !resetWeekly && !resetMonthly {
		return nil, ErrInvalidInput
	}
	if _, err := s.requireSubscriptionGroup(ctx, groupID); err != nil {
		return nil, err
	}
	subs, err := s.listActiveGroupSubscriptions(ctx, groupID)
	if err != nil {
		return nil, err
	}
	result := newGroupSubscriptionBatchResult()
	for i := range subs {
		sub := &subs[i]
		if err := ctx.Err(); err != nil {
			result.record(sub.ID, err)
			continue
		}
		_, resetErr := s.AdminResetQuota(ctx, sub.ID, resetDaily, resetWeekly, resetMonthly)
		result.record(sub.ID, resetErr)
	}
	return result, nil
}
