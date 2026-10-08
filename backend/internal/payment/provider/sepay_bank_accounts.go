package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	sepayDefaultAPIBase  = "https://my.sepay.vn/userapi"
	sepayHTTPTimeout     = 10 * time.Second
	sepayMaxResponseSize = 1 << 20
)

// SepayBankAccount is the subset of the SePay bank-account response used by
// the admin picker. It contains no credential fields.
type SepayBankAccount struct {
	ID                sepayFlexibleString `json:"id"`
	AccountNumber     string              `json:"account_number"`
	BankShortName     string              `json:"bank_short_name"`
	BankFullName      string              `json:"bank_full_name"`
	AccountHolderName string              `json:"account_holder_name"`
}

func (a SepayBankAccount) IDString() string {
	return strings.TrimSpace(a.ID.String())
}

// FetchSepayBankAccounts calls the SePay account-list API for the admin picker.
// This helper is intentionally separate from Sepay.CreatePayment and
// Sepay.VerifyNotification: settlement remains network-free.
func FetchSepayBankAccounts(ctx context.Context, config map[string]string) ([]SepayBankAccount, error) {
	apiToken := strings.TrimSpace(sepayConfigValue(config, "apiToken"))
	if apiToken == "" {
		return nil, fmt.Errorf("sepay apiToken is required to list bank accounts")
	}
	base := strings.TrimRight(strings.TrimSpace(sepayConfigValue(config, "apiBase")), "/")
	if base == "" {
		base = sepayDefaultAPIBase
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("sepay apiBase must be an absolute URL")
	}
	endpoint := strings.TrimRight(parsed.String(), "/") + "/bankaccounts/list"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create sepay bank-account request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := (&http.Client{Timeout: sepayHTTPTimeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("request sepay bank accounts: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, sepayMaxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read sepay bank-account response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("sepay bank-account request returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		BankAccounts []SepayBankAccount `json:"bankaccounts"`
		Data         []SepayBankAccount `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse sepay bank-account response: %w", err)
	}
	if len(payload.BankAccounts) > 0 {
		return payload.BankAccounts, nil
	}
	return payload.Data, nil
}
