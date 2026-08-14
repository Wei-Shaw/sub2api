// Package provider contains concrete payment provider implementations.
package provider

import (
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

// SePay constants.
const (
	defaultSepayAPIBase     = "https://userapi.sepay.vn"
	sepayHTTPTimeout        = 10 * time.Second
	maxSepayResponseSize    = 1 << 20 // 1MB
	maxSepayErrorSummary    = 512
	sepayWebhookMaxSkewSecs = 300
)

// SePay implements payment.Provider for the SePay bank-transfer gateway.
// Payments are VietQR transfers; creation is offline (local EMV payload),
// confirmation arrives via webhook, and the SePay API v2 is used only to
// query transaction status.
type SePay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewSePay creates a SePay provider.
// config keys: apiToken, apiBase, bankAccountNumber, bankBin, accountName,
// webhookSecret (recommended), webhookApiKey (fallback auth).
func NewSePay(instanceID string, config map[string]string) (*SePay, error) {
	for _, k := range []string{"apiToken", "bankAccountNumber", "bankBin"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("sepay config missing required key: %s", k)
		}
	}
	if strings.TrimSpace(config["webhookSecret"]) == "" && strings.TrimSpace(config["webhookApiKey"]) == "" {
		return nil, fmt.Errorf("sepay config requires webhookSecret (recommended) or webhookApiKey")
	}
	cfg := make(map[string]string, len(config))
	for k, v := range config {
		cfg[k] = v
	}
	if strings.TrimSpace(cfg["apiBase"]) == "" {
		cfg["apiBase"] = defaultSepayAPIBase
	}
	cfg["apiBase"] = strings.TrimRight(strings.TrimSpace(cfg["apiBase"]), "/")
	return &SePay{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: sepayHTTPTimeout},
	}, nil
}

func (s *SePay) Name() string        { return "SePay" }
func (s *SePay) ProviderKey() string { return payment.TypeSePay }
func (s *SePay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeSePay}
}

func (s *SePay) MerchantIdentityMetadata() map[string]string {
	if s == nil {
		return nil
	}
	// Key must match the webhook notification shape ("accountNumber", set in
	// VerifyNotification) so snapshot validation accepts both the webhook and
	// the query/reconcile paths. The snapshot builder stores this value as the
	// order's pinned merchant_id.
	return map[string]string{"accountNumber": strings.TrimSpace(s.config["bankAccountNumber"])}
}

// sepayNormalizeCode canonicalizes a transfer code for matching: uppercase,
// keep only letters and digits (drops the sub2_ underscore, tolerates bank
// content mutations such as accents or extra separators).
func sepayNormalizeCode(code string) string {
	return payment.NormalizeTransferCode(code)
}

// sepayCodeMatchesOrder reports whether a webhook/query code refers to the
// given out_trade_no, tolerating bank-side uppercasing and prefix omission.
func sepayCodeMatchesOrder(code, outTradeNo string) bool {
	c := sepayNormalizeCode(code)
	if c == "" {
		return false
	}
	full := sepayNormalizeCode(outTradeNo)
	if c == full {
		return true
	}
	return strings.HasPrefix(full, "SUB2") && c == strings.TrimPrefix(full, "SUB2")
}

// CreatePayment builds the VietQR payload offline. No upstream call: the
// transfer only exists once the customer pays, confirmed via webhook.
func (s *SePay) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amountVND, err := strconv.ParseInt(strings.TrimSpace(req.Amount), 10, 64)
	if err != nil || amountVND <= 0 {
		return nil, fmt.Errorf("sepay amount must be a positive integer VND value, got %q", req.Amount)
	}
	payload := buildVietQRPayload(
		strings.TrimSpace(s.config["bankBin"]),
		strings.TrimSpace(s.config["bankAccountNumber"]),
		amountVND,
		req.OrderID,
	)
	return &payment.CreatePaymentResponse{QRCode: payload, Currency: payment.CurrencyVND}, nil
}

// Refund is not supported: SePay has no refund API — refunds must be issued
// manually via bank transfer and the order adjusted in the admin panel.
func (s *SePay) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("sepay refund is not supported: issue refunds manually via bank transfer")
}

// sepayWebhookPayload mirrors the SePay transaction webhook JSON body.
type sepayWebhookPayload struct {
	ID              int64   `json:"id"`
	Gateway         string  `json:"gateway"`
	TransactionDate string  `json:"transactionDate"`
	AccountNumber   string  `json:"accountNumber"`
	SubAccount      string  `json:"subAccount"`
	Code            *string `json:"code"`
	Content         string  `json:"content"`
	TransferType    string  `json:"transferType"`
	Description     string  `json:"description"`
	TransferAmount  int64   `json:"transferAmount"`
	Accumulated     int64   `json:"accumulated"`
	ReferenceCode   string  `json:"referenceCode"`
}

