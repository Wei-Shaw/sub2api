package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	paymentprovider "github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

// --- Cancel & Expire ---

// Cancel rate limit configuration constants.
const (
	rateLimitUnitDay           = "day"
	rateLimitUnitMinute        = "minute"
	rateLimitUnitHour          = "hour"
	rateLimitModeFixed         = "fixed"
	checkPaidResultAlreadyPaid = "already_paid"
	checkPaidResultCancelled   = "cancelled"
	checkPaidResultUnpaid      = "unpaid"
	checkPaidResultUnknown     = "unknown"
	checkPaidResultRejected    = "rejected"

	pendingWxpayReconcileLimit   = 20
	pendingEasyPayReconcileLimit = 50
)

type checkPaidOptions struct {
	cancelIfUnpaid bool
	recoverySource string
}

type checkPaidResult struct {
	outcome   string
	recovered bool
}

type paymentRecoveryTrace struct {
	recovered bool
}

type paymentRecoveryContextValue struct {
	source    string
	queriedAt time.Time
	trace     *paymentRecoveryTrace
}

type paymentRecoveryContextKey struct{}

func withPaymentRecoveryContext(ctx context.Context, source string, trace *paymentRecoveryTrace) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, paymentRecoveryContextKey{}, paymentRecoveryContextValue{
		source: strings.TrimSpace(source), queriedAt: time.Now(), trace: trace,
	})
}

func paymentRecoveryContextFrom(ctx context.Context) (paymentRecoveryContextValue, bool) {
	if ctx == nil {
		return paymentRecoveryContextValue{}, false
	}
	value, ok := ctx.Value(paymentRecoveryContextKey{}).(paymentRecoveryContextValue)
	return value, ok && value.source != ""
}

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
		slog.Error("check cancel rate limit failed", "userID", userID, "error", err)
		return nil // fail open
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
	// rolling window
	switch unit {
	case rateLimitUnitMinute:
		return now.Add(-time.Duration(w) * time.Minute)
	case rateLimitUnitDay:
		return now.AddDate(0, 0, -w)
	default: // hour
		return now.Add(-time.Duration(w) * time.Hour)
	}
}

func (s *PaymentService) CancelOrder(ctx context.Context, orderID, userID int64) (string, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
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

func (s *PaymentService) AdminCancelOrder(ctx context.Context, orderID int64) (string, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return "", infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status != OrderStatusPending {
		return "", infraerrors.BadRequest("INVALID_STATUS", "order cannot be cancelled in current status")
	}
	return s.cancelCore(ctx, o, OrderStatusCancelled, "admin", "admin cancelled order")
}

func (s *PaymentService) cancelCore(ctx context.Context, o *dbent.PaymentOrder, fs, op, ad string) (string, error) {
	if o.PaymentTradeNo != "" || o.PaymentType != "" {
		result := s.checkPaid(ctx, o)
		switch result.outcome {
		case checkPaidResultAlreadyPaid:
			return checkPaidResultAlreadyPaid, nil
		case checkPaidResultUnknown:
			// EasyPay is the provider whose callback-loss fallback requires
			// strict UNKNOWN handling. Preserve the legacy cancellation/expiry
			// behavior for other providers instead of broadening this change.
			if paymentOrderUsesEasyPay(o) {
				return "", infraerrors.ServiceUnavailable("PAYMENT_STATUS_UNKNOWN", "payment status could not be confirmed; please retry later")
			}
		case checkPaidResultRejected:
			return "", infraerrors.Conflict("PAYMENT_REVIEW_REQUIRED", "payment provider returned an unsafe or inconsistent result; manual review is required")
		}
	}
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusPending)).SetStatus(fs).Save(ctx)
	if err != nil {
		return "", fmt.Errorf("update order status: %w", err)
	}
	if c > 0 {
		auditAction := "ORDER_CANCELLED"
		if fs == OrderStatusExpired {
			auditAction = "ORDER_EXPIRED"
		}
		s.writeAuditLog(ctx, o.ID, auditAction, op, map[string]any{"detail": ad})
	}
	return checkPaidResultCancelled, nil
}

