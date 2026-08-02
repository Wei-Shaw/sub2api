package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

const (
	lemonSqueezyDefaultAPIBase      = "https://api.lemonsqueezy.com"
	lemonSqueezyHTTPTimeout         = 15 * time.Second
	lemonSqueezyMaxResponseSize     = 1 << 20
	lemonSqueezyEventOrderCreated   = "order_created"
	lemonSqueezyEventOrderRefunded  = "order_refunded"
	lemonSqueezyOrderStatusPaid     = "paid"
	lemonSqueezyOrderStatusPending  = "pending"
	lemonSqueezyOrderStatusFailed   = "failed"
	lemonSqueezyOrderStatusRefunded = "refunded"
	lemonSqueezyOrderStatusPartial  = "partial_refund"
)

type LemonSqueezy struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

func NewLemonSqueezy(instanceID string, config map[string]string) (*LemonSqueezy, error) {
	for _, key := range []string{"apiKey", "storeId", "variantId", "webhookSecret"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("lemonsqueezy config missing required key: %s", key)
		}
	}

	cfg := cloneStringMap(config)
	apiBase, err := normalizeLemonSqueezyAPIBase(cfg["apiBase"])
	if err != nil {
		return nil, err
	}
	cfg["apiBase"] = apiBase

	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("lemonsqueezy config currency: %w", err)
	}
	cfg["currency"] = currency

	if _, err := strconv.ParseInt(strings.TrimSpace(cfg["storeId"]), 10, 64); err != nil {
		return nil, fmt.Errorf("lemonsqueezy config storeId must be a numeric ID")
	}
	if _, err := strconv.ParseInt(strings.TrimSpace(cfg["variantId"]), 10, 64); err != nil {
		return nil, fmt.Errorf("lemonsqueezy config variantId must be a numeric ID")
	}

	return &LemonSqueezy{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: lemonSqueezyHTTPTimeout},
	}, nil
}

func normalizeLemonSqueezyAPIBase(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		return lemonSqueezyDefaultAPIBase, nil
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("lemonsqueezy apiBase must be a valid URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("lemonsqueezy apiBase must use http or https")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawPath = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(parsed.Path, "/v1") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/v1")
	}
	return parsed.String(), nil
}

func (l *LemonSqueezy) Name() string        { return "Lemon Squeezy" }
func (l *LemonSqueezy) ProviderKey() string { return payment.TypeLemonSqueezy }
func (l *LemonSqueezy) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeLemonSqueezy}
}

func (l *LemonSqueezy) MerchantIdentityMetadata() map[string]string {
	if l == nil {
		return nil
	}
	return map[string]string{
		"store_id":   strings.TrimSpace(l.config["storeId"]),
		"variant_id": strings.TrimSpace(l.config["variantId"]),
		"currency":   l.currency(),
	}
}

func (l *LemonSqueezy) currency() string {
	if l == nil {
		return payment.DefaultPaymentCurrency
	}
	currency, err := payment.NormalizePaymentCurrency(l.config["currency"])
	if err != nil {
		return payment.DefaultPaymentCurrency
	}
	return currency
}

func (l *LemonSqueezy) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	storeID, _ := strconv.ParseInt(strings.TrimSpace(l.config["storeId"]), 10, 64)
	variantID, _ := strconv.ParseInt(strings.TrimSpace(l.config["variantId"]), 10, 64)
	currency := l.currency()
	customPrice, err := payment.AmountToMinorUnit(req.Amount, currency)
	if err != nil {
		return nil, fmt.Errorf("lemonsqueezy create payment: %w", err)
	}

	payload := lemonSqueezyCheckoutCreateRequest{
		Data: lemonSqueezyCheckoutCreateData{
			Type: "checkouts",
			Attributes: lemonSqueezyCheckoutCreateAttributes{
				CheckoutData: lemonSqueezyCheckoutData{
					Custom: map[string]any{
						"order_id":     req.OrderID,
						"payment_type": req.PaymentType,
					},
				},
				CheckoutOptions: lemonSqueezyCheckoutOptions{
					Embed: false,
				},
				ProductOptions: lemonSqueezyProductOptions{
					Name:              strings.TrimSpace(req.Subject),
					Description:       strings.TrimSpace(req.Subject),
					RedirectURL:       strings.TrimSpace(req.ReturnURL),
					ReceiptButtonText: "Return",
					ReceiptLinkURL:    strings.TrimSpace(req.ReturnURL),
					EnabledVariants:   []int64{variantID},
				},
				CustomPrice: customPrice,
				ExpiresAt:   time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
			},
			Relationships: lemonSqueezyCheckoutRelationships{
				Store:   lemonSqueezyRelationship{Data: lemonSqueezyRelationshipData{Type: "stores", ID: strconv.FormatInt(storeID, 10)}},
				Variant: lemonSqueezyRelationship{Data: lemonSqueezyRelationshipData{Type: "variants", ID: strconv.FormatInt(variantID, 10)}},
			},
		},
	}

	var resp lemonSqueezyCheckoutResponse
	if err := l.doJSON(ctx, http.MethodPost, "/v1/checkouts", payload, &resp); err != nil {
		return nil, fmt.Errorf("lemonsqueezy create payment: %w", err)
	}
	if strings.TrimSpace(resp.Data.ID) == "" || strings.TrimSpace(resp.Data.Attributes.URL) == "" {
		return nil, fmt.Errorf("lemonsqueezy create payment: missing checkout id or url")
	}

	return &payment.CreatePaymentResponse{
		TradeNo:  strings.TrimSpace(resp.Data.ID),
		PayURL:   strings.TrimSpace(resp.Data.Attributes.URL),
		Currency: currency,
	}, nil
}

