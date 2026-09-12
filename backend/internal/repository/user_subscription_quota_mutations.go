package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// All legacy mutators share the same group-first lock used by publication and
// final admission. A SQL NOT EXISTS guard alone would race first enablement.
func (r *userSubscriptionRepository) withQuotaMutation(ctx context.Context, id int64, rejectManaged bool, fn func(context.Context, *service.SubscriptionQuotaState) error) error {
	quota := &subscriptionQuotaRepository{client: r.client}
	groupID, err := quota.subscriptionGroupID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrSubscriptionNotFound
	}
	if err != nil {
		return err
	}
	return quota.WithGroupQuotaTx(ctx, groupID, false, func(txCtx context.Context) error {
		state, err := quota.EnsureSubscriptionQuotaState(txCtx, id)
		if err != nil {
			return err
		}
		if rejectManaged && state != nil {
			return service.ErrSubscriptionQuotaRequired
		}
		return fn(txCtx, state)
	})
}

func (r *userSubscriptionRepository) Create(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}
	quota := &subscriptionQuotaRepository{client: r.client}
	return quota.WithGroupQuotaTx(ctx, sub.GroupID, false, func(txCtx context.Context) error {
		if err := r.create(txCtx, sub); err != nil {
			return err
		}
		_, err := quota.EnsureSubscriptionQuotaState(txCtx, sub.ID)
		return err
	})
}

func (r *userSubscriptionRepository) Update(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}
	return r.withQuotaMutation(ctx, sub.ID, false, func(txCtx context.Context, state *service.SubscriptionQuotaState) error {
		current, err := r.GetByID(txCtx, sub.ID)
		if err != nil {
			return err
		}
		// Moving an existing subscription across groups could bypass the target
		// group's accounting lock. Assignment creates a separate subscription.
		if current.UserID != sub.UserID || current.GroupID != sub.GroupID {
			return service.ErrSubscriptionQuotaInvalid
		}
		if state == nil {
			return r.update(txCtx, sub)
		}
		// Only a preceding explicit bucket rotation may change a managed term.
		// Compatibility counters/anchors always come from the current buckets.
		if state.UserID != sub.UserID || state.GroupID != sub.GroupID || !state.TermStartsAt.Equal(sub.StartsAt) {
			return service.ErrSubscriptionQuotaInvalid
		}
		copy := *sub
		copy.DailyUsageUSD, copy.WeeklyUsageUSD, copy.MonthlyUsageUSD = state.DailyUsageUSD, state.WeeklyUsageUSD, state.MonthlyUsageUSD
		copy.DailyWindowStart, copy.WeeklyWindowStart, copy.MonthlyWindowStart = state.DailyWindowStart, state.WeeklyWindowStart, state.MonthlyWindowStart
		if err := r.update(txCtx, &copy); err != nil {
			return err
		}
		*sub = copy
		return nil
	})
}

func (r *userSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	err := r.withQuotaMutation(ctx, id, false, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error { return r.delete(txCtx, id) })
	if errors.Is(err, service.ErrSubscriptionNotFound) {
		return nil
	}
	return err
}

func (r *userSubscriptionRepository) Restore(ctx context.Context, id int64, status string) (*service.UserSubscription, error) {
	var result *service.UserSubscription
	err := r.withQuotaMutation(ctx, id, false, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		var err error
		result, err = r.restore(txCtx, id, status)
		return err
	})
	return result, err
}

func (r *userSubscriptionRepository) ExtendExpiry(ctx context.Context, id int64, at time.Time) error {
	return r.withQuotaMutation(ctx, id, false, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		return r.extendExpiry(txCtx, id, at)
	})
}
func (r *userSubscriptionRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	return r.withQuotaMutation(ctx, id, false, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		return r.updateStatus(txCtx, id, status)
	})
}
func (r *userSubscriptionRepository) UpdateNotes(ctx context.Context, id int64, notes string) error {
	return r.withQuotaMutation(ctx, id, false, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		return r.updateNotes(txCtx, id, notes)
	})
}
func (r *userSubscriptionRepository) ActivateWindows(ctx context.Context, id int64, daily, periodic time.Time) error {
	return r.withQuotaMutation(ctx, id, true, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		return r.activateWindows(txCtx, id, daily, periodic)
	})
}
func (r *userSubscriptionRepository) ResetUsageWindows(ctx context.Context, id int64, daily, weekly, monthly bool, dailyAt, periodicAt time.Time) error {
	return r.withQuotaMutation(ctx, id, true, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		return r.resetUsageWindows(txCtx, id, daily, weekly, monthly, dailyAt, periodicAt)
	})
}
func (r *userSubscriptionRepository) ResetDailyUsage(ctx context.Context, id int64, expected *time.Time, at time.Time) error {
	return r.withQuotaMutation(ctx, id, true, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		return r.resetDailyUsage(txCtx, id, expected, at)
	})
}
func (r *userSubscriptionRepository) ResetWeeklyUsage(ctx context.Context, id int64, expected *time.Time, at time.Time) error {
	return r.withQuotaMutation(ctx, id, true, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		return r.resetWeeklyUsage(txCtx, id, expected, at)
	})
}
func (r *userSubscriptionRepository) ResetMonthlyUsage(ctx context.Context, id int64, expected *time.Time, at time.Time) error {
	return r.withQuotaMutation(ctx, id, true, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		return r.resetMonthlyUsage(txCtx, id, expected, at)
	})
}
func (r *userSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, cost float64) error {
	return r.withQuotaMutation(ctx, id, true, func(txCtx context.Context, _ *service.SubscriptionQuotaState) error {
		return r.incrementUsage(txCtx, id, cost)
	})
}

func (r *userSubscriptionRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	quota := &subscriptionQuotaRepository{client: r.client}
	rows, err := quota.executor(ctx).QueryContext(ctx, `SELECT DISTINCT group_id FROM user_subscriptions WHERE status='active' AND deleted_at IS NULL AND expires_at<=$1 ORDER BY group_id`, time.Now())
	if err != nil {
		return 0, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return 0, err
	}
	var count int64
	for _, id := range ids {
		err = quota.WithGroupQuotaTx(ctx, id, false, func(txCtx context.Context) error {
			n, err := clientFromContext(txCtx, r.client).UserSubscription.Update().Where(usersubscription.GroupIDEQ(id), usersubscription.StatusEQ(service.SubscriptionStatusActive), usersubscription.ExpiresAtLTE(time.Now())).SetStatus(service.SubscriptionStatusExpired).Save(txCtx)
			count += int64(n)
			return err
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}
