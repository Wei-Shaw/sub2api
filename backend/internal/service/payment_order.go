package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/shopspring/decimal"
)

// --- Order Creation ---

func (s *PaymentService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
	}
	if normalized := NormalizeVisibleMethod(req.PaymentType); normalized != "" {
		req.PaymentType = normalized
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}
	plan, err := s.validateOrderInput(ctx, req, cfg)
	if err != nil {
		return nil, err
	}
	if err := s.checkCancelRateLimit(ctx, req.UserID, cfg); err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user.Status != payment.EntityStatusActive {
		return nil, infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
	}
	if s.notificationEmailService != nil {
		s.notificationEmailService.RememberRecipientLocale(ctx, req.UserID, user.Email, req.Locale)
	}
	orderAmount := req.Amount
	limitAmount := req.Amount
	if plan != nil {
		orderAmount = plan.Price
		limitAmount = plan.Price
	} else if req.OrderType == payment.OrderTypeBalance {
		orderAmount = calculateCreditedBalance(req.Amount, cfg.BalanceRechargeMultiplier)
	}
	feeRate := cfg.RechargeFeeRate
	methodCurrency := payment.DefaultPaymentCurrency
	if s.configService != nil {
		methodCurrency, err = s.configService.ValidateMethodCurrencyConsistency(ctx, req.PaymentType)
		if err != nil {
			return nil, err
		}
	}
	payAmountStr, payAmount, err := calculateCreateOrderPayAmountForOrderType(limitAmount, feeRate, methodCurrency, req.OrderType, cfg.SubscriptionUSDToCNYRate)
	if err != nil {
		return nil, err
	}
	sel, err := s.selectCreateOrderInstance(ctx, req, cfg, payAmount)
	if err != nil {
		return nil, err
	}
	if err := s.validateSelectedCreateOrderInstance(ctx, req, sel); err != nil {
		return nil, err
	}
	selectedCurrency := payment.DefaultPaymentCurrency
	if sel != nil {
		selectedCurrency = paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	}
	if selectedCurrency != methodCurrency {
		payAmountStr, payAmount, err = calculateCreateOrderPayAmountForOrderType(limitAmount, feeRate, selectedCurrency, req.OrderType, cfg.SubscriptionUSDToCNYRate)
		if err != nil {
			return nil, err
		}
	}
	if err := validateSelectedCreateOrderAmountCurrency(payAmountStr, sel); err != nil {
		return nil, err
	}
	oauthResp, err := s.maybeBuildWeChatOAuthRequiredResponseForSelection(ctx, req, limitAmount, payAmount, feeRate, sel)
	if err != nil {
		return nil, err
	}
	if oauthResp != nil {
		return oauthResp, nil
	}
	order, err := s.createOrderInTx(ctx, req, user, plan, cfg, orderAmount, limitAmount, feeRate, payAmount, sel)
	if err != nil {
		return nil, err
	}
	resp, err := s.invokeProvider(ctx, order, req, cfg, limitAmount, payAmountStr, payAmount, plan, sel)
	if err != nil {
		_, _ = s.entClient.PaymentOrder.UpdateOneID(order.ID).
			SetStatus(OrderStatusFailed).
			Save(ctx)
		return nil, err
	}
	return resp, nil
}

func (s *PaymentService) validateOrderInput(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig) (*dbent.SubscriptionPlan, error) {
	if req.OrderType == payment.OrderTypeBalance && cfg.BalanceDisabled {
		return nil, infraerrors.Forbidden("BALANCE_PAYMENT_DISABLED", "balance recharge has been disabled")
	}
	if req.OrderType == payment.OrderTypeSubscription {
		return s.validateSubOrder(ctx, req)
	}
	if math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) || req.Amount <= 0 {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
	}
	if (cfg.MinAmount > 0 && req.Amount < cfg.MinAmount) || (cfg.MaxAmount > 0 && req.Amount > cfg.MaxAmount) {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount out of range").
			WithMetadata(map[string]string{"min": fmt.Sprintf("%.2f", cfg.MinAmount), "max": fmt.Sprintf("%.2f", cfg.MaxAmount)})
	}
	return nil, nil
}

func (s *PaymentService) validateSubOrder(ctx context.Context, req CreateOrderRequest) (*dbent.SubscriptionPlan, error) {
	if req.PlanID == 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription order requires a plan")
	}
	plan, err := s.configService.GetPlan(ctx, req.PlanID)
	if err != nil || !plan.ForSale {
		return nil, infraerrors.NotFound("PLAN_NOT_AVAILABLE", "plan not found or not for sale")
	}
	group, err := s.groupRepo.GetByID(ctx, plan.GroupID)
	if err != nil || group.Status != payment.EntityStatusActive {
		return nil, infraerrors.NotFound("GROUP_NOT_FOUND", "subscription group is no longer available")
	}
	if !group.IsSubscriptionType() {
		return nil, infraerrors.BadRequest("GROUP_TYPE_MISMATCH", "group is not a subscription type")
	}
	return plan, nil
}

