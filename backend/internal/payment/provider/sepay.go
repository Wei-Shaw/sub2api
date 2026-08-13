// Package provider contains concrete payment provider implementations.
package provider

import (
	"context"
	"crypto/hmac"
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
	sepayHTTPTimeout      = 15 * time.Second
	maxSepayResponseSize  = 1 << 20 // 1MB
	sepayDefaultAPIBase   = "https://my.sepay.vn/userapi"
	sepayTransferTypeIn   = "in"
	sepayTransactionLimit = 100
)

// SePay implements payment.Provider for SePay, a Vietnamese bank-transfer
// reconciliation service.
//
// SePay has no "create payment" call: the user is shown a VietQR code that
// prefills a transfer to the merchant's bank account with the order code as the
// description, and SePay pushes a webhook once a matching credit lands. Polling
// is a secondary path and requires the optional user-API token.
type SePay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewSePay creates a new SePay provider.
// config keys: accountNumber, bankCode (or bankBin), apiKey, accountName,
// apiToken, apiBase.
func NewSePay(instanceID string, config map[string]string) (*SePay, error) {
	cfg := cloneStringMap(config)
	for _, k := range []string{"accountNumber", "apiKey"} {
		if strings.TrimSpace(cfg[k]) == "" {
			return nil, fmt.Errorf("sepay config missing required key: %s", k)
		}
	}
	if _, err := resolveVietQRBankBIN(cfg["bankBin"], cfg["bankCode"]); err != nil {
		return nil, fmt.Errorf("sepay config: %w", err)
	}
	if strings.TrimSpace(cfg["apiBase"]) == "" {
		cfg["apiBase"] = sepayDefaultAPIBase
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
	metadata := map[string]string{"currency": payment.CurrencySePay}
	if account := strings.TrimSpace(s.config["accountNumber"]); account != "" {
		metadata["merchant_id"] = account
	}
	return metadata
}

// CreatePayment renders the VietQR payload the user scans. No upstream call is
// made, so there is no trade number until the transfer actually arrives.
func (s *SePay) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	bin, err := resolveVietQRBankBIN(s.config["bankBin"], s.config["bankCode"])
	if err != nil {
		return nil, fmt.Errorf("sepay create payment: %w", err)
	}
	amount, err := payment.AmountToMinorUnit(req.Amount, payment.CurrencySePay)
	if err != nil {
		return nil, fmt.Errorf("sepay create payment: %w", err)
	}
	if amount <= 0 {
		return nil, fmt.Errorf("sepay create payment: amount must be greater than zero")
	}
	orderCode := payment.NormalizeTransferContent(req.OrderID)
	if orderCode == "" {
		return nil, fmt.Errorf("sepay create payment: order code is empty")
	}
	accountNumber := strings.TrimSpace(s.config["accountNumber"])
	qr := buildVietQRPayload(bin, accountNumber, strconv.FormatInt(amount, 10), orderCode)
	return &payment.CreatePaymentResponse{
		QRCode:   qr,
		Currency: payment.CurrencySePay,
		Transfer: &payment.BankTransferInfo{
			BankCode:      strings.ToUpper(strings.TrimSpace(s.config["bankCode"])),
			BankBIN:       bin,
			AccountNumber: accountNumber,
			AccountName:   strings.TrimSpace(s.config["accountName"]),
			Content:       orderCode,
			Amount:        strconv.FormatInt(amount, 10),
		},
	}, nil
}

// QueryOrder looks the order code up in the merchant's recent transactions.
// Without an `apiToken` there is no polling API available, so the order stays
// pending until the webhook arrives.
func (s *SePay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	orderCode := payment.NormalizeTransferContent(tradeNo)
	if orderCode == "" {
		return nil, fmt.Errorf("sepay query order: order code is empty")
	}
	token := strings.TrimSpace(s.config["apiToken"])
	if token == "" {
		return s.pendingQueryResponse(tradeNo), nil
	}

	transactions, err := s.listTransactions(ctx, token)
	if err != nil {
		return nil, err
	}
	for _, tx := range transactions {
		if payment.ExtractOrderCode(tx.TransactionContent) != orderCode {
			continue
		}
		amount, parseErr := strconv.ParseFloat(strings.TrimSpace(tx.AmountIn), 64)
		if parseErr != nil || amount <= 0 {
			continue
		}
		metadata := s.MerchantIdentityMetadata()
		if ref := strings.TrimSpace(tx.ReferenceNumber); ref != "" {
			metadata["reference_number"] = ref
		}
		return &payment.QueryOrderResponse{
			TradeNo:  sepayTradeNo(tx.ID, tx.ReferenceNumber),
			Status:   payment.ProviderStatusPaid,
			Amount:   amount,
			PaidAt:   sepayParseTime(tx.TransactionDate),
			Metadata: metadata,
		}, nil
	}
	return s.pendingQueryResponse(tradeNo), nil
}

func (s *SePay) pendingQueryResponse(tradeNo string) *payment.QueryOrderResponse {
	return &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Status:   payment.ProviderStatusPending,
		Metadata: s.MerchantIdentityMetadata(),
	}
}

type sepayTransaction struct {
	ID                 string `json:"id"`
	TransactionDate    string `json:"transaction_date"`
	AccountNumber      string `json:"account_number"`
	AmountIn           string `json:"amount_in"`
	TransactionContent string `json:"transaction_content"`
	ReferenceNumber    string `json:"reference_number"`
}

