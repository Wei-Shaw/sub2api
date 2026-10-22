package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

// NOWPayments API 常量
const (
	nowPaymentsAPIBase         = "https://api.nowpayments.io/v1"
	nowPaymentsHTTPTimeout     = 15 * time.Second
	nowPaymentsMaxResponseSize = 1 << 20 // 1MB
	nowPaymentsMaxErrorSummary = 512

	// NOWPayments 支付状态
	npStatusWaiting        = "waiting"
	npStatusConfirming     = "confirming"
	npStatusConfirmed      = "confirmed"
	npStatusSending        = "sending"
	npStatusPartiallyPaid  = "partially_paid"
	npStatusFinished       = "finished"
	npStatusFailed         = "failed"
	npStatusExpired        = "expired"
)

// NowPayments 实现 payment.Provider 接口，支持 NOWPayments 加密货币支付
// WooCross (https://woocross.com/) 底层使用 NOWPayments API，故直接集成 NOWPayments
type NowPayments struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewNowPayments 创建 NOWPayments Provider 实例
func NewNowPayments(instanceID string, config map[string]string) (*NowPayments, error) {
	for _, k := range []string{"apiKey", "ipnSecret"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("nowpayments config missing required key: %s", k)
		}
	}
	cfg := cloneStringMap(config)

	// 验证 apiBase（可选，默认使用正式环境）
	apiBase := strings.TrimSpace(cfg["apiBase"])
	if apiBase == "" {
		apiBase = nowPaymentsAPIBase
	} else {
		parsed, err := url.Parse(apiBase)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, fmt.Errorf("nowpayments apiBase must be an HTTPS URL")
		}
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		apiBase = parsed.String()
	}
	cfg["apiBase"] = apiBase

	// 默认计价法币
	priceCurrency := strings.TrimSpace(cfg["priceCurrency"])
	if priceCurrency == "" {
		cfg["priceCurrency"] = "USD"
	}

	// 默认支付币种
	payCurrency := strings.TrimSpace(cfg["payCurrency"])
	if payCurrency == "" {
		cfg["payCurrency"] = "usdt"
	}

	return &NowPayments{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: nowPaymentsHTTPTimeout},
	}, nil
}

func (n *NowPayments) Name() string        { return "NOWPayments / WooCross" }
func (n *NowPayments) ProviderKey() string { return payment.TypeNowPayments }
func (n *NowPayments) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeNowPayments}
}

func (n *NowPayments) MerchantIdentityMetadata() map[string]string {
	if n == nil {
		return nil
	}
	return map[string]string{
		"pay_currency": n.config["payCurrency"],
		"price_currency": n.config["priceCurrency"],
	}
}

// CreatePayment 创建 NOWPayments 支付订单
// NOWPayments 使用法币（如 USD）计价，用户用加密货币（如 USDT）支付
func (n *NowPayments) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("nowpayments create payment: invalid amount %s", req.Amount)
	}

	priceCurrency := strings.ToUpper(strings.TrimSpace(n.config["priceCurrency"]))
	payCurrency := strings.ToLower(strings.TrimSpace(req.InstanceSubMethods))
	if payCurrency == "" {
		payCurrency = strings.ToLower(strings.TrimSpace(n.config["payCurrency"]))
	}
	if payCurrency == "" {
		payCurrency = "usdt"
	}

	payload := npCreatePaymentRequest{
		PriceAmount:      amount.StringFixed(2),
		PriceCurrency:    priceCurrency,
		PayCurrency:      payCurrency,
		IPNCallbackURL:   req.NotifyURL,
		OrderID:          req.OrderID,
		OrderDescription: req.Subject,
		SuccessURL:       req.ReturnURL,
	}

	var resp npCreatePaymentResponse
	if err := n.doJSON(ctx, http.MethodPost, "/payment", payload, &resp); err != nil {
		return nil, fmt.Errorf("nowpayments create payment: %w", err)
	}

	if strings.TrimSpace(resp.PaymentID) == "" {
		return nil, fmt.Errorf("nowpayments create payment: empty payment_id in response")
	}

	result := &payment.CreatePaymentResponse{
		TradeNo:    resp.PaymentID,
		PayURL:     "", // NOWPayments 无支付页面 URL，前端需展示 pay_address 和 pay_amount
		QRCode:     resp.PayAddress,
		Currency:   strings.ToUpper(payCurrency),
		ResultType: payment.CreatePaymentResultOrderCreated,
		Metadata: map[string]string{
			"pay_amount":      fmt.Sprintf("%.8f", resp.PayAmount),
			"pay_currency":    resp.PayCurrency,
			"pay_address":     resp.PayAddress,
			"payment_id":      resp.PaymentID,
		},
	}

	if resp.PayinExtraID != "" {
		result.Metadata["payin_extra_id"] = resp.PayinExtraID
	}

	return result, nil
}

// QueryOrder 查询 NOWPayments 支付订单状态
func (n *NowPayments) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	paymentID := strings.TrimSpace(tradeNo)
	if paymentID == "" {
		return nil, fmt.Errorf("nowpayments query order: missing payment_id")
	}

	var resp npPaymentStatusResponse
	if err := n.doJSON(ctx, http.MethodGet, "/payment/"+url.PathEscape(paymentID), nil, &resp); err != nil {
		return nil, fmt.Errorf("nowpayments query order: %w", err)
	}

	return &payment.QueryOrderResponse{
		TradeNo: resp.PaymentID,
		Status:  npProviderStatus(resp.PaymentStatus),
		Amount:  resp.PayAmount,
		PaidAt:  resp.UpdatedAt,
		Metadata: map[string]string{
			"pay_currency":   resp.PayCurrency,
			"payment_status": resp.PaymentStatus,
		},
	}, nil
}

