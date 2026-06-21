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

const (
	xunhuPayHTTPTimeout     = 10 * time.Second
	maxXunhuPayResponseSize = 1 << 20
)

// XunhuPay implements the Sub2ApiPay legacy XunhuPay integration.
type XunhuPay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewXunhuPay creates a XunhuPay provider.
// config keys: appId, secret, gatewayUrl, notifyUrl, returnUrl
func NewXunhuPay(instanceID string, config map[string]string) (*XunhuPay, error) {
	for _, k := range []string{"appId", "secret", "gatewayUrl", "notifyUrl", "returnUrl"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("xunhupay config missing required key: %s", k)
		}
	}
	cfg := make(map[string]string, len(config))
	for k, v := range config {
		cfg[k] = v
	}
	cfg["gatewayUrl"] = strings.TrimSpace(cfg["gatewayUrl"])
	return &XunhuPay{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: xunhuPayHTTPTimeout},
	}, nil
}

func (x *XunhuPay) Name() string        { return "XunhuPay" }
func (x *XunhuPay) ProviderKey() string { return payment.TypeXunhuPay }
func (x *XunhuPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeWxpayXunhu}
}

func (x *XunhuPay) MerchantIdentityMetadata() map[string]string {
	if x == nil {
		return nil
	}
	appID := strings.TrimSpace(x.config["appId"])
	if appID == "" {
		return nil
	}
	return map[string]string{"appid": appID}
}

func (x *XunhuPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = x.config["notifyUrl"]
	}
	returnURL := req.ReturnURL
	if returnURL == "" {
		returnURL = x.config["returnUrl"]
	}
	params := map[string]string{
		"version":        "1.1",
		"appid":          x.config["appId"],
		"trade_order_id": req.OrderID,
		"total_fee":      req.Amount,
		"title":          req.Subject,
		"notify_url":     notifyURL,
		"return_url":     returnURL,
		"time":           strconv.FormatInt(time.Now().Unix(), 10),
		"nonce_str":      strconv.FormatInt(time.Now().UnixNano(), 10),
	}
	params["hash"] = xunhuPaySign(params, x.config["secret"])

	body, err := x.post(ctx, x.config["gatewayUrl"], params)
	if err != nil {
		return nil, fmt.Errorf("xunhupay create: %w", err)
	}
	var resp struct {
		ErrCode       any    `json:"errcode"`
		Code          any    `json:"code"`
		ErrMsg        string `json:"errmsg"`
		Msg           string `json:"msg"`
		Message       string `json:"message"`
		OrderID       string `json:"order_id"`
		TransactionID string `json:"transaction_id"`
		URL           string `json:"url"`
		URLQRCode     string `json:"url_qrcode"`
		QRCode        string `json:"qrcode"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("xunhupay parse create: %w", err)
	}
	if !xunhuPayCreateOK(resp.ErrCode, resp.Code, resp.URL, resp.URLQRCode, resp.QRCode) {
		msg := firstNonEmpty(resp.ErrMsg, resp.Msg, resp.Message, strings.TrimSpace(string(body)))
		return nil, fmt.Errorf("xunhupay error: %s", msg)
	}
	tradeNo := firstNonEmpty(resp.OrderID, resp.TransactionID, req.OrderID)
	payURL, qrCode := normalizeXunhuPayCreateURLs(resp.URL, resp.URLQRCode, resp.QRCode)
	return &payment.CreatePaymentResponse{TradeNo: tradeNo, PayURL: payURL, QRCode: qrCode}, nil
}

func (x *XunhuPay) QueryOrder(_ context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	return &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Status:   payment.ProviderStatusPending,
		Metadata: x.MerchantIdentityMetadata(),
	}, nil
}

func (x *XunhuPay) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("parse notify: %w", err)
	}
	params := make(map[string]string)
	for k := range values {
		params[k] = values.Get(k)
	}
	hash := params["hash"]
	if hash == "" {
		return nil, fmt.Errorf("missing hash")
	}
	if !xunhuPayVerifySign(params, x.config["secret"], hash) {
		return nil, fmt.Errorf("invalid signature")
	}
	amount, err := strconv.ParseFloat(params["total_fee"], 64)
	if err != nil || amount <= 0 {
		return nil, fmt.Errorf("invalid total_fee")
	}
	status := payment.ProviderStatusFailed
	switch strings.ToUpper(strings.TrimSpace(params["status"])) {
	case "OD", "SUCCESS", "TRADE_SUCCESS", "COMPLETED":
		status = payment.ProviderStatusSuccess
	}
	metadata := x.MerchantIdentityMetadata()
	if appID := strings.TrimSpace(params["appid"]); appID != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata["appid"] = appID
	}
	return &payment.PaymentNotification{
		TradeNo:  firstNonEmpty(params["transaction_id"], params["order_id"], params["trade_order_id"]),
		OrderID:  params["trade_order_id"],
		Amount:   amount,
		Status:   status,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

func (x *XunhuPay) Refund(_ context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	return &payment.RefundResponse{
		RefundID: firstNonEmpty(req.TradeNo, req.OrderID) + "-refund-unsupported",
		Status:   payment.ProviderStatusFailed,
	}, nil
}

func (x *XunhuPay) post(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := x.httpClient
	if client == nil {
		client = &http.Client{Timeout: xunhuPayHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxXunhuPayResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.Join(strings.Fields(string(body)), " "))
	}
	return body, nil
}

func normalizeXunhuPayCreateURLs(urlValue, urlQRCode, qrCode string) (payURL string, qrContent string) {
	payURL = strings.TrimSpace(urlValue)
	urlQRCode = strings.TrimSpace(urlQRCode)
	qrCode = strings.TrimSpace(qrCode)

	if payURL == "" && isHTTPURL(urlQRCode) && !isRemoteQRURL(urlQRCode) {
		payURL = urlQRCode
		urlQRCode = ""
	}
	if payURL == "" && isHTTPURL(qrCode) && !isRemoteQRURL(qrCode) {
		payURL = qrCode
		qrCode = ""
	}
	return payURL, firstNonEmpty(urlQRCode, qrCode)
}

func isRemoteQRURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	path := strings.ToLower(u.Path)
	if strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") || strings.HasSuffix(path, ".gif") || strings.HasSuffix(path, ".webp") || strings.HasSuffix(path, ".svg") {
		return true
	}
	return strings.Contains(path, "/qrcode/") || strings.Contains(path, "/qr_code/") || strings.Contains(path, "/qr/")
}

func isHTTPURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	return err == nil && u.IsAbs() && (u.Scheme == "http" || u.Scheme == "https")
}

func xunhuPaySign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "hash" || k == "signature" || v == "" {
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
	_, _ = buf.WriteString(secret)
	hash := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(hash[:])
}

func xunhuPayVerifySign(params map[string]string, secret string, sign string) bool {
	return hmac.Equal([]byte(xunhuPaySign(params, secret)), []byte(sign))
}

func xunhuPayCreateOK(errCode any, code any, values ...string) bool {
	if anyNonEmpty(values...) {
		return true
	}
	for _, v := range []any{errCode, code} {
		switch typed := v.(type) {
		case float64:
			if typed == 0 {
				return true
			}
		case string:
			s := strings.ToLower(strings.TrimSpace(typed))
			if s == "0" || s == "success" {
				return true
			}
		}
	}
	return false
}

func anyNonEmpty(values ...string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
