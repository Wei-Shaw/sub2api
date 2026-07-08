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

// Epusdt constants.
const (
	epusdtHTTPTimeout      = 15 * time.Second
	epusdtMaxResponseSize  = 1 << 20 // 1MB
	epusdtMaxErrorSummary  = 512
	epusdtSuccessCode      = 200
	epusdtStatusPending    = 1
	epusdtStatusPaid       = 2
	epusdtStatusExpired    = 3
	epusdtStatusWaitSelect = 4

	epusdtCreatePath      = "/payments/gmpay/v1/order/create-transaction"
	epusdtCheckStatusPath = "/pay/check-status/"
)

// Epusdt implements payment.Provider for the epusdt crypto payment gateway,
// using its native GMPay JSON API. epusdt returns a hosted cashier URL
// (payment_url); the buyer is redirected there to pay in USDT/crypto and the
// order is confirmed via the async notify callback.
type Epusdt struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewEpusdt creates a new Epusdt provider.
// config keys: pid, secretKey, apiBase, notifyUrl (required); returnUrl, token,
// network, currency (optional). token/network must be set together or both left
// empty (empty => epusdt creates a placeholder order and the buyer picks the
// chain on the cashier).
func NewEpusdt(instanceID string, config map[string]string) (*Epusdt, error) {
	for _, k := range []string{"pid", "secretKey", "apiBase", "notifyUrl"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("epusdt config missing required key: %s", k)
		}
	}
	cfg := cloneStringMap(config)
	apiBase, err := normalizeEpusdtAPIBase(cfg["apiBase"])
	if err != nil {
		return nil, err
	}
	cfg["apiBase"] = apiBase

	token := strings.TrimSpace(strings.ToLower(cfg["token"]))
	network := strings.TrimSpace(strings.ToLower(cfg["network"]))
	if (token == "") != (network == "") {
		return nil, fmt.Errorf("epusdt config token and network must be set together or both left empty")
	}
	cfg["token"] = token
	cfg["network"] = network

	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("epusdt config currency: %w", err)
	}
	cfg["currency"] = currency

	return &Epusdt{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: epusdtHTTPTimeout},
	}, nil
}

func normalizeEpusdtAPIBase(apiBase string) (string, error) {
	base := strings.TrimSpace(apiBase)
	if base == "" {
		return "", fmt.Errorf("epusdt apiBase is required")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("epusdt apiBase must be an absolute URL (e.g. https://pay.example.com)")
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/"), nil
}

func (e *Epusdt) apiBase() string {
	if e == nil {
		return ""
	}
	return e.config["apiBase"]
}

func (e *Epusdt) Name() string        { return "加密货币" }
func (e *Epusdt) ProviderKey() string { return payment.TypeEpusdt }
func (e *Epusdt) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeEpusdt}
}

func (e *Epusdt) MerchantIdentityMetadata() map[string]string {
	if e == nil {
		return nil
	}
	pid := strings.TrimSpace(e.config["pid"])
	if pid == "" {
		return nil
	}
	return map[string]string{"pid": pid}
}

func (e *Epusdt) currency() string {
	currency, err := payment.NormalizePaymentCurrency(e.config["currency"])
	if err != nil {
		return payment.DefaultPaymentCurrency
	}
	return currency
}

// CreatePayment creates a GMPay transaction and returns the hosted cashier URL.
func (e *Epusdt) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = e.config["notifyUrl"]
	}
	returnURL := req.ReturnURL
	if returnURL == "" {
		returnURL = e.config["returnUrl"]
	}

	params := map[string]string{
		"pid":          e.config["pid"],
		"order_id":     req.OrderID,
		"currency":     strings.ToLower(e.currency()),
		"amount":       req.Amount,
		"notify_url":   notifyURL,
		"redirect_url": returnURL,
		"name":         req.Subject,
	}
	// token/network are always set together or both empty (validated in NewEpusdt).
	if token := e.config["token"]; token != "" {
		params["token"] = token
		params["network"] = e.config["network"]
	}
	params["signature"] = epusdtSign(params, e.config["secretKey"])

	body, err := e.post(ctx, e.apiBase()+epusdtCreatePath, params)
	if err != nil {
		return nil, fmt.Errorf("epusdt create: %w", err)
	}
	var resp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       struct {
			TradeID    string `json:"trade_id"`
			PaymentURL string `json:"payment_url"`
			Status     int    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("epusdt parse create: %w", err)
	}
	if resp.StatusCode != epusdtSuccessCode {
		return nil, fmt.Errorf("epusdt create failed (%d): %s", resp.StatusCode, epusdtMessageOrSummary(resp.Message, body))
	}
	if strings.TrimSpace(resp.Data.PaymentURL) == "" {
		return nil, fmt.Errorf("epusdt create: missing payment_url")
	}
	return &payment.CreatePaymentResponse{
		TradeNo: resp.Data.TradeID,
		PayURL:  resp.Data.PaymentURL,
	}, nil
}

