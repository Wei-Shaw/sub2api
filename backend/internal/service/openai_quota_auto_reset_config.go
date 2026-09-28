package service

import (
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpenAIAutoResetCreditEnabledExtraKey     = "auto_reset_credit_enabled" // legacy: migrate to 7d only
	OpenAIAutoResetCredit5hEnabledExtraKey   = "auto_reset_credit_5h_enabled"
	OpenAIAutoResetCredit7dEnabledExtraKey   = "auto_reset_credit_7d_enabled"
	OpenAIAutoResetCredit5hThresholdExtraKey = "auto_reset_credit_5h_threshold" // legacy
	OpenAIAutoResetCredit7dThresholdExtraKey = "auto_reset_credit_7d_threshold" // legacy
	OpenAIAutoResetCreditStateExtraKey       = "codex_auto_reset_credit_state"
)

// Each window must be explicitly enabled; legacy settings only opt in to the 7d window.
type OpenAIAutoResetCreditConfig struct {
	Enabled   bool
	Enabled5h bool
	Enabled7d bool
}

func ResolveOpenAIAutoResetCreditConfig(account *Account) OpenAIAutoResetCreditConfig {
	var config OpenAIAutoResetCreditConfig
	if !isOpenAIAutoResetCreditAccount(account) || account.Extra == nil {
		return config
	}
	config.Enabled5h = resolveAccountExtraBool(account.Extra, OpenAIAutoResetCredit5hEnabledExtraKey)
	if value, ok := account.Extra[OpenAIAutoResetCredit7dEnabledExtraKey]; ok {
		config.Enabled7d, _ = value.(bool)
	} else {
		config.Enabled7d = resolveAccountExtraBool(account.Extra, OpenAIAutoResetCreditEnabledExtraKey)
	}
	config.Enabled = config.Enabled5h || config.Enabled7d
	return config
}

func isOpenAIAutoResetCreditAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth && !account.IsShadow()
}

// Accept legacy fields for migration, but never use legacy thresholds to redeem early.
func normalizeOpenAIAutoResetCreditExtra(platform, accountType string, isShadow bool, extra map[string]any) (map[string]any, error) {
	if extra == nil {
		return nil, nil
	}
	normalized := cloneOpenAIAutoResetExtra(extra)
	delete(normalized, OpenAIAutoResetCreditStateExtraKey)
	keys := []string{OpenAIAutoResetCredit5hEnabledExtraKey, OpenAIAutoResetCredit7dEnabledExtraKey}
	_, legacy := normalized[OpenAIAutoResetCreditEnabledExtraKey]
	hasConfig := legacy
	for _, key := range keys {
		_, ok := normalized[key]
		hasConfig = hasConfig || ok
	}
	if hasConfig && (platform != PlatformOpenAI || accountType != AccountTypeOAuth || isShadow) {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_ACCOUNT_INVALID", "automatic reset credits are only supported for OpenAI OAuth parent accounts")
	}
	for _, key := range keys {
		if value, ok := normalized[key]; ok {
			if _, valid := value.(bool); !valid {
				return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_ENABLED_INVALID", "%s must be a boolean", key)
			}
		}
	}
	if legacy {
		if _, valid := normalized[OpenAIAutoResetCreditEnabledExtraKey].(bool); !valid {
			return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_ENABLED_INVALID", "auto_reset_credit_enabled must be a boolean")
		}
		if _, explicit := normalized[OpenAIAutoResetCredit7dEnabledExtraKey]; !explicit {
			normalized[OpenAIAutoResetCredit7dEnabledExtraKey] = normalized[OpenAIAutoResetCreditEnabledExtraKey]
		}
	}
	delete(normalized, OpenAIAutoResetCreditEnabledExtraKey)
	delete(normalized, OpenAIAutoResetCredit5hThresholdExtraKey)
	delete(normalized, OpenAIAutoResetCredit7dThresholdExtraKey)
	return normalized, nil
}

func stripOpenAIAutoResetCreditManagedExtra(extra map[string]any, stripConfig bool) map[string]any {
	if extra == nil {
		return nil
	}
	delete(extra, OpenAIAutoResetCreditStateExtraKey)
	if stripConfig {
		delete(extra, OpenAIAutoResetCreditEnabledExtraKey)
		delete(extra, OpenAIAutoResetCredit5hEnabledExtraKey)
		delete(extra, OpenAIAutoResetCredit7dEnabledExtraKey)
		delete(extra, OpenAIAutoResetCredit5hThresholdExtraKey)
		delete(extra, OpenAIAutoResetCredit7dThresholdExtraKey)
	}
	return extra
}

func cloneOpenAIAutoResetExtra(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
