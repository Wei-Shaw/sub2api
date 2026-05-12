package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/moneyx"
)

// --- Refund Flow ---

// getOrderProviderInstance looks up the provider instance that processed this order.
// For legacy orders without provider_instance_id, it resolves only when the
// historical instance is uniquely identifiable from the stored order fields.
func (s *PaymentService) getOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return s.resolveUniqueLegacyOrderProviderInstance(ctx, o)
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, nil
	}
	return s.entClient.PaymentProviderInstance.Get(ctx, instID)
}

// getRefundOrderProviderInstance resolves the provider instance for refund paths.
// Refunds must be pinned to an explicit historical binding, so legacy
// "best-effort" provider guessing is intentionally not allowed here.
func (s *PaymentService) getRefundOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return nil, nil
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("order %d refund provider instance id is invalid: %s", o.ID, instIDStr)
	}
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, instID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, fmt.Errorf("order %d refund provider instance %s is missing", o.ID, instIDStr)
		}
		return nil, err
	}
	return inst, nil
}

func (s *PaymentService) resolveUniqueLegacyOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	paymentType := payment.GetBasePaymentType(strings.TrimSpace(o.PaymentType))
	providerKey := strings.TrimSpace(psStringValue(o.ProviderKey))
	if providerKey != "" {
		instances, err := s.entClient.PaymentProviderInstance.Query().
			Where(paymentproviderinstance.ProviderKeyEQ(providerKey)).
			All(ctx)
		if err != nil {
			return nil, err
		}
		matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
		if len(matched) == 1 {
			return matched[0], nil
		}
		return nil, nil
	}

	if paymentType == "" {
		return nil, nil
	}

	instances, err := s.entClient.PaymentProviderInstance.Query().
		All(ctx)
	if err != nil {
		return nil, err
	}

	matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
	if len(matched) == 1 {
		return matched[0], nil
	}
	return nil, nil
}

func psFilterLegacyOrderProviderInstances(orderPaymentType string, instances []*dbent.PaymentProviderInstance) []*dbent.PaymentProviderInstance {
	if len(instances) == 0 {
		return nil
	}
	if strings.TrimSpace(orderPaymentType) == "" {
		return instances
	}
	var matched []*dbent.PaymentProviderInstance
	for _, inst := range instances {
		if psLegacyOrderMatchesInstance(orderPaymentType, inst) {
			matched = append(matched, inst)
		}
	}
	return matched
}

func psLegacyOrderMatchesInstance(orderPaymentType string, inst *dbent.PaymentProviderInstance) bool {
	if inst == nil {
		return false
	}

	baseType := payment.GetBasePaymentType(strings.TrimSpace(orderPaymentType))
	instanceProviderKey := strings.TrimSpace(inst.ProviderKey)
	if baseType == "" {
		return false
	}

	if baseType == payment.TypeStripe {
		return instanceProviderKey == payment.TypeStripe
	}
	if instanceProviderKey == payment.TypeStripe {
		return false
	}
	if instanceProviderKey == baseType {
		return true
	}
	return payment.InstanceSupportsType(inst.SupportedTypes, baseType)
}

func (s *PaymentService) RequestRefund(ctx context.Context, oid, uid int64, reason string) error {
	return s.requestRefundEnabled(ctx, oid, uid, reason)
}

