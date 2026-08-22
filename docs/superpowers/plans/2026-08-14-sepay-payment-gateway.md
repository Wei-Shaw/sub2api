# SePay Payment Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the SePay bank-transfer gateway (VietQR + webhook) as a new payment provider supporting VND recharge and subscription orders.

**Architecture:** New `sepay` provider in `backend/internal/payment/provider/` following the EasyPay pattern. Payment creation is offline (build VietQR EMV payload locally); SePay API v2 is only used for `QueryOrder`; webhook `POST /api/v1/payment/webhook/sepay` confirms payments (HMAC-SHA256 or API-key auth, JSON `{"success":true}` response). A new `SUBSCRIPTION_USD_TO_VND_RATE` setting converts USD plan prices to VND.

**Tech Stack:** Go (gin, ent, shopspring/decimal) — no new dependencies. Vue 3 + vitest frontend. Spec: `docs/superpowers/specs/2026-08-14-sepay-payment-gateway-design.md`.

## Global Constraints

- Repo layout: Go backend in `backend/` (module `github.com/Wei-Shaw/sub2api`), Vue frontend in `frontend/`. Run backend tests from `backend/`: `cd backend && go test ./internal/...`.
- SePay webhook success response MUST be HTTP 200 with body `{"success": true}` (JSON) — SePay retries otherwise.
- VND is zero-decimal: amounts are integers, no cents. QR amount tag carries plain integer digits.
- Do not break existing providers (easypay, alipay, wxpay, stripe, airwallex). Keep their tests green.
- Webhook verification uses the raw body bytes exactly as received; signature string is `sha256={hex(hmac_sha256(timestamp + "." + rawBody, secret))}`; reject timestamp skew > 300 s.
- Bank apps may uppercase transfer content and SePay's code extraction may drop the `sub2_` prefix, so any code→order matching must tolerate both (case-insensitive, with/without prefix).
- Order ID format: `sub2_` + YYYYMMDD + 8 alphanumeric chars (constant `orderIDPrefix = "sub2_"` in `backend/internal/service/payment_service.go:49`).
- API v2: base `https://userapi.sepay.vn` (sandbox `https://userapi-sandbox.sepay.vn`), `Authorization: Bearer {apiToken}`, rate limit 3 req/s → `GET /v2/transactions?q={code}&transfer_type=in`.
- No refund API: `Refund` returns a "not supported" error; admin must not enable refund for sepay instances.
- All new exported Go symbols need doc comments matching repo style (short English comments).
- Commit after every task (git identity already configured repo-local).

---

### Task 1: `sepay` payment type + VND currency resolution

**Files:**
- Modify: `backend/internal/payment/types.go` (constants block at lines 12-20, `GetBasePaymentType` at ~line 81)
- Modify: `backend/internal/payment/currency.go` (add `CurrencyVND` const)
- Modify: `backend/internal/service/payment_currency.go:10-19`
- Test: `backend/internal/payment/types_test.go` (create or append)
- Test: `backend/internal/service/payment_currency_test.go` (create or append)

**Interfaces:**
- Produces: `payment.TypeSePay PaymentType = "sepay"` (used by every later task); `payment.CurrencyVND = "VND"`; service function `paymentProviderConfigCurrency("sepay", cfg) == "VND"`.

- [ ] **Step 1: Write failing tests**

`backend/internal/payment/types_test.go` (append inside package `payment`; create file with `package payment` if missing):

```go
package payment

import "testing"

func TestGetBasePaymentTypeSePay(t *testing.T) {
	if got := GetBasePaymentType("sepay"); got != TypeSePay {
		t.Fatalf("GetBasePaymentType(sepay) = %q, want %q", got, TypeSePay)
	}
	if got := GetBasePaymentType(string(TypeSePay)); got != TypeSePay {
		t.Fatalf("GetBasePaymentType(TypeSePay) = %q, want %q", got, TypeSePay)
	}
}
```

`backend/internal/service/payment_currency_test.go` (same package as other payment service tests — `service`):

```go
package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestPaymentProviderConfigCurrencySePay(t *testing.T) {
	if got := paymentProviderConfigCurrency(payment.TypeSePay, map[string]string{}); got != "VND" {
		t.Fatalf("sepay currency = %q, want VND", got)
	}
	// SePay is VND-only: a bogus currency config must not leak CNY default.
	if got := paymentProviderConfigCurrency(payment.TypeSePay, map[string]string{"currency": "USD"}); got != "VND" {
		t.Fatalf("sepay currency with override = %q, want VND", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/payment/ -run TestGetBasePaymentTypeSePay -v && go test ./internal/service/ -run TestPaymentProviderConfigCurrencySePay -v`
Expected: FAIL — `TypeSePay` undefined / currency returns `CNY`.

- [ ] **Step 3: Implement**

`backend/internal/payment/types.go` — add to the constant block after `TypeAirwallex` (line 20):

```go
	TypeSePay PaymentType = "sepay"
```

In `GetBasePaymentType`, add as the first case (before the EasyPay case):

```go
	case t == TypeSePay:
		return TypeSePay
```

`backend/internal/payment/currency.go` — add near `DefaultPaymentCurrency`:

```go
// CurrencyVND is the only currency SePay bank transfers support.
const CurrencyVND = "VND"
```

`backend/internal/service/payment_currency.go` — extend the switch in `paymentProviderConfigCurrency`:

```go
	case payment.TypeSePay:
		// SePay monitors Vietnamese bank transfers: VND only, not configurable.
		return payment.CurrencyVND
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/payment/ ./internal/service/ -run 'TestGetBasePaymentTypeSePay|TestPaymentProviderConfigCurrencySePay' -v`
Expected: PASS both.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/payment/types.go backend/internal/payment/currency.go backend/internal/service/payment_currency.go backend/internal/payment/types_test.go backend/internal/service/payment_currency_test.go
git commit -m "feat(payment): add sepay payment type with VND-only currency"
```

---

### Task 2: VietQR EMV payload builder

**Files:**
- Create: `backend/internal/payment/provider/vietqr.go`
- Test: `backend/internal/payment/provider/vietqr_test.go`

**Interfaces:**
- Produces (package `provider`): `buildVietQRPayload(bin, accountNumber string, amountVND int64, content string) string` — used by Task 3.

- [ ] **Step 1: Write failing tests**

`backend/internal/payment/provider/vietqr_test.go`:

```go
package provider

import (
	"strings"
	"testing"
)

func TestCRC16CCITTFalse(t *testing.T) {
	// Standard check value for CRC-16/CCITT-FALSE.
	if got := crc16CCITTFalse("123456789"); got != 0x29B1 {
		t.Fatalf("crc16CCITTFalse(123456789) = %#04x, want 0x29b1", got)
	}
}

func parseTLV(t *testing.T, payload, tag string) string {
	t.Helper()
	for i := 0; i+4 <= len(payload); {
		id := payload[i : i+2]
		len, ok := parseTwoDigitInt(payload[i+2 : i+4])
		if !ok || i+4+len > len(payload) {
			t.Fatalf("malformed TLV at offset %d", i)
		}
		value := payload[i+4 : i+4+len]
		if id == tag {
			return value
		}
		i += 4 + len
	}
	return ""
}