// VerifyNotification 验证 NOWPayments IPN（Webhook）回调
// 验证方式：HMAC-SHA512( request_body, ipn_secret_key )
// 签名放在 header: x-nowpayments-sig
func (n *NowPayments) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	ipnSecret := strings.TrimSpace(n.config["ipnSecret"])
	if ipnSecret == "" {
		return nil, fmt.Errorf("nowpayments ipnSecret not configured")
	}

	sig := strings.ToLower(strings.TrimSpace(headers["x-nowpayments-sig"]))
	if sig == "" {
		return nil, fmt.Errorf("nowpayments notification missing x-nowpayments-sig header")
	}

	mac := hmac.New(sha512.New, []byte(ipnSecret))
	_, _ = mac.Write([]byte(rawBody))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return nil, fmt.Errorf("nowpayments invalid IPN signature (got %s, expected %s)", sig, expected)
	}

	var ipn npIPN
	if err := json.Unmarshal([]byte(rawBody), &ipn); err != nil {
		return nil, fmt.Errorf("nowpayments parse IPN: %w", err)
	}

	if strings.TrimSpace(ipn.PaymentID) == "" || strings.TrimSpace(ipn.OrderID) == "" {
		return nil, fmt.Errorf("nowpayments IPN missing payment_id or order_id")
	}

	status := npProviderStatus(ipn.PaymentStatus)
	if status == payment.ProviderStatusPending {
		// 非最终状态，告知调用方忽略
		return nil, nil
	}

	return &payment.PaymentNotification{
		TradeNo: ipn.PaymentID,
		OrderID: ipn.OrderID,
		Amount:  ipn.PayAmount,
		Status:  status,
		RawData: rawBody,
		Metadata: map[string]string{
			"pay_currency":   ipn.PayCurrency,
			"payment_status": ipn.PaymentStatus,
		},
	}, nil
}

// Refund NOWPayments 不支持 API 退款，需在 Dashboard 手动操作
func (n *NowPayments) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("nowpayments: refund not supported via API, please process in NOWPayments dashboard")
}

// --------------- HTTP 请求封装 ---------------

func (n *NowPayments) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var bodyReader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, n.config["apiBase"]+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", n.config["apiKey"])

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, nowPaymentsMaxResponseSize))
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("nowpayments HTTP %d: %s", resp.StatusCode, summarizeNPResponse(body))
	}

	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("nowpayments parse response: %w", err)
	}
	return nil
}

// --------------- 状态映射 ---------------

func npProviderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case npStatusFinished:
		return payment.NotificationStatusSuccess
	case npStatusFailed, npStatusExpired:
		return payment.ProviderStatusFailed
	case npStatusConfirming, npStatusConfirmed, npStatusSending:
		return payment.ProviderStatusPaid
	default:
		return payment.ProviderStatusPending
	}
}

func summarizeNPResponse(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if len(summary) > nowPaymentsMaxErrorSummary {
		return summary[:nowPaymentsMaxErrorSummary] + "..."
	}
	return summary
}

// --------------- API 请求/响应结构体 ---------------

type npCreatePaymentRequest struct {
	PriceAmount      string `json:"price_amount"`
	PriceCurrency    string `json:"price_currency"`
	PayCurrency      string `json:"pay_currency,omitempty"`
	IPNCallbackURL   string `json:"ipn_callback_url,omitempty"`
	OrderID          string `json:"order_id,omitempty"`
	OrderDescription string `json:"order_description,omitempty"`
	SuccessURL       string `json:"success_url,omitempty"`
}

type npCreatePaymentResponse struct {
	PaymentID        string  `json:"payment_id"`
	PaymentStatus    string  `json:"payment_status"`
	PayAddress       string  `json:"pay_address"`
	PayAmount        float64 `json:"pay_amount"`
	PayCurrency      string  `json:"pay_currency"`
	PriceAmount      float64 `json:"price_amount"`
	PriceCurrency    string  `json:"price_currency"`
	OrderID          string  `json:"order_id"`
	PayinExtraID     string  `json:"payin_extra_id"`
}

type npPaymentStatusResponse struct {
	PaymentID     string  `json:"payment_id"`
	PaymentStatus string  `json:"payment_status"`
	PayAmount     float64 `json:"pay_amount"`
	PayCurrency   string  `json:"pay_currency"`
	PriceAmount   float64 `json:"price_amount"`
	PriceCurrency string  `json:"price_currency"`
	OrderID       string  `json:"order_id"`
	UpdatedAt     string  `json:"updated_at"`
}

type npIPN struct {
	PaymentID        string  `json:"payment_id"`
	PaymentStatus    string  `json:"payment_status"`
	PayAddress       string  `json:"pay_address"`
	PayAmount        float64 `json:"pay_amount"`
	PayCurrency      string  `json:"pay_currency"`
	PriceAmount      float64 `json:"price_amount"`
	PriceCurrency    string  `json:"price_currency"`
	OrderID          string  `json:"order_id"`
	OrderDescription string  `json:"order_description"`
	PaymentUpdatedAt string  `json:"payment_updated_at"`
}

// Ensure interface compliance
var (
	_ payment.Provider = (*NowPayments)(nil)
)
