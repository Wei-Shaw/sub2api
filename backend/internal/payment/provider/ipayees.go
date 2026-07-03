package provider

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// iPayees constants.
const (
	ipayeesHTTPTimeout     = 20 * time.Second
	maxIPayeesResponseSize = 1 << 20 // 1MB
	ipayeesStatusCompleted = "completed"
	ipayeesReturnRedirect  = "redirect"
	// ipayeesAPIKeyHeader is the (lower-cased) header iPayees echoes back on
	// webhooks and expects on API requests. The webhook handler lower-cases all
	// header keys before calling VerifyNotification, so we match in lower case.
	ipayeesAPIKeyHeader = "ip-ipayees-api-key"
)

// IPayees implements payment.Provider for the iPayees payment gateway
// (https://github.com/asifsofficial/ipayees). It is a hosted-redirect gateway:
// CreatePayment returns a URL the browser is redirected to, where the payer
// picks bKash/Nagad/card/etc. on iPayees' own checkout page.
//
// iPayees webhooks carry NO cryptographic signature — they only echo the shared
// API key in a header. Header equality alone is a weak guarantee, so
// VerifyNotification treats the webhook purely as a trigger and re-verifies the
// authoritative status/amount via the verify-payments API before crediting.
type IPayees struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewIPayees creates a new iPayees provider.
// config keys: apiBase (required), apiKey (required, secret), currency (required),
// contact (required by gateways that mandate email_mobile), customerName (optional).
func NewIPayees(instanceID string, config map[string]string) (*IPayees, error) {
	for _, k := range []string{"apiBase", "apiKey", "currency"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("ipayees config missing required key: %s", k)
		}
	}
	cfg := make(map[string]string, len(config))
	for k, v := range config {
		cfg[k] = v
	}
	cfg["apiBase"] = normalizeIPayeesAPIBase(cfg["apiBase"])
	if cfg["apiBase"] == "" {
		return nil, fmt.Errorf("ipayees config invalid apiBase")
	}
	return &IPayees{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: ipayeesHTTPTimeout},
	}, nil
}

// normalizeIPayeesAPIBase trims trailing slashes and any known endpoint suffix,
// leaving the ".../api" base that create-charge / verify-payments hang off.
func normalizeIPayeesAPIBase(apiBase string) string {
	base := strings.TrimSpace(apiBase)
	if base == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	path := strings.TrimRight(parsed.Path, "/")
	lower := strings.ToLower(path)
	for _, endpoint := range []string{"/create-charge", "/verify-payments"} {
		if strings.HasSuffix(lower, endpoint) {
			path = strings.TrimRight(path[:len(path)-len(endpoint)], "/")
			break
		}
	}
	parsed.Path = path
	return strings.TrimRight(parsed.String(), "/")
}

func (p *IPayees) Name() string        { return "iPayees" }
func (p *IPayees) ProviderKey() string { return payment.TypeIPayees }
func (p *IPayees) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeIPayees}
}

func (p *IPayees) currency() string {
	return strings.ToUpper(strings.TrimSpace(p.config["currency"]))
}

// CreatePayment initiates a hosted charge and returns the redirect URL.
func (p *IPayees) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := p.resolveURLs(req)
	if strings.TrimSpace(notifyURL) == "" || strings.TrimSpace(returnURL) == "" {
		return nil, fmt.Errorf("ipayees create: missing notify/return URL")
	}
	returnType := strings.TrimSpace(p.config["returnType"])
	if returnType == "" {
		returnType = ipayeesReturnRedirect
	}
	body := map[string]any{
		"full_name":    p.customerName(),
		"amount":       req.Amount,
		"currency":     p.currency(),
		"metadata":     map[string]string{"invoiceid": req.OrderID},
		"redirect_url": returnURL,
		"cancel_url":   returnURL,
		"webhook_url":  notifyURL,
		"return_type":  returnType,
		"product_name": strings.TrimSpace(req.Subject),
	}
	// Many iPayees deployments mandate email_mobile; send a merchant-controlled
	// fallback contact when configured so create-charge does not 400.
	if contact := strings.TrimSpace(p.config["contact"]); contact != "" {
		body["email_mobile"] = contact
	}

	raw, status, err := p.postJSON(ctx, p.endpoint("create-charge"), body)
	if err != nil {
		return nil, fmt.Errorf("ipayees create: %w", err)
	}
	var resp struct {
		Status  any    `json:"status"`
		ID      any    `json:"id"`
		URL     string `json:"url"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("ipayees create: bad response (HTTP %d): %s", status, summarizeIPayeesResponse(raw))
	}
	if !ipayeesTruthy(resp.Status) {
		msg := strings.TrimSpace(resp.Message)
		if msg == "" {
			msg = summarizeIPayeesResponse(raw)
		}
		return nil, fmt.Errorf("ipayees create failed (HTTP %d): %s", status, msg)
	}
	id := ipayeesStringify(resp.ID)
	if id == "" || strings.TrimSpace(resp.URL) == "" {
		return nil, fmt.Errorf("ipayees create: missing id/url in response")
	}
	return &payment.CreatePaymentResponse{
		TradeNo:  id,
		PayURL:   strings.TrimSpace(resp.URL),
		Currency: p.currency(),
	}, nil
}

// QueryOrder checks payment status via verify-payments.
func (p *IPayees) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	res, err := p.verify(ctx, tradeNo)
	if err != nil {
		return nil, fmt.Errorf("ipayees query: %w", err)
	}
	return &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Status:   res.providerStatus(),
		Amount:   res.amountFloat(),
		Metadata: res.metadata(),
	}, nil
}

// VerifyNotification handles an iPayees webhook. The webhook body is NOT trusted:
// after a constant-time API-key header check, the authoritative status/amount is
// re-fetched via verify-payments and returned for the service layer to reconcile
// against the order amount.
func (p *IPayees) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := p.checkAPIKey(headers); err != nil {
		return nil, err
	}
	var hook struct {
		ID       any `json:"id"`
		Metadata struct {
			InvoiceID any `json:"invoiceid"`
			OrderID   any `json:"order_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(rawBody), &hook); err != nil {
		return nil, fmt.Errorf("ipayees notify: bad json: %w", err)
	}
	chargeID := ipayeesStringify(hook.ID)
	if chargeID == "" {
		return nil, fmt.Errorf("ipayees notify: missing id")
	}
	orderID := ipayeesStringify(hook.Metadata.InvoiceID)
	if orderID == "" {
		orderID = ipayeesStringify(hook.Metadata.OrderID)
	}
	if orderID == "" {
		return nil, fmt.Errorf("ipayees notify: missing metadata.invoiceid")
	}

	// Authoritative re-verification — never trust the webhook's own status/amount.
	res, err := p.verify(ctx, chargeID)
	if err != nil {
		return nil, fmt.Errorf("ipayees notify verify: %w", err)
	}
	status := payment.ProviderStatusFailed
	if res.isPaid() {
		status = payment.ProviderStatusSuccess
	}
	return &payment.PaymentNotification{
		TradeNo:  chargeID,
		OrderID:  orderID,
		Amount:   res.amountFloat(),
		Status:   status,
		RawData:  rawBody,
		Metadata: res.metadata(),
	}, nil
}

