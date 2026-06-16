package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentproviderinstance"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// RefundPlan captures the ExecuteRefund inputs derived from PrepareRefund.
//
// Currency-typed fields are decimals so the gateway-amount derivation
// and balance deduction stay exact; the previous float64 form drifted
// by sub-cent on certain (orderAmount, payAmount, refundAmount) triples.
type RefundPlan struct {
	OrderID         int64
	Order           *pluginent.PaymentOrder
	RefundAmount    decimal.Decimal
	GatewayAmount   decimal.Decimal
	Reason          string
	Force           bool
	DeductBalance   bool
	DeductionType   string
	BalanceToDeduct decimal.Decimal
	SubDaysToDeduct int
	SubscriptionID  int64
}

// RefundResult is the outcome of ExecuteRefund.
type RefundResult struct {
	Success         bool            `json:"success"`
	Warning         string          `json:"warning,omitempty"`
	RequireForce    bool            `json:"require_force,omitempty"`
	BalanceDeducted decimal.Decimal `json:"balance_deducted,omitempty"`
	SubDaysDeducted int             `json:"subscription_days_deducted,omitempty"`
}

// RequestRefund is the user-initiated refund request entry point. Only
// COMPLETED balance orders qualify; the user's balance must cover the
// full refund amount (since we deduct from balance before refunding to
// the gateway).
func (s *PaymentService) RequestRefund(ctx context.Context, oid, uid int64, reason string) error {
	if s == nil || s.entClient == nil {
		return errPaymentServiceUnavailable
	}
	o, err := s.validateRefundRequest(ctx, oid, uid)
	if err != nil {
		return err
	}
	if err := s.checkRefundBalance(ctx, o); err != nil {
		return err
	}
	now := time.Now()
	by := fmt.Sprintf("%d", uid)
	nr := strings.TrimSpace(reason)
	c, err := s.entClient.PaymentOrder.Update().
		Where(
			paymentorder.IDEQ(int(oid)),
			paymentorder.UserIDEQ(uid),
			paymentorder.StatusEQ(OrderStatusCompleted),
			paymentorder.OrderTypeEQ(payment.OrderTypeBalance),
		).
		SetStatus(OrderStatusRefundRequested).
		SetRefundRequestedAt(now).
		SetRefundRequestReason(nr).
		SetRefundRequestedBy(by).
		SetRefundAmount(o.Amount).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if c == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	s.writeAuditLog(ctx, oid, "REFUND_REQUESTED", fmt.Sprintf("user:%d", uid), map[string]any{
		"amount": o.Amount,
		"reason": nr,
	})
	return nil
}

func (s *PaymentService) checkRefundBalance(ctx context.Context, o *pluginent.PaymentOrder) error {
	if s.host == nil {
		return infraerrors.ServiceUnavailable("HOST_USER_LOOKUP_UNAVAILABLE", "user lookup is not available")
	}
	user, err := s.host.GetUserByID(ctx, o.UserID)
	if err != nil {
		if errors.Is(err, pluginsdk.ErrHostUserLookupUnavailable) {
			return infraerrors.ServiceUnavailable("HOST_USER_LOOKUP_UNAVAILABLE", "user lookup is not available")
		}
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil || !user.Found {
		return infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	}
	if user.Balance.LessThan(o.Amount) {
		return infraerrors.BadRequest("BALANCE_NOT_ENOUGH", "refund amount exceeds balance")
	}
	return nil
}

func (s *PaymentService) validateRefundRequest(ctx context.Context, oid, uid int64) (*pluginent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, int(oid))
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
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil || inst == nil {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.AllowUserRefund {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "user refund is not enabled for this provider")
	}
	return o, nil
}

