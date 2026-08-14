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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

// NOWPayments constants.
const (
	nowPaymentsHTTPTimeout     = 20 * time.Second
	maxNowPaymentsResponseSize = 1 << 20 // 1MB
	nowPaymentsDefaultAPIBase  = "https://api.nowpayments.io/v1"
	nowPaymentsSignatureHeader = "x-nowpayments-sig"
	maxNowPaymentsErrorSummary = 512
	// An invoice fans out into one payment per deposit, so a page this size
	// covers the original plus any repeated deposits behind it.
	nowPaymentsInvoiceLookupLimit = 100
)

// NOWPayments payment_status values.
const (
	nowPaymentsStatusWaiting       = "waiting"
	nowPaymentsStatusConfirming    = "confirming"
	nowPaymentsStatusConfirmed     = "confirmed"
	nowPaymentsStatusSending       = "sending"
	nowPaymentsStatusPartiallyPaid = "partially_paid"
	nowPaymentsStatusFinished      = "finished"
	nowPaymentsStatusFailed        = "failed"
	nowPaymentsStatusRefunded      = "refunded"
	nowPaymentsStatusExpired       = "expired"
)

// NOWPayments implements payment.Provider for the NOWPayments crypto gateway.
//
// The integration uses hosted invoices rather than raw payments so NOWPayments
// owns coin selection and the exchange-rate countdown; we only ever see a price
// in the configured fiat currency.
type NOWPayments struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewNOWPayments creates a new NOWPayments provider.
// config keys: apiKey, ipnSecret, currency, apiBase, notifyUrl, successUrl,
// cancelUrl, partiallyPaidUrl, isFixedRate, isFeePaidByUser.
func NewNOWPayments(instanceID string, config map[string]string) (*NOWPayments, error) {
	cfg := cloneStringMap(config)
	for _, k := range []string{"apiKey", "ipnSecret"} {
		if strings.TrimSpace(cfg[k]) == "" {
			return nil, fmt.Errorf("nowpayments config missing required key: %s", k)
		}
	}
	currency := payment.CurrencyNowPayments
	if strings.TrimSpace(cfg["currency"]) != "" {
		normalized, err := payment.NormalizePaymentCurrency(cfg["currency"])
		if err != nil {
			return nil, fmt.Errorf("nowpayments config currency: %w", err)
		}
		currency = normalized
	}
	cfg["currency"] = currency
	if strings.TrimSpace(cfg["apiBase"]) == "" {
		cfg["apiBase"] = nowPaymentsDefaultAPIBase
	}
	cfg["apiBase"] = strings.TrimRight(strings.TrimSpace(cfg["apiBase"]), "/")
	return &NOWPayments{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: nowPaymentsHTTPTimeout},
	}, nil
}

func (n *NOWPayments) Name() string        { return "NOWPayments" }
func (n *NOWPayments) ProviderKey() string { return payment.TypeNowPayments }
func (n *NOWPayments) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeNowPayments}
}

func (n *NOWPayments) MerchantIdentityMetadata() map[string]string {
	if n == nil {
		return nil
	}
	return map[string]string{"currency": n.currency()}
}

func (n *NOWPayments) currency() string {
	if n == nil {
		return payment.CurrencyNowPayments
	}
	currency, err := payment.NormalizePaymentCurrency(n.config["currency"])
	if err != nil {
		return payment.CurrencyNowPayments
	}
	return currency
}

type nowPaymentsInvoiceRequest struct {
	PriceAmount      json.RawMessage `json:"price_amount"`
	PriceCurrency    string          `json:"price_currency"`
	OrderID          string          `json:"order_id"`
	OrderDescription string          `json:"order_description,omitempty"`
	IPNCallbackURL   string          `json:"ipn_callback_url,omitempty"`
	SuccessURL       string          `json:"success_url,omitempty"`
	CancelURL        string          `json:"cancel_url,omitempty"`
	PartiallyPaidURL string          `json:"partially_paid_url,omitempty"`
	IsFixedRate      bool            `json:"is_fixed_rate,omitempty"`
	IsFeePaidByUser  bool            `json:"is_fee_paid_by_user,omitempty"`
}

type nowPaymentsInvoiceResponse struct {
	ID            json.Number `json:"id"`
	InvoiceURL    string      `json:"invoice_url"`
	PriceAmount   string      `json:"price_amount"`
	PriceCurrency string      `json:"price_currency"`
}

