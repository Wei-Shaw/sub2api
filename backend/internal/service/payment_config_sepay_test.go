//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestListSepayBankAccountsUsesTemporaryTokenAndFiltersUnsafeRows(t *testing.T) {
	const token = "[REDACTED]"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization header did not match the test token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"bank-1","bank_short_name":"MBBank","bank_full_name":"MB Bank","account_number":"1900123456789","account_holder_name":"TEST HOLDER"},{"id":"","bank_short_name":"VCB","account_number":"0123"},{"id":"bank-2","bank_short_name":"","account_number":"999"}]}`))
	}))
	defer server.Close()

	svc := &PaymentConfigService{}
	options, err := svc.ListSepayBankAccounts(context.Background(), ListSepayBankAccountsRequest{
		APIToken: token,
		APIBase:  server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 {
		t.Fatalf("got %d options, want 1: %+v", len(options), options)
	}
	if options[0].ID != "bank-1" || options[0].Label != "MBBank · 1900123456789 · TEST HOLDER" {
		t.Fatalf("unexpected picker option: %+v", options[0])
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), token) {
		t.Fatalf("picker response contains token: %s", encoded)
	}
}

func TestListSepayBankAccountsEditFlowReusesStoredTokenServerSide(t *testing.T) {
	const token = "[REDACTED]"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization header did not match the stored test token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bankaccounts":[{"id":7,"bank_short_name":"ACB","account_number":"123456"}]}`))
	}))
	defer server.Close()

	client := newPaymentConfigServiceTestClient(t)
	instance, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSepay).
		SetName("SePay").
		SetConfig(`{"apiToken":"` + token + `","apiBase":"` + server.URL + `"}`).
		SetSupportedTypes(payment.TypeSepay).
		SetEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	svc := &PaymentConfigService{entClient: client}
	options, err := svc.ListSepayBankAccounts(context.Background(), ListSepayBankAccountsRequest{
		ProviderID: int64(instance.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].ID != "7" || options[0].BankShortName != "ACB" {
		t.Fatalf("unexpected edit-flow options: %+v", options)
	}
}

func TestListSepayBankAccountsRejectsNonSepayProviderID(t *testing.T) {
	client := newPaymentConfigServiceTestClient(t)
	instance, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("Stripe").
		SetConfig(`{"apiToken":"[REDACTED]"}`).
		SetSupportedTypes("card").
		SetEnabled(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	svc := &PaymentConfigService{entClient: client}
	_, err = svc.ListSepayBankAccounts(context.Background(), ListSepayBankAccountsRequest{ProviderID: int64(instance.ID)})
	if err == nil || !strings.Contains(err.Error(), "not a SePay provider") {
		t.Fatalf("non-SePay provider error = %v", err)
	}
}
