package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	infiniProdAPIBase     = "https://openapi.infini.money"
	infiniSandboxAPIBase  = "https://openapi-sandbox.infini.money"
	infiniProdHost        = "openapi.infini.money"
	infiniSandboxHost     = "openapi-sandbox.infini.money"
	infiniDefaultCurrency = "USD"

	infiniHTTPTimeout = 15 * time.Second
	infiniDialTimeout = 10 * time.Second

	// infiniMaxRedirectURLLen bounds success_url/failure_url. Infini forwards
	// them verbatim to the selected payment rail, and Binance Pay rejects a
	// cancelUrl of 256 characters or more.
	infiniMaxRedirectURLLen = 255
	infiniMaxResponseSize   = 1 << 20
	infiniMaxErrorSummary   = 512

	// infiniWebhookTolerance must cover Infini's full retry backoff (8 attempts
	// spanning roughly 16 minutes), because the signed timestamp belongs to the
	// original delivery and is not refreshed on retry.
	infiniWebhookTolerance = 30 * time.Minute

	infiniSignatureAlgorithm = "hmac-sha256"
	infiniSignedHeaders      = "@request-target date"

	infiniOrderPath = "/v1/acquiring/order"

	infiniWebhookSignatureHeader = "x-webhook-signature"
	infiniWebhookTimestampHeader = "x-webhook-timestamp"
	infiniWebhookEventIDHeader   = "x-webhook-event-id"

	infiniEventOrderCompleted   = "order.completed"
	infiniEventOrderLatePayment = "order.late_payment"
	infiniEventOrderExpired     = "order.expired"

	infiniOrderStatusPending     = "pending"
	infiniOrderStatusProcessing  = "processing"
	infiniOrderStatusPaid        = "paid"
	infiniOrderStatusPartialPaid = "partial_paid"
	infiniOrderStatusExpired     = "expired"
)

// Infini integrates Infini's hosted crypto checkout: an order is created
// upstream and the payer is redirected to the returned checkout URL.
type Infini struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

func NewInfini(instanceID string, config map[string]string) (*Infini, error) {
	for _, k := range []string{"keyId", "secretKey", "webhookSecret", "apiBase"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("infini config missing required key: %s", k)
		}
	}
	cfg := cloneStringMap(config)
	apiBase, err := normalizeInfiniAPIBase(cfg["apiBase"])
	if err != nil {
		return nil, err
	}
	cfg["apiBase"] = apiBase
	currency, err := normalizeInfiniCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("infini config currency: %w", err)
	}
	cfg["currency"] = currency
	return &Infini{
		instanceID: instanceID,
		config:     cfg,
		httpClient: newInfiniHTTPClient(),
	}, nil
}

// newInfiniHTTPClient prefers IPv4. Infini authorises merchants by source IP,
// and on a dual-stack host the default dialer may leave over IPv6, whose
// address is rarely the one registered in the whitelist.
func newInfiniHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: infiniDialTimeout, KeepAlive: 30 * time.Second}
	return &http.Client{
		Timeout: infiniHTTPTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if network == "tcp" {
					if conn, err := dialer.DialContext(ctx, "tcp4", addr); err == nil {
						return conn, nil
					}
				}
				return dialer.DialContext(ctx, network, addr)
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

// shortenInfiniRedirectURL trims a redirect URL down to what every payment rail
// accepts. The resume token is dropped first: the payment result page still
// resolves the order from out_trade_no without it.
func shortenInfiniRedirectURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) <= infiniMaxRedirectURLLen {
		return trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	for _, key := range []string{"resume_token", "status", "out_trade_no"} {
		query := parsed.Query()
		query.Del(key)
		parsed.RawQuery = query.Encode()
		if len(parsed.String()) <= infiniMaxRedirectURLLen {
			return parsed.String()
		}
	}
	parsed.RawQuery = ""
	if candidate := parsed.String(); len(candidate) <= infiniMaxRedirectURLLen {
		return candidate
	}
	return ""
}