// Refund is not supported by the iPayees API.
func (p *IPayees) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("ipayees does not support automated refunds")
}

// --- helpers ---

func (p *IPayees) customerName() string {
	if v := strings.TrimSpace(p.config["customerName"]); v != "" {
		return v
	}
	return "Customer"
}

func (p *IPayees) resolveURLs(req payment.CreatePaymentRequest) (string, string) {
	notifyURL := req.NotifyURL
	if strings.TrimSpace(notifyURL) == "" {
		notifyURL = p.config["notifyUrl"]
	}
	returnURL := req.ReturnURL
	if strings.TrimSpace(returnURL) == "" {
		returnURL = p.config["returnUrl"]
	}
	return notifyURL, returnURL
}

func (p *IPayees) endpoint(name string) string {
	return normalizeIPayeesAPIBase(p.config["apiBase"]) + "/" + name
}

// checkAPIKey validates the shared secret echoed in the webhook header using a
// constant-time comparison. Header keys are lower-cased by the caller.
func (p *IPayees) checkAPIKey(headers map[string]string) error {
	got := ""
	for _, h := range []string{ipayeesAPIKeyHeader, "http_ip_ipayees_api_key"} {
		if v := strings.TrimSpace(headers[h]); v != "" {
			got = v
			break
		}
	}
	if got == "" {
		return fmt.Errorf("ipayees notify: missing api key header")
	}
	want := strings.TrimSpace(p.config["apiKey"])
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return fmt.Errorf("ipayees notify: api key mismatch")
	}
	return nil
}

// ipayeesVerifyResult is the parsed verify-payments response.
type ipayeesVerifyResult struct {
	Status        string `json:"status"`
	TransactionID string `json:"transaction_id"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	Method        string `json:"method"`
}

func (r *ipayeesVerifyResult) isPaid() bool {
	return strings.EqualFold(strings.TrimSpace(r.Status), ipayeesStatusCompleted)
}

func (r *ipayeesVerifyResult) providerStatus() string {
	switch strings.ToLower(strings.TrimSpace(r.Status)) {
	case ipayeesStatusCompleted:
		return payment.ProviderStatusPaid
	case "failed", "cancelled", "canceled", "expired", "declined":
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func (r *ipayeesVerifyResult) amountFloat() float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(r.Amount), 64)
	return v
}

func (r *ipayeesVerifyResult) metadata() map[string]string {
	m := map[string]string{}
	if c := strings.ToUpper(strings.TrimSpace(r.Currency)); c != "" {
		m["currency"] = c
	}
	if r.Method != "" {
		m["method"] = strings.TrimSpace(r.Method)
	}
	if r.TransactionID != "" {
		m["transaction_id"] = strings.TrimSpace(r.TransactionID)
	}
	return m
}

func (p *IPayees) verify(ctx context.Context, chargeID string) (*ipayeesVerifyResult, error) {
	if strings.TrimSpace(chargeID) == "" {
		return nil, fmt.Errorf("empty charge id")
	}
	raw, status, err := p.postJSON(ctx, p.endpoint("verify-payments"), map[string]any{"id": chargeID})
	if err != nil {
		return nil, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("verify HTTP %d: %s", status, summarizeIPayeesResponse(raw))
	}
	var res ipayeesVerifyResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("verify bad json (HTTP %d): %s", status, summarizeIPayeesResponse(raw))
	}
	return &res, nil
}

func (p *IPayees) postJSON(ctx context.Context, endpoint string, payload map[string]any) ([]byte, int, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(buf)))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(ipayeesAPIKeyHeader, strings.TrimSpace(p.config["apiKey"]))
	client := p.httpClient
	if client == nil {
		client = &http.Client{Timeout: ipayeesHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(io.LimitReader(resp.Body, maxIPayeesResponseSize))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

// ipayeesTruthy interprets the create-charge "status" field, which may be a bool
// (true) or a string ("true"/"success").
func ipayeesTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "success" || s == "1"
	case float64:
		return t == 1
	default:
		return false
	}
}

// ipayeesStringify renders id-like fields that may arrive as string or number.
func ipayeesStringify(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func summarizeIPayeesResponse(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if summary == "" {
		return "<empty>"
	}
	if len(summary) > 512 {
		return summary[:512] + "..."
	}
	return summary
}
