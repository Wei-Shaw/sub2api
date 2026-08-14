package service

import (
	"context"
	"log/slog"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

// maxPaymentReferenceLen bounds an identifier that goes into a provider URL
// path. Both providers issue numeric ids far below this.
const maxPaymentReferenceLen = 64

// adoptPaymentReference records a payment identifier the checkout redirect
// carried, once the provider confirms it belongs to this order.
//
// A hosted checkout hands us an invoice id, but an invoice can only be polled
// through an endpoint NOWPayments gates behind a bearer token we do not hold.
// The redirect back from checkout carries the payment id instead, and the
// payment-scoped endpoint needs nothing but the API key — so promoting it to
// the order's trade number is what makes polling work at all. It is persisted
// rather than used for one lookup because the reconcile cron never sees a
// redirect, and it is the cron, not the result page, that recovers an order
// whose buyer closed the tab.
func (s *PaymentService) adoptPaymentReference(ctx context.Context, o *dbent.PaymentOrder, reference string) {
	reference = strings.TrimSpace(reference)
	if o == nil || reference == "" {
		return
	}
	if o.Status != OrderStatusPending && o.Status != OrderStatusExpired {
		return
	}
	if strings.EqualFold(reference, strings.TrimSpace(o.PaymentTradeNo)) {
		return
	}
	if !isSafePaymentReference(reference) {
		slog.Warn("rejected malformed payment reference", "orderID", o.ID)
		return
	}

	prov, err := s.getOrderProvider(ctx, o)
	if err != nil {
		return
	}
	verifier, ok := prov.(payment.PaymentReferenceVerifier)
	if !ok {
		return
	}

	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	resp, err := verifier.VerifyPaymentReference(ctx, reference, o.OutTradeNo)
	finishProviderCall()
	if err != nil {
		// A reference that does not verify is the ordinary case for a stale or
		// tampered link, so this is audited rather than returned: the caller is
		// still entitled to the order's real state.
		s.writeAuditLog(ctx, o.ID, "PAYMENT_REFERENCE_REJECTED", prov.ProviderKey(), map[string]any{
			"reference": reference,
			"detail":    err.Error(),
		})
		return
	}

	tradeNo := strings.TrimSpace(resp.TradeNo)
	if tradeNo == "" {
		return
	}
	updated, err := s.entClient.PaymentOrder.Update().
		Where(
			paymentorder.IDEQ(o.ID),
			// Only a still-open order may have its trade number redirected; a
			// settled one already points at the payment that paid it.
			paymentorder.StatusIn(OrderStatusPending, OrderStatusExpired),
		).
		SetPaymentTradeNo(tradeNo).
		Save(ctx)
	if err != nil {
		slog.Error("persist verified payment reference failed", "orderID", o.ID, "tradeNo", tradeNo, "error", err)
		return
	}
	if updated == 0 {
		return
	}
	o.PaymentTradeNo = tradeNo
	slog.Info("adopted verified payment reference", "orderID", o.ID, "tradeNo", tradeNo)
}

// isSafePaymentReference keeps an untrusted identifier to the shape providers
// actually issue, so it can be placed in a request path without surprises.
func isSafePaymentReference(reference string) bool {
	if reference == "" || len(reference) > maxPaymentReferenceLen {
		return false
	}
	for _, ch := range reference {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case ch == '_' || ch == '-':
		default:
			return false
		}
	}
	return true
}
