package provider

import (
	"context"
	"crypto/md5"
	"crypto/rand"
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
	hupijiaoDefaultAPIBase  = "https://api.xunhupay.com"
	hupijiaoHTTPTimeout     = 10 * time.Second
	maxHupijiaoResponseSize = 1 << 20
	hupijiaoVersion         = "1.1"
	hupijiaoStatusPaid      = "OD"
	hupijiaoStatusRefunded  = "CD"
	hupijiaoStatusPending   = "WP"
	hupijiaoStatusRefunding = "RD"
	hupijiaoStatusFailed    = "UD"
)

// Hupijiao implements the native Hupijiao payment API.
type Hupijiao struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewHupijiao creates a Hupijiao provider.
// Required config keys: appid, appsecret, apiBase, notifyUrl, returnUrl.
func NewHupijiao(instanceID string, config map[string]string) (*Hupijiao, error) {
	for _, key := range []string{"appid", "appsecret", "apiBase", "notifyUrl", "returnUrl"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("hupijiao config missing required key: %s", key)
		}
	}
	cfg := make(map[string]string, len(config))
	for key, value := range config {
		cfg[key] = value
	}
	cfg["apiBase"] = normalizeHupijiaoAPIBase(cfg["apiBase"])
	return &Hupijiao{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: hupijiaoHTTPTimeout},
	}, nil
}

func normalizeHupijiaoAPIBase(raw string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		return hupijiaoDefaultAPIBase
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(base, "/")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawPath = ""
	for _, endpoint := range []string{"/payment/do.html", "/payment/query.html", "/payment/refund.html"} {
		if strings.HasSuffix(strings.ToLower(parsed.Path), endpoint) {
			parsed.Path = strings.TrimRight(parsed.Path[:len(parsed.Path)-len(endpoint)], "/")
			break
		}
	}
	return strings.TrimRight(parsed.String(), "/")
}

func (h *Hupijiao) apiBase() string {
	if h == nil {
		return ""
	}
	return normalizeHupijiaoAPIBase(h.config["apiBase"])
}

func (h *Hupijiao) Name() string        { return "Hupijiao" }
func (h *Hupijiao) ProviderKey() string { return payment.TypeHupijiao }
func (h *Hupijiao) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay}
}

func (h *Hupijiao) MerchantIdentityMetadata() map[string]string {
	if h == nil || strings.TrimSpace(h.config["appid"]) == "" {
		return nil
	}
	return map[string]string{"appid": strings.TrimSpace(h.config["appid"])}
}

func (h *Hupijiao) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := h.resolveURLs(req)
	nonce := hupijiaoNonce()
	params := map[string]string{
		"version":        hupijiaoVersion,
		"lang":           "zh-cn",
		"appid":          h.config["appid"],
		"trade_order_id": req.OrderID,
		"total_fee":      req.Amount,
		"title":          truncateHupijiaoTitle(req.Subject),
		"time":           strconv.FormatInt(time.Now().Unix(), 10),
		"notify_url":     notifyURL,
		"return_url":     returnURL,
		"nonce_str":      nonce,
	}
	if req.PaymentType == payment.TypeWxpay {
		params["payment"] = "wechat"
	} else {
		params["payment"] = "alipay"
	}
	if req.PaymentType == payment.TypeWxpay && req.IsMobile {
		params["type"] = "WAP"
		params["wap_url"] = hupijiaoWAPOrigin(returnURL)
		params["wap_name"] = "Sub2API"
	}
	params["hash"] = hupijiaoHash(params, h.config["appsecret"])

	body, err := h.postJSON(ctx, "/payment/do.html", params)
	if err != nil {
		return nil, fmt.Errorf("hupijiao create: %w", err)
	}
	var resp hupijiaoCreateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("hupijiao parse create response: %w", err)
	}
	if resp.ErrCode != 0 {
		return nil, fmt.Errorf("hupijiao create error %d: %s", resp.ErrCode, resp.ErrMsg)
	}
	if err := h.verifyResponseHash(resp.rawFields(), resp.Hash); err != nil {
		return nil, fmt.Errorf("hupijiao create response signature: %w", err)
	}
	tradeNo := strings.TrimSpace(resp.OpenOrderID)
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(hupijiaoScalarString(resp.OpenID))
	}
	if tradeNo == "" {
		tradeNo = req.OrderID
	}
	if strings.TrimSpace(resp.URL) == "" && strings.TrimSpace(resp.URLQRCode) == "" {
		return nil, fmt.Errorf("hupijiao create response has no payment URL")
	}
	payURL := resp.URL
	if req.IsMobile && payURL == "" {
		payURL = resp.URLQRCode
	}
	// The frontend renders QRCode as a QR payload. Hupijiao's url_qrcode is a
	// provider-side QR endpoint intended to be displayed directly on desktop,
	// so encoding it again makes scanners open that endpoint instead of the
	// payment page. The url endpoint redirects to the appropriate mobile flow.
	qrCode := resp.URL
	if strings.TrimSpace(qrCode) == "" {
		qrCode = resp.URLQRCode
	}
	return &payment.CreatePaymentResponse{TradeNo: tradeNo, PayURL: payURL, QRCode: qrCode}, nil
}

