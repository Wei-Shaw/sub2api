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
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
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
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (e *EasyPay) Name() string        { return "EasyPay" }
func (e *EasyPay) ProviderKey() string  { return "easypay" }
func (e *EasyPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay}
}

func (e *EasyPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	params := map[string]string{
		"pid": e.config["pid"], "type": req.PaymentType,
		"out_trade_no": req.OrderID, "notify_url": req.NotifyURL,
		"return_url": req.ReturnURL, "name": req.Subject,
		"money": req.Amount, "clientip": req.ClientIP,
	}
	if cid := e.resolveCID(req.PaymentType); cid != "" {
		params["cid"] = cid
	}
	if req.IsMobile {
		params["device"] = "mobile"
	}
	params["sign"] = easyPaySign(params, e.config["pkey"])
	params["sign_type"] = "MD5"

	body, err := e.post(ctx, e.config["apiBase"]+"/mapi.php", params)
	if err != nil {
		return nil, fmt.Errorf("easypay create: %w", err)
	}
	var resp struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		TradeNo string `json:"trade_no"`
		PayURL  string `json:"payurl"`
		QRCode  string `json:"qrcode"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("easypay parse: %w", err)
	}
	if resp.Code != 1 {
		return nil, fmt.Errorf("easypay error: %s", resp.Msg)
	}
	return &payment.CreatePaymentResponse{TradeNo: resp.TradeNo, PayURL: resp.PayURL, QRCode: resp.QRCode}, nil
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
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("easypay parse query: %w", err)
	}
	status := "pending"
	if resp.Status == 1 {
		status = "paid"
	}
	return &payment.QueryOrderResponse{TradeNo: tradeNo, Status: status}, nil
}

func (e *EasyPay) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
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
	if !easyPayVerifySign(params, e.config["pkey"], sign) {
		return nil, fmt.Errorf("invalid signature")
	}
	status := "failed"
	if params["trade_status"] == "TRADE_SUCCESS" {
		status = "success"
	}
	return &payment.PaymentNotification{
		TradeNo: params["trade_no"], OrderID: params["out_trade_no"],
		Status: status, RawData: rawBody,
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
	if resp.Code != 1 {
		return nil, fmt.Errorf("easypay refund failed: %s", resp.Msg)
	}
	return &payment.RefundResponse{RefundID: req.TradeNo, Status: "success"}, nil
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
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
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
			buf.WriteByte('&')
		}
		buf.WriteString(k + "=" + params[k])
	}
	buf.WriteString(pkey)
	hash := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(hash[:])
}

func easyPayVerifySign(params map[string]string, pkey string, sign string) bool {
	return hmac.Equal([]byte(easyPaySign(params, pkey)), []byte(sign))
}
