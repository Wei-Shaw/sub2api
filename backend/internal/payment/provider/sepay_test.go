package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestNewSepayAllowsMissingBankAccountID(t *testing.T) {
	provider, err := NewSepay("sepay-1", map[string]string{
		"apiToken":  "token",
		"notifyUrl": "https://example.com/api/v1/payment/webhook/sepay",
	})
	if err != nil {
		t.Fatalf("NewSepay() error = %v", err)
	}
	if provider == nil {
		t.Fatal("NewSepay() returned nil provider")
	}
}

func TestSepayCreatePaymentAutoDiscoversSingleBankAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/bankaccounts/list":
			_, _ = w.Write([]byte(`{"bankaccounts":[{"id":"53975","account_number":"333999333333","bank_short_name":"MBBank","account_holder_name":"NGUYEN THANH CONG"}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider, err := NewSepay("sepay-1", map[string]string{
		"apiToken":  "token",
		"notifyUrl": "https://example.com/api/v1/payment/webhook/sepay",
		"apiBase":   server.URL,
	})
	if err != nil {
		t.Fatalf("NewSepay() error = %v", err)
	}

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:         "sub2_20260429AbC123xY",
		Amount:          "50000",
		PaymentCurrency: "VND",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	if !strings.Contains(resp.QRCode, "acc=333999333333") {
		t.Fatalf("QRCode missing account number: %s", resp.QRCode)
	}
	if !strings.Contains(resp.QRCode, "bank=MBBank") {
		t.Fatalf("QRCode missing bank short name: %s", resp.QRCode)
	}
	if strings.Contains(resp.QRCode, "sub2_") {
		t.Fatalf("QRCode leaked internal order prefix: %s", resp.QRCode)
	}
	if !strings.Contains(resp.QRCode, "VC20260429AbC123xY") {
		t.Fatalf("QRCode missing SePay transfer reference: %s", resp.QRCode)
	}
}

func TestSepayCreatePaymentRequiresBankAccountWhenMultipleAccounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bankaccounts/list" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"bankaccounts":[{"id":"1","account_number":"111","bank_short_name":"VCB"},{"id":"2","account_number":"222","bank_short_name":"MBBank"}]}`))
	}))
	defer server.Close()

	provider, err := NewSepay("sepay-1", map[string]string{
		"apiToken":  "token",
		"notifyUrl": "https://example.com/api/v1/payment/webhook/sepay",
		"apiBase":   server.URL,
	})
	if err != nil {
		t.Fatalf("NewSepay() error = %v", err)
	}

	_, err = provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:         "sub2_20260429AbC123xY",
		Amount:          "50000",
		PaymentCurrency: "VND",
	})
	if err == nil || !strings.Contains(err.Error(), "multiple bank accounts") {
		t.Fatalf("CreatePayment() error = %v, want multiple bank accounts", err)
	}
}

func TestSepayCreatePaymentUsesExplicitBankAccountID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bankaccounts/details/53975" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"bankaccount":{"id":"53975","account_number":"333999333333","bank_short_name":"MBBank","account_holder_name":"NGUYEN THANH CONG"}}`))
	}))
	defer server.Close()

	provider, err := NewSepay("sepay-1", map[string]string{
		"apiToken":      "token",
		"notifyUrl":     "https://example.com/api/v1/payment/webhook/sepay",
		"apiBase":       server.URL,
		"bankAccountId": "53975",
	})
	if err != nil {
		t.Fatalf("NewSepay() error = %v", err)
	}

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:         "sub2_20260429AbC123xY",
		Amount:          "50000",
		PaymentCurrency: "VND",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	if !strings.Contains(resp.QRCode, "acc=333999333333") {
		t.Fatalf("QRCode missing explicit account number: %s", resp.QRCode)
	}
}

func TestSepayVerifyNotificationExtractsLegacyOrderCodeFromContent(t *testing.T) {
	provider, err := NewSepay("sepay-1", map[string]string{
		"apiToken":  "token",
		"notifyUrl": "https://example.com/api/v1/payment/webhook/sepay",
	})
	if err != nil {
		t.Fatalf("NewSepay() error = %v", err)
	}

	notification, err := provider.VerifyNotification(context.Background(), `{
		"id": 92704,
		"code": null,
		"content": "NAP sub2_20260429AbC123xY cho user",
		"transferType": "in",
		"transferAmount": 50000,
		"referenceCode": "MBVCB.3278907687"
	}`, nil)
	if err != nil {
		t.Fatalf("VerifyNotification() error = %v", err)
	}
	if notification == nil {
		t.Fatal("VerifyNotification() returned nil notification")
	}
	if notification.OrderID != "sub2_20260429AbC123xY" {
		t.Fatalf("OrderID = %q, want sub2_20260429AbC123xY", notification.OrderID)
	}
}

