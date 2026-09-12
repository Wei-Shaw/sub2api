package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// SetSubscriptionQuotaRepository installs transactional quota accounting during
// dependency construction. Tests and legacy embeddings may leave it absent.
func (s *SubscriptionService) SetSubscriptionQuotaRepository(repo SubscriptionQuotaRepository) {
	s.quotaRepo = repo
	if s.billingCacheService != nil {
		s.billingCacheService.SetSubscriptionQuotaAdmission(s.AdmitSubscriptionQuota)
	}
}

type subscriptionGroupTxKey struct{}

func subscriptionGroupTxHeld(ctx context.Context) bool {
	held, _ := ctx.Value(subscriptionGroupTxKey{}).(bool)
	return held
}

func (s *SubscriptionService) withKnownSubscriptionGroupTx(ctx context.Context, groupID int64, fn func(context.Context) error) error {
	if s.quotaRepo == nil {
		return s.withSubscriptionUpdateTx(ctx, fn)
	}
	return s.quotaRepo.WithGroupQuotaTx(ctx, groupID, false, func(txCtx context.Context) error {
		return fn(context.WithValue(txCtx, subscriptionGroupTxKey{}, true))
	})
}

func (s *SubscriptionService) withSubscriptionGroupTx(ctx context.Context, id int64, fn func(context.Context) error) error {
	if s.quotaRepo == nil {
		return s.withSubscriptionUpdateTx(ctx, fn)
	}
	// This lookup only discovers the lock key. The subscription is reloaded
	// under the group lock before any decision or mutation.
	sub, err := s.userSubRepo.GetByIDIncludeDeleted(ctx, id)
	if err != nil {
		return err
	}
	return s.withKnownSubscriptionGroupTx(ctx, sub.GroupID, func(txCtx context.Context) error {
		// Ensure uses group -> subscription order and also locks soft-deleted
		// rows needed by Restore. On unmanaged groups it creates no buckets.
		if _, err := s.quotaRepo.EnsureSubscriptionQuotaState(txCtx, id); err != nil {
			return err
		}
		return fn(txCtx)
	})
}

func (s *SubscriptionService) rotateRenewedSubscriptionQuota(ctx context.Context, renewed *UserSubscription, at time.Time) error {
	if s.quotaRepo == nil {
		return nil
	}
	_, err := s.quotaRepo.RotateSubscriptionQuota(ctx, RotateSubscriptionQuotaInput{SubscriptionID: renewed.ID, Dimensions: SubscriptionQuotaDimensions{true, true, true}, Reason: "term_renewal", At: at, DailyWindowStart: renewed.DailyWindowStart, WeeklyWindowStart: renewed.WeeklyWindowStart, MonthlyWindowStart: renewed.MonthlyWindowStart, NewTerm: true, TermStartsAt: renewed.StartsAt})
	return err
}

func applySubscriptionQuotaState(sub *UserSubscription, state *SubscriptionQuotaState) {
	if state == nil {
		return
	}
	sub.DailyUsageUSD, sub.WeeklyUsageUSD, sub.MonthlyUsageUSD = state.DailyUsageUSD, state.WeeklyUsageUSD, state.MonthlyUsageUSD
	sub.DailyWindowStart, sub.WeeklyWindowStart, sub.MonthlyWindowStart = state.DailyWindowStart, state.WeeklyWindowStart, state.MonthlyWindowStart
	sub.Quota = state.Clone()
}

func (s *SubscriptionService) maintainSubscriptionQuota(ctx context.Context, id int64, now time.Time, activate bool) (*UserSubscription, error) {
	var result *UserSubscription
	err := s.withSubscriptionGroupTx(ctx, id, func(txCtx context.Context) error {
		var err error
		result, err = s.maintainSubscriptionQuotaLocked(txCtx, id, now, activate)
		return err
	})
	return result, err
}

func (s *SubscriptionService) maintainSubscriptionQuotaLocked(ctx context.Context, id int64, now time.Time, activate bool) (*UserSubscription, error) {
	sub, err := s.userSubRepo.GetByIDForUpdate(ctx, id)
	if err != nil {
		return nil, err
	}
	if sub.DeletedAt != nil || sub.Status != SubscriptionStatusActive || sub.StartsAt.After(now) || !sub.ExpiresAt.After(now) {
		return sub, nil
	}
	state, err := s.quotaRepo.EnsureSubscriptionQuotaState(ctx, id)
	if err != nil {
		return nil, err
	}
	if state == nil {
		if activate {
			if err := s.checkAndActivateWindowAt(ctx, sub, now); err != nil {
				return nil, err
			}
		}
		if err := s.CheckAndResetWindows(ctx, sub); err != nil {
			return nil, err
		}
		return s.userSubRepo.GetByID(ctx, id)
	}
	applySubscriptionQuotaState(sub, state)
	input := RotateSubscriptionQuotaInput{SubscriptionID: id, Reason: "window_maintenance", At: now, Expected: state.Clone()}
	if activate && !sub.IsWindowActivated() {
		daily := timezone.StartOfDay(now)
		input.Dimensions = SubscriptionQuotaDimensions{true, true, true}
		input.DailyWindowStart, input.WeeklyWindowStart, input.MonthlyWindowStart = &daily, &now, &now
		input.Reason = "first_use"
		input.PreserveUsage = true
	} else {
		if start, ok := sub.automaticDailyWindowStartAt(now); ok {
			input.Dimensions.Daily = true
			input.DailyWindowStart = &start
		}
		if start, ok := sub.automaticWindowStartAt(sub.WeeklyWindowStart, 7*24*time.Hour, now); ok {
			input.Dimensions.Weekly = true
			input.WeeklyWindowStart = &start
		}
		if start, ok := sub.automaticWindowStartAt(sub.MonthlyWindowStart, 30*24*time.Hour, now); ok {
			input.Dimensions.Monthly = true
			input.MonthlyWindowStart = &start
		}
	}
	if input.Dimensions.Any() {
		state, err = s.quotaRepo.RotateSubscriptionQuota(ctx, input)
		if err != nil {
			return nil, err
		}
		applySubscriptionQuotaState(sub, state)
	}
	return CloneSubscriptionForRequest(sub), nil
}