func paymentOrderUsesEasyPay(order *dbent.PaymentOrder) bool {
	if order == nil {
		return false
	}
	keys := []string{psStringValue(order.ProviderKey)}
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
		keys = append(keys, snapshot.ProviderKey)
	}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == payment.TypeEasyPay || strings.HasPrefix(key, payment.TypeEasyPay+"_") {
			return true
		}
	}
	return false
}

func (s *PaymentService) checkPaid(ctx context.Context, o *dbent.PaymentOrder) checkPaidResult {
	return s.checkPaidWithOptions(ctx, o, checkPaidOptions{cancelIfUnpaid: true})
}

func (s *PaymentService) reconcilePaid(ctx context.Context, o *dbent.PaymentOrder) checkPaidResult {
	return s.checkPaidWithOptions(ctx, o, checkPaidOptions{})
}

func (s *PaymentService) checkPaidWithOptions(ctx context.Context, o *dbent.PaymentOrder, opts checkPaidOptions) checkPaidResult {
	prov, err := s.getOrderProvider(ctx, o)
	if err != nil {
		slog.Warn("resolve payment provider for query failed", "orderID", o.ID, "error", err)
		return checkPaidResult{outcome: checkPaidResultUnknown}
	}
	queryRef := paymentOrderQueryReference(o, prov)
	if queryRef == "" {
		slog.Warn("payment order query reference is missing", "orderID", o.ID, "provider", prov.ProviderKey())
		return checkPaidResult{outcome: checkPaidResultUnknown}
	}
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	resp, err := prov.QueryOrder(ctx, queryRef)
	finishProviderCall()
	if err != nil {
		if errors.Is(err, paymentprovider.ErrEasyPayQueryIdentityMismatch) {
			s.writeAuditLog(ctx, o.ID, "PAYMENT_QUERY_IDENTITY_MISMATCH", prov.ProviderKey(), map[string]any{
				"query_ref": queryRef,
				"detail":    err.Error(),
			})
			return checkPaidResult{outcome: checkPaidResultRejected}
		}
		slog.Warn("query upstream failed", "orderID", o.ID, "error", err)
		return checkPaidResult{outcome: checkPaidResultUnknown}
	}
	if resp == nil {
		slog.Warn("query upstream returned an empty response", "orderID", o.ID, "provider", prov.ProviderKey())
		return checkPaidResult{outcome: checkPaidResultUnknown}
	}
	if resp.Status == payment.ProviderStatusPaid {
		if !isValidProviderAmount(resp.Amount) {
			s.writeAuditLog(ctx, o.ID, "PAYMENT_INVALID_AMOUNT", prov.ProviderKey(), map[string]any{
				"expected": o.PayAmount,
				"paid":     resp.Amount,
				"tradeNo":  resp.TradeNo,
				"queryRef": queryRef,
			})
			slog.Warn("query upstream returned invalid paid amount", "orderID", o.ID, "queryRef", queryRef, "paid", resp.Amount)
			retriedResp, retryOK := requeryPaidOrderOnce(ctx, prov, queryRef)
			if !retryOK {
				return checkPaidResult{outcome: checkPaidResultRejected}
			}
			resp = retriedResp
		}
		notificationTradeNo := o.PaymentTradeNo
		if upstreamTradeNo := strings.TrimSpace(resp.TradeNo); paymentOrderShouldPersistUpstreamTradeNo(queryRef, upstreamTradeNo, notificationTradeNo) {
			if _, updateErr := s.entClient.PaymentOrder.Update().
				Where(paymentorder.IDEQ(o.ID)).
				SetPaymentTradeNo(upstreamTradeNo).
				Save(ctx); updateErr != nil {
				slog.Error("persist upstream trade no during checkPaid failed", "orderID", o.ID, "tradeNo", upstreamTradeNo, "error", updateErr)
			} else {
				o.PaymentTradeNo = upstreamTradeNo
			}
			notificationTradeNo = upstreamTradeNo
		}
		trace := &paymentRecoveryTrace{}
		notificationCtx := ctx
		if opts.recoverySource != "" {
			notificationCtx = withPaymentRecoveryContext(ctx, opts.recoverySource, trace)
		}
		if err := s.HandlePaymentNotification(notificationCtx, &payment.PaymentNotification{TradeNo: notificationTradeNo, OrderID: o.OutTradeNo, Amount: resp.Amount, Status: payment.ProviderStatusSuccess, Metadata: resp.Metadata}, prov.ProviderKey()); err != nil {
			slog.Error("fulfillment failed during checkPaid", "orderID", o.ID, "error", err)
			return checkPaidResult{outcome: checkPaidResultRejected}
		}
		return checkPaidResult{outcome: checkPaidResultAlreadyPaid, recovered: trace.recovered}
	}
	switch resp.Status {
	case payment.ProviderStatusPending, payment.ProviderStatusFailed:
		// These are explicit, successfully parsed provider states. Only these
		// states are safe to cancel or expire; query failures stay UNKNOWN.
	default:
		slog.Warn("query upstream returned unknown status", "orderID", o.ID, "provider", prov.ProviderKey(), "status", resp.Status)
		return checkPaidResult{outcome: checkPaidResultUnknown}
	}
	if opts.cancelIfUnpaid {
		if cp, ok := prov.(payment.CancelableProvider); ok {
			finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
			_ = cp.CancelPayment(ctx, queryRef)
			finishProviderCall()
		}
	}
	return checkPaidResult{outcome: checkPaidResultUnpaid}
}

