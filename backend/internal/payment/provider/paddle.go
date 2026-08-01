package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	paddleAPIBase              = "https://api.paddle.com"
	paddleHTTPTimeout          = 15 * time.Second
	paddleMaxResponseSize      = 1 << 20
	paddleHeaderAuth           = "Authorization"
	paddleHeaderContentType    = "Content-Type"
	paddleHeaderSignature      = "paddle-signature"
	paddleWebhookTolerance     = 5 * time.Minute
	paddleEventTransactionPaid = "transaction.paid"
	paddleEventTransactionDone = "transaction.completed"
	paddleEventTransactionFail = "transaction.canceled"
)

type Paddle struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

type paddleTransactionPayload struct {
	Items []struct {
		Quantity int `json:"quantity"`
		Price    struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			UnitPrice   struct {
				Amount       string `json:"amount"`
				CurrencyCode string `json:"currency_code"`
			} `json:"unit_price"`
			TaxMode string `json:"tax_mode"`
		} `json:"price"`
	} `json:"items"`
	CustomData map[string]any `json:"custom_data,omitempty"`
}

type paddleTransactionEnvelope struct {
	Data struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		CurrencyCode string `json:"currency_code"`
		CustomData   struct {
			OrderID string `json:"orderId"`
		} `json:"custom_data"`
		Details struct {
			Totals struct {
				Total string `json:"total"`
			} `json:"totals"`
		} `json:"details"`
		BilledAt string `json:"billed_at"`
	} `json:"data"`
}

type paddleWebhookEnvelope struct {
	EventType  string `json:"event_type"`
	OccurredAt string `json:"occurred_at"`
	Data       struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		CurrencyCode string `json:"currency_code"`
		CustomData   struct {
			OrderID string `json:"orderId"`
		} `json:"custom_data"`
		Details struct {
			Totals struct {
				Total string `json:"total"`
			} `json:"totals"`
		} `json:"details"`
	} `json:"data"`
}

func NewPaddle(instanceID string, config map[string]string) (*Paddle, error) {
	if strings.TrimSpace(config["apiKey"]) == "" {
		return nil, fmt.Errorf("paddle config missing required key: apiKey")
	}
	return &Paddle{
		instanceID: instanceID,
		config:     config,
		httpClient: &http.Client{Timeout: paddleHTTPTimeout},
	}, nil
}

func (p *Paddle) Name() string        { return "Paddle" }
func (p *Paddle) ProviderKey() string { return payment.TypePaddle }
func (p *Paddle) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypePaddle}
}

func (p *Paddle) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	currency := strings.ToUpper(strings.TrimSpace(req.PaymentCurrency))
	if currency == "" {
		currency = "USD"
	}
	minorAmount, err := decimalAmountToMinorUnits(req.Amount, currency)
	if err != nil {
		return nil, fmt.Errorf("paddle create payment: %w", err)
	}
	payload := paddleTransactionPayload{
		CustomData: map[string]any{
			"orderId":          req.OrderID,
			"paymentType":      req.PaymentType,
			"ledgerCurrency":   req.LedgerCurrency,
			"ledgerAmount":     req.LedgerAmount,
			"providerInstance": p.instanceID,
		},
	}
	payload.Items = []struct {
		Quantity int `json:"quantity"`
		Price    struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			UnitPrice   struct {
				Amount       string `json:"amount"`
				CurrencyCode string `json:"currency_code"`
			} `json:"unit_price"`
			TaxMode string `json:"tax_mode"`
		} `json:"price"`
	}{
		{
			Quantity: 1,
			Price: struct {
				Name        string `json:"name"`
				Description string `json:"description,omitempty"`
				UnitPrice   struct {
					Amount       string `json:"amount"`
					CurrencyCode string `json:"currency_code"`
				} `json:"unit_price"`
				TaxMode string `json:"tax_mode"`
			}{
				Name:        strings.TrimSpace(req.Subject),
				Description: strings.TrimSpace(req.Subject),
				TaxMode:     "account_setting",
			},
		},
	}
	if payload.Items[0].Price.Name == "" {
		payload.Items[0].Price.Name = "Sub2API Payment"
	}
	payload.Items[0].Price.UnitPrice.Amount = minorAmount
	payload.Items[0].Price.UnitPrice.CurrencyCode = currency

	respBody, err := p.doRequest(ctx, http.MethodPost, "/transactions", payload)
	if err != nil {
		return nil, fmt.Errorf("paddle create payment: %w", err)
	}
	var resp paddleTransactionEnvelope
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("paddle create payment: parse response: %w", err)
	}
	if strings.TrimSpace(resp.Data.ID) == "" {
		return nil, fmt.Errorf("paddle create payment: missing transaction id")
	}
	return &payment.CreatePaymentResponse{
		TradeNo:    resp.Data.ID,
		CheckoutID: resp.Data.ID,
	}, nil
}

