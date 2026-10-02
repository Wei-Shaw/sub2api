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
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	gpmpayBaseURL          = "https://api.gpmpay.com/api/v1"
	gpmpayHTTPTimeout      = 20 * time.Second
	gpmpayMaxResponseBytes = 1 << 20
	// gpmpaySignatureTolerance mirrors the SDK's DEFAULT_TOLERANCE_SECONDS.
	gpmpaySignatureTolerance = 5 * time.Minute
	// GPM Pay only watches Vietnamese bank accounts.
	gpmpayCurrency = "VND"
)

// GPMPay implements payment.Provider for GPM Pay (gpmpay.com).
//
// GPM Pay is not a checkout gateway: it watches the merchant's bank account and
// posts an HMAC-signed webhook for every transfer. There is no upstream order,
// so CreatePayment builds a VietQR locally with the order code in the transfer
// memo, and VerifyNotification recovers the order from that memo. Whether the
// amount covers the order is checked by the service layer, as for every provider.
type GPMPay struct {
	instanceID string
	config     map[string]string
	client     *http.Client
}

// NewGPMPay builds a GPM Pay provider from instance config.
//
// Config keys: apiToken, webhookSecret (the HMAC endpoint's signing secret),
// bankBin + accountNumber (the account GPM Pay watches), and allowSimulated
// ("true" to credit transfers from GPM Pay's simulator — for testing only).
func NewGPMPay(instanceID string, config map[string]string) (*GPMPay, error) {
	cfg := cloneStringMap(config)
	for _, key := range []string{"apiToken", "webhookSecret", "bankBin", "accountNumber"} {
		if strings.TrimSpace(cfg[key]) == "" {
			return nil, infraerrors.BadRequest("GPMPAY_CONFIG_MISSING_KEY",
				"gpmpay config missing required key: "+key).
				WithMetadata(map[string]string{"field": key})
		}
	}
	cfg["currency"] = gpmpayCurrency
	return &GPMPay{instanceID: instanceID, config: cfg, client: &http.Client{Timeout: gpmpayHTTPTimeout}}, nil
}

func (g *GPMPay) Name() string        { return "GPM Pay" }
func (g *GPMPay) ProviderKey() string { return payment.TypeGPMPay }

func (g *GPMPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeGPMPayBankTransfer}
}

func (g *GPMPay) cfg(key string) string { return strings.TrimSpace(g.config[key]) }

// CreatePayment builds the VietQR the payer scans. Purely local and replayable.
func (g *GPMPay) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	switch strings.TrimSpace(req.PaymentType) {
	case payment.TypeGPMPayBankTransfer, payment.TypeGPMPay, "":
	default:
		return nil, infraerrors.BadRequest("GPMPAY_UNSUPPORTED_PAYMENT_TYPE",
			"gpmpay does not support payment type: "+req.PaymentType)
	}
	// A fractional VND amount produces a QR that can never be reconciled.
	minor, err := payment.AmountToMinorUnit(req.Amount, gpmpayCurrency)
	if err != nil {
		return nil, fmt.Errorf("gpmpay create payment: %w", err)
	}
	return &payment.CreatePaymentResponse{
		TradeNo:    req.OrderID,
		QRCode:     buildVietQRPayload(g.cfg("bankBin"), g.cfg("accountNumber"), minor, payment.TransferCodeForOutTradeNo(req.OrderID)),
		Currency:   gpmpayCurrency,
		ResultType: payment.CreatePaymentResultOrderCreated,
	}, nil
}

// gpmpayWebhookPayload is the subset of the webhook body we rely on.
type gpmpayWebhookPayload struct {
	ID             string      `json:"id"`
	Content        string      `json:"content"`
	TransferType   string      `json:"transferType"`
	TransferAmount json.Number `json:"transferAmount"`
	Source         string      `json:"source"`
	// Test marks the ping GPM Pay sends when the endpoint is registered.
	Test bool `json:"test"`
}

// VerifyNotification checks the HMAC signature and maps an incoming transfer to an order.
//
// GPM Pay posts every transfer on the account, so outgoing transfers, the
// registration ping and transfers without an order code are acknowledged and ignored.
// So are simulated transfers unless allowSimulated is set: anyone with access to
// the GPM Pay dashboard can fire one, and crediting them would mint balance.
func (g *GPMPay) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := gpmpayVerifySignature(headers["x-gpmpay-signature"], rawBody, g.cfg("webhookSecret"), time.Now()); err != nil {
		return nil, err
	}
	var p gpmpayWebhookPayload
	if err := json.Unmarshal([]byte(rawBody), &p); err != nil {
		return nil, fmt.Errorf("gpmpay notification decode: %w", err)
	}
	if p.Test || !strings.EqualFold(p.TransferType, "in") ||
		(strings.EqualFold(p.Source, "SIMULATED") && g.cfg("allowSimulated") != "true") {
		return nil, nil
	}
	outTradeNo := payment.OutTradeNoFromTransferContent(p.Content)
	if outTradeNo == "" {
		return nil, nil
	}
	amount, err := decimal.NewFromString(p.TransferAmount.String())
	if err != nil {
		return nil, fmt.Errorf("gpmpay notification amount %q: %w", p.TransferAmount, err)
	}
	return &payment.PaymentNotification{
		TradeNo: p.ID,
		OrderID: outTradeNo,
		Amount:  amount,
		Status:  payment.NotificationStatusSuccess,
		RawData: rawBody,
	}, nil
}

