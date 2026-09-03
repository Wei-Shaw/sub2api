package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	sepay "github.com/emizuki/sepay-go-sdk"
	"github.com/shopspring/decimal"
)

// sepayHTTPTimeout 限制对 SePay Open API 的单次调用时长。
// SDK 默认使用零值 http.Client（无超时），webhook 与建单路径都不能被上游拖死。
const sepayHTTPTimeout = 20 * time.Second

// sepayEnvSandbox / sepayEnvProduction 是实例配置里 env 字段允许的取值。
const (
	sepayEnvSandbox    = "sandbox"
	sepayEnvProduction = "production"
)

// SePay 实现 payment.CancelableProvider，接入 SePay（越南）收银台。
//
// 与被它替换掉的服务商不同，SePay 收银台不是一个可以直接跳转的 GET 链接：
// 商户需要把一组带 HMAC-SHA256 签名的字段用 POST 表单提交到 pay.sepay.vn。
// 因此 CreatePayment 返回的是 FormAction 与 FormFields，由服务端的自动提交
// 页面负责真正发起跳转，见 handler.PaymentCheckoutHandler。
type SePay struct {
	instanceID string
	config     map[string]string
	client     *sepay.Client
}

// NewSePay 用实例配置构造 SePay 服务商。
//
// 配置键：merchantId、secretKey（密钥）、env（sandbox / production）、
// currency（默认 VND）。
func NewSePay(instanceID string, config map[string]string) (*SePay, error) {
	cfg := cloneStringMap(config)

	merchantID := strings.TrimSpace(cfg["merchantId"])
	if merchantID == "" {
		return nil, infraerrors.BadRequest("SEPAY_CONFIG_MISSING_KEY",
			"sepay config missing required key: merchantId").
			WithMetadata(map[string]string{"field": "merchantId"})
	}
	secretKey := strings.TrimSpace(cfg["secretKey"])
	if secretKey == "" {
		return nil, infraerrors.BadRequest("SEPAY_CONFIG_MISSING_KEY",
			"sepay config missing required key: secretKey").
			WithMetadata(map[string]string{"field": "secretKey"})
	}

	env, err := normalizeSePayEnv(cfg["env"])
	if err != nil {
		return nil, err
	}
	cfg["env"] = env

	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, infraerrors.BadRequest("SEPAY_CONFIG_INVALID_CURRENCY",
			fmt.Sprintf("sepay config currency: %v", err))
	}
	cfg["currency"] = currency

	sdkEnv := sepay.Production
	if env == sepayEnvSandbox {
		sdkEnv = sepay.Sandbox
	}
	client, err := sepay.NewClient(sepay.Config{
		Env:        sdkEnv,
		MerchantID: merchantID,
		SecretKey:  secretKey,
	})
	if err != nil {
		return nil, infraerrors.BadRequest("SEPAY_CONFIG_INVALID", err.Error())
	}
	client.SetHTTPClient(&http.Client{Timeout: sepayHTTPTimeout})

	return &SePay{instanceID: instanceID, config: cfg, client: client}, nil
}

func normalizeSePayEnv(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", sepayEnvProduction, "prod", "live":
		return sepayEnvProduction, nil
	case sepayEnvSandbox, "test", "demo":
		return sepayEnvSandbox, nil
	default:
		return "", infraerrors.BadRequest("SEPAY_CONFIG_INVALID_ENV",
			"sepay config env must be either sandbox or production").
			WithMetadata(map[string]string{"field": "env"})
	}
}

func (s *SePay) Name() string        { return "SePay" }
func (s *SePay) ProviderKey() string { return payment.TypeSePay }

func (s *SePay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{
		payment.TypeSePayBankTransfer,
		payment.TypeSePayNapas,
		payment.TypeSePayCard,
	}
}

func (s *SePay) merchantID() string { return strings.TrimSpace(s.config["merchantId"]) }
func (s *SePay) secretKey() string  { return strings.TrimSpace(s.config["secretKey"]) }

func (s *SePay) currency() string {
	currency, err := payment.NormalizePaymentCurrency(s.config["currency"])
	if err != nil {
		return payment.DefaultPaymentCurrency
	}
	return currency
}

func (s *SePay) env() string {
	if strings.EqualFold(strings.TrimSpace(s.config["env"]), sepayEnvSandbox) {
		return sepayEnvSandbox
	}
	return sepayEnvProduction
}

