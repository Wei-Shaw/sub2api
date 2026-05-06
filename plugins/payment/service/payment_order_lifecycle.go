package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment/provider"
)

// Cancel rate limit configuration constants.
const (
	rateLimitUnitDay           = "day"
	rateLimitUnitMinute        = "minute"
	rateLimitUnitHour          = "hour"
	rateLimitModeFixed         = "fixed"
	checkPaidResultAlreadyPaid = "already_paid"
	checkPaidResultCancelled   = "cancelled"
	// checkPaidResultExpired is returned by cancelCore when an order
	// successfully transitioned from PENDING to EXPIRED via the CAS
	// update (i.e. expiry sweep actually flipped the row, not a no-op).
	checkPaidResultExpired = "expired"
)

// errLifecycleServiceUnavailable signals the plugin service was constructed
// without a working ent client. Handlers translate this to 503.
var errLifecycleServiceUnavailable = errors.New("payment: order lifecycle service unavailable")

// checkCancelRateLimit refuses excessive cancellation activity per user.
// Counts the ORDER_CANCELLED audit-log entries for the user inside the
// configured window and returns TOO_MANY_REQUESTS once the limit is hit.
// Failures during the count are intentionally ignored ("fail open") so a
// transient DB error doesn't block legitimate order creation.
func (s *PaymentService) checkCancelRateLimit(ctx context.Context, userID int64, cfg *PaymentConfig) error {
	if !cfg.CancelRateLimitEnabled || cfg.CancelRateLimitMax <= 0 {
		return nil
	}
	windowStart := cancelRateLimitWindowStart(cfg)
	operator := fmt.Sprintf("user:%d", userID)
	count, err := s.entClient.PaymentAuditLog.Query().
		Where(
			paymentauditlog.ActionEQ("ORDER_CANCELLED"),
			paymentauditlog.OperatorEQ(operator),
			paymentauditlog.CreatedAtGTE(windowStart),
		).Count(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("check cancel rate limit failed", "user_id", userID, "error", err)
		}
		return nil
	}
	if count >= cfg.CancelRateLimitMax {
		return infraerrors.TooManyRequests("CANCEL_RATE_LIMITED", "cancel rate limited").
			WithMetadata(map[string]string{
				"max":    strconv.Itoa(cfg.CancelRateLimitMax),
				"window": strconv.Itoa(cfg.CancelRateLimitWindow),
				"unit":   cfg.CancelRateLimitUnit,
			})
	}
	return nil
}

func cancelRateLimitWindowStart(cfg *PaymentConfig) time.Time {
	now := time.Now()
	w := cfg.CancelRateLimitWindow
	if w <= 0 {
		w = 1
	}
	unit := cfg.CancelRateLimitUnit
	if unit == "" {
		unit = rateLimitUnitDay
	}
	if cfg.CancelRateLimitMode == rateLimitModeFixed {
		return cancelRateLimitFixedStart(now, w, unit)
	}
	return cancelRateLimitRollingStart(now, w, unit)
}

func cancelRateLimitFixedStart(now time.Time, w int, unit string) time.Time {
	switch unit {
	case rateLimitUnitMinute:
		t := now.Truncate(time.Minute)
		return t.Add(-time.Duration(w-1) * time.Minute)
	case rateLimitUnitDay:
		y, m, d := now.Date()
		t := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		return t.AddDate(0, 0, -(w - 1))
	default: // hour
		t := now.Truncate(time.Hour)
		return t.Add(-time.Duration(w-1) * time.Hour)
	}
}

func cancelRateLimitRollingStart(now time.Time, w int, unit string) time.Time {
	switch unit {
	case rateLimitUnitMinute:
		return now.Add(-time.Duration(w) * time.Minute)
	case rateLimitUnitDay:
		return now.AddDate(0, 0, -w)
	default: // hour
		return now.Add(-time.Duration(w) * time.Hour)
	}
}

