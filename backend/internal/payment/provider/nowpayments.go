package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// nowPaymentsHTTPTimeout 限制对 NOWPayments API 的单次调用时长。
const nowPaymentsHTTPTimeout = 20 * time.Second

// nowPaymentsEnvSandbox / nowPaymentsEnvProduction 是实例配置里 env 字段允许的取值。
const (
	nowPaymentsEnvSandbox    = "sandbox"
	nowPaymentsEnvProduction = "production"
)

// NOWPayments 的两套 API 地址。沙箱账号的 API Key 在生产地址上无效，反之亦然。
const (
	nowPaymentsProductionBaseURL = "https://api.nowpayments.io/v1"
	nowPaymentsSandboxBaseURL    = "https://api-sandbox.nowpayments.io/v1"
)

// nowPaymentsMaxResponseBytes 限制读取上游响应的字节数，避免异常响应撑爆内存。
const nowPaymentsMaxResponseBytes = 1 << 20

// nowPaymentsIsMissingResource 判断一次查询是不是「上游还没有这条记录」。
//
// 账单刚创建、用户还没选币种时，/payment/{id} 会返回 404 或 400，这属于正常
// 状态而不是错误。只有查询路径可以这样解读——创建账单时的 400 是参数错误。
func nowPaymentsIsMissingResource(err error) bool {
	var httpErr *nowPaymentsHTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	return httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusBadRequest
}

// nowPaymentsHTTPError 保留上游的状态码与响应体，供调用方自己解读。
type nowPaymentsHTTPError struct {
	StatusCode int
	Body       string
}

func (e *nowPaymentsHTTPError) Error() string {
	return fmt.Sprintf("upstream returned %d: %s", e.StatusCode, e.Body)
}

// NowPayments 实现 payment.Provider，接入 NOWPayments 的托管收银台（invoice）。
//
// NOWPayments 的收银台是一个普通的 GET 链接（invoice_url），
// 因此 CreatePayment 直接返回 PayURL。
//
// 结算判定完全依赖 IPN 回调：回调体用 IPN Secret 做 HMAC-SHA512 签名，
// 官方各语言 SDK 与 WooCommerce 插件都只认这一条路径，上游没有「按账单号查订单」
// 的接口可用来做二次确认。
type NowPayments struct {
	instanceID string
	config     map[string]string
	client     *http.Client
}

// NewNowPayments 用实例配置构造 NOWPayments 服务商。
//
// 配置键：apiKey（API 密钥）、ipnSecretKey（IPN 签名密钥）、
// env（sandbox / production）、currency（计价币种，默认 USD）、
// payCurrency（可选，指定结算加密货币，留空则由用户在收银台选择）。
func NewNowPayments(instanceID string, config map[string]string) (*NowPayments, error) {
	cfg := cloneStringMap(config)

	apiKey := strings.TrimSpace(cfg["apiKey"])
	if apiKey == "" {
		return nil, infraerrors.BadRequest("NOWPAYMENTS_CONFIG_MISSING_KEY",
			"nowpayments config missing required key: apiKey").
			WithMetadata(map[string]string{"field": "apiKey"})
	}
	// ipnSecretKey 是必填项：回调签名是这个通道唯一的真实性凭据，
	// 上游没有可用来复核订单状态的查询接口，缺了它任何人都能伪造一次成功回调。
	ipnSecretKey := strings.TrimSpace(cfg["ipnSecretKey"])
	if ipnSecretKey == "" {
		return nil, infraerrors.BadRequest("NOWPAYMENTS_CONFIG_MISSING_KEY",
			"nowpayments config missing required key: ipnSecretKey").
			WithMetadata(map[string]string{"field": "ipnSecretKey"})
	}

	env, err := normalizeNowPaymentsEnv(cfg["env"])
	if err != nil {
		return nil, err
	}
	cfg["env"] = env

	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, infraerrors.BadRequest("NOWPAYMENTS_CONFIG_INVALID_CURRENCY",
			fmt.Sprintf("nowpayments config currency: %v", err))
	}
	cfg["currency"] = currency

	return &NowPayments{
		instanceID: instanceID,
		config:     cfg,
		client:     &http.Client{Timeout: nowPaymentsHTTPTimeout},
	}, nil
}