func normalizeInfiniAPIBase(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "", fmt.Errorf("infini apiBase is required")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("infini apiBase must be an HTTPS URL")
	}
	host := strings.ToLower(parsed.Host)
	if host != infiniProdHost && host != infiniSandboxHost {
		return "", fmt.Errorf("infini apiBase host must be %s or %s", infiniProdHost, infiniSandboxHost)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawPath = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path != "" {
		return "", fmt.Errorf("infini apiBase must not carry a path")
	}
	return parsed.String(), nil
}

// normalizeInfiniCurrency defaults to USD rather than the platform-wide CNY,
// because Infini prices orders in foreign currency only.
func normalizeInfiniCurrency(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return infiniDefaultCurrency, nil
	}
	return payment.NormalizePaymentCurrency(raw)
}

func (i *Infini) Name() string        { return "Infini" }
func (i *Infini) ProviderKey() string { return payment.TypeInfini }
func (i *Infini) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeInfini}
}

func (i *Infini) MerchantIdentityMetadata() map[string]string {
	if i == nil {
		return nil
	}
	metadata := map[string]string{"currency": i.currency()}
	if keyID := strings.TrimSpace(i.config["keyId"]); keyID != "" {
		metadata["key_id"] = keyID
	}
	return metadata
}

func (i *Infini) currency() string {
	if i == nil {
		return infiniDefaultCurrency
	}
	currency, err := normalizeInfiniCurrency(i.config["currency"])
	if err != nil {
		return infiniDefaultCurrency
	}
	return currency
}

func (i *Infini) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("infini create payment: invalid amount %s", req.Amount)
	}
	orderID := strings.TrimSpace(req.OrderID)
	if orderID == "" {
		return nil, fmt.Errorf("infini create payment: missing order id")
	}

	currency := i.currency()
	redirectURL := shortenInfiniRedirectURL(req.ReturnURL)
	payload := infiniCreateOrderRequest{
		Amount:          amount.String(),
		Currency:        currency,
		RequestID:       infiniDeterministicRequestID("order", orderID, amount.String(), currency),
		ClientReference: orderID,
		OrderDesc:       strings.TrimSpace(req.Subject),
		SuccessURL:      redirectURL,
		FailureURL:      redirectURL,
		Email:           strings.TrimSpace(req.PayerEmail),
	}
	if req.ExpiresIn > 0 {
		payload.ExpiresIn = req.ExpiresIn
	}

	var order infiniOrder
	if err := i.doJSON(ctx, http.MethodPost, infiniOrderPath, payload, &order); err != nil {
		return nil, fmt.Errorf("infini create order: %w", err)
	}
	if strings.TrimSpace(order.OrderID) == "" || strings.TrimSpace(order.CheckoutURL) == "" {
		return nil, fmt.Errorf("infini create order: missing order_id or checkout_url")
	}
	return &payment.CreatePaymentResponse{
		TradeNo:    order.OrderID,
		PayURL:     order.CheckoutURL,
		Currency:   currency,
		ResultType: payment.CreatePaymentResultOrderCreated,
	}, nil
}

func (i *Infini) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	orderID := strings.TrimSpace(tradeNo)
	if orderID == "" {
		return nil, fmt.Errorf("infini query order: missing order id")
	}
	path := infiniOrderPath + "?order_id=" + url.QueryEscape(orderID)

	var order infiniOrder
	if err := i.doJSON(ctx, http.MethodGet, path, nil, &order); err != nil {
		return nil, fmt.Errorf("infini query order: %w", err)
	}
	return &payment.QueryOrderResponse{
		TradeNo:  order.OrderID,
		Status:   infiniProviderStatus(order.Status, order.Amount, order.AmountConfirmed),
		Amount:   infiniDecimal(order.Amount).InexactFloat64(),
		Metadata: infiniOrderMetadata(order.Status, order.Currency, order.AmountConfirming, order.AmountConfirmed, order.ExceptionTags),
	}, nil
}

