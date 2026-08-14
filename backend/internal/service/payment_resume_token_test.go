//go:build unit

package service

import (
	"strings"
	"testing"
	"time"
)

func newTestResumeService(t *testing.T) *PaymentResumeService {
	t.Helper()
	return NewPaymentResumeService([]byte("resume-signing-key-for-tests"))
}

func testResumeClaims() ResumeTokenClaims {
	return ResumeTokenClaims{
		OrderID:            918273,
		UserID:             4455,
		ProviderInstanceID: "2",
		ProviderKey:        "nowpayments",
		PaymentType:        "nowpayments",
	}
}

func TestCreateTokenRoundTrips(t *testing.T) {
	svc := newTestResumeService(t)
	token, err := svc.CreateToken(testResumeClaims())
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	parsed, err := svc.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if parsed.OrderID != 918273 || parsed.UserID != 4455 {
		t.Fatalf("claims = %+v, want order 918273 / user 4455", parsed)
	}
	if parsed.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("ExpiresAt = %d, want a future timestamp", parsed.ExpiresAt)
	}
	want := paymentResumeBindHash("nowpayments", "2", "nowpayments")
	if string(parsed.BindHash) != string(want) {
		t.Fatalf("BindHash = %x, want %x", parsed.BindHash, want)
	}
}

// The token rides in a hosted checkout's return URL, and NOWPayments refuses an
// invoice whose success_url passes 255 characters. The signed-JSON form it
// replaced ran past 260 characters on its own.
func TestCreateTokenStaysShortEnoughForAHostedRedirect(t *testing.T) {
	svc := newTestResumeService(t)
	token, err := svc.CreateToken(testResumeClaims())
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if len(token) > 64 {
		t.Fatalf("token is %d characters, want at most 64: %s", len(token), token)
	}

	returnURL, err := buildPaymentReturnURL(
		"https://panel.example-domain.com/payment/result",
		918273,
		"sub2_20260814ABCD1234",
		token,
	)
	if err != nil {
		t.Fatalf("buildPaymentReturnURL: %v", err)
	}
	if len(returnURL) > 255 {
		t.Fatalf("return URL is %d characters, over the 255 NOWPayments accepts: %s", len(returnURL), returnURL)
	}
}

func TestParseTokenRejectsTampering(t *testing.T) {
	svc := newTestResumeService(t)
	token, err := svc.CreateToken(testResumeClaims())
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	flipped := []byte(token)
	if flipped[0] == 'A' {
		flipped[0] = 'B'
	} else {
		flipped[0] = 'A'
	}
	if _, err := svc.ParseToken(string(flipped)); err == nil {
		t.Fatal("ParseToken accepted a tampered token")
	}
	if _, err := svc.ParseToken(token[:len(token)-1]); err == nil {
		t.Fatal("ParseToken accepted a truncated token")
	}
	if _, err := newTestOtherResumeService().ParseToken(token); err == nil {
		t.Fatal("ParseToken accepted a token signed with another key")
	}
}

func newTestOtherResumeService() *PaymentResumeService {
	return NewPaymentResumeService([]byte("a-completely-different-signing-key"))
}

func TestParseTokenRejectsExpired(t *testing.T) {
	svc := newTestResumeService(t)
	claims := testResumeClaims()
	claims.ExpiresAt = time.Now().Add(-time.Minute).Unix()
	token, err := svc.CreateToken(claims)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if _, err := svc.ParseToken(token); err == nil {
		t.Fatal("ParseToken accepted an expired token")
	}
}

// Tokens minted before the packed format still have to resolve until the last
// one issued ages out of its 24-hour life.
func TestParseTokenStillAcceptsLegacySignedJSON(t *testing.T) {
	svc := newTestResumeService(t)
	claims := testResumeClaims()
	claims.ExpiresAt = time.Now().Add(time.Hour).Unix()
	legacy, err := svc.createSignedToken(claims)
	if err != nil {
		t.Fatalf("createSignedToken: %v", err)
	}
	if !strings.Contains(legacy, ".") {
		t.Fatalf("legacy token %q lost its separator", legacy)
	}
	parsed, err := svc.ParseToken(legacy)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if parsed.OrderID != claims.OrderID || parsed.ProviderKey != "nowpayments" {
		t.Fatalf("claims = %+v, want the legacy string claims preserved", parsed)
	}
	if len(parsed.BindHash) != 0 {
		t.Fatal("legacy token produced a bind hash, which would bypass the string checks")
	}
}

func TestPaymentResumeBindHashDistinguishesProviders(t *testing.T) {
	base := paymentResumeBindHash("nowpayments", "2", "nowpayments")
	if string(base) == string(paymentResumeBindHash("sepay", "2", "sepay")) {
		t.Fatal("bind hash ignored the provider key")
	}
	if string(base) == string(paymentResumeBindHash("nowpayments", "3", "nowpayments")) {
		t.Fatal("bind hash ignored the instance id")
	}
	if string(base) != string(paymentResumeBindHash("NOWPayments", "2", "NowPayments")) {
		t.Fatal("bind hash is case sensitive, but the claims it replaced were not")
	}
}