func normalizeNowPaymentsEnv(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", nowPaymentsEnvProduction, "prod", "live":
		return nowPaymentsEnvProduction, nil
	case nowPaymentsEnvSandbox, "test", "demo":
		return nowPaymentsEnvSandbox, nil
	default:
		return "", infraerrors.BadRequest("NOWPAYMENTS_CONFIG_INVALID_ENV",
			"nowpayments config env must be either sandbox or production").
			WithMetadata(map[string]string{"field": "env"})
	}
}

func (n *NowPayments) Name() string        { return "NOWPayments" }
func (n *NowPayments) ProviderKey() string { return payment.TypeNowPayments }

func (n *NowPayments) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeNowPaymentsCrypto}
}

func (n *NowPayments) apiKey() string       { return strings.TrimSpace(n.config["apiKey"]) }
func (n *NowPayments) ipnSecretKey() string { return strings.TrimSpace(n.config["ipnSecretKey"]) }
func (n *NowPayments) payCurrency() string {
	return strings.ToLower(strings.TrimSpace(n.config["payCurrency"]))
}

func (n *NowPayments) currency() string {
	currency, err := payment.NormalizePaymentCurrency(n.config["currency"])
	if err != nil {
		return payment.DefaultPaymentCurrency
	}
	return currency
}

func (n *NowPayments) env() string {
	if strings.EqualFold(strings.TrimSpace(n.config["env"]), nowPaymentsEnvSandbox) {
		return nowPaymentsEnvSandbox
	}
	return nowPaymentsEnvProduction
}

func (n *NowPayments) baseURL() string {
	if n.env() == nowPaymentsEnvSandbox {
		return nowPaymentsSandboxBaseURL
	}
	return nowPaymentsProductionBaseURL
}

// CreatePayment 创建一张托管账单，返回可直接跳转的收银台地址。
func (n *NowPayments) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if err := nowPaymentsValidatePaymentType(req.PaymentType); err != nil {
		return nil, err
	}

	currency := n.currency()
	amount, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	if err != nil {
		return nil, fmt.Errorf("nowpayments create payment: invalid amount %q: %w", req.Amount, err)
	}
	// 先按币种精度校验金额：零小数币种不接受小数位。
	if _, err := payment.AmountToMinorUnit(req.Amount, currency); err != nil {
		return nil, fmt.Errorf("nowpayments create payment: %w", err)
	}

	// IPN 是这个网关唯一的到账凭据：上游没有可以复核账单状态的查询接口。
	// 少了回调地址，账单能开出来、用户能付钱，但订单永远停在挂起——
	// 那比建单直接失败糟糕得多。
	notifyURL := strings.TrimSpace(req.NotifyURL)
	if notifyURL == "" {
		return nil, infraerrors.ServiceUnavailable("NOWPAYMENTS_NOTIFY_URL_REQUIRED",
			"nowpayments needs an IPN callback URL: without it a paid invoice is never credited")
	}

	body := nowPaymentsInvoiceRequest{
		PriceAmount:      json.Number(amount.String()),
		PriceCurrency:    strings.ToLower(currency),
		OrderID:          req.OrderID,
		OrderDescription: req.Subject,
		IPNCallbackURL:   notifyURL,
		SuccessURL:       nowPaymentsRedirectURL(req.ReturnURL),
		CancelURL:        nowPaymentsRedirectURL(req.ReturnURL),
		PayCurrency:      n.payCurrency(),
	}

	var invoice nowPaymentsInvoice
	if err := n.doRequest(ctx, http.MethodPost, "/invoice", body, &invoice); err != nil {
		// 上游对无效参数只回一句 INTERNAL_ERROR，不说是哪个字段。把整个请求体
		// 带上——密钥走的是请求头，不在这里面——否则每排查一个字段就要改一次代码。
		encoded, _ := json.Marshal(body)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR",
			fmt.Sprintf("nowpayments create payment: %v", err)).
			WithMetadata(map[string]string{
				"request_body": nowPaymentsTruncateTo(string(encoded), 1500),
				"env":          n.env(),
			})
	}
	invoiceURL := strings.TrimSpace(invoice.InvoiceURL)
	if invoiceURL == "" {
		return nil, fmt.Errorf("nowpayments create payment: upstream returned no invoice_url")
	}

	return &payment.CreatePaymentResponse{
		// 账单号是这一步唯一能拿到的上游编号；真正的 payment_id 要等用户
		// 在收银台选定币种之后才由 IPN 带回来。
		TradeNo:    string(invoice.ID),
		PayURL:     invoiceURL,
		Currency:   currency,
		PaymentEnv: n.env(),
		ResultType: payment.CreatePaymentResultOrderCreated,
	}, nil
}

