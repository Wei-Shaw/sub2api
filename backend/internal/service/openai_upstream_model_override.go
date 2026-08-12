package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAISolModel   = "gpt-5.6-sol"
	openAITerraModel = "gpt-5.6-terra"
)

func openAIFinalUpstreamModel(account *Account, model string) string {
	if account == nil || account.Platform != PlatformOpenAI {
		return model
	}
	return openAIFinalOpenAIModel(model)
}

func openAIFinalOpenAIModel(model string) string {
	if strings.TrimSpace(model) == openAISolModel {
		return openAITerraModel
	}
	return model
}

// rewriteOpenAIFinalUpstreamBody applies wire-only model overrides. Callers keep
// using the original body/model for billing and usage metadata.
func rewriteOpenAIFinalUpstreamBody(account *Account, body []byte) []byte {
	if account == nil || account.Platform != PlatformOpenAI {
		return body
	}
	return rewriteOpenAIFinalOpenAIUpstreamBody(body)
}

func rewriteOpenAIFinalOpenAIUpstreamBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	rewritten := body
	for _, path := range []string{"model", "session.model"} {
		model := gjson.GetBytes(rewritten, path)
		if model.Type != gjson.String || strings.TrimSpace(model.String()) != openAISolModel {
			continue
		}
		next, err := sjson.SetBytes(rewritten, path, openAITerraModel)
		if err == nil {
			rewritten = next
		}
	}
	return rewritten
}

func restoreOpenAIFinalOpenAIResponseBody(logicalModel string, body []byte) []byte {
	if strings.TrimSpace(logicalModel) != openAISolModel || len(body) == 0 {
		return body
	}

	restored := body
	for _, path := range []string{"model", "response.model", "session.model"} {
		model := gjson.GetBytes(restored, path)
		if model.Type != gjson.String || strings.TrimSpace(model.String()) != openAITerraModel {
			continue
		}
		next, err := sjson.SetBytes(restored, path, openAISolModel)
		if err == nil {
			restored = next
		}
	}
	return restored
}

func rewriteOpenAIFinalUpstreamPayload(account *Account, payload map[string]any) {
	if account == nil || account.Platform != PlatformOpenAI || len(payload) == 0 {
		return
	}
	if model, ok := payload["model"].(string); ok {
		payload["model"] = openAIFinalUpstreamModel(account, model)
	}
	if session, ok := payload["session"].(map[string]any); ok {
		if model, ok := session["model"].(string); ok {
			wireModel := openAIFinalUpstreamModel(account, model)
			if wireModel != model {
				wireSession := make(map[string]any, len(session))
				for key, value := range session {
					wireSession[key] = value
				}
				wireSession["model"] = wireModel
				payload["session"] = wireSession
			}
		}
	}
}

func usesOpenAISolToTerraOverride(account *Account, input *OpenAIRecordUsageInput) bool {
	if account == nil || account.Platform != PlatformOpenAI || input == nil || input.Result == nil {
		return false
	}
	return strings.TrimSpace(input.Result.UpstreamModel) == openAISolModel
}
