package provider

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const antomAPIPath = "/ams/api/v1/payments/"

// Antom uses Hosted Checkout with automatic capture. TradeNo remains the
// merchant paymentRequestId because session creation does not return paymentId.
type Antom struct {
	config     map[string]string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	client     *http.Client
}

func NewAntom(_ string, config map[string]string) (*Antom, error) {
	cfg := cloneStringMap(config)
	for _, field := range []string{"clientId", "merchantPrivateKey", "antomPublicKey"} {
		cfg[field] = strings.TrimSpace(cfg[field])
		if cfg[field] == "" {
			return nil, fmt.Errorf("antom config missing required key: %s", field)
		}
	}
	base := strings.TrimRight(strings.TrimSpace(cfg["apiBase"]), "/")
	if base == "" {
		base = "https://open-sea-global.alipay.com"
	}
	switch base {
	case "https://open-sea-global.alipay.com", "https://open-na-global.alipay.com", "https://open-de-global.alipay.com", "https://open.antglobal-us.com", "https://open-sea.alipay.com", "https://open-na.alipay.com":
	default:
		return nil, fmt.Errorf("antom apiBase must be an official HTTPS regional gateway origin")
	}
	cfg["apiBase"] = base
	if strings.TrimSpace(cfg["keyVersion"]) == "" {
		cfg["keyVersion"] = "1"
	}
	if version, err := strconv.Atoi(cfg["keyVersion"]); err != nil || version < 1 {
		return nil, fmt.Errorf("antom keyVersion must be a positive integer")
	}
	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("antom currency: %w", err)
	}
	cfg["currency"] = currency
	if strings.TrimSpace(cfg["settlementCurrency"]) != "" {
		cfg["settlementCurrency"], err = payment.NormalizePaymentCurrency(cfg["settlementCurrency"])
		if err != nil {
			return nil, fmt.Errorf("antom settlementCurrency: %w", err)
		}
	}
	privateDER, err := antomKeyDER(cfg["merchantPrivateKey"])
	if err != nil {
		return nil, fmt.Errorf("antom merchantPrivateKey: %w", err)
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(privateDER)
	if err != nil {
		parsed, parseErr := x509.ParsePKCS8PrivateKey(privateDER)
		if parseErr != nil {
			return nil, fmt.Errorf("antom merchantPrivateKey must be PKCS1 or PKCS8 RSA")
		}
		var ok bool
		privateKey, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("antom merchantPrivateKey must be RSA")
		}
	}
	if privateKey.N.BitLen() < 2048 {
		return nil, fmt.Errorf("antom merchantPrivateKey must be at least 2048 bits")
	}
	if err := privateKey.Validate(); err != nil {
		return nil, fmt.Errorf("antom merchantPrivateKey is invalid")
	}
	publicDER, err := antomKeyDER(cfg["antomPublicKey"])
	if err != nil {
		return nil, fmt.Errorf("antom antomPublicKey: %w", err)
	}
	var publicKey *rsa.PublicKey
	if parsed, parseErr := x509.ParsePKIXPublicKey(publicDER); parseErr == nil {
		publicKey, _ = parsed.(*rsa.PublicKey)
	} else {
		publicKey, _ = x509.ParsePKCS1PublicKey(publicDER)
	}
	if publicKey == nil || publicKey.N.BitLen() < 2048 {
		return nil, fmt.Errorf("antom antomPublicKey must be an RSA public key of at least 2048 bits")
	}
	return &Antom{config: cfg, privateKey: privateKey, publicKey: publicKey, client: &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func antomKeyDER(raw string) ([]byte, error) {
	if block, rest := pem.Decode([]byte(raw)); block != nil {
		if len(bytes.TrimSpace(rest)) != 0 {
			return nil, fmt.Errorf("unexpected data after PEM key")
		}
		return block.Bytes, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(raw), ""))
	if err != nil {
		return nil, fmt.Errorf("expected PEM or base64 DER key")
	}
	return decoded, nil
}

func (a *Antom) Name() string        { return "Antom" }
func (a *Antom) ProviderKey() string { return payment.TypeAntom }
func (a *Antom) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAntom}
}
func (a *Antom) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"app_id": a.config["clientId"], "currency": a.config["currency"]}
}

