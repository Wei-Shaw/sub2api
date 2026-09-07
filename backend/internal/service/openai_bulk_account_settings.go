package service

import (
	"fmt"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
)

type bulkOpenAISettings struct {
	longContextBilling          bool
	endpointCapabilities        bool
	genericEndpointCapabilities bool
	responsesMode               bool
	capabilitiesIncludeChat     bool
	forcedResponsesMode         bool
}

func (s bulkOpenAISettings) any() bool {
	return s.longContextBilling || s.endpointCapabilities || s.genericEndpointCapabilities || s.responsesMode
}

func normalizeBulkOpenAISettings(input *BulkUpdateAccountsInput) (bulkOpenAISettings, error) {
	var settings bulkOpenAISettings
	if input == nil {
		return settings, nil
	}

	if _, exists := input.Extra[openAILongContextBillingEnabledKey]; exists {
		settings.longContextBilling = true
		if err := ValidateOpenAILongContextBillingExtra(PlatformOpenAI, input.Extra); err != nil {
			return settings, err
		}
	}

	if raw, exists := input.Credentials[openAIEndpointCapabilitiesCredentialKey]; exists {
		settings.endpointCapabilities = true
		capabilities, includeChat, err := normalizeBulkOpenAIEndpointCapabilities(raw)
		if err != nil {
			return settings, err
		}
		settings.capabilitiesIncludeChat = includeChat
		input.Credentials[openAIEndpointCapabilitiesCredentialKey] = capabilities
	}
	if _, exists := input.Credentials[endpointCapabilitiesCredentialKey]; exists {
		// The provider-neutral key is normalized only after the target accounts
		// are loaded: Gemini and Zhipu deliberately have different capability
		// vocabularies, and Gemini's embedding option depends on account type.
		settings.genericEndpointCapabilities = true
	}

	if raw, exists := input.Extra[openai_compat.ExtraKeyResponsesMode]; exists {
		settings.responsesMode = true
		mode, forced, err := normalizeBulkOpenAIResponsesMode(raw)
		if err != nil {
			return settings, err
		}
		settings.forcedResponsesMode = forced
		input.Extra[openai_compat.ExtraKeyResponsesMode] = mode
	}

	if settings.endpointCapabilities && !settings.capabilitiesIncludeChat {
		if settings.forcedResponsesMode {
			return settings, infraerrors.BadRequest(
				"OPENAI_RESPONSES_MODE_INVALID",
				"a forced Responses route requires the chat_completions endpoint capability",
			)
		}
		if input.Extra == nil {
			input.Extra = make(map[string]any, 1)
		}
		input.Extra[openai_compat.ExtraKeyResponsesMode] = nil
		settings.responsesMode = true
	}

	return settings, nil
}

func normalizeBulkOpenAIEndpointCapabilities(raw any) (any, bool, error) {
	if raw == nil {
		return nil, true, nil
	}

	values := make([]string, 0, 3)
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, false, invalidBulkOpenAIEndpointCapabilities()
			}
			values = append(values, value)
		}
	case []string:
		values = append(values, typed...)
	default:
		return nil, false, invalidBulkOpenAIEndpointCapabilities()
	}

	selected := make(map[string]bool, 3)
	for _, value := range values {
		switch OpenAIEndpointCapability(value) {
		case OpenAIEndpointCapabilityChatCompletions, OpenAIEndpointCapabilityEmbeddings, OpenAIEndpointCapabilityRerank:
			selected[value] = true
		default:
			return nil, false, invalidBulkOpenAIEndpointCapabilities()
		}
	}
	if len(selected) == 0 {
		return nil, false, invalidBulkOpenAIEndpointCapabilities()
	}

	includeChat := selected[string(OpenAIEndpointCapabilityChatCompletions)]
	normalized := make([]string, 0, len(selected))
	for _, capability := range []OpenAIEndpointCapability{
		OpenAIEndpointCapabilityChatCompletions,
		OpenAIEndpointCapabilityEmbeddings,
		OpenAIEndpointCapabilityRerank,
	} {
		if selected[string(capability)] {
			normalized = append(normalized, string(capability))
		}
	}
	return normalized, includeChat, nil
}

func invalidBulkOpenAIEndpointCapabilities() error {
	return infraerrors.BadRequest(
		"OPENAI_ENDPOINT_CAPABILITIES_INVALID",
		"openai_capabilities must contain chat_completions, embeddings, rerank, or a supported combination",
	)
}