func requeryPaidOrderOnce(ctx context.Context, prov payment.Provider, queryRef string) (*payment.QueryOrderResponse, bool) {
	if prov == nil || strings.TrimSpace(queryRef) == "" {
		return nil, false
	}
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	resp, err := prov.QueryOrder(ctx, queryRef)
	finishProviderCall()
	if err != nil {
		slog.Warn("query upstream retry failed", "queryRef", queryRef, "error", err)
		return nil, false
	}
	if resp == nil || resp.Status != payment.ProviderStatusPaid || !isValidProviderAmount(resp.Amount) {
		return nil, false
	}
	return resp, true
}

func paymentOrderQueryReference(order *dbent.PaymentOrder, prov payment.Provider) string {
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

// VerifyOrderByOutTradeNo actively queries the upstream provider to check
// if a payment was made, and processes it if so. This handles the case where
// the provider's notify callback was missed (e.g. EasyPay popup mode).
func (s *PaymentService) VerifyOrderByOutTradeNo(ctx context.Context, outTradeNo string, userID int64) (*dbent.PaymentOrder, error) {
	outTradeNo, err := normalizeOrderLookupOutTradeNo(outTradeNo)
	if err != nil {
		return nil, err
	}
	o, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNo(outTradeNo)).
		Only(ctx)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	// Only verify orders that are still pending or recently expired
	if o.Status == OrderStatusPending || o.Status == OrderStatusExpired {
		result := s.reconcilePaid(ctx, o)
		if result.outcome == checkPaidResultAlreadyPaid {
			// Reload order to get updated status
			o, err = s.entClient.PaymentOrder.Get(ctx, o.ID)
			if err != nil {
				return nil, fmt.Errorf("reload order: %w", err)
			}
		}
	}
	return o, nil
}