// CreatePayment opens a hosted NOWPayments invoice and returns its checkout URL.
func (n *NOWPayments) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	currency := n.currency()
	priceAmount, err := nowPaymentsPriceLiteral(req.Amount, currency)
	if err != nil {
		return nil, err
	}

	body := nowPaymentsInvoiceRequest{
		PriceAmount:      priceAmount,
		PriceCurrency:    strings.ToLower(currency),
		OrderID:          req.OrderID,
		OrderDescription: req.Subject,
		IPNCallbackURL:   strings.TrimSpace(n.config["notifyUrl"]),
		SuccessURL:       n.resolveRedirectURL("successUrl", req.ReturnURL),
		CancelURL:        n.resolveRedirectURL("cancelUrl", req.ReturnURL),
		PartiallyPaidURL: strings.TrimSpace(n.config["partiallyPaidUrl"]),
		IsFixedRate:      nowPaymentsBoolConfig(n.config["isFixedRate"]),
		IsFeePaidByUser:  nowPaymentsBoolConfig(n.config["isFeePaidByUser"]),
	}

	raw, err := n.post(ctx, "/invoice", body)
	if err != nil {
		return nil, err
	}
	var invoice nowPaymentsInvoiceResponse
	if err := json.Unmarshal(raw, &invoice); err != nil {
		return nil, fmt.Errorf("nowpayments create payment: parse response: %w", err)
	}
	if strings.TrimSpace(invoice.InvoiceURL) == "" {
		return nil, fmt.Errorf("nowpayments create payment: upstream returned no invoice url")
	}
	return &payment.CreatePaymentResponse{
		TradeNo:  invoice.ID.String(),
		PayURL:   invoice.InvoiceURL,
		IntentID: invoice.ID.String(),
		Currency: currency,
	}, nil
}

// resolveRedirectURL prefers the per-instance override so an admin can pin the
// post-checkout landing page, falling back to the order's return URL.
func (n *NOWPayments) resolveRedirectURL(configKey, fallback string) string {
	if configured := strings.TrimSpace(n.config[configKey]); configured != "" {
		return configured
	}
	return strings.TrimSpace(fallback)
}

// nowPaymentsPriceLiteral renders the order amount as a bare JSON number.
// NOWPayments types price_amount as a number and rejects a quoted string, so
// the value goes over the wire as a literal rather than through a Go string.
func nowPaymentsPriceLiteral(amount, currency string) (json.RawMessage, error) {
	if _, err := payment.AmountToMinorUnit(amount, currency); err != nil {
		return nil, fmt.Errorf("nowpayments create payment: %w", err)
	}
	// AmountToMinorUnit also accepts forms JSON does not (leading +, exponents),
	// so round-trip through decimal to get a plain literal.
	parsed, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil {
		return nil, fmt.Errorf("nowpayments create payment: invalid amount: %s", amount)
	}
	return json.RawMessage(parsed.String()), nil
}

type nowPaymentsPaymentResponse struct {
	PaymentID       json.Number `json:"payment_id"`
	ParentPaymentID json.Number `json:"parent_payment_id"`
	InvoiceID       json.Number `json:"invoice_id"`
	PaymentStatus   string      `json:"payment_status"`
	OrderID         string      `json:"order_id"`
	PriceAmount     json.Number `json:"price_amount"`
	PriceCurrency   string      `json:"price_currency"`
	ActuallyPaid    json.Number `json:"actually_paid"`
	UpdatedAt       string      `json:"updated_at"`
}

type nowPaymentsPaymentListResponse struct {
	Data []nowPaymentsPaymentResponse `json:"data"`
}

// QueryOrder resolves a payment by its NOWPayments payment ID.
//
// Until the buyer picks a coin an invoice has no payment behind it, so the
// identifier we stored at checkout is an invoice ID that this endpoint rejects.
// That miss falls through to the invoice-scoped listing instead of being
// reported as an error, and once a payment exists the caller persists its ID.
func (n *NOWPayments) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, fmt.Errorf("nowpayments query order: trade number is empty")
	}
	raw, status, err := n.get(ctx, "/payment/"+url.PathEscape(tradeNo))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound || status == http.StatusBadRequest {
		return n.queryOrderByInvoice(ctx, tradeNo)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("nowpayments query order: HTTP %d: %s", status, nowPaymentsErrorSummary(raw))
	}

	var parsed nowPaymentsPaymentResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("nowpayments query order: parse response: %w", err)
	}
	return n.queryOrderResponse(&parsed, tradeNo), nil
}