func parseTwoDigitInt(s string) (int, bool) {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

func TestBuildVietQRPayload(t *testing.T) {
	got := buildVietQRPayload("970422", "0123456789", 10000, "sub2_20260814aB3kX9mQ")

	if want := "000201010212"; !strings.HasPrefix(got, want) {
		t.Fatalf("prefix = %q, want %q", got[:12], want)
	}
	if v := parseTLV(t, got, "53"); v != "704" {
		t.Fatalf("currency tag 53 = %q, want 704", v)
	}
	if v := parseTLV(t, got, "54"); v != "10000" {
		t.Fatalf("amount tag 54 = %q, want 10000", v)
	}
	if v := parseTLV(t, got, "58"); v != "VN" {
		t.Fatalf("country tag 58 = %q, want VN", v)
	}
	merchant := parseTLV(t, got, "38")
	if v := parseTLV(t, merchant, "00"); v != "A000000727" {
		t.Fatalf("napas GUID = %q, want A000000727", v)
	}
	if v := parseTLV(t, merchant, "01"); v != "970422" {
		t.Fatalf("bin = %q, want 970422", v)
	}
	if v := parseTLV(t, merchant, "02"); v != "0123456789" {
		t.Fatalf("account = %q, want 0123456789", v)
	}
	if v := parseTLV(t, parseTLV(t, got, "62"), "08"); v != "sub2_20260814aB3kX9mQ" {
		t.Fatalf("content = %q", v)
	}

	// CRC tag must cover payload + "6304" and match the trailing 4 hex chars.
	idx := strings.LastIndex(got, "6304")
	if idx < 0 {
		t.Fatal("missing CRC tag")
	}
	if crc := crc16CCITTFalse(got[:idx+4]); fmt.Sprintf("%04X", crc) != got[idx+4:] {
		t.Fatalf("CRC = %s, want %04X", got[idx+4:], crc)
	}
}
```

(imports: `"fmt"`, `"strings"`, `"testing"`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/payment/provider/ -run 'TestCRC16|TestBuildVietQRPayload' -v`
Expected: FAIL — `undefined: crc16CCITTFalse` / `buildVietQRPayload`.

- [ ] **Step 3: Implement**

`backend/internal/payment/provider/vietqr.go`:

```go
package provider

import (
	"fmt"
	"strconv"
	"strings"
)

// buildVietQRPayload builds an EMVCo merchant-presented QR string following
// the VietQR/NAPAS standard: banking apps scan it and prefill the beneficiary
// account, amount and transfer content.
func buildVietQRPayload(bin, accountNumber string, amountVND int64, content string) string {
	merchantAccount := tlv("00", "A000000727") + tlv("01", bin) + tlv("02", accountNumber)
	payload := tlv("00", "01") + // Payload Format Indicator
		tlv("01", "12") + // Point of Initiation: dynamic (amount included)
		tlv("38", merchantAccount) + // Merchant Account Information (NAPAS)
		tlv("53", "704") + // Transaction Currency: VND
		tlv("54", strconv.FormatInt(amountVND, 10)) + // Transaction Amount
		tlv("58", "VN") + // Country Code
		tlv("62", tlv("08", content)) // Additional Data: purpose (transfer content)
	return payload + "6304" + strings.ToUpper(fmt.Sprintf("%04X", crc16CCITTFalse(payload+"6304")))
}

// tlv encodes one EMVCo TLV field with a two-digit length prefix.
func tlv(tag, value string) string {
	return tag + fmt.Sprintf("%02d", len(value)) + value
}

// crc16CCITTFalse computes CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF, no
// reflection, no final XOR) — the checksum mandated by EMVCo QR (tag 63).
func crc16CCITTFalse(data string) uint16 {
	crc := uint16(0xFFFF)
	for i := 0; i < len(data); i++ {
		crc ^= uint16(data[i]) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/payment/provider/ -run 'TestCRC16|TestBuildVietQRPayload' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/payment/provider/vietqr.go backend/internal/payment/provider/vietqr_test.go
git commit -m "feat(payment): add VietQR EMV payload builder"
```

---

### Task 3: SePay provider — config, CreatePayment, Refund

**Files:**
- Create: `backend/internal/payment/provider/sepay.go`
- Test: `backend/internal/payment/provider/sepay_test.go`

**Interfaces:**
- Consumes: `buildVietQRPayload` (Task 2), `payment.TypeSePay` (Task 1).
- Produces: `NewSePay(instanceID string, config map[string]string) (*SePay, error)`; methods on `*SePay`: `Name() string`, `ProviderKey() string`, `SupportedTypes() []payment.PaymentType`, `MerchantIdentityMetadata() map[string]string`, `CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error)`, `Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error)`, `QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error)` (Task 5), `VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error)` (Task 4); helper `sepayCodeMatchesOrder(code, outTradeNo string) bool`.

- [ ] **Step 1: Write failing tests**

`backend/internal/payment/provider/sepay_test.go`:

```go
package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func sepayTestConfig() map[string]string {
	return map[string]string{
		"apiToken":          "tok_64_chars_00000000000000000000000000000000000000000000000000000000",
		"bankAccountNumber": "0123456789",
		"bankBin":           "970422",
		"webhookSecret":     "secret",
	}
}

func TestNewSePayConfigValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{"missing apiToken", func(c map[string]string) { delete(c, "apiToken") }, "apiToken"},
		{"missing bankAccountNumber", func(c map[string]string) { delete(c, "bankAccountNumber") }, "bankAccountNumber"},
		{"missing bankBin", func(c map[string]string) { delete(c, "bankBin") }, "bankBin"},
		{"no webhook auth", func(c map[string]string) { delete(c, "webhookSecret") }, "webhook"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := sepayTestConfig()
			tc.mutate(cfg)
			_, err := NewSePay("1", cfg)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestNewSePayApiKeyOnlyConfigIsValid(t *testing.T) {
	cfg := sepayTestConfig()
	delete(cfg, "webhookSecret")
	cfg["webhookApiKey"] = "key"
	if _, err := NewSePay("1", cfg); err != nil {
		t.Fatalf("apikey-only config should be valid: %v", err)
	}
}

func TestSePayCreatePayment(t *testing.T) {
	p, err := NewSePay("1", sepayTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_20260814aB3kX9mQ",
		Amount:  "50000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Currency != "VND" {
		t.Fatalf("currency = %q, want VND", resp.Currency)
	}
	if resp.QRCode == "" || !strings.Contains(resp.QRCode, "6304") {
		t.Fatalf("QRCode = %q, want EMV payload", resp.QRCode)
	}
	// QR amount tag must carry the integer VND amount.
	if !strings.Contains(resp.QRCode, tlv("54", "50000")) {
		t.Fatalf("QR payload missing amount TLV: %s", resp.QRCode)
	}
	if !strings.Contains(resp.QRCode, "sub2_20260814aB3kX9mQ") {
		t.Fatalf("QR payload missing transfer content: %s", resp.QRCode)
	}
}

func TestSePayCreatePaymentRejectsNonIntegerAmount(t *testing.T) {
	p, _ := NewSePay("1", sepayTestConfig())
	if _, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "x", Amount: "50.5"}); err == nil {
		t.Fatal("expected error for fractional VND amount")
	}
	if _, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "x", Amount: "0"}); err == nil {
		t.Fatal("expected error for zero amount")
	}
}

func TestSePayRefundUnsupported(t *testing.T) {
	p, _ := NewSePay("1", sepayTestConfig())
	_, err := p.Refund(context.Background(), payment.RefundRequest{})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("err = %v, want not supported", err)
	}
}

func TestSepayCodeMatchesOrder(t *testing.T) {
	const out = "sub2_20260814aB3kX9mQ"
	cases := []struct{ code string; want bool }{
		{"sub2_20260814aB3kX9mQ", true},
		{"SUB2_20260814AB3KX9MQ", true}, // bank uppercased content
		{"20260814aB3kX9mQ", true},      // SePay stripped the prefix
		{"20260814AB3KX9MQ", true},      // stripped + uppercased
		{"sub2_19990101zzzzzzzz", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := sepayCodeMatchesOrder(tc.code, out); got != tc.want {
			t.Errorf("sepayCodeMatchesOrder(%q) = %v, want %v", tc.code, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/payment/provider/ -run 'TestNewSePay|TestSePay|TestSepayCodeMatches' -v`
Expected: FAIL — `undefined: NewSePay`.

- [ ] **Step 3: Implement**

Create `backend/internal/payment/provider/sepay.go` with everything except `VerifyNotification`/`QueryOrder` (added in Tasks 4-5). No stubs are needed: `NewSePay` returns the concrete `*SePay` type, and the provider is only registered as a `payment.Provider` in Task 6, after the remaining methods exist.

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/payment/provider/ -run 'TestNewSePay|TestSePay|TestSepayCodeMatches' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/payment/provider/sepay.go backend/internal/payment/provider/sepay_test.go
git commit -m "feat(payment): add SePay provider config, VietQR CreatePayment, refund stub"
```

---

### Task 4: SePay VerifyNotification (HMAC-SHA256 / API key)

**Files:**
- Modify: `backend/internal/payment/provider/sepay.go` (add VerifyNotification)
- Test: `backend/internal/payment/provider/sepay_test.go` (append)

**Interfaces:**
- Consumes: `*SePay` from Task 3.
- Produces: `VerifyNotification(ctx, rawBody string, headers map[string]string) (*payment.PaymentNotification, error)` — headers keys are lowercase (the webhook handler lowercases them). Returns `(nil, nil)` for `transferType == "out"`.

- [ ] **Step 1: Write failing tests**

Append to `backend/internal/payment/provider/sepay_test.go`:

```go
func sepayNotifyBody(code string, amount int64) string {
	if code == "" {
		return `{"id":92704,"gateway":"Vietcombank","transactionDate":"2024-07-02 11:08:33","accountNumber":"1017588888","subAccount":"","code":null,"content":"chuyen tien","transferType":"in","transferAmount":` + strconv.FormatInt(amount, 10) + `,"accumulated":0,"referenceCode":"FT24012345678"}`
	}
	return `{"id":92704,"gateway":"Vietcombank","transactionDate":"2024-07-02 11:08:33","accountNumber":"1017588888","subAccount":"","code":"` + code + `","content":"` + code + ` chuyen tien","transferType":"in","transferAmount":` + strconv.FormatInt(amount, 10) + `,"accumulated":0,"referenceCode":"FT24012345678"}`
}

func sepaySignedHeaders(body string, secret string, ts int64) map[string]string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "." + body))
	return map[string]string{
		"x-sepay-signature": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"x-sepay-timestamp": strconv.FormatInt(ts, 10),
	}
}

func TestSePayVerifyNotificationHMAC(t *testing.T) {
	p, _ := NewSePay("1", sepayTestConfig())
	body := sepayNotifyBody("sub2_20260814aB3kX9mQ", 50000)
	now := time.Now().Unix()

	n, err := p.VerifyNotification(context.Background(), body, sepaySignedHeaders(body, "secret", now))
	if err != nil {
		t.Fatal(err)
	}
	if n.OrderID != "sub2_20260814aB3kX9mQ" || n.Amount != 50000 || n.TradeNo != "FT24012345678" {
		t.Fatalf("notification = %+v", n)
	}
	if n.Status != payment.NotificationStatusSuccess {
		t.Fatalf("status = %q", n.Status)
	}
	if n.Metadata["accountNumber"] != "1017588888" || n.Metadata["gateway"] != "Vietcombank" {
		t.Fatalf("metadata = %v", n.Metadata)
	}
}