func (s *PaymentService) requestRefundEnabled(ctx context.Context, oid, uid int64, reason string) error {
	o, err := s.validateRefundRequest(ctx, oid, uid)
	if err != nil {
		return err
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	requiredBalanceDeduction := calculateBalanceDeductionAmount(o.Amount, o.PayAmount, refundableOrderPayAmount(o.Amount, o.PayAmount))
	if moneyx.Currency(u.Balance).LessThan(moneyx.Currency(requiredBalanceDeduction)) {
		return infraerrors.BadRequest("BALANCE_NOT_ENOUGH", "refund amount exceeds balance")
	}
	nr := strings.TrimSpace(reason)
	now := time.Now()
	by := fmt.Sprintf("%d", uid)
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.UserIDEQ(uid), paymentorder.StatusEQ(OrderStatusCompleted), paymentorder.OrderTypeEQ(payment.OrderTypeBalance)).SetStatus(OrderStatusRefundRequested).SetRefundRequestedAt(now).SetRefundRequestReason(nr).SetRefundRequestedBy(by).SetRefundAmount(refundableOrderPayAmount(o.Amount, o.PayAmount)).Save(ctx)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if c == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	s.writeAuditLog(ctx, oid, "REFUND_REQUESTED", fmt.Sprintf("user:%d", uid), map[string]any{"amount": o.Amount, "reason": nr})
	return nil
}

func (s *PaymentService) validateRefundRequest(ctx context.Context, oid, uid int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != uid {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission")
	}
	if o.OrderType != payment.OrderTypeBalance {
		return nil, infraerrors.BadRequest("INVALID_ORDER_TYPE", "only balance orders can request refund")
	}
	if o.Status != OrderStatusCompleted {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "only completed orders can request refund")
	}
	// Check provider instance allows user refund
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil || inst == nil {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.AllowUserRefund {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "user refund is not enabled for this provider")
	}
	return o, nil
}

func (s *PaymentService) PrepareRefund(ctx context.Context, oid int64, amt float64, reason string, force, deduct bool) (*RefundPlan, *RefundResult, error) {
	return s.prepareRefundEnabled(ctx, oid, amt, reason, force, deduct)
}

