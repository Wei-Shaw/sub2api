package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestListSepayBankAccountsUsesStoredProviderToken(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/bankaccounts/list" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"bankaccounts":[{"id":53975,"account_number":"333999333333","bank_short_name":"MBBank","account_holder_name":"NGUYEN THANH CONG"}]}`))
	}))
	defer server.Close()

	instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey: payment.TypeSepay,
		Name:        "SePay",
		Config: map[string]string{
			"apiToken":  "stored-token",
			"apiBase":   server.URL,
			"notifyUrl": "https://example.com/api/v1/payment/webhook/sepay",
		},
		SupportedTypes: []string{payment.TypeSepay},
		Enabled:        false,
	})
	if err != nil {
		t.Fatalf("CreateProviderInstance() error = %v", err)
	}

	accounts, err := svc.ListSepayBankAccounts(ctx, ListSepayBankAccountsRequest{ProviderID: instance.ID})
	if err != nil {
		t.Fatalf("ListSepayBankAccounts() error = %v", err)
	}
	if gotAuth != "Bearer stored-token" {
		t.Fatalf("Authorization = %q, want stored token", gotAuth)
	}
	if len(accounts) != 1 || accounts[0].ID != "53975" || accounts[0].BankShortName != "MBBank" {
		t.Fatalf("accounts = %#v, want stored SePay bank account", accounts)
	}
}

func TestListSepayBankAccountsRequiresTokenOrProviderID(t *testing.T) {
	svc := &PaymentConfigService{}
	_, err := svc.ListSepayBankAccounts(context.Background(), ListSepayBankAccountsRequest{})
	if err == nil {
		t.Fatal("ListSepayBankAccounts() error = nil, want missing-token error")
	}
}

func TestCreateSepayProviderRequiresWebhookAPIKey(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client, encryptionKey: []byte("0123456789abcdef0123456789abcdef")}

	_, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    payment.TypeSepay,
		Name:           "sepay-without-webhook-key",
		SupportedTypes: []string{payment.TypeSepay},
		Enabled:        true,
		Config: map[string]string{
			"apiToken":      "token",
			"notifyUrl":     "https://example.com/api/v1/payment/webhook/sepay",
			"bankAccountId": "123",
		},
	})
	if err == nil {
		t.Fatal("CreateProviderInstance() error = nil, want missing webhook API key")
	}
}
