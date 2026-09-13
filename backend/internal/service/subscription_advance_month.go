package service

import (
	"context"
	"log"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrMonthlyAdvanceUnavailable = infraerrors.BadRequest("MONTHLY_ADVANCE_UNAVAILABLE", "monthly quota cannot be advanced for this subscription")
	ErrMonthlyAdvanceStale       = infraerrors.Conflict("MONTHLY_ADVANCE_STALE", "subscription changed; refresh the preview before confirming")
)

// MonthlyAdvancePreview records every window anchor changed by a monthly
// advance, so confirmation cannot overwrite a concurrent monthly/weekly reset.
type MonthlyAdvancePreview struct {
	SubscriptionID     int64      `json:"subscription_id"`
	MonthlyWindowStart time.Time  `json:"monthly_window_start"`
	WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
	ExpiresAt          time.Time  `json:"expires_at"`
	NewExpiresAt       time.Time  `json:"new_expires_at"`
	DeductSeconds      int64      `json:"deduct_seconds"`
}

func monthlyAdvancePreview(sub *UserSubscription, group *Group, now time.Time) (*MonthlyAdvancePreview, error) {
	if sub.Status != SubscriptionStatusActive || now.Before(sub.StartsAt) || !now.Before(sub.ExpiresAt) || group == nil || !group.IsActive() || !group.IsSubscriptionType() || !group.HasMonthlyLimit() || sub.MonthlyWindowStart == nil || sub.MonthlyUsageUSD <= 0 {
		return nil, ErrMonthlyAdvanceUnavailable
	}
	resetAt := sub.MonthlyResetTime()
	if now.Before(*sub.MonthlyWindowStart) || !now.Before(*resetAt) || !sub.ExpiresAt.After(*resetAt) {
		return nil, ErrMonthlyAdvanceUnavailable
	}
	remaining := resetAt.Sub(now)
	return &MonthlyAdvancePreview{
		SubscriptionID: sub.ID, MonthlyWindowStart: *sub.MonthlyWindowStart,
		WeeklyWindowStart: sub.WeeklyWindowStart, ExpiresAt: sub.ExpiresAt,
		NewExpiresAt:  sub.ExpiresAt.Add(-remaining),
		DeductSeconds: int64((remaining + time.Second - 1) / time.Second),
	}, nil
}

func (s *SubscriptionService) PreviewAdvanceMonth(ctx context.Context, userID, id int64) (*MonthlyAdvancePreview, error) {
	sub, err := s.userSubRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	group, err := s.groupRepo.GetByID(ctx, sub.GroupID)
	if err != nil {
		return nil, err
	}
	return monthlyAdvancePreview(sub, group, s.now())
}

// Spend the same time on the weekly clock as on the subscription/monthly
// clocks. Keep usage unless the advance actually crosses a weekly boundary.
func weeklyWindowAfterMonthlyAdvance(sub *UserSubscription, newExpiry, now time.Time) (*time.Time, float64) {
	if sub.WeeklyWindowStart == nil {
		return nil, sub.WeeklyUsageUSD
	}
	start := sub.windowResetAnchor(*sub.WeeklyWindowStart).Add(newExpiry.Sub(sub.ExpiresAt))
	effective := *sub
	effective.WeeklyWindowStart = &start
	effective.ExpiresAt = newExpiry
	if next, ok := effective.automaticWindowStartAt(&start, 7*24*time.Hour, now); ok {
		return &next, 0
	}
	return &start, sub.WeeklyUsageUSD
}

func (s *SubscriptionService) AdvanceMonth(ctx context.Context, userID, id int64, expectedStart time.Time, expectedWeeklyStart *time.Time, expectedExpiry time.Time) (*MonthlyAdvancePreview, error) {
	var result *MonthlyAdvancePreview
	var groupID int64
	err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		sub, err := s.userSubRepo.GetByIDForUpdate(txCtx, id)
		if err != nil {
			return err
		}
		if sub.UserID != userID {
			return ErrSubscriptionNotFound
		}
		if sub.MonthlyWindowStart == nil || !sub.MonthlyWindowStart.Equal(expectedStart) || !sameMonthlyAdvanceWindow(sub.WeeklyWindowStart, expectedWeeklyStart) || !sub.ExpiresAt.Equal(expectedExpiry) {
			return ErrMonthlyAdvanceStale
		}
		group, err := s.groupRepo.GetByID(txCtx, sub.GroupID)
		if err != nil {
			return err
		}
		now := s.now().Truncate(time.Microsecond)
		result, err = monthlyAdvancePreview(sub, group, now)
		if err != nil {
			return err
		}
		weeklyStart, weeklyUsage := weeklyWindowAfterMonthlyAdvance(sub, result.NewExpiresAt, now)
		if err := s.userSubRepo.ApplyMonthlyAdvance(txCtx, id, now, result.NewExpiresAt, weeklyStart, weeklyUsage); err != nil {
			return err
		}
		groupID = sub.GroupID
		return nil
	})
	if err != nil {
		return nil, err
	}
	invalidate := func() {
		if err := s.invalidateSubscriptionCaches(userID, groupID); err != nil {
			log.Printf("advance monthly quota cache invalidation: %v", err)
		}
	}
	if outerTx := dbent.TxFromContext(ctx); outerTx != nil {
		outerTx.OnCommit(func(next dbent.Committer) dbent.Committer {
			return dbent.CommitFunc(func(commitCtx context.Context, tx *dbent.Tx) error {
				if err := next.Commit(commitCtx, tx); err != nil {
					return err
				}
				invalidate()
				return nil
			})
		})
	} else {
		invalidate()
	}
	return result, nil
}

func sameMonthlyAdvanceWindow(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
