//go:build unit

package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// Vectors produced by the official @gpmpay/sdk 0.4.0 (signWebhookPayload, buildVietQrPayload).
const (
	gpmpayTestBody      = `{"id":"b6fec789-f2bd-445d-88e5-86aee24f35c8","content":"MBVCB.123 SUB220260930AB3KX9MQ chuyển tiền","transferType":"in","transferAmount":50000}`
	gpmpayTestSecret    = "whsec_test"
	gpmpayTestTimestamp = 1790000000
	gpmpayTestSignature = "t=1790000000,v1=8bb1eddc21acffb8da119960a39609ea402bc99b7f6ca624aadf725003902648"
	gpmpayTestVietQR    = "00020101021238540010A00000072701240006970422011003429710860208QRIBFTTA53037045405500005802VN62240820SUB220260930AB3KX9MQ6304ACE2"
)

func newTestGPMPay(t *testing.T) *GPMPay {
	t.Helper()
	g, err := NewGPMPay("inst-1", map[string]string{
		"apiToken": "gpm_TESTpub1_abcdefghijklmnopqrstuvwx", "webhookSecret": gpmpayTestSecret,
		"bankBin": "970422", "accountNumber": "0342971086",
	})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// gpmpayTestSign signs like the SDK; TestGPMPaySignatureMatchesSDK pins the scheme to a real SDK vector.
func gpmpayTestSign(body string) map[string]string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(gpmpayTestSecret))
	_, _ = mac.Write([]byte(ts + "." + body))
	return map[string]string{"x-gpmpay-signature": "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))}
}

func TestGPMPaySignatureMatchesSDK(t *testing.T) {
	at := time.Unix(gpmpayTestTimestamp, 0)
	if err := gpmpayVerifySignature(gpmpayTestSignature, gpmpayTestBody, gpmpayTestSecret, at); err != nil {
		t.Fatalf("SDK signature rejected: %v", err)
	}
	for name, tc := range map[string]struct {
		header, body, secret string
		now                  time.Time
	}{
		"tampered body": {gpmpayTestSignature, strings.Replace(gpmpayTestBody, "50000", "500000", 1), gpmpayTestSecret, at},
		"wrong secret":  {gpmpayTestSignature, gpmpayTestBody, "other", at},
		"skew 600s":     {gpmpayTestSignature, gpmpayTestBody, gpmpayTestSecret, at.Add(600 * time.Second)},
		"missing":       {"", gpmpayTestBody, gpmpayTestSecret, at},
	} {
		if gpmpayVerifySignature(tc.header, tc.body, tc.secret, tc.now) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestGPMPayCreatePaymentBuildsSDKVietQR(t *testing.T) {
	g := newTestGPMPay(t)
	resp, err := g.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_20260930aB3kX9mQ", Amount: "50000", PaymentType: payment.TypeGPMPayBankTransfer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.QRCode != gpmpayTestVietQR {
		t.Fatalf("QR mismatch:\n got %s\nwant %s", resp.QRCode, gpmpayTestVietQR)
	}
	if _, err := g.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "sub2_x", Amount: "50000.5"}); err == nil {
		t.Fatal("fractional VND accepted")
	}
}

func TestGPMPayVerifyNotification(t *testing.T) {
	g := newTestGPMPay(t)

	n, err := g.VerifyNotification(context.Background(), gpmpayTestBody, gpmpayTestSign(gpmpayTestBody))
	if err != nil || n == nil {
		t.Fatalf("incoming transfer: n=%v err=%v", n, err)
	}
	// The bank uppercased the memo; the service layer resolves it case-insensitively.
	if n.OrderID != "sub2_20260930AB3KX9MQ" || n.Amount.String() != "50000" || n.TradeNo != "b6fec789-f2bd-445d-88e5-86aee24f35c8" {
		t.Fatalf("unexpected notification: %+v", n)
	}

	for name, body := range map[string]string{
		"outgoing":  `{"id":"1","content":"SUB220260930AB3KX9MQ","transferType":"out","transferAmount":50000}`,
		"ping":      `{"id":"0","content":"x","transferType":"in","transferAmount":1,"test":true}`,
		"no code":   `{"id":"2","content":"tien nha thang 9","transferType":"in","transferAmount":50000}`,
		"simulated": `{"id":"3","content":"SUB220260930AB3KX9MQ","transferType":"in","transferAmount":50000,"source":"SIMULATED"}`,
	} {
		if n, err := g.VerifyNotification(context.Background(), body, gpmpayTestSign(body)); err != nil || n != nil {
			t.Errorf("%s: want ignored, got n=%v err=%v", name, n, err)
		}
	}
	if _, err := g.VerifyNotification(context.Background(), gpmpayTestBody, map[string]string{}); err == nil {
		t.Fatal("unsigned webhook accepted")
	}
}

type gpmpayRoundTrip func(*http.Request) *http.Response

func (f gpmpayRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

func TestGPMPayQueryOrderPicksMatchingIncomingTransfer(t *testing.T) {
	g := newTestGPMPay(t)
	var gotURL, gotAuth string
	g.client = &http.Client{Transport: gpmpayRoundTrip(func(r *http.Request) *http.Response {
		gotURL, gotAuth = r.URL.String(), r.Header.Get("Authorization")
		body := `{"statusCode":200,"data":{"data":[
			{"id":"a","type":"IN","amount":"20000","transferContent":"SUB220260930AB3KX9MQ","transactionTime":"2026-09-30T10:00:00Z"},
			{"id":"b","type":"IN","amount":"50000","transferContent":"ck SUB220260930AB3KX9MQ","transactionTime":"2026-09-30T10:05:00Z"},
			{"id":"c","type":"IN","amount":"90000","transferContent":"SUB220260930ZZZZZZZZ","transactionTime":"2026-09-30T10:06:00Z"}
		],"meta":{"page":1}}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
	})}

	resp, err := g.QueryOrder(context.Background(), "sub2_20260930aB3kX9mQ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotURL, "search=SUB220260930AB3KX9MQ") || gotAuth != "Bearer gpm_TESTpub1_abcdefghijklmnopqrstuvwx" {
		t.Errorf("bad request: url=%s auth=%s", gotURL, gotAuth)
	}
	if resp.Status != payment.ProviderStatusPaid || resp.TradeNo != "b" || resp.Amount.String() != "50000" {
		t.Fatalf("unexpected query response: %+v", resp)
	}
}

func TestTransferCodeRoundTrip(t *testing.T) {
	code := payment.TransferCodeForOutTradeNo("sub2_20260930aB3kX9mQ")
	if code != "SUB220260930AB3KX9MQ" {
		t.Fatalf("code = %s", code)
	}
	if got := payment.OutTradeNoFromTransferContent("IBFT " + code + "FT2627"); !strings.EqualFold(got, "sub2_20260930aB3kX9mQ") {
		t.Fatalf("round trip = %s", got)
	}
}