// gpmpayVerifySignature checks X-GPMPay-Signature: "t=<unix>,v1=<hex HMAC-SHA256(secret, t + "." + body)>".
func gpmpayVerifySignature(header, rawBody, secret string, now time.Time) error {
	var ts int64
	var v1 string
	for _, segment := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(segment, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "t":
			ts, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "v1":
			v1 = strings.TrimSpace(value)
		}
	}
	if ts <= 0 || v1 == "" {
		return fmt.Errorf("gpmpay notification missing or malformed X-GPMPay-Signature")
	}
	if skew := now.Sub(time.Unix(ts, 0)); skew > gpmpaySignatureTolerance || skew < -gpmpaySignatureTolerance {
		return fmt.Errorf("gpmpay notification timestamp outside tolerance (t=%d)", ts)
	}
	provided, err := hex.DecodeString(v1)
	if err != nil {
		return fmt.Errorf("gpmpay notification signature is not hex")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	_, _ = mac.Write([]byte(rawBody))
	if !hmac.Equal(mac.Sum(nil), provided) {
		return fmt.Errorf("gpmpay notification signature mismatch")
	}
	return nil
}

// gpmpayTransaction is the subset of a REST transaction we rely on.
// REST names differ from the webhook's: transferContent/amount/type vs content/transferAmount/transferType.
type gpmpayTransaction struct {
	ID              string      `json:"id"`
	Type            string      `json:"type"`
	Amount          json.Number `json:"amount"`
	TransferContent string      `json:"transferContent"`
	TransactionTime string      `json:"transactionTime"`
}

// QueryOrder searches the account's incoming transfers for the order code.
// It is the fallback for a missed webhook; the largest matching transfer wins,
// mirroring the webhook path, which judges each transfer on its own.
func (g *GPMPay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, fmt.Errorf("gpmpay query order: trade number is required")
	}
	query := url.Values{
		"search": {payment.TransferCodeForOutTradeNo(tradeNo)},
		"type":   {"IN"},
		"limit":  {"50"},
	}
	body, err := g.get(ctx, "/transactions?"+query.Encode())
	if err != nil {
		return nil, fmt.Errorf("gpmpay query order %s: %w", tradeNo, err)
	}
	txns, err := gpmpayDecodeTransactions(body)
	if err != nil {
		return nil, fmt.Errorf("gpmpay query order %s: %w", tradeNo, err)
	}

	resp := &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Status:   payment.ProviderStatusPending,
		Metadata: map[string]string{"currency": gpmpayCurrency},
	}
	for _, t := range txns {
		if !strings.EqualFold(t.Type, "IN") || !strings.EqualFold(payment.OutTradeNoFromTransferContent(t.TransferContent), tradeNo) {
			continue
		}
		amount, err := decimal.NewFromString(t.Amount.String())
		if err != nil || !amount.GreaterThan(resp.Amount) {
			continue
		}
		resp.TradeNo = t.ID
		resp.Status = payment.ProviderStatusPaid
		resp.Amount = amount
		resp.PaidAt = t.TransactionTime
	}
	return resp, nil
}

func (g *GPMPay) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gpmpayBaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.cfg("apiToken"))
	req.Header.Set("Accept", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, gpmpayMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, nowPaymentsTruncate(string(body)))
	}
	return body, nil
}

// gpmpayDecodeTransactions unwraps a list response: {statusCode, message, data: {data: [...], meta}}.
func gpmpayDecodeTransactions(body []byte) ([]gpmpayTransaction, error) {
	var envelope struct {
		Data struct {
			Data []gpmpayTransaction `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode transactions: %w", err)
	}
	return envelope.Data.Data, nil
}

// buildVietQRPayload builds an EMVCo VietQR (NAPAS 247) string, ported from the SDK's buildVietQrPayload.
func buildVietQRPayload(bankBin, accountNumber string, amount int64, description string) string {
	tlv := func(id, value string) string { return id + fmt.Sprintf("%02d", len(value)) + value }
	if len(description) > 25 {
		description = description[:25]
	}
	beneficiary := tlv("00", bankBin) + tlv("01", accountNumber)
	merchantInfo := tlv("00", "A000000727") + tlv("01", beneficiary) + tlv("02", "QRIBFTTA")
	base := tlv("00", "01") + tlv("01", "12") + tlv("38", merchantInfo) + tlv("53", "704") +
		tlv("54", strconv.FormatInt(amount, 10)) + tlv("58", "VN") + tlv("62", tlv("08", description)) + "6304"
	return base + fmt.Sprintf("%04X", crc16CCITT(base))
}

func crc16CCITT(s string) uint16 {
	crc := uint16(0xFFFF)
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
