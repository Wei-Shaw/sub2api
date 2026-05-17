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
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/userdevice"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// --- Order Creation ---

func (s *PaymentService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
	}
	if normalized := NormalizeVisibleMethod(req.PaymentType); normalized != "" {
		req.PaymentType = normalized
	}
	if err := s.applyPaymentQuoteToCreateOrder(&req); err != nil {
		return nil, err
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}
	if err := s.resolveRequestPaymentCurrency(ctx, &req, cfg); err != nil {
		return nil, err
	}
	plan, err := s.validateOrderInput(ctx, req, cfg)
	if err != nil {
		return nil, err
	}
	amounts, err := computeCreateOrderAmounts(req, cfg, plan, time.Now())
	if err != nil {
		return nil, err
	}
	if err := validateLedgerAmountLimits(req.OrderType, amounts.LimitLedgerAmount, cfg); err != nil {
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
	ledgerAmount := amounts.LedgerAmount
	paymentAmount := amounts.PaymentAmount
	feeRate := cfg.RechargeFeeRate
	paymentCurrency := amounts.FXSnapshot.PaymentCurrency
	payAmountStr, payAmount, err := calculateCreateOrderPayAmount(paymentAmount, feeRate, paymentCurrency)
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
	if err := validateProviderCurrency(sel.ProviderKey, req.PaymentType, paymentCurrency, sel.Config, cfg.CurrencyCapabilities); err != nil {
		return nil, err
	}
	if err := validateSelectedCreateOrderAmountCurrency(payAmountStr, sel); err != nil {
		return nil, err
	}
	oauthResp, err := s.maybeBuildWeChatOAuthRequiredResponseForSelection(ctx, req, ledgerAmount, payAmount, feeRate, sel)
	if err != nil {
		return nil, err
	}
	if oauthResp != nil {
		return oauthResp, nil
	}
	order, err := s.createOrderInTx(ctx, req, user, plan, cfg, amounts, ledgerAmount, paymentAmount, feeRate, payAmount, sel)
	if err != nil {
		return nil, err
	}
	resp, err := s.invokeProvider(ctx, order, req, cfg, ledgerAmount, payAmountStr, payAmount, plan, sel)
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

func (s *PaymentService) createOrderInTx(ctx context.Context, req CreateOrderRequest, user *User, plan *dbent.SubscriptionPlan, cfg *PaymentConfig, amounts createOrderAmounts, ledgerAmount, paymentAmount, feeRate, payAmount float64, sel *payment.InstanceSelection) (*dbent.PaymentOrder, error) {
	snapshot := amounts.FXSnapshot
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.checkPendingLimit(ctx, tx, req.UserID, cfg.MaxPendingOrders); err != nil {
		return nil, err
	}
	if err := s.checkDailyLimit(ctx, tx, req.UserID, ledgerAmount, cfg.DailyLimit); err != nil {
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
	paymentCurrency := snapshot.PaymentCurrency
	if paymentCurrency == "" {
		paymentCurrency = normalizeCurrencyCode(req.PaymentCurrency, cfg.LedgerCurrency)
	}
	ledgerCurrency := snapshot.LedgerCurrency
	if ledgerCurrency == "" {
		ledgerCurrency = normalizeCurrencyCode(cfg.LedgerCurrency, defaultLedgerCurrency)
	}
	providerSnapshot := buildPaymentOrderProviderSnapshot(sel, req)
	if amounts.BalancePackage != nil {
		providerSnapshot = withBalancePackageProviderSnapshot(providerSnapshot, *amounts.BalancePackage)
	}
	selectedInstanceID := ""
	selectedProviderKey := ""
	if sel != nil {
		selectedInstanceID = strings.TrimSpace(sel.InstanceID)
		selectedProviderKey = strings.TrimSpace(sel.ProviderKey)
	}
	b := tx.PaymentOrder.Create().
		SetUserID(req.UserID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetNillableUserNotes(psNilIfEmpty(user.Notes)).
		SetAmount(ledgerAmount).
		SetPayAmount(payAmount).
		SetPaymentCurrency(paymentCurrency).
		SetPaymentAmount(paymentAmount).
		SetLedgerCurrency(ledgerCurrency).
		SetLedgerAmount(ledgerAmount).
		SetFxRatePaymentToLedger(snapshot.RatePaymentToLedger).
		SetNillableFxSource(psNilIfEmpty(snapshot.Source)).
		SetNillableFxTimestamp(&snapshot.Timestamp).
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
	if amounts.BalancePackage != nil && amounts.BalancePackage.BalanceGroupID != nil && *amounts.BalancePackage.BalanceGroupID > 0 {
		b.SetBalanceGroupID(*amounts.BalancePackage.BalanceGroupID)
	}
	if amounts.BalancePackage != nil && amounts.BalancePackage.ActualCredits > 0 {
		b.SetActualCredits(amounts.BalancePackage.ActualCredits)
	}
	if plan != nil {
		b.SetPlanID(plan.ID).SetSubscriptionGroupID(plan.GroupID).SetSubscriptionDays(psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit))
		// For subscription orders, compute actual_credits from ledger_amount using the
		// subscription group's rate_multiplier and token_price_per_million.
		if amounts.BalancePackage == nil || amounts.BalancePackage.ActualCredits <= 0 {
			if subGroup, err := s.groupRepo.GetByID(ctx, plan.GroupID); err == nil && subGroup != nil && subGroup.RateMultiplier > 0 {
				var tokenPrice float64
				if subGroup.TokenPricePerMillion != nil && *subGroup.TokenPricePerMillion > 0 {
					tokenPrice = *subGroup.TokenPricePerMillion
				}
				if tokenPrice > 0 {
					subCredits := computeActualCreditsFromRateMultiplier(ledgerAmount, subGroup.RateMultiplier, tokenPrice)
					if subCredits > 0 {
						b.SetActualCredits(subCredits)
					}
				}
			}
		}
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
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit order transaction: %w", err)
	}
	return order, nil
}

func withBalancePackageProviderSnapshot(snapshot map[string]any, pkg BalanceRechargePackage) map[string]any {
	if snapshot == nil {
		snapshot = map[string]any{"schema_version": 2}
	}
	snapshot["balance_package"] = map[string]any{
		"id":               pkg.ID,
		"label":            pkg.Label,
		"amount_ledger":    pkg.AmountLedger,
		"actual_credits":   pkg.ActualCredits,
		"credit_unit":      pkg.CreditUnit,
		"balance_group_id": pkg.BalanceGroupID,
		"group_id":         pkg.BalanceGroupID,
	}
	return snapshot
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
		snapshot["currency"] = "CNY" // WxPay always uses CNY
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

func (s *PaymentService) checkDailyLimit(ctx context.Context, tx *dbent.Tx, userID int64, amount, limit float64) error {
	if limit <= 0 {
		return nil
	}
	ts := psStartOfDayUTC(time.Now())
	orders, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusIn(OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted), paymentorder.PaidAtGTE(ts)).All(ctx)
	if err != nil {
		return fmt.Errorf("query daily usage: %w", err)
	}
	var used float64
	for _, o := range orders {
		used += dailyLimitLedgerAmountForOrder(o)
	}
	if used+amount > limit {
		return infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", "daily_limit_exceeded").
			WithMetadata(map[string]string{"remaining": fmt.Sprintf("%.2f", math.Max(0, limit-used))})
	}
	return nil
}

func dailyLimitLedgerAmountForOrder(o *dbent.PaymentOrder) float64 {
	if o == nil {
		return 0
	}
	if o.LedgerAmount > 0 {
		return o.LedgerAmount
	}
	return o.Amount
}

func (s *PaymentService) selectCreateOrderInstance(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig, payAmount float64) (*payment.InstanceSelection, error) {
	selectCtx, err := s.prepareCreateOrderSelectionContext(ctx, req, cfg)
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

func (s *PaymentService) prepareCreateOrderSelectionContext(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig) (context.Context, error) {
	ctx = selectionContextWithPaymentCurrency(ctx, req.PaymentCurrency, cfg)
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
	if isManualPaymentSelection(sel) {
		pr, err := buildManualCreatePaymentResponse(sel)
		if err != nil {
			return nil, err
		}
		_, err = s.entClient.PaymentOrder.UpdateOneID(order.ID).
			SetNillablePayURL(psNilIfEmpty(pr.PayURL)).
			SetNillableQrCode(psNilIfEmpty(pr.QRCode)).
			SetNillableQrCodeImg(psNilIfEmpty(pr.QRCodeImg)).
			SetNillableProviderInstanceID(psNilIfEmpty(sel.InstanceID)).
			SetNillableProviderKey(psNilIfEmpty(sel.ProviderKey)).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("update manual order with payment details: %w", err)
		}
		s.writeAuditLog(ctx, order.ID, "ORDER_CREATED", fmt.Sprintf("user:%d", req.UserID), map[string]any{
			"paymentAmount":  req.Amount,
			"creditedAmount": order.Amount,
			"payAmount":      order.PayAmount,
			"paymentType":    req.PaymentType,
			"orderType":      req.OrderType,
			"paymentSource":  NormalizePaymentSource(req.PaymentSource),
			"paymentMode":    paymentModeManual,
		})
		resp := buildCreateOrderResponse(order, req, payAmount, sel, pr, payment.CreatePaymentResultOrderCreated)
		return resp, nil
	}
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
	returnURL := req.ReturnURL
	firstPartyFrontendURL := ""
	if sel.ProviderKey == payment.TypePaddle {
		firstPartyFrontendURL, err = s.resolveFirstPartyFrontendURL(ctx, req.ReturnURL)
		if err != nil {
			return nil, err
		}
		returnURL, err = buildFirstPartyPaymentResultURL(firstPartyFrontendURL)
		if err != nil {
			return nil, err
		}
	}
	canonicalReturnURL, err := CanonicalizeReturnURL(returnURL, req.SrcHost, req.SrcURL)
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
	}, sel, order, subject)
	pr, err := prov.CreatePayment(ctx, providerReq)
	if err != nil {
		slog.Error("[PaymentService] CreatePayment failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) {
			return nil, appErr
		}
		return nil, classifyCreatePaymentError(req, sel.ProviderKey, err)
	}
	_, err = s.entClient.PaymentOrder.UpdateOneID(order.ID).
		SetNillablePaymentTradeNo(psNilIfEmpty(pr.TradeNo)).
		SetNillablePayURL(psNilIfEmpty(pr.PayURL)).
		SetNillableQrCode(psNilIfEmpty(pr.QRCode)).
		SetNillableQrCodeImg(psNilIfEmpty(pr.QRCodeImg)).
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
	if sel.ProviderKey == payment.TypePaddle {
		checkoutURL, err := buildFirstPartyCheckoutURL(firstPartyFrontendURL, order, firstNonEmptyPaymentString(pr.CheckoutID, pr.TradeNo), resumeToken)
		if err != nil {
			return nil, err
		}
		pr.CheckoutURL = checkoutURL
		pr.PayURL = ""
	}
	resp := buildCreateOrderResponse(order, req, payAmount, sel, pr, resultType)
	resp.ResumeToken = resumeToken
	return resp, nil
}

