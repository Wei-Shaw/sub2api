package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"

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
	cases := []struct {
		code string
		want bool
	}{
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
