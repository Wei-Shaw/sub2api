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
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	dogPayDefaultAPIBase     = "https://api.dogpay.com"
	dogPayAuthPath           = "/open-api/v1/auth/access_token"
	dogPayPayPath            = "/open-api/v1/pay"
	dogPayCurrencyConfigPath = "/open-api/v1/pay/currency-config"
	dogPayHTTPTimeout        = 15 * time.Second
	dogPayTokenBufferTime    = 1 * time.Minute
)

// DogPayFixedCurrency is the only currency supported by DogPay.
const DogPayFixedCurrency = "USD"

// DogPayFixedPayChannel is the default payment channel.
const DogPayFixedPayChannel = "pay_002"

// DogPay implements the payment.Provider interface for DogPay (crypto payment gateway).
type DogPay struct {
	instanceID string
	config     map[string]string // appId, secret, apiBase

	mu         sync.Mutex
	httpClient *http.Client

	// OAuth2 token cache
	tokenCache     *dogPayTokenCache
	tokenExpiresAt time.Time
}

type dogPayTokenCache struct {
	AccessToken string
	ExpiresIn   int
}

// NewDogPay creates a new DogPay provider instance.
func NewDogPay(instanceID string, config map[string]string) (*DogPay, error) {
	required := []string{"appId", "secret"}
	for _, k := range required {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("dogpay config missing required key: %s", k)
		}
	}

	// Clone config map to avoid mutating the original
	cfg := make(map[string]string, len(config))
	for k, v := range config {
		cfg[k] = v
	}

	// Set default API base
	if strings.TrimSpace(cfg["apiBase"]) == "" {
		cfg["apiBase"] = dogPayDefaultAPIBase
	}

	return &DogPay{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: dogPayHTTPTimeout},
	}, nil
}

// Name returns a human-readable name for this provider.
func (d *DogPay) Name() string { return "DogPay" }

// ProviderKey returns the unique key identifying this provider type.
func (d *DogPay) ProviderKey() string { return payment.TypeDogPay }

// SupportedTypes returns the list of payment types this provider handles.
func (d *DogPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeDogPay}
}

// MerchantIdentityMetadata returns non-sensitive merchant identity for snapshot consistency.
func (d *DogPay) MerchantIdentityMetadata() map[string]string {
	if d == nil {
		return nil
	}
	return map[string]string{"currency": DogPayFixedCurrency}
}

func (d *DogPay) currency() string {
	return DogPayFixedCurrency
}

// apiBase returns the configured DogPay API base URL.
func (d *DogPay) apiBase() string {
	if base := strings.TrimSpace(d.config["apiBase"]); base != "" {
		return strings.TrimRight(base, "/")
	}
	return dogPayDefaultAPIBase
}