// queryOrderByInvoice finds the payment a hosted invoice produced.
//
// Repeated and wrong-asset deposits land on the same invoice as extra payments
// carrying a parent_payment_id, and NOWPayments warns they can be underpaid, so
// only the original payment is allowed to settle the order.
func (n *NOWPayments) queryOrderByInvoice(ctx context.Context, invoiceID string) (*payment.QueryOrderResponse, error) {
	query := url.Values{}
	query.Set("invoiceId", invoiceID)
	query.Set("limit", strconv.Itoa(nowPaymentsInvoiceLookupLimit))
	query.Set("page", "0")
	raw, status, err := n.get(ctx, "/payment/?"+query.Encode())
	if err != nil {
		return nil, err
	}
	pending := &payment.QueryOrderResponse{
		TradeNo:  invoiceID,
		Status:   payment.ProviderStatusPending,
		Metadata: n.MerchantIdentityMetadata(),
	}
	if status == http.StatusNotFound || status == http.StatusBadRequest {
		return pending, nil
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("nowpayments query order: invoice %s: HTTP %d: %s", invoiceID, status, nowPaymentsErrorSummary(raw))
	}
	var list nowPaymentsPaymentListResponse
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("nowpayments query order: parse invoice payments: %w", err)
	}
	for i := range list.Data {
		if nowPaymentsIdentifier(list.Data[i].ParentPaymentID, "") != "" {
			continue
		}
		return n.queryOrderResponse(&list.Data[i], invoiceID), nil
	}
	return pending, nil
}

// queryOrderResponse projects an upstream payment onto our status contract.
func (n *NOWPayments) queryOrderResponse(parsed *nowPaymentsPaymentResponse, fallbackTradeNo string) *payment.QueryOrderResponse {
	amount, _ := parsed.PriceAmount.Float64()
	metadata := n.MerchantIdentityMetadata()
	if currency, currencyErr := payment.NormalizePaymentCurrency(parsed.PriceCurrency); currencyErr == nil {
		metadata["currency"] = currency
	}
	return &payment.QueryOrderResponse{
		TradeNo:  nowPaymentsIdentifier(parsed.PaymentID, fallbackTradeNo),
		Status:   nowPaymentsQueryStatus(parsed.PaymentStatus),
		Amount:   amount,
		PaidAt:   nowPaymentsPaidAt(parsed.PaymentStatus, parsed.UpdatedAt),
		Metadata: metadata,
	}
}

// VerifyNotification authenticates and parses a NOWPayments IPN callback.
//
// Statuses that are neither terminal-success nor terminal-failure (waiting,
// confirming, …) carry no decision for us, so they return a nil notification
// and the handler acks them.
func (n *NOWPayments) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := n.verifySignature(rawBody, headers); err != nil {
		return nil, err
	}
	var parsed nowPaymentsPaymentResponse
	if err := json.Unmarshal([]byte(rawBody), &parsed); err != nil {
		return nil, fmt.Errorf("nowpayments verify notification: parse body: %w", err)
	}
	// Repeated and wrong-asset deposits arrive as child payments against the
	// original one. NOWPayments settles them at the rate of the day, so their
	// amount need not cover the order — they are acked and left for a human.
	if nowPaymentsIdentifier(parsed.ParentPaymentID, "") != "" {
		return nil, nil
	}
	status, decided := nowPaymentsNotificationStatus(parsed.PaymentStatus)
	if !decided {
		return nil, nil
	}
	orderID := strings.TrimSpace(parsed.OrderID)
	if orderID == "" {
		return nil, fmt.Errorf("nowpayments verify notification: missing order_id")
	}
	amount, _ := parsed.PriceAmount.Float64()
	metadata := n.MerchantIdentityMetadata()
	if currency, currencyErr := payment.NormalizePaymentCurrency(parsed.PriceCurrency); currencyErr == nil {
		metadata["currency"] = currency
	}
	if invoiceID := strings.TrimSpace(parsed.InvoiceID.String()); invoiceID != "" && invoiceID != "0" {
		metadata["invoice_id"] = invoiceID
	}
	return &payment.PaymentNotification{
		TradeNo:  nowPaymentsIdentifier(parsed.PaymentID, ""),
		OrderID:  orderID,
		Amount:   amount,
		Status:   status,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

// verifySignature reproduces NOWPayments' HMAC-SHA512 over the request body
// serialised with its object keys sorted.
func (n *NOWPayments) verifySignature(rawBody string, headers map[string]string) error {
	secret := strings.TrimSpace(n.config["ipnSecret"])
	if secret == "" {
		return fmt.Errorf("nowpayments ipnSecret not configured")
	}
	presented := strings.TrimSpace(headers[nowPaymentsSignatureHeader])
	if presented == "" {
		return fmt.Errorf("nowpayments notification missing %s header", nowPaymentsSignatureHeader)
	}
	canonical, err := nowPaymentsCanonicalJSON(rawBody)
	if err != nil {
		return fmt.Errorf("nowpayments verify notification: %w", err)
	}
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(canonical)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(presented)), []byte(expected)) {
		return fmt.Errorf("nowpayments verify notification: signature mismatch")
	}
	return nil
}