func (h *Hupijiao) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	params := map[string]string{
		"appid":           h.config["appid"],
		"out_trade_order": strings.TrimSpace(tradeNo),
		"time":            strconv.FormatInt(time.Now().Unix(), 10),
		"nonce_str":       hupijiaoNonce(),
	}
	params["hash"] = hupijiaoHash(params, h.config["appsecret"])
	body, err := h.postForm(ctx, "/payment/query.html", params)
	if err != nil {
		return nil, fmt.Errorf("hupijiao query: %w", err)
	}
	var resp hupijiaoQueryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("hupijiao parse query response: %w", err)
	}
	if resp.ErrCode != 0 {
		return nil, fmt.Errorf("hupijiao query error %d: %s", resp.ErrCode, resp.ErrMsg)
	}
	// The official Hupijiao SDK does not verify the query response hash because
	// the response payload nests the order fields under data. The request itself
	// is authenticated with appid/appsecret/hash, and the HTTPS API response is
	// still checked for a successful HTTP/application status below.
	status := payment.ProviderStatusPending
	switch strings.ToUpper(strings.TrimSpace(resp.Data.Status)) {
	case hupijiaoStatusPaid:
		status = payment.ProviderStatusPaid
	case hupijiaoStatusRefunded:
		status = payment.ProviderStatusRefunded
	case hupijiaoStatusFailed:
		status = payment.ProviderStatusFailed
	case hupijiaoStatusRefunding:
		status = payment.ProviderStatusPending
	case hupijiaoStatusPending:
		status = payment.ProviderStatusPending
	}
	responseTradeNo := strings.TrimSpace(resp.Data.OpenOrderID)
	if responseTradeNo == "" {
		responseTradeNo = strings.TrimSpace(resp.Data.TransactionID)
	}
	if responseTradeNo == "" {
		responseTradeNo = tradeNo
	}
	amount, _ := strconv.ParseFloat(strings.TrimSpace(resp.Data.TotalFee), 64)
	metadata := h.MerchantIdentityMetadata()
	if metadata == nil {
		metadata = map[string]string{}
	}
	metadata["status"] = strings.TrimSpace(resp.Data.Status)
	return &payment.QueryOrderResponse{TradeNo: responseTradeNo, Status: status, Amount: amount, Metadata: metadata}, nil
}

