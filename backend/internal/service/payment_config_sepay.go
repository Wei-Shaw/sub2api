package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
)

// ListSepayBankAccountsRequest contains SePay API credentials used to discover
// bank accounts while configuring a provider instance. New-provider flows send a
// temporary apiToken; edit-provider flows may send providerId so the server can
// reuse the stored token without exposing it to the browser.
type ListSepayBankAccountsRequest struct {
	APIToken   string `json:"apiToken"`
	APIBase    string `json:"apiBase"`
	ProviderID int64  `json:"providerId"`
}

// SepayBankAccountOption is safe to return to the admin UI for account choice.
type SepayBankAccountOption struct {
	ID                string `json:"id"`
	BankShortName     string `json:"bank_short_name"`
	BankFullName      string `json:"bank_full_name,omitempty"`
	AccountNumber     string `json:"account_number"`
	AccountHolderName string `json:"account_holder_name,omitempty"`
	Label             string `json:"label"`
}

// ListSepayBankAccounts fetches linked SePay bank accounts server-side so the
// admin UI can render a select box instead of asking operators to know the raw
// bankAccountId.
func (s *PaymentConfigService) ListSepayBankAccounts(ctx context.Context, req ListSepayBankAccountsRequest) ([]SepayBankAccountOption, error) {
	config, err := s.resolveSepayBankAccountListConfig(ctx, req)
	if err != nil {
		return nil, err
	}

	accounts, err := provider.FetchSepayBankAccounts(ctx, config)
	if err != nil {
		return nil, err
	}

	options := make([]SepayBankAccountOption, 0, len(accounts))
	for _, account := range accounts {
		id := account.IDString()
		bank := strings.TrimSpace(account.BankShortName)
		number := strings.TrimSpace(account.AccountNumber)
		if id == "" || bank == "" || number == "" {
			continue
		}
		holder := strings.TrimSpace(account.AccountHolderName)
		label := bank + " · " + number
		if holder != "" {
			label += " · " + holder
		}
		options = append(options, SepayBankAccountOption{
			ID:                id,
			BankShortName:     bank,
			BankFullName:      strings.TrimSpace(account.BankFullName),
			AccountNumber:     number,
			AccountHolderName: holder,
			Label:             label,
		})
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no usable bank accounts returned by SePay")
	}
	return options, nil
}

func (s *PaymentConfigService) resolveSepayBankAccountListConfig(ctx context.Context, req ListSepayBankAccountsRequest) (map[string]string, error) {
	config := map[string]string{
		"apiToken":  strings.TrimSpace(req.APIToken),
		"apiBase":   strings.TrimSpace(req.APIBase),
		"notifyUrl": "https://example.invalid/api/v1/payment/webhook/sepay",
	}
	if config["apiToken"] != "" {
		return config, nil
	}
	if req.ProviderID <= 0 {
		return nil, fmt.Errorf("sepay apiToken is required to load bank accounts")
	}
	if s.entClient == nil {
		return nil, fmt.Errorf("payment config storage is unavailable")
	}

	instance, err := s.entClient.PaymentProviderInstance.Get(ctx, req.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("load sepay provider instance: %w", err)
	}
	if instance.ProviderKey != payment.TypeSepay {
		return nil, fmt.Errorf("provider instance %d is not a SePay provider", req.ProviderID)
	}
	stored, err := s.decryptConfig(instance.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt sepay provider config: %w", err)
	}
	config["apiToken"] = strings.TrimSpace(stored["apiToken"])
	if config["apiBase"] == "" {
		config["apiBase"] = strings.TrimSpace(stored["apiBase"])
	}
	if config["apiToken"] == "" {
		return nil, fmt.Errorf("sepay apiToken is required to load bank accounts")
	}
	return config, nil
}
