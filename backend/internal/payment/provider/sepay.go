package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
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

type Sepay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

type sepayBankAccount struct {
	ID                string `json:"id"`
	AccountNumber     string `json:"account_number"`
	BankShortName     string `json:"bank_short_name"`
	AccountHolderName string `json:"account_holder_name"`
}

func NewSepay(instanceID string, config map[string]string) (*Sepay, error) {
	for _, k := range []string{"apiToken", "bankAccountId", "notifyUrl"} {
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
	qr, err := s.buildQRCodeURL(bankAccount, int64(math.Round(amount)), req.OrderID)
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
	wantedUpper := strings.ToUpper(wanted)
	for _, tx := range resp.Transactions {
		if strings.EqualFold(strings.TrimSpace(tx.Code), wanted) || strings.Contains(strings.ToUpper(tx.TransactionContent), wantedUpper) {
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
		const prefix = "Apikey "
		if !strings.HasPrefix(auth, prefix) || strings.TrimSpace(strings.TrimPrefix(auth, prefix)) != apiKey {
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
	orderID := strings.TrimSpace(payload.Code)
	if orderID == "" {
		orderID = strings.TrimSpace(payload.TransactionContent)
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
	base := strings.TrimRight(strings.TrimSpace(s.config["apiBase"]), "/")
	if base == "" {
		base = sepayDefaultAPIBase
	}
	return base + "/bankaccounts/details/" + url.PathEscape(strings.TrimSpace(s.config["bankAccountId"]))
}

func (s *Sepay) transactionsEndpoint(query url.Values) string {
	base := strings.TrimRight(strings.TrimSpace(s.config["apiBase"]), "/")
	if base == "" {
		base = sepayDefaultAPIBase
	}
	endpoint := base + "/transactions/list"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return endpoint
}

func (s *Sepay) getBankAccount(ctx context.Context) (*sepayBankAccount, error) {
	respBody, err := s.doRequest(ctx, http.MethodGet, s.bankAccountEndpoint(), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		BankAccount sepayBankAccount `json:"bankaccount"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse bank account response: %w", err)
	}
	if strings.TrimSpace(resp.BankAccount.AccountNumber) == "" || strings.TrimSpace(resp.BankAccount.BankShortName) == "" {
		return nil, fmt.Errorf("bank account details missing account_number or bank_short_name")
	}
	return &resp.BankAccount, nil
}

func (s *Sepay) buildQRCodeURL(bankAccount *sepayBankAccount, amount int64, orderCode string) (string, error) {
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
	query.Set("des", strings.TrimSpace(orderCode))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
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