// ReconcilePendingPaymentOrders actively checks providers that require a
// server-side callback fallback. Each provider has an independent batch so one
// provider's backlog cannot starve another.
func (s *PaymentService) ReconcilePendingPaymentOrders(ctx context.Context) (int, error) {
	type providerReconcileResult struct {
		recovered int
		err       error
	}
	results := make(chan providerReconcileResult, 2)

	// A slow or unavailable provider must not consume the whole cycle and starve
	// the other provider's callback-recovery path.
	go func() {
		recovered, err := s.ReconcilePendingWxpayOrders(ctx)
		results <- providerReconcileResult{recovered: recovered, err: err}
	}()
	go func() {
		easyPayEnabled := true
		if s.configService != nil {
			cfg, err := s.configService.GetPaymentConfig(ctx)
			if err != nil {
				results <- providerReconcileResult{err: fmt.Errorf("load payment reconcile settings: %w", err)}
				return
			}
			easyPayEnabled = cfg.EasyPayAutoReconcileEnabled
		}
		if !easyPayEnabled {
			results <- providerReconcileResult{}
			return
		}
		recovered, err := s.ReconcilePendingEasyPayOrders(ctx)
		results <- providerReconcileResult{recovered: recovered, err: err}
	}()

	recovered := 0
	var reconcileErrors []error
	for range 2 {
		result := <-results
		recovered += result.recovered
		if result.err != nil {
			reconcileErrors = append(reconcileErrors, result.err)
		}
	}
	return recovered, errors.Join(reconcileErrors...)
}

// ReconcilePendingWxpayOrders keeps the existing missed-notification fallback
// for native WeChat providers.
func (s *PaymentService) ReconcilePendingWxpayOrders(ctx context.Context) (int, error) {
	now := time.Now()
	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.StatusEQ(OrderStatusPending),
			paymentorder.ExpiresAtGT(now),
			paymentorder.Or(
				paymentorder.PaymentTypeEQ(payment.TypeWxpay),
				paymentorder.PaymentTypeHasPrefix(payment.TypeWxpay+"_"),
				paymentorder.ProviderKeyEQ(payment.TypeWxpay),
				paymentorder.ProviderKeyHasPrefix(payment.TypeWxpay+"_"),
			),
		).
		Order(dbent.Asc(paymentorder.FieldCreatedAt)).
		Limit(pendingWxpayReconcileLimit).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query pending wxpay orders: %w", err)
	}

	recovered := 0
	for _, order := range orders {
		if ctx.Err() != nil {
			break
		}
		result := s.checkPaidWithOptions(ctx, order, checkPaidOptions{recoverySource: "scheduled_reconcile"})
		if result.recovered {
			recovered++
		}
	}
	return recovered, nil
}

// ReconcilePendingEasyPayOrders checks EasyPay by provider_key. The visible
// payment_type is normally alipay or wxpay, so filtering payment_type=easypay
// would silently miss production orders.
func (s *PaymentService) ReconcilePendingEasyPayOrders(ctx context.Context) (int, error) {
	s.easyPayReconcileMu.Lock()
	defer s.easyPayReconcileMu.Unlock()

	now := time.Now()
	cursor := s.easyPayReconcileCursor
	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.StatusEQ(OrderStatusPending),
			paymentorder.ExpiresAtGT(now),
			paymentorder.IDGT(cursor),
			paymentorder.Or(
				paymentorder.ProviderKeyEQ(payment.TypeEasyPay),
				paymentorder.ProviderKeyHasPrefix(payment.TypeEasyPay+"_"),
			),
		).
		Order(dbent.Asc(paymentorder.FieldID)).
		Limit(pendingEasyPayReconcileLimit).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query pending easypay orders: %w", err)
	}
	// Wrap only after the cursor reaches the end. Do not fill a short tail with
	// old rows in the same cycle: that keeps the batch boundary observable and
	// guarantees newer orders get their turn before older unpaid rows repeat.
	if len(orders) == 0 && cursor > 0 {
		wrapped, wrapErr := s.entClient.PaymentOrder.Query().
			Where(
				paymentorder.StatusEQ(OrderStatusPending),
				paymentorder.ExpiresAtGT(now),
				paymentorder.IDLTE(cursor),
				paymentorder.Or(
					paymentorder.ProviderKeyEQ(payment.TypeEasyPay),
					paymentorder.ProviderKeyHasPrefix(payment.TypeEasyPay+"_"),
				),
			).
			Order(dbent.Asc(paymentorder.FieldID)).
			Limit(pendingEasyPayReconcileLimit).
			All(ctx)
		if wrapErr != nil {
			return 0, fmt.Errorf("query wrapped pending easypay orders: %w", wrapErr)
		}
		orders = append(orders, wrapped...)
	}

	recovered := 0
	for _, order := range orders {
		if ctx.Err() != nil {
			break
		}
		// Move the cursor even when the provider is temporarily unavailable so
		// a long-lived unpaid order cannot starve newer orders forever.
		s.easyPayReconcileCursor = order.ID
		result := s.checkPaidWithOptions(ctx, order, checkPaidOptions{recoverySource: "scheduled_reconcile"})
		if result.recovered {
			recovered++
		}
	}
	return recovered, nil
}

