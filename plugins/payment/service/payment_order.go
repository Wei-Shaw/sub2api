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
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment/provider"
)

// CreateOrderRequest is the input the handler passes to CreateOrder.
type CreateOrderRequest struct {
	UserID          int64
	Amount          decimal.Decimal
	PaymentType     string
	OpenID          string
	ClientIP        string
	IsMobile        bool
	IsWeChatBrowser bool
	SrcHost         string
	SrcURL          string
	ReturnURL       string
	PaymentSource   string
	OrderType       string
	PlanID          int64
}

// CreateOrderResponse is the result returned by CreateOrder.
type CreateOrderResponse struct {
	OrderID      int64                           `json:"order_id"`
	Amount       decimal.Decimal                 `json:"amount"`
	PayAmount    decimal.Decimal                 `json:"pay_amount"`
	FeeRate      decimal.Decimal                 `json:"fee_rate"`
	Status       string                          `json:"status"`
	ResultType   payment.CreatePaymentResultType `json:"result_type,omitempty"`
	PaymentType  string                          `json:"payment_type"`
	OutTradeNo   string                          `json:"out_trade_no,omitempty"`
	PayURL       string                          `json:"pay_url,omitempty"`
	QRCode       string                          `json:"qr_code,omitempty"`
	ClientSecret string                          `json:"client_secret,omitempty"`
	OAuth        *payment.WechatOAuthInfo        `json:"oauth,omitempty"`
	JSAPI        *payment.WechatJSAPIPayload     `json:"jsapi,omitempty"`
	JSAPIPayload *payment.WechatJSAPIPayload     `json:"jsapi_payload,omitempty"`
	ExpiresAt    time.Time                       `json:"expires_at"`
	PaymentMode  string                          `json:"payment_mode,omitempty"`
	ResumeToken  string                          `json:"resume_token,omitempty"`
}

// OrderListParams are the filters supplied to GetUserOrders / AdminListOrders.
type OrderListParams struct {
	Page        int
	PageSize    int
	Status      string
	OrderType   string
	PaymentType string
	Keyword     string
}

// errPaymentServiceUnavailable signals the plugin service was constructed
// without a working ent client / load balancer / config service. Handlers
// translate this to 503.
var errPaymentServiceUnavailable = errors.New("payment: service unavailable")

// outTradeNoMaxAttempts caps the random-suffix retry loop for out_trade_no
// generation. The window is large enough that collisions on the second
// attempt are vanishingly rare; the limit only exists so a buggy generator
// can't loop forever.
const outTradeNoMaxAttempts = 5