func (s *PaymentService) prepareRefundEnabled(ctx context.Context, oid int64, amt float64, reason string, force, deduct bool) (*RefundPlan, *RefundResult, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	ok := []string{OrderStatusCompleted, OrderStatusPartiallyRefunded, OrderStatusRefundRequested, OrderStatusRefundFailed}
	if !psSliceContains(ok, o.Status) {
		return nil, nil, infraerrors.BadRequest("INVALID_STATUS", "order status does not allow refund")
	}
	// Check provider instance allows admin refund
	inst, instErr := s.getRefundOrderProviderInstance(ctx, o)
	if instErr != nil {
		slog.Warn("refund: provider instance lookup failed", "orderID", oid, "error", instErr)
		return nil, nil, infraerrors.InternalServer("PROVIDER_LOOKUP_FAILED", "failed to look up payment provider for this order")
	}
	if inst == nil {
		// Legacy order without provider_instance_id — block refund
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.RefundEnabled {
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not enabled for this provider")
	}
	if math.IsNaN(amt) || math.IsInf(amt, 0) {
		return nil, nil, infraerrors.BadRequest("INVALID_AMOUNT", "invalid refund amount")
	}
	refundablePayAmount := refundableOrderPayAmount(o.Amount, o.PayAmount)
	existingRefundAmount := moneyx.NonNegative(moneyx.Currency(o.RefundAmount)).InexactFloat64()
	if o.Status == OrderStatusRefundRequested && o.RefundRequestedAt != nil && o.RefundAt == nil {
		// REFUND_REQUESTED stores the user's requested amount in refund_amount so
		// the admin UI can prefill it, but it is not an executed refund yet.
		existingRefundAmount = 0
	}
	remainingRefundAmount := moneyx.Currency(refundablePayAmount).Sub(moneyx.Currency(existingRefundAmount)).InexactFloat64()
	if remainingRefundAmount <= 0 {
		return nil, nil, infraerrors.BadRequest("REFUND_AMOUNT_EXCEEDED", "refund amount exceeds paid amount")
	}
	if amt <= 0 {
		amt = remainingRefundAmount
	}
	if moneyx.Currency(amt).GreaterThan(moneyx.Currency(remainingRefundAmount)) {
		return nil, nil, infraerrors.BadRequest("REFUND_AMOUNT_EXCEEDED", "refund amount exceeds paid amount")
	}
	orderCurrency := PaymentOrderCurrency(o)
	ga := calculateGatewayRefundAmount(o.Amount, o.PayAmount, amt, orderCurrency)
	rr := strings.TrimSpace(reason)
	if rr == "" && o.RefundRequestReason != nil {
		rr = *o.RefundRequestReason
	}
	if rr == "" {
		rr = fmt.Sprintf("refund order:%d", o.ID)
	}
	totalRefundAmount := moneyx.Currency(existingRefundAmount).Add(moneyx.Currency(amt)).InexactFloat64()
	p := &RefundPlan{OrderID: oid, Order: o, RefundAmount: amt, TotalRefundAmount: totalRefundAmount, GatewayAmount: ga, Reason: rr, Force: force, DeductBalance: deduct, DeductionType: payment.DeductionTypeNone}
	if deduct {
		if er := s.prepDeduct(ctx, o, p, force); er != nil {
			return nil, er, nil
		}
	}
	return p, nil, nil
}

func (s *PaymentService) prepDeduct(ctx context.Context, o *dbent.PaymentOrder, p *RefundPlan, force bool) *RefundResult {
	if o.OrderType == payment.OrderTypeSubscription {
		p.DeductionType = payment.DeductionTypeSubscription
		if o.SubscriptionGroupID != nil && o.SubscriptionDays != nil {
			p.SubDaysToDeduct = *o.SubscriptionDays
			sub, err := s.subscriptionSvc.GetActiveSubscription(ctx, o.UserID, *o.SubscriptionGroupID)
			if err == nil && sub != nil {
				p.SubscriptionID = sub.ID
			} else if !force {
				return &RefundResult{Success: false, Warning: "cannot find active subscription for deduction, use force", RequireForce: true}
			}
		}
		return nil
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		if !force {
			return &RefundResult{Success: false, Warning: "cannot fetch user balance, use force", RequireForce: true}
		}
		return nil
	}
	p.DeductionType = payment.DeductionTypeBalance
	balanceDeductionAmount := calculateBalanceDeductionAmount(o.Amount, o.PayAmount, p.RefundAmount)
	p.BalanceToDeduct = moneyx.Min(
		moneyx.Currency(balanceDeductionAmount),
		moneyx.Currency(u.Balance),
	).InexactFloat64()
	return nil
}

func (s *PaymentService) ExecuteRefund(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusIn(OrderStatusCompleted, OrderStatusPartiallyRefunded, OrderStatusRefundRequested, OrderStatusRefundFailed)).SetStatus(OrderStatusRefunding).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil, infraerrors.Conflict("CONFLICT", "order status changed")
	}
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		// Skip balance deduction on retry if previous attempt already deducted
		// but failed to roll back (REFUND_ROLLBACK_FAILED in audit log).
		if !s.hasAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED") {
			if err := s.userRepo.DeductBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
				s.restoreStatus(ctx, p)
				return nil, fmt.Errorf("deduction: %w", err)
			}
		} else {
			slog.Warn("skipping balance deduction on retry (previous rollback failed)", "orderID", p.OrderID)
			p.BalanceToDeduct = 0
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if !s.hasAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED") {
			_, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, -p.SubDaysToDeduct)
			if err != nil {
				if errors.Is(err, ErrAdjustWouldExpire) {
					// Deduction would expire the subscription — revoke it entirely
					slog.Info("subscription deduction would expire, revoking", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct)
					if revokeErr := s.subscriptionSvc.RevokeSubscription(ctx, p.SubscriptionID); revokeErr != nil {
						s.restoreStatus(ctx, p)
						return nil, fmt.Errorf("revoke subscription: %w", revokeErr)
					}
				} else {
					// Other errors (DB failure, not found) — abort refund
					s.restoreStatus(ctx, p)
					return nil, fmt.Errorf("deduct subscription days: %w", err)
				}
			}
		} else {
			slog.Warn("skipping subscription deduction on retry (previous rollback failed)", "orderID", p.OrderID)
			p.SubDaysToDeduct = 0
		}
	}
	if err := s.gwRefund(ctx, p); err != nil {
		return s.handleGwFail(ctx, p, err)
	}
	return s.markRefundOk(ctx, p)
}