// VerifyNotification authenticates and parses a SePay webhook. Outgoing
// transactions return (nil, nil) so the caller acks with 200. OrderID carries
// the raw extracted code; the service layer resolves it to the canonical
// out_trade_no (banks may uppercase content, SePay may drop the prefix).
func (s *SePay) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := s.verifyWebhookAuth(rawBody, headers); err != nil {
		return nil, err
	}
	var payload sepayWebhookPayload
	if err := json.Unmarshal([]byte(rawBody), &payload); err != nil {
		return nil, fmt.Errorf("sepay parse notify: %w", err)
	}
	if strings.TrimSpace(payload.TransferType) != "in" {
		return nil, nil
	}
	code := ""
	if payload.Code != nil {
		code = strings.TrimSpace(*payload.Code)
	}
	if code == "" {
		return nil, fmt.Errorf("sepay notify missing payment code")
	}
	tradeNo := strings.TrimSpace(payload.ReferenceCode)
	if tradeNo == "" {
		tradeNo = strconv.FormatInt(payload.ID, 10)
	}
	metadata := map[string]string{"accountNumber": payload.AccountNumber}
	if payload.Gateway != "" {
		metadata["gateway"] = payload.Gateway
	}
	return &payment.PaymentNotification{
		TradeNo:  tradeNo,
		OrderID:  code,
		Amount:   float64(payload.TransferAmount),
		Status:   payment.NotificationStatusSuccess,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

// verifyWebhookAuth checks HMAC-SHA256 (preferred) or the Apikey header.
// Signature: sha256={hex(hmac_sha256(timestamp + "." + rawBody, secret))}.
func (s *SePay) verifyWebhookAuth(rawBody string, headers map[string]string) error {
	if secret := strings.TrimSpace(s.config["webhookSecret"]); secret != "" {
		signature := strings.TrimSpace(headers["x-sepay-signature"])
		if !strings.HasPrefix(signature, "sha256=") {
			return fmt.Errorf("missing X-SePay-Signature")
		}
		timestamp := strings.TrimSpace(headers["x-sepay-timestamp"])
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid X-SePay-Timestamp")
		}
		skew := time.Now().Unix() - ts
		if skew < 0 {
			skew = -skew
		}
		if skew > sepayWebhookMaxSkewSecs {
			return fmt.Errorf("sepay webhook timestamp outside ±%d second window", sepayWebhookMaxSkewSecs)
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(timestamp + "." + rawBody))
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(signature)) {
			return fmt.Errorf("sepay webhook signature mismatch")
		}
		return nil
	}
	apiKey := strings.TrimSpace(s.config["webhookApiKey"])
	auth := strings.TrimSpace(headers["authorization"])
	const apikeyPrefix = "Apikey "
	if !strings.HasPrefix(auth, apikeyPrefix) {
		return fmt.Errorf("missing Authorization Apikey header")
	}
	if !hmac.Equal([]byte(strings.TrimSpace(strings.TrimPrefix(auth, apikeyPrefix))), []byte(apiKey)) {
		return fmt.Errorf("sepay webhook api key mismatch")
	}
	return nil
}

// sepayTransaction mirrors one element of GET /v2/transactions data.
type sepayTransaction struct {
	ID                 string `json:"id"`
	TransactionDate    string `json:"transaction_date"`
	TransferType       string `json:"transfer_type"`
	AmountIn           int64  `json:"amount_in"`
	TransactionContent string `json:"transaction_content"`
	ReferenceNumber    string `json:"reference_number"`
	Code               string `json:"code"`
}

// QueryOrder looks up the order's transfer in SePay API v2. tradeNo carries
// the order's out_trade_no; the q= search covers the extracted payment code.
// Banks and SePay's code extraction drop separators (the sub2_ underscore), so
// when the raw out_trade_no yields no match the query retries with its
// separator-stripped normalized form.
func (s *SePay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	outTradeNo := strings.TrimSpace(tradeNo)
	if outTradeNo == "" {
		return nil, fmt.Errorf("sepay query: empty order reference")
	}
	for _, q := range sepayQueryVariants(outTradeNo) {
		resp, err := s.queryTransactions(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, tx := range resp {
			if !sepayCodeMatchesOrder(tx.Code, outTradeNo) {
				continue
			}
			return &payment.QueryOrderResponse{
				TradeNo:  strings.TrimSpace(tx.ReferenceNumber),
				Status:   payment.ProviderStatusPaid,
				Amount:   float64(tx.AmountIn),
				PaidAt:   strings.TrimSpace(tx.TransactionDate),
				Metadata: s.MerchantIdentityMetadata(),
			}, nil
		}
	}
	return &payment.QueryOrderResponse{
		TradeNo:  outTradeNo,
		Status:   payment.ProviderStatusPending,
		Metadata: s.MerchantIdentityMetadata(),
	}, nil
}

// sepayQueryVariants returns the q= search terms for an out_trade_no: the raw
// value first, then the normalized (letters+digits only) form which matches
// content the bank or SePay extracted without separators.
func sepayQueryVariants(outTradeNo string) []string {
	normalized := sepayNormalizeCode(outTradeNo)
	if normalized == "" || strings.EqualFold(normalized, outTradeNo) {
		return []string{outTradeNo}
	}
	return []string{outTradeNo, normalized}
}

func (s *SePay) queryTransactions(ctx context.Context, q string) ([]sepayTransaction, error) {
	endpoint := s.config["apiBase"] + "/v2/transactions?q=" + url.QueryEscape(q) + "&transfer_type=in&per_page=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.config["apiToken"])
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sepay query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSepayResponseSize))
	if err != nil {
		return nil, fmt.Errorf("sepay query read: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("sepay query unauthorized: check apiToken")
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("sepay query rate limited (retry after %ss)", resp.Header.Get("Retry-After"))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("sepay query HTTP %d: %s", resp.StatusCode, summarizeSepayBody(body))
	}
	var parsed struct {
		Status string             `json:"status"`
		Data   []sepayTransaction `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("sepay query parse: %w", err)
	}
	return parsed.Data, nil
}

func summarizeSepayBody(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if summary == "" {
		return "<empty>"
	}
	if len(summary) > maxSepayErrorSummary {
		return summary[:maxSepayErrorSummary] + "..."
	}
	return summary
}
