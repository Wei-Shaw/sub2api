package service

import (
	"context"
	"log"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// subscriptionRefundAdjustment records the expiry interval a refund removed
// from one subscription. Rollback and pending-refund settlement replay this
// record instead of deleting the subscription or recomputing whole order days,
// so a renewal bought in between is never lost.
type subscriptionRefundAdjustment struct {
	SubscriptionID int64     `json:"subscriptionID"`
	BeforeExpiry   time.Time `json:"beforeExpiry"`
	AfterExpiry    time.Time `json:"afterExpiry"`
	BeforeStatus   string    `json:"beforeStatus"`
	AfterStatus    string    `json:"afterStatus"`
}

// removed is the length of the interval the refund took away.
func (a *subscriptionRefundAdjustment) removed() time.Duration {
	if a == nil || !a.BeforeExpiry.After(a.AfterExpiry) {
		return 0
	}
	return a.BeforeExpiry.Sub(a.AfterExpiry)
}

// deductRefundSubscriptionDays removes days from a subscription for a refund.
// When fewer days remain, the subscription ends now and is marked expired; the
// row is kept so the deduction stays reversible. A subscription that has
// already ended has nothing left to remove and yields a nil adjustment.
func (s *SubscriptionService) deductRefundSubscriptionDays(ctx context.Context, subscriptionID int64, days int) (*subscriptionRefundAdjustment, error) {
	if days <= 0 {
		return nil, nil
	}
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	return s.shortenSubscriptionForRefund(ctx, subscriptionID, func(expiresAt time.Time) time.Time {
		return expiresAt.AddDate(0, 0, -days)
	})
}

// reapplyRefundSubscription removes again an interval that a pending refund
// gave back, once the gateway confirms the refund.
func (s *SubscriptionService) reapplyRefundSubscription(ctx context.Context, previous *subscriptionRefundAdjustment) (*subscriptionRefundAdjustment, error) {
	removed := previous.removed()
	if removed <= 0 {
		return nil, nil
	}
	return s.shortenSubscriptionForRefund(ctx, previous.SubscriptionID, func(expiresAt time.Time) time.Time {
		return expiresAt.Add(-removed)
	})
}

func (s *SubscriptionService) shortenSubscriptionForRefund(ctx context.Context, subscriptionID int64, target func(time.Time) time.Time) (*subscriptionRefundAdjustment, error) {
	var adjustment *subscriptionRefundAdjustment
	var userID, groupID int64
	err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		sub, err := s.userSubRepo.GetByIDForUpdate(txCtx, subscriptionID)
		if err != nil {
			return err
		}
		now := s.refundAdjustmentNow()
		if !sub.ExpiresAt.After(now) {
			return nil
		}
		expiresAt := target(sub.ExpiresAt)
		if !expiresAt.Before(sub.ExpiresAt) {
			return nil
		}
		status := sub.Status
		if !expiresAt.After(now) {
			expiresAt = now
			status = SubscriptionStatusExpired
		}
		if err := s.userSubRepo.ExtendExpiry(txCtx, sub.ID, expiresAt); err != nil {
			return err
		}
		if status != sub.Status {
			if err := s.userSubRepo.UpdateStatus(txCtx, sub.ID, status); err != nil {
				return err
			}
		}
		adjustment = &subscriptionRefundAdjustment{
			SubscriptionID: sub.ID,
			BeforeExpiry:   sub.ExpiresAt,
			AfterExpiry:    expiresAt,
			BeforeStatus:   sub.Status,
			AfterStatus:    status,
		}
		userID, groupID = sub.UserID, sub.GroupID
		return nil
	})
	if err != nil {
		return nil, err
	}
	if adjustment != nil {
		s.invalidateRefundSubscriptionCaches(ctx, userID, groupID)
	}
	return adjustment, nil
}

// restoreRefundSubscription gives back the interval a refund removed. Only the
// removed length is added to the current expiry, so a renewal bought in the
// meantime is kept. A subscription an administrator revoked in the meantime is
// not brought back: the lookup fails and the caller records the failure.
func (s *SubscriptionService) restoreRefundSubscription(ctx context.Context, adjustment *subscriptionRefundAdjustment) error {
	if adjustment == nil {
		return nil
	}
	var userID, groupID int64
	err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		sub, err := s.userSubRepo.GetByIDForUpdate(txCtx, adjustment.SubscriptionID)
		if err != nil {
			return err
		}
		now := s.refundAdjustmentNow()
		expiresAt := sub.ExpiresAt.Add(adjustment.removed())
		if expiresAt.After(MaxExpiresAt) {
			expiresAt = MaxExpiresAt
		}
		status := sub.Status
		switch {
		case sub.Status == adjustment.AfterStatus && adjustment.AfterStatus != adjustment.BeforeStatus:
			// Only this refund changed the status, so put the original back.
			status = adjustment.BeforeStatus
		case sub.Status == SubscriptionStatusExpired && expiresAt.After(now):
			status = SubscriptionStatusActive
		}
		if status == SubscriptionStatusActive && !expiresAt.After(now) {
			status = SubscriptionStatusExpired
		}
		if !expiresAt.Equal(sub.ExpiresAt) {
			if err := s.userSubRepo.ExtendExpiry(txCtx, sub.ID, expiresAt); err != nil {
				return err
			}
		}
		if status != sub.Status {
			if err := s.userSubRepo.UpdateStatus(txCtx, sub.ID, status); err != nil {
				return err
			}
		}
		userID, groupID = sub.UserID, sub.GroupID
		return nil
	})
	if err != nil {
		return err
	}
	s.invalidateRefundSubscriptionCaches(ctx, userID, groupID)
	return nil
}

// refundAdjustmentNow is truncated to the database timestamp precision so a
// recorded expiry compares equal to the stored one.
func (s *SubscriptionService) refundAdjustmentNow() time.Time {
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	return now.UTC().Truncate(time.Microsecond)
}

// invalidateRefundSubscriptionCaches drops cached subscription state once the
// change is visible: after the surrounding transaction commits, if any.
func (s *SubscriptionService) invalidateRefundSubscriptionCaches(ctx context.Context, userID, groupID int64) {
	invalidate := func() {
		if err := s.invalidateSubscriptionCaches(userID, groupID); err != nil {
			log.Printf("Warning: invalidate refunded subscription cache user=%d group=%d: %v", userID, groupID, err)
		}
	}
	tx := dbent.TxFromContext(ctx)
	if tx == nil {
		invalidate()
		return
	}
	tx.OnCommit(func(next dbent.Committer) dbent.Committer {
		return dbent.CommitFunc(func(ctx context.Context, tx *dbent.Tx) error {
			if err := next.Commit(ctx, tx); err != nil {
				return err
			}
			invalidate()
			return nil
		})
	})
}