func (s *PaymentService) gwRefund(ctx context.Context, p *RefundPlan) error {
	if p.Order.PaymentTradeNo == "" {
		s.writeAuditLog(ctx, p.Order.ID, refundAuditAction("REFUND_NO_TRADE_NO", p.TotalRefundAmount), "admin", map[string]any{"detail": "skipped"})
		return nil
	}

	// Use the exact provider instance that created this order, not a random one
	// from the registry. Each instance has its own merchant credentials.
	prov, err := s.getRefundProvider(ctx, p.Order)
	if err != nil {
		return fmt.Errorf("get refund provider: %w", err)
	}
	if err := validateProviderSnapshotMetadata(p.Order, prov.ProviderKey(), providerMerchantIdentityMetadata(prov)); err != nil {
		s.writeAuditLog(ctx, p.Order.ID, refundAuditAction("REFUND_PROVIDER_METADATA_MISMATCH", p.TotalRefundAmount), "admin", map[string]any{
			"detail": err.Error(),
		})
		return err
	}
	resp, err := prov.Refund(ctx, payment.RefundRequest{
		TradeNo: p.Order.PaymentTradeNo,
		OrderID: p.Order.OutTradeNo,
		Amount:  formatGatewayRefundAmount(p.GatewayAmount, p.Order),
		Reason:  p.Reason,
	})
	if err != nil {
		return err
	}
	return validateRefundProviderResponse(resp)
}

func formatGatewayRefundAmount(amount float64, order *dbent.PaymentOrder) string {
	return payment.FormatAmountForCurrency(amount, PaymentOrderCurrency(order))
}

func validateRefundProviderResponse(resp *payment.RefundResponse) error {
	if resp == nil {
		return fmt.Errorf("payment refund response missing")
	}
	status := strings.TrimSpace(resp.Status)
	switch status {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded, payment.ProviderStatusPending:
		return nil
	case payment.ProviderStatusFailed:
		return fmt.Errorf("payment refund failed: status %s", status)
	default:
		return fmt.Errorf("payment refund returned unknown status: %s", status)
	}
}

// getRefundProvider creates a provider using the order's original instance config.
// Delegates to getOrderProvider which handles instance lookup and fallback.
func (s *PaymentService) getRefundProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, fmt.Errorf("refund provider instance is unavailable for order %d", o.ID)
	}
	return s.createProviderFromInstance(ctx, inst)
}

func (s *PaymentService) handleGwFail(ctx context.Context, p *RefundPlan, gErr error) (*RefundResult, error) {
	if s.RollbackRefund(ctx, p, gErr) {
		s.restoreStatus(ctx, p)
		s.writeAuditLog(ctx, p.OrderID, refundAuditAction("REFUND_GATEWAY_FAILED", p.TotalRefundAmount), "admin", map[string]any{"detail": psErrMsg(gErr)})
		return &RefundResult{Success: false, Warning: "gateway failed: " + psErrMsg(gErr) + ", rolled back"}, nil
	}
	now := time.Now()
	_, _ = s.entClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(OrderStatusRefundFailed).SetFailedAt(now).SetFailedReason(psErrMsg(gErr)).Save(ctx)
	s.writeAuditLog(ctx, p.OrderID, refundAuditAction("REFUND_FAILED", p.TotalRefundAmount), "admin", map[string]any{"detail": psErrMsg(gErr)})
	return nil, infraerrors.InternalServer("REFUND_FAILED", psErrMsg(gErr))
}