func (s *PaymentService) createOrderInTx(ctx context.Context, req CreateOrderRequest, user *User, plan *dbent.SubscriptionPlan, cfg *PaymentConfig, orderAmount, limitAmount, feeRate, payAmount float64, sel *payment.InstanceSelection) (*dbent.PaymentOrder, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// 待处理订单数与当日额度都是「先读后判断」，必须先拿到用户行锁把同一用户的
	// 并发建单串行化，否则 N 个并发请求会各自读到同一个旧值后一起通过。
	if err := lockUserRowForOrderLimits(ctx, tx, req.UserID); err != nil {
		return nil, err
	}
	if err := s.checkPendingLimit(ctx, tx, req.UserID, cfg.MaxPendingOrders); err != nil {
		return nil, err
	}
	if err := s.checkDailyLimit(ctx, tx, req.UserID, limitAmount, cfg.DailyLimit); err != nil {
		return nil, err
	}
	tm := cfg.OrderTimeoutMin
	if tm <= 0 {
		tm = defaultOrderTimeoutMin
	}
	exp := time.Now().Add(time.Duration(tm) * time.Minute)
	outTradeNo, err := s.allocateOutTradeNo(ctx, tx)
	if err != nil {
		return nil, err
	}
	providerSnapshot := buildPaymentOrderProviderSnapshot(sel, req)
	selectedInstanceID := ""
	selectedProviderKey := ""
	if sel != nil {
		selectedInstanceID = strings.TrimSpace(sel.InstanceID)
		selectedProviderKey = strings.TrimSpace(sel.ProviderKey)
	}
	rechargeCode, err := generateRechargeCode()
	if err != nil {
		return nil, err
	}
	b := tx.PaymentOrder.Create().
		SetUserID(req.UserID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetNillableUserNotes(psNilIfEmpty(user.Notes)).
		SetAmount(orderAmount).
		SetPayAmount(payAmount).
		SetFeeRate(feeRate).
		SetRechargeCode(rechargeCode).
		SetOutTradeNo(outTradeNo).
		SetPaymentType(req.PaymentType).
		SetPaymentTradeNo("").
		SetOrderType(req.OrderType).
		SetStatus(OrderStatusPending).
		SetExpiresAt(exp).
		SetClientIP(req.ClientIP).
		SetSrcHost(req.SrcHost)
	if req.SrcURL != "" {
		b.SetSrcURL(req.SrcURL)
	}
	if selectedInstanceID != "" {
		b.SetProviderInstanceID(selectedInstanceID)
	}
	if selectedProviderKey != "" {
		b.SetProviderKey(selectedProviderKey)
	}
	if providerSnapshot != nil {
		b.SetProviderSnapshot(providerSnapshot)
	}
	if plan != nil {
		b.SetPlanID(plan.ID).SetSubscriptionGroupID(plan.GroupID).SetSubscriptionDays(psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit))
	}
	order, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit order transaction: %w", err)
	}
	return order, nil
}

