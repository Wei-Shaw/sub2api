package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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

	pendingWxpayReconcileLimit = 20
)

type checkPaidOptions struct {
	cancelIfUnpaid bool
REDACTED

func (s *PaymentService) checkCancelRateLimit(ctx context.Context, userID int64, cfg *PaymentConfig) error {
	if !cfg.CancelRateLimitEnabled || cfg.CancelRateLimitMax <= 0 {
		return nil
REDACTED
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
REDACTED
	if count >= cfg.CancelRateLimitMax {
		return infraerrors.TooManyRequests("CANCEL_RATE_LIMITED", "cancel rate limited").
			WithMetadata(map[string]string{
				"max":    strconv.Itoa(cfg.CancelRateLimitMax),
				"window": strconv.Itoa(cfg.CancelRateLimitWindow),
				"unit":   cfg.CancelRateLimitUnit,
		REDACTED)
REDACTED
	return nil
REDACTED

func cancelRateLimitWindowStart(cfg *PaymentConfig) time.Time {
	now := time.Now()
	w := cfg.CancelRateLimitWindow
	if w <= 0 {
		w = 1
REDACTED
	unit := cfg.CancelRateLimitUnit
	if unit == "" {
		unit = rateLimitUnitDay
REDACTED
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
	REDACTED
REDACTED
	// rolling window
	switch unit {
	case rateLimitUnitMinute:
		return now.Add(-time.Duration(w) * time.Minute)
	case rateLimitUnitDay:
		return now.AddDate(0, 0, -w)
	default: // hour
		return now.Add(-time.Duration(w) * time.Hour)
REDACTED
REDACTED

func (s *PaymentService) CancelOrder(ctx context.Context, orderID, userID int64) (string, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return "", infraerrors.NotFound("NOT_FOUND", "order not found")
REDACTED
	if o.UserID != userID {
		return "", infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
REDACTED
	if o.Status != OrderStatusPending {
		return "", infraerrors.BadRequest("INVALID_STATUS", "order cannot be cancelled in current status")
REDACTED
	return s.cancelCore(ctx, o, OrderStatusCancelled, fmt.Sprintf("user:%d", userID), "user cancelled order")
REDACTED

func (s *PaymentService) AdminCancelOrder(ctx context.Context, orderID int64) (string, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return "", infraerrors.NotFound("NOT_FOUND", "order not found")
REDACTED
	if o.Status != OrderStatusPending {
		return "", infraerrors.BadRequest("INVALID_STATUS", "order cannot be cancelled in current status")
REDACTED
	return s.cancelCore(ctx, o, OrderStatusCancelled, "admin", "admin cancelled order")
REDACTED

func (s *PaymentService) cancelCore(ctx context.Context, o *dbent.PaymentOrder, fs, op, ad string) (string, error) {
	if o.PaymentTradeNo != "" || o.PaymentType != "" {
		if s.checkPaid(ctx, o) == checkPaidResultAlreadyPaid {
			return checkPaidResultAlreadyPaid, nil
	REDACTED
REDACTED
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusPending)).SetStatus(fs).Save(ctx)
	if err != nil {
		return "", fmt.Errorf("update order status: %w", err)
REDACTED
	if c > 0 {
		auditAction := "ORDER_CANCELLED"
		if fs == OrderStatusExpired {
			auditAction = "ORDER_EXPIRED"
	REDACTED
		s.writeAuditLog(ctx, o.ID, auditAction, op, map[string]any{"detail": adREDACTED)
REDACTED
	return checkPaidResultCancelled, nil
REDACTED

func (s *PaymentService) checkPaid(ctx context.Context, o *dbent.PaymentOrder) string {
	return s.checkPaidWithOptions(ctx, o, checkPaidOptions{cancelIfUnpaid: trueREDACTED)
REDACTED

func (s *PaymentService) reconcilePaid(ctx context.Context, o *dbent.PaymentOrder) string {
	return s.checkPaidWithOptions(ctx, o, checkPaidOptions{REDACTED)
REDACTED

func (s *PaymentService) checkPaidWithOptions(ctx context.Context, o *dbent.PaymentOrder, opts checkPaidOptions) string {
	prov, err := s.getOrderProvider(ctx, o)
	if err != nil {
		return ""
REDACTED
	queryRef := paymentOrderQueryReference(o, prov)
	if queryRef == "" {
		return ""
REDACTED
	resp, err := prov.QueryOrder(ctx, queryRef)
	if err != nil {
		slog.Warn("query upstream failed", "orderID", o.ID, "error", err)
		return ""
REDACTED
	if resp.Status == payment.ProviderStatusPaid {
		if !isValidProviderAmount(resp.Amount) {
			s.writeAuditLog(ctx, o.ID, "PAYMENT_INVALID_AMOUNT", prov.ProviderKey(), map[string]any{
				"expected": o.PayAmount,
				"paid":     resp.Amount,
				"tradeNo":  resp.TradeNo,
				"queryRef": queryRef,
		REDACTED)
			slog.Warn("query upstream returned invalid paid amount", "orderID", o.ID, "queryRef", queryRef, "paid", resp.Amount)
			retriedResp, retryOK := requeryPaidOrderOnce(ctx, prov, queryRef)
			if !retryOK {
				return ""
		REDACTED
			resp = retriedResp
	REDACTED
		notificationTradeNo := o.PaymentTradeNo
		if upstreamTradeNo := strings.TrimSpace(resp.TradeNo); paymentOrderShouldPersistUpstreamTradeNo(queryRef, upstreamTradeNo, notificationTradeNo) {
			if _, updateErr := s.entClient.PaymentOrder.Update().
				Where(paymentorder.IDEQ(o.ID)).
				SetPaymentTradeNo(upstreamTradeNo).
				Save(ctx); updateErr != nil {
				slog.Error("persist upstream trade no during checkPaid failed", "orderID", o.ID, "tradeNo", upstreamTradeNo, "error", updateErr)
		REDACTED else {
				o.PaymentTradeNo = upstreamTradeNo
		REDACTED
			notificationTradeNo = upstreamTradeNo
	REDACTED
		if err := s.HandlePaymentNotification(ctx, &payment.PaymentNotification{TradeNo: notificationTradeNo, OrderID: o.OutTradeNo, Amount: resp.Amount, Status: payment.ProviderStatusSuccess, Metadata: resp.MetadataREDACTED, prov.ProviderKey()); err != nil {
			slog.Error("fulfillment failed during checkPaid", "orderID", o.ID, "error", err)
			// Still return already_paid — order was paid, fulfillment can be retried
	REDACTED
		return checkPaidResultAlreadyPaid
REDACTED
	if !opts.cancelIfUnpaid {
		return ""
REDACTED
	if cp, ok := prov.(payment.CancelableProvider); ok {
		_ = cp.CancelPayment(ctx, queryRef)
REDACTED
	return ""
REDACTED

func requeryPaidOrderOnce(ctx context.Context, prov payment.Provider, queryRef string) (*payment.QueryOrderResponse, bool) {
	if prov == nil || strings.TrimSpace(queryRef) == "" {
		return nil, false
REDACTED
	resp, err := prov.QueryOrder(ctx, queryRef)
	if err != nil {
		slog.Warn("query upstream retry failed", "queryRef", queryRef, "error", err)
		return nil, false
REDACTED
	if resp == nil || resp.Status != payment.ProviderStatusPaid || !isValidProviderAmount(resp.Amount) {
		return nil, false
REDACTED
	return resp, true
REDACTED

func paymentOrderQueryReference(order *dbent.PaymentOrder, prov payment.Provider) string {
	if order == nil {
		return ""
REDACTED

	providerKey := ""
	if prov != nil {
		providerKey = strings.TrimSpace(prov.ProviderKey())
REDACTED
	if providerKey == "" {
		if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
			providerKey = strings.TrimSpace(snapshot.ProviderKey)
	REDACTED
REDACTED
	if providerKey == "" {
		providerKey = strings.TrimSpace(psStringValue(order.ProviderKey))
REDACTED
	if providerKey == "" {
		providerKey = strings.TrimSpace(order.PaymentType)
REDACTED

	switch payment.GetBasePaymentType(providerKey) {
	case payment.TypeAlipay, payment.TypeEasyPay, payment.TypeWxpay:
		return strings.TrimSpace(order.OutTradeNo)
	default:
		if tradeNo := strings.TrimSpace(order.PaymentTradeNo); tradeNo != "" {
			return tradeNo
	REDACTED
		return strings.TrimSpace(order.OutTradeNo)
REDACTED
REDACTED

func paymentOrderShouldPersistUpstreamTradeNo(queryRef, upstreamTradeNo, currentTradeNo string) bool {
	upstreamTradeNo = strings.TrimSpace(upstreamTradeNo)
	if upstreamTradeNo == "" {
		return false
REDACTED
	if strings.EqualFold(upstreamTradeNo, strings.TrimSpace(currentTradeNo)) {
		return false
REDACTED
	if strings.EqualFold(upstreamTradeNo, strings.TrimSpace(queryRef)) {
		return false
REDACTED
	return true
REDACTED

// VerifyOrderByOutTradeNo actively queries the upstream provider to check
// if a payment was made, and processes it if so. This handles the case where
// the provider's notify callback was missed (e.g. EasyPay popup mode).
func (s *PaymentService) VerifyOrderByOutTradeNo(ctx context.Context, outTradeNo string, userID int64) (*dbent.PaymentOrder, error) {
	outTradeNo, err := normalizeOrderLookupOutTradeNo(outTradeNo)
	if err != nil {
		return nil, err
REDACTED
	o, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNo(outTradeNo)).
		Only(ctx)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
REDACTED
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
REDACTED
	// Only verify orders that are still pending or recently expired
	if o.Status == OrderStatusPending || o.Status == OrderStatusExpired {
		result := s.reconcilePaid(ctx, o)
		if result == checkPaidResultAlreadyPaid {
			// Reload order to get updated status
			o, err = s.entClient.PaymentOrder.Get(ctx, o.ID)
			if err != nil {
				return nil, fmt.Errorf("reload order: %w", err)
		REDACTED
	REDACTED
REDACTED
	return o, nil
REDACTED

// ReconcilePendingWxpayOrders actively checks recent pending WeChat orders so
// missed provider notifications do not wait until order expiry to fulfill.
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
REDACTED

	recovered := 0
	for _, order := range orders {
		if s.reconcilePaid(ctx, order) == checkPaidResultAlreadyPaid {
			recovered++
	REDACTED
REDACTED
	return recovered, nil
REDACTED

// VerifyOrderPublic returns the currently persisted public order state without
// triggering any upstream reconciliation. Signed resume-token recovery is the
// only public recovery path allowed to query upstream state.
func (s *PaymentService) VerifyOrderPublic(ctx context.Context, outTradeNo string) (*dbent.PaymentOrder, error) {
	outTradeNo, err := normalizeOrderLookupOutTradeNo(outTradeNo)
	if err != nil {
		return nil, err
REDACTED
	o, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNo(outTradeNo)).
		Only(ctx)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
REDACTED
	return o, nil
REDACTED

func normalizeOrderLookupOutTradeNo(raw string) (string, error) {
	outTradeNo := strings.TrimSpace(raw)
	if outTradeNo == "" {
		return "", infraerrors.BadRequest("INVALID_OUT_TRADE_NO", "out_trade_no is required")
REDACTED
	if len(outTradeNo) > 64 {
		return "", infraerrors.BadRequest("INVALID_OUT_TRADE_NO", "out_trade_no is invalid")
REDACTED
	for _, ch := range outTradeNo {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case ch == '_' || ch == '-':
		default:
			return "", infraerrors.BadRequest("INVALID_OUT_TRADE_NO", "out_trade_no is invalid")
	REDACTED
REDACTED
	return outTradeNo, nil
REDACTED

func (s *PaymentService) ExpireTimedOutOrders(ctx context.Context) (int, error) {
	now := time.Now()
	orders, err := s.entClient.PaymentOrder.Query().Where(paymentorder.StatusEQ(OrderStatusPending), paymentorder.ExpiresAtLTE(now)).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query expired: %w", err)
REDACTED
	n := 0
	for _, o := range orders {
		// Check upstream payment status before expiring — the user may have
		// paid just before timeout and the webhook hasn't arrived yet.
		outcome, _ := s.cancelCore(ctx, o, OrderStatusExpired, "system", "order expired")
		if outcome == checkPaidResultAlreadyPaid {
			slog.Info("order was paid during expiry", "orderID", o.ID)
			continue
	REDACTED
		if outcome != "" {
			n++
	REDACTED
REDACTED
	return n, nil
REDACTED

// getOrderProvider creates a provider using the order's original instance config.
// Falls back to registry lookup if instance ID is missing (legacy orders).
func (s *PaymentService) getOrderProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("load order provider instance: %w", err)
REDACTED
	if inst != nil {
		return s.createProviderFromInstance(ctx, inst)
REDACTED
	if !paymentOrderAllowsRegistryFallback(o) {
		return nil, fmt.Errorf("order %d provider instance is unresolved", o.ID)
REDACTED
	providerKey := paymentOrderFallbackProviderKey(s.registry, o)
	if providerKey == "" {
		return nil, fmt.Errorf("order %d provider fallback key is missing", o.ID)
REDACTED
	if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
		return nil, fmt.Errorf("order %d provider fallback is ambiguous for %s", o.ID, providerKey)
REDACTED
	s.EnsureProviders(ctx)
	return s.registry.GetProvider(o.PaymentType)
REDACTED

func paymentOrderAllowsRegistryFallback(order *dbent.PaymentOrder) bool {
	if order == nil {
		return false
REDACTED
	if psOrderProviderSnapshot(order) != nil {
		return false
REDACTED
	if strings.TrimSpace(psStringValue(order.ProviderInstanceID)) != "" {
		return false
REDACTED
	if strings.TrimSpace(psStringValue(order.ProviderKey)) != "" {
		return false
REDACTED
	return true
REDACTED

func paymentOrderFallbackProviderKey(registry *payment.Registry, order *dbent.PaymentOrder) string {
	if order == nil {
		return ""
REDACTED
	if registry != nil {
		if key := strings.TrimSpace(registry.GetProviderKey(payment.PaymentType(order.PaymentType))); key != "" {
			return key
	REDACTED
REDACTED
	return strings.TrimSpace(payment.GetBasePaymentType(strings.TrimSpace(order.PaymentType)))
REDACTED

func (s *PaymentService) createProviderFromInstance(ctx context.Context, inst *dbent.PaymentProviderInstance) (payment.Provider, error) {
	if inst == nil {
		return nil, fmt.Errorf("payment provider instance is missing")
REDACTED

	cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
	if err != nil {
		return nil, fmt.Errorf("load provider instance config: %w", err)
REDACTED
	if inst.PaymentMode != "" {
		cfg["paymentMode"] = inst.PaymentMode
REDACTED

	instID := strconv.FormatInt(int64(inst.ID), 10)
	prov, err := createPaymentProviderFromInstance(inst.ProviderKey, instID, cfg)
	if err != nil {
		return nil, fmt.Errorf("create provider from instance: %w", err)
REDACTED
	return prov, nil
REDACTED

func psStringValue(value *string) string {
	if value == nil {
		return ""
REDACTED
	return *value
REDACTED