// refundAuditAction returns a stable per-refund event key while staying within
// the historical action column width. Repeated partial refunds on the same
// order must keep distinct audit rows instead of colliding on (order_id, action).
func refundAuditAction(base string, totalRefundAmount float64) string {
	suffix := strings.ReplaceAll(moneyx.Currency(totalRefundAmount).StringFixed(2), ".", "_")
	action := base + "@" + suffix
	if len(action) <= 50 {
		return action
	}

	sum := sha1.Sum([]byte(action))
	hashSuffix := hex.EncodeToString(sum[:4])
	maxBaseLen := 50 - 1 - len(hashSuffix)
	if maxBaseLen < 1 {
		return action[:50]
	}
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
	}
	return base + "@" + hashSuffix
}

func (s *PaymentService) markRefundOk(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	fs := OrderStatusRefunded
	if moneyx.Currency(p.TotalRefundAmount).LessThan(moneyx.Currency(refundableOrderPayAmount(p.Order.Amount, p.Order.PayAmount))) {
		fs = OrderStatusPartiallyRefunded
	}
	now := time.Now()
	_, err := s.entClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(fs).SetRefundAmount(p.TotalRefundAmount).SetRefundReason(p.Reason).SetRefundAt(now).SetForceRefund(p.Force).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mark refund: %w", err)
	}
	reversed, reversalErr := s.applyCustomReferralRefundForOrder(ctx, p)
	auditDetail := map[string]any{"refundAmount": p.RefundAmount, "totalRefundAmount": p.TotalRefundAmount, "reason": p.Reason, "balanceDeducted": p.BalanceToDeduct, "force": p.Force}
	if reversed > 0 {
		auditDetail["referralReversed"] = reversed
	}
	if reversalErr != nil {
		auditDetail["referralReversalError"] = reversalErr.Error()
		s.writeAuditLog(ctx, p.OrderID, refundAuditAction("REFUND_SUCCESS", p.TotalRefundAmount), "admin", auditDetail)
		return &RefundResult{
			Success:         true,
			Warning:         "退款已成功，但返佣冲正失败：" + reversalErr.Error() + "。请在订单列表点击补冲销重试。",
			BalanceDeducted: p.BalanceToDeduct,
			SubDaysDeducted: p.SubDaysToDeduct,
		}, nil
	}
	s.writeAuditLog(ctx, p.OrderID, refundAuditAction("REFUND_SUCCESS", p.TotalRefundAmount), "admin", auditDetail)
	return &RefundResult{Success: true, BalanceDeducted: p.BalanceToDeduct, SubDaysDeducted: p.SubDaysToDeduct}, nil
}

func (s *PaymentService) applyCustomReferralRefundForOrder(ctx context.Context, p *RefundPlan) (float64, error) {
	if s == nil || s.customReferralService == nil || p == nil || p.Order == nil || p.RefundAmount <= 0 {
		return 0, nil
	}
	refundAmount := customReferralRefundBaseAmount(p.Order, p.TotalRefundAmount, p.GatewayAmount)
	if refundAmount <= 0 {
		return 0, nil
	}
	reversed, err := s.customReferralService.ReverseCommissionForRefund(ctx, CustomReferralRefundInput{
		OrderID:      p.OrderID,
		RefundAmount: refundAmount,
		Reason:       strings.TrimSpace(p.Reason),
		RefundedAt:   time.Now(),
	})
	if err != nil {
		s.writeAuditLog(ctx, p.OrderID, refundAuditAction("CUSTOM_REFERRAL_REFUND_FAILED", p.TotalRefundAmount), "system", map[string]any{
			"error": err.Error(),
		})
		return 0, err
	}
	if reversed <= 0 {
		return 0, nil
	}
	s.writeAuditLog(ctx, p.OrderID, refundAuditAction("CUSTOM_REFERRAL_COMMISSION_REVERSED", p.TotalRefundAmount), "system", map[string]any{
		"refundAmount":           refundAmount,
		"incrementalOrderRefund": p.RefundAmount,
		"totalOrderRefund":       p.TotalRefundAmount,
		"gatewayRefund":          p.GatewayAmount,
		"reversedAmount":         reversed,
	})
	return reversed, nil
}