// QueryOrder queries the payment status via the check-status endpoint.
func (e *Epusdt) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	tradeID := strings.TrimSpace(tradeNo)
	if tradeID == "" {
		return nil, fmt.Errorf("epusdt query order: missing trade id")
	}
	body, err := e.get(ctx, e.apiBase()+epusdtCheckStatusPath+url.PathEscape(tradeID))
	if err != nil {
		return nil, fmt.Errorf("epusdt query order: %w", err)
	}
	var resp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       struct {
			TradeID string `json:"trade_id"`
			Status  int    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("epusdt parse query: %w", err)
	}
	if resp.StatusCode != epusdtSuccessCode {
		return nil, fmt.Errorf("epusdt query failed (%d): %s", resp.StatusCode, epusdtMessageOrSummary(resp.Message, body))
	}
	returnedTradeNo := tradeID
	if strings.TrimSpace(resp.Data.TradeID) != "" {
		returnedTradeNo = resp.Data.TradeID
	}
	return &payment.QueryOrderResponse{
		TradeNo:  returnedTradeNo,
		Status:   epusdtProviderStatus(resp.Data.Status),
		Metadata: e.MerchantIdentityMetadata(),
	}, nil
}

// VerifyNotification parses and verifies a GMPay JSON callback.
func (e *Epusdt) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	params, err := epusdtDecodeNotification(rawBody)
	if err != nil {
		return nil, err
	}
	sign := params["signature"]
	if sign == "" {
		return nil, fmt.Errorf("epusdt notify missing signature")
	}
	if !epusdtVerifySign(params, e.config["secretKey"], sign) {
		return nil, fmt.Errorf("epusdt notify invalid signature")
	}

	status := payment.ProviderStatusFailed
	if statusVal, _ := strconv.Atoi(params["status"]); statusVal == epusdtStatusPaid {
		status = payment.NotificationStatusSuccess
	}
	amount, _ := strconv.ParseFloat(params["amount"], 64)

	metadata := e.MerchantIdentityMetadata()
	if pid := strings.TrimSpace(params["pid"]); pid != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata["pid"] = pid
	}
	return &payment.PaymentNotification{
		TradeNo:  params["trade_id"],
		OrderID:  params["order_id"],
		Amount:   amount,
		Status:   status,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

// Refund is not supported by epusdt (crypto payments are non-refundable).
func (e *Epusdt) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("epusdt does not support refunds")
}

// epusdtDecodeNotification parses the GMPay JSON callback body into a string map.
// json.Decoder.UseNumber preserves the numeric literals exactly as epusdt sent
// (and signed) them, so the recomputed signature matches.
func epusdtDecodeNotification(rawBody string) (map[string]string, error) {
	dec := json.NewDecoder(strings.NewReader(rawBody))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("epusdt parse notify: %w", err)
	}
	params := make(map[string]string, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case nil:
			continue
		case string:
			params[k] = val
		case json.Number:
			params[k] = val.String()
		case bool:
			params[k] = strconv.FormatBool(val)
		default:
			b, err := json.Marshal(val)
			if err != nil {
				return nil, fmt.Errorf("epusdt notify unsupported field %q", k)
			}
			params[k] = string(b)
		}
	}
	return params, nil
}

func epusdtProviderStatus(status int) string {
	switch status {
	case epusdtStatusPaid:
		return payment.ProviderStatusPaid
	case epusdtStatusExpired:
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func (e *Epusdt) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return e.do(req)
}

func (e *Epusdt) post(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return e.do(req)
}

func (e *Epusdt) do(req *http.Request) ([]byte, error) {
	client := e.httpClient
	if client == nil {
		client = &http.Client{Timeout: epusdtHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, epusdtMaxResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarizeEpusdtResponse(body))
	}
	return body, nil
}

func epusdtMessageOrSummary(message string, body []byte) string {
	if msg := strings.TrimSpace(message); msg != "" {
		return msg
	}
	return summarizeEpusdtResponse(body)
}

func summarizeEpusdtResponse(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if summary == "" {
		return "<empty>"
	}
	if len(summary) > epusdtMaxErrorSummary {
		return summary[:epusdtMaxErrorSummary] + "..."
	}
	return summary
}

// epusdtSign implements the GMPay signature: non-empty params sorted by key
// ASCII, joined as k1=v1&k2=v2, with the secret_key appended directly, then MD5
// (lowercase hex). Only "signature" is excluded.
func epusdtSign(params map[string]string, secretKey string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "signature" || v == "" {
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
	_, _ = buf.WriteString(secretKey)
	hash := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(hash[:])
}

func epusdtVerifySign(params map[string]string, secretKey string, sign string) bool {
	return hmac.Equal([]byte(epusdtSign(params, secretKey)), []byte(sign))
}

var (
	_ payment.Provider                 = (*Epusdt)(nil)
	_ payment.MerchantIdentityProvider = (*Epusdt)(nil)
)