// CancelOrder is the user-initiated cancel path. Validates ownership and
// the PENDING precondition; the rest of the work — including upstream
// reconciliation — happens inside cancelCore.
func (s *PaymentService) CancelOrder(ctx context.Context, orderID, userID int64) (string, error) {
	if s == nil || s.entClient == nil {
		return "", errLifecycleServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(orderID))
	if err != nil {
		return "", infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return "", infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	if o.Status != OrderStatusPending {
		return "", infraerrors.BadRequest("INVALID_STATUS", "order cannot be cancelled in current status")
	}
	return s.cancelCore(ctx, o, OrderStatusCancelled, fmt.Sprintf("user:%d", userID), "user cancelled order")
}

// AdminCancelOrder is the operator path; identical guarantees minus
// the ownership check.
func (s *PaymentService) AdminCancelOrder(ctx context.Context, orderID int64) (string, error) {
	if s == nil || s.entClient == nil {
		return "", errLifecycleServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(orderID))
	if err != nil {
		return "", infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status != OrderStatusPending {
		return "", infraerrors.BadRequest("INVALID_STATUS", "order cannot be cancelled in current status")
	}
	return s.cancelCore(ctx, o, OrderStatusCancelled, "admin", "admin cancelled order")
}

// cancelCore is the merged implementation behind CancelOrder /
// AdminCancelOrder / ExpireTimedOutOrders. Before transitioning the
// status it asks the upstream provider whether the user actually paid
// — a successful upstream response promotes the order to PAID via the
// fulfillment pipeline and the cancel is suppressed.
func (s *PaymentService) cancelCore(ctx context.Context, o *pluginent.PaymentOrder, fs, op, ad string) (string, error) {
	if o.PaymentTradeNo != "" || o.PaymentType != "" {
		if s.checkPaid(ctx, o) == checkPaidResultAlreadyPaid {
			return checkPaidResultAlreadyPaid, nil
		}
	}
	c, err := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusPending)).
		SetStatus(fs).Save(ctx)
	if err != nil {
		return "", fmt.Errorf("update order status: %w", err)
	}
	if c > 0 {
		auditAction := "ORDER_CANCELLED"
		if fs == OrderStatusExpired {
			auditAction = "ORDER_EXPIRED"
		}
		s.writeAuditLog(ctx, int64(o.ID), auditAction, op, map[string]any{"detail": ad})
		if fs == OrderStatusExpired {
			return checkPaidResultExpired, nil
		}
	}
	return checkPaidResultCancelled, nil
}

// checkPaid asks the upstream provider whether an order has actually
// been paid, even though our DB still shows PENDING. Returns the
// "already_paid" sentinel when the upstream response was processed
// (the order is now PAID/RECHARGING/COMPLETED via HandlePaymentNotification).
func (s *PaymentService) checkPaid(ctx context.Context, o *pluginent.PaymentOrder) string {
	prov, err := s.getOrderProvider(ctx, o)
	if err != nil {
		return ""
	}
	queryRef := paymentOrderQueryReference(o, prov)
	if queryRef == "" {
		return ""
	}
	resp, err := prov.QueryOrder(ctx, queryRef)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("query upstream failed", "order_id", o.ID, "error", err)
		}
		return ""
	}
	if resp.Status == payment.ProviderStatusPaid {
		return s.handleUpstreamPaid(ctx, o, prov, queryRef, resp)
	}
	if cp, ok := prov.(payment.CancelableProvider); ok {
		_ = cp.CancelPayment(ctx, queryRef)
	}
	return ""
}

// handleUpstreamPaid drives the upstream-says-paid branch of checkPaid.
// Validates the amount, retries once on the obvious garbage-value path,
// persists the trade no when the upstream tells us about a fresh one,
// and finally feeds the result through the standard fulfillment pipeline.
func (s *PaymentService) handleUpstreamPaid(
	ctx context.Context,
	o *pluginent.PaymentOrder,
	prov payment.Provider,
	queryRef string,
	resp *payment.QueryOrderResponse,
) string {
	if !isValidProviderAmount(resp.Amount) {
		s.writeAuditLog(ctx, int64(o.ID), "PAYMENT_INVALID_AMOUNT", prov.ProviderKey(), map[string]any{
			"expected": o.PayAmount,
			"paid":     resp.Amount,
			"tradeNo":  resp.TradeNo,
			"queryRef": queryRef,
		})
		retried, ok := requeryPaidOrderOnce(ctx, prov, queryRef)
		if !ok {
			return ""
		}
		resp = retried
	}
	notificationTradeNo := o.PaymentTradeNo
	if upstreamTradeNo := strings.TrimSpace(resp.TradeNo); paymentOrderShouldPersistUpstreamTradeNo(queryRef, upstreamTradeNo, notificationTradeNo) {
		if _, updErr := s.entClient.PaymentOrder.Update().
			Where(paymentorder.IDEQ(o.ID)).
			SetPaymentTradeNo(upstreamTradeNo).
			Save(ctx); updErr != nil {
			if s.logger != nil {
				s.logger.Error("persist upstream trade no during checkPaid failed",
					"order_id", o.ID, "trade_no", upstreamTradeNo, "error", updErr)
			}
		} else {
			o.PaymentTradeNo = upstreamTradeNo
			notificationTradeNo = upstreamTradeNo
		}
	}
	notif := &payment.PaymentNotification{
		TradeNo:  notificationTradeNo,
		OrderID:  o.OutTradeNo,
		Amount:   resp.Amount,
		Status:   payment.ProviderStatusSuccess,
		Metadata: resp.Metadata,
	}
	if err := s.HandlePaymentNotification(ctx, notif, prov.ProviderKey()); err != nil && s.logger != nil {
		s.logger.Error("fulfillment failed during checkPaid", "order_id", o.ID, "error", err)
	}
	return checkPaidResultAlreadyPaid
}