// MerchantIdentityMetadata 暴露订单快照要比对的非敏感商户身份字段。
func (s *SePay) MerchantIdentityMetadata() map[string]string {
	if s == nil {
		return nil
	}
	return map[string]string{
		"merchant_id": s.merchantID(),
		"currency":    s.currency(),
		"env":         s.env(),
	}
}

// sepayMethodForPaymentType 把面板的支付方式映射到 SePay 的 payment_method。
func sepayMethodForPaymentType(paymentType string) (sepay.PaymentMethod, error) {
	switch strings.TrimSpace(paymentType) {
	case payment.TypeSePayBankTransfer, payment.TypeSePay, "":
		return sepay.BankTransfer, nil
	case payment.TypeSePayNapas:
		return sepay.NapasBankTransfer, nil
	case payment.TypeSePayCard:
		return sepay.Card, nil
	default:
		return "", infraerrors.BadRequest("SEPAY_UNSUPPORTED_PAYMENT_TYPE",
			fmt.Sprintf("sepay does not support payment type: %s", paymentType)).
			WithMetadata(map[string]string{"payment_type": paymentType})
	}
}

// CreatePayment 生成一份带签名的收银台表单。
//
// SePay 的 checkout 初始化完全是本地签名，不会产生任何上游调用，所以这个方法
// 是幂等且可重放的——自动提交页面在用户重新打开支付链接时会再算一次同样的表单。
func (s *SePay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	_ = ctx

	method, err := sepayMethodForPaymentType(req.PaymentType)
	if err != nil {
		return nil, err
	}

	currency := s.currency()
	amount, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	if err != nil {
		return nil, fmt.Errorf("sepay create payment: invalid amount %q: %w", req.Amount, err)
	}
	// 先按币种精度校验金额，越南盾等零小数币种不接受小数位。
	if _, err := payment.AmountToMinorUnit(req.Amount, currency); err != nil {
		return nil, fmt.Errorf("sepay create payment: %w", err)
	}

	fields := sepay.OnetimePaymentFields{
		Operation:          sepay.OperationPurchase,
		PaymentMethod:      method,
		OrderInvoiceNumber: req.OrderID,
		OrderAmount:        amount.InexactFloat64(),
		Currency:           currency,
		OrderDescription:   req.Subject,
	}
	if returnURL := strings.TrimSpace(req.ReturnURL); returnURL != "" {
		fields.SuccessURL = sepay.String(returnURL)
		fields.ErrorURL = sepay.String(returnURL)
		fields.CancelURL = sepay.String(returnURL)
	}

	signed := s.client.Checkout.InitOneTimePaymentFields(fields)

	return &payment.CreatePaymentResponse{
		// SePay 在收银台完成前不会分配上游交易号，订单号就是我们的 out_trade_no。
		TradeNo:    req.OrderID,
		Currency:   currency,
		PaymentEnv: s.env(),
		ResultType: payment.CreatePaymentResultFormPost,
		FormAction: s.client.Checkout.InitCheckoutURL(),
		FormFields: signed.FormValues(),
	}, nil
}

// QueryOrder 按订单号（order_invoice_number，即 out_trade_no）查询上游状态。
func (s *SePay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, fmt.Errorf("sepay query order: trade number is required")
	}
	resp, err := s.client.Order.Retrieve(ctx, url.PathEscape(tradeNo))
	if err != nil {
		return nil, fmt.Errorf("sepay query order %s: %w", tradeNo, err)
	}
	order, err := parseSePayOrder(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sepay query order %s: %w", tradeNo, err)
	}
	return s.buildQueryResponse(tradeNo, order), nil
}

func (s *SePay) buildQueryResponse(tradeNo string, order *sepayOrder) *payment.QueryOrderResponse {
	upstreamTradeNo := strings.TrimSpace(order.OrderID)
	if upstreamTradeNo == "" {
		upstreamTradeNo = tradeNo
	}
	currency := strings.ToUpper(strings.TrimSpace(order.Currency))
	if currency == "" {
		currency = s.currency()
	}
	return &payment.QueryOrderResponse{
		TradeNo: upstreamTradeNo,
		Status:  sepayStatusToProviderStatus(order.Status()),
		Amount:  order.Amount(),
		PaidAt:  strings.TrimSpace(order.PaidAt),
		Metadata: map[string]string{
			"merchant_id":  s.merchantID(),
			"currency":     currency,
			"env":          s.env(),
			"order_status": order.Status(),
		},
	}
}