func nowPaymentsValidatePaymentType(paymentType string) error {
	switch strings.TrimSpace(paymentType) {
	case payment.TypeNowPaymentsCrypto, payment.TypeNowPayments, "":
		return nil
	default:
		return infraerrors.BadRequest("NOWPAYMENTS_UNSUPPORTED_PAYMENT_TYPE",
			fmt.Sprintf("nowpayments does not support payment type: %s", paymentType)).
			WithMetadata(map[string]string{"payment_type": paymentType})
	}
}

// QueryOrder 按 payment_id 查询上游支付状态。
//
// 注意 tradeNo 的两种可能：订单刚创建时它是账单号（invoice id），此时上游查不到
// 对应的 payment，接口会返回 404/400。这种情况一律当作「尚未支付」返回 pending，
// 而不是错误——账单尚未产生支付本来就是正常状态。只有 IPN 回调把 payment_id
// 写回订单之后，这里才真正查得到东西。
func (n *NowPayments) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, fmt.Errorf("nowpayments query order: trade number is required")
	}

	var paid nowPaymentsPayment
	err := n.doRequest(ctx, http.MethodGet, "/payment/"+nowPaymentsPathEscape(tradeNo), nil, &paid)
	if nowPaymentsIsMissingResource(err) {
		return &payment.QueryOrderResponse{
			TradeNo: tradeNo,
			Status:  payment.ProviderStatusPending,
			Amount:  decimal.Zero,
			Metadata: map[string]string{
				"currency": n.currency(),
				"env":      n.env(),
			},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("nowpayments query order %s: %w", tradeNo, err)
	}

	upstreamTradeNo := strings.TrimSpace(string(paid.PaymentID))
	if upstreamTradeNo == "" {
		upstreamTradeNo = tradeNo
	}
	currency := strings.ToUpper(strings.TrimSpace(paid.PriceCurrency))
	if currency == "" {
		currency = n.currency()
	}
	status := nowPaymentsStatusToProviderStatus(paid.PaymentStatus)

	// 金额一律取 price_amount，也就是我们下单时报给上游的法币金额。
	// 绝不能用 actually_paid / outcome_amount：那是链上实付的加密货币数量，
	// 与订单应付金额不同量纲，拿去比对必然误判。
	amount := paid.PriceAmount.Decimal()
	if status != payment.ProviderStatusPaid {
		amount = decimal.Zero
	}

	return &payment.QueryOrderResponse{
		TradeNo: upstreamTradeNo,
		Status:  status,
		Amount:  amount,
		PaidAt:  strings.TrimSpace(paid.UpdatedAt),
		Metadata: map[string]string{
			"currency":       currency,
			"env":            n.env(),
			"payment_status": strings.TrimSpace(paid.PaymentStatus),
			"pay_currency":   strings.TrimSpace(paid.PayCurrency),
			"actually_paid":  strings.TrimSpace(string(paid.ActuallyPaid)),
			"invoice_id":     strings.TrimSpace(string(paid.InvoiceID)),
		},
	}, nil
}

