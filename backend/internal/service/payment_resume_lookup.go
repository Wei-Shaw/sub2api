package service

import (
	"context"
	"crypto/hmac"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GetPublicOrderByResumeToken resolves an order from a signed resume token and
// reconciles it against the provider.
//
// paymentReference is the provider payment id a hosted checkout appended to its
// return URL, empty when the redirect carried none. The resume token proves the
// caller holds this checkout session; the reference itself proves nothing and is
// verified upstream before it is believed.
func (s *PaymentService) GetPublicOrderByResumeToken(ctx context.Context, token string, paymentReference string) (*dbent.PaymentOrder, error) {
	claims, err := s.paymentResume().ParseToken(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}

	order, err := s.entClient.PaymentOrder.Get(ctx, claims.OrderID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
		}
		return nil, fmt.Errorf("get order by resume token: %w", err)
	}
	if claims.UserID > 0 && order.UserID != claims.UserID {
		return nil, invalidResumeTokenMatchError()
	}
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
	if len(claims.BindHash) > 0 {
		// Packed token: the three identity claims were folded into one digest.
		expected := paymentResumeBindHash(orderProviderKey, orderProviderInstanceID, order.PaymentType)
		if !hmac.Equal(claims.BindHash, expected) {
			return nil, invalidResumeTokenMatchError()
		}
	} else {
		if claims.ProviderInstanceID != "" && orderProviderInstanceID != claims.ProviderInstanceID {
			return nil, invalidResumeTokenMatchError()
		}
		if claims.ProviderKey != "" && !strings.EqualFold(orderProviderKey, claims.ProviderKey) {
			return nil, invalidResumeTokenMatchError()
		}
		if claims.PaymentType != "" && !strings.EqualFold(strings.TrimSpace(order.PaymentType), strings.TrimSpace(claims.PaymentType)) {
			return nil, invalidResumeTokenMatchError()
		}
	}
	if order.Status == OrderStatusPending || order.Status == OrderStatusExpired {
		// Before asking whether the order was paid, take the payment id off the
		// redirect: without it the question is asked against an invoice id the
		// pollable endpoint does not accept.
		s.adoptPaymentReference(ctx, order, paymentReference)
		result := s.reconcilePaid(ctx, order)
		if result == checkPaidResultAlreadyPaid {
			order, err = s.entClient.PaymentOrder.Get(ctx, order.ID)
			if err != nil {
				return nil, fmt.Errorf("reload order by resume token: %w", err)
			}
		}
	}

	return order, nil
}

func invalidResumeTokenMatchError() error {
	return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token does not match the payment order")
}
