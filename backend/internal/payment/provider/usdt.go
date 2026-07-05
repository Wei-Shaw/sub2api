package provider

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// USDT constants.
const (
	usdtHTTPTimeout      = 10 * time.Second
	usdtDefaultTradeType = "usdt.trc20"
)

// USDT implements payment.Provider for self-hosted crypto gateways that speak
// the EasyPay (彩虹易支付) protocol but settle in stablecoins — e.g. BEpusdt.
//
// Unlike the EasyPay provider (which exposes alipay/wxpay payment types and can
// call mapi.php), this provider exposes a single first-class "usdt" payment type
// and always uses the hosted checkout page (submit.php). The on-chain trade type
// sent to the gateway (default "usdt.trc20") is configurable so other chains or
// stablecoins can be served by additional instances without code changes.
type USDT struct {
	instanceID string
	config     map[string]string
	tradeType  string
}

// NewUSDT creates a new USDT provider.
// config keys: pid, pkey, apiBase, notifyUrl, returnUrl, tradeType (optional).
func NewUSDT(instanceID string, config map[string]string) (*USDT, error) {
	for _, k := range []string{"pid", "pkey", "apiBase", "notifyUrl", "returnUrl"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("usdt config missing required key: %s", k)
		}
	}
	cfg := make(map[string]string, len(config))
	for k, v := range config {
		cfg[k] = v
	}
	cfg["apiBase"] = normalizeEasyPayAPIBase(cfg["apiBase"])
	tradeType := strings.TrimSpace(cfg["tradeType"])
	if tradeType == "" {
		tradeType = usdtDefaultTradeType
	}
	return &USDT{instanceID: instanceID, config: cfg, tradeType: tradeType}, nil
}

func (u *USDT) Name() string        { return "USDT" }
func (u *USDT) ProviderKey() string { return payment.TypeUsdt }
func (u *USDT) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeUsdt}
}

func (u *USDT) apiBase() string {
	if u == nil {
		return ""
	}
	return normalizeEasyPayAPIBase(u.config["apiBase"])
}

func (u *USDT) MerchantIdentityMetadata() map[string]string {
	if u == nil {
		return nil
	}
	pid := strings.TrimSpace(u.config["pid"])
	if pid == "" {
		return nil
	}
	return map[string]string{"pid": pid}
}

// CreatePayment builds a hosted-checkout (submit.php) URL for browser redirect.
// No server-side API call is made — the user is redirected to the gateway's
// hosted cashier. The crypto trade type (e.g. usdt.trc20) is sent natively, so
// the gateway accepts it directly without any type rewriting.
func (u *USDT) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := u.resolveURLs(req)
	params := map[string]string{
		"pid":          u.config["pid"],
		"type":         u.tradeType,
		"out_trade_no": req.OrderID,
		"notify_url":   notifyURL,
		"return_url":   returnURL,
		"name":         req.Subject,
		"money":        req.Amount,
	}
	if req.IsMobile {
		params["device"] = deviceMobile
	}
	params["sign"] = easyPaySign(params, u.config["pkey"])
	params["sign_type"] = signTypeMD5

	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	payURL := u.apiBase() + "/submit.php?" + q.Encode()
	return &payment.CreatePaymentResponse{PayURL: payURL}, nil
}

func (u *USDT) resolveURLs(req payment.CreatePaymentRequest) (string, string) {
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = u.config["notifyUrl"]
	}
	returnURL := req.ReturnURL
	if returnURL == "" {
		returnURL = u.config["returnUrl"]
	}
	return notifyURL, returnURL
}

// QueryOrder is best-effort. The hosted gateway confirms payment via the notify
// callback (the source of truth); polling is only a reconciliation safety net.
func (u *USDT) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	return &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Status:   payment.ProviderStatusPending,
		Metadata: u.MerchantIdentityMetadata(),
	}, nil
}

// VerifyNotification verifies the gateway's async notify callback (MD5 sign over
// the form params, identical to the EasyPay protocol).
func (u *USDT) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("parse notify: %w", err)
	}
	params := make(map[string]string)
	for k := range values {
		params[k] = values.Get(k)
	}
	sign := params["sign"]
	if sign == "" {
		return nil, fmt.Errorf("missing sign")
	}
	if !easyPayVerifySign(params, u.config["pkey"], sign) {
		return nil, fmt.Errorf("invalid signature")
	}
	status := payment.ProviderStatusFailed
	if params["trade_status"] == tradeStatusSuccess {
		status = payment.ProviderStatusSuccess
	}
	amount, _ := strconv.ParseFloat(params["money"], 64)

	metadata := u.MerchantIdentityMetadata()
	if pid := strings.TrimSpace(params["pid"]); pid != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata["pid"] = pid
	}
	return &payment.PaymentNotification{
		TradeNo: params["trade_no"], OrderID: params["out_trade_no"],
		Amount: amount, Status: status, RawData: rawBody, Metadata: metadata,
	}, nil
}

// Refund is not supported for crypto settlements (on-chain refunds are manual).
func (u *USDT) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("usdt: refund not supported")
}