// AdmitSubscriptionQuota is the final authoritative check, after request/WS
// waits. Its group share lock orders the immutable token against publication.
func (s *SubscriptionService) AdmitSubscriptionQuota(ctx context.Context, id int64, group *Group) (*UserSubscription, error) {
	if group == nil || s.quotaRepo == nil {
		return nil, ErrSubscriptionQuotaUnavailable
	}
	var result *UserSubscription
	err := s.withKnownSubscriptionGroupTx(ctx, group.ID, func(txCtx context.Context) error {
		freshGroup, err := s.groupRepo.GetByID(txCtx, group.ID)
		if err != nil {
			return err
		}
		if !freshGroup.IsSubscriptionType() || freshGroup.Status != StatusActive {
			return ErrSubscriptionInvalid
		}
		sub, err := s.userSubRepo.GetByIDForUpdate(txCtx, id)
		if err != nil {
			return err
		}
		if sub.GroupID != group.ID {
			return ErrSubscriptionQuotaInvalid
		}
		now, err := s.quotaRepo.QuotaNow(txCtx)
		if err != nil {
			return err
		}
		if sub.DeletedAt != nil || sub.Status == SubscriptionStatusSuspended {
			return ErrSubscriptionSuspended
		}
		if sub.Status != SubscriptionStatusActive || sub.StartsAt.After(now) || !sub.ExpiresAt.After(now) {
			return ErrSubscriptionExpired
		}
		sub, err = s.maintainSubscriptionQuotaLocked(txCtx, id, now, true)
		if err != nil {
			return err
		}
		if err = checkSubscriptionQuotaAdmissionLimits(sub, freshGroup); err != nil {
			return err
		}
		if sub.Quota == nil {
			sub.Quota = &SubscriptionQuotaSnapshot{SubscriptionID: sub.ID, UserID: sub.UserID, GroupID: sub.GroupID, TermStartsAt: sub.StartsAt, DailyWindowStart: sub.DailyWindowStart, WeeklyWindowStart: sub.WeeklyWindowStart, MonthlyWindowStart: sub.MonthlyWindowStart}
		}
		sub.Quota.AdmittedAt = now
		result = CloneSubscriptionForRequest(sub)
		return nil
	})
	return result, err
}

func checkSubscriptionQuotaAdmissionLimits(sub *UserSubscription, group *Group) error {
	if group.HasDailyLimit() && sub.DailyUsageUSD >= *group.DailyLimitUSD {
		return ErrDailyLimitExceeded
	}
	if group.HasWeeklyLimit() && sub.WeeklyUsageUSD >= *group.WeeklyLimitUSD {
		return ErrWeeklyLimitExceeded
	}
	if group.HasMonthlyLimit() && sub.MonthlyUsageUSD >= *group.MonthlyLimitUSD {
		return ErrMonthlyLimitExceeded
	}
	return nil
}

func (s *SubscriptionService) adminResetSubscriptionQuota(ctx context.Context, id int64, dimensions SubscriptionQuotaDimensions) (*UserSubscription, error) {
	var result *UserSubscription
	err := s.withSubscriptionGroupTx(ctx, id, func(txCtx context.Context) error {
		sub, err := s.userSubRepo.GetByIDForUpdate(txCtx, id)
		if err != nil {
			return err
		}
		now := s.now()
		daily := timezone.StartOfDay(now)
		state, err := s.quotaRepo.EnsureSubscriptionQuotaState(txCtx, id)
		if err != nil {
			return err
		}
		if state == nil {
			err = s.userSubRepo.ResetUsageWindows(txCtx, id, dimensions.Daily, dimensions.Weekly, dimensions.Monthly, daily, now)
		} else {
			_, err = s.quotaRepo.RotateSubscriptionQuota(txCtx, RotateSubscriptionQuotaInput{SubscriptionID: id, Dimensions: dimensions, Reason: "admin_reset", At: now, DailyWindowStart: &daily, WeeklyWindowStart: &now, MonthlyWindowStart: &now})
		}
		if err != nil {
			return err
		}
		result, err = s.userSubRepo.GetByID(txCtx, sub.ID)
		return err
	})
	if err == nil && result != nil {
		err = s.invalidateSubscriptionCaches(result.UserID, result.GroupID)
	}
	return result, err
}