func (s *PaymentService) RetryCustomReferralRefund(ctx context.Context, oid int64) (float64, error) {
	if s == nil || s.customReferralService == nil {
		return 0, infraerrors.BadRequest("CUSTOM_REFERRAL_UNAVAILABLE", "custom referral service is unavailable")
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return 0, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status != OrderStatusRefunded && o.Status != OrderStatusPartiallyRefunded {
		return 0, infraerrors.BadRequest("INVALID_STATUS", "only refunded orders can retry referral reversal")
	}
	if o.RefundAmount <= 0 {
		return 0, infraerrors.BadRequest("INVALID_REFUND_AMOUNT", "order has no refund amount")
	}
	reason := strings.TrimSpace(psStringValue(o.RefundReason))
	if reason == "" {
		reason = fmt.Sprintf("refund order:%d", o.ID)
	}
	refundAmount := customReferralRefundBaseAmount(o, o.RefundAmount, 0)
	if refundAmount <= 0 {
		return 0, infraerrors.BadRequest("INVALID_REFUND_AMOUNT", "order has no refundable paid amount")
	}
	reversed, err := s.customReferralService.ReverseCommissionForRefund(ctx, CustomReferralRefundInput{
		OrderID:      o.ID,
		RefundAmount: refundAmount,
		Reason:       reason,
		RefundedAt:   valueOrNow(o.RefundAt),
	})
	if err != nil {
		s.writeAuditLog(ctx, o.ID, "CUSTOM_REFERRAL_REFUND_RETRY_FAILED", "admin", map[string]any{"error": err.Error()})
		return 0, err
	}
	if reversed > 0 {
		s.writeAuditLog(ctx, o.ID, "CUSTOM_REFERRAL_REFUND_RETRIED", "admin", map[string]any{
			"refundAmount":   refundAmount,
			"orderRefund":    o.RefundAmount,
			"reversedAmount": reversed,
		})
	}
	return reversed, nil
}

func customReferralRefundBaseAmount(o *dbent.PaymentOrder, orderRefundAmount, gatewayRefundAmount float64) float64 {
	// Order/refund amounts arrive from legacy Ent/API float64 fields; proportional
	// refund math is done with moneyx/decimal before returning to the boundary.
	if o == nil {
		return 0
	}
	baseAmount := customReferralCommissionBaseAmount(o)
	refundablePayAmount := refundableOrderPayAmount(o.Amount, o.PayAmount)
	if baseAmount <= 0 || orderRefundAmount <= 0 || refundablePayAmount <= 0 {
		return 0
	}
	if moneyx.Currency(orderRefundAmount).GreaterThanOrEqual(moneyx.Currency(refundablePayAmount)) {
		return baseAmount
	}
	return moneyx.Proportion(baseAmount, orderRefundAmount, refundablePayAmount, moneyx.ScaleCurrency).InexactFloat64()
}

func (s *PaymentService) RollbackRefund(ctx context.Context, p *RefundPlan, gErr error) bool {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		if err := s.userRepo.UpdateBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
			slog.Error("[CRITICAL] rollback failed", "orderID", p.OrderID, "amount", p.BalanceToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "balanceDeducted": p.BalanceToDeduct})
			return false
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if _, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, p.SubDaysToDeduct); err != nil {
			slog.Error("[CRITICAL] subscription rollback failed", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "subDaysDeducted": p.SubDaysToDeduct})
			return false
		}
	}
	return true
}

func (s *PaymentService) restoreStatus(ctx context.Context, p *RefundPlan) {
	rs := OrderStatusCompleted
	switch p.Order.Status {
	case OrderStatusRefundRequested:
		rs = OrderStatusRefundRequested
	case OrderStatusPartiallyRefunded:
		rs = OrderStatusPartiallyRefunded
	}
	_, _ = s.entClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(rs).Save(ctx)
}