func (i *Infini) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := verifyInfiniWebhookSignature(rawBody, headers, i.config["webhookSecret"], time.Now()); err != nil {
		return nil, err
	}

	var event infiniWebhookOrderEvent
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("infini parse webhook: %w", err)
	}
	eventName := strings.ToLower(strings.TrimSpace(event.Event))
	switch eventName {
	case infiniEventOrderCompleted, infiniEventOrderLatePayment, infiniEventOrderExpired:
	default:
		return nil, nil
	}

	orderID := strings.TrimSpace(event.OrderID)
	clientReference := strings.TrimSpace(event.ClientReference)
	if orderID == "" || clientReference == "" {
		return nil, fmt.Errorf("infini webhook missing order_id or client_reference")
	}
	amount := infiniDecimal(event.Amount)
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("infini webhook invalid amount: %s", event.Amount)
	}

	notification := &payment.PaymentNotification{
		TradeNo:  orderID,
		OrderID:  clientReference,
		Amount:   amount.InexactFloat64(),
		RawData:  rawBody,
		Metadata: infiniOrderMetadata(event.Status, event.Currency, event.AmountConfirming, event.AmountConfirmed, event.ExceptionTags),
	}
	status := strings.ToLower(strings.TrimSpace(event.Status))

	switch eventName {
	case infiniEventOrderCompleted:
		if status != infiniOrderStatusPaid {
			return nil, fmt.Errorf("infini completed webhook has non-paid status: %s", event.Status)
		}
		notification.Status = payment.NotificationStatusSuccess
		return notification, nil

	case infiniEventOrderLatePayment:
		// A late payment only credits the order when the confirmed on-chain
		// amount covers it. Amount stays the order amount so an overpayment
		// still matches the order; the surplus goes through Infini's refund
		// claim flow.
		if infiniDecimal(event.AmountConfirmed).LessThan(amount) {
			notification.Status = payment.ProviderStatusFailed
			notification.Anomaly = payment.NotificationAnomalyPartialPaid
			return notification, nil
		}
		notification.Status = payment.NotificationStatusSuccess
		return notification, nil

	default:
		// order.expired carries an anomaly only when funds did arrive but fell
		// short; a plain unpaid expiry needs no record.
		if status != infiniOrderStatusPartialPaid {
			return nil, nil
		}
		notification.Status = payment.ProviderStatusFailed
		notification.Anomaly = payment.NotificationAnomalyPartialPaid
		return notification, nil
	}
}

// Refund is unsupported: Infini exposes no merchant-initiated refund API.
// Short or excess payments are refunded by the payer claiming them by email.
func (i *Infini) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, infraerrors.BadRequest("INFINI_REFUND_UNSUPPORTED", "infini does not support merchant-initiated refunds")
}

func infiniOrderMetadata(status, currency string, confirming, confirmed infiniAmount, exceptionTags []string) map[string]string {
	metadata := map[string]string{}
	if v := strings.TrimSpace(status); v != "" {
		metadata["status"] = strings.ToLower(v)
	}
	if v := strings.ToUpper(strings.TrimSpace(currency)); v != "" {
		metadata["currency"] = v
	}
	if v := strings.TrimSpace(confirming.String()); v != "" {
		metadata["amount_confirming"] = v
	}
	if v := strings.TrimSpace(confirmed.String()); v != "" {
		metadata["amount_confirmed"] = v
	}
	if len(exceptionTags) > 0 {
		metadata["exception_tags"] = strings.Join(exceptionTags, ",")
	}
	return metadata
}

func infiniProviderStatus(status string, amount, amountConfirmed infiniAmount) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case infiniOrderStatusPaid:
		return payment.ProviderStatusPaid
	case infiniOrderStatusPending, infiniOrderStatusProcessing:
		return payment.ProviderStatusPending
	case infiniOrderStatusExpired, infiniOrderStatusPartialPaid:
		// Funds confirmed after expiry still settle the order; anything short
		// of the order amount does not.
		want := infiniDecimal(amount)
		if want.GreaterThan(decimal.Zero) && infiniDecimal(amountConfirmed).GreaterThanOrEqual(want) {
			return payment.ProviderStatusPaid
		}
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