func (a *Antom) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := a.amount(req.Amount)
	if err != nil {
		return nil, err
	}
	if req.OrderID == "" || len(req.OrderID) > 64 {
		return nil, fmt.Errorf("antom requires a paymentRequestId of 1-64 bytes")
	}
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = a.config["notifyUrl"]
	}
	for _, raw := range []string{notifyURL, req.ReturnURL} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("antom requires absolute HTTP(S) notify and return URLs")
		}
	}
	env := map[string]string{"terminalType": "WEB", "clientIp": req.ClientIP}
	if req.IsMobile {
		ua := strings.ToLower(req.UserAgent)
		switch {
		case strings.Contains(ua, "android"):
			env["osType"] = "ANDROID"
		case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"), strings.Contains(ua, "ipod"), strings.Contains(ua, "macintosh"):
			env["osType"] = "IOS"
		}
		// Use browser checkout when the device OS cannot be identified rather than
		// sending an invalid WAP environment or inventing an operating system.
		if env["osType"] != "" {
			env["terminalType"] = "WAP"
		}
	}
	payload := map[string]any{
		"productCode": "CASHIER_PAYMENT", "productScene": "CHECKOUT_PAYMENT", "paymentRequestId": req.OrderID,
		"paymentAmount": amount, "paymentNotifyUrl": notifyURL, "paymentRedirectUrl": req.ReturnURL,
		"paymentFactor": map[string]string{"captureMode": "AUTOMATIC"},
		"order":         map[string]any{"referenceOrderId": req.OrderID, "orderDescription": req.Subject, "orderAmount": amount, "buyer": map[string]string{"referenceBuyerId": req.BuyerID}},
		"env":           env,
	}
	if currency := a.config["settlementCurrency"]; currency != "" {
		payload["settlementStrategy"] = map[string]string{"settlementCurrency": currency}
	}
	var resp antomResponse
	if err := a.call(ctx, "createPaymentSession", payload, &resp); err != nil {
		return nil, err
	}
	if err := resp.Result.success(); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(resp.NormalURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("antom returned no valid HTTPS checkout URL")
	}
	return &payment.CreatePaymentResponse{TradeNo: req.OrderID, PayURL: resp.NormalURL, Currency: a.config["currency"]}, nil
}

func (a *Antom) inquire(ctx context.Context, key, id string) (*antomResponse, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("antom payment identifier is required")
	}
	var resp antomResponse
	if err := a.call(ctx, "inquiryPayment", map[string]string{key: id}, &resp); err != nil {
		return nil, err
	}
	if err := resp.Result.success(); err != nil {
		return nil, err
	}
	if resp.PaymentRequestID == "" || resp.PaymentID == "" {
		return nil, fmt.Errorf("antom inquiry missing payment identifiers")
	}
	if (key == "paymentRequestId" && resp.PaymentRequestID != id) || (key == "paymentId" && resp.PaymentID != id) {
		return nil, fmt.Errorf("antom inquiry payment identifier mismatch")
	}
	return &resp, nil
}

func (a *Antom) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	resp, err := a.inquire(ctx, "paymentRequestId", tradeNo)
	if err != nil {
		return nil, err
	}
	amount, err := a.readAmount(resp.PaymentAmount)
	if err != nil {
		return nil, err
	}
	status := payment.ProviderStatusPending
	switch resp.PaymentStatus {
	case "FAIL", "CANCELLED":
		status = payment.ProviderStatusFailed
	case "SUCCESS":
		// Missing method information is not evidence of an APM payment. Fail closed
		// until capture proof arrives rather than crediting a card authorization.
		if resp.PaymentMethodType != "" && !antomNeedsCapture(resp.PaymentMethodType) {
			status = payment.ProviderStatusPaid
		} else {
			captured := decimal.Zero
			for _, transaction := range resp.Transactions {
				if transaction.Type != "CAPTURE" || transaction.Status != "SUCCESS" || transaction.Result.Status != "S" {
					continue
				}
				value, err := a.readAmount(transaction.Amount)
				if err != nil {
					return nil, err
				}
				captured = captured.Add(decimal.NewFromFloat(value))
			}
			if captured.Equal(decimal.NewFromFloat(amount)) {
				status = payment.ProviderStatusPaid
			}
		}
	}
	return &payment.QueryOrderResponse{TradeNo: resp.PaymentRequestID, Status: status, Amount: amount, PaidAt: resp.PaymentTime, Metadata: a.MerchantIdentityMetadata()}, nil
}