// VerifyNotification 校验并解析 SePay 的支付回调。
//
// 回调体本身只用来定位订单号：真正决定订单是否已付的是随后一次带 Basic 认证的
// 上游订单查询。这样即使伪造的回调命中了这个端点，也无法把订单推到已支付——
// 上游查询返回的状态和金额才是唯一依据。
// 回调若自带 signature，则按与收银台相同的 HMAC-SHA256 规则校验，签名不符直接拒绝。
func (s *SePay) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	_ = headers

	fields, err := parseSePayNotificationFields(rawBody)
	if err != nil {
		return nil, err
	}

	if signature := strings.TrimSpace(fields["signature"]); signature != "" {
		expected := sepaySignFields(fields, s.secretKey())
		if !hmac.Equal([]byte(signature), []byte(expected)) {
			return nil, fmt.Errorf("sepay notification signature mismatch")
		}
	}

	outTradeNo := sepayFirstNonEmpty(fields, "order_invoice_number", "orderInvoiceNumber", "invoice_number")
	if outTradeNo == "" {
		return nil, fmt.Errorf("sepay notification missing order_invoice_number")
	}

	// 回调携带的商户号必须与本实例一致，避免把别的商户的回调认到自己的订单上。
	if merchant := sepayFirstNonEmpty(fields, "merchant", "merchant_id", "merchantId"); merchant != "" {
		if !strings.EqualFold(merchant, s.merchantID()) {
			return nil, fmt.Errorf("sepay notification merchant mismatch: expected %s, got %s", s.merchantID(), merchant)
		}
	}

	queried, err := s.QueryOrder(ctx, outTradeNo)
	if err != nil {
		return nil, fmt.Errorf("sepay notification upstream confirmation failed: %w", err)
	}

	var status string
	switch queried.Status {
	case payment.ProviderStatusPaid, payment.ProviderStatusSuccess:
		status = payment.NotificationStatusSuccess
	case payment.ProviderStatusPending:
		// 上游仍未入账：当作无关事件处理，由调用方回 200 并等待下一次回调。
		return nil, nil
	default:
		status = payment.ProviderStatusFailed
	}

	metadata := make(map[string]string, len(queried.Metadata))
	for k, v := range queried.Metadata {
		metadata[k] = v
	}

	return &payment.PaymentNotification{
		TradeNo:  queried.TradeNo,
		OrderID:  outTradeNo,
		Amount:   queried.Amount,
		Status:   status,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

// CancelPayment 取消一笔尚未结算的上游订单。
//
// 二维码/银行转账订单走 order/cancel，卡支付走 order/voidTransaction；
// 服务商接口不区分方式，因此先取消、失败再尝试撤销授权。
func (s *SePay) CancelPayment(ctx context.Context, tradeNo string) error {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return fmt.Errorf("sepay cancel payment: trade number is required")
	}
	if _, err := s.client.Order.Cancel(ctx, tradeNo); err != nil {
		if _, voidErr := s.client.Order.VoidTransaction(ctx, tradeNo); voidErr != nil {
			return fmt.Errorf("sepay cancel payment %s: %w", tradeNo, err)
		}
	}
	return nil
}

// --- 上游响应解析 ---

// sepayOrder 是 SePay 订单详情里我们依赖的字段子集。
// 上游可能把订单包在 data 里，也可能直接返回对象，两种形状都要能读。
type sepayOrder struct {
	OrderInvoiceNumber string      `json:"order_invoice_number"`
	OrderID            string      `json:"order_id"`
	OrderStatus        string      `json:"order_status"`
	StatusField        string      `json:"status"`
	OrderAmount        json.Number `json:"order_amount"`
	AmountField        json.Number `json:"amount"`
	Currency           string      `json:"currency"`
	PaidAt             string      `json:"paid_at"`
}

// Status 返回上游订单状态，兼容 order_status 与 status 两种字段名。
func (o *sepayOrder) Status() string {
	if v := strings.TrimSpace(o.OrderStatus); v != "" {
		return v
	}
	return strings.TrimSpace(o.StatusField)
}