// infiniDeterministicRequestID derives a stable UUID so a retried create-order
// call reuses the same upstream idempotency key.
func infiniDeterministicRequestID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	var id uuid.UUID
	copy(id[:], hash[:16])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}

// infiniSigningString builds the string Infini signs:
//
//	{keyId}\n{METHOD} {path}\ndate: {GMT}\n
//
// The trailing newline is part of the signed bytes. The request target keeps its
// query string; the body, Digest and Content-Type stay out of the signature.
func infiniSigningString(keyID, method, pathWithQuery, date string) string {
	return keyID + "\n" + strings.ToUpper(strings.TrimSpace(method)) + " " + pathWithQuery + "\ndate: " + date + "\n"
}

func infiniSignature(secret, signingString string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingString))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func infiniAuthorizationHeader(keyID, signature string) string {
	return fmt.Sprintf(`Signature keyId="%s",algorithm="%s",headers="%s",signature="%s"`,
		keyID, infiniSignatureAlgorithm, infiniSignedHeaders, signature)
}

func infiniDigestHeader(body []byte) string {
	sum := sha256.Sum256(body)
	return "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
}

func verifyInfiniWebhookSignature(rawBody string, headers map[string]string, secret string, now time.Time) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return fmt.Errorf("infini webhookSecret not configured")
	}
	timestamp := strings.TrimSpace(headers[infiniWebhookTimestampHeader])
	eventID := strings.TrimSpace(headers[infiniWebhookEventIDHeader])
	signature := strings.ToLower(strings.TrimSpace(headers[infiniWebhookSignatureHeader]))
	if timestamp == "" || eventID == "" || signature == "" {
		return fmt.Errorf("infini notification missing webhook signature headers")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + eventID + "." + rawBody))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("infini invalid signature")
	}

	ts, err := parseInfiniWebhookTimestamp(timestamp)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now()
	}
	if diff := now.Sub(ts).Abs(); diff > infiniWebhookTolerance {
		return fmt.Errorf("infini webhook timestamp outside tolerance")
	}
	return nil
}

// parseInfiniWebhookTimestamp accepts both second and millisecond precision
// Unix timestamps.
func parseInfiniWebhookTimestamp(raw string) (time.Time, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return time.Time{}, fmt.Errorf("infini invalid webhook timestamp")
	}
	if value >= 1e12 {
		return time.UnixMilli(value), nil
	}
	return time.Unix(value, 0), nil
}

func (i *Infini) doJSON(ctx context.Context, method, pathWithQuery string, payload any, out any) error {
	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = encoded
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, i.config["apiBase"]+pathWithQuery, bodyReader)
	if err != nil {
		return err
	}

	keyID := strings.TrimSpace(i.config["keyId"])
	date := time.Now().UTC().Format(http.TimeFormat)
	signature := infiniSignature(i.config["secretKey"], infiniSigningString(keyID, method, pathWithQuery, date))
	req.Header.Set("Date", date)
	req.Header.Set("Authorization", infiniAuthorizationHeader(keyID, signature))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Digest", infiniDigestHeader(body))
	}

	respBody, status, err := i.do(req)
	if err != nil {
		return err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return formatInfiniHTTPError(status, respBody)
	}
	if out == nil || len(bytes.TrimSpace(respBody)) == 0 {
		return nil
	}
	return decodeInfiniPayload(status, respBody, out)
}