func (s *PaymentService) allocateOutTradeNo(ctx context.Context, tx *dbent.Tx) (string, error) {
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		candidate := generateOutTradeNo()
		exists, err := tx.PaymentOrder.Query().Where(paymentorder.OutTradeNo(candidate)).Exist(ctx)
		if err != nil {
			return "", fmt.Errorf("check out_trade_no uniqueness: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("generate unique out_trade_no: exhausted %d attempts", maxAttempts)
}

// rechargeCodeCharset 是 Crockford 风格的 32 字符表（去掉了 I/O/0/1 等易混字符）。
// 长度正好 32，所以 byte % len 不引入模偏差。
var rechargeCodeCharset = []byte("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

const (
	rechargeCodePrefix = "PAY-"
	// rechargeCodeRandomLen 个字符 × 5 bit = 130 bit 熵。
	// redeem_codes.code 是 VARCHAR(32)，"PAY-" + 26 = 30 字符，刚好放得下。
	rechargeCodeRandomLen = 26
)

// generateRechargeCode 生成充值订单对应的兑换码。
//
// 旧格式 "PAY-<orderID>-<UnixNano%100000>" 只有约 17 bit 熵，而且订单 id 是自增的：
// 任何用户都能用自己的订单 id 推出别人的订单 id，再对 POST /api/v1/redeem 暴力枚举
// 5 位后缀，在「已创建兑换码但还没兑换」的窗口里抢走别人的充值。新格式不再携带订单 id，
// 全部来自 crypto/rand。
func generateRechargeCode() (string, error) {
	buf := make([]byte, rechargeCodeRandomLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate recharge code: %w", err)
	}
	for i := range buf {
		buf[i] = rechargeCodeCharset[int(buf[i])%len(rechargeCodeCharset)]
	}
	return rechargeCodePrefix + string(buf), nil
}

// lockUserRowForOrderLimits 对用户行加排他锁，串行化同一用户的并发建单。
//
// PostgreSQL 默认隔离级别是 READ COMMITTED：checkPendingLimit / checkDailyLimit 都是
// 「聚合读 -> 判断 -> 插入订单」。并发事务互相看不到对方未提交的插入，聚合结果里也没有
// 可以加锁的目标行（幻读），所以单纯把读改成单条 SQL 并不能防住并发绕过——必须有一个
// 共同的锁点。拿到行锁之后的语句会重新取快照，能看到对方已提交的订单。
//
// SQLite（单元测试用的内存库）不支持 FOR UPDATE，直接跳过；生产只跑 PostgreSQL。
func lockUserRowForOrderLimits(ctx context.Context, tx *dbent.Tx, userID int64) error {
	if tx == nil {
		return nil
	}
	client := tx.Client()
	if paymentAuditDialect(client) != dialect.Postgres {
		return nil
	}
	rows, err := client.QueryContext(ctx, "SELECT 1 FROM users WHERE id = $1 FOR UPDATE", userID)
	if err != nil {
		return fmt.Errorf("lock user row for order limits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() { //nolint:revive // 只为持锁，行内容不需要
	}
	return rows.Err()
}

// sumPaymentOrderAmount 在库内对指定字段求和，避免把当天所有订单读进内存再累加。
func sumPaymentOrderAmount(ctx context.Context, tx *dbent.Tx, preds []predicate.PaymentOrder, field string) (float64, error) {
	var result []struct {
		Sum float64 `json:"sum"`
	}
	if err := tx.PaymentOrder.Query().
		Where(preds...).
		Aggregate(dbent.As(dbent.Sum(field), "sum")).
		Scan(ctx, &result); err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return 0, nil
	}
	return result[0].Sum, nil
}

// checkPendingLimit 统计用户当前待支付订单数。
// 调用方必须已经通过 lockUserRowForOrderLimits 持有用户行锁，否则并发请求会一起通过。
func (s *PaymentService) checkPendingLimit(ctx context.Context, tx *dbent.Tx, userID int64, max int) error {
	if max <= 0 {
		max = defaultMaxPendingOrders
	}
	c, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusEQ(OrderStatusPending)).Count(ctx)
	if err != nil {
		return fmt.Errorf("count pending orders: %w", err)
	}
	if c >= max {
		return infraerrors.TooManyRequests("TOO_MANY_PENDING", "too_many_pending").
			WithMetadata(map[string]string{"max": strconv.Itoa(max)})
	}
	return nil
}

func buildPaymentOrderProviderSnapshot(sel *payment.InstanceSelection, req CreateOrderRequest) map[string]any {
	if sel == nil {
		return nil
	}

	snapshot := map[string]any{}
	snapshot["schema_version"] = 2

	instanceID := strings.TrimSpace(sel.InstanceID)
	if instanceID != "" {
		snapshot["provider_instance_id"] = instanceID
	}

	providerKey := strings.TrimSpace(sel.ProviderKey)
	if providerKey != "" {
		snapshot["provider_key"] = providerKey
	}

	paymentMode := strings.TrimSpace(sel.PaymentMode)
	if paymentMode != "" {
		snapshot["payment_mode"] = paymentMode
	}

	if providerKey == payment.TypeWxpay {
		if merchantAppID := paymentOrderSnapshotWxpayAppID(sel, req); merchantAppID != "" {
			snapshot["merchant_app_id"] = merchantAppID
		}
		if merchantID := strings.TrimSpace(sel.Config["mchId"]); merchantID != "" {
			snapshot["merchant_id"] = merchantID
		}
		snapshot["currency"] = payment.DefaultPaymentCurrency
	}
	if providerKey == payment.TypeAlipay {
		if merchantAppID := strings.TrimSpace(sel.Config["appId"]); merchantAppID != "" {
			snapshot["merchant_app_id"] = merchantAppID
		}
	}
	if providerKey == payment.TypeEasyPay {
		if merchantID := strings.TrimSpace(sel.Config["pid"]); merchantID != "" {
			snapshot["merchant_id"] = merchantID
		}
	}
	if providerKey == payment.TypeStripe {
		snapshot["currency"] = paymentProviderConfigCurrency(providerKey, sel.Config)
	}
	if providerKey == payment.TypeAirwallex {
		if accountID := strings.TrimSpace(sel.Config["accountId"]); accountID != "" {
			snapshot["merchant_id"] = accountID
		}
		snapshot["currency"] = paymentProviderConfigCurrency(providerKey, sel.Config)
	}

	if len(snapshot) == 1 {
		return nil
	}
	return snapshot
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

// sumDailyOrderAmount 按「余额订单算实付、其余算订单金额」的口径求和。
// extra 是这一类订单的附加谓词（已支付 / 未过期挂起）。
func sumDailyOrderAmount(ctx context.Context, tx *dbent.Tx, userID int64, extra ...predicate.PaymentOrder) (float64, error) {
	base := func() []predicate.PaymentOrder {
		preds := []predicate.PaymentOrder{paymentorder.UserIDEQ(userID)}
		return append(preds, extra...)
	}
	balanceUsed, err := sumPaymentOrderAmount(ctx, tx,
		append(base(), paymentorder.OrderTypeEQ(payment.OrderTypeBalance)), paymentorder.FieldPayAmount)
	if err != nil {
		return 0, err
	}
	otherUsed, err := sumPaymentOrderAmount(ctx, tx,
		append(base(), paymentorder.OrderTypeNEQ(payment.OrderTypeBalance)), paymentorder.FieldAmount)
	if err != nil {
		return 0, err
	}
	return balanceUsed + otherUsed, nil
}

// unexpiredPendingDailyLimitPredicates 返回「仍会被支付」的挂起订单谓词。
//
// 只统计未过期的挂起订单：已过期的订单不会再被履约（toPaid 只在过期宽限期内
// 接受回调），继续占额度就成了永久性的封锁；未过期的订单则随时可能被支付，
// 必须把额度预留出来。
func unexpiredPendingDailyLimitPredicates(now time.Time) []predicate.PaymentOrder {
	return []predicate.PaymentOrder{
		paymentorder.StatusEQ(OrderStatusPending),
		paymentorder.ExpiresAtGT(now),
	}
}

// earliestUnexpiredPendingExpiry 返回未过期挂起订单里最早的过期时间，
// 用于告诉用户被占用的额度什么时候会自动释放。
func earliestUnexpiredPendingExpiry(ctx context.Context, tx *dbent.Tx, userID int64, now time.Time) (time.Time, bool) {
	preds := append([]predicate.PaymentOrder{paymentorder.UserIDEQ(userID)}, unexpiredPendingDailyLimitPredicates(now)...)
	order, err := tx.PaymentOrder.Query().
		Where(preds...).
		Order(dbent.Asc(paymentorder.FieldExpiresAt)).
		First(ctx)
	if err != nil || order == nil {
		return time.Time{}, false
	}
	return order.ExpiresAt, true
}

// checkDailyLimit 统计用户当天额度是否还放得下本次订单。
// 余额订单按实付金额计入，其余（订阅）按订单金额计入，聚合语句都在库内求和。
// 调用方必须已经通过 lockUserRowForOrderLimits 持有用户行锁。
//
// 除了已支付（paid/recharging/completed）的订单，未过期的挂起订单也计入额度：
// 否则用户可以连开 MaxPendingOrders 个挂起订单——每一个建单时都读到 used=0，
// 都判定「放得下」——再把它们全部付掉，实际入账额是日限额的数倍。履约侧不能补检查
// （钱已经收了，不可能拒绝入账），所以只能在建单时预留额度。
// 被占用的额度不会永久锁死：挂起订单到 OrderTimeoutMin 分钟后过期就自动释放。
func (s *PaymentService) checkDailyLimit(ctx context.Context, tx *dbent.Tx, userID int64, amount, limit float64) error {
	if limit <= 0 {
		return nil
	}
	now := time.Now()
	ts := psStartOfDayUTC(now)
	paidUsed, err := sumDailyOrderAmount(ctx, tx, userID,
		paymentorder.StatusIn(OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted),
		paymentorder.PaidAtGTE(ts))
	if err != nil {
		return fmt.Errorf("query daily usage: %w", err)
	}
	if paidUsed+amount > limit {
		return infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", "daily_limit_exceeded").
			WithMetadata(map[string]string{"remaining": fmt.Sprintf("%.2f", math.Max(0, limit-paidUsed))})
	}

	// 未过期的挂起订单不按创建日期过滤：它们一旦被支付，paid_at 就落在支付当天，
	// 也就是「现在」，所以对当天额度的占用与创建时间无关。
	pendingHeld, err := sumDailyOrderAmount(ctx, tx, userID, unexpiredPendingDailyLimitPredicates(now)...)
	if err != nil {
		return fmt.Errorf("query pending daily usage: %w", err)
	}
	if pendingHeld <= 0 || paidUsed+pendingHeld+amount <= limit {
		return nil
	}

	remaining := math.Max(0, limit-paidUsed-pendingHeld)
	meta := map[string]string{
		"remaining": fmt.Sprintf("%.2f", remaining),
		"held":      fmt.Sprintf("%.2f", pendingHeld),
	}
	detail := ""
	if expiresAt, ok := earliestUnexpiredPendingExpiry(ctx, tx, userID, now); ok {
		meta["held_until"] = expiresAt.UTC().Format(time.RFC3339)
		retryAfter := int(math.Ceil(expiresAt.Sub(now).Seconds()))
		if retryAfter < 0 {
			retryAfter = 0
		}
		meta["retry_after_seconds"] = strconv.Itoa(retryAfter)
		detail = fmt.Sprintf(", the earliest frees up at %s", meta["held_until"])
	}
	return infraerrors.TooManyRequests("DAILY_LIMIT_PENDING_HOLD",
		fmt.Sprintf("unpaid pending orders are holding %.2f of today's limit%s; pay or cancel them to free the headroom", pendingHeld, detail)).
		WithMetadata(meta)
}

func (s *PaymentService) selectCreateOrderInstance(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig, payAmount float64) (*payment.InstanceSelection, error) {
	selectCtx, err := s.prepareCreateOrderSelectionContext(ctx, req)
	if err != nil {
		return nil, err
	}
	sel, err := s.loadBalancer.SelectInstance(selectCtx, "", req.PaymentType, payment.Strategy(cfg.LoadBalanceStrategy), payAmount)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "method_not_configured").
			WithMetadata(map[string]string{"payment_type": req.PaymentType})
	}
	if sel == nil {
		return nil, infraerrors.TooManyRequests("NO_AVAILABLE_INSTANCE", "no_available_instance")
	}
	return sel, nil
}

func (s *PaymentService) prepareCreateOrderSelectionContext(ctx context.Context, req CreateOrderRequest) (context.Context, error) {
	if !requestNeedsWeChatJSAPICompatibility(req) {
		return ctx, nil
	}
	if !s.usesOfficialWxpayVisibleMethod(ctx) {
		return ctx, nil
	}
	expectedAppID, _, err := s.getWeChatPaymentOAuthCredential(ctx)
	if err != nil {
		return nil, err
	}
	return payment.WithWxpayJSAPIAppID(ctx, expectedAppID), nil
}

func requestNeedsWeChatJSAPICompatibility(req CreateOrderRequest) bool {
	if payment.GetBasePaymentType(req.PaymentType) != payment.TypeWxpay {
		return false
	}
	return req.IsWeChatBrowser || strings.TrimSpace(req.OpenID) != ""
}

func (s *PaymentService) usesOfficialWxpayVisibleMethod(ctx context.Context) bool {
	if s == nil || s.configService == nil {
		return false
	}
	inst, err := s.configService.resolveEnabledVisibleMethodInstance(ctx, payment.TypeWxpay)
	if err != nil {
		return false
	}
	if inst == nil {
		return false
	}
	return inst.ProviderKey == payment.TypeWxpay
}

func (s *PaymentService) invokeProvider(ctx context.Context, order *dbent.PaymentOrder, req CreateOrderRequest, cfg *PaymentConfig, limitAmount float64, payAmountStr string, payAmount float64, plan *dbent.SubscriptionPlan, sel *payment.InstanceSelection) (*CreateOrderResponse, error) {
	prov, err := provider.CreateProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		slog.Error("[PaymentService] CreateProvider failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		// If the provider returned a structured ApplicationError (e.g. WXPAY_CONFIG_MISSING_KEY),
		// pass it through with provider context added to metadata. Otherwise wrap as PAYMENT_PROVIDER_MISCONFIGURED.
		if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) {
			md := map[string]string{"provider": sel.ProviderKey, "instance_id": sel.InstanceID}
			for k, v := range appErr.Metadata {
				md[k] = v
			}
			return nil, appErr.WithMetadata(md)
		}
		return nil, infraerrors.ServiceUnavailable("PAYMENT_PROVIDER_MISCONFIGURED", "provider_misconfigured").
			WithMetadata(map[string]string{"provider": sel.ProviderKey, "instance_id": sel.InstanceID})
	}
	subject := s.buildPaymentSubject(plan, limitAmount, cfg, sel)
	outTradeNo := order.OutTradeNo
	canonicalReturnURL, err := CanonicalizeReturnURL(req.ReturnURL, req.SrcHost, req.SrcURL)
	if err != nil {
		return nil, err
	}
	resumeToken := ""
	if resume := s.paymentResume(); resume != nil {
		if canonicalReturnURL != "" && resume.isSigningConfigured() {
			resumeToken, err = resume.CreateToken(ResumeTokenClaims{
				OrderID:            order.ID,
				UserID:             order.UserID,
				ProviderInstanceID: sel.InstanceID,
				ProviderKey:        sel.ProviderKey,
				PaymentType:        req.PaymentType,
				CanonicalReturnURL: canonicalReturnURL,
			})
			if err != nil {
				return nil, fmt.Errorf("create payment resume token: %w", err)
			}
		}
	}
	providerReturnURL, err := buildPaymentReturnURL(canonicalReturnURL, order.ID, outTradeNo, resumeToken)
	if err != nil {
		return nil, err
	}
	providerReq := buildProviderCreatePaymentRequest(CreateOrderRequest{
		PaymentType: req.PaymentType,
		OpenID:      req.OpenID,
		ClientIP:    req.ClientIP,
		IsMobile:    req.IsMobile,
		ReturnURL:   providerReturnURL,
	}, sel, outTradeNo, payAmountStr, subject)
	providerReq.AlipayMobilePrecreate = shouldUseAlipayMobilePrecreate(req, cfg, sel)
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	pr, err := prov.CreatePayment(ctx, providerReq)
	finishProviderCall()
	if err != nil {
		slog.Error("[PaymentService] CreatePayment failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) {
			return nil, appErr
		}
		return nil, classifyCreatePaymentError(req, sel.ProviderKey, err)
	}
	sanitizeCreatePaymentResponseDetails(pr)
	_, err = s.entClient.PaymentOrder.UpdateOneID(order.ID).
		SetNillablePaymentTradeNo(psNilIfEmpty(pr.TradeNo)).
		SetNillablePayURL(psNilIfEmpty(pr.PayURL)).
		SetNillableQrCode(psNilIfEmpty(pr.QRCode)).
		SetNillableProviderInstanceID(psNilIfEmpty(sel.InstanceID)).
		SetNillableProviderKey(psNilIfEmpty(sel.ProviderKey)).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update order with payment details: %w", err)
	}
	s.writeAuditLog(ctx, order.ID, "ORDER_CREATED", fmt.Sprintf("user:%d", req.UserID), map[string]any{
		"paymentAmount":  req.Amount,
		"creditedAmount": order.Amount,
		"payAmount":      order.PayAmount,
		"paymentType":    req.PaymentType,
		"orderType":      req.OrderType,
		"paymentSource":  NormalizePaymentSource(req.PaymentSource),
	})
	resultType := pr.ResultType
	if resultType == "" {
		resultType = payment.CreatePaymentResultOrderCreated
	}
	resp := buildCreateOrderResponse(order, req, payAmount, sel, pr, resultType)
	resp.ResumeToken = resumeToken
	resp.AlipayMobilePrecreateDeepLink = providerReq.AlipayMobilePrecreate && strings.TrimSpace(pr.QRCode) != ""
	return resp, nil
}