func (l *LemonSqueezy) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	ref := strings.TrimSpace(tradeNo)
	if ref == "" {
		return nil, fmt.Errorf("lemonsqueezy query order: missing trade number")
	}
	if _, err := strconv.ParseInt(ref, 10, 64); err == nil {
		var resp lemonSqueezyOrderResponse
		if err := l.doJSON(ctx, http.MethodGet, "/v1/orders/"+url.PathEscape(ref), nil, &resp); err != nil {
			return nil, fmt.Errorf("lemonsqueezy query order: %w", err)
		}
		return l.orderQueryResponse(resp.Data), nil
	}

	var resp lemonSqueezyCheckoutLookupResponse
	if err := l.doJSON(ctx, http.MethodGet, "/v1/checkouts/"+url.PathEscape(ref), nil, &resp); err != nil {
		return nil, fmt.Errorf("lemonsqueezy query checkout: %w", err)
	}
	return &payment.QueryOrderResponse{
		TradeNo: ref,
		Status:  payment.ProviderStatusPending,
		Amount:  0,
		Metadata: map[string]string{
			"currency": l.currency(),
			"store_id": strings.TrimSpace(l.config["storeId"]),
		},
	}, nil
}

func (l *LemonSqueezy) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	signature := strings.TrimSpace(headers["x-signature"])
	if signature == "" {
		return nil, fmt.Errorf("lemonsqueezy notification missing x-signature header")
	}
	expected := lemonSqueezyWebhookSignature(rawBody, l.config["webhookSecret"])
	if !hmac.Equal([]byte(strings.ToLower(signature)), []byte(expected)) {
		return nil, fmt.Errorf("lemonsqueezy verify notification: signature mismatch")
	}

	var event lemonSqueezyWebhookEvent
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("lemonsqueezy parse webhook: %w", err)
	}

	switch strings.TrimSpace(event.Meta.EventName) {
	case lemonSqueezyEventOrderCreated:
		orderID := strings.TrimSpace(stringValue(event.Meta.CustomData["order_id"]))
		if orderID == "" {
			return nil, fmt.Errorf("lemonsqueezy webhook missing custom_data.order_id")
		}
		if strings.TrimSpace(event.Data.ID) == "" {
			return nil, fmt.Errorf("lemonsqueezy webhook missing order id")
		}
		currency := normalizeLemonSqueezyCurrency(event.Data.Attributes.Currency, l.currency())
		return &payment.PaymentNotification{
			TradeNo: strings.TrimSpace(event.Data.ID),
			OrderID: orderID,
			Amount:  lemonSqueezyAmount(event.Data.Attributes.Total, currency),
			Status:  payment.NotificationStatusSuccess,
			RawData: rawBody,
			Metadata: map[string]string{
				"currency":   currency,
				"status":     strings.TrimSpace(event.Data.Attributes.Status),
				"store_id":   strconv.FormatInt(event.Data.Attributes.StoreID, 10),
				"variant_id": strconv.FormatInt(event.Data.Attributes.FirstOrderItem.VariantID, 10),
			},
		}, nil
	case lemonSqueezyEventOrderRefunded:
		return nil, nil
	default:
		return nil, nil
	}
}

