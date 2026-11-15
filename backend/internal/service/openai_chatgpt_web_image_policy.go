package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

const AccountExtraChatGPTWebImageDefaultModel = "chatgpt_web_image_default_model"

// A model here is a compatibility entry. The web provider
// selects its own image backend; this is never evidence of its actual model ID.
type chatGPTWebImageModel struct {
	RequestedModel string
	MappedModel    string
}

func isFreeChatGPTWebImageAccount(account *Account) bool {
	return account != nil && account.IsOpenAIOAuth() && strings.EqualFold(strings.TrimSpace(account.GetCredential("plan_type")), "free")
}

// Every Free image request follows the web policy. Missing or disabled opt-in
// is denied instead of falling back to the unsupported native Codex image path.
func UsesChatGPTWebImagePolicy(account *Account) bool {
	return isFreeChatGPTWebImageAccount(account) || chatGPTWebImageEnabled(account)
}

func chatGPTWebDefaultImageModel(_ *Account) string {
	return "gpt-image-2"
}

// Native image names are web compatibility entries, independent of the chat
// allowlist or legacy image remapping. Existing aliases remain usable only when
// their explicit account mapping points at an image model.
func chatGPTWebImageModelID(account *Account, requested string) (string, bool) {
	requested = strings.TrimSpace(requested)
	if IsGPTImageGenerationModel(requested) {
		return requested, true
	}
	if account != nil {
		if mapped, matched := account.ResolveMappedModel(requested); matched && IsGPTImageGenerationModel(mapped) {
			return strings.TrimSpace(mapped), true
		}
	}
	return "", false
}

func resolveChatGPTWebImageModel(account *Account, requested string) (chatGPTWebImageModel, error) {
	if err := validateChatGPTWebImageAccount(account); err != nil {
		return chatGPTWebImageModel{}, err
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = chatGPTWebDefaultImageModel(account)
	}
	mapped, image := chatGPTWebImageModelID(account, requested)
	if !image {
		return chatGPTWebImageModel{}, chatGPTWebImageRequestError("chatgpt_web_image_model_not_allowed", "The requested model is not an image compatibility entry or an image alias", "model")
	}
	return chatGPTWebImageModel{RequestedModel: requested, MappedModel: strings.TrimSpace(mapped)}, nil
}

type chatGPTWebImageSchedulingModelContextKey struct{}

func chatGPTWebImageSchedulingModel(ctx context.Context, requested string) string {
	if IsGPTImageGenerationModel(requested) {
		return requested
	}
	if ctx != nil {
		if mapped, ok := ctx.Value(chatGPTWebImageSchedulingModelContextKey{}).(string); ok && mapped != "" {
			requested = mapped
		}
	}
	return requested
}

// Scheduling and forwarding share the account switch and consent policy.
func chatGPTWebImageSchedulingFailureReason(ctx context.Context, account *Account, requested string) string {
	if !UsesChatGPTWebImagePolicy(account) {
		return ""
	}
	requested = chatGPTWebImageSchedulingModel(ctx, requested)
	if _, err := resolveChatGPTWebImageModel(account, requested); err != nil {
		if publicErr, ok := err.(*OpenAIImagesUpstreamError); ok {
			return publicErr.Code
		}
		return "chatgpt_web_image_unavailable"
	}
	return ""
}

func chatGPTWebImageOwnsNativeModel(account *Account, model string) bool {
	return isFreeChatGPTWebImageAccount(account) && IsGPTImageGenerationModel(model) && validateChatGPTWebImageAccount(account) == nil
}

func isOpenAIImageToolDeclaration(raw any) bool {
	tool, _ := raw.(map[string]any)
	name, _ := tool["name"].(string)
	namespace, _ := tool["namespace"].(string)
	return tool["type"] == "image_generation" ||
		(tool["type"] == "namespace" && name == "image_gen") ||
		isOpenAIImageGenFunctionReference(namespace, name)
}

// PrepareOpenAIImageDisabledRequest keeps ordinary HTTP chat usable when image
// tools are declared by the client on every turn. Explicit selection is denied;
// passive image declarations are removed so no upstream (including Pro) can
// execute them despite the group's disabled image permission.
func PrepareOpenAIImageDisabledRequest(body []byte) ([]byte, error) {
	denied := func() ([]byte, error) {
		return nil, chatGPTWebImageRequestError("image_generation_disabled", ImageGenerationPermissionMessage(), "tools")
	}
	if IsGPTImageGenerationModel(gjson.GetBytes(body, "model").String()) ||
		openAIJSONToolChoiceSelectsExplicitImageGeneration(gjson.GetBytes(body, "tool_choice")) {
		return denied()
	}
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, err
	}
	removed, remaining := 0, 0
	filter := func(raw any) []any {
		tools, _ := raw.([]any)
		kept := make([]any, 0, len(tools))
		for _, tool := range tools {
			if isOpenAIImageToolDeclaration(tool) {
				removed++
				continue
			}
			remaining++
			kept = append(kept, tool)
		}
		return kept
	}
	if raw, exists := request["tools"]; exists {
		request["tools"] = filter(raw)
	}
	if input, ok := request["input"].([]any); ok {
		for _, raw := range input {
			if item, ok := raw.(map[string]any); ok && item["type"] == "additional_tools" {
				item["tools"] = filter(item["tools"])
			}
		}
	}
	if removed == 0 {
		return body, nil
	}
	if request["tool_choice"] == "required" && remaining == 0 {
		return denied()
	}
	return json.Marshal(request)
}