func TestSePayVerifyNotificationHMACFailures(t *testing.T) {
	p, _ := NewSePay("1", sepayTestConfig())
	body := sepayNotifyBody("sub2_20260814aB3kX9mQ", 50000)
	now := time.Now().Unix()

	if _, err := p.VerifyNotification(context.Background(), body, sepaySignedHeaders(body, "wrong", now)); err == nil {
		t.Fatal("expected signature mismatch error")
	}
	if _, err := p.VerifyNotification(context.Background(), body, map[string]string{"x-sepay-timestamp": strconv.FormatInt(now, 10)}); err == nil {
		t.Fatal("expected missing signature error")
	}
	if _, err := p.VerifyNotification(context.Background(), body, sepaySignedHeaders(body, "secret", now-3600)); err == nil {
		t.Fatal("expected timestamp skew error")
	}
	// Signed over different body.
	if _, err := p.VerifyNotification(context.Background(), sepayNotifyBody("other", 1), sepaySignedHeaders(body, "secret", now)); err == nil {
		t.Fatal("expected signature mismatch for altered body")
	}
}

func TestSePayVerifyNotificationApiKey(t *testing.T) {
	cfg := sepayTestConfig()
	delete(cfg, "webhookSecret")
	cfg["webhookApiKey"] = "key123"
	p, _ := NewSePay("1", cfg)
	body := sepayNotifyBody("sub2_20260814aB3kX9mQ", 50000)

	if _, err := p.VerifyNotification(context.Background(), body, map[string]string{"authorization": "Apikey key123"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.VerifyNotification(context.Background(), body, map[string]string{"authorization": "Apikey nope"}); err == nil {
		t.Fatal("expected api key mismatch")
	}
	if _, err := p.VerifyNotification(context.Background(), body, nil); err == nil {
		t.Fatal("expected missing header error")
	}
}

func TestSePayVerifyNotificationOutAndNullCode(t *testing.T) {
	p, _ := NewSePay("1", sepayTestConfig())
	now := time.Now().Unix()

	outBody := `{"id":1,"gateway":"VCB","transactionDate":"2024-07-02 11:08:33","accountNumber":"1","subAccount":"","code":"sub2_20260814aB3kX9mQ","content":"x","transferType":"out","transferAmount":100,"referenceCode":"FT1"}`
	n, err := p.VerifyNotification(context.Background(), outBody, sepaySignedHeaders(outBody, "secret", now))
	if err != nil || n != nil {
		t.Fatalf("out transaction: n=%v err=%v, want nil/nil", n, err)
	}

	nullCode := sepayNotifyBody("", 50000)
	if _, err := p.VerifyNotification(context.Background(), nullCode, sepaySignedHeaders(nullCode, "secret", now)); err == nil {
		t.Fatal("expected missing payment code error")
	}
}
```

Add these imports to the test file: `"crypto/hmac"`, `"crypto/sha256"`, `"encoding/hex"`, `"strconv"`, `"time"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/payment/provider/ -run 'TestSePayVerifyNotification' -v`
Expected: FAIL — `p.VerifyNotification undefined`.

- [ ] **Step 3: Implement**

Append to `backend/internal/payment/provider/sepay.go` (add imports `"crypto/hmac"`, `"crypto/sha256"`, `"encoding/hex"`, `"encoding/json"`):

```go
// sepayWebhookPayload mirrors the SePay transaction webhook JSON body.
type sepayWebhookPayload struct {
	ID              int64  `json:"id"`
	Gateway         string `json:"gateway"`
	TransactionDate string `json:"transactionDate"`
	AccountNumber   string `json:"accountNumber"`
	SubAccount      string `json:"subAccount"`
	Code            *string `json:"code"`
	Content         string `json:"content"`
	TransferType    string `json:"transferType"`
	Description     string `json:"description"`
	TransferAmount  int64  `json:"transferAmount"`
	Accumulated     int64  `json:"accumulated"`
	ReferenceCode   string `json:"referenceCode"`
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/payment/provider/ -run 'TestSePayVerifyNotification' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/payment/provider/sepay.go backend/internal/payment/provider/sepay_test.go
git commit -m "feat(payment): SePay webhook verification (HMAC-SHA256 / API key)"
```

---

### Task 5: SePay QueryOrder via API v2

**Files:**
- Modify: `backend/internal/payment/provider/sepay.go` (add QueryOrder)
- Test: `backend/internal/payment/provider/sepay_test.go` (append)

**Interfaces:**
- Consumes: `*SePay` from Task 3, `sepayCodeMatchesOrder`.
- Produces: `QueryOrder(ctx, tradeNo string) (*payment.QueryOrderResponse, error)` — `tradeNo` is the order's out_trade_no (service convention, see Task 8).

- [ ] **Step 1: Write failing tests**

Append to `backend/internal/payment/provider/sepay_test.go` (add `"net/http"`, `"net/http/httptest"`, `"net/url"` imports as needed):

```go
func sepayQueryServer(t *testing.T, queries *[]url.Values, respond func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*queries = append(*queries, r.URL.Query())
		respond(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSePayQueryOrderPaid(t *testing.T) {
	var queries []url.Values
	srv := sepayQueryServer(t, &queries, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+sepayTestConfig()["apiToken"] {
			t.Errorf("auth header = %q", got)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":[{"id":"a1b2","transaction_date":"2026-08-14 09:30:00","transfer_type":"in","amount_in":50000,"transaction_content":"sub2_20260814aB3kX9mQ chuyen tien","reference_number":"FT26069ABC","code":"SUB2_20260814AB3KX9MQ"}]}`))
	})
	cfg := sepayTestConfig()
	cfg["apiBase"] = srv.URL
	p, _ := NewSePay("1", cfg)

	resp, err := p.QueryOrder(context.Background(), "sub2_20260814aB3kX9mQ")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != payment.ProviderStatusPaid || resp.Amount != 50000 || resp.TradeNo != "FT26069ABC" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.PaidAt != "2026-08-14 09:30:00" {
		t.Fatalf("paidAt = %q", resp.PaidAt)
	}
	if len(queries) != 1 || queries[0].Get("q") != "sub2_20260814aB3kX9mQ" || queries[0].Get("transfer_type") != "in" {
		t.Fatalf("queries = %v", queries)
	}
}

func TestSePayQueryOrderPending(t *testing.T) {
	var queries []url.Values
	srv := sepayQueryServer(t, &queries, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":[{"id":"c3","transaction_date":"2026-08-14 09:30:00","transfer_type":"in","amount_in":1,"code":"SUB2_19990101ZZZZZZZZ"}]}`))
	})
	cfg := sepayTestConfig()
	cfg["apiBase"] = srv.URL
	p, _ := NewSePay("1", cfg)

	resp, err := p.QueryOrder(context.Background(), "sub2_20260814aB3kX9mQ")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != payment.ProviderStatusPending {
		t.Fatalf("status = %q, want pending (code does not match order)", resp.Status)
	}
}

func TestSePayQueryOrderHTTPErrors(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		wantErr string
	}{
		{http.StatusUnauthorized, `{"error":{"code":"unauthorized"}}`, "unauthorized"},
		{http.StatusTooManyRequests, `{"error":{"code":"rate_limited"}}`, "rate"},
		{http.StatusInternalServerError, `boom`, "HTTP 500"},
	} {
		var queries []url.Values
		srv := sepayQueryServer(t, &queries, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		cfg := sepayTestConfig()
		cfg["apiBase"] = srv.URL
		p, _ := NewSePay("1", cfg)
		_, err := p.QueryOrder(context.Background(), "sub2_20260814aB3kX9mQ")
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("status %d: err = %v, want containing %q", tc.status, err, tc.wantErr)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/payment/provider/ -run 'TestSePayQueryOrder' -v`
Expected: FAIL — `p.QueryOrder undefined`.

- [ ] **Step 3: Implement**

Append to `backend/internal/payment/provider/sepay.go` (add imports `"io"`, `"net/url"`):

```go
// sepayTransaction mirrors one element of GET /v2/transactions data.
type sepayTransaction struct {
	ID                string `json:"id"`
	TransactionDate   string `json:"transaction_date"`
	TransferType      string `json:"transfer_type"`
	AmountIn          int64  `json:"amount_in"`
	TransactionContent string `json:"transaction_content"`
	ReferenceNumber   string `json:"reference_number"`
	Code              string `json:"code"`
}

// QueryOrder looks up the order's transfer in SePay API v2. tradeNo carries
// the order's out_trade_no; the q= search covers the extracted payment code.
func (s *SePay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	outTradeNo := strings.TrimSpace(tradeNo)
	if outTradeNo == "" {
		return nil, fmt.Errorf("sepay query: empty order reference")
	}
	endpoint := s.config["apiBase"] + "/v2/transactions?q=" + url.QueryEscape(outTradeNo) + "&transfer_type=in"
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
	for _, tx := range parsed.Data {
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
	return &payment.QueryOrderResponse{
		TradeNo:  outTradeNo,
		Status:   payment.ProviderStatusPending,
		Metadata: s.MerchantIdentityMetadata(),
	}, nil
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/payment/provider/ -run 'TestSePayQueryOrder' -v && go test ./internal/payment/provider/`
Expected: PASS (whole provider package green).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/payment/provider/sepay.go backend/internal/payment/provider/sepay_test.go
git commit -m "feat(payment): SePay QueryOrder via API v2 transactions"
```

---

### Task 6: Register SePay in the provider factory

**Files:**
- Modify: `backend/internal/payment/provider/factory.go:11-22`
- Test: `backend/internal/payment/provider/factory_test.go` (create or append)

**Interfaces:**
- Consumes: `NewSePay` (Task 3).
- Produces: `CreateProvider("sepay", ...)` returns a `*SePay`.

- [ ] **Step 1: Write failing test**

`backend/internal/payment/provider/factory_test.go` (create with `package provider` if missing):

```go
package provider

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestCreateProviderSePay(t *testing.T) {
	p, err := CreateProvider(payment.TypeSePay, "7", sepayTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	if p.ProviderKey() != payment.TypeSePay {
		t.Fatalf("provider key = %q", p.ProviderKey())
	}
	if _, err := CreateProvider(payment.TypeSePay, "7", map[string]string{}); err == nil {
		t.Fatal("expected config validation error from factory")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/payment/provider/ -run TestCreateProviderSePay -v`
Expected: FAIL — `unknown provider key: sepay`.

- [ ] **Step 3: Implement**

In `factory.go`, add to the switch in `CreateProvider` after the airwallex case:

```go
	case payment.TypeSePay:
		return NewSePay(instanceID, config)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/payment/provider/ -run TestCreateProviderSePay -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/payment/provider/factory.go backend/internal/payment/provider/factory_test.go
git commit -m "feat(payment): register SePay in provider factory"
```

---

### Task 7: Webhook route + handler for SePay

**Files:**
- Modify: `backend/internal/handler/payment_webhook_handler.go` (SepayNotify, extractOutTradeNo case, writeSuccessResponse case)
- Modify: `backend/internal/server/routes/payment.go:59-69` (route)
- Test: `backend/internal/handler/payment_webhook_handler_test.go` (append)

**Interfaces:**
- Consumes: `payment.TypeSePay`, `SePay.VerifyNotification` (Task 4).
- Produces: `POST /api/v1/payment/webhook/sepay`; success body `{"success":true}`.

- [ ] **Step 1: Write failing tests**

The existing `backend/internal/handler/payment_webhook_handler_test.go` (build tag `//go:build unit`) tests `writeSuccessResponse` and `extractOutTradeNo` directly with `gin.CreateTestContext`. Extend both tables.

Add a case to the `TestWriteSuccessResponse` table (after the airwallex case):

```go
		{
			name:            "sepay returns JSON success body",
			providerKey:     payment.TypeSePay,
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"success":true}`,
		},
```

(The table-driven test asserts `w.Code`, content type and `w.Body.String()`; the JSON body comparison works because gin renders `{"success":true}` with no spaces.)

Add two cases to the `TestExtractOutTradeNo` table:

```go
		{
			name:        "sepay json payload with code",
			providerKey: payment.TypeSePay,
			rawBody:     `{"code":"sub2_20260814aB3kX9mQ","transferType":"in","transferAmount":50000}`,
			want:        "sub2_20260814aB3kX9mQ",
		},
		{
			name:        "sepay json payload with null code",
			providerKey: payment.TypeSePay,
			rawBody:     `{"code":null,"transferType":"in","transferAmount":50000}`,
			want:        "",
		},
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -tags=unit ./internal/handler/ -run 'TestWriteSuccessResponse|TestExtractOutTradeNo' -v`
Expected: FAIL — the new table cases fail (sepay falls into `default` → plain-text `success`; extractOutTradeNo returns `""` for the JSON code body).

- [ ] **Step 3: Implement**

`backend/internal/handler/payment_webhook_handler.go` — add after `AirwallexWebhook`:

```go
// SepayNotify handles SePay transaction webhooks.
// POST /api/v1/payment/webhook/sepay
func (h *PaymentWebhookHandler) SepayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeSePay)
}
```

In `extractOutTradeNo`, add a case (JSON body, `code` may be null → empty):

```go
	case payment.TypeSePay:
		var payload struct {
			Code *string `json:"code"`
		}
		if err := json.Unmarshal([]byte(rawBody), &payload); err == nil && payload.Code != nil {
			return strings.TrimSpace(*payload.Code)
		}
```

In `writeSuccessResponse`, add before `default`:

```go
	case payment.TypeSePay:
		// SePay requires exactly {"success":true} with HTTP 200/201.
		c.JSON(http.StatusOK, gin.H{"success": true})
```

`backend/internal/server/routes/payment.go` — add to the webhook group:

```go
		webhook.POST("/sepay", webhookHandler.SepayNotify)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -tags=unit ./internal/handler/ -run 'TestWriteSuccessResponse|TestExtractOutTradeNo' -v && go test -tags=unit ./internal/handler/`
Expected: PASS (whole handler unit suite green). Also verify the route: `grep -n "webhook/sepay" backend/internal/server/routes/payment.go` shows the new POST line.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/payment_webhook_handler.go backend/internal/server/routes/payment.go backend/internal/handler/payment_webhook_handler_test.go
git commit -m "feat(payment): SePay webhook route with required JSON success body"
```

---

### Task 8: Lenient sepay code → order resolution (service layer)

**Files:**
- Modify: `backend/internal/service/payment_webhook_provider.go:32-35` (GetWebhookProviders prologue)
- Modify: `backend/internal/service/payment_fulfillment.go:47-58` (HandlePaymentNotification NotFound branch)
- Modify: `backend/internal/service/payment_order_lifecycle.go:249-253` (paymentOrderQueryReference case list)
- Test: `backend/internal/service/payment_sepay_resolution_test.go` (create)

**Interfaces:**
- Consumes: `payment.TypeSePay` (Task 1); ent predicate `paymentorder.OutTradeNoEqualFold` (exists, `backend/ent/paymentorder/where.go:714`).
- Produces: `(*PaymentService).resolveSepayOutTradeNo(ctx, code) string`; `(*PaymentService).resolveSepayNotificationOrderID(ctx, providerKey, code) (int64, bool)`; `paymentOrderQueryReference` returns `order.OutTradeNo` for sepay.

- [ ] **Step 1: Write failing tests**

Create `backend/internal/service/payment_sepay_resolution_test.go`, following the sqlite/enttest pattern of `payment_fulfillment_order_not_found_test.go` and the order-creation pattern of `payment_fulfillment_test.go` (`createPaymentFulfillmentSubscriptionOrder`):

```go
//go:build unit

package service

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/dbent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/dialect"
	entsql "github.com/Wei-Shaw/sub2api/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/internal/payment"

	"github.com/stretchr/testify/require"
)

func newSepayResolutionTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:sepay_resolution_"+strconv.FormatInt(time.Now().UnixNano(), 10)+"?mode=memory&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createSepayTestOrder(t *testing.T, ctx context.Context, client *dbent.Client, outTradeNo string) *dbent.PaymentOrder {
	t.Helper()
	user, err := client.User.Create().
		SetEmail("sepay-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com").
		SetPasswordHash("hash").
		SetUsername("sepay-user").
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(50000).
		SetPayAmount(50000).
		SetFeeRate(0).
		SetRechargeCode("PAY-SEPAY-" + strconv.FormatInt(time.Now().UnixNano(), 10)).
		SetOutTradeNo(outTradeNo).
		SetPaymentType(payment.TypeSePay).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetSrcURL("/api/v1/payment/orders").
		Save(ctx)
	require.NoError(t, err)
	return order
}

// TestResolveSepayOutTradeNo verifies bank-side mutations of the transfer code:
// exact, uppercased, prefix-stripped and prefix-stripped+uppercased variants
// all resolve to the canonical out_trade_no.
func TestResolveSepayOutTradeNo(t *testing.T) {
	ctx := context.Background()
	client := newSepayResolutionTestClient(t)
	svc := &PaymentService{entClient: client, providersLoaded: true}

	const canonical = "sub2_20260814aB3kX9mQ"
	order := createSepayTestOrder(t, ctx, client, canonical)

	for _, code := range []string{
		"sub2_20260814aB3kX9mQ",
		"SUB2_20260814AB3KX9MQ",
		"20260814aB3kX9mQ",
		"20260814AB3KX9MQ",
	} {
		require.Equal(t, canonical, svc.resolveSepayOutTradeNo(ctx, code), "code %q", code)
	}
	require.Equal(t, "sub2_19990101zzzzzzzz",
		svc.resolveSepayOutTradeNo(ctx, "sub2_19990101zzzzzzzz"), "unknown code round-trips unchanged")
	require.Equal(t, "", svc.resolveSepayOutTradeNo(ctx, "  "))

	oid, ok := svc.resolveSepayNotificationOrderID(ctx, payment.TypeSePay, "20260814AB3KX9MQ")
	require.True(t, ok)
	require.Equal(t, order.ID, oid)

	_, ok = svc.resolveSepayNotificationOrderID(ctx, payment.TypeAlipay, canonical)
	require.False(t, ok, "non-sepay provider must not use sepay resolution")
}
```

Also add the pure-function query-reference test to the same file:

```go
func TestPaymentOrderQueryReferenceSePay(t *testing.T) {
	order := &dbent.PaymentOrder{OutTradeNo: "sub2_20260814aB3kX9mQ", PaymentType: payment.TypeSePay}
	require.Equal(t, "sub2_20260814aB3kX9mQ", paymentOrderQueryReference(order, nil),
		"sepay must query by out_trade_no (no upstream tradeNo exists while pending)")
}
```

Note: if `SetSrcURL`/`SetSrcHost` are optional in the schema the calls can stay (they are plain setters); the required set mirrors `createPaymentFulfillmentSubscriptionOrder` in `payment_fulfillment_test.go:855-874`. If a required field is still missing, `go test` will name it in the compile error — add the corresponding `Set<Field>` with a trivial value.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test -tags=unit ./internal/service/ -run 'TestResolveSepayOutTradeNo|TestPaymentOrderQueryReferenceSePay' -v`
Expected: FAIL — `undefined: svc.resolveSepayOutTradeNo` / sepay takes the default branch.

- [ ] **Step 3: Implement**

`payment_webhook_provider.go` — add at the top of `GetWebhookProviders` (before `if outTradeNo != ""`):

```go
	if strings.TrimSpace(providerKey) == payment.TypeSePay {
		outTradeNo = s.resolveSepayOutTradeNo(ctx, outTradeNo)
	}
```

And add the helper (same file):

```go
// resolveSepayOutTradeNo maps a SePay webhook code back to the canonical
// out_trade_no. Banks may uppercase the transfer content and SePay's payment
// code extraction may drop the configured prefix, so resolution falls back to
// case-insensitive lookups (raw code, then code with the sub2_ prefix).
func (s *PaymentService) resolveSepayOutTradeNo(ctx context.Context, code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	for _, cand := range []string{code, orderIDPrefix + code} {
		order, err := s.entClient.PaymentOrder.Query().
			Where(paymentorder.OutTradeNoEqualFold(cand)).Only(ctx)
		if err == nil && order != nil {
			return order.OutTradeNo
		}
	}
	return code
}
```

`payment_fulfillment.go` — in `HandlePaymentNotification`, extend the NotFound branch (after the legacy `parseLegacyPaymentOrderID` attempt, before returning `ErrOrderNotFound`):

```go
		if oid, ok := s.resolveSepayNotificationOrderID(ctx, pk, n.OrderID); ok {
			return s.confirmPayment(ctx, oid, n.TradeNo, n.Amount, pk, n.Metadata)
		}
```

And add the helper (same file):

```go
// resolveSepayNotificationOrderID resolves lenient SePay codes (uppercased or
// prefix-stripped by banks) to the internal order ID.
func (s *PaymentService) resolveSepayNotificationOrderID(ctx context.Context, providerKey, code string) (int64, bool) {
	if strings.TrimSpace(providerKey) != payment.TypeSePay {
		return 0, false
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return 0, false
	}
	for _, cand := range []string{code, orderIDPrefix + code} {
		order, err := s.entClient.PaymentOrder.Query().
			Where(paymentorder.OutTradeNoEqualFold(cand)).Only(ctx)
		if err == nil && order != nil {
			return order.ID, true
		}
	}
	return 0, false
}
```

`payment_order_lifecycle.go` — in `paymentOrderQueryReference`, add sepay to the out_trade_no case:

```go
	case payment.TypeAlipay, payment.TypeEasyPay, payment.TypeWxpay, payment.TypeSePay:
		return strings.TrimSpace(order.OutTradeNo)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test -tags=unit ./internal/service/ -run 'TestResolveSepayOutTradeNo|TestPaymentOrderQueryReferenceSePay' -v && go test ./internal/service/`
Expected: PASS (whole service package green).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/payment_webhook_provider.go backend/internal/service/payment_fulfillment.go backend/internal/service/payment_order_lifecycle.go backend/internal/service/payment_sepay_resolution_test.go
git commit -m "feat(payment): lenient sepay code resolution for webhooks and queries"
```

---

### Task 9: Provider registration keys, refund block, sensitive + protected config fields

**Files:**
- Modify: `backend/internal/service/payment_config_providers.go` (`validProviderKeys` map at line 180-182; both sensitive/protected maps at lines 118-131; refund guard in `CreateProviderInstance` ~line 207 and `UpdateProviderInstance` ~line 423)
- Test: `backend/internal/service/payment_config_providers_test.go` (append)

**Interfaces:**
- Consumes: `payment.TypeSePay`.
- Produces: admin can create sepay provider instances (`validProviderKeys`); enabling `refund_enabled` on sepay returns `VALIDATION_ERROR`; sepay secrets are masked by the admin GET API and preserved on empty re-submit; `bankAccountNumber`/`bankBin` cannot change while orders are in progress; helper `providerSupportsRefund(providerKey string) bool`.

- [ ] **Step 1: Write failing test**

Append to `backend/internal/service/payment_config_providers_test.go` (plain unit test, no DB):

```go
func TestSepayProviderRegistrationAndRefundBlock(t *testing.T) {
	if !validProviderKeys[payment.TypeSePay] {
		t.Error("sepay must be a valid provider key")
	}
	if err := validateProviderRequest(payment.TypeSePay, "SePay VN", "sepay"); err != nil {
		t.Fatalf("sepay provider request should validate: %v", err)
	}
	if providerSupportsRefund(payment.TypeSePay) {
		t.Error("sepay has no refund API and must not report refund support")
	}
	if !providerSupportsRefund(payment.TypeStripe) {
		t.Error("stripe refund support regression")
	}
	if err := validateProviderRefundSupport(payment.TypeSePay, true); err == nil {
		t.Error("enabling refund on sepay must be rejected")
	}
	if err := validateProviderRefundSupport(payment.TypeSePay, false); err != nil {
		t.Errorf("refund disabled should always be accepted: %v", err)
	}
	if err := validateProviderRefundSupport(payment.TypeStripe, true); err != nil {
		t.Errorf("stripe refund enabled should be accepted: %v", err)
	}
}

func TestSepaySensitiveConfigFields(t *testing.T) {
	for _, field := range []string{"apiToken", "webhookSecret", "webhookApiKey"} {
		if !isSensitiveProviderConfigField(payment.TypeSePay, field) {
			t.Errorf("%s should be sensitive for sepay", field)
		}
	}
	for _, field := range []string{"bankAccountNumber", "bankBin", "accountName", "apiBase"} {
		if isSensitiveProviderConfigField(payment.TypeSePay, field) {
			t.Errorf("%s should not be sensitive for sepay", field)
		}
	}
	if !hasPendingOrderProtectedConfigChange(payment.TypeSePay,
		map[string]string{"bankAccountNumber": "1"},
		map[string]string{"bankAccountNumber": "2"}) {
		t.Error("bankAccountNumber change must be blocked with pending orders")
	}
	if hasPendingOrderProtectedConfigChange(payment.TypeSePay,
		map[string]string{"accountName": "A"},
		map[string]string{"accountName": "B"}) {
		t.Error("accountName change must be allowed with pending orders")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -tags=unit ./internal/service/ -run 'TestSepayProviderRegistration|TestSepaySensitiveConfigFields' -v`
Expected: FAIL — sepay absent from `validProviderKeys`/maps, helpers undefined.

- [ ] **Step 3: Implement**

In `payment_config_providers.go`:

1. `validProviderKeys` (line 181):

```go
	payment.TypeEasyPay: true, payment.TypeAlipay: true, payment.TypeWxpay: true, payment.TypeStripe: true, payment.TypeAirwallex: true, payment.TypeSePay: true,
```

2. Add refund-support helpers next to `validProviderKeys`:

```go
// refundCapableProviders lists provider keys whose upstream API supports
// refunds. SePay monitors bank transfers and has no refund API.
var refundCapableProviders = map[string]bool{
	payment.TypeEasyPay: true, payment.TypeAlipay: true, payment.TypeWxpay: true, payment.TypeStripe: true, payment.TypeAirwallex: true,
}

func providerSupportsRefund(providerKey string) bool {
	return refundCapableProviders[providerKey]
}

// validateProviderRefundSupport rejects enabling refunds on providers whose
// upstream has no refund API (currently sepay only).
func validateProviderRefundSupport(providerKey string, refundEnabled bool) error {
	if refundEnabled && !providerSupportsRefund(providerKey) {
		return infraerrors.BadRequest("VALIDATION_ERROR",
			fmt.Sprintf("provider %s does not support refunds", providerKey))
	}
	return nil
}
```

3. Call the guard in `CreateProviderInstance` before the ent `Create()` (line ~206):

```go
	if err := validateProviderRefundSupport(req.ProviderKey, req.RefundEnabled); err != nil {
		return nil, err
	}
```

4. And in `UpdateProviderInstance` inside `if req.RefundEnabled != nil {` (line ~423), before `u.SetRefundEnabled(*req.RefundEnabled)`:

```go
		if err := validateProviderRefundSupport(inst.ProviderKey, *req.RefundEnabled); err != nil {
			return nil, err
		}
```

(`inst` is the loaded instance variable already in scope in that function — confirm its actual name by reading the surrounding code.)

5. `providerSensitiveConfigFields`:

```go
	payment.TypeSePay: {"apitoken": {}, "webhooksecret": {}, "webhookapikey": {}},
```

6. `providerPendingOrderProtectedConfigFields`:

```go
	payment.TypeSePay: {"apitoken": {}, "webhooksecret": {}, "webhookapikey": {}, "bankaccountnumber": {}, "bankbin": {}},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -tags=unit ./internal/service/ -run 'TestSepayProviderRegistration|TestSepaySensitiveConfigFields' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/payment_config_providers.go backend/internal/service/payment_config_providers_test.go
git commit -m "feat(payment): sepay provider registration, refund block, config field protection"
```

---

### Task 10: `SUBSCRIPTION_USD_TO_VND_RATE` backend plumbing

**Files:**
- Modify: `backend/internal/service/payment_config_service.go` (const ~line 30, struct ~line 63, request ~line 96, keys slice ~line 222, parse ~line 251, validate ~line 330, save ~line 374)
- Modify: `backend/internal/service/payment_amounts.go` (normalize helper)
- Modify: `backend/internal/service/payment_order.go` (guard ~line 68; signature/callers at lines 72, 88, 646-666)
- Modify: `backend/internal/handler/payment_handler.go` (checkout-info ~lines 150, 167)
- Modify: `backend/internal/handler/dto/settings.go` (~line 275)
- Modify: `backend/internal/handler/admin/setting_handler.go` (~line 358)
- Modify: `backend/internal/handler/admin/setting_handler_update.go` (~lines 310, 2050, 2326, 2397)
- Test: `backend/internal/service/payment_order_sepay_vnd_test.go` (create)
- Test: existing tests calling `calculateCreateOrderPayAmountForOrderType` / `calculateSubscriptionGatewayBaseAmount` (update call sites — find with `grep -rn "calculateCreateOrderPayAmountForOrderType\|calculateSubscriptionGatewayBaseAmount" backend/internal --include="*_test.go"`)

**Interfaces:**
- Consumes: `payment.CurrencyVND` (Task 1), `PaymentConfig`.
- Produces: `PaymentConfig.SubscriptionUSDToVNDRate float64` (JSON `subscription_usd_to_vnd_rate`); `calculateCreateOrderPayAmountForOrderType(limitAmount, feeRate float64, currency, orderType string, cfg *PaymentConfig)`; `calculateSubscriptionGatewayBaseAmount(amount float64, cfg *PaymentConfig, currency string)`; checkout/admin DTO field `subscription_usd_to_vnd_rate` / `payment_subscription_usd_to_vnd_rate`; error code `SUBSCRIPTION_VND_RATE_REQUIRED`; validation error `INVALID_SUBSCRIPTION_USD_TO_VND_RATE`.

- [ ] **Step 1: Write failing tests**

`backend/internal/service/payment_order_sepay_vnd_test.go`:

```go
package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestCalculateSubscriptionGatewayBaseAmountVND(t *testing.T) {
	cfg := &PaymentConfig{SubscriptionUSDToVNDRate: 25000}
	cases := []struct {
		name     string
		cfg      *PaymentConfig
		currency string
		amount   float64
		want     float64
	}{
		{"vnd rate applies", cfg, payment.CurrencyVND, 9.9, 247500},
		{"vnd rate zero keeps price", &PaymentConfig{}, payment.CurrencyVND, 9.9, 9.9},
		{"cny unaffected", &PaymentConfig{SubscriptionUSDToCNYRate: 7.2}, payment.DefaultPaymentCurrency, 10, 72},
		{"other currency untouched", cfg, "USD", 9.9, 9.9},
		{"nil cfg safe", nil, payment.CurrencyVND, 9.9, 9.9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calculateSubscriptionGatewayBaseAmount(tc.amount, tc.cfg, tc.currency); got != tc.want {
				t.Fatalf("= %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreateOrderPayAmountForOrderTypeVND(t *testing.T) {
	cfg := &PaymentConfig{SubscriptionUSDToVNDRate: 25000}
	str, amt, err := calculateCreateOrderPayAmountForOrderType(9.9, 0, payment.CurrencyVND, payment.OrderTypeSubscription, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if str != "247500" || amt != 247500 {
		t.Fatalf("str=%q amt=%v, want 247500", str, amt)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/service/ -run 'TestCalculateSubscriptionGatewayBaseAmountVND|TestCreateOrderPayAmountForOrderTypeVND' -v`
Expected: FAIL — signatures undefined / VND not converted.

- [ ] **Step 3: Implement**

`payment_config_service.go` — five edits, each mirroring the adjacent CNY line (grep `SubscriptionUSDToCNYRate` to find them):

1. Const block:
```go
	// SettingSubscriptionUSDToVNDRate 是订阅 VND 换算汇率（1 USD = X VND）。
	// 0/未配置 = 关闭换算。SePay（VND）订阅必须配置该项，否则下单被拒绝。
	SettingSubscriptionUSDToVNDRate = "SUBSCRIPTION_USD_TO_VND_RATE"
```
2. `PaymentConfig` struct field after `SubscriptionUSDToCNYRate`:
```go
	SubscriptionUSDToVNDRate float64 `json:"subscription_usd_to_vnd_rate"`
```
3. `UpdatePaymentConfigRequest` field after `SubscriptionUSDToCNYRate`:
```go
	SubscriptionUSDToVNDRate *float64 `json:"subscription_usd_to_vnd_rate"`
```
4. Keys slice: append `SettingSubscriptionUSDToVNDRate` next to `SettingSubscriptionUSDToCNYRate`.
5. Parse:
```go
		SubscriptionUSDToVNDRate: normalizeSubscriptionUSDToVNDRate(pcParseFloat(vals[SettingSubscriptionUSDToVNDRate], 0)),
```
6. Validation (mirror the CNY `INVALID_SUBSCRIPTION_USD_TO_CNY_RATE` block):
```go
	if req.SubscriptionUSDToVNDRate != nil {
		v := *req.SubscriptionUSDToVNDRate
		if v < 0 {
			return infraerrors.BadRequest("INVALID_SUBSCRIPTION_USD_TO_VND_RATE", "subscription USD to VND rate must be 0 (disabled) or a positive number")
		}
	}
```
7. Save:
```go
	if req.SubscriptionUSDToVNDRate != nil {
		m[SettingSubscriptionUSDToVNDRate] = formatPositiveFloatExact(req.SubscriptionUSDToVNDRate)
	}
```

`payment_amounts.go` — add next to `normalizeSubscriptionUSDToCNYRate`:

```go
// normalizeSubscriptionUSDToVNDRate 将非法值归一为 0（换算关闭）。
func normalizeSubscriptionUSDToVNDRate(rate float64) float64 {
	return normalizeSubscriptionUSDToCNYRate(rate)
}
```

`payment_order.go` — three edits:

1. Guard in `CreateOrder` right after `ValidateMethodCurrencyConsistency` returns (before the first `calculateCreateOrderPayAmountForOrderType` call):
```go
	if req.OrderType == payment.OrderTypeSubscription && methodCurrency == payment.CurrencyVND {
		if normalizeSubscriptionUSDToVNDRate(cfg.SubscriptionUSDToVNDRate) <= 0 {
			return nil, infraerrors.BadRequest("SUBSCRIPTION_VND_RATE_REQUIRED",
				"subscription orders via VND methods require the USD to VND rate to be configured")
		}
	}
```
2. Change both call sites (lines ~72 and ~88) to pass `cfg` instead of `cfg.SubscriptionUSDToCNYRate`:
```go
	payAmountStr, payAmount, err := calculateCreateOrderPayAmountForOrderType(limitAmount, feeRate, methodCurrency, req.OrderType, cfg)
```
(and identically for the `selectedCurrency != methodCurrency` re-computation.)
3. Replace the two functions at the bottom of the file:
```go
func calculateCreateOrderPayAmountForOrderType(limitAmount, feeRate float64, currency, orderType string, cfg *PaymentConfig) (string, float64, error) {
	paymentAmount := limitAmount
	if orderType == payment.OrderTypeSubscription {
		paymentAmount = calculateSubscriptionGatewayBaseAmount(limitAmount, cfg, currency)
	}
	return calculateCreateOrderPayAmount(paymentAmount, feeRate, currency)
}

// calculateSubscriptionGatewayBaseAmount 计算订阅订单的网关扣款基数。
// 换算是显式 opt-in：CNY 通道按 SUBSCRIPTION_USD_TO_CNY_RATE、VND 通道按
// SUBSCRIPTION_USD_TO_VND_RATE（1 USD = rate），未配置时保持 price 直付。
func calculateSubscriptionGatewayBaseAmount(amount float64, cfg *PaymentConfig, currency string) float64 {
	if cfg == nil {
		return amount
	}
	var rate float64
	switch currency {
	case payment.DefaultPaymentCurrency:
		rate = normalizeSubscriptionUSDToCNYRate(cfg.SubscriptionUSDToCNYRate)
	case payment.CurrencyVND:
		rate = normalizeSubscriptionUSDToVNDRate(cfg.SubscriptionUSDToVNDRate)
	default:
		return amount
	}
	if rate <= 0 {
		return amount
	}
	return decimal.NewFromFloat(amount).
		Mul(decimal.NewFromFloat(rate)).
		Round(int32(payment.CurrencyMaxFractionDigits(currency))).
		InexactFloat64()
}
```

Then fix all callers, including tests: `cd backend && go build ./... && go vet ./internal/service/` and update every `calculateCreateOrderPayAmountForOrderType(..., someRate)` / `calculateSubscriptionGatewayBaseAmount(amount, rate, currency)` call found by:
`grep -rn "calculateCreateOrderPayAmountForOrderType\|calculateSubscriptionGatewayBaseAmount" backend/internal --include="*_test.go"`.
In tests, pass a `&PaymentConfig{SubscriptionUSDToCNYRate: oldRate}` (or the full cfg the test already has) in place of the old float argument.

`payment_handler.go` — checkout-info: add to the struct and the response construction next to the CNY field:
```go
	SubscriptionUSDToVNDRate float64 `json:"subscription_usd_to_vnd_rate"`
```
```go
		SubscriptionUSDToVNDRate:      cfg.SubscriptionUSDToVNDRate,
```

Admin plumbing (mirror every `PaymentSubscriptionUSDToCNYRate` occurrence — grep it in each file):
- `dto/settings.go`: `PaymentSubscriptionUSDToVNDRate float64 `json:"payment_subscription_usd_to_vnd_rate"``.
- `setting_handler.go`: `PaymentSubscriptionUSDToVNDRate: paymentCfg.SubscriptionUSDToVNDRate,`.
- `setting_handler_update.go`: request field `PaymentSubscriptionUSDToVNDRate *float64 `json:"payment_subscription_usd_to_vnd_rate"``; apply `SubscriptionUSDToVNDRate: req.PaymentSubscriptionUSDToVNDRate` in the update call; include in the response mapping and in the "payment settings changed" dirty-check condition (`req.PaymentSubscriptionUSDToVNDRate != nil || ...`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go build ./... && go test ./internal/service/ -run 'VND' -v && go test ./internal/service/ ./internal/handler/...`
Expected: PASS everywhere; no compile errors from signature change.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service backend/internal/handler
git commit -m "feat(payment): SUBSCRIPTION_USD_TO_VND_RATE for sepay subscription orders"
```

---

### Task 11: Frontend — provider config, method plumbing, i18n, admin options

**Files:**
- Modify: `frontend/src/components/payment/providerConfig.ts` (lines 39-45, 51, 107-113, 127-164)
- Modify: `frontend/src/components/payment/paymentFlow.ts` (lines 12-21)
- Modify: `frontend/src/i18n/locales/en/misc.ts` + `frontend/src/i18n/locales/zh/misc.ts` (methods block)
- Modify: `frontend/src/i18n/locales/en/admin/settings.ts` + `frontend/src/i18n/locales/zh/admin/settings.ts` (provider + field labels)
- Modify: `frontend/src/views/admin/SettingsView.vue` (~lines 12056-12060, 12112-12117)
- Test: `frontend/src/components/payment/__tests__/providerConfig.spec.ts` (append)

**Interfaces:**
- Consumes: backend route `POST /api/v1/payment/webhook/sepay` (Task 7).
- Produces: admin can create sepay provider instances; user checkout shows a "SePay" method; label resolution keys `payment.methods.sepay`, `admin.settings.payment.providerSepay`, `admin.settings.payment.field_apiToken`, `field_bankAccountNumber`, `field_bankBin`, `field_accountName`, `field_webhookApiKey`, `field_sepayApiBaseHint`.

- [ ] **Step 1: Write failing test**

Append to `frontend/src/components/payment/__tests__/providerConfig.spec.ts` (mirror the airwallex describe at the top of that file):

```ts
describe('PROVIDER_CONFIG_FIELDS.sepay', () => {
  const findField = (key: string) =>
    (PROVIDER_CONFIG_FIELDS.sepay || []).find(f => f.key === key)

  it('declares sepay supported types and method order', () => {
    expect(PROVIDER_SUPPORTED_TYPES.sepay).toEqual(['sepay'])
    expect(METHOD_ORDER).toContain('sepay')
    expect(WEBHOOK_PATHS.sepay).toBe('/api/v1/payment/webhook/sepay')
  })

  it('marks credentials as sensitive and bank details as required', () => {
    expect(findField('apiToken')?.sensitive).toBe(true)
    expect(findField('webhookSecret')?.sensitive).toBe(true)
    expect(findField('webhookApiKey')?.sensitive).toBe(true)
    expect(findField('bankAccountNumber')?.optional).toBeUndefined()
    expect(findField('bankBin')?.optional).toBeUndefined()
    expect(findField('accountName')?.optional).toBe(true)
    expect(findField('webhookApiKey')?.optional).toBe(true)
    expect(findField('apiBase')?.defaultValue).toBe('https://userapi.sepay.vn')
  })
})
```

Adjust the import at the top of the spec to also bring in `METHOD_ORDER` and `WEBHOOK_PATHS` if not already imported.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/components/payment/__tests__/providerConfig.spec.ts`
Expected: FAIL — sepay entries missing.

- [ ] **Step 3: Implement**

`providerConfig.ts`:

```ts
export const PROVIDER_SUPPORTED_TYPES: Record<string, string[]> = {
  easypay: ['alipay', 'wxpay'],
  alipay: ['alipay'],
  wxpay: ['wxpay'],
  stripe: ['card', 'alipay', 'wxpay', 'link'],
  airwallex: ['airwallex'],
  sepay: ['sepay'],
}
```

```ts
export const METHOD_ORDER = ['alipay', 'alipay_direct', 'wxpay', 'wxpay_direct', 'stripe', 'airwallex', 'sepay'] as const
```

```ts
export const WEBHOOK_PATHS: Record<string, string> = {
  easypay: '/api/v1/payment/webhook/easypay',
  alipay: '/api/v1/payment/webhook/alipay',
  wxpay: '/api/v1/payment/webhook/wxpay',
  stripe: '/api/v1/payment/webhook/stripe',
  airwallex: '/api/v1/payment/webhook/airwallex',
  sepay: '/api/v1/payment/webhook/sepay',
}
```

Add to `PROVIDER_CONFIG_FIELDS` (after airwallex):

```ts
  sepay: [
    { key: 'apiToken', label: '', sensitive: true },
    { key: 'apiBase', label: '', sensitive: false, defaultValue: 'https://userapi.sepay.vn', hintKey: 'admin.settings.payment.field_sepayApiBaseHint' },
    { key: 'bankAccountNumber', label: '', sensitive: false },
    { key: 'bankBin', label: '', sensitive: false },
    { key: 'accountName', label: '', sensitive: false, optional: true },
    { key: 'webhookSecret', label: '', sensitive: true },
    { key: 'webhookApiKey', label: '', sensitive: true, optional: true },
  ],
```

(`field_apiBase` and `field_webhookSecret` labels already exist — they are shared with airwallex/stripe. `PROVIDER_CALLBACK_PATHS` gets no sepay entry: SePay webhooks are configured in the SePay dashboard, not passed per-request.)

`paymentFlow.ts`:

```ts
const VISIBLE_METHOD_ALIASES = {
  alipay: 'alipay',
  alipay_direct: 'alipay',
  wxpay: 'wxpay',
  wxpay_direct: 'wxpay',
  stripe: 'stripe',
  airwallex: 'airwallex',
  sepay: 'sepay',
} as const

export type VisiblePaymentMethod = 'alipay' | 'wxpay' | 'stripe' | 'airwallex' | 'sepay'
```

i18n `en/misc.ts` methods block (next to `airwallex: 'Airwallex',`):

```ts
      sepay: 'SePay',
```

`zh/misc.ts` (next to `airwallex: 'Airwallex',`):

```ts
      sepay: 'SePay',
```

`en/admin/settings.ts` — next to the airwallex provider/field keys (locate `providerAirwallex` and the airwallex field hints):

```ts
    providerSepay: 'SePay',
    field_apiToken: 'API Token',
    field_bankAccountNumber: 'Bank Account Number',
    field_bankBin: 'Bank BIN',
    field_accountName: 'Account Holder Name',
    field_webhookApiKey: 'Webhook API Key',
    field_sepayApiBaseHint: 'Defaults to https://userapi.sepay.vn. Use https://userapi-sandbox.sepay.vn for sandbox testing.',
```

`zh/admin/settings.ts` — same keys:

```ts
    providerSepay: 'SePay',
    field_apiToken: 'API Token',
    field_bankAccountNumber: '银行账号',
    field_bankBin: '银行 BIN 码',
    field_accountName: '户名',
    field_webhookApiKey: 'Webhook API Key',
    field_sepayApiBaseHint: '默认 https://userapi.sepay.vn，沙箱环境使用 https://userapi-sandbox.sepay.vn。',
```

(Place them at the correct nesting level inside the `payment` section — grep `providerAirwallex` in each file for the exact spot. The `webhookSecret`/`apiBase` field labels already exist and are reused.)

`SettingsView.vue` — two one-line additions:

```ts
  { value: "sepay", label: t("payment.methods.sepay") },
```
in `allPaymentTypes`, and

```ts
  { value: "sepay", label: t("admin.settings.payment.providerSepay") },
```
in `providerKeyOptions`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/components/payment/__tests__/providerConfig.spec.ts src/components/payment/__tests__/paymentFlow.spec.ts src/components/payment/__tests__/PaymentMethodSelector.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/payment/providerConfig.ts frontend/src/components/payment/paymentFlow.ts frontend/src/i18n frontend/src/views/admin/SettingsView.vue frontend/src/components/payment/__tests__/providerConfig.spec.ts
git commit -m "feat(payment-frontend): sepay provider config, method plumbing and i18n"
```

---

### Task 12: Frontend — VND subscription rate display + admin input

**Files:**
- Modify: `frontend/src/types/payment.ts` (lines ~37 and ~72: both interfaces containing `subscription_usd_to_cny_rate`)
- Modify: `frontend/src/api/admin/settings.ts` (~lines 656, 966)
- Modify: `frontend/src/api/admin/payment.ts` (~lines 27, 47)
- Modify: `frontend/src/views/user/PaymentView.vue` (~lines 505, 523-527, 596-600)
- Modify: `frontend/src/views/admin/SettingsView.vue` (~lines 7790-7815 input markup, 9471 form default, 11270 submit payload)
- Modify: `frontend/src/i18n/locales/en/admin/settings.ts` + `zh/admin/settings.ts` (rate label keys)
- Test: `frontend/src/views/user/__tests__/PaymentView.spec.ts` (update defaults), `frontend/src/views/admin/__tests__/SettingsView.spec.ts` (update defaults)

**Interfaces:**
- Consumes: backend JSON fields `subscription_usd_to_vnd_rate` (checkout-info) and `payment_subscription_usd_to_vnd_rate` (admin settings) from Task 10.
- Produces: user-facing subscription prices in VND; admin input for the rate.

- [ ] **Step 1: Update tests first (defaults must include the new field)**

In `PaymentView.spec.ts` and `SettingsView.spec.ts`, every mock/fixture that contains `subscription_usd_to_cny_rate: 0` (grep both files) gains `subscription_usd_to_vnd_rate: 0` (PaymentView fixtures, e.g. line ~109) / `payment_subscription_usd_to_vnd_rate: 0` (SettingsView fixtures). Then add this VND twin next to the existing CNY conversion test (`mountSubscriptionConfirm` + `formatPaymentAmount` are already used in that file — see the `subscription_usd_to_cny_rate: 7.15` test around line 297):

```ts
  it('converts subscription price to VND when the VND rate is configured', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        subscription_usd_to_vnd_rate: 25000,
      },
      method: {
        currency: 'VND',
      },
      plan: {
        price: 9.99,
        original_price: 12.99,
      },
    })

    const text = wrapper.text()
    const convertedPrice = formatPaymentAmount(249750, 'VND')
    const convertedOriginalPrice = formatPaymentAmount(324750, 'VND')

    expect(text).toContain(convertedPrice)
    expect(text).toContain(convertedOriginalPrice)
    expect(text).not.toContain(formatPaymentAmount(9.99, 'VND'))
  })
```

(If `mountSubscriptionConfirm`'s `method` fixture needs a payment-type key for the visible method, pass the same value the CNY test uses — the currency field is what drives the conversion path.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend && npx vitest run src/views/user/__tests__/PaymentView.spec.ts src/views/admin/__tests__/SettingsView.spec.ts`
Expected: FAIL — new field missing from types/defaults; VND conversion not implemented.

- [ ] **Step 3: Implement**

`types/payment.ts` — add to both interfaces next to the CNY field:

```ts
  subscription_usd_to_vnd_rate: number
```

`api/admin/settings.ts` — same, in both spots (response + request types):

```ts
  payment_subscription_usd_to_vnd_rate: number;
```
```ts
  payment_subscription_usd_to_vnd_rate?: number;
```

`api/admin/payment.ts` — same two spots:

```ts
  subscription_usd_to_vnd_rate: number
```
```ts
  subscription_usd_to_vnd_rate?: number
```

`PaymentView.vue`:
1. Default checkout object (~line 505): add `subscription_usd_to_vnd_rate: 0,`.
2. Next to `subscriptionUsdToCnyRate` (~line 523):

```ts
// 订阅 VND 换算汇率（1 USD = X VND）。0 = 未配置（后端会拒绝 VND 订阅下单）。
const subscriptionUsdToVndRate = computed(() => {
  const rate = checkout.value.subscription_usd_to_vnd_rate
  return Number.isFinite(rate) && rate > 0 ? rate : 0
})
```
3. Replace `subscriptionPaymentAmountForCurrency` (~line 596):

```ts
function subscriptionPaymentAmountForCurrency(value: number, currency: string): number {
  if (currency === 'VND') {
    const vndRate = subscriptionUsdToVndRate.value
    return vndRate > 0 ? roundPaymentAmount(value * vndRate, currency) : roundPaymentAmount(value, currency)
  }
  const rate = subscriptionUsdToCnyRate.value
  if (rate <= 0 || currency !== DEFAULT_PAYMENT_CURRENCY) return roundPaymentAmount(value, currency)
  return roundPaymentAmount(value * rate, currency)
}
```

`SettingsView.vue`:
1. Form default (~line 9471): `payment_subscription_usd_to_vnd_rate: 0,`.
2. Submit payload (~line 11270): `payment_subscription_usd_to_vnd_rate: Number(form.payment_subscription_usd_to_vnd_rate) || 0,`.
3. Input markup — duplicate the CNY rate input block (~lines 7789-7810) directly below it and change: label key `subscriptionUsdToVndRate`, model `payment_subscription_usd_to_vnd_rate`, placeholder key `subscriptionUsdToVndRateDisabled`, `step="1"` (VND rates are large integers):

```html
                  <div>
                    <label class="input-label">{{
                      t("admin.settings.payment.subscriptionUsdToVndRate")
                    }}</label>
                    <input
                      :value="form.payment_subscription_usd_to_vnd_rate || ''"
                      @input="
                        form.payment_subscription_usd_to_vnd_rate =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="1"
                      min="0"
                      class="input"
                      :placeholder="
                        t(
                          'admin.settings.payment.subscriptionUsdToVndRateDisabled',
                        )
                      "
                    />
                  </div>
```

i18n `en/admin/settings.ts` (next to `subscriptionUsdToCnyRate` keys — grep for placement):

```ts
    subscriptionUsdToVndRate: 'Subscription USD→VND Rate (1 USD = X VND)',
    subscriptionUsdToVndRateDisabled: 'Disabled — VND subscription checkout will be rejected',
```

`zh/admin/settings.ts`:

```ts
    subscriptionUsdToVndRate: '订阅 USD→VND 汇率（1 USD = X VND）',
    subscriptionUsdToVndRateDisabled: '未配置 — VND 订阅下单将被拒绝',
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend && npx vitest run`
Expected: PASS (whole frontend suite green; fix any fixture that still misses the new field).

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(payment-frontend): VND subscription rate display and admin input"
```

---

### Task 13: Full verification

**Files:** none (verification only).

- [ ] **Step 1: Backend build + full tests**

Run: `cd backend && go build ./... && go test ./... && go test -tags=unit ./...`
Expected: all PASS. (Two passes are needed: handler/service unit tests carry the `//go:build unit` tag; provider package tests are untagged.)

- [ ] **Step 2: Frontend type-check + full tests**

Run: `cd frontend && npx vitest run && npm run build`
Expected: build succeeds, tests PASS. (If `npm run build` is not the build script, check `frontend/package.json` scripts and use the build/typecheck script present.)

- [ ] **Step 3: Smoke checklist (code review level, no live SePay account needed)**

Verify by reading code (no external calls):
1. `POST /api/v1/payment/webhook/sepay` replies exactly `{"success":true}` on the happy path (Task 7 test proves it).
2. A sepay provider instance with `payment_mode` unset still renders the QR: `CreatePayment` returns `QRCode` and the frontend `determinePaymentLaunchKind` picks `qr_waiting` because `prefersQr` is true whenever `qrCode` is set and `paymentMode` is not redirect/popup.
3. `grep -rn "sepay" backend/internal/service/payment_config_service.go backend/internal/service/payment_order.go` shows the VND guard runs before any order row is written.

- [ ] **Step 4: Final commit if anything was fixed**

```bash
git add -A && git commit -m "chore(payment): sepay integration final fixes"
```

---

## Notes for implementers

- The repo's dev guide is `DEV_GUIDE.md`; payment docs live in `docs/PAYMENT.md` (both worth skimming before starting).
- The spec for this plan: `docs/superpowers/specs/2026-08-14-sepay-payment-gateway-design.md`.
- Admin setup (documented for the user, not code): on the SePay dashboard create a webhook pointing at `https://<panel-host>/api/v1/payment/webhook/sepay` with HMAC secret (or API key), and configure the payment-code extraction so codes match the `sub2_` prefix used in transfer content.
- SePay docs: https://developer.sepay.vn/vi/sepay-webhooks/tich-hop-webhook (payload), .../xac-thuc (auth), https://developer.sepay.vn/vi/sepay-api/v2/giao-dich/danh-sach (query API).
