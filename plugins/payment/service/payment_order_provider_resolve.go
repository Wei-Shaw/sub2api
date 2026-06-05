package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment/provider"
)

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
	notificationTradeNo := s.persistUpstreamTradeNo(ctx, o, queryRef, resp)
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

// persistUpstreamTradeNo updates the order's trade no if the upstream
// returned a new one. Returns the trade no to use for the notification.
func (s *PaymentService) persistUpstreamTradeNo(
	ctx context.Context,
	o *pluginent.PaymentOrder,
	queryRef string,
	resp *payment.QueryOrderResponse,
) string {
	notificationTradeNo := o.PaymentTradeNo
	upstreamTradeNo := strings.TrimSpace(resp.TradeNo)
	if !paymentOrderShouldPersistUpstreamTradeNo(queryRef, upstreamTradeNo, notificationTradeNo) {
		return notificationTradeNo
	}
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
	return notificationTradeNo
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
// provider expects to look up an order.
func paymentOrderQueryReference(order *pluginent.PaymentOrder, prov payment.Provider) string {
	if order == nil {
		return ""
	}
	providerKey := resolveQueryProviderKey(order, prov)
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

// resolveQueryProviderKey determines the provider key for query reference
// resolution, trying multiple sources in priority order.
func resolveQueryProviderKey(order *pluginent.PaymentOrder, prov payment.Provider) string {
	if prov != nil {
		if k := strings.TrimSpace(prov.ProviderKey()); k != "" {
			return k
		}
	}
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
		if k := strings.TrimSpace(snapshot.ProviderKey); k != "" {
			return k
		}
	}
	if k := strings.TrimSpace(psStringValue(order.ProviderKey)); k != "" {
		return k
	}
	return strings.TrimSpace(order.PaymentType)
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

// getOrderProvider creates a provider using the order's pinned instance
// configuration.
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
	return s.resolveProviderViaRegistry(ctx, o)
}

// resolveProviderViaRegistry falls back to the provider registry for
// legacy pre-snapshot orders without pinned instance data.
func (s *PaymentService) resolveProviderViaRegistry(ctx context.Context, o *pluginent.PaymentOrder) (payment.Provider, error) {
	providerKey := paymentOrderFallbackProviderKey(o)
	if providerKey == "" {
		return nil, fmt.Errorf("order %d provider fallback key is missing", o.ID)
	}
	if s.registry == nil {
		return nil, fmt.Errorf("order %d provider registry is unavailable", o.ID)
	}
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
// payment_provider_instances row.
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
// buggy provider integrations.
func isValidProviderAmount(amount decimal.Decimal) bool {
	return amount.IsPositive()
}
