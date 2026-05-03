package service

import (
	"context"
	"fmt"
	"strings"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
)

// GetPublicOrderByResumeToken validates a resume-token and returns the
// associated payment order. The token is the only authentication
// material required for the public resume flow, so the matching checks
// here (user / provider / payment-type) double as authorisation.
//
// When the order is still in PENDING / EXPIRED state we opportunistically
// poll the provider via checkPaid so a long-pending order that just
// completed is reported as PAID without forcing the caller to wait for
// the next webhook delivery.
func (s *PaymentService) GetPublicOrderByResumeToken(ctx context.Context, token string) (*pluginent.PaymentOrder, error) {
	resume := s.Resume()
	if resume == nil {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
	}
	claims, err := resume.ParseToken(ctx, strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	if s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_DB_UNAVAILABLE",
			"payment database is not configured")
	}
	order, err := s.entClient.PaymentOrder.Get(ctx, int(claims.OrderID))
	if err != nil {
		if pluginent.IsNotFound(err) {
			return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
		}
		return nil, fmt.Errorf("get order by resume token: %w", err)
	}
	if claims.UserID > 0 && order.UserID != claims.UserID {
		return nil, invalidResumeTokenMatchError()
	}
	if err := matchOrderAgainstClaims(order, claims); err != nil {
		return nil, err
	}
	// Opportunistic refresh: a slow webhook may not yet have flipped
	// the order to PAID. The current stub returns the order unchanged
	// because checkPaid is not ported yet.
	order = s.maybeReloadAfterPaidCheck(ctx, order)
	return order, nil
}

// matchOrderAgainstClaims rejects the resume request when the claim's
// provider/payment-type fields disagree with the order's current
// recorded values. The snapshot is consulted before the order column so
// a provider rotation does not invalidate a still-valid token.
func matchOrderAgainstClaims(order *pluginent.PaymentOrder, claims *ResumeTokenClaims) error {
	snapshot := psOrderProviderSnapshot(order)
	orderProviderInstanceID := strings.TrimSpace(psStringValue(order.ProviderInstanceID))
	orderProviderKey := strings.TrimSpace(psStringValue(order.ProviderKey))
	if snapshot != nil {
		if snapshot.ProviderInstanceID != "" {
			orderProviderInstanceID = snapshot.ProviderInstanceID
		}
		if snapshot.ProviderKey != "" {
			orderProviderKey = snapshot.ProviderKey
		}
	}
	if claims.ProviderInstanceID != "" && orderProviderInstanceID != claims.ProviderInstanceID {
		return invalidResumeTokenMatchError()
	}
	if claims.ProviderKey != "" && !strings.EqualFold(orderProviderKey, claims.ProviderKey) {
		return invalidResumeTokenMatchError()
	}
	if claims.PaymentType != "" &&
		NormalizeVisibleMethod(order.PaymentType) != NormalizeVisibleMethod(claims.PaymentType) {
		return invalidResumeTokenMatchError()
	}
	return nil
}

// maybeReloadAfterPaidCheck triggers an opportunistic provider check
// for orders still in PENDING / EXPIRED state, reloading the row from
// the database if checkPaid transitioned the order to PAID.
func (s *PaymentService) maybeReloadAfterPaidCheck(ctx context.Context, order *pluginent.PaymentOrder) *pluginent.PaymentOrder {
	if order == nil || s == nil || s.entClient == nil {
		return order
	}
	if order.Status != OrderStatusPending && order.Status != OrderStatusExpired {
		return order
	}
	if s.checkPaid(ctx, order) != checkPaidResultAlreadyPaid {
		return order
	}
	reloaded, err := s.entClient.PaymentOrder.Get(ctx, order.ID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("reload after checkPaid failed", "order_id", order.ID, "error", err)
		}
		return order
	}
	return reloaded
}

func invalidResumeTokenMatchError() error {
	return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token does not match the payment order")
}

// ParseWeChatPaymentResumeToken is a thin pass-through to the embedded
// PaymentResumeService so the in-WeChat handler can validate tokens
// without reaching into the resume service directly.
func (s *PaymentService) ParseWeChatPaymentResumeToken(ctx context.Context, token string) (*WeChatPaymentResumeClaims, error) {
	resume := s.Resume()
	if resume == nil {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
	}
	return resume.ParseWeChatPaymentResumeToken(ctx, strings.TrimSpace(token))
}
