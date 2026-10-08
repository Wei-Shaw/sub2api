package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
)

// ListSepayBankAccountsRequest contains the temporary token for a new provider
// or the provider ID whose encrypted token should be reused during editing.
type ListSepayBankAccountsRequest struct {
	APIToken   string `json:"apiToken"`
	APIBase    string `json:"apiBase"`
	ProviderID int64  `json:"providerId"`
}

// SepayBankAccountOption is safe to return to the admin UI. It contains no
// token, webhook key, or other credential.
type SepayBankAccountOption struct {
	ID                string `json:"id"`
	BankShortName     string `json:"bank_short_name"`
	BankFullName      string `json:"bank_full_name,omitempty"`
	AccountNumber     string `json:"account_number"`
	AccountHolderName string `json:"account_holder_name,omitempty"`
	Label             string `json:"label"`
}

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
		"apiToken": strings.TrimSpace(req.APIToken),
		"apiBase":  strings.TrimSpace(req.APIBase),
	}
	if config["apiToken"] != "" {
		return config, nil
	}
	if req.ProviderID <= 0 {
		return nil, fmt.Errorf("sepay apiToken is required to load bank accounts")
	}
	if s == nil || s.entClient == nil {
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
	for key, value := range stored {
		if strings.EqualFold(key, "apiToken") {
			config["apiToken"] = strings.TrimSpace(value)
		}
		if strings.EqualFold(key, "apiBase") && config["apiBase"] == "" {
			config["apiBase"] = strings.TrimSpace(value)
		}
	}
	if config["apiToken"] == "" {
		return nil, fmt.Errorf("sepay apiToken is required to load bank accounts")
	}
	return config, nil
}
