package provider

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	sepayDefaultAPIBase  = "https://my.sepay.vn/userapi"
	sepayDefaultQRBase   = "https://qr.sepay.vn/img"
	sepayDefaultCurrency = "VND"
	sepayTransferTypeIn  = "in"
	sepayHTTPTimeout     = 10 * time.Second
	maxSepayResponseSize = 1 << 20
)

const (
	sepayCanonicalOrderPrefix = "vclaw_"
	sepayLegacyOrderPrefix    = "sub2_"
)

var (
	sepayOrderCodePattern   = regexp.MustCompile(`(?i)\b(?:vclaw|sub2)_[a-z0-9]+\b`)
	sepayTransferRefPattern = regexp.MustCompile(`(?i)\b(?:VCLAW|VC)([0-9]{8}[a-z0-9]{8})\b`)
	sepayOrderSuffixPattern = regexp.MustCompile(`(?i)^([0-9]{8}[a-z0-9]{8})$`)
)

type Sepay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

type sepayFlexibleString string

func (s *sepayFlexibleString) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*s = sepayFlexibleString(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		*s = sepayFlexibleString(number.String())
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(string(data)), "null") {
		*s = ""
		return nil
	}
	return fmt.Errorf("unsupported SePay string value: %s", string(data))
}

type SepayBankAccount struct {
	ID                sepayFlexibleString `json:"id"`
	AccountNumber     string              `json:"account_number"`
	BankShortName     string              `json:"bank_short_name"`
	BankFullName      string              `json:"bank_full_name"`
	AccountHolderName string              `json:"account_holder_name"`
}

func (a SepayBankAccount) IDString() string {
	return strings.TrimSpace(string(a.ID))
}

func NewSepay(instanceID string, config map[string]string) (*Sepay, error) {
	for _, k := range []string{"apiToken", "notifyUrl"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("sepay config missing required key: %s", k)
		}
	}
	return &Sepay{
		instanceID: instanceID,
		config:     config,
		httpClient: &http.Client{Timeout: sepayHTTPTimeout},
	}, nil
}

func (s *Sepay) Name() string        { return "SePay" }
func (s *Sepay) ProviderKey() string { return payment.TypeSepay }
func (s *Sepay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeSepay}
}

func (s *Sepay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := strconv.ParseFloat(strings.TrimSpace(req.Amount), 64)
	if err != nil {
		return nil, fmt.Errorf("sepay create payment: invalid amount: %w", err)
	}
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return nil, fmt.Errorf("sepay create payment: amount must be positive")
	}
	if math.Abs(amount-math.Round(amount)) > 1e-9 {
		return nil, fmt.Errorf("sepay create payment: VND amount must be a whole number")
	}
	currency := strings.ToUpper(strings.TrimSpace(req.PaymentCurrency))
	if currency == "" {
		currency = sepayDefaultCurrency
	}
	if currency != sepayDefaultCurrency {
		return nil, fmt.Errorf("sepay create payment: unsupported currency %s", currency)
	}

	bankAccount, err := s.getBankAccount(ctx)
	if err != nil {
		return nil, fmt.Errorf("sepay create payment: %w", err)
	}
	transferContent := buildSepayTransferContent(req.OrderID)
	qr, err := s.buildQRCodeURL(bankAccount, int64(math.Round(amount)), transferContent)
	if err != nil {
		return nil, fmt.Errorf("sepay create payment: %w", err)
	}

	return &payment.CreatePaymentResponse{TradeNo: req.OrderID, QRCode: qr}, nil
}