// PrepareRefund builds an executable RefundPlan. The first return value
// is the plan when validation passed; the second is a short-circuit
// RefundResult when the caller still needs to confirm something (e.g.
// an admin must pass force=true to proceed despite a missing
// subscription record).
func (s *PaymentService) PrepareRefund(ctx context.Context, oid int64, amt decimal.Decimal, reason string, force, deduct bool) (*RefundPlan, *RefundResult, error) {
	if s == nil || s.entClient == nil {
		return nil, nil, errPaymentServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(oid))
	if err != nil {
		return nil, nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if err := validatePrepareRefundStatus(o); err != nil {
		return nil, nil, err
	}
	if err := s.requireRefundProviderEnabled(ctx, o); err != nil {
		return nil, nil, err
	}
	amt, err = normalizeRefundAmount(o, amt)
	if err != nil {
		return nil, nil, err
	}
	plan := &RefundPlan{
		OrderID:       oid,
		Order:         o,
		RefundAmount:  amt,
		GatewayAmount: calculateGatewayRefundAmount(o.Amount, o.PayAmount, amt),
		Reason:        deriveRefundReason(o, reason),
		Force:         force,
		DeductBalance: deduct,
		DeductionType: payment.DeductionTypeNone,
	}
	if deduct {
		if er := s.prepDeduct(ctx, o, plan, force); er != nil {
			return nil, er, nil
		}
	}
	return plan, nil, nil
}

func validatePrepareRefundStatus(o *pluginent.PaymentOrder) error {
	allowed := []string{OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed}
	if !psSliceContains(allowed, o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "order status does not allow refund")
	}
	return nil
}

func (s *PaymentService) requireRefundProviderEnabled(ctx context.Context, o *pluginent.PaymentOrder) error {
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("refund: provider instance lookup failed", "order_id", o.ID, "error", err)
		}
		return infraerrors.InternalServer("PROVIDER_LOOKUP_FAILED", "failed to look up payment provider for this order")
	}
	if inst == nil {
		return infraerrors.Forbidden("REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.RefundEnabled {
		return infraerrors.Forbidden("REFUND_DISABLED", "refund is not enabled for this provider")
	}
	return nil
}

func normalizeRefundAmount(o *pluginent.PaymentOrder, amt decimal.Decimal) (decimal.Decimal, error) {
	if amt.Sign() <= 0 {
		amt = o.Amount
	}
	// Decimal-precision comparison: refund amount must not exceed the
	// order amount even by sub-cent fractions. A 0.01 float tolerance
	// previously let an attacker request a refund up to 1 cent over the
	// recharge silently. Comparing in fen keeps the rule explicit.
	if yuanToFen(amt) > yuanToFen(o.Amount) {
		return decimal.Zero, infraerrors.BadRequest("REFUND_AMOUNT_EXCEEDED", "refund amount exceeds recharge")
	}
	return amt, nil
}

func deriveRefundReason(o *pluginent.PaymentOrder, supplied string) string {
	r := strings.TrimSpace(supplied)
	if r != "" {
		return r
	}
	if o.RefundRequestReason != nil {
		if v := strings.TrimSpace(*o.RefundRequestReason); v != "" {
			return v
		}
	}
	return fmt.Sprintf("refund order:%d", o.ID)
}

func (s *PaymentService) prepDeduct(ctx context.Context, o *pluginent.PaymentOrder, p *RefundPlan, force bool) *RefundResult {
	if o.OrderType == payment.OrderTypeSubscription {
		return s.prepSubscriptionDeduct(o, p, force)
	}
	return s.prepBalanceDeduct(ctx, o, p, force)
}

func (s *PaymentService) prepSubscriptionDeduct(o *pluginent.PaymentOrder, p *RefundPlan, force bool) *RefundResult {
	p.DeductionType = payment.DeductionTypeSubscription
	if o.SubscriptionGroupID == nil || o.SubscriptionDays == nil {
		return nil
	}
	p.SubDaysToDeduct = *o.SubscriptionDays
	if force {
		return nil
	}
	// We can't query the host for active-subscription rows from the
	// plugin, so absent --force we conservatively flag the require_force
	// path. A future SDK addition could expose ActiveSubscription lookup.
	return &RefundResult{
		Success:      false,
		Warning:      "subscription deduction requires force in plugin mode",
		RequireForce: true,
	}
}