func normalizeBulkGeminiEndpointCapabilities(raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	values := make([]string, 0, 2)
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, invalidBulkGeminiEndpointCapabilities()
			}
			values = append(values, value)
		}
	case []string:
		values = append(values, typed...)
	default:
		return nil, invalidBulkGeminiEndpointCapabilities()
	}
	selected := make(map[string]bool, 2)
	for _, value := range values {
		switch value {
		case "gemini_native", string(OpenAIEndpointCapabilityEmbeddings):
			selected[value] = true
		default:
			return nil, invalidBulkGeminiEndpointCapabilities()
		}
	}
	if len(selected) == 0 {
		return nil, invalidBulkGeminiEndpointCapabilities()
	}
	normalized := make([]string, 0, len(selected))
	for _, capability := range []string{"gemini_native", string(OpenAIEndpointCapabilityEmbeddings)} {
		if selected[capability] {
			normalized = append(normalized, capability)
		}
	}
	return normalized, nil
}

func normalizeBulkZhipuEndpointCapabilities(raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	values, err := bulkEndpointCapabilityStrings(raw)
	if err != nil {
		return nil, invalidBulkZhipuEndpointCapabilities()
	}
	selected := make(map[string]bool, 2)
	for _, value := range values {
		switch OpenAIEndpointCapability(value) {
		case OpenAIEndpointCapabilityChatCompletions, OpenAIEndpointCapabilityEmbeddings:
			selected[value] = true
		default:
			return nil, invalidBulkZhipuEndpointCapabilities()
		}
	}
	if len(selected) == 0 {
		return nil, invalidBulkZhipuEndpointCapabilities()
	}
	normalized := make([]string, 0, len(selected))
	for _, capability := range []OpenAIEndpointCapability{
		OpenAIEndpointCapabilityChatCompletions,
		OpenAIEndpointCapabilityEmbeddings,
	} {
		if selected[string(capability)] {
			normalized = append(normalized, string(capability))
		}
	}
	return normalized, nil
}

func bulkEndpointCapabilityStrings(raw any) ([]string, error) {
	values := make([]string, 0, 2)
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("endpoint capability must be a string")
			}
			values = append(values, value)
		}
	case []string:
		values = append(values, typed...)
	default:
		return nil, fmt.Errorf("endpoint capabilities must be an array")
	}
	return values, nil
}

func invalidBulkZhipuEndpointCapabilities() error {
	return infraerrors.BadRequest(
		"ZHIPU_ENDPOINT_CAPABILITIES_INVALID",
		"endpoint_capabilities must contain chat_completions, embeddings, or both",
	)
}

// normalizeBulkGenericEndpointCapabilities resolves the provider-neutral
// credentials key after target loading. Bulk edits must be homogeneous here so
// a single JSONB patch never gives a provider another provider's vocabulary.
func normalizeBulkGenericEndpointCapabilities(input *BulkUpdateAccountsInput, settings bulkOpenAISettings, targetsByID map[int64]*Account) error {
	if input == nil || !settings.genericEndpointCapabilities {
		return nil
	}
	platform := ""
	geminiHasNonAPIKey := false
	for _, accountID := range input.AccountIDs {
		account := targetsByID[accountID]
		if account == nil {
			return invalidBulkOpenAITarget(accountID, "account does not exist")
		}
		if platform == "" {
			platform = account.Platform
		} else if account.Platform != platform {
			return invalidBulkOpenAITarget(accountID, "endpoint capabilities require homogeneous Gemini or Zhipu targets")
		}
		if account.Platform == PlatformGemini && account.Type != AccountTypeAPIKey {
			geminiHasNonAPIKey = true
		}
	}
	raw := input.Credentials[endpointCapabilitiesCredentialKey]
	switch platform {
	case PlatformGemini:
		capabilities, err := normalizeBulkGeminiEndpointCapabilities(raw)
		if err != nil {
			return err
		}
		if geminiHasNonAPIKey && capabilities != nil {
			values := capabilities.([]string)
			sanitized := make([]string, 0, 1)
			for _, value := range values {
				if value != string(OpenAIEndpointCapabilityEmbeddings) {
					sanitized = append(sanitized, value)
				}
			}
			if len(sanitized) == 0 {
				input.Credentials[endpointCapabilitiesCredentialKey] = nil
				return nil
			}
			capabilities = sanitized
		}
		input.Credentials[endpointCapabilitiesCredentialKey] = capabilities
	case PlatformZhipu:
		capabilities, err := normalizeBulkZhipuEndpointCapabilities(raw)
		if err != nil {
			return err
		}
		input.Credentials[endpointCapabilitiesCredentialKey] = capabilities
	default:
		return invalidBulkOpenAITarget(input.AccountIDs[0], "endpoint capabilities require homogeneous Gemini or Zhipu targets")
	}
	return nil
}

