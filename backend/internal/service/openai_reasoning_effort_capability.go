package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const unsupportedOpenAIReasoningEffortCode = "unsupported_reasoning_effort"

// UnsupportedOpenAIReasoningEffortError indicates that an account's final
// upstream model cannot serve the requested reasoning effort.
type UnsupportedOpenAIReasoningEffortError struct {
	Effort        string
	UpstreamModel string
}

func (e *UnsupportedOpenAIReasoningEffortError) Error() string {
	return fmt.Sprintf(
		"reasoning effort %q is only supported when the final upstream model is %q or %q; resolved model was %q",
		e.Effort,
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		e.UpstreamModel,
	)
}

func (e *UnsupportedOpenAIReasoningEffortError) Code() string {
	return unsupportedOpenAIReasoningEffortCode
}

func openAIRequestHasUltraReasoning(body []byte) bool {
	for _, path := range []string{"reasoning.effort", "reasoning_effort", "output_config.effort"} {
		value := gjson.GetBytes(body, path)
		if value.Exists() && value.Type == gjson.String && NormalizeMaxReasoningEffort(value.String()) == "ultra" {
			return true
		}
	}
	return false
}

func isOpenAIUltraReasoningModel(upstreamModel string) bool {
	switch strings.TrimSpace(upstreamModel) {
	case "gpt-5.6-sol", "gpt-5.6-terra":
		return true
	default:
		return false
	}
}

func supportsOpenAIUltraReasoning(account *Account, upstreamModel string) bool {
	return account != nil && account.Platform == PlatformOpenAI && isOpenAIUltraReasoningModel(upstreamModel)
}

func validateOpenAIReasoningEffortForUpstream(account *Account, upstreamModel string, body []byte) error {
	if !openAIRequestHasUltraReasoning(body) || supportsOpenAIUltraReasoning(account, upstreamModel) {
		return nil
	}
	return &UnsupportedOpenAIReasoningEffortError{
		Effort:        "ultra",
		UpstreamModel: strings.TrimSpace(upstreamModel),
	}
}

func normalizeAndValidateOpenAIReasoningEffortForUpstream(account *Account, upstreamModel string, body []byte) ([]byte, error) {
	if account == nil || account.Platform != PlatformOpenAI {
		return body, nil
	}

	normalized := body
	for _, path := range []string{"reasoning.effort", "reasoning_effort", "output_config.effort"} {
		value := gjson.GetBytes(normalized, path)
		if !value.Exists() || value.Type != gjson.String || NormalizeMaxReasoningEffort(value.String()) != "ultra" || value.String() == "ultra" {
			continue
		}
		next, err := sjson.SetBytes(normalized, path, "ultra")
		if err != nil {
			return body, fmt.Errorf("normalize OpenAI reasoning effort %s: %w", path, err)
		}
		normalized = next
	}
	if err := validateOpenAIReasoningEffortForUpstream(account, upstreamModel, normalized); err != nil {
		return body, err
	}
	return normalized, nil
}

// ValidateOpenAIReasoningEffortForAccount validates the request after applying
// the selected account's model mapping and upstream model normalization.
func ValidateOpenAIReasoningEffortForAccount(account *Account, requestedModel string, body []byte) error {
	if account == nil {
		return nil
	}
	upstreamModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(requestedModel))
	return validateOpenAIReasoningEffortForUpstream(account, upstreamModel, body)
}

func buildUnsupportedOpenAIReasoningEffortWSEvent(err *UnsupportedOpenAIReasoningEffortError) []byte {
	if err == nil {
		return nil
	}
	return []byte(fmt.Sprintf(
		`{"type":"error","error":{"type":"invalid_request_error","code":%q,"message":%q}}`,
		err.Code(),
		err.Error(),
	))
}