// CreateOrder builds a new payment order, asks the upstream provider to
// initiate payment and persists the response. The caller is the user-facing
// /payments handler; admin "manual" creation goes through a different path.
func (s *PaymentService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	if s == nil || s.entClient == nil || s.loadBalancer == nil || s.config == nil {
		return nil, errPaymentServiceUnavailable
	}
	req.normalize()
	cfg, err := s.config.GetPaymentConfig(ctx)
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
	user, err := s.lookupOrderUser(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	pricing := computeOrderPricing(req, plan, cfg)
	sel, err := s.selectCreateOrderInstance(ctx, req, cfg, pricing.PayAmount)
	if err != nil {
		return nil, err
	}
	order, err := s.persistOrder(ctx, req, user, plan, cfg, pricing, sel)
	if err != nil {
		return nil, err
	}
	resp, err := s.invokeProvider(ctx, order, req, cfg, pricing, plan, sel)
	if err != nil {
		_, _ = s.entClient.PaymentOrder.UpdateOneID(order.ID).
			SetStatus(OrderStatusFailed).Save(ctx)
		return nil, err
	}
	s.publishOrderCreated(ctx, order)
	return resp, nil
}

// normalize fills in zero-value request fields with defaults that callers
// commonly omit. Keeps the CreateOrder body free of defaulting noise.
func (r *CreateOrderRequest) normalize() {
	if r.OrderType == "" {
		r.OrderType = payment.OrderTypeBalance
	}
	if normalized := NormalizeVisibleMethod(r.PaymentType); normalized != "" {
		r.PaymentType = normalized
	}
}

// lookupOrderUser fetches the buyer through the host SDK. Returns 503
// when the host has no user-lookup capability granted, 403 when the
// user is not active.
func (s *PaymentService) lookupOrderUser(ctx context.Context, userID int64) (*pluginsdk.HostUserInfo, error) {
	user, err := s.host.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pluginsdk.ErrHostUserLookupUnavailable) {
			return nil, infraerrors.ServiceUnavailable("USER_LOOKUP_UNAVAILABLE", "user lookup is not available")
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user == nil || !user.Found {
		return nil, infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	}
	return user, nil
}

// orderPricing bundles the four currency values derived from the
// request, plan and config so each one is computed exactly once. All
// amounts are decimal — PayAmountStr is kept alongside as the formatted
// string we hand to the upstream gateway (most provider SDKs prefer
// strings over native floats).
type orderPricing struct {
	OrderAmount  decimal.Decimal
	LimitAmount  decimal.Decimal
	PayAmount    decimal.Decimal
	PayAmountStr string
	FeeRate      decimal.Decimal
}

// computeOrderPricing applies the plan/balance multiplier rules and
// computes the gateway-facing pay amount. Subscription orders take
// their price from the plan; balance orders may be multiplied by a
// configurable bonus rate.
func computeOrderPricing(req CreateOrderRequest, plan *SubscriptionPlan, cfg *PaymentConfig) orderPricing {
	orderAmount := req.Amount
	limitAmount := req.Amount
	if plan != nil {
		orderAmount = plan.Price
		limitAmount = plan.Price
	} else if req.OrderType == payment.OrderTypeBalance {
		orderAmount = calculateCreditedBalance(req.Amount, cfg.BalanceRechargeMultiplier)
	}
	payAmount := payment.CalculatePayAmount(limitAmount, cfg.RechargeFeeRate)
	return orderPricing{
		OrderAmount:  orderAmount,
		LimitAmount:  limitAmount,
		PayAmount:    payAmount,
		PayAmountStr: payAmount.StringFixed(2),
		FeeRate:      cfg.RechargeFeeRate,
	}
}

// publishOrderCreated emits the payment.order.created plugin event. Best
// effort: failures are logged at warn level but never fail the order.
func (s *PaymentService) publishOrderCreated(ctx context.Context, o *pluginent.PaymentOrder) {
	if s == nil || s.events == nil || o == nil {
		return
	}
	planIDStr := ""
	if o.PlanID != nil {
		planIDStr = strconv.FormatInt(*o.PlanID, 10)
	}
	ev := &pluginsdk.HostEvent{
		EventType: pluginsdk.EventTypePaymentOrderCreated,
		Payload: &pb.HostEvent_PaymentOrderCreated{
			PaymentOrderCreated: &pluginsdk.PaymentOrderCreated{
				OrderId:           int64(o.ID),
				OutTradeNo:        o.OutTradeNo,
				UserId:            o.UserID,
				AmountCents:       yuanToFen(o.Amount),
				PlanId:            planIDStr,
				ProviderKey:       o.PaymentType,
				BizType:           o.OrderType,
				CreatedAtUnixNano: o.CreatedAt.UnixNano(),
			},
		},
	}
	if err := s.events.Publish(ctx, ev); err != nil && s.logger != nil {
		s.logger.Warn("publish payment.order.created failed", "order_id", o.ID, "error", err)
	}
}

// invokeProvider calls into the upstream payment provider, persists the
// returned trade-no / pay-url / qr-code on the order, and assembles the
// CreateOrderResponse.
func (s *PaymentService) invokeProvider(
	ctx context.Context,
	order *pluginent.PaymentOrder,
	req CreateOrderRequest,
	cfg *PaymentConfig,
	pricing orderPricing,
	plan *SubscriptionPlan,
	sel *payment.InstanceSelection,
) (*CreateOrderResponse, error) {
	prov, err := provider.CreateProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		return nil, classifyCreateProviderError(sel, err)
	}
	subject := buildPaymentSubject(plan, pricing.LimitAmount, cfg)
	canonical, err := CanonicalizeReturnURL(req.ReturnURL, req.SrcHost, req.SrcURL)
	if err != nil {
		return nil, err
	}
	resumeToken, err := s.maybeIssueResumeToken(ctx, order, sel, req, canonical)
	if err != nil {
		return nil, err
	}
	providerReturnURL, err := buildPaymentReturnURL(canonical, int64(order.ID), order.OutTradeNo, resumeToken)
	if err != nil {
		return nil, err
	}
	pr, err := prov.CreatePayment(ctx, buildProviderCreatePaymentRequest(req, sel, order.OutTradeNo, pricing.PayAmountStr, subject, providerReturnURL))
	if err != nil {
		return nil, classifyCreatePaymentError(req, sel.ProviderKey, err)
	}
	if err := s.persistProviderResult(ctx, order, sel, pr); err != nil {
		return nil, err
	}
	s.writeAuditLog(ctx, int64(order.ID), "ORDER_CREATED", fmt.Sprintf("user:%d", req.UserID), map[string]any{
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
	resp := buildCreateOrderResponse(order, req, pricing.PayAmount, sel, pr, resultType)
	resp.ResumeToken = resumeToken
	return resp, nil
}

// maybeIssueResumeToken mints a resume token when a canonical return URL
// was supplied and the resume signing key is configured. Failures other
// than "no key configured" are surfaced; missing keys collapse to no token.
func (s *PaymentService) maybeIssueResumeToken(
	ctx context.Context,
	order *pluginent.PaymentOrder,
	sel *payment.InstanceSelection,
	req CreateOrderRequest,
	canonicalReturnURL string,
) (string, error) {
	resume := s.Resume()
	if resume == nil || canonicalReturnURL == "" {
		return "", nil
	}
	token, err := resume.CreateToken(ctx, ResumeTokenClaims{
		OrderID:            int64(order.ID),
		UserID:             order.UserID,
		ProviderInstanceID: sel.InstanceID,
		ProviderKey:        sel.ProviderKey,
		PaymentType:        req.PaymentType,
		CanonicalReturnURL: canonicalReturnURL,
	})
	if err != nil {
		// Fall back silently when the host has not configured the signing
		// key — resume tokens are an optional feature.
		if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) && appErr.Status.Reason == paymentResumeNotConfiguredCode {
			return "", nil
		}
		return "", fmt.Errorf("create payment resume token: %w", err)
	}
	return token, nil
}

func (s *PaymentService) persistProviderResult(
	ctx context.Context,
	order *pluginent.PaymentOrder,
	sel *payment.InstanceSelection,
	pr *payment.CreatePaymentResponse,
) error {
	_, err := s.entClient.PaymentOrder.UpdateOneID(order.ID).
		SetNillablePaymentTradeNo(psNilIfEmpty(pr.TradeNo)).
		SetNillablePayURL(psNilIfEmpty(pr.PayURL)).
		SetNillableQrCode(psNilIfEmpty(pr.QRCode)).
		SetNillableProviderInstanceID(psNilIfEmpty(sel.InstanceID)).
		SetNillableProviderKey(psNilIfEmpty(sel.ProviderKey)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update order with payment details: %w", err)
	}
	return nil
}

func buildProviderCreatePaymentRequest(req CreateOrderRequest, sel *payment.InstanceSelection, outTradeNo, payAmount, subject, providerReturnURL string) payment.CreatePaymentRequest {
	return payment.CreatePaymentRequest{
		OrderID:            outTradeNo,
		Amount:             payAmount,
		PaymentType:        req.PaymentType,
		Subject:            subject,
		ReturnURL:          providerReturnURL,
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

func buildCreateOrderResponse(order *pluginent.PaymentOrder, req CreateOrderRequest, payAmount decimal.Decimal, sel *payment.InstanceSelection, pr *payment.CreatePaymentResponse, resultType payment.CreatePaymentResultType) *CreateOrderResponse {
	return &CreateOrderResponse{
		OrderID:      int64(order.ID),
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
		OAuth:        pr.OAuth,
		JSAPI:        pr.JSAPI,
		JSAPIPayload: pr.JSAPI,
		ExpiresAt:    order.ExpiresAt,
		PaymentMode:  sel.PaymentMode,
	}
}

func classifyCreateProviderError(sel *payment.InstanceSelection, err error) error {
	if err == nil {
		return nil
	}
	if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) {
		md := map[string]string{"provider": sel.ProviderKey, "instance_id": sel.InstanceID}
		for k, v := range appErr.Metadata {
			md[k] = v
		}
		return appErr.WithMetadata(md)
	}
	return infraerrors.ServiceUnavailable("PAYMENT_PROVIDER_MISCONFIGURED", "provider_misconfigured").
		WithMetadata(map[string]string{"provider": sel.ProviderKey, "instance_id": sel.InstanceID})
}

func classifyCreatePaymentError(req CreateOrderRequest, providerKey string, err error) error {
	if err == nil {
		return nil
	}
	if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) {
		return appErr
	}
	if providerKey == payment.TypeWxpay &&
		payment.GetBasePaymentType(req.PaymentType) == payment.TypeWxpay &&
		strings.Contains(err.Error(), "wxpay h5 payments are not authorized for this merchant") {
		return infraerrors.ServiceUnavailable(
			"WECHAT_H5_NOT_AUTHORIZED",
			"wechat h5 payment is not available for this merchant",
		).WithMetadata(map[string]string{"action": "open_in_wechat_or_scan_qr"})
	}
	return infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", fmt.Sprintf("payment gateway error: %s", err.Error()))
}

func buildPaymentSubject(plan *SubscriptionPlan, limitAmount decimal.Decimal, cfg *PaymentConfig) string {
	if plan != nil {
		if plan.ProductName != "" {
			return plan.ProductName
		}
		return "Sub2API Subscription " + plan.Name
	}
	amountStr := limitAmount.StringFixed(2)
	pf := strings.TrimSpace(cfg.ProductNamePrefix)
	sf := strings.TrimSpace(cfg.ProductNameSuffix)
	if pf != "" || sf != "" {
		return strings.TrimSpace(pf + " " + amountStr + " " + sf)
	}
	return "Sub2API " + amountStr + " CNY"
}