func invalidBulkGeminiEndpointCapabilities() error {
	return infraerrors.BadRequest(
		"GEMINI_ENDPOINT_CAPABILITIES_INVALID",
		"endpoint_capabilities must contain gemini_native, embeddings, or both",
	)
}

func normalizeBulkOpenAIResponsesMode(raw any) (any, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	mode, ok := raw.(string)
	if !ok {
		return nil, false, invalidBulkOpenAIResponsesMode()
	}
	switch openai_compat.ResponsesSupportMode(mode) {
	case openai_compat.ResponsesSupportModeAuto:
		return nil, false, nil
	case openai_compat.ResponsesSupportModeForceResponses,
		openai_compat.ResponsesSupportModeForceChatCompletions:
		return mode, true, nil
	default:
		return nil, false, invalidBulkOpenAIResponsesMode()
	}
}

func invalidBulkOpenAIResponsesMode() error {
	return infraerrors.BadRequest(
		"OPENAI_RESPONSES_MODE_INVALID",
		"openai_responses_mode must be auto, force_responses, force_chat_completions, or null",
	)
}

func validateBulkOpenAISettingsTargets(
	input *BulkUpdateAccountsInput,
	settings bulkOpenAISettings,
	targetsByID map[int64]*Account,
) (int, error) {
	if input == nil || !settings.any() {
		return 0, nil
	}

	inheritedCount := 0
	for _, accountID := range input.AccountIDs {
		account, ok := targetsByID[accountID]
		if !ok || account == nil {
			return 0, invalidBulkOpenAITarget(accountID, "account does not exist")
		}

		if settings.longContextBilling {
			if account.Platform != PlatformOpenAI || !supportsOpenAILongContextBilling(account.Type) {
				return 0, invalidBulkOpenAITarget(accountID, "long-context billing requires an OpenAI OAuth, setup-token, or API-key account")
			}
			if account.IsShadow() {
				inheritedCount++
			}
		}

		if settings.endpointCapabilities || settings.responsesMode {
			if account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
				return 0, invalidBulkOpenAITarget(accountID, "endpoint capabilities and Responses routing require an OpenAI API-key account")
			}
		}
		if settings.genericEndpointCapabilities {
			if account.Platform != PlatformGemini && account.Platform != PlatformZhipu {
				return 0, invalidBulkOpenAITarget(accountID, "endpoint capabilities require a Gemini or Zhipu account")
			}
		}

		if settings.forcedResponsesMode && !settings.capabilitiesIncludeChat &&
			!settings.endpointCapabilities &&
			!account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions) {
			return 0, invalidBulkOpenAITarget(accountID, "a forced Responses route requires the chat_completions endpoint capability")
		}
	}

	if settings.longContextBilling && inheritedCount == len(input.AccountIDs) && bulkUpdateOnlyChangesLongContext(input) {
		return 0, infraerrors.BadRequest(
			"OPENAI_LONG_CONTEXT_PARENT_REQUIRED",
			"long-context billing is owned by parent accounts; select at least one parent account",
		)
	}
	return inheritedCount, nil
}

func supportsOpenAILongContextBilling(accountType string) bool {
	switch accountType {
	case AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey:
		return true
	default:
		return false
	}
}

func invalidBulkOpenAITarget(accountID int64, message string) error {
	return infraerrors.BadRequest(
		"OPENAI_BULK_TARGET_INVALID",
		fmt.Sprintf("account %d: %s", accountID, message),
	).WithMetadata(map[string]string{"account_id": strconv.FormatInt(accountID, 10)})
}

func bulkUpdateOnlyChangesLongContext(input *BulkUpdateAccountsInput) bool {
	if input == nil || input.Name != "" || input.ProxyID != nil || input.Concurrency != nil ||
		input.Priority != nil || input.RateMultiplier != nil || input.LoadFactor != nil ||
		input.Status != "" || input.Schedulable != nil || input.GroupIDs != nil ||
		len(input.Credentials) != 0 || input.ProbeEnabled != nil {
		return false
	}
	if len(input.Extra) != 1 {
		return false
	}
	_, ok := input.Extra[openAILongContextBillingEnabledKey]
	return ok
}
