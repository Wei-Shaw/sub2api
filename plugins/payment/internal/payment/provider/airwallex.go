package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/shopspring/decimal"
)

const (
	airwallexDemoAPIBase      = "https://api-demo.airwallex.com/api/v1"
	airwallexProdAPIBase      = "https://api.airwallex.com/api/v1"
	airwallexDefaultCountry   = "CN"
	airwallexHTTPTimeout      = 15 * time.Second
	airwallexMaxResponseSize  = 1 << 20
	airwallexMaxErrorSummary  = 512
	airwallexTokenSkew        = 2 * time.Minute
	airwallexWebhookTolerance = 5 * time.Minute

	airwallexEventPaymentSucceeded = "payment_intent.succeeded"
	airwallexEventPaymentCancelled = "payment_intent.cancelled"

	airwallexPaymentStatusSucceeded = "SUCCEEDED"
	airwallexPaymentStatusCancelled = "CANCELLED"
	airwallexRefundStatusReceived   = "RECEIVED"
	airwallexRefundStatusAccepted   = "ACCEPTED"
	airwallexRefundStatusSettled    = "SETTLED"
	airwallexRefundStatusFailed     = "FAILED"
)

type Airwallex struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

type airwallexTokenState struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

var airwallexAccessTokens sync.Map

func NewAirwallex(instanceID string, config map[string]string) (*Airwallex, error) {
	for _, k := range []string{"clientId", "apiKey", "webhookSecret", "apiBase"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("airwallex config missing required key: %s", k)
		}
	}
	cfg := cloneStringMap(config)
	apiBase, err := normalizeAirwallexAPIBase(cfg["apiBase"])
	if err != nil {
		return nil, err
	}
	cfg["apiBase"] = apiBase
	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("airwallex config currency: %w", err)
	}
	cfg["currency"] = currency
	countryCode, err := normalizeAirwallexCountryCode(cfg["countryCode"])
	if err != nil {
		return nil, err
	}
	cfg["countryCode"] = countryCode
	return &Airwallex{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: airwallexHTTPTimeout},
	}, nil
}

func normalizeAirwallexCountryCode(raw string) (string, error) {
	countryCode := strings.ToUpper(strings.TrimSpace(raw))
	if countryCode == "" {
		return airwallexDefaultCountry, nil
	}
	if len(countryCode) != 2 {
		return "", fmt.Errorf("airwallex config countryCode must be a two-letter ISO country code")
	}
	for _, ch := range countryCode {
		if ch < 'A' || ch > 'Z' {
			return "", fmt.Errorf("airwallex config countryCode must be a two-letter ISO country code")
		}
	}
	return countryCode, nil
}

func normalizeAirwallexAPIBase(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "", fmt.Errorf("airwallex apiBase is required")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("airwallex apiBase must be an HTTPS URL")
	}
	host := strings.ToLower(parsed.Host)
	if host != "api-demo.airwallex.com" && host != "api.airwallex.com" {
		return "", fmt.Errorf("airwallex apiBase host must be api-demo.airwallex.com or api.airwallex.com")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawPath = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/api/v1"
	}
	if parsed.Path != "/api/v1" {
		return "", fmt.Errorf("airwallex apiBase path must be /api/v1")
	}
	return parsed.String(), nil
}

func (a *Airwallex) Name() string        { return "空中云汇" }
func (a *Airwallex) ProviderKey() string { return payment.TypeAirwallex }
func (a *Airwallex) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAirwallex}
}

func (a *Airwallex) MerchantIdentityMetadata() map[string]string {
	if a == nil {
		return nil
	}
	metadata := map[string]string{"currency": a.currency()}
	if accountID := strings.TrimSpace(a.config["accountId"]); accountID != "" {
		metadata["account_id"] = accountID
	}
	return metadata
}

func (a *Airwallex) currency() string {
	if a == nil {
		return payment.DefaultPaymentCurrency
	}
	currency, err := payment.NormalizePaymentCurrency(a.config["currency"])
	if err != nil {
		return payment.DefaultPaymentCurrency
	}
	return currency
}

func (a *Airwallex) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("airwallex create payment: invalid amount %s", req.Amount)
	}
	token, err := a.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("airwallex auth: %w", err)
	}

	currency := a.currency()
	requestID := airwallexDeterministicRequestID("payment-intent", req.OrderID, req.Amount, currency)
	payload := airwallexCreatePaymentIntentRequest{
		RequestID:       requestID,
		Amount:          newAirwallexRequestAmount(amount),
		Currency:        currency,
		MerchantOrderID: req.OrderID,
		ReturnURL:       req.ReturnURL,
		Metadata: map[string]string{
			"order_id": req.OrderID,
		},
	}
	if descriptor := strings.TrimSpace(a.config["descriptor"]); descriptor != "" {
		payload.Descriptor = descriptor
	}

	var intent airwallexPaymentIntent
	if err := a.doJSON(ctx, http.MethodPost, "/pa/payment_intents/create", token, payload, &intent); err != nil {
		return nil, fmt.Errorf("airwallex create payment: %w", err)
	}
	if strings.TrimSpace(intent.ID) == "" || strings.TrimSpace(intent.ClientSecret) == "" {
		return nil, fmt.Errorf("airwallex create payment: missing payment intent id or client secret")
	}
	return &payment.CreatePaymentResponse{
		TradeNo:      intent.ID,
		ClientSecret: intent.ClientSecret,
		IntentID:     intent.ID,
		Currency:     currency,
		CountryCode:  a.config["countryCode"],
		PaymentEnv:   a.checkoutEnv(),
	}, nil
}