func (l *LemonSqueezy) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	orderID := strings.TrimSpace(req.TradeNo)
	if orderID == "" {
		return nil, fmt.Errorf("lemonsqueezy refund missing order id")
	}
	currency := l.currency()
	refundAmount, err := payment.AmountToMinorUnit(req.Amount, currency)
	if err != nil {
		return nil, fmt.Errorf("lemonsqueezy refund: %w", err)
	}

	payload := lemonSqueezyRefundRequest{
		Data: lemonSqueezyRefundRequestData{
			Type: "orders",
			ID:   orderID,
			Attributes: lemonSqueezyRefundAttributes{
				Amount: refundAmount,
			},
		},
	}

	var resp lemonSqueezyOrderResponse
	if err := l.doJSON(ctx, http.MethodPost, "/v1/orders/"+url.PathEscape(orderID)+"/refund", payload, &resp); err != nil {
		return nil, fmt.Errorf("lemonsqueezy refund: %w", err)
	}

	status := lemonSqueezyRefundStatus(resp.Data.Attributes)
	return &payment.RefundResponse{
		RefundID: orderID,
		Status:   status,
	}, nil
}

func (l *LemonSqueezy) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	orderID := strings.TrimSpace(req.RefundID)
	if orderID == "" {
		orderID = strings.TrimSpace(req.TradeNo)
	}
	if orderID == "" {
		return nil, fmt.Errorf("lemonsqueezy query refund: missing order id")
	}
	var resp lemonSqueezyOrderResponse
	if err := l.doJSON(ctx, http.MethodGet, "/v1/orders/"+url.PathEscape(orderID), nil, &resp); err != nil {
		return nil, fmt.Errorf("lemonsqueezy query refund: %w", err)
	}
	return &payment.RefundResponse{
		RefundID: orderID,
		Status:   lemonSqueezyRefundStatus(resp.Data.Attributes),
	}, nil
}

func (l *LemonSqueezy) orderQueryResponse(order lemonSqueezyOrderData) *payment.QueryOrderResponse {
	currency := normalizeLemonSqueezyCurrency(order.Attributes.Currency, l.currency())
	return &payment.QueryOrderResponse{
		TradeNo: strings.TrimSpace(order.ID),
		Status:  lemonSqueezyProviderStatus(order.Attributes),
		Amount:  lemonSqueezyAmount(order.Attributes.Total, currency),
		Metadata: map[string]string{
			"currency":   currency,
			"status":     strings.TrimSpace(order.Attributes.Status),
			"store_id":   strconv.FormatInt(order.Attributes.StoreID, 10),
			"variant_id": strconv.FormatInt(order.Attributes.FirstOrderItem.VariantID, 10),
		},
	}
}

func lemonSqueezyProviderStatus(attrs lemonSqueezyOrderAttributes) string {
	status := strings.ToLower(strings.TrimSpace(attrs.Status))
	switch {
	case attrs.Refunded || status == lemonSqueezyOrderStatusRefunded:
		return payment.ProviderStatusRefunded
	case status == lemonSqueezyOrderStatusPartial:
		return payment.ProviderStatusRefunded
	case status == lemonSqueezyOrderStatusPaid:
		return payment.ProviderStatusPaid
	case status == lemonSqueezyOrderStatusFailed:
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func lemonSqueezyRefundStatus(attrs lemonSqueezyOrderAttributes) string {
	status := strings.ToLower(strings.TrimSpace(attrs.Status))
	if attrs.Refunded || status == lemonSqueezyOrderStatusRefunded || status == lemonSqueezyOrderStatusPartial || lemonSqueezyAmount(attrs.RefundedAmount, "") > 0 {
		return payment.ProviderStatusSuccess
	}
	if status == lemonSqueezyOrderStatusFailed {
		return payment.ProviderStatusFailed
	}
	return payment.ProviderStatusPending
}

func normalizeLemonSqueezyCurrency(raw, fallback string) string {
	currency, err := payment.NormalizePaymentCurrency(raw)
	if err == nil {
		return currency
	}
	currency, err = payment.NormalizePaymentCurrency(fallback)
	if err == nil {
		return currency
	}
	return payment.DefaultPaymentCurrency
}

func lemonSqueezyAmount(raw json.Number, currency string) float64 {
	value := strings.TrimSpace(raw.String())
	if value == "" {
		return 0
	}
	if strings.Contains(value, ".") {
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return parsed
		}
		return 0
	}
	minor, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return payment.MinorUnitToAmount(minor, normalizeLemonSqueezyCurrency(currency, payment.DefaultPaymentCurrency))
}

func (l *LemonSqueezy) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, l.config["apiBase"]+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.api+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(l.config["apiKey"]))
	if payload != nil {
		req.Header.Set("Content-Type", "application/vnd.api+json")
	}

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	limitedBody := io.LimitReader(resp.Body, lemonSqueezyMaxResponseSize)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(limitedBody)
		return fmt.Errorf("lemonsqueezy api %s %s failed: %s", method, path, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	decoder := json.NewDecoder(limitedBody)
	decoder.UseNumber()
	return decoder.Decode(out)
}