func antomNeedsCapture(method string) bool {
	switch method {
	case "CARD", "APPLEPAY", "GOOGLEPAY":
		return true
	}
	return false
}

func (a *Antom) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	path, method := headers[":path"], headers[":method"]
	if path == "" {
		u, err := url.Parse(a.config["notifyUrl"])
		if err != nil || u.Path == "" {
			return nil, fmt.Errorf("antom notification request target missing")
		}
		path = u.RequestURI()
	}
	if method == "" {
		method = http.MethodPost
	}
	if err := a.verify(method, path, headers["client-id"], headers["request-time"], headers["signature"], []byte(rawBody)); err != nil {
		return nil, err
	}
	var event antomResponse
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("antom invalid notification JSON")
	}
	if event.NotifyType != "PAYMENT_RESULT" && event.NotifyType != "CAPTURE_RESULT" {
		return nil, nil
	}
	if event.Result.Status != "S" {
		return nil, nil
	}
	if event.Result.Code != "SUCCESS" {
		return nil, fmt.Errorf("antom inconsistent notification result")
	}
	var amount float64
	var err error
	switch event.NotifyType {
	case "PAYMENT_RESULT":
		if event.PaymentRequestID == "" || event.PaymentID == "" {
			return nil, fmt.Errorf("antom notification missing payment identifiers")
		}
		if antomNeedsCapture(event.PaymentMethodType) {
			return nil, nil
		}
		if event.PaymentMethodType == "" {
			// Missing method type cannot prove capture. Query for authoritative
			// payment details rather than inferring settlement from API success.
			queried, err := a.QueryOrder(ctx, event.PaymentRequestID)
			if err != nil {
				return nil, err
			}
			if queried.Status != payment.ProviderStatusPaid {
				return nil, nil
			}
			amount = queried.Amount
			notifiedAmount, err := a.readAmount(event.PaymentAmount)
			if err != nil || notifiedAmount != amount {
				return nil, fmt.Errorf("antom notification amount mismatch")
			}
		} else {
			amount, err = a.readAmount(event.PaymentAmount)
		}
	case "CAPTURE_RESULT":
		amount, err = a.readAmount(event.CaptureAmount)
		if err != nil {
			return nil, err
		}
		queried, err := a.inquire(ctx, "paymentId", event.PaymentID)
		if err != nil {
			return nil, err
		}
		expected, err := a.readAmount(queried.PaymentAmount)
		if err != nil {
			return nil, err
		}
		if amount != expected {
			return nil, fmt.Errorf("antom capture does not cover the full order amount")
		}
		event.PaymentRequestID = queried.PaymentRequestID
	}
	if err != nil {
		return nil, err
	}
	return &payment.PaymentNotification{TradeNo: event.PaymentRequestID, OrderID: event.PaymentRequestID, Amount: amount, Status: payment.NotificationStatusSuccess, RawData: rawBody, Metadata: a.MerchantIdentityMetadata()}, nil
}

func antomRefundRequestID(tradeNo, attemptID string) string {
	digest := sha256.Sum256([]byte(tradeNo + "\n" + attemptID))
	return hex.EncodeToString(digest[:])
}