func shouldUseAlipayMobilePrecreate(req CreateOrderRequest, cfg *PaymentConfig, sel *payment.InstanceSelection) bool {
	return cfg != nil &&
		cfg.AlipayMobilePrecreateDeepLink &&
		req.IsMobile &&
		sel != nil &&
		strings.EqualFold(strings.TrimSpace(sel.ProviderKey), payment.TypeAlipay)
}

func sanitizeCreatePaymentResponseDetails(pr *payment.CreatePaymentResponse) {
	if pr == nil {
		return
	}
	pr.TradeNo = removePostgresTextNUL(pr.TradeNo)
	pr.PayURL = removePostgresTextNUL(pr.PayURL)
	pr.QRCode = removePostgresTextNUL(pr.QRCode)
}

func removePostgresTextNUL(value string) string {
	if !strings.ContainsRune(value, 0) {
		return value
	}
	return strings.ReplaceAll(value, "\x00", "")
}

func buildProviderCreatePaymentRequest(req CreateOrderRequest, sel *payment.InstanceSelection, orderID, amount, subject string) payment.CreatePaymentRequest {
	return payment.CreatePaymentRequest{
		OrderID:            orderID,
		Amount:             amount,
		PaymentType:        req.PaymentType,
		Subject:            subject,
		ReturnURL:          req.ReturnURL,
		OpenID:             strings.TrimSpace(req.OpenID),
		ClientIP:           req.ClientIP,
		IsMobile:           req.IsMobile,
		InstanceSubMethods: selectedInstanceSupportedTypes(sel),
	}
}