func lemonSqueezyWebhookSignature(rawBody, secret string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(secret)))
	_, _ = mac.Write([]byte(rawBody))
	return hex.EncodeToString(mac.Sum(nil))
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

type lemonSqueezyCheckoutCreateRequest struct {
	Data lemonSqueezyCheckoutCreateData `json:"data"`
}

type lemonSqueezyCheckoutCreateData struct {
	Type          string                               `json:"type"`
	Attributes    lemonSqueezyCheckoutCreateAttributes `json:"attributes"`
	Relationships lemonSqueezyCheckoutRelationships    `json:"relationships"`
}

type lemonSqueezyCheckoutCreateAttributes struct {
	CheckoutData    lemonSqueezyCheckoutData    `json:"checkout_data"`
	CheckoutOptions lemonSqueezyCheckoutOptions `json:"checkout_options"`
	ProductOptions  lemonSqueezyProductOptions  `json:"product_options"`
	CustomPrice     int64                       `json:"custom_price"`
	ExpiresAt       string                      `json:"expires_at,omitempty"`
}

type lemonSqueezyCheckoutData struct {
	Custom map[string]any `json:"custom"`
}

type lemonSqueezyCheckoutOptions struct {
	Embed bool `json:"embed"`
}

type lemonSqueezyProductOptions struct {
	Name              string  `json:"name,omitempty"`
	Description       string  `json:"description,omitempty"`
	RedirectURL       string  `json:"redirect_url,omitempty"`
	ReceiptButtonText string  `json:"receipt_button_text,omitempty"`
	ReceiptLinkURL    string  `json:"receipt_link_url,omitempty"`
	EnabledVariants   []int64 `json:"enabled_variants,omitempty"`
}

type lemonSqueezyCheckoutRelationships struct {
	Store   lemonSqueezyRelationship `json:"store"`
	Variant lemonSqueezyRelationship `json:"variant"`
}

type lemonSqueezyRelationship struct {
	Data lemonSqueezyRelationshipData `json:"data"`
}

type lemonSqueezyRelationshipData struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type lemonSqueezyCheckoutResponse struct {
	Data lemonSqueezyCheckoutDataResponse `json:"data"`
}

type lemonSqueezyCheckoutLookupResponse struct {
	Data lemonSqueezyCheckoutDataResponse `json:"data"`
}

type lemonSqueezyCheckoutDataResponse struct {
	ID         string                         `json:"id"`
	Attributes lemonSqueezyCheckoutAttributes `json:"attributes"`
}

type lemonSqueezyCheckoutAttributes struct {
	URL string `json:"url"`
}

type lemonSqueezyWebhookEvent struct {
	Meta lemonSqueezyWebhookMeta `json:"meta"`
	Data lemonSqueezyOrderData   `json:"data"`
}

type lemonSqueezyWebhookMeta struct {
	EventName  string         `json:"event_name"`
	CustomData map[string]any `json:"custom_data"`
}

type lemonSqueezyOrderResponse struct {
	Data lemonSqueezyOrderData `json:"data"`
}

type lemonSqueezyOrderData struct {
	ID         string                      `json:"id"`
	Attributes lemonSqueezyOrderAttributes `json:"attributes"`
}

type lemonSqueezyOrderAttributes struct {
	StoreID        int64                      `json:"store_id"`
	Currency       string                     `json:"currency"`
	Status         string                     `json:"status"`
	Total          json.Number                `json:"total"`
	Refunded       bool                       `json:"refunded"`
	RefundedAmount json.Number                `json:"refunded_amount"`
	FirstOrderItem lemonSqueezyFirstOrderItem `json:"first_order_item"`
}

type lemonSqueezyFirstOrderItem struct {
	VariantID int64 `json:"variant_id"`
}

type lemonSqueezyRefundRequest struct {
	Data lemonSqueezyRefundRequestData `json:"data"`
}

type lemonSqueezyRefundRequestData struct {
	Type       string                       `json:"type"`
	ID         string                       `json:"id"`
	Attributes lemonSqueezyRefundAttributes `json:"attributes"`
}

type lemonSqueezyRefundAttributes struct {
	Amount int64 `json:"amount"`
}

var (
	_ payment.Provider                 = (*LemonSqueezy)(nil)
	_ payment.RefundQueryProvider      = (*LemonSqueezy)(nil)
	_ payment.MerchantIdentityProvider = (*LemonSqueezy)(nil)
)
