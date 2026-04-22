// Package provider contains concrete payment provider implementations.
package provider

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// EasyPay constants.
const (
	easypayCodeSuccess     = 1
	easypayStatusPaid      = 1
	easypayHTTPTimeout     = 10 * time.Second
	maxEasypayResponseSize = 1 << 20 // 1MB
	tradeStatusSuccess     = "TRADE_SUCCESS"
	signTypeMD5            = "MD5"
	paymentModePopup       = "popup"
	deviceMobile           = "mobile"
)

// EasyPay implements payment.Provider for the EasyPay aggregation platform.
type EasyPay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewEasyPay creates a new EasyPay provider.
// config keys: pid, pkey, apiBase, notifyUrl, returnUrl, cid, cidAlipay, cidWxpay
func NewEasyPay(instanceID string, config map[string]string) (*EasyPay, error) {
	for _, k := range []string{"pid", "pkey", "apiBase", "notifyUrl", "returnUrl"} {
		if config[k] == "" {
			return nil, fmt.Errorf("easypay config missing required key: %s", k)
		}
	}
	return &EasyPay{
		instanceID: instanceID,
		config:     config,
		httpClient: &http.Client{Timeout: easypayHTTPTimeout},
	}, nil
}

func (e *EasyPay) Name() string        { return "EasyPay" }
func (e *EasyPay) ProviderKey() string { return payment.TypeEasyPay }
func (e *EasyPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay}
}

func (e *EasyPay) MerchantIdentityMetadata() map[string]string {
	if e == nil {
		return nil
	}
	pid := strings.TrimSpace(e.config["pid"])
	if pid == "" {
		return nil
	}
	return map[string]string{"pid": pid}
}

func (e *EasyPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	// Payment mode determined by instance config, not payment type.
	// "popup" → hosted page (submit.php); "qrcode"/default → API call (mapi.php).
	mode := e.config["paymentMode"]
	if mode == paymentModePopup {
		return e.createRedirectPayment(req)
	}
	return e.createAPIPayment(ctx, req)
}

// createRedirectPayment builds a submit.php URL for browser redirect.
// No server-side API call — the user is redirected to EasyPay's hosted page.
// TradeNo is empty; it arrives via the notify callback after payment.
func (e *EasyPay) createRedirectPayment(req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := e.resolveURLs(req)
	params := map[string]string{
		"pid": e.config["pid"], "type": req.PaymentType,
		"out_trade_no": req.OrderID, "notify_url": notifyURL,
		"return_url": returnURL, "name": req.Subject,
		"money": req.Amount,
	}
	if cid := e.resolveCID(req.PaymentType); cid != "" {
		params["cid"] = cid
	}
	if req.IsMobile {
		params["device"] = deviceMobile
	}
	params["sign"] = easyPaySign(params, e.config["pkey"])
	params["sign_type"] = signTypeMD5

	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	base := strings.TrimRight(e.config["apiBase"], "/")
	payURL := base + "/submit.php?" + q.Encode()
	return &payment.CreatePaymentResponse{PayURL: payURL}, nil
}

