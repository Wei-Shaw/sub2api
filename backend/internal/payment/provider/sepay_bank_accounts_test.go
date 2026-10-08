//go:build unit

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchSepayBankAccountsUsesPickerTokenAndSafeFields(t *testing.T) {
	const token = "[REDACTED]"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/bankaccounts/list" {
			t.Errorf("request = %s %s, want GET /bankaccounts/list", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization header did not match the test token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bankaccounts":[{"id":123,"bank_short_name":"MBBank","bank_full_name":"MB Bank","account_number":"1900123456789","account_holder_name":"TEST HOLDER","apiToken":"must-not-be-decoded"},{"id":"42","bank_short_name":"VCB","account_number":"0123"}]}`))
	}))
	defer server.Close()

	accounts, err := FetchSepayBankAccounts(context.Background(), map[string]string{
		"apiToken": token,
		"apiBase":  server.URL + "/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("got %d accounts, want 2", len(accounts))
	}
	if accounts[0].IDString() != "123" || accounts[0].BankShortName != "MBBank" || accounts[0].AccountNumber != "1900123456789" {
		t.Fatalf("unexpected first account: %+v", accounts[0])
	}
	if accounts[1].IDString() != "42" {
		t.Fatalf("numeric/string ID normalization failed: %+v", accounts[1])
	}

	encoded, err := json.Marshal(accounts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), token) || strings.Contains(string(encoded), "must-not-be-decoded") {
		t.Fatalf("picker account result contains credential-like data: %s", encoded)
	}
}

func TestFetchSepayBankAccountsRejectsMissingTokenAndHTTPError(t *testing.T) {
	if _, err := FetchSepayBankAccounts(context.Background(), map[string]string{"apiBase": "https://example.test"}); err == nil || !strings.Contains(err.Error(), "apiToken is required") {
		t.Fatalf("missing token error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()
	if _, err := FetchSepayBankAccounts(context.Background(), map[string]string{
		"apiToken": "picker-token",
		"apiBase":  server.URL,
	}); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("HTTP error = %v", err)
	}
}