// VerifyOrderPublic returns the currently persisted public order state without
// triggering any upstream reconciliation. Signed resume-token recovery is the
// only public recovery path allowed to query upstream state.
func (s *PaymentService) VerifyOrderPublic(ctx context.Context, outTradeNo string) (*dbent.PaymentOrder, error) {
	outTradeNo, err := normalizeOrderLookupOutTradeNo(outTradeNo)
	if err != nil {
		return nil, err
	}
	o, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNo(outTradeNo)).
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

func (s *PaymentService) ExpireTimedOutOrders(ctx context.Context) (int, error) {
	now := time.Now()
	orders, err := s.entClient.PaymentOrder.Query().Where(paymentorder.StatusEQ(OrderStatusPending), paymentorder.ExpiresAtLTE(now)).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query expired: %w", err)
	}
	n := 0
	for _, o := range orders {
		// Check upstream payment status before expiring — the user may have
		// paid just before timeout and the webhook hasn't arrived yet.
		outcome, cancelErr := s.cancelCore(ctx, o, OrderStatusExpired, "system", "order expired")
		if cancelErr != nil {
			slog.Warn("order expiry deferred because payment status is uncertain", "orderID", o.ID, "error", cancelErr)
			continue
		}
		if outcome == checkPaidResultAlreadyPaid {
			slog.Info("order was paid during expiry", "orderID", o.ID)
			continue
		}
		if outcome != "" {
			n++
		}
	}
	return n, nil
}

// getOrderProvider creates a provider using the order's original instance config.
// Falls back to registry lookup if instance ID is missing (legacy orders).
func (s *PaymentService) getOrderProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
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
	providerKey := paymentOrderFallbackProviderKey(s.registry, o)
	if providerKey == "" {
		return nil, fmt.Errorf("order %d provider fallback key is missing", o.ID)
	}
	if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
		return nil, fmt.Errorf("order %d provider fallback is ambiguous for %s", o.ID, providerKey)
	}
	s.EnsureProviders(ctx)
	return s.registry.GetProvider(o.PaymentType)
}

func paymentOrderAllowsRegistryFallback(order *dbent.PaymentOrder) bool {
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

func paymentOrderFallbackProviderKey(registry *payment.Registry, order *dbent.PaymentOrder) string {
	if order == nil {
		return ""
	}
	if registry != nil {
		if key := strings.TrimSpace(registry.GetProviderKey(payment.PaymentType(order.PaymentType))); key != "" {
			return key
		}
	}
	return strings.TrimSpace(payment.GetBasePaymentType(strings.TrimSpace(order.PaymentType)))
}

func (s *PaymentService) createProviderFromInstance(ctx context.Context, inst *dbent.PaymentProviderInstance) (payment.Provider, error) {
	if inst == nil {
		return nil, fmt.Errorf("payment provider instance is missing")
	}

	cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
	if err != nil {
		return nil, fmt.Errorf("load provider instance config: %w", err)
	}
	if inst.PaymentMode != "" {
		cfg["paymentMode"] = inst.PaymentMode
	}

	instID := strconv.FormatInt(int64(inst.ID), 10)
	prov, err := createPaymentProviderFromInstance(inst.ProviderKey, instID, cfg)
	if err != nil {
		return nil, fmt.Errorf("create provider from instance: %w", err)
	}
	return prov, nil
}

func psStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