func (s *Sepay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	bankAccount, err := s.getBankAccount(ctx)
	if err != nil {
		return nil, fmt.Errorf("sepay query order: %w", err)
	}

	query := url.Values{}
	query.Set("account_number", strings.TrimSpace(bankAccount.AccountNumber))
	query.Set("limit", "20")
	respBody, err := s.doRequest(ctx, http.MethodGet, s.transactionsEndpoint(query), nil)
	if err != nil {
		return nil, fmt.Errorf("sepay query order: %w", err)
	}

	var resp struct {
		Transactions []struct {
			ID                 string  `json:"id"`
			AmountIn           float64 `json:"amount_in"`
			TransactionContent string  `json:"transaction_content"`
			Code               string  `json:"code"`
			ReferenceNumber    string  `json:"reference_number"`
			TransactionDate    string  `json:"transaction_date"`
		} `json:"transactions"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("sepay query order: parse response: %w", err)
	}

	wanted := strings.TrimSpace(tradeNo)
	wantedTransferRef := buildSepayTransferReference(wanted)
	wantedUpper := strings.ToUpper(wanted)
	for _, tx := range resp.Transactions {
		codeOrderID := normalizeSepayOrderID(tx.Code)
		contentOrderID := extractSepayOrderCode(tx.TransactionContent)
		contentUpper := strings.ToUpper(tx.TransactionContent)
		if strings.EqualFold(codeOrderID, wanted) ||
			strings.EqualFold(contentOrderID, wanted) ||
			strings.Contains(contentUpper, wantedUpper) ||
			(wantedTransferRef != "" && strings.Contains(strings.ToUpper(tx.TransactionContent), strings.ToUpper(wantedTransferRef))) {
			trade := strings.TrimSpace(tx.ReferenceNumber)
			if trade == "" {
				trade = strings.TrimSpace(tx.ID)
			}
			return &payment.QueryOrderResponse{
				TradeNo:  trade,
				Status:   payment.ProviderStatusPaid,
				Amount:   tx.AmountIn,
				Currency: sepayDefaultCurrency,
				PaidAt:   strings.TrimSpace(tx.TransactionDate),
			}, nil
		}
	}

	return &payment.QueryOrderResponse{TradeNo: wanted, Status: payment.ProviderStatusPending, Currency: sepayDefaultCurrency}, nil
}

func (s *Sepay) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if apiKey := strings.TrimSpace(s.config["webhookApiKey"]); apiKey != "" {
		auth := strings.TrimSpace(headers["authorization"])
		scheme, token, ok := strings.Cut(auth, " ")
		if !ok || !strings.EqualFold(scheme, "Apikey") || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(apiKey)) != 1 {
			return nil, fmt.Errorf("sepay notification authorization mismatch")
		}
	}
	var payload struct {
		ID                 int64   `json:"id"`
		Code               string  `json:"code"`
		TransferType       string  `json:"transferType"`
		TransferAmount     float64 `json:"transferAmount"`
		ReferenceCode      string  `json:"referenceCode"`
		TransactionContent string  `json:"content"`
	}
	if err := json.Unmarshal([]byte(rawBody), &payload); err != nil {
		return nil, fmt.Errorf("sepay parse notification: %w", err)
	}
	orderID := normalizeSepayOrderID(payload.Code)
	if orderID == "" {
		orderID = extractSepayOrderCode(payload.TransactionContent)
	}
	if orderID == "" || !strings.EqualFold(strings.TrimSpace(payload.TransferType), sepayTransferTypeIn) {
		return nil, nil
	}
	tradeNo := strings.TrimSpace(payload.ReferenceCode)
	if tradeNo == "" && payload.ID > 0 {
		tradeNo = strconv.FormatInt(payload.ID, 10)
	}
	return &payment.PaymentNotification{
		TradeNo:  tradeNo,
		OrderID:  orderID,
		Amount:   payload.TransferAmount,
		Currency: sepayDefaultCurrency,
		Status:   payment.ProviderStatusSuccess,
		RawData:  rawBody,
	}, nil
}

func (s *Sepay) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("sepay refund not supported")
}

func (s *Sepay) CancelPayment(_ context.Context, _ string) error {
	return fmt.Errorf("sepay cancel payment: not supported for QR flow")
}

func (s *Sepay) bankAccountEndpoint() string {
	return s.sepayAPIBase() + "/bankaccounts/details/" + url.PathEscape(strings.TrimSpace(s.config["bankAccountId"]))
}

func (s *Sepay) bankAccountsEndpoint() string {
	return s.sepayAPIBase() + "/bankaccounts/list"
}

func (s *Sepay) sepayAPIBase() string {
	base := strings.TrimRight(strings.TrimSpace(s.config["apiBase"]), "/")
	if base == "" {
		base = sepayDefaultAPIBase
	}
	return base
}

func (s *Sepay) transactionsEndpoint(query url.Values) string {
	endpoint := s.sepayAPIBase() + "/transactions/list"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return endpoint
}

func (s *Sepay) getBankAccount(ctx context.Context) (*SepayBankAccount, error) {
	if strings.TrimSpace(s.config["bankAccountId"]) == "" {
		return s.discoverSingleBankAccount(ctx)
	}

	respBody, err := s.doRequest(ctx, http.MethodGet, s.bankAccountEndpoint(), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		BankAccount SepayBankAccount `json:"bankaccount"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse bank account response: %w", err)
	}
	if !isUsableSepayBankAccount(resp.BankAccount) {
		return nil, fmt.Errorf("bank account details missing account_number or bank_short_name")
	}
	return &resp.BankAccount, nil
}

