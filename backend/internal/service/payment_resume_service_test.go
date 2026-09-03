//go:build unit

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestNormalizeVisibleMethods(t *testing.T) {
	t.Parallel()

	// 归一化只做去空白和去重：sepay_* 是三个独立的可见方式，
	// 折叠成网关键会让下单选不中用户点的那一个。
	got := NormalizeVisibleMethods([]string{
		payment.TypeSePayBankTransfer,
		" " + payment.TypeSePayNapas + " ",
		payment.TypeSePayCard,
		payment.TypeSePayBankTransfer,
		"",
	})

	want := []string{payment.TypeSePayBankTransfer, payment.TypeSePayNapas, payment.TypeSePayCard}
	if len(got) != len(want) {
		t.Fatalf("NormalizeVisibleMethods len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NormalizeVisibleMethods[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestCanonicalizeReturnURL(t *testing.T) {
	t.Parallel()

	got, err := CanonicalizeReturnURL("https://example.com/payment/result?b=2#a", "example.com", "")
	if err != nil {
		t.Fatalf("CanonicalizeReturnURL returned error: %v", err)
	}
	if got != "https://example.com/payment/result?b=2" {
		t.Fatalf("CanonicalizeReturnURL = %q, want %q", got, "https://example.com/payment/result?b=2")
	}
}

func TestCanonicalizeReturnURLRejectsRelativeURL(t *testing.T) {
	t.Parallel()

	if _, err := CanonicalizeReturnURL("/payment/result", "example.com", ""); err == nil {
		t.Fatal("CanonicalizeReturnURL should reject relative URLs")
	}
}

func TestCanonicalizeReturnURLRejectsExternalHost(t *testing.T) {
	t.Parallel()

	if _, err := CanonicalizeReturnURL("https://evil.example/payment/result", "app.example.com", ""); err == nil {
		t.Fatal("CanonicalizeReturnURL should reject external hosts")
	}
}

func TestCanonicalizeReturnURLAllowsConfiguredFrontendHost(t *testing.T) {
	t.Parallel()

	got, err := CanonicalizeReturnURL(
		"https://app.example.com/payment/result?from=checkout",
		"api.example.com",
		"https://app.example.com/purchase",
	)
	if err != nil {
		t.Fatalf("CanonicalizeReturnURL returned error: %v", err)
	}
	if got != "https://app.example.com/payment/result?from=checkout" {
		t.Fatalf("CanonicalizeReturnURL = %q, want %q", got, "https://app.example.com/payment/result?from=checkout")
	}
}

func TestCanonicalizeReturnURLRejectsNonCanonicalPath(t *testing.T) {
	t.Parallel()

	if _, err := CanonicalizeReturnURL("https://app.example.com/orders/42", "app.example.com", ""); err == nil {
		t.Fatal("CanonicalizeReturnURL should reject non-canonical result paths")
	}
}

func TestBuildPaymentReturnURL(t *testing.T) {
	t.Parallel()

	got, err := buildPaymentReturnURL("https://example.com/payment/result?from=checkout#fragment", 42, "sub2_42", "resume-token")
	if err != nil {
		t.Fatalf("buildPaymentReturnURL returned error: %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}
	if parsed.Fragment != "" {
		t.Fatalf("buildPaymentReturnURL should strip fragments, got %q", parsed.Fragment)
	}
	query := parsed.Query()
	if query.Get("from") != "checkout" {
		t.Fatalf("expected original query to be preserved, got %q", query.Get("from"))
	}
	if query.Get("order_id") != strconv.FormatInt(42, 10) {
		t.Fatalf("order_id = %q", query.Get("order_id"))
	}
	if query.Get("out_trade_no") != "sub2_42" {
		t.Fatalf("out_trade_no = %q", query.Get("out_trade_no"))
	}
	if query.Get("resume_token") != "resume-token" {
		t.Fatalf("resume_token = %q", query.Get("resume_token"))
	}
	if query.Get("status") != "success" {
		t.Fatalf("status = %q", query.Get("status"))
	}
}

func TestBuildPaymentReturnURLWithoutResumeTokenStillIncludesOutTradeNo(t *testing.T) {
	t.Parallel()

	got, err := buildPaymentReturnURL("https://example.com/payment/result", 42, "sub2_42", "")
	if err != nil {
		t.Fatalf("buildPaymentReturnURL returned error: %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}
	query := parsed.Query()
	if query.Get("order_id") != "42" {
		t.Fatalf("order_id = %q", query.Get("order_id"))
	}
	if query.Get("out_trade_no") != "sub2_42" {
		t.Fatalf("out_trade_no = %q", query.Get("out_trade_no"))
	}
	if query.Get("resume_token") != "" {
		t.Fatalf("resume_token = %q, want empty", query.Get("resume_token"))
	}
}

func TestBuildPaymentReturnURLEmptyBase(t *testing.T) {
	t.Parallel()

	got, err := buildPaymentReturnURL("", 42, "sub2_42", "resume-token")
	if err != nil {
		t.Fatalf("buildPaymentReturnURL returned error: %v", err)
	}
	if got != "" {
		t.Fatalf("buildPaymentReturnURL = %q, want empty string", got)
	}
}

func TestPaymentResumeTokenRoundTrip(t *testing.T) {
	t.Parallel()

	svc := NewPaymentResumeService([]byte("0123456789abcdef0123456789abcdef"))
	token, err := svc.CreateToken(ResumeTokenClaims{
		OrderID:            42,
		UserID:             7,
		ProviderInstanceID: "19",
		ProviderKey:        "easypay",
		PaymentType:        "wxpay",
		CanonicalReturnURL: "https://example.com/payment/result",
		IssuedAt:           1234567890,
	})
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
	}

	claims, err := svc.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken returned error: %v", err)
	}
	if claims.OrderID != 42 || claims.UserID != 7 {
		t.Fatalf("claims mismatch: %+v", claims)
	}
	if claims.ProviderInstanceID != "19" || claims.ProviderKey != "easypay" || claims.PaymentType != "wxpay" {
		t.Fatalf("claims provider snapshot mismatch: %+v", claims)
	}
	if claims.CanonicalReturnURL != "https://example.com/payment/result" {
		t.Fatalf("claims return URL = %q", claims.CanonicalReturnURL)
	}
}

func TestCreateTokenRejectsMissingSigningKey(t *testing.T) {
	t.Parallel()

	svc := NewPaymentResumeService(nil)
	_, err := svc.CreateToken(ResumeTokenClaims{OrderID: 42})
	if err == nil {
		t.Fatal("CreateToken should reject missing signing key")
	}
}

func TestParseTokenRejectsFallbackSignedTokenWhenSigningKeyMissing(t *testing.T) {
	t.Parallel()

	token := mustCreateFallbackSignedToken(t, ResumeTokenClaims{OrderID: 42, UserID: 7})
	svc := NewPaymentResumeService(nil)
	_, err := svc.ParseToken(token)
	if err == nil {
		t.Fatal("ParseToken should reject tokens when signing key is missing")
	}
}

func TestParseTokenRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	svc := NewPaymentResumeService([]byte("0123456789abcdef0123456789abcdef"))
	token, err := svc.CreateToken(ResumeTokenClaims{
		OrderID:   42,
		UserID:    7,
		IssuedAt:  time.Now().Add(-25 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
	}

	_, err = svc.ParseToken(token)
	if err == nil {
		t.Fatal("ParseToken should reject expired tokens")
	}
}

type captureLoadBalancer struct {
	lastProviderKey string
	lastPaymentType string
}

func (c *captureLoadBalancer) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	return map[string]string{}, nil
}

func (c *captureLoadBalancer) SelectInstance(_ context.Context, providerKey string, paymentType payment.PaymentType, _ payment.Strategy, _ float64) (*payment.InstanceSelection, error) {
	c.lastProviderKey = providerKey
	c.lastPaymentType = paymentType
	return &payment.InstanceSelection{ProviderKey: providerKey, SupportedTypes: paymentType}, nil
}

func mustCreateFallbackSignedToken(t *testing.T, claims any) string {
	t.Helper()

	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte("sub2api-payment-resume"))
	_, _ = mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encodedPayload + "." + signature
}
