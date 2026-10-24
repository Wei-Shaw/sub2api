package service

import (
	"context"
	"log/slog"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// adjustRefundSubscription pins deductions and rollbacks to the purchased term.
// A replacement purchase may commit while a provider refund is pending; neither
// its final deduction nor a failed refund's rollback may change that new term.
func (s *PaymentService) adjustRefundSubscription(ctx context.Context, p *RefundPlan, days int) (*UserSubscription, error) {
	if p.SubscriptionStartsAt == nil && p.Order != nil && p.Order.CompletedAt != nil {
		// Legacy pending refunds can still be tied to their original purchase
		// using completion time. Recheck the captured start under the lock below.
		sub, err := s.subscriptionSvc.userSubRepo.GetByID(ctx, p.SubscriptionID)
		if err != nil {
			return nil, err
		}
		if p.Order.CompletedAt.Before(sub.StartsAt) {
			if days < 0 {
				p.SubDaysToDeduct = 0
			}
			return sub, nil
		}
		p.SubscriptionStartsAt = &sub.StartsAt
	}
	if p.SubscriptionStartsAt == nil {
		// Old records with neither term metadata nor completion time retain
		// their legacy behavior.
		return s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, days)
	}
	svc := s.subscriptionSvc
	var updated *UserSubscription
	err := svc.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		sub, err := svc.userSubRepo.GetByIDForUpdate(txCtx, p.SubscriptionID)
		if err != nil {
			return err
		}
		updated = sub
		if !sub.StartsAt.Equal(*p.SubscriptionStartsAt) {
			if days < 0 {
				p.SubDaysToDeduct = 0
			}
			return nil
		}
		if days > MaxValidityDays || days < -MaxValidityDays {
			return infraerrors.BadRequest("INVALID_DAYS", "refund adjustment days exceed maximum")
		}
		now := time.Now()
		if svc.now != nil {
			now = svc.now()
		}
		sub.ExpiresAt = sub.ExpiresAt.AddDate(0, 0, days)
		if sub.ExpiresAt.After(MaxExpiresAt) {
			sub.ExpiresAt = MaxExpiresAt
		}
		if !sub.ExpiresAt.After(now) {
			// Keep the row so a failed/pending refund can restore this exact
			// term, without soft-deleting a subscription another order uses.
			sub.Status = SubscriptionStatusExpired
		} else if sub.Status == SubscriptionStatusExpired {
			sub.Status = SubscriptionStatusActive
		}
		return svc.userSubRepo.Update(txCtx, sub)
	})
	if err != nil {
		return nil, err
	}
	// The enclosing failure-finalization transaction invalidates after commit.
	if dbent.TxFromContext(ctx) == nil {
		if err := svc.invalidateSubscriptionCaches(updated.UserID, updated.GroupID); err != nil {
			// The adjustment has committed. A cache error must not make a
			// caller retry the same debit as though the mutation had failed.
			slog.Warn("refund subscription cache invalidation failed", "subscriptionID", updated.ID, "error", err)
		}
	}
	return updated, nil
}