func (s *PaymentService) prepBalanceDeduct(ctx context.Context, o *pluginent.PaymentOrder, p *RefundPlan, force bool) *RefundResult {
	if s.host == nil {
		if force {
			return nil
		}
		return &RefundResult{Success: false, Warning: "cannot fetch user balance, use force", RequireForce: true}
	}
	user, err := s.host.GetUserByID(ctx, o.UserID)
	if err != nil || user == nil || !user.Found {
		if !force {
			return &RefundResult{Success: false, Warning: "cannot fetch user balance, use force", RequireForce: true}
		}
		return nil
	}
	p.DeductionType = payment.DeductionTypeBalance
	// min(refund, balance) at decimal precision so we never over-debit
	// the user even by a sub-cent amount.
	p.BalanceToDeduct = p.RefundAmount
	if user.Balance.LessThan(p.RefundAmount) {
		p.BalanceToDeduct = user.Balance
	}
	return nil
}

// getOrderProviderInstance resolves the provider instance attached to an order.
// Order resolution prefers the snapshot column, then provider_instance_id,
// and only falls back to the registry-by-key lookup for legacy orders.
func (s *PaymentService) getOrderProviderInstance(ctx context.Context, o *pluginent.PaymentOrder) (*pluginent.PaymentProviderInstance, error) {
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
	return s.entClient.PaymentProviderInstance.Get(ctx, int(instID))
}

// getRefundOrderProviderInstance is the stricter variant used by refund paths.
// Refund-time resolution requires an explicit historical pin.
func (s *PaymentService) getRefundOrderProviderInstance(ctx context.Context, o *pluginent.PaymentOrder) (*pluginent.PaymentProviderInstance, error) {
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
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, int(instID))
	if err != nil {
		if pluginent.IsNotFound(err) {
			return nil, fmt.Errorf("order %d refund provider instance %s is missing", o.ID, instIDStr)
		}
		return nil, err
	}
	return inst, nil
}

// resolveUniqueLegacyOrderProviderInstance returns the unique enabled
// instance matching the order's stored provider_key, when one exists.
func (s *PaymentService) resolveUniqueLegacyOrderProviderInstance(ctx context.Context, o *pluginent.PaymentOrder) (*pluginent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}
	providerKey := strings.TrimSpace(psStringValue(o.ProviderKey))
	if providerKey == "" {
		providerKey = payment.GetBasePaymentType(strings.TrimSpace(o.PaymentType))
	}
	if providerKey == "" {
		return nil, nil
	}
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKeyEQ(providerKey),
			paymentproviderinstance.EnabledEQ(true),
		).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query legacy provider instances: %w", err)
	}
	if len(instances) == 1 {
		return instances[0], nil
	}
	return nil, nil
}

// LogAdminForceRefundNoDeduct records the (force=true, deduct_balance=false)
// admin refund path in the audit log AND emits a structured warn-level
// slog entry. The combination is the silent free-money path: a refund is
// pushed to the upstream gateway without debiting the user's balance, so
// the user keeps both the recharge credit and the gateway refund. The
// admin handler requires ConfirmNoBalanceDeduction=true to reach this
// path; this method is the operational paper trail.
func (s *PaymentService) LogAdminForceRefundNoDeduct(ctx context.Context, orderID, adminID int64, amount decimal.Decimal, reason string) {
	if s == nil {
		return
	}
	if s.logger != nil {
		s.logger.Warn("admin refund: force without balance deduction (silent free-money path acknowledged)",
			"order_id", orderID,
			"admin_id", adminID,
			"amount", amount.String(),
			"reason", reason,
		)
	}
	s.writeAuditLog(ctx, orderID, "REFUND_ADMIN_FORCE_NO_DEDUCT", fmt.Sprintf("admin:%d", adminID), map[string]any{
		"amount": amount,
		"reason": reason,
	})
}