// createAPIPayment calls mapi.php to get payurl/qrcode (existing behavior).
func (e *EasyPay) createAPIPayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := e.resolveURLs(req)
	params := map[string]string{
		"pid": e.config["pid"], "type": req.PaymentType,
		"out_trade_no": req.OrderID, "notify_url": notifyURL,
		"return_url": returnURL, "name": req.Subject,
		"money": req.Amount, "clientip": req.ClientIP,
	}
	if cid := e.resolveCID(req.PaymentType); cid != "" {
		params["cid"] = cid
	}
	if req.IsMobile {
		params["device"] = deviceMobile
	}
	params["sign"] = easyPaySign(params, e.config["pkey"])
	params["sign_type"] = signTypeMD5

	body, err := e.post(ctx, strings.TrimRight(e.config["apiBase"], "/")+"/mapi.php", params)
	if err != nil {
		return nil, fmt.Errorf("easypay create: %w", err)
	}
	var resp struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		TradeNo string `json:"trade_no"`
		PayURL  string `json:"payurl"`
		PayURL2 string `json:"payurl2"` // H5 mobile payment URL
		QRCode  string `json:"qrcode"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("easypay parse: %w", err)
	}
	if resp.Code != easypayCodeSuccess {
		return nil, fmt.Errorf("easypay error: %s", resp.Msg)
	}
	payURL := resp.PayURL
	if req.IsMobile && resp.PayURL2 != "" {
		payURL = resp.PayURL2
	}
	return &payment.CreatePaymentResponse{TradeNo: resp.TradeNo, PayURL: payURL, QRCode: resp.QRCode}, nil
}

// resolveURLs returns (notifyURL, returnURL) preferring request values,
// falling back to instance config.
func (e *EasyPay) resolveURLs(req payment.CreatePaymentRequest) (string, string) {
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = e.config["notifyUrl"]
	}
	returnURL := req.ReturnURL
	if returnURL == "" {
		returnURL = e.config["returnUrl"]
	}
	return notifyURL, returnURL
}

func (e *EasyPay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	params := map[string]string{
		"act": "order", "pid": e.config["pid"],
		"key": e.config["pkey"], "out_trade_no": tradeNo,
	}
	body, err := e.post(ctx, e.config["apiBase"]+"/api.php", params)
	if err != nil {
		return nil, fmt.Errorf("easypay query: %w", err)
	}
	var resp struct {
		Code   int    `json:"code"`
		Msg    string `json:"msg"`
		Status int    `json:"status"`
		Money  string `json:"money"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("easypay parse query: %w", err)
	}
	status := payment.ProviderStatusPending
	if resp.Status == easypayStatusPaid {
		status = payment.ProviderStatusPaid
	}
	amount, _ := strconv.ParseFloat(resp.Money, 64)
	return &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Status:   status,
		Amount:   amount,
		Metadata: e.MerchantIdentityMetadata(),
	}, nil
}

func (e *EasyPay) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("parse notify: %w", err)
	}
	// url.ParseQuery already decodes values — no additional decode needed.
	params := make(map[string]string)
	for k := range values {
		params[k] = values.Get(k)
	}
	sign := params["sign"]
	if sign == "" {
		return nil, fmt.Errorf("missing sign")
	}
	if !easyPayVerifySign(params, e.config["pkey"], sign) {
		return nil, fmt.Errorf("invalid signature")
	}
	status := payment.ProviderStatusFailed
	if params["trade_status"] == tradeStatusSuccess {
		status = payment.ProviderStatusSuccess
	}
	amount, _ := strconv.ParseFloat(params["money"], 64)

	metadata := e.MerchantIdentityMetadata()
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

func (e *EasyPay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	params := map[string]string{
		"pid": e.config["pid"], "key": e.config["pkey"],
		"trade_no": req.TradeNo, "out_trade_no": req.OrderID, "money": req.Amount,
	}
	body, err := e.post(ctx, e.config["apiBase"]+"/api.php?act=refund", params)
	if err != nil {
		return nil, fmt.Errorf("easypay refund: %w", err)
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("easypay parse refund: %w", err)
	}
	if resp.Code != easypayCodeSuccess {
		return nil, fmt.Errorf("easypay refund failed: %s", resp.Msg)
	}
	return &payment.RefundResponse{RefundID: req.TradeNo, Status: payment.ProviderStatusSuccess}, nil
}

func (e *EasyPay) resolveCID(paymentType string) string {
	if strings.HasPrefix(paymentType, "alipay") {
		if v := e.config["cidAlipay"]; v != "" {
			return v
		}
		return e.config["cid"]
	}
	if v := e.config["cidWxpay"]; v != "" {
		return v
	}
	return e.config["cid"]
}

func (e *EasyPay) post(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, maxEasypayResponseSize))
}

func easyPaySign(params map[string]string, pkey string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			_ = buf.WriteByte('&')
		}
		_, _ = buf.WriteString(k + "=" + params[k])
	}
	_, _ = buf.WriteString(pkey)
	hash := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(hash[:])
}

func easyPayVerifySign(params map[string]string, pkey string, sign string) bool {
	return hmac.Equal([]byte(easyPaySign(params, pkey)), []byte(sign))
}