// Amount 返回上游订单金额的十进制精确值，解析不出时返回零值。
func (o *sepayOrder) Amount() decimal.Decimal {
	for _, raw := range []json.Number{o.OrderAmount, o.AmountField} {
		text := strings.TrimSpace(raw.String())
		if text == "" {
			continue
		}
		if d, err := decimal.NewFromString(text); err == nil {
			return d
		}
	}
	return decimal.Zero
}

func parseSePayOrder(body []byte) (*sepayOrder, error) {
	var envelope struct {
		Data *sepayOrder `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Data != nil {
		return envelope.Data, nil
	}
	var order sepayOrder
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("decode order response: %w", err)
	}
	if order.Status() == "" && strings.TrimSpace(order.OrderInvoiceNumber) == "" {
		return nil, fmt.Errorf("order response has neither status nor order_invoice_number")
	}
	return &order, nil
}

// sepayStatusToProviderStatus 把 SePay 的订单状态映射到服务层的状态取值。
// 未知状态一律视为 pending：宁可让订单继续等待轮询/回调，也不能凭猜测判定失败。
func sepayStatusToProviderStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED", "PAID", "SUCCESS", "SUCCEEDED", "SETTLED":
		return payment.ProviderStatusPaid
	case "FAILED", "ERROR", "DECLINED", "CANCELLED", "CANCELED", "EXPIRED", "VOIDED", "REJECTED":
		return payment.ProviderStatusFailed
	case "REFUNDED":
		return payment.ProviderStatusRefunded
	default:
		return payment.ProviderStatusPending
	}
}

// parseSePayNotificationFields 把回调体解析成扁平的字段表。
// SePay 的 IPN 可能是 JSON，也可能是表单编码，两种都要接。
func parseSePayNotificationFields(rawBody string) (map[string]string, error) {
	trimmed := strings.TrimSpace(rawBody)
	if trimmed == "" {
		return nil, fmt.Errorf("sepay notification body is empty")
	}

	if strings.HasPrefix(trimmed, "{") {
		var payload map[string]any
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return nil, fmt.Errorf("sepay notification decode: %w", err)
		}
		fields := make(map[string]string, len(payload))
		sepayFlattenNotification(payload, fields)
		if len(fields) == 0 {
			return nil, fmt.Errorf("sepay notification body has no usable fields")
		}
		return fields, nil
	}

	values, err := url.ParseQuery(trimmed)
	if err != nil {
		return nil, fmt.Errorf("sepay notification decode: %w", err)
	}
	fields := make(map[string]string, len(values))
	for key := range values {
		fields[key] = values.Get(key)
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("sepay notification body has no usable fields")
	}
	return fields, nil
}

// sepayFlattenNotification 抹平一层嵌套（例如 data 里再包一层订单对象），
// 顶层字段优先，嵌套字段只在顶层缺失时补入。
func sepayFlattenNotification(payload map[string]any, out map[string]string) {
	nested := make(map[string]string)
	for key, value := range payload {
		switch typed := value.(type) {
		case map[string]any:
			sepayFlattenNotification(typed, nested)
		default:
			if text, ok := sepayScalarToString(value); ok {
				out[key] = text
			}
		}
	}
	for key, value := range nested {
		if _, exists := out[key]; !exists {
			out[key] = value
		}
	}
}

func sepayScalarToString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case bool:
		return strconv.FormatBool(typed), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}

func sepayFirstNonEmpty(fields map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fields[key]); value != "" {
			return value
		}
	}
	return ""
}

// sepaySignFieldOrder 是签名的规范字段顺序，与 SDK 内部 signFieldOrder 保持一致。
// SDK 没有导出签名函数，回调校验只能在这里重建同一套规则。
var sepaySignFieldOrder = []string{
	"merchant",
	"env",
	"operation",
	"payment_method",
	"order_amount",
	"currency",
	"order_invoice_number",
	"order_description",
	"customer_id",
	"agreement_id",
	"agreement_name",
	"agreement_type",
	"agreement_payment_frequency",
	"agreement_amount_per_payment",
	"success_url",
	"error_url",
	"cancel_url",
	"order_id",
}

// sepaySignFields 按规范顺序拼接字段并计算 HMAC-SHA256，输出 base64。
func sepaySignFields(fields map[string]string, secretKey string) string {
	parts := make([]string, 0, len(sepaySignFieldOrder))
	for _, key := range sepaySignFieldOrder {
		value, ok := fields[key]
		if !ok {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(strings.Join(parts, ",")))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