// VerifyNotification 校验并解析 NOWPayments 的 IPN 回调。
//
// 回调体带 x-nowpayments-sig 头，值是用 IPN Secret 对「键名排序后的 JSON」
// 做的 HMAC-SHA512。签名不符直接拒收：这个通道没有可用来复核的查询接口，
// 签名就是唯一的真实性凭据。
func (n *NowPayments) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	signature := strings.TrimSpace(headers["x-nowpayments-sig"])
	if signature == "" {
		return nil, fmt.Errorf("nowpayments notification missing x-nowpayments-sig header")
	}

	canonical, err := nowPaymentsCanonicalJSON(rawBody)
	if err != nil {
		return nil, fmt.Errorf("nowpayments notification decode: %w", err)
	}
	expected := nowPaymentsSign(canonical, n.ipnSecretKey())
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(expected)) != 1 {
		return nil, fmt.Errorf("nowpayments notification signature mismatch")
	}

	var notify nowPaymentsPayment
	if err := json.Unmarshal([]byte(rawBody), &notify); err != nil {
		return nil, fmt.Errorf("nowpayments notification decode: %w", err)
	}

	outTradeNo := strings.TrimSpace(notify.OrderID)
	if outTradeNo == "" {
		return nil, fmt.Errorf("nowpayments notification missing order_id")
	}

	status := nowPaymentsStatusToProviderStatus(notify.PaymentStatus)
	if status == payment.ProviderStatusPending {
		// 中间态（waiting / confirming / sending / partially_paid）：
		// 当作无关事件，由调用方回 200 等待下一次回调。
		return nil, nil
	}

	tradeNo := strings.TrimSpace(string(notify.PaymentID))
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(string(notify.InvoiceID))
	}
	metadata := map[string]string{
		"currency":       n.currency(),
		"env":            n.env(),
		"payment_status": strings.TrimSpace(notify.PaymentStatus),
		"pay_currency":   strings.TrimSpace(notify.PayCurrency),
		"actually_paid":  strings.TrimSpace(string(notify.ActuallyPaid)),
		"invoice_id":     strings.TrimSpace(string(notify.InvoiceID)),
	}

	if status != payment.ProviderStatusPaid {
		return &payment.PaymentNotification{
			TradeNo:  tradeNo,
			OrderID:  outTradeNo,
			Amount:   decimal.Zero,
			Status:   payment.ProviderStatusFailed,
			RawData:  rawBody,
			Metadata: metadata,
		}, nil
	}

	// 计价币种必须与实例配置一致，否则拿 price_amount 与订单金额比对没有意义
	// （例如实例配的是 USD，回调却是另一套币种的报价）。
	notifyCurrency := strings.ToUpper(strings.TrimSpace(notify.PriceCurrency))
	if notifyCurrency != "" && !strings.EqualFold(notifyCurrency, n.currency()) {
		return nil, fmt.Errorf("nowpayments notification currency mismatch: expected %s, got %s", n.currency(), notifyCurrency)
	}

	amount := notify.PriceAmount.Decimal()
	if !amount.IsPositive() {
		return nil, fmt.Errorf("nowpayments notification has no usable price_amount")
	}

	return &payment.PaymentNotification{
		TradeNo:  tradeNo,
		OrderID:  outTradeNo,
		Amount:   amount,
		Status:   payment.NotificationStatusSuccess,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

// --- 签名 ---

// nowPaymentsCanonicalJSON 把回调体重新序列化成上游签名时用的规范形式：
// 键名按字典序排列、数字保留原样、不做 HTML 转义。
//
// 三点都是必须的：
//   - Go 的 encoding/json 序列化 map 时本来就按键名排序，所以解成 map 再编码
//     即可得到与 PHP 的 ksort、Node 的 Object.keys().sort() 相同的顺序；
//   - UseNumber 让 0.00034 这类数字保持字面量，否则经 float64 往返会变成
//     3.4e-05 之类的写法，签名必然对不上；
//   - SetEscapeHTML(false) 关掉 Go 默认把尖括号与 & 转成转义序列的行为，
//     上游各语言的 json 编码都不会那么写。
func nowPaymentsCanonicalJSON(rawBody string) ([]byte, error) {
	decoder := json.NewDecoder(strings.NewReader(rawBody))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	if _, ok := payload.(map[string]any); !ok {
		return nil, fmt.Errorf("notification body is not a JSON object")
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return nil, err
	}
	// Encode 会补一个换行，签名的原文里没有它。
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func nowPaymentsSign(canonical []byte, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	_, _ = mac.Write(canonical)
	return hex.EncodeToString(mac.Sum(nil))
}

// --- 上游响应解析 ---

type nowPaymentsInvoiceRequest struct {
	PriceAmount      json.Number `json:"price_amount"`
	PriceCurrency    string      `json:"price_currency"`
	OrderID          string      `json:"order_id"`
	OrderDescription string      `json:"order_description,omitempty"`
	IPNCallbackURL   string      `json:"ipn_callback_url,omitempty"`
	SuccessURL       string      `json:"success_url,omitempty"`
	CancelURL        string      `json:"cancel_url,omitempty"`
	PayCurrency      string      `json:"pay_currency,omitempty"`
}

type nowPaymentsInvoice struct {
	ID            nowPaymentsScalar `json:"id"`
	OrderID       string            `json:"order_id"`
	InvoiceURL    string            `json:"invoice_url"`
	PriceAmount   nowPaymentsScalar `json:"price_amount"`
	PriceCurrency string            `json:"price_currency"`
}

// nowPaymentsPayment 是支付详情与 IPN 回调共用的字段子集——两者形状一致。
type nowPaymentsPayment struct {
	PaymentID     nowPaymentsScalar `json:"payment_id"`
	InvoiceID     nowPaymentsScalar `json:"invoice_id"`
	OrderID       string            `json:"order_id"`
	PaymentStatus string            `json:"payment_status"`
	PriceAmount   nowPaymentsScalar `json:"price_amount"`
	PriceCurrency string            `json:"price_currency"`
	PayCurrency   string            `json:"pay_currency"`
	ActuallyPaid  nowPaymentsScalar `json:"actually_paid"`
	OutcomeAmount nowPaymentsScalar `json:"outcome_amount"`
	UpdatedAt     string            `json:"updated_at"`
}

// nowPaymentsScalar 兼容上游把同一个字段有时写成数字、有时写成字符串的习惯
// （payment_id、price_amount 在不同接口里两种形状都出现过）。
type nowPaymentsScalar string

func (s *nowPaymentsScalar) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "null" {
		*s = ""
		return nil
	}
	var unquoted string
	if err := json.Unmarshal(data, &unquoted); err == nil {
		*s = nowPaymentsScalar(unquoted)
		return nil
	}
	*s = nowPaymentsScalar(text)
	return nil
}

// Decimal 返回字段的十进制精确值，解析不出时返回零值。
func (s nowPaymentsScalar) Decimal() decimal.Decimal {
	text := strings.TrimSpace(string(s))
	if text == "" {
		return decimal.Zero
	}
	value, err := decimal.NewFromString(text)
	if err != nil {
		return decimal.Zero
	}
	return value
}

// nowPaymentsStatusToProviderStatus 把上游支付状态映射到服务层的状态取值。
//
// partially_paid 归入 pending 而不是失败：链上确实收到了钱，只是不够。
// 订单继续挂着等待补付或到期，绝不能按不足额的金额履约。
// 未知状态同样按 pending 处理，宁可继续等，也不凭猜测判失败。
func nowPaymentsStatusToProviderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "finished":
		return payment.ProviderStatusPaid
	case "failed", "expired":
		return payment.ProviderStatusFailed
	case "refunded":
		return payment.ProviderStatusRefunded
	default:
		return payment.ProviderStatusPending
	}
}