func requeryPaidOrderOnce(ctx context.Context, prov payment.Provider, queryRef string) (*payment.QueryOrderResponse, bool) {
	if prov == nil || strings.TrimSpace(queryRef) == "" {
		return nil, false
	}
	resp, err := prov.QueryOrder(ctx, queryRef)
	if err != nil {
		return nil, false
	}
	if resp == nil || resp.Status != payment.ProviderStatusPaid || !isValidProviderAmount(resp.Amount) {
		return nil, false
	}
	return resp, true
}

// paymentOrderQueryReference returns the identifier the upstream
// provider expects to look up an order. Alipay/WxPay/EasyPay key off
// our own out_trade_no; Stripe and friends prefer the upstream trade_no
// when it has been populated.
func paymentOrderQueryReference(order *pluginent.PaymentOrder, prov payment.Provider) string {
	if order == nil {
		return ""
	}
	providerKey := ""
	if prov != nil {
		providerKey = strings.TrimSpace(prov.ProviderKey())
	}
	if providerKey == "" {
		if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
			providerKey = strings.TrimSpace(snapshot.ProviderKey)
		}
	}
	if providerKey == "" {
		providerKey = strings.TrimSpace(psStringValue(order.ProviderKey))
	}
	if providerKey == "" {
		providerKey = strings.TrimSpace(order.PaymentType)
	}
	switch payment.GetBasePaymentType(providerKey) {
	case payment.TypeAlipay, payment.TypeEasyPay, payment.TypeWxpay:
		return strings.TrimSpace(order.OutTradeNo)
	default:
		if tradeNo := strings.TrimSpace(order.PaymentTradeNo); tradeNo != "" {
			return tradeNo
		}
		return strings.TrimSpace(order.OutTradeNo)
	}
}

func paymentOrderShouldPersistUpstreamTradeNo(queryRef, upstreamTradeNo, currentTradeNo string) bool {
	upstreamTradeNo = strings.TrimSpace(upstreamTradeNo)
	if upstreamTradeNo == "" {
		return false
	}
	if strings.EqualFold(upstreamTradeNo, strings.TrimSpace(currentTradeNo)) {
		return false
	}
	if strings.EqualFold(upstreamTradeNo, strings.TrimSpace(queryRef)) {
		return false
	}
	return true
}

// VerifyOrderByOutTradeNo is the authenticated reconciliation entry
// point for "did I actually pay?" UX flows (e.g. EasyPay popup mode
// where the notify callback may have been missed).
func (s *PaymentService) VerifyOrderByOutTradeNo(ctx context.Context, outTradeNo string, userID int64) (*pluginent.PaymentOrder, error) {
	if s == nil || s.entClient == nil {
		return nil, errLifecycleServiceUnavailable
	}
	outTradeNo, err := normalizeOrderLookupOutTradeNo(outTradeNo)
	if err != nil {
		return nil, err
	}
	o, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNoEQ(outTradeNo)).
		Only(ctx)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	if o.Status == OrderStatusPending || o.Status == OrderStatusExpired {
		if s.checkPaid(ctx, o) == checkPaidResultAlreadyPaid {
			o, err = s.entClient.PaymentOrder.Get(ctx, o.ID)
			if err != nil {
				return nil, fmt.Errorf("reload order: %w", err)
			}
		}
	}
	return o, nil
}

// VerifyOrderPublic returns the persisted public order state without
// triggering upstream reconciliation. Anonymous callers (e.g. result
// page on a different device) only see the local snapshot.
func (s *PaymentService) VerifyOrderPublic(ctx context.Context, outTradeNo string) (*pluginent.PaymentOrder, error) {
	if s == nil || s.entClient == nil {
		return nil, errLifecycleServiceUnavailable
	}
	outTradeNo, err := normalizeOrderLookupOutTradeNo(outTradeNo)
	if err != nil {
		return nil, err
	}
	o, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNoEQ(outTradeNo)).
		Only(ctx)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	return o, nil
}

func normalizeOrderLookupOutTradeNo(raw string) (string, error) {
	outTradeNo := strings.TrimSpace(raw)
	if outTradeNo == "" {
		return "", infraerrors.BadRequest("INVALID_OUT_TRADE_NO", "out_trade_no is required")
	}
	if len(outTradeNo) > 64 {
		return "", infraerrors.BadRequest("INVALID_OUT_TRADE_NO", "out_trade_no is invalid")
	}
	for _, ch := range outTradeNo {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case ch == '_' || ch == '-':
		default:
			return "", infraerrors.BadRequest("INVALID_OUT_TRADE_NO", "out_trade_no is invalid")
		}
	}
	return outTradeNo, nil
}