func (s *SePay) listTransactions(ctx context.Context, token string) ([]sepayTransaction, error) {
	q := url.Values{}
	q.Set("account_number", strings.TrimSpace(s.config["accountNumber"]))
	q.Set("limit", strconv.Itoa(sepayTransactionLimit))
	endpoint := s.config["apiBase"] + "/transactions/list?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("sepay query order: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("sepay query order: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSepayResponseSize))
	if err != nil {
		return nil, fmt.Errorf("sepay query order: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("sepay query order: HTTP %d", resp.StatusCode)
	}
	var parsed struct {
		Status       int                `json:"status"`
		Error        *string            `json:"error"`
		Transactions []sepayTransaction `json:"transactions"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("sepay query order: parse response: %w", err)
	}
	if parsed.Error != nil && strings.TrimSpace(*parsed.Error) != "" {
		return nil, fmt.Errorf("sepay query order: %s", *parsed.Error)
	}
	return parsed.Transactions, nil
}

// sepayWebhookPayload is the JSON body SePay POSTs when a bank transaction
// matching the merchant's account is detected.
type sepayWebhookPayload struct {
	ID              json.Number `json:"id"`
	Gateway         string      `json:"gateway"`
	TransactionDate string      `json:"transactionDate"`
	AccountNumber   string      `json:"accountNumber"`
	Code            *string     `json:"code"`
	Content         string      `json:"content"`
	TransferType    string      `json:"transferType"`
	TransferAmount  float64     `json:"transferAmount"`
	SubAccount      *string     `json:"subAccount"`
	ReferenceCode   string      `json:"referenceCode"`
	Description     string      `json:"description"`
}

// VerifyNotification authenticates and parses a SePay webhook.
//
// Outgoing transfers and credits that carry no recognisable order code are not
// errors — they are simply events we have nothing to do with, so they return a
// nil notification and the handler acks them.
func (s *SePay) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := s.verifyWebhookAuth(headers); err != nil {
		return nil, err
	}
	var payload sepayWebhookPayload
	if err := json.Unmarshal([]byte(rawBody), &payload); err != nil {
		return nil, fmt.Errorf("sepay verify notification: parse body: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(payload.TransferType), sepayTransferTypeIn) {
		return nil, nil
	}
	expectedAccount := strings.TrimSpace(s.config["accountNumber"])
	payloadAccount := strings.TrimSpace(payload.AccountNumber)
	if expectedAccount != "" && payloadAccount != "" && !strings.EqualFold(payloadAccount, expectedAccount) {
		return nil, fmt.Errorf("sepay verify notification: account number mismatch")
	}

	orderCode := sepayOrderCode(payload)
	if orderCode == "" {
		return nil, nil
	}

	metadata := s.MerchantIdentityMetadata()
	if ref := strings.TrimSpace(payload.ReferenceCode); ref != "" {
		metadata["reference_number"] = ref
	}
	if gateway := strings.TrimSpace(payload.Gateway); gateway != "" {
		metadata["gateway"] = gateway
	}
	return &payment.PaymentNotification{
		TradeNo:  sepayTradeNo(payload.ID.String(), payload.ReferenceCode),
		OrderID:  orderCode,
		Amount:   payload.TransferAmount,
		Status:   payment.ProviderStatusSuccess,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

// sepayOrderCode prefers SePay's own parsed `code` field and falls back to
// scanning the raw transfer content.
func sepayOrderCode(payload sepayWebhookPayload) string {
	if payload.Code != nil {
		if code := payment.ExtractOrderCode(*payload.Code); code != "" {
			return code
		}
	}
	if code := payment.ExtractOrderCode(payload.Content); code != "" {
		return code
	}
	return payment.ExtractOrderCode(payload.Description)
}

// verifyWebhookAuth checks the shared secret SePay sends in the Authorization
// header. SePay documents the `Apikey` scheme; `Bearer` is accepted because
// some SePay dashboard versions emit it instead.
func (s *SePay) verifyWebhookAuth(headers map[string]string) error {
	expected := strings.TrimSpace(s.config["apiKey"])
	if expected == "" {
		return fmt.Errorf("sepay apiKey not configured")
	}
	raw := strings.TrimSpace(headers["authorization"])
	if raw == "" {
		return fmt.Errorf("sepay notification missing authorization header")
	}
	presented := raw
	for _, scheme := range []string{"apikey ", "bearer "} {
		if len(raw) >= len(scheme) && strings.EqualFold(raw[:len(scheme)], scheme) {
			presented = strings.TrimSpace(raw[len(scheme):])
			break
		}
	}
	if !hmac.Equal([]byte(presented), []byte(expected)) {
		return fmt.Errorf("sepay notification authorization mismatch")
	}
	return nil
}

func (s *SePay) client() *http.Client {
	if s.httpClient != nil {
		return s.httpClient
	}
	return &http.Client{Timeout: sepayHTTPTimeout}
}

// sepayTradeNo picks the most durable upstream identifier available.
func sepayTradeNo(id, referenceCode string) string {
	if id = strings.TrimSpace(id); id != "" && id != "0" {
		return id
	}
	return strings.TrimSpace(referenceCode)
}

// sepayParseTime converts SePay's "2006-01-02 15:04:05" local timestamps to
// RFC3339, returning an empty string when the input is unusable.
func sepayParseTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local)
	if err != nil {
		return ""
	}
	return parsed.Format(time.RFC3339)
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Ensure interface compliance.
var (
	_ payment.Provider                 = (*SePay)(nil)
	_ payment.MerchantIdentityProvider = (*SePay)(nil)
)