// --- HTTP ---

func (n *NowPayments) doRequest(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, n.baseURL()+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-api-key", n.apiKey())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	payloadBytes, err := io.ReadAll(io.LimitReader(resp.Body, nowPaymentsMaxResponseBytes))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 状态码原样带出来：把 400 一律当成「资源不存在」是 QueryOrder 的语义，
		// 放在这里会让创建账单时的参数错误变成一句「not found」，上游到底说了
		// 什么全部丢掉。
		return &nowPaymentsHTTPError{
			StatusCode: resp.StatusCode,
			Body:       nowPaymentsTruncate(string(payloadBytes)),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payloadBytes, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func nowPaymentsTruncate(text string) string {
	return nowPaymentsTruncateTo(text, 200)
}

func nowPaymentsTruncateTo(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "...(truncated)"
}

// nowPaymentsMaxRedirectURLLength 是 success_url / cancel_url 的安全长度。
//
// 上游对超长的回跳地址只回一句 INTERNAL_ERROR，不说是哪个字段。我们的回跳地址
// 带着一个 JWT 形态的 resume token，轻易就超过 255。
const nowPaymentsMaxRedirectURLLength = 255

// nowPaymentsRedirectURL 把回跳地址压到上游能接受的长度。
//
// 砍掉的是 resume_token：结果页还能用 order_id 与 out_trade_no 定位这张单，
// 而带着一个会被整体拒绝的地址，等于一张建不出来的账单。
func nowPaymentsRedirectURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) <= nowPaymentsMaxRedirectURLLength {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	query.Del("resume_token")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// nowPaymentsPathEscape 只允许上游编号里出现的字符进入路径，避免拼接出别的接口。
func nowPaymentsPathEscape(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, value)
}