func TestSepayCreatePaymentUsesMeaningfulTransferContentWithoutInternalPrefix(t *testing.T) {
	provider, err := NewSepay("sepay-1", map[string]string{
		"apiToken":      "token",
		"notifyUrl":     "https://example.com/api/v1/payment/webhook/sepay",
		"bankAccountId": "53975",
	})
	if err != nil {
		t.Fatalf("NewSepay() error = %v", err)
	}
	qr, err := provider.buildQRCodeURL(&SepayBankAccount{AccountNumber: "333999333333", BankShortName: "MBBank"}, 50000, buildSepayTransferContent("sub2_20260429AbC123xY"))
	if err != nil {
		t.Fatalf("buildQRCodeURL() error = %v", err)
	}
	if strings.Contains(qr, "sub2_") {
		t.Fatalf("QRCode leaked internal sub2 prefix: %s", qr)
	}
	if !strings.Contains(qr, "VClaw") || !strings.Contains(qr, "VC20260429AbC123xY") {
		t.Fatalf("QRCode transfer content should be meaningful and include hidden reference: %s", qr)
	}
}

func TestSepayVerifyNotificationExtractsTransferReferenceFromContent(t *testing.T) {
	provider, err := NewSepay("sepay-1", map[string]string{
		"apiToken":  "token",
		"notifyUrl": "https://example.com/api/v1/payment/webhook/sepay",
	})
	if err != nil {
		t.Fatalf("NewSepay() error = %v", err)
	}

	notification, err := provider.VerifyNotification(context.Background(), `{
		"id": 92704,
		"code": null,
		"content": "Cam on VClaw VC20260429AbC123xY",
		"transferType": "in",
		"transferAmount": 50000,
		"referenceCode": "MBVCB.3278907687"
	}`, nil)
	if err != nil {
		t.Fatalf("VerifyNotification() error = %v", err)
	}
	if notification == nil {
		t.Fatal("VerifyNotification() returned nil notification")
	}
	if notification.OrderID != "sub2_20260429AbC123xY" {
		t.Fatalf("OrderID = %q, want sub2_20260429AbC123xY", notification.OrderID)
	}
}

func TestSepayVerifyNotificationNormalizesWebhookCodeSuffix(t *testing.T) {
	provider, err := NewSepay("sepay-1", map[string]string{
		"apiToken":  "token",
		"notifyUrl": "https://example.com/api/v1/payment/webhook/sepay",
	})
	if err != nil {
		t.Fatalf("NewSepay() error = %v", err)
	}

	notification, err := provider.VerifyNotification(context.Background(), `{
		"id": 92704,
		"code": "20260429AbC123xY",
		"content": "Cam on VClaw VC20260429AbC123xY",
		"transferType": "in",
		"transferAmount": 50000,
		"referenceCode": "MBVCB.3278907687"
	}`, nil)
	if err != nil {
		t.Fatalf("VerifyNotification() error = %v", err)
	}
	if notification == nil {
		t.Fatal("VerifyNotification() returned nil notification")
	}
	if notification.OrderID != "sub2_20260429AbC123xY" {
		t.Fatalf("OrderID = %q, want sub2_20260429AbC123xY", notification.OrderID)
	}
}

func TestSepayVerifyNotificationRequiresConfiguredWebhookAPIKey(t *testing.T) {
	provider, err := NewSepay("sepay-1", map[string]string{
		"apiToken":      "token",
		"notifyUrl":     "https://example.com/api/v1/payment/webhook/sepay",
		"webhookApiKey": "webhook-secret",
	})
	if err != nil {
		t.Fatalf("NewSepay() error = %v", err)
	}
	body := `{
		"id": 92704,
		"code": "20260429AbC123xY",
		"content": "Cam on VClaw VC20260429AbC123xY",
		"transferType": "in",
		"transferAmount": 50000,
		"referenceCode": "MBVCB.3278907687"
	}`

	if _, err := provider.VerifyNotification(context.Background(), body, map[string]string{"authorization": "Apikey wrong"}); err == nil {
		t.Fatal("VerifyNotification() error = nil, want authorization mismatch")
	}
	notification, err := provider.VerifyNotification(context.Background(), body, map[string]string{"authorization": "apikey webhook-secret"})
	if err != nil {
		t.Fatalf("VerifyNotification() error = %v", err)
	}
	if notification == nil || notification.OrderID != "sub2_20260429AbC123xY" {
		t.Fatalf("notification = %#v, want matched SePay order", notification)
	}
}

func TestSepayListBankAccountsAcceptsNumericIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bankaccounts/list" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"bankaccounts":[{"id":123,"account_number":"0071000888888","bank_short_name":"Vietcombank","bank_full_name":"Ngân hàng TMCP Ngoại Thương Việt Nam","account_holder_name":"NGUYEN VAN A"}]}`))
	}))
	defer server.Close()

	accounts, err := FetchSepayBankAccounts(context.Background(), map[string]string{
		"apiToken":  "token",
		"notifyUrl": "https://example.com/api/v1/payment/webhook/sepay",
		"apiBase":   server.URL,
	})
	if err != nil {
		t.Fatalf("FetchSepayBankAccounts() error = %v", err)
	}
	if len(accounts) != 1 || accounts[0].IDString() != "123" {
		t.Fatalf("accounts = %#v, want numeric ID normalized to string", accounts)
	}
}
