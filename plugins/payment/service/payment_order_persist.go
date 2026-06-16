package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment/provider"
)

// persistOrder runs the per-user limit checks, allocates an out_trade_no,
// and writes the new order row inside a single transaction. Returns the
// persisted order or a 4xx ApplicationError when a limit is hit.
func (s *PaymentService) persistOrder(
	ctx context.Context,
	req CreateOrderRequest,
	user *pluginsdk.HostUserInfo,
	plan *SubscriptionPlan,
	cfg *PaymentConfig,
	pricing orderPricing,
	sel *payment.InstanceSelection,
) (*pluginent.PaymentOrder, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.checkPendingLimit(ctx, tx, req.UserID, cfg.MaxPendingOrders); err != nil {
		return nil, err
	}
	if err := s.checkDailyLimit(ctx, tx, req.UserID, pricing.LimitAmount, cfg.DailyLimit); err != nil {
		return nil, err
	}
	exp := time.Now().Add(orderTimeoutDuration(cfg))
	outTradeNo, err := s.allocateOutTradeNo(ctx, tx)
	if err != nil {
		return nil, err
	}
	order, err := saveNewOrder(ctx, tx, newOrderRowInput{
		Req:          req,
		User:         user,
		Plan:         plan,
		Pricing:      pricing,
		Selection:    sel,
		OutTradeNo:   outTradeNo,
		ExpiresAt:    exp,
		ProviderSnap: buildPaymentOrderProviderSnapshot(sel, req),
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit order transaction: %w", err)
	}
	return order, nil
}

func orderTimeoutDuration(cfg *PaymentConfig) time.Duration {
	tm := cfg.OrderTimeoutMin
	if tm <= 0 {
		tm = defaultOrderTimeoutMin
	}
	return time.Duration(tm) * time.Minute
}

// newOrderRowInput bundles the eight inputs to saveNewOrder so the call
// site doesn't drift past the linter's parameter-count threshold.
type newOrderRowInput struct {
	Req          CreateOrderRequest
	User         *pluginsdk.HostUserInfo
	Plan         *SubscriptionPlan
	Pricing      orderPricing
	Selection    *payment.InstanceSelection
	OutTradeNo   string
	ExpiresAt    time.Time
	ProviderSnap map[string]any
}

func saveNewOrder(ctx context.Context, tx *pluginent.Tx, in newOrderRowInput) (*pluginent.PaymentOrder, error) {
	b := tx.PaymentOrder.Create().
		SetUserID(in.Req.UserID).
		SetUserEmail(in.User.Email).
		SetUserName(in.User.Username).
		SetAmount(in.Pricing.OrderAmount).
		SetPayAmount(in.Pricing.PayAmount).
		SetFeeRate(in.Pricing.FeeRate).
		SetRechargeCode("").
		SetOutTradeNo(in.OutTradeNo).
		SetPaymentType(in.Req.PaymentType).
		SetPaymentTradeNo("").
		SetOrderType(in.Req.OrderType).
		SetStatus(OrderStatusPending).
		SetExpiresAt(in.ExpiresAt).
		SetClientIP(in.Req.ClientIP).
		SetSrcHost(in.Req.SrcHost)
	if in.Req.SrcURL != "" {
		b.SetSrcURL(in.Req.SrcURL)
	}
	if sel := in.Selection; sel != nil {
		if id := strings.TrimSpace(sel.InstanceID); id != "" {
			b.SetProviderInstanceID(id)
		}
		if k := strings.TrimSpace(sel.ProviderKey); k != "" {
			b.SetProviderKey(k)
		}
	}
	if in.ProviderSnap != nil {
		b.SetProviderSnapshot(in.ProviderSnap)
	}
	if in.Plan != nil {
		b.SetPlanID(int64(in.Plan.ID)).
			SetSubscriptionGroupID(in.Plan.GroupID).
			SetSubscriptionDays(psComputeValidityDays(in.Plan.ValidityDays, in.Plan.ValidityUnit))
	}
	order, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	code := fmt.Sprintf("PAY-%d-%d", order.ID, time.Now().UnixNano()%100000)
	order, err = tx.PaymentOrder.UpdateOneID(order.ID).SetRechargeCode(code).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("set recharge code: %w", err)
	}
	return order, nil
}