func (p *Paddle) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	respBody, err := p.doRequest(ctx, http.MethodGet, "/transactions/"+tradeNo, nil)
	if err != nil {
		return nil, fmt.Errorf("paddle query order: %w", err)
	}
	var resp paddleTransactionEnvelope
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("paddle query order: parse response: %w", err)
	}
	amount, _ := minorUnitsToDecimal(resp.Data.Details.Totals.Total, resp.Data.CurrencyCode)
	return &payment.QueryOrderResponse{
		TradeNo:  firstNonEmpty(resp.Data.ID, tradeNo),
		Status:   mapPaddleStatus(resp.Data.Status),
		Amount:   amount,
		Currency: strings.ToUpper(strings.TrimSpace(resp.Data.CurrencyCode)),
		PaidAt:   strings.TrimSpace(resp.Data.BilledAt),
	}, nil
}

func (p *Paddle) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	secret := strings.TrimSpace(p.config["webhookSecret"])
	if secret == "" {
		return nil, fmt.Errorf("paddle webhookSecret not configured")
	}
	sigHeader := strings.TrimSpace(headers[paddleHeaderSignature])
	if sigHeader == "" {
		return nil, fmt.Errorf("paddle notification missing Paddle-Signature header")
	}
	if err := verifyPaddleSignature(secret, rawBody, sigHeader, time.Now()); err != nil {
		return nil, fmt.Errorf("paddle verify notification: %w", err)
	}
	var event paddleWebhookEnvelope
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("paddle parse notification: %w", err)
	}
	status := ""
	switch event.EventType {
	case paddleEventTransactionPaid, paddleEventTransactionDone:
		status = payment.ProviderStatusSuccess
	case paddleEventTransactionFail:
		status = payment.ProviderStatusFailed
	default:
		return nil, nil
	}
	amount, _ := minorUnitsToDecimal(event.Data.Details.Totals.Total, event.Data.CurrencyCode)
	return &payment.PaymentNotification{
		TradeNo:  strings.TrimSpace(event.Data.ID),
		OrderID:  strings.TrimSpace(event.Data.CustomData.OrderID),
		Amount:   amount,
		Currency: strings.ToUpper(strings.TrimSpace(event.Data.CurrencyCode)),
		Status:   status,
		RawData:  rawBody,
	}, nil
}

func (p *Paddle) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("paddle refund not supported")
}

func (p *Paddle) doRequest(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.apiBase(), "/")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(paddleHeaderAuth, "Bearer "+strings.TrimSpace(p.config["apiKey"]))
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set(paddleHeaderContentType, "application/json")
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	limited := io.LimitReader(resp.Body, paddleMaxResponseSize)
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("paddle api %s %s: status=%d body=%s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func (p *Paddle) apiBase() string {
	if base := strings.TrimSpace(p.config["apiBase"]); base != "" {
		return base
	}
	return paddleAPIBase
}

func verifyPaddleSignature(secret, rawBody, header string, now time.Time) error {
	parts := strings.Split(header, ";")
	values := make(map[string]string, len(parts))
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		values[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	tsRaw := values["ts"]
	h1 := values["h1"]
	if tsRaw == "" || h1 == "" {
		return fmt.Errorf("invalid signature header")
	}
	tsUnix, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid signature timestamp: %w", err)
	}
	ts := time.Unix(tsUnix, 0)
	if now.Sub(ts) > paddleWebhookTolerance || ts.Sub(now) > paddleWebhookTolerance {
		return fmt.Errorf("signature timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(tsRaw)); err != nil {
		return fmt.Errorf("write paddle timestamp into hmac: %w", err)
	}
	if _, err := mac.Write([]byte(":")); err != nil {
		return fmt.Errorf("write paddle separator into hmac: %w", err)
	}
	if _, err := mac.Write([]byte(rawBody)); err != nil {
		return fmt.Errorf("write paddle body into hmac: %w", err)
	}
	expected := mac.Sum(nil)
	provided, err := hex.DecodeString(h1)
	if err != nil {
		return fmt.Errorf("invalid signature digest: %w", err)
	}
	if subtle.ConstantTimeCompare(expected, provided) != 1 {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func mapPaddleStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid", "completed", "billed":
		return payment.ProviderStatusPaid
	case "canceled", "cancelled", "past_due", "failed":
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func decimalAmountToMinorUnits(amount string, currency string) (string, error) {
	minor, err := payment.AmountToMinorUnits(amount, currency)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(minor, 10), nil
}

func minorUnitsToDecimal(amount string, currency string) (float64, error) {
	minor, err := strconv.ParseInt(strings.TrimSpace(amount), 10, 64)
	if err != nil {
		return 0, err
	}
	return payment.MinorUnitsToAmount(minor, currency), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
