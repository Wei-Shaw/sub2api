// Package provider contains concrete payment provider implementations.
package provider

import (
	"context"
	"fmt"
	"net/http"
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
	return map[string]string{"bankAccountNumber": strings.TrimSpace(s.config["bankAccountNumber"])}
}

// sepayNormalizeCode canonicalizes a transfer code for matching: uppercase,
// keep only letters and digits (drops the sub2_ underscore, tolerates bank
// content mutations such as accents or extra separators).
func sepayNormalizeCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
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
