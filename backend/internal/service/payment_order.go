package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// --- Order Creation ---

func (s *PaymentService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
REDACTED
	if normalized := NormalizeVisibleMethod(req.PaymentType); normalized != "" {
		req.PaymentType = normalized
REDACTED
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
REDACTED
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
REDACTED
	plan, err := s.validateOrderInput(ctx, req, cfg)
	if err != nil {
		return nil, err
REDACTED
	if err := s.checkCancelRateLimit(ctx, req.UserID, cfg); err != nil {
		return nil, err
REDACTED
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	if user.Status != payment.EntityStatusActive {
		return nil, infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
REDACTED
	if s.notificationEmailService != nil {
		s.notificationEmailService.RememberRecipientLocale(ctx, req.UserID, user.Email, req.Locale)
REDACTED
	orderAmount := req.Amount
	limitAmount := req.Amount
	if plan != nil {
		orderAmount = plan.Price
		limitAmount = plan.Price
REDACTED else if req.OrderType == payment.OrderTypeBalance {
		orderAmount = calculateCreditedBalance(req.Amount, cfg.BalanceRechargeMultiplier)
REDACTED
	feeRate := cfg.RechargeFeeRate
	methodCurrency := payment.DefaultPaymentCurrency
	if s.configService != nil {
		methodCurrency, err = s.configService.ValidateMethodCurrencyConsistency(ctx, req.PaymentType)
		if err != nil {
			return nil, err
	REDACTED
REDACTED
	payAmountStr, payAmount, err := calculateCreateOrderPayAmountForOrderType(limitAmount, feeRate, methodCurrency, req.OrderType, cfg.BalanceRechargeMultiplier)
	if err != nil {
		return nil, err
REDACTED
	sel, err := s.selectCreateOrderInstance(ctx, req, cfg, payAmount)
	if err != nil {
		return nil, err
REDACTED
	if err := s.validateSelectedCreateOrderInstance(ctx, req, sel); err != nil {
		return nil, err
REDACTED
	selectedCurrency := payment.DefaultPaymentCurrency
	if sel != nil {
		selectedCurrency = paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
REDACTED
	if selectedCurrency != methodCurrency {
		payAmountStr, payAmount, err = calculateCreateOrderPayAmountForOrderType(limitAmount, feeRate, selectedCurrency, req.OrderType, cfg.BalanceRechargeMultiplier)
		if err != nil {
			return nil, err
	REDACTED
REDACTED
	if err := validateSelectedCreateOrderAmountCurrency(payAmountStr, sel); err != nil {
		return nil, err
REDACTED
	oauthResp, err := s.maybeBuildWeChatOAuthRequiredResponseForSelection(ctx, req, limitAmount, payAmount, feeRate, sel)
	if err != nil {
		return nil, err
REDACTED
	if oauthResp != nil {
		return oauthResp, nil
REDACTED
	order, err := s.createOrderInTx(ctx, req, user, plan, cfg, orderAmount, limitAmount, feeRate, payAmount, sel)
	if err != nil {
		return nil, err
REDACTED
	resp, err := s.invokeProvider(ctx, order, req, cfg, limitAmount, payAmountStr, payAmount, plan, sel)
	if err != nil {
		_, _ = s.entClient.PaymentOrder.UpdateOneID(order.ID).
			SetStatus(OrderStatusFailed).
			Save(ctx)
		return nil, err
REDACTED
	return resp, nil
REDACTED

func (s *PaymentService) validateOrderInput(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig) (*dbent.SubscriptionPlan, error) {
	if req.OrderType == payment.OrderTypeBalance && cfg.BalanceDisabled {
		return nil, infraerrors.Forbidden("BALANCE_PAYMENT_DISABLED", "balance recharge has been disabled")
REDACTED
	if req.OrderType == payment.OrderTypeSubscription {
		return s.validateSubOrder(ctx, req)
REDACTED
	if math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) || req.Amount <= 0 {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
REDACTED
	if (cfg.MinAmount > 0 && req.Amount < cfg.MinAmount) || (cfg.MaxAmount > 0 && req.Amount > cfg.MaxAmount) {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount out of range").
			WithMetadata(map[string]string{"min": fmt.Sprintf("%.2f", cfg.MinAmount), "max": fmt.Sprintf("%.2f", cfg.MaxAmount)REDACTED)
REDACTED
	return nil, nil
REDACTED

func (s *PaymentService) validateSubOrder(ctx context.Context, req CreateOrderRequest) (*dbent.SubscriptionPlan, error) {
	if req.PlanID == 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription order requires a plan")
REDACTED
	plan, err := s.configService.GetPlan(ctx, req.PlanID)
	if err != nil || !plan.ForSale {
		return nil, infraerrors.NotFound("PLAN_NOT_AVAILABLE", "plan not found or not for sale")
REDACTED
	group, err := s.groupRepo.GetByID(ctx, plan.GroupID)
	if err != nil || group.Status != payment.EntityStatusActive {
		return nil, infraerrors.NotFound("GROUP_NOT_FOUND", "subscription group is no longer available")
REDACTED
	if !group.IsSubscriptionType() {
		return nil, infraerrors.BadRequest("GROUP_TYPE_MISMATCH", "group is not a subscription type")
REDACTED
	return plan, nil
REDACTED

func (s *PaymentService) createOrderInTx(ctx context.Context, req CreateOrderRequest, user *User, plan *dbent.SubscriptionPlan, cfg *PaymentConfig, orderAmount, limitAmount, feeRate, payAmount float64, sel *payment.InstanceSelection) (*dbent.PaymentOrder, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()
	if err := s.checkPendingLimit(ctx, tx, req.UserID, cfg.MaxPendingOrders); err != nil {
		return nil, err
REDACTED
	if err := s.checkDailyLimit(ctx, tx, req.UserID, limitAmount, cfg.DailyLimit); err != nil {
		return nil, err
REDACTED
	tm := cfg.OrderTimeoutMin
	if tm <= 0 {
		tm = defaultOrderTimeoutMin
REDACTED
	exp := time.Now().Add(time.Duration(tm) * time.Minute)
	outTradeNo, err := s.allocateOutTradeNo(ctx, tx)
	if err != nil {
		return nil, err
REDACTED
	providerSnapshot := buildPaymentOrderProviderSnapshot(sel, req)
	selectedInstanceID := ""
	selectedProviderKey := ""
	if sel != nil {
		selectedInstanceID = strings.TrimSpace(sel.InstanceID)
		selectedProviderKey = strings.TrimSpace(sel.ProviderKey)
REDACTED
	b := tx.PaymentOrder.Create().
		SetUserID(req.UserID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetNillableUserNotes(psNilIfEmpty(user.Notes)).
		SetAmount(orderAmount).
		SetPayAmount(payAmount).
		SetFeeRate(feeRate).
		SetRechargeCode("").
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
REDACTED
	if selectedInstanceID != "" {
		b.SetProviderInstanceID(selectedInstanceID)
REDACTED
	if selectedProviderKey != "" {
		b.SetProviderKey(selectedProviderKey)
REDACTED
	if providerSnapshot != nil {
		b.SetProviderSnapshot(providerSnapshot)
REDACTED
	if plan != nil {
		b.SetPlanID(plan.ID).SetSubscriptionGroupID(plan.GroupID).SetSubscriptionDays(psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit))
REDACTED
	order, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
REDACTED
	code := fmt.Sprintf("PAY-%d-%d", order.ID, time.Now().UnixNano()%100000)
	order, err = tx.PaymentOrder.UpdateOneID(order.ID).SetRechargeCode(code).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("set recharge code: %w", err)
REDACTED
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit order transaction: %w", err)
REDACTED
	return order, nil
REDACTED

func (s *PaymentService) allocateOutTradeNo(ctx context.Context, tx *dbent.Tx) (string, error) {
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		candidate := generateOutTradeNo()
		exists, err := tx.PaymentOrder.Query().Where(paymentorder.OutTradeNo(candidate)).Exist(ctx)
		if err != nil {
			return "", fmt.Errorf("check out_trade_no uniqueness: %w", err)
	REDACTED
		if !exists {
			return candidate, nil
	REDACTED
REDACTED
	return "", fmt.Errorf("generate unique out_trade_no: exhausted %d attempts", maxAttempts)
REDACTED

func (s *PaymentService) checkPendingLimit(ctx context.Context, tx *dbent.Tx, userID int64, max int) error {
	if max <= 0 {
		max = defaultMaxPendingOrders
REDACTED
	c, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusEQ(OrderStatusPending)).Count(ctx)
	if err != nil {
		return fmt.Errorf("count pending orders: %w", err)
REDACTED
	if c >= max {
		return infraerrors.TooManyRequests("TOO_MANY_PENDING", "too_many_pending").
			WithMetadata(map[string]string{"max": strconv.Itoa(max)REDACTED)
REDACTED
	return nil
REDACTED

func buildPaymentOrderProviderSnapshot(sel *payment.InstanceSelection, req CreateOrderRequest) map[string]any {
	if sel == nil {
		return nil
REDACTED

	snapshot := map[string]any{REDACTED
	snapshot["schema_version"] = 2

	instanceID := strings.TrimSpace(sel.InstanceID)
	if instanceID != "" {
		snapshot["provider_instance_id"] = instanceID
REDACTED

	providerKey := strings.TrimSpace(sel.ProviderKey)
	if providerKey != "" {
		snapshot["provider_key"] = providerKey
REDACTED

	paymentMode := strings.TrimSpace(sel.PaymentMode)
	if paymentMode != "" {
		snapshot["payment_mode"] = paymentMode
REDACTED

	if providerKey == payment.TypeWxpay {
		if merchantAppID := paymentOrderSnapshotWxpayAppID(sel, req); merchantAppID != "" {
			snapshot["merchant_app_id"] = merchantAppID
	REDACTED
		if merchantID := strings.TrimSpace(sel.Config["mchId"]); merchantID != "" {
			snapshot["merchant_id"] = merchantID
	REDACTED
		snapshot["currency"] = payment.DefaultPaymentCurrency
REDACTED
	if providerKey == payment.TypeAlipay {
		if merchantAppID := strings.TrimSpace(sel.Config["appId"]); merchantAppID != "" {
			snapshot["merchant_app_id"] = merchantAppID
	REDACTED
REDACTED
	if providerKey == payment.TypeEasyPay {
		if merchantID := strings.TrimSpace(sel.Config["pid"]); merchantID != "" {
			snapshot["merchant_id"] = merchantID
	REDACTED
REDACTED
	if providerKey == payment.TypeStripe {
		snapshot["currency"] = paymentProviderConfigCurrency(providerKey, sel.Config)
REDACTED
	if providerKey == payment.TypeAirwallex {
		if accountID := strings.TrimSpace(sel.Config["accountId"]); accountID != "" {
			snapshot["merchant_id"] = accountID
	REDACTED
		snapshot["currency"] = paymentProviderConfigCurrency(providerKey, sel.Config)
REDACTED

	if len(snapshot) == 1 {
		return nil
REDACTED
	return snapshot
REDACTED

func paymentOrderSnapshotWxpayAppID(sel *payment.InstanceSelection, req CreateOrderRequest) string {
	if sel == nil || strings.TrimSpace(sel.ProviderKey) != payment.TypeWxpay {
		return ""
REDACTED
	if strings.TrimSpace(req.OpenID) != "" {
		return strings.TrimSpace(provider.ResolveWxpayJSAPIAppID(sel.Config))
REDACTED
	return strings.TrimSpace(sel.Config["appId"])
REDACTED

func (s *PaymentService) checkDailyLimit(ctx context.Context, tx *dbent.Tx, userID int64, amount, limit float64) error {
	if limit <= 0 {
		return nil
REDACTED
	ts := psStartOfDayUTC(time.Now())
	orders, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusIn(OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted), paymentorder.PaidAtGTE(ts)).All(ctx)
	if err != nil {
		return fmt.Errorf("query daily usage: %w", err)
REDACTED
	var used float64
	for _, o := range orders {
		if o.OrderType == payment.OrderTypeBalance {
			used += o.PayAmount
			continue
	REDACTED
		used += o.Amount
REDACTED
	if used+amount > limit {
		return infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", "daily_limit_exceeded").
			WithMetadata(map[string]string{"remaining": fmt.Sprintf("%.2f", math.Max(0, limit-used))REDACTED)
REDACTED
	return nil
REDACTED

func (s *PaymentService) selectCreateOrderInstance(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig, payAmount float64) (*payment.InstanceSelection, error) {
	selectCtx, err := s.prepareCreateOrderSelectionContext(ctx, req)
	if err != nil {
		return nil, err
REDACTED
	sel, err := s.loadBalancer.SelectInstance(selectCtx, "", req.PaymentType, payment.Strategy(cfg.LoadBalanceStrategy), payAmount)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "method_not_configured").
			WithMetadata(map[string]string{"payment_type": req.PaymentTypeREDACTED)
REDACTED
	if sel == nil {
		return nil, infraerrors.TooManyRequests("NO_AVAILABLE_INSTANCE", "no_available_instance")
REDACTED
	return sel, nil
REDACTED

func (s *PaymentService) prepareCreateOrderSelectionContext(ctx context.Context, req CreateOrderRequest) (context.Context, error) {
	if !requestNeedsWeChatJSAPICompatibility(req) {
		return ctx, nil
REDACTED
	if !s.usesOfficialWxpayVisibleMethod(ctx) {
		return ctx, nil
REDACTED
	expectedAppID, _, err := s.getWeChatPaymentOAuthCredential(ctx)
	if err != nil {
		return nil, err
REDACTED
	return payment.WithWxpayJSAPIAppID(ctx, expectedAppID), nil
REDACTED

func requestNeedsWeChatJSAPICompatibility(req CreateOrderRequest) bool {
	if payment.GetBasePaymentType(req.PaymentType) != payment.TypeWxpay {
		return false
REDACTED
	return req.IsWeChatBrowser || strings.TrimSpace(req.OpenID) != ""
REDACTED

func (s *PaymentService) usesOfficialWxpayVisibleMethod(ctx context.Context) bool {
	if s == nil || s.configService == nil {
		return false
REDACTED
	inst, err := s.configService.resolveEnabledVisibleMethodInstance(ctx, payment.TypeWxpay)
	if err != nil {
		return false
REDACTED
	if inst == nil {
		return false
REDACTED
	return inst.ProviderKey == payment.TypeWxpay
REDACTED

func (s *PaymentService) invokeProvider(ctx context.Context, order *dbent.PaymentOrder, req CreateOrderRequest, cfg *PaymentConfig, limitAmount float64, payAmountStr string, payAmount float64, plan *dbent.SubscriptionPlan, sel *payment.InstanceSelection) (*CreateOrderResponse, error) {
	prov, err := provider.CreateProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		slog.Error("[PaymentService] CreateProvider failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		// If the provider returned a structured ApplicationError (e.g. WXPAY_CONFIG_MISSING_KEY),
		// pass it through with provider context added to metadata. Otherwise wrap as PAYMENT_PROVIDER_MISCONFIGURED.
		if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) {
			md := map[string]string{"provider": sel.ProviderKey, "instance_id": sel.InstanceIDREDACTED
			for k, v := range appErr.Metadata {
				md[k] = v
		REDACTED
			return nil, appErr.WithMetadata(md)
	REDACTED
		return nil, infraerrors.ServiceUnavailable("PAYMENT_PROVIDER_MISCONFIGURED", "provider_misconfigured").
			WithMetadata(map[string]string{"provider": sel.ProviderKey, "instance_id": sel.InstanceIDREDACTED)
REDACTED
	subject := s.buildPaymentSubject(plan, limitAmount, cfg, sel)
	outTradeNo := order.OutTradeNo
	canonicalReturnURL, err := CanonicalizeReturnURL(req.ReturnURL, req.SrcHost, req.SrcURL)
	if err != nil {
		return nil, err
REDACTED
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
		REDACTED)
			if err != nil {
				return nil, fmt.Errorf("create payment resume token: %w", err)
		REDACTED
	REDACTED
REDACTED
	providerReturnURL, err := buildPaymentReturnURL(canonicalReturnURL, order.ID, outTradeNo, resumeToken)
	if err != nil {
		return nil, err
REDACTED
	providerReq := buildProviderCreatePaymentRequest(CreateOrderRequest{
		PaymentType: req.PaymentType,
		OpenID:      req.OpenID,
		ClientIP:    req.ClientIP,
		IsMobile:    req.IsMobile,
		ReturnURL:   providerReturnURL,
REDACTED, sel, outTradeNo, payAmountStr, subject)
	pr, err := prov.CreatePayment(ctx, providerReq)
	if err != nil {
		slog.Error("[PaymentService] CreatePayment failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) {
			return nil, appErr
	REDACTED
		return nil, classifyCreatePaymentError(req, sel.ProviderKey, err)
REDACTED
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
REDACTED
	s.writeAuditLog(ctx, order.ID, "ORDER_CREATED", fmt.Sprintf("user:%d", req.UserID), map[string]any{
		"paymentAmount":  req.Amount,
		"creditedAmount": order.Amount,
		"payAmount":      order.PayAmount,
		"paymentType":    req.PaymentType,
		"orderType":      req.OrderType,
		"paymentSource":  NormalizePaymentSource(req.PaymentSource),
REDACTED)
	resultType := pr.ResultType
	if resultType == "" {
		resultType = payment.CreatePaymentResultOrderCreated
REDACTED
	resp := buildCreateOrderResponse(order, req, payAmount, sel, pr, resultType)
	resp.ResumeToken = resumeToken
	return resp, nil
REDACTED

func sanitizeCreatePaymentResponseDetails(pr *payment.CreatePaymentResponse) {
	if pr == nil {
		return
REDACTED
	pr.TradeNo = removePostgresTextNUL(pr.TradeNo)
	pr.PayURL = removePostgresTextNUL(pr.PayURL)
	pr.QRCode = removePostgresTextNUL(pr.QRCode)
REDACTED

func removePostgresTextNUL(value string) string {
	if !strings.ContainsRune(value, 0) {
		return value
REDACTED
	return strings.ReplaceAll(value, "\x00", "")
REDACTED

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
REDACTED
REDACTED

func selectedInstanceSupportedTypes(sel *payment.InstanceSelection) string {
	if sel == nil {
		return ""
REDACTED
	return sel.SupportedTypes
REDACTED

func (s *PaymentService) buildPaymentSubject(plan *dbent.SubscriptionPlan, limitAmount float64, cfg *PaymentConfig, sel *payment.InstanceSelection) string {
	if plan != nil {
		productName := plan.ProductName
		if productName == "" {
			productName = "Sub2API Subscription " + plan.Name
	REDACTED
		return applyPaymentProductNameAffix(productName, cfg)
REDACTED
	currency := payment.DefaultPaymentCurrency
	if sel != nil {
		currency = paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
REDACTED
	amountStr := payment.FormatAmountForCurrency(limitAmount, currency)
	if hasPaymentProductNameAffix(cfg) {
		return applyPaymentProductNameAffix(amountStr, cfg)
REDACTED
	return "Sub2API " + amountStr + " " + currency
REDACTED

func hasPaymentProductNameAffix(cfg *PaymentConfig) bool {
	if cfg == nil {
		return false
REDACTED
	pf := strings.TrimSpace(cfg.ProductNamePrefix)
	sf := strings.TrimSpace(cfg.ProductNameSuffix)
	return pf != "" || sf != ""
REDACTED

func applyPaymentProductNameAffix(productName string, cfg *PaymentConfig) string {
	if !hasPaymentProductNameAffix(cfg) {
		return productName
REDACTED
	pf := strings.TrimSpace(cfg.ProductNamePrefix)
	sf := strings.TrimSpace(cfg.ProductNameSuffix)
	return strings.TrimSpace(pf + " " + productName + " " + sf)
REDACTED

func (s *PaymentService) maybeBuildWeChatOAuthRequiredResponse(ctx context.Context, req CreateOrderRequest, amount, payAmount, feeRate float64) (*CreateOrderResponse, error) {
	return s.maybeBuildWeChatOAuthRequiredResponseForSelection(ctx, req, amount, payAmount, feeRate, nil)
REDACTED

func (s *PaymentService) maybeBuildWeChatOAuthRequiredResponseForSelection(ctx context.Context, req CreateOrderRequest, amount, payAmount, feeRate float64, sel *payment.InstanceSelection) (*CreateOrderResponse, error) {
	if sel != nil && sel.ProviderKey != "" && sel.ProviderKey != payment.TypeWxpay {
		return nil, nil
REDACTED
	if strings.TrimSpace(req.OpenID) != "" || !req.IsWeChatBrowser || payment.GetBasePaymentType(req.PaymentType) != payment.TypeWxpay {
		return nil, nil
REDACTED
	return s.buildWeChatOAuthRequiredResponse(ctx, req, amount, payAmount, feeRate)
REDACTED

func (s *PaymentService) buildWeChatOAuthRequiredResponse(ctx context.Context, req CreateOrderRequest, amount, payAmount, feeRate float64) (*CreateOrderResponse, error) {
	appID, _, err := s.getWeChatPaymentOAuthCredential(ctx)
	if err != nil {
		return nil, err
REDACTED
	if err := s.paymentResume().ensureSigningKey(); err != nil {
		return nil, err
REDACTED

	authorizeURL, err := buildWeChatPaymentOAuthStartURL(req, "snsapi_base")
	if err != nil {
		return nil, err
REDACTED

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
	REDACTED,
REDACTED, nil
REDACTED

func (s *PaymentService) validateSelectedCreateOrderInstance(ctx context.Context, req CreateOrderRequest, sel *payment.InstanceSelection) error {
	if !requiresWeChatJSAPICompatibleSelection(req, sel) {
		return nil
REDACTED
	expectedAppID, _, err := s.getWeChatPaymentOAuthCredential(ctx)
	if err != nil {
		return err
REDACTED
	selectedAppID := provider.ResolveWxpayJSAPIAppID(sel.Config)
	if selectedAppID == "" || selectedAppID != expectedAppID {
		return infraerrors.TooManyRequests("NO_AVAILABLE_INSTANCE", "selected payment instance is not compatible with the current WeChat OAuth app")
REDACTED
	return nil
REDACTED

func calculateCreateOrderPayAmount(limitAmount, feeRate float64, currency string) (string, float64, error) {
	if err := validateCreateOrderAmountCurrency(limitAmount, currency); err != nil {
		return "", 0, err
REDACTED
	payAmountStr := payment.CalculatePayAmountForCurrency(limitAmount, feeRate, currency)
	if _, err := payment.AmountToMinorUnit(payAmountStr, currency); err != nil {
		return "", 0, infraerrors.BadRequest("INVALID_AMOUNT", err.Error()).
			WithMetadata(map[string]string{"currency": currencyREDACTED)
REDACTED
	payAmount, err := strconv.ParseFloat(payAmountStr, 64)
	if err != nil {
		return "", 0, infraerrors.BadRequest("INVALID_AMOUNT", "invalid payment amount").
			WithMetadata(map[string]string{"currency": currencyREDACTED)
REDACTED
	return payAmountStr, payAmount, nil
REDACTED

func calculateCreateOrderPayAmountForOrderType(limitAmount, feeRate float64, currency, orderType string, multiplier float64) (string, float64, error) {
	paymentAmount := limitAmount
	if orderType == payment.OrderTypeSubscription {
		paymentAmount = calculateSubscriptionGatewayBaseAmount(limitAmount, multiplier, currency)
REDACTED
	return calculateCreateOrderPayAmount(paymentAmount, feeRate, currency)
REDACTED

func calculateSubscriptionGatewayBaseAmount(amount, multiplier float64, currency string) float64 {
	if currency != payment.DefaultPaymentCurrency {
		return amount
REDACTED
	return decimal.NewFromFloat(amount).
		Div(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
		Round(int32(payment.CurrencyMaxFractionDigits(currency))).
		InexactFloat64()
REDACTED

func validateCreateOrderAmountCurrency(amount float64, currency string) error {
	amountStr := strconv.FormatFloat(amount, 'f', -1, 64)
	if _, err := payment.AmountToMinorUnit(amountStr, currency); err != nil {
		return infraerrors.BadRequest("INVALID_AMOUNT", err.Error()).
			WithMetadata(map[string]string{"currency": currencyREDACTED)
REDACTED
	return nil
REDACTED

func validateSelectedCreateOrderAmountCurrency(payAmount string, sel *payment.InstanceSelection) error {
	if sel == nil {
		return nil
REDACTED
	currency := paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	if _, err := payment.AmountToMinorUnit(payAmount, currency); err != nil {
		return infraerrors.BadRequest("INVALID_AMOUNT", err.Error()).
			WithMetadata(map[string]string{"currency": currencyREDACTED)
REDACTED
	return nil
REDACTED

func requiresWeChatJSAPICompatibleSelection(req CreateOrderRequest, sel *payment.InstanceSelection) bool {
	if sel == nil || sel.ProviderKey != payment.TypeWxpay || payment.GetBasePaymentType(req.PaymentType) != payment.TypeWxpay {
		return false
REDACTED
	return req.IsWeChatBrowser || strings.TrimSpace(req.OpenID) != ""
REDACTED

func (s *PaymentService) getWeChatPaymentOAuthCredential(ctx context.Context) (string, string, error) {
	if s == nil || s.configService == nil || s.configService.settingRepo == nil {
		return "", "", infraerrors.ServiceUnavailable(
			"WECHAT_PAYMENT_MP_NOT_CONFIGURED",
			"wechat in-app payment requires a complete WeChat MP OAuth credential",
		)
REDACTED
	cfg, err := (&SettingService{settingRepo: s.configService.settingRepoREDACTED).GetWeChatConnectOAuthConfig(ctx)
	appID := strings.TrimSpace(cfg.AppIDForMode("mp"))
	appSecret := strings.TrimSpace(cfg.AppSecretForMode("mp"))
	if err != nil || !cfg.SupportsMode("mp") || appID == "" || appSecret == "" {
		return "", "", infraerrors.ServiceUnavailable(
			"WECHAT_PAYMENT_MP_NOT_CONFIGURED",
			"wechat in-app payment requires a complete WeChat MP OAuth credential",
		)
REDACTED
	return appID, appSecret, nil
REDACTED

func classifyCreatePaymentError(req CreateOrderRequest, providerKey string, err error) error {
	if err == nil {
		return nil
REDACTED
	if providerKey == payment.TypeWxpay &&
		payment.GetBasePaymentType(req.PaymentType) == payment.TypeWxpay &&
		strings.Contains(err.Error(), "wxpay h5 payments are not authorized for this merchant") {
		return infraerrors.ServiceUnavailable(
			"WECHAT_H5_NOT_AUTHORIZED",
			"wechat h5 payment is not available for this merchant",
		).WithMetadata(map[string]string{
			"action": "open_in_wechat_or_scan_qr",
	REDACTED)
REDACTED
	return infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", fmt.Sprintf("payment gateway error: %s", err.Error()))
REDACTED

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
REDACTED
REDACTED

func buildWeChatPaymentOAuthStartURL(req CreateOrderRequest, scope string) (string, error) {
	u, err := url.Parse("/api/v1/auth/oauth/wechat/payment/start")
	if err != nil {
		return "", fmt.Errorf("build wechat payment oauth start url: %w", err)
REDACTED
	q := u.Query()
	q.Set("payment_type", strings.TrimSpace(req.PaymentType))
	if req.Amount > 0 {
		q.Set("amount", strconv.FormatFloat(req.Amount, 'f', -1, 64))
REDACTED
	if orderType := strings.TrimSpace(req.OrderType); orderType != "" {
		q.Set("order_type", orderType)
REDACTED
	if req.PlanID > 0 {
		q.Set("plan_id", strconv.FormatInt(req.PlanID, 10))
REDACTED
	if scope = strings.TrimSpace(scope); scope != "" {
		q.Set("scope", scope)
REDACTED
	if redirectTo := paymentRedirectPathFromURL(req.SrcURL); redirectTo != "" {
		q.Set("redirect", redirectTo)
REDACTED
	u.RawQuery = q.Encode()
	return u.String(), nil
REDACTED

func paymentRedirectPathFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "/purchase"
REDACTED
	if strings.HasPrefix(rawURL, "/") && !strings.HasPrefix(rawURL, "//") {
		return normalizePaymentRedirectPath(rawURL)
REDACTED
	u, err := url.Parse(rawURL)
	if err != nil {
		return "/purchase"
REDACTED
	path := strings.TrimSpace(u.EscapedPath())
	if path == "" {
		path = strings.TrimSpace(u.Path)
REDACTED
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "/purchase"
REDACTED
	if strings.TrimSpace(u.RawQuery) != "" {
		path += "?" + u.RawQuery
REDACTED
	return normalizePaymentRedirectPath(path)
REDACTED

func normalizePaymentRedirectPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/purchase"
REDACTED
	if path == "/payment" {
		return "/purchase"
REDACTED
	if strings.HasPrefix(path, "/payment?") {
		return "/purchase" + strings.TrimPrefix(path, "/payment")
REDACTED
	return path
REDACTED

// --- Order Queries ---

func (s *PaymentService) GetOrder(ctx context.Context, orderID, userID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
REDACTED
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
REDACTED
	return o, nil
REDACTED

func (s *PaymentService) GetOrderByID(ctx context.Context, orderID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
REDACTED
	return o, nil
REDACTED

func (s *PaymentService) GetUserOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID))
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
REDACTED
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
REDACTED
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
REDACTED
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count user orders: %w", err)
REDACTED
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query user orders: %w", err)
REDACTED
	return orders, total, nil
REDACTED

// AdminListOrders returns a paginated list of orders. If userID > 0, filters by user.
func (s *PaymentService) AdminListOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query()
	if userID > 0 {
		q = q.Where(paymentorder.UserIDEQ(userID))
REDACTED
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
REDACTED
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
REDACTED
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
REDACTED
	if p.Keyword != "" {
		q = q.Where(paymentorder.Or(
			paymentorder.OutTradeNoContainsFold(p.Keyword),
			paymentorder.UserEmailContainsFold(p.Keyword),
			paymentorder.UserNameContainsFold(p.Keyword),
		))
REDACTED
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin orders: %w", err)
REDACTED
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin orders: %w", err)
REDACTED
	return orders, total, nil
REDACTED