func (a *Antom) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	amount, err := a.amount(req.Amount)
	if err != nil {
		return nil, err
	}
	original, err := a.inquire(ctx, "paymentRequestId", req.TradeNo)
	if err != nil {
		return nil, err
	}
	if _, err := a.readAmount(original.PaymentAmount); err != nil {
		return nil, err
	}
	id := antomRefundRequestID(req.TradeNo, req.AttemptID)
	payload := map[string]any{"paymentId": original.PaymentID, "refundRequestId": id, "refundAmount": amount, "refundReason": req.Reason}
	if notifyURL := a.config["notifyUrl"]; notifyURL != "" {
		payload["refundNotifyUrl"] = notifyURL
	}
	var resp antomResponse
	result := &payment.RefundResponse{RefundID: id, Status: payment.ProviderStatusPending}
	if err := a.call(ctx, "refund", payload, &resp); err != nil {
		// The gateway may have accepted the refund before the connection failed.
		// Preserve the idempotency key so inquiry can resolve the uncertainty.
		return result, err
	}
	switch resp.Result.Status {
	case "S":
		if err := resp.Result.success(); err != nil {
			return result, err
		}
		result.Status = payment.ProviderStatusSuccess
	case "F":
		result.Status = payment.ProviderStatusFailed
		return result, resp.Result.success()
	case "U":
	default:
		return result, fmt.Errorf("antom refund missing result status")
	}
	return result, nil
}

func (a *Antom) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	id := strings.TrimSpace(req.RefundID)
	if id == "" {
		return nil, fmt.Errorf("antom refund request identifier missing")
	}
	var resp antomResponse
	if err := a.call(ctx, "inquiryRefund", map[string]string{"refundRequestId": id}, &resp); err != nil {
		return nil, err
	}
	if err := resp.Result.success(); err != nil {
		return nil, err
	}
	if resp.RefundRequestID != "" && resp.RefundRequestID != id {
		return nil, fmt.Errorf("antom refund identifier mismatch")
	}
	status := payment.ProviderStatusPending
	switch resp.RefundStatus {
	case "SUCCESS":
		actual, err := a.readAmount(resp.RefundAmount)
		if err != nil {
			return nil, err
		}
		expected, err := a.amount(req.Amount)
		if err != nil {
			return nil, err
		}
		expectedValue, _ := a.readAmount(expected)
		if actual != expectedValue {
			return nil, fmt.Errorf("antom refund amount mismatch")
		}
		status = payment.ProviderStatusSuccess
	case "FAIL":
		status = payment.ProviderStatusFailed
	}
	return &payment.RefundResponse{RefundID: id, Status: status}, nil
}

func (a *Antom) CancelPayment(ctx context.Context, tradeNo string) error {
	if strings.TrimSpace(tradeNo) == "" {
		return fmt.Errorf("antom payment identifier is required")
	}
	var resp antomResponse
	if err := a.call(ctx, "cancel", map[string]string{"paymentRequestId": tradeNo}, &resp); err != nil {
		return err
	}
	return resp.Result.success()
}

func antomMinorUnit(currency string) int {
	// The shared helper preserves Stripe's legacy two-decimal transport for
	// ISK/UGX. Antom follows ISO minor units instead.
	if currency == "ISK" || currency == "UGX" {
		return 0
	}
	return payment.CurrencyMinorUnit(currency)
}

func (a *Antom) amount(raw string) (antomAmount, error) {
	currency := a.config["currency"]
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !value.IsPositive() {
		return antomAmount{}, fmt.Errorf("antom amount must be positive")
	}
	minor := value.Shift(int32(antomMinorUnit(currency)))
	if !minor.IsInteger() || minor.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return antomAmount{}, fmt.Errorf("antom amount exceeds currency precision or range")
	}
	if currency == "IDR" && !value.IsInteger() {
		return antomAmount{}, fmt.Errorf("antom IDR amount must be a whole rupiah")
	}
	return antomAmount{Currency: currency, Value: minor.StringFixed(0)}, nil
}

func (a *Antom) readAmount(amount antomAmount) (float64, error) {
	if amount.Currency != a.config["currency"] {
		return 0, fmt.Errorf("antom currency mismatch")
	}
	for _, c := range amount.Value {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("antom amount must be an integer minor unit")
		}
	}
	value, err := strconv.ParseInt(amount.Value, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("antom invalid payment amount")
	}
	return decimal.NewFromInt(value).Shift(-int32(antomMinorUnit(amount.Currency))).InexactFloat64(), nil
}