func (h *Hupijiao) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("parse notify: %w", err)
	}
	params := make(map[string]string, len(values))
	for key := range values {
		params[key] = values.Get(key)
	}
	sign := strings.TrimSpace(params["hash"])
	if sign == "" {
		return nil, fmt.Errorf("missing hash")
	}
	if !hupijiaoVerifyHash(params, h.config["appsecret"], sign) {
		return nil, fmt.Errorf("invalid hash")
	}
	amount, _ := strconv.ParseFloat(strings.TrimSpace(params["total_fee"]), 64)
	status := payment.ProviderStatusFailed
	if strings.EqualFold(strings.TrimSpace(params["status"]), hupijiaoStatusPaid) {
		status = payment.ProviderStatusSuccess
	}
	metadata := h.MerchantIdentityMetadata()
	if metadata == nil {
		metadata = map[string]string{}
	}
	if appid := strings.TrimSpace(params["appid"]); appid != "" {
		metadata["appid"] = appid
	}
	metadata["status"] = strings.TrimSpace(params["status"])
	return &payment.PaymentNotification{
		TradeNo:  firstNonEmpty(params["transaction_id"], params["open_order_id"]),
		OrderID:  params["trade_order_id"],
		Amount:   amount,
		Status:   status,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

func (h *Hupijiao) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	orderID := strings.TrimSpace(req.OrderID)
	if orderID == "" {
		orderID = strings.TrimSpace(req.TradeNo)
	}
	if orderID == "" {
		return nil, fmt.Errorf("hupijiao refund missing order identifier")
	}
	params := map[string]string{
		"appid":          h.config["appid"],
		"trade_order_id": orderID,
		"reason":         strings.TrimSpace(req.Reason),
		"time":           strconv.FormatInt(time.Now().Unix(), 10),
		"nonce_str":      hupijiaoNonce(),
	}
	params["hash"] = hupijiaoHash(params, h.config["appsecret"])
	body, err := h.postForm(ctx, "/payment/refund.html", params)
	if err != nil {
		return nil, fmt.Errorf("hupijiao refund: %w", err)
	}
	var resp hupijiaoRefundResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("hupijiao parse refund response: %w", err)
	}
	if resp.ErrCode != 0 {
		return nil, fmt.Errorf("hupijiao refund error %d: %s", resp.ErrCode, resp.ErrMsg)
	}
	if err := h.verifyResponseHash(resp.rawFields(), resp.Hash); err != nil {
		return nil, fmt.Errorf("hupijiao refund response signature: %w", err)
	}
	status := payment.ProviderStatusFailed
	switch strings.ToUpper(strings.TrimSpace(resp.RefundStatus)) {
	case hupijiaoStatusRefunded:
		status = payment.ProviderStatusSuccess
	case hupijiaoStatusRefunding:
		status = payment.ProviderStatusPending
	}
	return &payment.RefundResponse{
		RefundID: firstNonEmpty(resp.OutRefundNo, resp.TransactionID, orderID),
		Status:   status,
	}, nil
}

type hupijiaoCreateResponse struct {
	OpenID      json.RawMessage `json:"openid"`
	OpenOrderID string          `json:"open_order_id"`
	URLQRCode   string          `json:"url_qrcode"`
	URL         string          `json:"url"`
	ErrCode     int             `json:"errcode"`
	ErrMsg      string          `json:"errmsg"`
	Hash        string          `json:"hash"`
}

func (r hupijiaoCreateResponse) rawFields() map[string]string {
	return map[string]string{"openid": hupijiaoScalarString(r.OpenID), "open_order_id": r.OpenOrderID, "url_qrcode": r.URLQRCode, "url": r.URL, "errcode": strconv.Itoa(r.ErrCode), "errmsg": r.ErrMsg}
}

// hupijiaoScalarString accepts scalar fields that the API may return as either
// JSON strings or JSON numbers (notably openid).
func hupijiaoScalarString(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	return ""
}

type hupijiaoQueryData struct {
	Status        string `json:"status"`
	OpenOrderID   string `json:"open_order_id"`
	TransactionID string `json:"transaction_id"`
	TotalFee      string `json:"total_fee"`
}

