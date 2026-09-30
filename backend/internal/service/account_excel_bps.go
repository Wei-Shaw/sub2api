package service

import "strings"

// Excel / Basispoints (BPS) account switches. Ported from ranxi2001/sub2api;
// all settings live in accounts.extra so no migration is needed.
const (
	ExcelBPSEnabledKey                = "openai_excel_bps"
	ExcelBPSModelsKey                 = "openai_excel_bps_models"
	ExcelBPSIgnoreImagesKey           = "openai_excel_bps_ignore_images"
	ExcelBPSIgnoreEncryptedContentKey = "openai_excel_bps_ignore_encrypted_content"
	ExcelBPSCacheCreationAsInputKey   = "openai_excel_bps_cache_creation_as_input"
	ExcelBPSAutoDisableOn403Key       = "openai_excel_bps_auto_disable_on_403"
	// ExcelBPS403DisabledAtKey records (RFC 3339, UTC) when an upstream 403
	// automatically turned BPS off. Only DisableExcelBPSOn403 writes it.
	ExcelBPS403DisabledAtKey = "openai_excel_bps_403_disabled_at"
)

// IsExcelBPSEnabled routes an existing ChatGPT OAuth account to the Excel
// gateway. Credentials and refresh remain on the original account.
func (a *Account) IsExcelBPSEnabled() bool {
	if a == nil || a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth || a.IsShadow() || a.IsOpenAIAgentIdentity() || a.IsOpenAIPersonalAccessToken() {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(a.GetCredential("plan_type")), "free") {
		return false
	}
	return a.excelBPSFlag(ExcelBPSEnabledKey)
}

func (a *Account) excelBPSFlag(key string) bool {
	enabled, _ := a.Extra[key].(bool)
	return enabled
}

// IsExcelBPSIgnoreImagesEnabled opts into text-only forwarding.
func (a *Account) IsExcelBPSIgnoreImagesEnabled() bool {
	return a.IsExcelBPSEnabled() && a.excelBPSFlag(ExcelBPSIgnoreImagesKey)
}

// IsExcelBPSIgnoreEncryptedContentEnabled replaces ciphertext BPS cannot
// forward (e.g. old Codex sub-agent messages) with an omission notice instead
// of rejecting the whole request.
func (a *Account) IsExcelBPSIgnoreEncryptedContentEnabled() bool {
	return a.IsExcelBPSEnabled() && a.excelBPSFlag(ExcelBPSIgnoreEncryptedContentKey)
}

// IsExcelBPSCacheCreationAsInputEnabled bills cache writes as ordinary input.
func (a *Account) IsExcelBPSCacheCreationAsInputEnabled() bool {
	return a.IsExcelBPSEnabled() && a.excelBPSFlag(ExcelBPSCacheCreationAsInputKey)
}

// IsExcelBPSAutoDisableOn403Enabled opts into disabling BPS after a generic 403.
func (a *Account) IsExcelBPSAutoDisableOn403Enabled() bool {
	return a.IsExcelBPSEnabled() && a.excelBPSFlag(ExcelBPSAutoDisableOn403Key)
}

// isExcelBPSAllModelsEnabled: no model list means every model uses BPS. An
// explicit list, even an empty one, never enables BPS for all models.
func (a *Account) isExcelBPSAllModelsEnabled() bool {
	if !a.IsExcelBPSEnabled() {
		return false
	}
	_, scoped := a.Extra[ExcelBPSModelsKey]
	return !scoped
}

// IsExcelBPSEnabledForModel selects the protocol after account model mapping.
// The list selects a protocol; it does not restrict access to other models.
func (a *Account) IsExcelBPSEnabledForModel(requestedModel string) bool {
	if !a.IsExcelBPSEnabled() {
		return false
	}
	return a.isExcelBPSUpstreamModelEnabled(a.GetMappedModel(requestedModel))
}

func (a *Account) isExcelBPSUpstreamModelEnabled(model string) bool {
	if !a.IsExcelBPSEnabled() {
		return false
	}
	raw, scoped := a.Extra[ExcelBPSModelsKey]
	if !scoped {
		return true
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	switch models := raw.(type) {
	case []string:
		for _, selected := range models {
			if strings.TrimSpace(selected) == model {
				return true
			}
		}
	case []any:
		for _, selected := range models {
			if name, ok := selected.(string); ok && strings.TrimSpace(name) == model {
				return true
			}
		}
	}
	return false
}