// accessToken obtains an OAuth2 access token with in-memory caching.
func (d *DogPay) accessToken(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check cache with buffer time
	if d.tokenCache != nil && time.Now().Add(dogPayTokenBufferTime).Before(d.tokenExpiresAt) {
		return d.tokenCache.AccessToken, nil
	}

	payload := map[string]string{
		"grant_type": "client_credential",
		"appid":      d.config["appId"],
		"secret":     d.config["secret"],
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("dogpay marshal auth payload: %w", err)
	}

	url := d.apiBase() + dogPayAuthPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("dogpay create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("dogpay auth request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("dogpay read auth response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("dogpay auth HTTP %d: %s", resp.StatusCode, truncateString(string(respBody), 512))
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("dogpay parse auth response: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("dogpay auth failed: code=%d message=%s", result.Code, result.Message)
	}

	accessToken := strings.TrimSpace(result.Data.AccessToken)
	if accessToken == "" || result.Data.ExpiresIn <= 0 {
		return "", fmt.Errorf("dogpay auth returned invalid token")
	}

	d.tokenCache = &dogPayTokenCache{
		AccessToken: accessToken,
		ExpiresIn:   result.Data.ExpiresIn,
	}
	d.tokenExpiresAt = time.Now().Add(time.Duration(result.Data.ExpiresIn) * time.Second)

	return accessToken, nil
}

// getCurrencyConfig fetches the active USD currency configuration from DogPay.
func (d *DogPay) getCurrencyConfig(ctx context.Context, token string) (currencyConfigID string, err error) {
	url := d.apiBase() + dogPayCurrencyConfigPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("dogpay create currency config request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("dogpay currency config request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("dogpay read currency config response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("dogpay currency config HTTP %d: %s", resp.StatusCode, truncateString(string(body), 512))
	}

	var result struct {
		Code int `json:"code"`
		Data []struct {
			ID         string `json:"id"`
			PayChannel string `json:"pay_channel"`
			Currency   string `json:"currency"`
			Status     string `json:"status"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("dogpay parse currency config: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("dogpay currency config failed: code=%d message=%s", result.Code, result.Message)
	}

	for _, cfg := range result.Data {
		if !strings.EqualFold(strings.TrimSpace(cfg.Currency), DogPayFixedCurrency) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(cfg.PayChannel), DogPayFixedPayChannel) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(cfg.Status), "active") {
			continue
		}
		currencyConfigID = strings.TrimSpace(cfg.ID)
		if currencyConfigID != "" {
			return currencyConfigID, nil
		}
	}

	return "", fmt.Errorf("dogpay: no active %s currency config found for pay_channel=%s", DogPayFixedCurrency, DogPayFixedPayChannel)
}

// CreatePayment creates a DogPay payment order.
func (d *DogPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	token, err := d.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("dogpay auth: %w", err)
	}

	// Get currency config
	currencyConfigID, err := d.getCurrencyConfig(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("dogpay currency config: %w", err)
	}

	successURL, failureURL := buildDogPayRedirectURLs(req.ReturnURL)
	payload := map[string]string{
		"orderAmount":      req.Amount,
		"goodsName":        req.Subject,
		"callId":           req.OrderID,
		"currencyConfigId": currencyConfigID,
		"successUrl":       successURL,
		"failureUrl":       failureURL,
		"payChannel":       DogPayFixedPayChannel,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("dogpay marshal payment payload: %w", err)
	}

	url := d.apiBase() + dogPayPayPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("dogpay create payment request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("dogpay payment request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("dogpay read payment response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("dogpay payment HTTP %d: %s", resp.StatusCode, truncateString(string(respBody), 512))
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			PayInfo struct {
				PayURL string `json:"payUrl"`
			} `json:"payInfo"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("dogpay parse payment response: %w", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("dogpay create payment failed: code=%d message=%s", result.Code, result.Message)
	}

	payURL := strings.TrimSpace(result.Data.PayInfo.PayURL)
	if payURL == "" {
		return nil, fmt.Errorf("dogpay create payment: empty pay URL")
	}

	return &payment.CreatePaymentResponse{
		TradeNo:    req.OrderID, // callId serves as trade number
		PayURL:     payURL,
		Currency:   d.currency(),
		ResultType: payment.CreatePaymentResultOrderCreated,
	}, nil
}

// QueryOrder queries the payment status. DogPay doesn't provide a direct query API;
// status is updated via webhook callbacks.
func (d *DogPay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	return &payment.QueryOrderResponse{
		TradeNo: tradeNo,
		Status:  payment.ProviderStatusPending,
	}, nil
}

// VerifyNotification verifies and parses a DogPay webhook event.
// Returns nil notification for irrelevant events (caller should return 200).
func (d *DogPay) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	appID := d.config["appId"]
	if appID == "" {
		return nil, fmt.Errorf("dogpay appId not configured")
	}

	// Extract signature from headers
	sig := headers["wh-signature"]
	if sig == "" {
		sig = headers["x-webhook-signature"]
	}
	if sig == "" {
		return nil, fmt.Errorf("dogpay webhook missing signature header")
	}

	// Verify HMAC-SHA512 signature using appId as the key
	if !d.verifySignature([]byte(rawBody), sig, appID) {
		return nil, fmt.Errorf("dogpay webhook invalid signature")
	}

	// Parse webhook payload
	var req struct {
		EventIdentifier string `json:"event_identifier"`
		Data            struct {
			ID               string `json:"id"`
			IDNo             string `json:"idNo"`
			PayIdNo          string `json:"payIdNo"`
			Status           string `json:"status"`
			Amount           string `json:"amount"`
			Currency         string `json:"currency"`
			CurrencyConfigID string `json:"currencyConfigId"`
			PayChannel       string `json:"payChannel"`
			CallID           string `json:"callId"`
			TxHash           string `json:"txHash"`
			TransferAmount   string `json:"transferAmount"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(rawBody), &req); err != nil {
		return nil, fmt.Errorf("dogpay parse webhook: %w", err)
	}

	// Only process completed payment events
	if req.EventIdentifier != "pay.transaction.update" || req.Data.Status != "completed" {
		return nil, nil
	}

	amount, _ := parseFloatSafe(req.Data.Amount)

	return &payment.PaymentNotification{
		TradeNo: req.Data.CallID,
		OrderID: req.Data.CallID,
		Amount:  amount,
		Status:  payment.ProviderStatusSuccess,
		RawData: rawBody,
		Metadata: map[string]string{
			"currency":         req.Data.Currency,
			"payChannel":       req.Data.PayChannel,
			"currencyConfigId": req.Data.CurrencyConfigID,
			"txHash":           req.Data.TxHash,
			"transferAmount":   req.Data.TransferAmount,
			"dogpayId":         req.Data.ID,
			"dogpayIdNo":       req.Data.IDNo,
			"dogpayPayIdNo":    req.Data.PayIdNo,
		},
	}, nil
}

// Refund is not supported by DogPay (crypto payments are irreversible).
func (d *DogPay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("dogpay does not support refunds")
}

// verifySignature verifies HMAC-SHA512 signature.
func (d *DogPay) verifySignature(payload []byte, signature, secret string) bool {
	if signature == "" || secret == "" {
		return false
	}
	hasher := hmac.New(sha512.New, []byte(secret))
	hasher.Write(payload)
	expected := hex.EncodeToString(hasher.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// --- Helper functions ---

// buildDogPayRedirectURLs constructs separate success and failure redirect URLs for DogPay.
// DogPay requires distinct successUrl and failureUrl parameters. Both point to the canonical
// payment result page so the frontend can resolve the order and display its final status.
func buildDogPayRedirectURLs(baseReturnURL string) (successURL, failureURL string) {
	// If the base return URL is empty, return empty strings (DogPay will use its default).
	if baseReturnURL == "" {
		return "", ""
	}
	// The service passes the canonical result URL with order context in its query string.
	// Preserve that context while changing only the provider-specific status parameter.
	parsed, err := url.Parse(baseReturnURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ""
	}

	parsed.Path = "/payment/result"
	parsed.RawPath = ""
	parsed.Fragment = ""

	query := parsed.Query()
	query.Set("status", "success")
	parsed.RawQuery = query.Encode()
	successURL = parsed.String()

	query.Set("status", "failure")
	parsed.RawQuery = query.Encode()
	failureURL = parsed.String()
	return successURL, failureURL
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func parseFloatSafe(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