type hupijiaoQueryResponse struct {
	ErrCode int               `json:"errcode"`
	ErrMsg  string            `json:"errmsg"`
	Data    hupijiaoQueryData `json:"data"`
	Hash    string            `json:"hash"`
}

func (r hupijiaoQueryResponse) rawFields() map[string]string {
	return map[string]string{"errcode": strconv.Itoa(r.ErrCode), "errmsg": r.ErrMsg, "status": r.Data.Status, "open_order_id": r.Data.OpenOrderID, "transaction_id": r.Data.TransactionID, "total_fee": r.Data.TotalFee}
}

type hupijiaoRefundResponse struct {
	TradeOrderID  string `json:"trade_order_id"`
	TransactionID string `json:"transaction_id"`
	OutRefundNo   string `json:"out_refund_no"`
	RefundStatus  string `json:"refund_status"`
	RefundFee     string `json:"refund_fee"`
	ErrCode       int    `json:"errcode"`
	ErrMsg        string `json:"errmsg"`
	Hash          string `json:"hash"`
}

func (r hupijiaoRefundResponse) rawFields() map[string]string {
	return map[string]string{"trade_order_id": r.TradeOrderID, "transaction_id": r.TransactionID, "out_refund_no": r.OutRefundNo, "refund_status": r.RefundStatus, "refund_fee": r.RefundFee, "errcode": strconv.Itoa(r.ErrCode), "errmsg": r.ErrMsg}
}

func (h *Hupijiao) resolveURLs(req payment.CreatePaymentRequest) (string, string) {
	notifyURL := strings.TrimSpace(req.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(h.config["notifyUrl"])
	}
	returnURL := strings.TrimSpace(req.ReturnURL)
	if returnURL == "" {
		returnURL = strings.TrimSpace(h.config["returnUrl"])
	}
	return notifyURL, returnURL
}

func (h *Hupijiao) postJSON(ctx context.Context, path string, params map[string]string) ([]byte, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.apiBase()+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := h.httpClient
	if client == nil {
		client = &http.Client{Timeout: hupijiaoHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxHupijiaoResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarizeHupijiaoResponse(responseBody))
	}
	return responseBody, nil
}

func (h *Hupijiao) postForm(ctx context.Context, path string, params map[string]string) ([]byte, error) {
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.apiBase()+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := h.httpClient
	if client == nil {
		client = &http.Client{Timeout: hupijiaoHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxHupijiaoResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarizeHupijiaoResponse(responseBody))
	}
	return responseBody, nil
}

func (h *Hupijiao) verifyResponseHash(fields map[string]string, provided string) error {
	if strings.TrimSpace(provided) == "" {
		return nil
	}
	if !hupijiaoVerifyHash(fields, h.config["appsecret"], provided) {
		return fmt.Errorf("invalid response hash")
	}
	return nil
}

func hupijiaoHash(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "hash" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for index, key := range keys {
		if index > 0 {
			_ = builder.WriteByte('&')
		}
		_, _ = builder.WriteString(key)
		_ = builder.WriteByte('=')
		_, _ = builder.WriteString(params[key])
	}
	_, _ = builder.WriteString(secret)
	digest := md5.Sum([]byte(builder.String()))
	return hex.EncodeToString(digest[:])
}

func hupijiaoVerifyHash(params map[string]string, secret, provided string) bool {
	expected := hupijiaoHash(params, secret)
	return strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(provided))
}

func hupijiaoNonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func hupijiaoWAPOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func truncateHupijiaoTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "Sub2API Recharge"
	}
	if len([]rune(title)) <= 32 {
		return title
	}
	return string([]rune(title)[:32])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func summarizeHupijiaoResponse(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if len(text) > 512 {
		return text[:512] + "..."
	}
	if text == "" {
		return "<empty>"
	}
	return text
}