func (s *PaymentService) resolveFirstPartyFrontendURL(ctx context.Context, fallbackReturnURL string) (string, error) {
	frontendURL := ""
	if s != nil && s.configService != nil {
		frontendURL = s.configService.GetFrontendURL(ctx)
	}
	if frontendURL == "" {
		frontendURL = originFromHTTPSURL(fallbackReturnURL)
	}
	if frontendURL == "" {
		return "", infraerrors.ServiceUnavailable("PAYMENT_FRONTEND_URL_REQUIRED", "paddle first-party checkout requires a configured https frontend_url")
	}
	return frontendURL, nil
}

func buildFirstPartyPaymentResultURL(frontendURL string) (string, error) {
	base, err := parseFirstPartyFrontendURL(frontendURL)
	if err != nil {
		return "", err
	}
	base.Path = joinURLPath(base.Path, "/payment/result")
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

func buildFirstPartyCheckoutURL(frontendURL string, order *dbent.PaymentOrder, checkoutID string, resumeToken string) (string, error) {
	if order == nil {
		return "", fmt.Errorf("build first-party checkout url: order is nil")
	}
	checkoutID = strings.TrimSpace(checkoutID)
	if checkoutID == "" {
		return "", infraerrors.ServiceUnavailable("PADDLE_CHECKOUT_ID_REQUIRED", "paddle first-party checkout requires a transaction checkout id")
	}
	base, err := parseFirstPartyFrontendURL(frontendURL)
	if err != nil {
		return "", err
	}
	base.Path = joinURLPath(base.Path, "/checkout")
	query := base.Query()
	query.Set("provider", payment.TypePaddle)
	query.Set("checkout_id", checkoutID)
	query.Set("order_id", strconv.FormatInt(order.ID, 10))
	query.Set("out_trade_no", order.OutTradeNo)
	if strings.TrimSpace(resumeToken) != "" {
		query.Set("resume_token", strings.TrimSpace(resumeToken))
	}
	if !order.ExpiresAt.IsZero() {
		query.Set("expires_at", order.ExpiresAt.UTC().Format(time.RFC3339))
	}
	base.RawQuery = query.Encode()
	base.Fragment = ""
	return base.String(), nil
}

func parseFirstPartyFrontendURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_FRONTEND_URL_INVALID", "payment frontend_url must be an absolute https URL")
	}
	if parsed.Scheme != "https" {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_FRONTEND_URL_INVALID", "payment frontend_url must use https")
	}
	return parsed, nil
}

func originFromHTTPSURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func joinURLPath(basePath string, leaf string) string {
	basePath = strings.TrimRight(strings.TrimSpace(basePath), "/")
	leaf = "/" + strings.TrimLeft(strings.TrimSpace(leaf), "/")
	if basePath == "" || basePath == "/" {
		return leaf
	}
	return basePath + leaf
}

func firstNonEmptyPaymentString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func buildProviderCreatePaymentRequest(req CreateOrderRequest, sel *payment.InstanceSelection, order *dbent.PaymentOrder, subject string) payment.CreatePaymentRequest {
	paymentCurrency := normalizeCurrencyCode(order.PaymentCurrency, defaultLedgerCurrency)
	ledgerCurrency := normalizeCurrencyCode(order.LedgerCurrency, defaultLedgerCurrency)
	return payment.CreatePaymentRequest{
		OrderID:            order.OutTradeNo,
		Amount:             formatCurrencyAmountForProvider(order.PayAmount, paymentCurrency),
		PaymentCurrency:    paymentCurrency,
		LedgerCurrency:     ledgerCurrency,
		LedgerAmount:       formatCurrencyAmountForProvider(order.LedgerAmount, ledgerCurrency),
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
		if plan.ProductName != "" {
			return plan.ProductName
		}
		return "Sub2API Subscription " + plan.Name
	}
	if sel != nil && strings.TrimSpace(sel.ProviderKey) == payment.TypePaddle {
		return "VClaw Credit"
	}
	currency := payment.DefaultPaymentCurrency
	if sel != nil {
		currency = paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	}
	amountStr := payment.FormatAmountForCurrency(limitAmount, currency)
	pf := strings.TrimSpace(cfg.ProductNamePrefix)
	sf := strings.TrimSpace(cfg.ProductNameSuffix)
	if pf != "" || sf != "" {
		return strings.TrimSpace(pf + " " + amountStr + " " + sf)
	}
	return "Sub2API " + amountStr + " " + currency
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
	resp := &CreateOrderResponse{
		OrderID:         order.ID,
		Amount:          order.Amount,
		PaymentAmount:   order.PaymentAmount,
		PaymentCurrency: normalizeCurrencyCode(order.PaymentCurrency, defaultLedgerCurrency),
		LedgerAmount:    order.LedgerAmount,
		LedgerCurrency:  normalizeCurrencyCode(order.LedgerCurrency, defaultLedgerCurrency),
		FXRate:          order.FxRatePaymentToLedger,
		FXSource:        psStringValue(order.FxSource),
		PayAmount:       payAmount,
		FeeRate:         order.FeeRate,
		Status:          OrderStatusPending,
		ResultType:      resultType,
		PaymentType:     req.PaymentType,
		OutTradeNo:      order.OutTradeNo,
		PayURL:          pr.PayURL,
		CheckoutURL:     pr.CheckoutURL,
		QRCode:          pr.QRCode,
		QRCodeImg:       pr.QRCodeImg,
		ClientSecret:    pr.ClientSecret,
		IntentID:        pr.IntentID,
		Currency:        pr.Currency,
		CountryCode:     pr.CountryCode,
		PaymentEnv:      pr.PaymentEnv,
		CheckoutID:      pr.CheckoutID,
		OAuth:           pr.OAuth,
		JSAPI:           pr.JSAPI,
		JSAPIPayload:    pr.JSAPI,
		ExpiresAt:       order.ExpiresAt,
		PaymentMode:     sel.PaymentMode,
	}
	if order.FxTimestamp != nil {
		resp.FXTimestamp = *order.FxTimestamp
	}
	return resp
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
	if amountMode := strings.TrimSpace(req.AmountMode); amountMode != "" {
		q.Set("amount_mode", amountMode)
	}
	if quoteID := strings.TrimSpace(req.QuoteID); quoteID != "" {
		q.Set("quote_id", quoteID)
	}
	if paymentCurrency := normalizeCurrencyCode(req.PaymentCurrency, ""); paymentCurrency != "" {
		q.Set("payment_currency", paymentCurrency)
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
	if p.UserIDs != nil {
		if len(p.UserIDs) == 0 {
			q = q.Where(paymentorder.UserIDEQ(0))
		} else {
			q = q.Where(paymentorder.UserIDIn(p.UserIDs...))
		}
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
			paymentorder.HasUserWith(dbuser.HasDevicesWith(userdevice.DeviceCodeContainsFold(p.Keyword))),
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

// GetDeviceCodesByUserIDs returns a map of userID -> device_code for the given user IDs.
// It picks the primary device (most recently logged in) for each user.
func (s *PaymentService) GetDeviceCodesByUserIDs(ctx context.Context, userIDs []int64) map[int64]string {
	return LookupDeviceCodesByUserIDs(ctx, s.entClient, userIDs)
}