// ExpireTimedOutOrders sweeps orders whose timeout has elapsed and
// transitions them to EXPIRED. Each candidate is first reconciled
// against the upstream provider so a payment that arrived inside the
// grace window is not lost. Returns the number of orders successfully
// transitioned to EXPIRED so the caller can emit a single summary log
// (and avoid one log line per order).
func (s *PaymentService) ExpireTimedOutOrders(ctx context.Context) (int, error) {
	if s == nil || s.entClient == nil {
		return 0, nil
	}
	now := time.Now()
	orders, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.StatusEQ(OrderStatusPending), paymentorder.ExpiresAtLTE(now)).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query expired: %w", err)
	}
	expired := 0
	for _, o := range orders {
		outcome, _ := s.cancelCore(ctx, o, OrderStatusExpired, "system", "order expired")
		switch outcome {
		case checkPaidResultExpired:
			expired++
		case checkPaidResultAlreadyPaid:
			if s.logger != nil {
				s.logger.Info("order was paid during expiry sweep", "order_id", o.ID)
			}
		}
	}
	return expired, nil
}

// getOrderProvider creates a provider using the order's pinned instance
// configuration. Falls back to the registry only when the order has no
// snapshot, no provider_instance_id, and no provider_key — which is
// only the case for legacy pre-snapshot orders.
func (s *PaymentService) getOrderProvider(ctx context.Context, o *pluginent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("load order provider instance: %w", err)
	}
	if inst != nil {
		return s.createProviderFromInstance(ctx, inst)
	}
	if !paymentOrderAllowsRegistryFallback(o) {
		return nil, fmt.Errorf("order %d provider instance is unresolved", o.ID)
	}
	providerKey := paymentOrderFallbackProviderKey(o)
	if providerKey == "" {
		return nil, fmt.Errorf("order %d provider fallback key is missing", o.ID)
	}
	if s.registry == nil {
		return nil, fmt.Errorf("order %d provider registry is unavailable", o.ID)
	}
	// Ambiguity guard: if multiple enabled instances share the same
	// provider_key, we cannot pick one safely — refunds (and other
	// instance-pinned operations) might hit the wrong merchant. Mirror
	// the webhook-side check so legacy orders without snapshot/instance
	// are rejected explicitly instead of silently routing.
	if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
		return nil, fmt.Errorf("order %d provider fallback is ambiguous for %s", o.ID, providerKey)
	}
	s.EnsureProviders(ctx)
	return s.registry.GetProvider(o.PaymentType)
}

func paymentOrderAllowsRegistryFallback(order *pluginent.PaymentOrder) bool {
	if order == nil {
		return false
	}
	if psOrderProviderSnapshot(order) != nil {
		return false
	}
	if strings.TrimSpace(psStringValue(order.ProviderInstanceID)) != "" {
		return false
	}
	if strings.TrimSpace(psStringValue(order.ProviderKey)) != "" {
		return false
	}
	return true
}

func paymentOrderFallbackProviderKey(order *pluginent.PaymentOrder) string {
	if order == nil {
		return ""
	}
	return strings.TrimSpace(payment.GetBasePaymentType(strings.TrimSpace(order.PaymentType)))
}

// createProviderFromInstance materialises a provider from a stored
// payment_provider_instances row by decrypting its config through the
// load balancer (which owns the SDK SecretEncryptor handle).
func (s *PaymentService) createProviderFromInstance(ctx context.Context, inst *pluginent.PaymentProviderInstance) (payment.Provider, error) {
	if inst == nil {
		return nil, fmt.Errorf("payment provider instance is missing")
	}
	if s == nil || s.loadBalancer == nil {
		return nil, errors.New("payment: load balancer unavailable")
	}
	cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
	if err != nil {
		return nil, fmt.Errorf("load provider instance config: %w", err)
	}
	if inst.PaymentMode != "" {
		cfg["paymentMode"] = inst.PaymentMode
	}
	instID := strconv.FormatInt(int64(inst.ID), 10)
	prov, err := provider.CreateProvider(inst.ProviderKey, instID, cfg)
	if err != nil {
		return nil, fmt.Errorf("create provider from instance: %w", err)
	}
	return prov, nil
}

// isValidProviderAmount filters out non-positive values returned by
// buggy provider integrations. decimal.Decimal carries no NaN/Inf so
// the IEEE-754 guards from the float64 era are no longer required.
func isValidProviderAmount(amount decimal.Decimal) bool {
	return amount.IsPositive()
}