func (a *Antom) call(ctx context.Context, operation string, payload any, out *antomResponse) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	path := antomAPIPath + operation
	requestTime := strconv.FormatInt(time.Now().UnixMilli(), 10)
	digest := sha256.Sum256(antomSignatureContent(http.MethodPost, path, a.config["clientId"], requestTime, body))
	signature, err := rsa.SignPKCS1v15(rand.Reader, a.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return fmt.Errorf("antom request signing failed: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config["apiBase"]+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("client-id", a.config["clientId"])
	req.Header.Set("request-time", requestTime)
	req.Header.Set("signature", "algorithm=RSA256, keyVersion="+a.config["keyVersion"]+", signature="+url.QueryEscape(base64.StdEncoding.EncodeToString(signature)))
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("antom %s request failed: %w", operation, err)
	}
	defer resp.Body.Close()
	const maxResponseSize = 1 << 20
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return fmt.Errorf("antom read response: %w", err)
	}
	if len(responseBody) > maxResponseSize {
		return fmt.Errorf("antom response too large")
	}
	if err := a.verify(http.MethodPost, path, resp.Header.Get("client-id"), resp.Header.Get("response-time"), resp.Header.Get("signature"), responseBody); err != nil {
		return fmt.Errorf("antom %s HTTP status %d: %w", operation, resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("antom %s HTTP status %d", operation, resp.StatusCode)
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("antom invalid %s response JSON", operation)
	}
	return nil
}

func antomSignatureContent(method, path, clientID, timestamp string, body []byte) []byte {
	return []byte(method + " " + path + "\n" + clientID + "." + timestamp + "." + string(body))
}

func (a *Antom) verify(method, path, clientID, timestamp, header string, body []byte) error {
	if method != http.MethodPost || path == "" || clientID != a.config["clientId"] || timestamp == "" {
		return fmt.Errorf("antom invalid signature context")
	}
	fields := map[string]string{}
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || fields[key] != "" {
			return fmt.Errorf("antom invalid signature header")
		}
		fields[key] = value
	}
	if fields["algorithm"] != "RSA256" || fields["keyVersion"] == "" || fields["signature"] == "" {
		return fmt.Errorf("antom invalid signature algorithm or fields")
	}
	encoded, err := url.QueryUnescape(fields["signature"])
	if err != nil {
		return fmt.Errorf("antom invalid signature encoding")
	}
	signature, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("antom invalid signature encoding")
	}
	digest := sha256.Sum256(antomSignatureContent(method, path, clientID, timestamp, body))
	if err := rsa.VerifyPKCS1v15(a.publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return fmt.Errorf("antom signature verification failed")
	}
	return nil
}

type antomAmount struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}
type antomResult struct {
	Code   string `json:"resultCode"`
	Status string `json:"resultStatus"`
}

func (r antomResult) success() error {
	if r.Status == "S" && r.Code == "SUCCESS" {
		return nil
	}
	return fmt.Errorf("antom operation not successful: status=%s code=%s", r.Status, r.Code)
}

type antomTransaction struct {
	Type   string      `json:"transactionType"`
	Status string      `json:"transactionStatus"`
	Result antomResult `json:"transactionResult"`
	Amount antomAmount `json:"transactionAmount"`
}
type antomResponse struct {
	Result            antomResult        `json:"result"`
	NormalURL         string             `json:"normalUrl"`
	NotifyType        string             `json:"notifyType"`
	PaymentRequestID  string             `json:"paymentRequestId"`
	PaymentID         string             `json:"paymentId"`
	PaymentStatus     string             `json:"paymentStatus"`
	PaymentMethodType string             `json:"paymentMethodType"`
	PaymentAmount     antomAmount        `json:"paymentAmount"`
	PaymentTime       string             `json:"paymentTime"`
	CaptureAmount     antomAmount        `json:"captureAmount"`
	Transactions      []antomTransaction `json:"transactions"`
	RefundRequestID   string             `json:"refundRequestId"`
	RefundStatus      string             `json:"refundStatus"`
	RefundAmount      antomAmount        `json:"refundAmount"`
}

var (
	_ payment.Provider                 = (*Antom)(nil)
	_ payment.RefundQueryProvider      = (*Antom)(nil)
	_ payment.CancelableProvider       = (*Antom)(nil)
	_ payment.MerchantIdentityProvider = (*Antom)(nil)
)