// nowPaymentsCanonicalJSON re-serialises a JSON body the way NOWPayments signs
// it: every object's keys in ascending order, no insignificant whitespace, and
// the original numeric literals preserved so 1.0 does not become 1.
//
// The upstream signature is produced by JSON.stringify, which leaves <, > and &
// alone, so HTML escaping stays off — otherwise any order description
// containing one of them would fail to verify.
func nowPaymentsCanonicalJSON(rawBody string) ([]byte, error) {
	decoder := json.NewDecoder(strings.NewReader(rawBody))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("parse body: %w", err)
	}
	var buf bytes.Buffer
	if err := writeCanonicalJSON(&buf, value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonicalJSON(buf *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalScalar(buf, key); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := writeCanonicalJSON(buf, typed[key]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case []any:
		buf.WriteByte('[')
		for i, item := range typed {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalJSON(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	default:
		return writeCanonicalScalar(buf, value)
	}
}

// writeCanonicalScalar encodes one JSON scalar without HTML escaping.
func writeCanonicalScalar(buf *bytes.Buffer, value any) error {
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return err
	}
	// Encode terminates every value with a newline that has no place here.
	buf.Truncate(buf.Len() - 1)
	return nil
}

// nowPaymentsNotificationStatus maps an IPN status onto a payment decision.
// The second return value is false while the payment is still in flight.
func nowPaymentsNotificationStatus(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case nowPaymentsStatusFinished:
		return payment.ProviderStatusSuccess, true
	case nowPaymentsStatusFailed, nowPaymentsStatusExpired, nowPaymentsStatusRefunded:
		return payment.ProviderStatusFailed, true
	default:
		return "", false
	}
}

// nowPaymentsQueryStatus maps a polled status. Unlike the IPN path, polling has
// to answer with something for every state, so in-flight maps to pending.
func nowPaymentsQueryStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case nowPaymentsStatusFinished:
		return payment.ProviderStatusPaid
	case nowPaymentsStatusFailed, nowPaymentsStatusExpired:
		return payment.ProviderStatusFailed
	case nowPaymentsStatusRefunded:
		return payment.ProviderStatusRefunded
	case nowPaymentsStatusWaiting, nowPaymentsStatusConfirming, nowPaymentsStatusConfirmed,
		nowPaymentsStatusSending, nowPaymentsStatusPartiallyPaid:
		return payment.ProviderStatusPending
	default:
		return payment.ProviderStatusPending
	}
}

func nowPaymentsPaidAt(status, updatedAt string) string {
	if strings.ToLower(strings.TrimSpace(status)) != nowPaymentsStatusFinished {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(updatedAt))
	if err != nil {
		return ""
	}
	return parsed.Format(time.RFC3339)
}

func nowPaymentsIdentifier(value json.Number, fallback string) string {
	id := strings.TrimSpace(value.String())
	if id != "" && id != "0" {
		return id
	}
	return strings.TrimSpace(fallback)
}

func nowPaymentsBoolConfig(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), "true")
}

func (n *NOWPayments) post(ctx context.Context, path string, body any) ([]byte, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("nowpayments request: marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.config["apiBase"]+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("nowpayments request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	raw, status, err := n.do(req)
	if err != nil {
		return nil, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("nowpayments request %s: HTTP %d: %s", path, status, nowPaymentsErrorSummary(raw))
	}
	return raw, nil
}

func (n *NOWPayments) get(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.config["apiBase"]+path, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("nowpayments request: %w", err)
	}
	return n.do(req)
}

func (n *NOWPayments) do(req *http.Request) ([]byte, int, error) {
	req.Header.Set("x-api-key", strings.TrimSpace(n.config["apiKey"]))
	client := n.httpClient
	if client == nil {
		client = &http.Client{Timeout: nowPaymentsHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("nowpayments request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxNowPaymentsResponseSize))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("nowpayments request: read body: %w", err)
	}
	return raw, resp.StatusCode, nil
}

// nowPaymentsErrorSummary extracts the upstream message when there is one,
// falling back to a whitespace-collapsed body excerpt.
func nowPaymentsErrorSummary(raw []byte) string {
	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &parsed); err == nil && strings.TrimSpace(parsed.Message) != "" {
		return strings.TrimSpace(parsed.Message)
	}
	summary := strings.Join(strings.Fields(string(raw)), " ")
	if summary == "" {
		return "<empty>"
	}
	if len(summary) > maxNowPaymentsErrorSummary {
		return summary[:maxNowPaymentsErrorSummary] + "..."
	}
	return summary
}

// Ensure interface compliance.
var (
	_ payment.Provider                 = (*NOWPayments)(nil)
	_ payment.MerchantIdentityProvider = (*NOWPayments)(nil)
)
