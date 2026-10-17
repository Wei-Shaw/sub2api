package service

import (
	"encoding/json"
	"slices"
	"strings"
)

// SubscriptionModelPolicy is also included in the generated platform catalog.
// Keep credential scope and retired families in one source for routing and forms.
// A family includes its exact ID and variants separated by a hyphen.
type SubscriptionModelPolicy struct {
	AccountTypes             []string `json:"account_types"`
	IncludeCredentialShadows bool     `json:"include_credential_shadows"`
	RetiredFamilies          []string `json:"retired_families"`
}

func openAISubscriptionModelPolicy() *SubscriptionModelPolicy {
	return &SubscriptionModelPolicy{
		AccountTypes:             []string{AccountTypeOAuth, AccountTypeSetupToken},
		IncludeCredentialShadows: true,
		RetiredFamilies:          []string{"gpt-5.5"},
	}
}

// GPT-5.5 retired from Codex with ChatGPT sign-in on 2026-10-14 at
// 19:00 UTC. API-key upstreams remain supported. Do not infer upstream
// eligibility from the API key a client uses to authenticate to this gateway.
// https://learn.chatgpt.com/docs/models#gpt-55-retirement
func isRetiredOpenAISubscriptionModel(account *Account, upstreamModel string) bool {
	if account == nil || !account.IsOpenAI() {
		return false
	}
	policy := openAISubscriptionModelPolicy()
	if !slices.Contains(policy.AccountTypes, account.Type) && (!policy.IncludeCredentialShadows || !account.IsShadow()) {
		return false
	}
	model := canonicalizeOpenAIModelAliasSpelling(upstreamModel)
	for _, family := range policy.RetiredFamilies {
		if model == family || strings.HasPrefix(model, family+"-") {
			return true
		}
	}
	return false
}

// Only remove entries blocked by retirement. Unknown models and other platforms
// retain their existing listing policy. An explicitly mapped replacement is
// allowed, but a public alias targeting retired GPT-5.5 is not.
func openAIModelRetiredForAccounts(accounts []Account, modelID string) bool {
	retired := false
	for i := range accounts {
		account := &accounts[i]
		if !account.IsOpenAI() {
			continue
		}
		target := resolveOpenAIAccountUpstreamModelForRequest(account, modelID, false)
		if isRetiredOpenAISubscriptionModel(account, target) {
			retired = true
		} else if account.IsModelSupported(modelID) {
			return false
		}
	}
	return retired
}

func filterRetiredOpenAIModelIDs(accounts []Account, models []string) []string {
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if !openAIModelRetiredForAccounts(accounts, model) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

// Discovery returns upstream IDs, not public aliases. Do not apply account
// mappings here: a replacement alias must not make a retired source ID live.
func filterRetiredOpenAIUpstreamModelIDs(account *Account, models []string) []string {
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if !isRetiredOpenAISubscriptionModel(account, model) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

// Entries must already use client-visible names, not raw upstream IDs. Resolve
// each public name exactly as routing does, after projection and before the
// final client ETag is calculated. Preserve unknown fields.
func filterRetiredOpenAIPublicModelsBody(body []byte, accounts []Account, codex bool) ([]byte, bool, error) {
	field, idField := "data", "id"
	if codex {
		field, idField = "models", "slug"
	}
	envelope, entries, err := modelCatalogEntries(body, field)
	if err != nil {
		return nil, false, err
	}
	filtered := make([]json.RawMessage, 0, len(entries))
	for _, raw := range entries {
		var entry map[string]json.RawMessage
		var id string
		if json.Unmarshal(raw, &entry) == nil {
			_ = json.Unmarshal(entry[idField], &id)
		}
		if !openAIModelRetiredForAccounts(accounts, id) {
			filtered = append(filtered, raw)
		}
	}
	if len(filtered) == len(entries) {
		return body, false, nil
	}
	envelope[field], err = json.Marshal(filtered)
	if err != nil {
		return nil, false, err
	}
	encoded, err := json.Marshal(envelope)
	return encoded, true, err
}

// Compact-only administrator mappings are evaluated against their actual
// target, without treating an old client-visible name as a retired target.
func supportsOpenAIRequestModel(account *Account, requestedModel string, requireCompact bool) bool {
	if isRetiredOpenAISubscriptionModel(account, resolveOpenAIAccountUpstreamModelForRequest(account, requestedModel, requireCompact)) {
		return false
	}
	return account.isModelMappingSupported(requestedModel)
}