// decodeInfiniPayload unwraps Infini's {code,message,data} envelope. A non-zero
// code is an error even under HTTP 200; responses that carry the business object
// at the top level are decoded as-is.
func decodeInfiniPayload(status int, body []byte, out any) error {
	var envelope infiniEnvelope
	payload := body
	if err := json.Unmarshal(body, &envelope); err == nil {
		if envelope.Code != 0 {
			return formatInfiniEnvelopeError(status, envelope)
		}
		if data := bytes.TrimSpace(envelope.Data); len(data) > 0 && !bytes.Equal(data, []byte("null")) {
			payload = envelope.Data
		}
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

func (i *Infini) do(req *http.Request) ([]byte, int, error) {
	client := i.httpClient
	if client == nil {
		client = &http.Client{Timeout: infiniHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, infiniMaxResponseSize))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func formatInfiniHTTPError(status int, body []byte) error {
	var envelope infiniEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && (envelope.Code != 0 || envelope.Message != "") {
		return formatInfiniEnvelopeError(status, envelope)
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("HTTP %d: %s; Infini rejected the credentials, check Key ID/Secret Key, the API Base environment (sandbox: %s, production: %s) and that the server clock is within 300s of Infini's",
			status, summarizeInfiniResponse(body), infiniSandboxAPIBase, infiniProdAPIBase)
	}
	return fmt.Errorf("HTTP %d: %s", status, summarizeInfiniResponse(body))
}

func formatInfiniEnvelopeError(status int, envelope infiniEnvelope) error {
	message := strings.TrimSpace(envelope.Message)
	if detail := strings.TrimSpace(envelope.Detail); detail != "" {
		message += ": " + detail
	}
	if envelope.Code == 0 {
		return fmt.Errorf("HTTP %d: %s", status, message)
	}
	return fmt.Errorf("HTTP %d: code=%d %s", status, envelope.Code, message)
}

func summarizeInfiniResponse(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if summary == "" {
		return "<empty>"
	}
	if len(summary) > infiniMaxErrorSummary {
		return summary[:infiniMaxErrorSummary] + "..."
	}
	return summary
}

// infiniAmount decodes an amount that Infini may send as a JSON string or a
// JSON number.
type infiniAmount string

func (a *infiniAmount) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*a = ""
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		*a = infiniAmount(strings.TrimSpace(decoded))
		return nil
	}
	*a = infiniAmount(trimmed)
	return nil
}

func (a infiniAmount) String() string { return string(a) }

func infiniDecimal(raw infiniAmount) decimal.Decimal {
	value, err := decimal.NewFromString(strings.TrimSpace(raw.String()))
	if err != nil {
		return decimal.Zero
	}
	return value
}

type infiniCreateOrderRequest struct {
	Amount          string `json:"amount"`
	Currency        string `json:"currency"`
	RequestID       string `json:"request_id"`
	ClientReference string `json:"client_reference"`
	OrderDesc       string `json:"order_desc,omitempty"`
	ExpiresIn       int    `json:"expires_in,omitempty"`
	SuccessURL      string `json:"success_url,omitempty"`
	FailureURL      string `json:"failure_url,omitempty"`
	Email           string `json:"email,omitempty"`
}

type infiniOrder struct {
	OrderID          string       `json:"order_id"`
	RequestID        string       `json:"request_id"`
	CheckoutURL      string       `json:"checkout_url"`
	ClientReference  string       `json:"client_reference"`
	Status           string       `json:"status"`
	Amount           infiniAmount `json:"amount"`
	Currency         string       `json:"currency"`
	AmountConfirming infiniAmount `json:"amount_confirming"`
	AmountConfirmed  infiniAmount `json:"amount_confirmed"`
	ExpiresAt        int64        `json:"expires_at"`
	CreatedAt        int64        `json:"created_at"`
	ExceptionTags    []string     `json:"exception_tags"`
}

type infiniWebhookOrderEvent struct {
	Event            string       `json:"event"`
	OrderID          string       `json:"order_id"`
	ClientReference  string       `json:"client_reference"`
	Status           string       `json:"status"`
	Amount           infiniAmount `json:"amount"`
	Currency         string       `json:"currency"`
	AmountConfirming infiniAmount `json:"amount_confirming"`
	AmountConfirmed  infiniAmount `json:"amount_confirmed"`
	ExceptionTags    []string     `json:"exception_tags"`
}

// infiniEnvelope is Infini's common response wrapper. Successful calls carry
// code 0 and put the business object in data.
type infiniEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Detail  string          `json:"detail"`
	Data    json.RawMessage `json:"data"`
}