func (a *Airwallex) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	intentID := strings.TrimSpace(tradeNo)
	if intentID == "" {
		return nil, fmt.Errorf("airwallex query order: missing payment intent id")
	}
	token, err := a.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("airwallex auth: %w", err)
	}

	var intent airwallexPaymentIntent
	if err := a.doJSON(ctx, http.MethodGet, "/pa/payment_intents/"+url.PathEscape(intentID), token, nil, &intent); err != nil {
		return nil, fmt.Errorf("airwallex query order: %w", err)
	}
	return &payment.QueryOrderResponse{
		TradeNo:  intent.ID,
		Status:   airwallexProviderStatus(intent.Status),
		Amount:   intent.Amount,
		Metadata: a.intentMetadata(intent, ""),
	}, nil
}

func (a *Airwallex) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := verifyAirwallexWebhookSignature(rawBody, headers, a.config["webhookSecret"], time.Now()); err != nil {
		return nil, err
	}

	var event airwallexWebhookEvent
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("airwallex parse webhook: %w", err)
	}
	switch event.Name {
	case airwallexEventPaymentSucceeded, airwallexEventPaymentCancelled:
	default:
		return nil, nil
	}

	var intent airwallexPaymentIntent
	if err := json.Unmarshal(event.Data.Object, &intent); err != nil {
		return nil, fmt.Errorf("airwallex parse payment intent: %w", err)
	}
	if strings.TrimSpace(intent.ID) == "" || strings.TrimSpace(intent.MerchantOrderID) == "" {
		return nil, fmt.Errorf("airwallex webhook missing payment intent id or merchant_order_id")
	}
	status := payment.ProviderStatusFailed
	if event.Name == airwallexEventPaymentSucceeded {
		if strings.ToUpper(strings.TrimSpace(intent.Status)) != airwallexPaymentStatusSucceeded {
			return nil, fmt.Errorf("airwallex succeeded webhook has non-succeeded status: %s", intent.Status)
		}
		status = payment.NotificationStatusSuccess
	}

	return &payment.PaymentNotification{
		TradeNo:  intent.ID,
		OrderID:  intent.MerchantOrderID,
		Amount:   intent.Amount,
		Status:   status,
		RawData:  rawBody,
		Metadata: a.intentMetadata(intent, event.accountID()),
	}, nil
}

func (a *Airwallex) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	intentID := strings.TrimSpace(req.TradeNo)
	if intentID == "" {
		return nil, fmt.Errorf("airwallex refund missing payment intent id")
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("airwallex refund: invalid amount %s", req.Amount)
	}
	token, err := a.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("airwallex auth: %w", err)
	}

	payload := airwallexCreateRefundRequest{
		RequestID:       airwallexDeterministicRequestID("refund", intentID, req.Amount),
		PaymentIntentID: intentID,
		Amount:          newAirwallexRequestAmount(amount),
		Reason:          strings.TrimSpace(req.Reason),
	}
	if payload.Reason == "" {
		payload.Reason = "refund"
	}

	var resp airwallexRefund
	if err := a.doJSON(ctx, http.MethodPost, "/pa/refunds/create", token, payload, &resp); err != nil {
		return nil, fmt.Errorf("airwallex refund: %w", err)
	}
	if strings.TrimSpace(resp.ID) == "" {
		return nil, fmt.Errorf("airwallex refund: missing refund id")
	}
	refundResp := &payment.RefundResponse{
		RefundID: resp.ID,
		Status:   airwallexRefundProviderStatus(resp.Status),
	}
	if refundResp.Status != payment.ProviderStatusSuccess {
		return refundResp, fmt.Errorf("airwallex refund not settled: status %s", strings.ToUpper(strings.TrimSpace(resp.Status)))
	}
	return refundResp, nil
}

func (a *Airwallex) CancelPayment(ctx context.Context, tradeNo string) error {
	intentID := strings.TrimSpace(tradeNo)
	if intentID == "" {
		return nil
	}
	token, err := a.accessToken(ctx)
	if err != nil {
		return fmt.Errorf("airwallex auth: %w", err)
	}
	var intent airwallexPaymentIntent
	if err := a.doJSON(ctx, http.MethodPost, "/pa/payment_intents/"+url.PathEscape(intentID)+"/cancel", token, nil, &intent); err != nil {
		return fmt.Errorf("airwallex cancel payment: %w", err)
	}
	return nil
}

func (a *Airwallex) intentMetadata(intent airwallexPaymentIntent, accountID string) map[string]string {
	metadata := map[string]string{
		"currency": strings.ToUpper(strings.TrimSpace(intent.Currency)),
		"status":   strings.ToUpper(strings.TrimSpace(intent.Status)),
	}
	if accountID = strings.TrimSpace(accountID); accountID != "" {
		metadata["account_id"] = accountID
	} else if configured := strings.TrimSpace(a.config["accountId"]); configured != "" {
		metadata["account_id"] = configured
	}
	return metadata
}

func (a *Airwallex) checkoutEnv() string {
	if strings.EqualFold(a.config["apiBase"], airwallexProdAPIBase) {
		return "prod"
	}
	return "demo"
}