func (s *Sepay) discoverSingleBankAccount(ctx context.Context) (*SepayBankAccount, error) {
	respBody, err := s.doRequest(ctx, http.MethodGet, s.bankAccountsEndpoint(), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		BankAccounts []SepayBankAccount `json:"bankaccounts"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse bank accounts response: %w", err)
	}
	usable := make([]SepayBankAccount, 0, len(resp.BankAccounts))
	for _, account := range resp.BankAccounts {
		if isUsableSepayBankAccount(account) {
			usable = append(usable, account)
		}
	}
	switch len(usable) {
	case 0:
		return nil, fmt.Errorf("no usable bank accounts returned by SePay")
	case 1:
		return &usable[0], nil
	default:
		return nil, fmt.Errorf("multiple bank accounts returned by SePay; configure bankAccountId explicitly")
	}
}

func isUsableSepayBankAccount(account SepayBankAccount) bool {
	return strings.TrimSpace(account.AccountNumber) != "" && strings.TrimSpace(account.BankShortName) != ""
}

func (s *Sepay) buildQRCodeURL(bankAccount *SepayBankAccount, amount int64, transferContent string) (string, error) {
	if bankAccount == nil {
		return "", fmt.Errorf("missing bank account")
	}
	qrBase := strings.TrimSpace(s.config["qrBase"])
	if qrBase == "" {
		qrBase = sepayDefaultQRBase
	}
	parsed, err := url.Parse(qrBase)
	if err != nil {
		return "", fmt.Errorf("invalid qr base: %w", err)
	}
	query := parsed.Query()
	query.Set("acc", strings.TrimSpace(bankAccount.AccountNumber))
	query.Set("bank", strings.TrimSpace(bankAccount.BankShortName))
	query.Set("amount", strconv.FormatInt(amount, 10))
	query.Set("des", strings.TrimSpace(transferContent))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func FetchSepayBankAccounts(ctx context.Context, config map[string]string) ([]SepayBankAccount, error) {
	sepay, err := NewSepay("_bank_accounts_", config)
	if err != nil {
		return nil, err
	}
	return sepay.ListBankAccounts(ctx)
}

func (s *Sepay) ListBankAccounts(ctx context.Context) ([]SepayBankAccount, error) {
	respBody, err := s.doRequest(ctx, http.MethodGet, s.bankAccountsEndpoint(), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		BankAccounts []SepayBankAccount `json:"bankaccounts"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse bank accounts response: %w", err)
	}
	return resp.BankAccounts, nil
}

func buildSepayTransferContent(orderID string) string {
	ref := buildSepayTransferReference(orderID)
	if ref == "" {
		return strings.TrimSpace(orderID)
	}
	phrases := []string{
		"Cam on VClaw",
		"Nap vi VClaw",
		"Gia han dich vu VClaw",
		"Thanh toan dich vu VClaw",
	}
	checksum := 0
	for _, ch := range orderID {
		checksum += int(ch)
	}
	return phrases[checksum%len(phrases)] + " " + ref
}

func buildSepayTransferReference(orderID string) string {
	suffix := strings.TrimSpace(orderID)
	lower := strings.ToLower(suffix)
	for _, prefix := range []string{sepayCanonicalOrderPrefix, sepayLegacyOrderPrefix} {
		if strings.HasPrefix(lower, prefix) {
			suffix = suffix[len(prefix):]
			break
		}
	}
	if suffix == "" {
		return ""
	}
	return "VC" + suffix
}

func NormalizeSepayOrderID(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if orderID := sepayOrderCodePattern.FindString(code); orderID != "" {
		return normalizePrefixedSepayOrderID(orderID)
	}
	if match := sepayTransferRefPattern.FindStringSubmatch(code); len(match) == 2 {
		return sepayCanonicalOrderPrefix + match[1]
	}
	if match := sepayOrderSuffixPattern.FindStringSubmatch(code); len(match) == 2 {
		return sepayCanonicalOrderPrefix + match[1]
	}
	return ""
}

func ExtractSepayOrderIDFromContent(content string) string {
	content = strings.TrimSpace(content)
	if orderID := sepayOrderCodePattern.FindString(content); orderID != "" {
		return normalizePrefixedSepayOrderID(orderID)
	}
	if match := sepayTransferRefPattern.FindStringSubmatch(content); len(match) == 2 {
		return sepayCanonicalOrderPrefix + match[1]
	}
	return ""
}

func normalizePrefixedSepayOrderID(orderID string) string {
	if strings.HasPrefix(strings.ToLower(orderID), sepayCanonicalOrderPrefix) {
		return strings.ToLower(orderID[:len(sepayCanonicalOrderPrefix)]) + orderID[len(sepayCanonicalOrderPrefix):]
	}
	if strings.HasPrefix(strings.ToLower(orderID), sepayLegacyOrderPrefix) {
		return strings.ToLower(orderID[:len(sepayLegacyOrderPrefix)]) + orderID[len(sepayLegacyOrderPrefix):]
	}
	return strings.TrimSpace(orderID)
}

func normalizeSepayOrderID(code string) string {
	return NormalizeSepayOrderID(code)
}

func extractSepayOrderCode(content string) string {
	return ExtractSepayOrderIDFromContent(content)
}

func (s *Sepay) doRequest(ctx context.Context, method, endpoint string, body []byte) ([]byte, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(s.config["apiToken"]))
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxSepayResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sepay api %s returned %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return bodyBytes, nil
}