func (s *PaymentService) allocateOutTradeNo(ctx context.Context, tx *pluginent.Tx) (string, error) {
	for attempt := 0; attempt < outTradeNoMaxAttempts; attempt++ {
		candidate := generateOutTradeNo()
		exists, err := tx.PaymentOrder.Query().Where(paymentorder.OutTradeNo(candidate)).Exist(ctx)
		if err != nil {
			return "", fmt.Errorf("check out_trade_no uniqueness: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("generate unique out_trade_no: exhausted %d attempts", outTradeNoMaxAttempts)
}

func (s *PaymentService) checkPendingLimit(ctx context.Context, tx *pluginent.Tx, userID int64, maxPending int) error {
	if maxPending <= 0 {
		maxPending = defaultMaxPendingOrders
	}
	c, err := tx.PaymentOrder.Query().
		Where(paymentorder.UserIDEQ(userID), paymentorder.StatusEQ(OrderStatusPending)).
		Count(ctx)
	if err != nil {
		return fmt.Errorf("count pending orders: %w", err)
	}
	if c >= maxPending {
		return infraerrors.TooManyRequests("TOO_MANY_PENDING", "too_many_pending").
			WithMetadata(map[string]string{"max": strconv.Itoa(maxPending)})
	}
	return nil
}

func (s *PaymentService) checkDailyLimit(ctx context.Context, tx *pluginent.Tx, userID int64, amount, limit decimal.Decimal) error {
	if !limit.IsPositive() {
		return nil
	}
	since := psStartOfDayUTC(time.Now())
	orders, err := tx.PaymentOrder.Query().Where(
		paymentorder.UserIDEQ(userID),
		paymentorder.StatusIn(OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted),
		paymentorder.PaidAtGTE(since),
	).All(ctx)
	if err != nil {
		return fmt.Errorf("query daily usage: %w", err)
	}
	used := sumDailyUsage(orders)
	if used.Add(amount).GreaterThan(limit) {
		remaining := limit.Sub(used)
		if remaining.IsNegative() {
			remaining = decimal.Zero
		}
		return infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", "daily_limit_exceeded").
			WithMetadata(map[string]string{"remaining": remaining.StringFixed(2)})
	}
	return nil
}

func sumDailyUsage(orders []*pluginent.PaymentOrder) decimal.Decimal {
	used := decimal.Zero
	for _, o := range orders {
		if o.OrderType == payment.OrderTypeBalance {
			used = used.Add(o.PayAmount)
			continue
		}
		used = used.Add(o.Amount)
	}
	return used
}

// buildPaymentOrderProviderSnapshot captures the provider/instance details
// recorded on an order at creation time for later webhook validation.
func buildPaymentOrderProviderSnapshot(sel *payment.InstanceSelection, req CreateOrderRequest) map[string]any {
	if sel == nil {
		return nil
	}
	snapshot := map[string]any{"schema_version": 2}
	if id := strings.TrimSpace(sel.InstanceID); id != "" {
		snapshot["provider_instance_id"] = id
	}
	if k := strings.TrimSpace(sel.ProviderKey); k != "" {
		snapshot["provider_key"] = k
	}
	if mode := strings.TrimSpace(sel.PaymentMode); mode != "" {
		snapshot["payment_mode"] = mode
	}
	addProviderSnapshotMerchantFields(snapshot, sel, req)
	if len(snapshot) == 1 {
		return nil
	}
	return snapshot
}

func addProviderSnapshotMerchantFields(snapshot map[string]any, sel *payment.InstanceSelection, req CreateOrderRequest) {
	switch strings.TrimSpace(sel.ProviderKey) {
	case payment.TypeWxpay:
		if appID := paymentOrderSnapshotWxpayAppID(sel, req); appID != "" {
			snapshot["merchant_app_id"] = appID
		}
		if mch := strings.TrimSpace(sel.Config["mchId"]); mch != "" {
			snapshot["merchant_id"] = mch
		}
		snapshot["currency"] = "CNY"
	case payment.TypeAlipay:
		if appID := strings.TrimSpace(sel.Config["appId"]); appID != "" {
			snapshot["merchant_app_id"] = appID
		}
	case payment.TypeEasyPay:
		if pid := strings.TrimSpace(sel.Config["pid"]); pid != "" {
			snapshot["merchant_id"] = pid
		}
	}
}

func paymentOrderSnapshotWxpayAppID(sel *payment.InstanceSelection, req CreateOrderRequest) string {
	if sel == nil || strings.TrimSpace(sel.ProviderKey) != payment.TypeWxpay {
		return ""
	}
	if strings.TrimSpace(req.OpenID) != "" {
		return strings.TrimSpace(provider.ResolveWxpayJSAPIAppID(sel.Config))
	}
	return strings.TrimSpace(sel.Config["appId"])
}
