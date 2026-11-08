package service

import (
	"fmt"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
)

type bulkOpenAISettings struct {
	longContextBilling      bool
	endpointCapabilities    bool
	responsesMode           bool
	capabilitiesIncludeChat bool
	forcedResponsesMode     bool
}

func (s bulkOpenAISettings) any() bool {
	return s.longContextBilling || s.endpointCapabilities || s.responsesMode
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

	values := make([]string, 0, 2)
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

	selected := make(map[string]bool, 2)
	for _, value := range values {
		switch OpenAIEndpointCapability(value) {
		case OpenAIEndpointCapabilityChatCompletions, OpenAIEndpointCapabilityEmbeddings:
			selected[value] = true
		default:
			return nil, false, invalidBulkOpenAIEndpointCapabilities()
		}
	}
	if len(selected) == 0 {
		return nil, false, invalidBulkOpenAIEndpointCapabilities()
	}

	includeChat := selected[string(OpenAIEndpointCapabilityChatCompletions)]
	if includeChat && selected[string(OpenAIEndpointCapabilityEmbeddings)] {
		return nil, true, nil
	}
	if includeChat {
		return []string{string(OpenAIEndpointCapabilityChatCompletions)}, true, nil
	}
	return []string{string(OpenAIEndpointCapabilityEmbeddings)}, false, nil
}

func invalidBulkOpenAIEndpointCapabilities() error {
	return infraerrors.BadRequest(
		"OPENAI_ENDPOINT_CAPABILITIES_INVALID",
		"openai_capabilities must contain chat_completions, embeddings, or both",
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