func selectedInstanceSupportedTypes(sel *payment.InstanceSelection) string {
	if sel == nil {
		return ""
	}
	return sel.SupportedTypes
}

func (s *PaymentService) buildPaymentSubject(plan *dbent.SubscriptionPlan, limitAmount float64, cfg *PaymentConfig, sel *payment.InstanceSelection) string {
	if plan != nil {
		productName := plan.ProductName
		if productName == "" {
			productName = "Sub2API Subscription " + plan.Name
		}
		return applyPaymentProductNameAffix(productName, cfg)
	}
	currency := payment.DefaultPaymentCurrency
	if sel != nil {
		currency = paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	}
	amountStr := payment.FormatAmountForCurrency(limitAmount, currency)
	if hasPaymentProductNameAffix(cfg) {
		return applyPaymentProductNameAffix(amountStr, cfg)
	}
	return "Sub2API " + amountStr + " " + currency
}

func hasPaymentProductNameAffix(cfg *PaymentConfig) bool {
	if cfg == nil {
		return false
	}
	pf := strings.TrimSpace(cfg.ProductNamePrefix)
	sf := strings.TrimSpace(cfg.ProductNameSuffix)
	return pf != "" || sf != ""
}

func applyPaymentProductNameAffix(productName string, cfg *PaymentConfig) string {
	if !hasPaymentProductNameAffix(cfg) {
		return productName
	}
	pf := strings.TrimSpace(cfg.ProductNamePrefix)
	sf := strings.TrimSpace(cfg.ProductNameSuffix)
	return strings.TrimSpace(pf + " " + productName + " " + sf)
}

func (s *PaymentService) maybeBuildWeChatOAuthRequiredResponse(ctx context.Context, req CreateOrderRequest, amount, payAmount, feeRate float64) (*CreateOrderResponse, error) {
	return s.maybeBuildWeChatOAuthRequiredResponseForSelection(ctx, req, amount, payAmount, feeRate, nil)
}

func (s *PaymentService) maybeBuildWeChatOAuthRequiredResponseForSelection(ctx context.Context, req CreateOrderRequest, amount, payAmount, feeRate float64, sel *payment.InstanceSelection) (*CreateOrderResponse, error) {
	if sel != nil && sel.ProviderKey != "" && sel.ProviderKey != payment.TypeWxpay {
		return nil, nil
	}
	if strings.TrimSpace(req.OpenID) != "" || !req.IsWeChatBrowser || payment.GetBasePaymentType(req.PaymentType) != payment.TypeWxpay {
		return nil, nil
	}
	return s.buildWeChatOAuthRequiredResponse(ctx, req, amount, payAmount, feeRate)
}

func (s *PaymentService) buildWeChatOAuthRequiredResponse(ctx context.Context, req CreateOrderRequest, amount, payAmount, feeRate float64) (*CreateOrderResponse, error) {
	appID, _, err := s.getWeChatPaymentOAuthCredential(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.paymentResume().ensureSigningKey(); err != nil {
		return nil, err
	}

	authorizeURL, err := buildWeChatPaymentOAuthStartURL(req, "snsapi_base")
	if err != nil {
		return nil, err
	}

	return &CreateOrderResponse{
		Amount:      amount,
		PayAmount:   payAmount,
		FeeRate:     feeRate,
		ResultType:  payment.CreatePaymentResultOAuthRequired,
		PaymentType: req.PaymentType,
		OAuth: &payment.WechatOAuthInfo{
			AuthorizeURL: authorizeURL,
			AppID:        appID,
			Scope:        "snsapi_base",
			RedirectURL:  "/auth/wechat/payment/callback",
		},
	}, nil
}

func (s *PaymentService) validateSelectedCreateOrderInstance(ctx context.Context, req CreateOrderRequest, sel *payment.InstanceSelection) error {
	if !requiresWeChatJSAPICompatibleSelection(req, sel) {
		return nil
	}
	expectedAppID, _, err := s.getWeChatPaymentOAuthCredential(ctx)
	if err != nil {
		return err
	}
	selectedAppID := provider.ResolveWxpayJSAPIAppID(sel.Config)
	if selectedAppID == "" || selectedAppID != expectedAppID {
		return infraerrors.TooManyRequests("NO_AVAILABLE_INSTANCE", "selected payment instance is not compatible with the current WeChat OAuth app")
	}
	return nil
}

func calculateCreateOrderPayAmount(limitAmount, feeRate float64, currency string) (string, float64, error) {
	if err := validateCreateOrderAmountCurrency(limitAmount, currency); err != nil {
		return "", 0, err
	}
	payAmountStr := payment.CalculatePayAmountForCurrency(limitAmount, feeRate, currency)
	if _, err := payment.AmountToMinorUnit(payAmountStr, currency); err != nil {
		return "", 0, infraerrors.BadRequest("INVALID_AMOUNT", err.Error()).
			WithMetadata(map[string]string{"currency": currency})
	}
	payAmount, err := strconv.ParseFloat(payAmountStr, 64)
	if err != nil {
		return "", 0, infraerrors.BadRequest("INVALID_AMOUNT", "invalid payment amount").
			WithMetadata(map[string]string{"currency": currency})
	}
	return payAmountStr, payAmount, nil
}

func calculateCreateOrderPayAmountForOrderType(limitAmount, feeRate float64, currency, orderType string, usdToCnyRate float64) (string, float64, error) {
	paymentAmount := limitAmount
	if orderType == payment.OrderTypeSubscription {
		paymentAmount = calculateSubscriptionGatewayBaseAmount(limitAmount, usdToCnyRate, currency)
	}
	return calculateCreateOrderPayAmount(paymentAmount, feeRate, currency)
}

// calculateSubscriptionGatewayBaseAmount 计算订阅订单的网关扣款基数。
// 换算是显式 opt-in：仅当管理员配置了订阅汇率（rate > 0，1 USD = rate CNY）
// 且网关币种为 CNY 时，按 price × rate 换算；未配置时保持 price 直付的存量行为。
func calculateSubscriptionGatewayBaseAmount(amount, usdToCnyRate float64, currency string) float64 {
	rate := normalizeSubscriptionUSDToCNYRate(usdToCnyRate)
	if rate <= 0 || currency != payment.DefaultPaymentCurrency {
		return amount
	}
	return decimal.NewFromFloat(amount).
		Mul(decimal.NewFromFloat(rate)).
		Round(int32(payment.CurrencyMaxFractionDigits(currency))).
		InexactFloat64()
}

func validateCreateOrderAmountCurrency(amount float64, currency string) error {
	amountStr := strconv.FormatFloat(amount, 'f', -1, 64)
	if _, err := payment.AmountToMinorUnit(amountStr, currency); err != nil {
		return infraerrors.BadRequest("INVALID_AMOUNT", err.Error()).
			WithMetadata(map[string]string{"currency": currency})
	}
	return nil
}

func validateSelectedCreateOrderAmountCurrency(payAmount string, sel *payment.InstanceSelection) error {
	if sel == nil {
		return nil
	}
	currency := paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	if _, err := payment.AmountToMinorUnit(payAmount, currency); err != nil {
		return infraerrors.BadRequest("INVALID_AMOUNT", err.Error()).
			WithMetadata(map[string]string{"currency": currency})
	}
	return nil
}

func requiresWeChatJSAPICompatibleSelection(req CreateOrderRequest, sel *payment.InstanceSelection) bool {
	if sel == nil || sel.ProviderKey != payment.TypeWxpay || payment.GetBasePaymentType(req.PaymentType) != payment.TypeWxpay {
		return false
	}
	return req.IsWeChatBrowser || strings.TrimSpace(req.OpenID) != ""
}

func (s *PaymentService) getWeChatPaymentOAuthCredential(ctx context.Context) (string, string, error) {
	if s == nil || s.configService == nil || s.configService.settingRepo == nil {
		return "", "", infraerrors.ServiceUnavailable(
			"WECHAT_PAYMENT_MP_NOT_CONFIGURED",
			"wechat in-app payment requires a complete WeChat MP OAuth credential",
		)
	}
	cfg, err := (&SettingService{settingRepo: s.configService.settingRepo}).GetWeChatConnectOAuthConfig(ctx)
	appID := strings.TrimSpace(cfg.AppIDForMode("mp"))
	appSecret := strings.TrimSpace(cfg.AppSecretForMode("mp"))
	if err != nil || !cfg.SupportsMode("mp") || appID == "" || appSecret == "" {
		return "", "", infraerrors.ServiceUnavailable(
			"WECHAT_PAYMENT_MP_NOT_CONFIGURED",
			"wechat in-app payment requires a complete WeChat MP OAuth credential",
		)
	}
	return appID, appSecret, nil
}

func classifyCreatePaymentError(req CreateOrderRequest, providerKey string, err error) error {
	if err == nil {
		return nil
	}
	if providerKey == payment.TypeWxpay &&
		payment.GetBasePaymentType(req.PaymentType) == payment.TypeWxpay &&
		strings.Contains(err.Error(), "wxpay h5 payments are not authorized for this merchant") {
		return infraerrors.ServiceUnavailable(
			"WECHAT_H5_NOT_AUTHORIZED",
			"wechat h5 payment is not available for this merchant",
		).WithMetadata(map[string]string{
			"action": "open_in_wechat_or_scan_qr",
		})
	}
	return infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", fmt.Sprintf("payment gateway error: %s", err.Error()))
}

func buildCreateOrderResponse(order *dbent.PaymentOrder, req CreateOrderRequest, payAmount float64, sel *payment.InstanceSelection, pr *payment.CreatePaymentResponse, resultType payment.CreatePaymentResultType) *CreateOrderResponse {
	return &CreateOrderResponse{
		OrderID:      order.ID,
		Amount:       order.Amount,
		PayAmount:    payAmount,
		FeeRate:      order.FeeRate,
		Status:       OrderStatusPending,
		ResultType:   resultType,
		PaymentType:  req.PaymentType,
		OutTradeNo:   order.OutTradeNo,
		PayURL:       pr.PayURL,
		QRCode:       pr.QRCode,
		ClientSecret: pr.ClientSecret,
		IntentID:     pr.IntentID,
		Currency:     pr.Currency,
		CountryCode:  pr.CountryCode,
		PaymentEnv:   pr.PaymentEnv,
		OAuth:        pr.OAuth,
		JSAPI:        pr.JSAPI,
		JSAPIPayload: pr.JSAPI,
		ExpiresAt:    order.ExpiresAt,
		PaymentMode:  sel.PaymentMode,
	}
}

func buildWeChatPaymentOAuthStartURL(req CreateOrderRequest, scope string) (string, error) {
	u, err := url.Parse("/api/v1/auth/oauth/wechat/payment/start")
	if err != nil {
		return "", fmt.Errorf("build wechat payment oauth start url: %w", err)
	}
	q := u.Query()
	q.Set("payment_type", strings.TrimSpace(req.PaymentType))
	if req.Amount > 0 {
		q.Set("amount", strconv.FormatFloat(req.Amount, 'f', -1, 64))
	}
	if orderType := strings.TrimSpace(req.OrderType); orderType != "" {
		q.Set("order_type", orderType)
	}
	if req.PlanID > 0 {
		q.Set("plan_id", strconv.FormatInt(req.PlanID, 10))
	}
	if scope = strings.TrimSpace(scope); scope != "" {
		q.Set("scope", scope)
	}
	if redirectTo := paymentRedirectPathFromURL(req.SrcURL); redirectTo != "" {
		q.Set("redirect", redirectTo)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func paymentRedirectPathFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "/purchase"
	}
	if strings.HasPrefix(rawURL, "/") && !strings.HasPrefix(rawURL, "//") {
		return normalizePaymentRedirectPath(rawURL)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "/purchase"
	}
	path := strings.TrimSpace(u.EscapedPath())
	if path == "" {
		path = strings.TrimSpace(u.Path)
	}
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "/purchase"
	}
	if strings.TrimSpace(u.RawQuery) != "" {
		path += "?" + u.RawQuery
	}
	return normalizePaymentRedirectPath(path)
}

func normalizePaymentRedirectPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/purchase"
	}
	if path == "/payment" {
		return "/purchase"
	}
	if strings.HasPrefix(path, "/payment?") {
		return "/purchase" + strings.TrimPrefix(path, "/payment")
	}
	return path
}

// --- Order Queries ---

func (s *PaymentService) GetOrder(ctx context.Context, orderID, userID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	return o, nil
}

func (s *PaymentService) GetOrderByID(ctx context.Context, orderID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	return o, nil
}

func (s *PaymentService) GetUserOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID))
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
	}
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
	}
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count user orders: %w", err)
	}
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query user orders: %w", err)
	}
	return orders, total, nil
}

// AdminListOrders returns a paginated list of orders. If userID > 0, filters by user.
func (s *PaymentService) AdminListOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query()
	if userID > 0 {
		q = q.Where(paymentorder.UserIDEQ(userID))
	}
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
	}
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
	}
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
	}
	if p.Keyword != "" {
		q = q.Where(paymentorder.Or(
			paymentorder.OutTradeNoContainsFold(p.Keyword),
			paymentorder.UserEmailContainsFold(p.Keyword),
			paymentorder.UserNameContainsFold(p.Keyword),
		))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin orders: %w", err)
	}
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin orders: %w", err)
	}
	return orders, total, nil
}
