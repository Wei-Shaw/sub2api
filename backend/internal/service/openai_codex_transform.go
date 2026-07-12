package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

var codexModelMap = map[string]string{
	"gpt-5.6-sol":          "gpt-5.6-sol",
	"gpt-5.6-terra":        "gpt-5.6-terra",
	"gpt-5.6-luna":         "gpt-5.6-luna",
	"gpt-5.5":              "gpt-5.5",
	"gpt-5.5-pro":          "gpt-5.5-pro",
	"codex-auto-review":    "codex-auto-review",
	"gpt-5.4":              "gpt-5.4",
	"gpt-5.4-mini":         "gpt-5.4-mini",
	"gpt-5.4-none":         "gpt-5.4",
	"gpt-5.4-low":          "gpt-5.4",
	"gpt-5.4-medium":       "gpt-5.4",
	"gpt-5.4-high":         "gpt-5.4",
	"gpt-5.4-xhigh":        "gpt-5.4",
	"gpt-5.4-chat-latest":  "gpt-5.4",
	"gpt-5.3":              "gpt-5.3-codex",
	"gpt-5.3-none":         "gpt-5.3-codex",
	"gpt-5.3-low":          "gpt-5.3-codex",
	"gpt-5.3-medium":       "gpt-5.3-codex",
	"gpt-5.3-high":         "gpt-5.3-codex",
	"gpt-5.3-xhigh":        "gpt-5.3-codex",
	"gpt-5.3-codex":        "gpt-5.3-codex",
	"gpt-5.3-codex-spark":  "gpt-5.3-codex-spark",
	"gpt-5.3-codex-low":    "gpt-5.3-codex",
	"gpt-5.3-codex-medium": "gpt-5.3-codex",
	"gpt-5.3-codex-high":   "gpt-5.3-codex",
	"gpt-5.3-codex-xhigh":  "gpt-5.3-codex",
	"gpt-5.2":              "gpt-5.2",
	"gpt-5.2-none":         "gpt-5.2",
	"gpt-5.2-low":          "gpt-5.2",
	"gpt-5.2-medium":       "gpt-5.2",
	"gpt-5.2-high":         "gpt-5.2",
	"gpt-5.2-xhigh":        "gpt-5.2",
	"gpt-5":                "gpt-5.4",
	"gpt-5-mini":           "gpt-5.4",
	"gpt-5-nano":           "gpt-5.4",
	"gpt-5.1":              "gpt-5.4",
	"gpt-5.1-codex":        "gpt-5.3-codex",
	"gpt-5.1-codex-max":    "gpt-5.3-codex",
	"gpt-5.1-codex-mini":   "gpt-5.3-codex",
	"gpt-5.2-codex":        "gpt-5.2",
	"codex-mini-latest":    "gpt-5.3-codex",
	"gpt-5-codex":          "gpt-5.3-codex",
}

var codexVersionModelPrefixes = []struct {
	prefix string
	target string
}{
	{prefix: "gpt-5.6-sol", target: "gpt-5.6-sol"},
	{prefix: "gpt-5.6-terra", target: "gpt-5.6-terra"},
	{prefix: "gpt-5.6-luna", target: "gpt-5.6-luna"},
	{prefix: "gpt-5.3-codex-spark", target: "gpt-5.3-codex-spark"},
	{prefix: "gpt-5.3-codex", target: "gpt-5.3-codex"},
	{prefix: "gpt-5.4-mini", target: "gpt-5.4-mini"},
	{prefix: "gpt-5.4-nano", target: "gpt-5.4-nano"},
	{prefix: "gpt-5.5-pro", target: "gpt-5.5-pro"},
	{prefix: "gpt-5.5", target: "gpt-5.5"},
	{prefix: "gpt-5.4", target: "gpt-5.4"},
	{prefix: "gpt-5.2", target: "gpt-5.2"},
}

type codexTransformResult struct {
	Modified        bool
	NormalizedModel string
	PromptCacheKey  string
}

type codexOAuthTransformOptions struct {
	IsCodexCLI              bool
	IsCompact               bool
	SkipDefaultInstructions bool
	PreserveToolCallIDs     bool
}

const (
	codexImageGenerationBridgeMarker = "<sub2api-codex-image-generation>"
	codexImageGenerationBridgeText   = codexImageGenerationBridgeMarker + "\nWhen the user asks for raster image generation or editing, use the OpenAI Responses native `image_generation` tool attached to this request. The local Codex client may not expose an `image_gen` namespace, but that does not mean image generation is unavailable. Do not ask the user to switch to CLI fallback solely because `image_gen` is absent.\n</sub2api-codex-image-generation>"
	codexSparkImageUnsupportedMarker = "<sub2api-codex-spark-image-unsupported>"
	codexSparkImageUnsupportedText   = codexSparkImageUnsupportedMarker + "\nThe current model is gpt-5.3-codex-spark, which does not support image generation, image editing, image input, the `image_generation` tool, or Codex `image_gen`/`$imagegen` workflows. If the user asks for image generation or image editing, clearly explain this model limitation and ask them to switch to a non-Spark Codex model such as gpt-5.3-codex or gpt-5.4. Do not claim that the local environment merely lacks image_gen tooling, and do not suggest CLI fallback as the primary fix while the model remains Spark.\n</sub2api-codex-spark-image-unsupported>"
)

var openAIChatGPTInternalUnsupportedFields = []string{
	"user",
	"metadata",
	"prompt_cache_retention",
	"safety_identifier",
	"stream_options",
}

var openAICodexOAuthUnsupportedFields = append([]string{
	"max_output_tokens",
	"max_completion_tokens",
	"temperature",
	"top_p",
	"frequency_penalty",
	"presence_penalty",
}, openAIChatGPTInternalUnsupportedFields...)

func applyCodexOAuthTransform(reqBody map[string]any, isCodexCLI bool, isCompact bool) codexTransformResult {
	return applyCodexOAuthTransformWithOptions(reqBody, codexOAuthTransformOptions{
		IsCodexCLI: isCodexCLI,
		IsCompact:  isCompact,
	})
}

func applyCodexOAuthTransformWithOptions(reqBody map[string]any, opts codexOAuthTransformOptions) codexTransformResult {
	result := codexTransformResult{}
	// 工具续链需求会影响存储策略与 input 过滤逻辑。
	needsToolContinuation := NeedsToolContinuation(reqBody)

	model := ""
	if v, ok := reqBody["model"].(string); ok {
		model = v
	}
	normalizedModel := strings.TrimSpace(model)
	if normalizedModel != "" {
		if model != normalizedModel {
			reqBody["model"] = normalizedModel
			result.Modified = true
		}
		result.NormalizedModel = normalizedModel
	}

	if opts.IsCompact {
		if _, ok := reqBody["store"]; ok {
			delete(reqBody, "store")
			result.Modified = true
		}
		if _, ok := reqBody["stream"]; ok {
			delete(reqBody, "stream")
			result.Modified = true
		}
	} else {
		// OAuth 走 ChatGPT internal API 时，store 必须为 false；显式 true 也会强制覆盖。
		// 避免上游返回 "Store must be set to false"。
		if v, ok := reqBody["store"].(bool); !ok || v {
			reqBody["store"] = false
			result.Modified = true
		}
		if v, ok := reqBody["stream"].(bool); !ok || !v {
			reqBody["stream"] = true
			result.Modified = true
		}
	}

	// Strip parameters unsupported by ChatGPT internal Codex endpoint.
	for _, key := range openAICodexOAuthUnsupportedFields {
		if _, ok := reqBody[key]; ok {
			delete(reqBody, key)
			result.Modified = true
		}
	}

	// 请求带 reasoning 时补齐 include:["reasoning.encrypted_content"]，与真实 Codex 对齐
	// （compact 端点形态不同，单独处理，此处跳过）。
	if !opts.IsCompact && ensureCodexReasoningInclude(reqBody) {
		result.Modified = true
	}

	// 兼容遗留的 functions 和 function_call，转换为 tools 和 tool_choice
	if functionsRaw, ok := reqBody["functions"]; ok {
		if functions, k := functionsRaw.([]any); k {
			tools := make([]any, 0, len(functions))
			for _, f := range functions {
				tools = append(tools, map[string]any{
					"type":     "function",
					"function": f,
				})
			}
			reqBody["tools"] = tools
		}
		delete(reqBody, "functions")
		result.Modified = true
	}

	if fcRaw, ok := reqBody["function_call"]; ok {
		if fcStr, ok := fcRaw.(string); ok {
			// e.g. "auto", "none"
			reqBody["tool_choice"] = fcStr
		} else if fcObj, ok := fcRaw.(map[string]any); ok {
			// e.g. {"name": "my_func"}
			if name, ok := fcObj["name"].(string); ok && strings.TrimSpace(name) != "" {
				reqBody["tool_choice"] = map[string]any{
					"type": "function",
					"name": name,
				}
			}
		}
		delete(reqBody, "function_call")
		result.Modified = true
	}

	if normalizeCodexTools(reqBody) {
		result.Modified = true
	}
	if normalizeCodexToolChoice(reqBody) {
		result.Modified = true
	}

	if v, ok := reqBody["prompt_cache_key"].(string); ok {
		result.PromptCacheKey = strings.TrimSpace(v)
		if isOpenAICompatMessagesBridgeRequestBody(reqBody) {
			delete(reqBody, "prompt_cache_key")
			result.Modified = true
		}
	}

	// ChatGPT internal Codex endpoint does not accept role:"system".
	// Keep the guidance in input as developer for Responses JSON mode, and
	// also mirror it into instructions because Codex OAuth requires it.
	if extractSystemMessagesFromInput(reqBody) {
		result.Modified = true
	}

	// instructions 处理逻辑：根据是否是 Codex CLI 分别调用不同方法
	if !opts.SkipDefaultInstructions && applyInstructions(reqBody, opts.IsCodexCLI) {
		result.Modified = true
	}
	if isCodexSparkModel(normalizedModel) && applyCodexSparkImageUnsupportedInstructions(reqBody) {
		result.Modified = true
	}
	// gpt-5.3-codex-spark rejects the image_generation tool upstream (HTTP 400,
	// param=tools); Codex CLI advertises it by default, so strip it for spark.
	if isCodexSparkModel(normalizedModel) && stripCodexSparkImageGenerationTools(reqBody) {
		result.Modified = true
	}

	// 续链场景保留 item_reference 与 id，避免 call_id 上下文丢失。
	if input, ok := reqBody["input"].([]any); ok {
		if normalizedInput, modified := normalizeCodexToolRoleMessages(input); modified {
			input = normalizedInput
			result.Modified = true
		}
		if normalizedInput, modified := normalizeCodexMessageContentText(input); modified {
			input = normalizedInput
			result.Modified = true
		}
		input = filterCodexInputWithOptions(input, codexInputFilterOptions{
			PreserveReferences: needsToolContinuation,
			PreserveCallIDs:    opts.PreserveToolCallIDs,
		})
		reqBody["input"] = input
		result.Modified = true
	} else if inputStr, ok := reqBody["input"].(string); ok {
		// ChatGPT codex endpoint requires input to be a list, not a string.
		// Convert string input to the expected message array format.
		trimmed := strings.TrimSpace(inputStr)
		if trimmed != "" {
			reqBody["input"] = []any{
				map[string]any{
					"type":    "message",
					"role":    "user",
					"content": inputStr,
				},
			}
		} else {
			reqBody["input"] = []any{}
		}
		result.Modified = true
	}

	return result
}

func normalizeCodexToolChoice(reqBody map[string]any) bool {
	choice, ok := reqBody["tool_choice"]
	if !ok || choice == nil {
		return false
	}
	choiceMap, ok := choice.(map[string]any)
	if !ok {
		return false
	}
	choiceType := strings.TrimSpace(firstNonEmptyString(choiceMap["type"]))
	if choiceType == "" {
		return false
	}
	modified := false
	if choiceType == "function" {
		name := strings.TrimSpace(firstNonEmptyString(choiceMap["name"]))
		if name == "" {
			if function, ok := choiceMap["function"].(map[string]any); ok {
				name = strings.TrimSpace(firstNonEmptyString(function["name"]))
			}
		}
		if name == "" {
			reqBody["tool_choice"] = "auto"
			return true
		}
		if strings.TrimSpace(firstNonEmptyString(choiceMap["name"])) != name {
			choiceMap["name"] = name
			modified = true
		}
		if _, ok := choiceMap["function"]; ok {
			delete(choiceMap, "function")
			modified = true
		}
		if !codexToolsContainFunctionName(reqBody["tools"], name) {
			reqBody["tool_choice"] = "auto"
			return true
		}
		return modified
	}
	if codexToolsContainType(reqBody["tools"], choiceType) {
		return modified
	}
	reqBody["tool_choice"] = "auto"
	return true
}

func codexToolsContainType(rawTools any, toolType string) bool {
	tools, ok := rawTools.([]any)
	if !ok || strings.TrimSpace(toolType) == "" {
		return false
	}
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(tool["type"])) == toolType {
			return true
		}
	}
	return false
}

func codexToolsContainFunctionName(rawTools any, name string) bool {
	tools, ok := rawTools.([]any)
	if !ok || strings.TrimSpace(name) == "" {
		return false
	}
	normalizedName := strings.TrimSpace(name)
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(tool["type"])) != "function" {
			continue
		}
		toolName := strings.TrimSpace(firstNonEmptyString(tool["name"]))
		if toolName == "" {
			if function, ok := tool["function"].(map[string]any); ok {
				toolName = strings.TrimSpace(firstNonEmptyString(function["name"]))
			}
		}
		if toolName == normalizedName {
			return true
		}
	}
	return false
}

func normalizeCodexToolRoleMessages(input []any) ([]any, bool) {
	if len(input) == 0 {
		return input, false
	}

	modified := false
	normalized := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}
		role, _ := m["role"].(string)
		if strings.TrimSpace(role) != "tool" {
			normalized = append(normalized, item)
			continue
		}

		callID := firstNonEmptyString(m["call_id"], m["tool_call_id"], m["id"])
		callID = strings.TrimSpace(callID)
		if callID == "" {
			// Responses does not accept role:"tool". If no call id is available,
			// preserve the text as a user message instead of sending invalid input.
			fallback := make(map[string]any, len(m))
			for key, value := range m {
				fallback[key] = value
			}
			fallback["role"] = "user"
			delete(fallback, "tool_call_id")
			normalized = append(normalized, fallback)
			modified = true
			continue
		}

		output := extractTextFromContent(m["content"])
		if output == "" {
			if value, ok := m["output"].(string); ok {
				output = value
			}
		}
		if output == "" && m["content"] != nil {
			if b, err := json.Marshal(m["content"]); err == nil {
				output = string(b)
			}
		}

		normalized = append(normalized, map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  output,
		})
		modified = true
	}
	if !modified {
		return input, false
	}
	return normalized, true
}

func normalizeCodexMessageContentText(input []any) ([]any, bool) {
	if len(input) == 0 {
		return input, false
	}

	modified := false
	normalized := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(firstNonEmptyString(m["type"])) != "message" {
			normalized = append(normalized, item)
			continue
		}
		parts, ok := m["content"].([]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}

		var newItem map[string]any
		var newParts []any
		ensureItemCopy := func() {
			if newItem != nil {
				return
			}
			newItem = make(map[string]any, len(m))
			for key, value := range m {
				newItem[key] = value
			}
			newParts = make([]any, len(parts))
			copy(newParts, parts)
		}

		for i, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			text, hasText := part["text"]
			if !hasText {
				continue
			}
			if _, ok := text.(string); ok {
				continue
			}

			ensureItemCopy()
			newPart := make(map[string]any, len(part))
			for key, value := range part {
				newPart[key] = value
			}
			newPart["text"] = stringifyCodexContentText(text)
			newParts[i] = newPart
			modified = true
		}

		if newItem != nil {
			newItem["content"] = newParts
			normalized = append(normalized, newItem)
			continue
		}
		normalized = append(normalized, item)
	}
	if !modified {
		return input, false
	}
	return normalized, true
}

func stringifyCodexContentText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprint(v)
	}
}

func normalizeCodexModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "gpt-5.4"
	}
	if mapped, ok := normalizeKnownCodexModel(model); ok {
		return mapped
	}
	return model
}

func normalizeKnownCodexModel(model string) (string, bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", false
	}
	if isOpenAIImageGenerationModel(model) {
		return model, true
	}

	modelID := lastOpenAIModelSegment(model)

	if normalized := canonicalizeOpenAIModelAliasSpelling(modelID); normalized != "" {
		modelID = normalized
	}
	if mapped := normalizeKnownOpenAICodexModel(modelID); mapped != "" {
		return mapped, true
	}
	key := codexModelLookupKey(modelID)
	if key == "" {
		return "", false
	}
	if mapped := getNormalizedCodexModel(key); mapped != "" {
		return mapped, true
	}
	for _, item := range codexVersionModelPrefixes {
		if key == item.prefix {
			return item.target, true
		}
		suffix, ok := strings.CutPrefix(key, item.prefix+"-")
		if ok && isKnownCodexModelSuffix(suffix) {
			return item.target, true
		}
	}
	return "", false
}

func codexModelLookupKey(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
	}
	return strings.ToLower(strings.Join(strings.Fields(modelID), "-"))
}

func isKnownCodexModelSuffix(suffix string) bool {
	switch suffix {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	}
	return isCodexDateSuffix(suffix)
}

func isCodexDateSuffix(suffix string) bool {
	parts := strings.Split(suffix, "-")
	if len(parts) != 3 || len(parts[0]) != 4 || len(parts[1]) != 2 || len(parts[2]) != 2 {
		return false
	}
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func isCodexSparkModel(model string) bool {
	return normalizeCodexModel(model) == "gpt-5.3-codex-spark"
}

func hasOpenAIImageGenerationTool(reqBody map[string]any) bool {
	if toolsContainImageGeneration(reqBody["tools"]) {
		return true
	}
	return inputContainsImageGenerationTool(reqBody["input"])
}

func toolsContainImageGeneration(rawTools any) bool {
	if rawTools == nil {
		return false
	}
	tools, ok := rawTools.([]any)
	if !ok {
		return false
	}
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if isOpenAIImageGenerationToolMap(toolMap) {
			return true
		}
	}
	return false
}

func isOpenAIImageGenerationToolMap(tool map[string]any) bool {
	return isOpenAIImageGenerationType(firstNonEmptyString(tool["type"])) ||
		isImageGenNamespaceToolMap(tool)
}

func isImageGenNamespaceToolMap(tool map[string]any) bool {
	return strings.TrimSpace(firstNonEmptyString(tool["type"])) == "namespace" &&
		isOpenAIImageGenNamespaceName(firstNonEmptyString(tool["name"]))
}

func inputContainsImageGenerationTool(rawInput any) bool {
	input, ok := rawInput.([]any)
	if !ok {
		return false
	}
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(item["type"])) != "additional_tools" {
			continue
		}
		if toolsContainImageGeneration(item["tools"]) {
			return true
		}
	}
	return false
}

// stripOpenAIImageGenerationTools keeps account-level strip policy symmetric
// across standard Responses tools, Responses Lite additional_tools, and tool_choice.
func stripOpenAIImageGenerationTools(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	modified := stripOpenAIImageGenerationToolList(reqBody, "tools")
	if stripOpenAIImageGenerationToolsFromInput(reqBody) {
		modified = true
	}
	if openAIAnyToolChoiceSelectsImageGeneration(reqBody["tool_choice"]) {
		delete(reqBody, "tool_choice")
		modified = true
	}
	return modified
}

func stripOpenAIImageGenerationToolList(container map[string]any, key string) bool {
	rawTools, ok := container[key]
	if !ok || rawTools == nil {
		return false
	}
	tools, ok := rawTools.([]any)
	if !ok {
		return false
	}
	filtered := make([]any, 0, len(tools))
	removed := false
	for _, rawTool := range tools {
		if toolMap, ok := rawTool.(map[string]any); ok && isOpenAIImageGenerationToolMap(toolMap) {
			removed = true
			continue
		}
		filtered = append(filtered, rawTool)
	}
	if !removed {
		return false
	}
	if len(filtered) == 0 {
		delete(container, key)
	} else {
		container[key] = filtered
	}
	return true
}

func stripOpenAIImageGenerationToolsFromInput(reqBody map[string]any) bool {
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
	}

	filteredInput := make([]any, 0, len(input))
	modified := false
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || strings.TrimSpace(firstNonEmptyString(item["type"])) != "additional_tools" {
			filteredInput = append(filteredInput, rawItem)
			continue
		}
		if !stripOpenAIImageGenerationToolList(item, "tools") {
			filteredInput = append(filteredInput, rawItem)
			continue
		}
		modified = true
		if _, hasTools := item["tools"]; hasTools {
			filteredInput = append(filteredInput, rawItem)
		}
		// An empty additional_tools carrier is not useful upstream; drop the item
		// after its only declared capability has been removed.
	}
	if modified {
		reqBody["input"] = filteredInput
	}
	return modified
}

// stripOpenAIImageGenerationToolsFromRawPayload is the shared adapter for paths
// that forward raw HTTP or WebSocket payloads without the normal request map.
func stripOpenAIImageGenerationToolsFromRawPayload(payload []byte) ([]byte, bool, error) {
	if !openAIRequestBodyHasImageGenerationDeclaration(payload) {
		if json.Valid(payload) {
			return payload, false, nil
		}
		var invalidPayload map[string]any
		return payload, false, json.Unmarshal(payload, &invalidPayload)
	}
	payloadMap := make(map[string]any)
	if err := json.Unmarshal(payload, &payloadMap); err != nil {
		return payload, false, err
	}
	if !stripOpenAIImageGenerationTools(payloadMap) {
		return payload, false, nil
	}
	rebuilt, err := json.Marshal(payloadMap)
	if err != nil {
		return payload, false, err
	}
	return rebuilt, true, nil
}

// stripCodexSparkImageGenerationTools removes image tool declarations and choices.
// gpt-5.3-codex-spark rejects those capabilities upstream, while Codex clients may
// advertise them by default.
func stripCodexSparkImageGenerationTools(reqBody map[string]any) bool {
	return stripOpenAIImageGenerationTools(reqBody)
}

func hasOpenAIInputImage(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	return hasOpenAIInputImageValue(reqBody["input"]) || hasOpenAIInputImageValue(reqBody["messages"])
}

func hasOpenAIInputImageValue(value any) bool {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if hasOpenAIInputImageValue(item) {
				return true
			}
		}
	case map[string]any:
		if strings.TrimSpace(firstNonEmptyString(v["type"])) == "input_image" {
			return true
		}
		if _, ok := v["image_url"]; ok {
			return true
		}
		return hasOpenAIInputImageValue(v["content"])
	}
	return false
}

func validateCodexSparkInput(reqBody map[string]any, model string) error {
	if !isCodexSparkModel(model) || !hasOpenAIInputImage(reqBody) {
		return nil
	}
	return fmt.Errorf("model %q does not support image input", strings.TrimSpace(model))
}

func normalizeOpenAIResponsesImageGenerationTools(reqBody map[string]any) bool {
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
	}
	tools, ok := rawTools.([]any)
	if !ok {
		return false
	}

	modified := false
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if !ok || strings.TrimSpace(firstNonEmptyString(toolMap["type"])) != "image_generation" {
			continue
		}
		if _, ok := toolMap["output_format"]; !ok {
			if value := strings.TrimSpace(firstNonEmptyString(toolMap["format"])); value != "" {
				toolMap["output_format"] = value
				modified = true
			}
		}
		if _, ok := toolMap["output_compression"]; !ok {
			if value, exists := toolMap["compression"]; exists && value != nil {
				toolMap["output_compression"] = value
				modified = true
			}
		}
		if _, ok := toolMap["format"]; ok {
			delete(toolMap, "format")
			modified = true
		}
		if _, ok := toolMap["compression"]; ok {
			delete(toolMap, "compression")
			modified = true
		}
	}
	return modified
}

func ensureOpenAIResponsesImageGenerationTool(reqBody map[string]any) bool {
	if len(reqBody) == 0 {
		return false
	}
	if isCodexSparkModel(firstNonEmptyString(reqBody["model"])) {
		return false
	}

	tool := map[string]any{
		"type":          "image_generation",
		"output_format": "png",
	}

	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		reqBody["tools"] = []any{tool}
		return true
	}

	tools, ok := rawTools.([]any)
	if !ok {
		reqBody["tools"] = []any{tool}
		return true
	}
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(toolMap["type"])) == "image_generation" {
			return false
		}
	}

	reqBody["tools"] = append(tools, tool)
	return true
}

func ensureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody map[string]any) bool {
	if len(reqBody) == 0 || !hasOpenAIImageGenerationTool(reqBody) {
		return false
	}
	if isCodexSparkModel(firstNonEmptyString(reqBody["model"])) {
		return false
	}
	if _, ok := reqBody["tool_choice"]; ok {
		return false
	}
	reqBody["tool_choice"] = "auto"
	return true
}

func applyCodexImageGenerationBridgeInstructions(reqBody map[string]any) bool {
	if len(reqBody) == 0 || !hasOpenAIImageGenerationTool(reqBody) {
		return false
	}
	if isCodexSparkModel(firstNonEmptyString(reqBody["model"])) {
		return false
	}

	existing, _ := reqBody["instructions"].(string)
	if strings.Contains(existing, codexImageGenerationBridgeMarker) {
		return false
	}

	existing = strings.TrimRight(existing, " \t\r\n")
	if strings.TrimSpace(existing) == "" {
		reqBody["instructions"] = codexImageGenerationBridgeText
		return true
	}

	reqBody["instructions"] = existing + "\n\n" + codexImageGenerationBridgeText
	return true
}

func applyCodexSparkImageUnsupportedInstructions(reqBody map[string]any) bool {
	if len(reqBody) == 0 {
		return false
	}
	existing, _ := reqBody["instructions"].(string)
	if strings.Contains(existing, codexSparkImageUnsupportedMarker) {
		return false
	}
	existing = strings.TrimRight(existing, " \t\r\n")
	if strings.TrimSpace(existing) == "" {
		reqBody["instructions"] = codexSparkImageUnsupportedText
		return true
	}
	reqBody["instructions"] = existing + "\n\n" + codexSparkImageUnsupportedText
	return true
}

func validateOpenAIResponsesImageModel(reqBody map[string]any, model string) error {
	if !hasOpenAIImageGenerationTool(reqBody) {
		return nil
	}
	model = strings.TrimSpace(model)
	if !isOpenAIImageGenerationModel(model) {
		return nil
	}
	return fmt.Errorf("/v1/responses image_generation requests require a Responses-capable text model; image-only model %q is not allowed", model)
}

func normalizeOpenAIResponsesImageOnlyModel(reqBody map[string]any) bool {
	if len(reqBody) == 0 {
		return false
	}
	imageModel := strings.TrimSpace(firstNonEmptyString(reqBody["model"]))
	if !isOpenAIImageGenerationModel(imageModel) {
		return false
	}

	modified := false
	tools, _ := reqBody["tools"].([]any)
	imageToolIndex := -1
	for i, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(toolMap["type"])) == "image_generation" {
			imageToolIndex = i
			break
		}
	}
	if imageToolIndex < 0 {
		tools = append(tools, map[string]any{
			"type":  "image_generation",
			"model": imageModel,
		})
		imageToolIndex = len(tools) - 1
		reqBody["tools"] = tools
		modified = true
	}

	if toolMap, ok := tools[imageToolIndex].(map[string]any); ok {
		if strings.TrimSpace(firstNonEmptyString(toolMap["model"])) == "" {
			toolMap["model"] = imageModel
			modified = true
		}
		for _, key := range []string{
			"size",
			"quality",
			"background",
			"output_format",
			"output_compression",
			"moderation",
			"style",
			"partial_images",
		} {
			if value, exists := reqBody[key]; exists && value != nil {
				if _, toolHas := toolMap[key]; !toolHas {
					toolMap[key] = value
				}
				delete(reqBody, key)
				modified = true
			}
		}
	}

	if prompt := strings.TrimSpace(firstNonEmptyString(reqBody["prompt"])); prompt != "" {
		if _, hasInput := reqBody["input"]; !hasInput {
			reqBody["input"] = prompt
		}
		delete(reqBody, "prompt")
		modified = true
	}

	if _, ok := reqBody["tool_choice"]; !ok {
		reqBody["tool_choice"] = map[string]any{"type": "image_generation"}
		modified = true
	}
	if imageModel != openAIImagesResponsesMainModel {
		modified = true
	}
	reqBody["model"] = openAIImagesResponsesMainModel
	return modified
}

func normalizeOpenAIModelForUpstream(account *Account, model string) string {
	if account == nil || account.Type == AccountTypeOAuth {
		return normalizeCodexModel(model)
	}
	return strings.TrimSpace(model)
}

func SupportsVerbosity(model string) bool {
	if !strings.HasPrefix(model, "gpt-") {
		return true
	}

	var major, minor int
	n, _ := fmt.Sscanf(model, "gpt-%d.%d", &major, &minor)

	if major > 5 {
		return true
	}
	if major < 5 {
		return false
	}

	// gpt-5
	if n == 1 {
		return true
	}

	return minor >= 3
}

func getNormalizedCodexModel(modelID string) string {
	key := codexModelLookupKey(modelID)
	if key == "" {
		return ""
	}
	if mapped, ok := codexModelMap[key]; ok {
		return mapped
	}
	return ""
}

// extractTextFromContent extracts plain text from a content value that is either
// a Go string or a []any of text-like content-part maps.
func extractTextFromContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, part := range v {
			m, ok := part.(map[string]any)
			if !ok {
				continue
			}
			switch t, _ := m["type"].(string); t {
			case "text", "input_text", "output_text":
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// extractSystemMessagesFromInput scans input for role=="system", maps those
// items to developer, and mirrors their text into reqBody["instructions"].
// It preserves the input items so Responses JSON mode can still see JSON
// instructions in input messages.
func extractSystemMessagesFromInput(reqBody map[string]any) bool {
	input, ok := reqBody["input"].([]any)
	if !ok || len(input) == 0 {
		return false
	}

	var systemTexts []string
	modified := false
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := m["role"].(string); role != "system" {
			continue
		}
		m["role"] = "developer"
		modified = true
		if text := extractTextFromContent(m["content"]); text != "" {
			systemTexts = append(systemTexts, text)
		}
	}

	if len(systemTexts) == 0 {
		return modified
	}

	extracted := strings.Join(systemTexts, "\n\n")
	if existing, ok := reqBody["instructions"].(string); ok && strings.TrimSpace(existing) != "" {
		reqBody["instructions"] = extracted + "\n\n" + existing
	} else {
		reqBody["instructions"] = extracted
	}
	return true
}

func extractPromptLikeInstructionsFromInput(reqBody map[string]any) string {
	input, ok := reqBody["input"].([]any)
	if !ok || len(input) == 0 {
		return ""
	}
	var texts []string
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		switch role {
		case "developer", "system":
			if text := strings.TrimSpace(extractTextFromContent(m["content"])); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return strings.Join(texts, "\n\n")
}

// defaultCodexSynthInstructions 返回合成路径在 instructions 为空时应填入的默认提示词。
//
// 按 model 选择真实 Codex CLI 的 base instructions（codex 系→GPT-5-Codex，
// gpt-5.2→GPT-5.2，gpt-5.1/gpt-5→GPT-5.1），使合成请求在提示词层面贴近真实 Codex 行为；
// 若内嵌 prompt 意外为空，回退到最小占位符以保证字段非空。
func defaultCodexSynthInstructions(model string) string {
	if instructions := strings.TrimSpace(openai.CodexBaseInstructionsForModel(model)); instructions != "" {
		return instructions
	}
	return "You are a helpful coding assistant."
}

// ensureCodexReasoningInclude 在请求带 reasoning 时补齐 include:["reasoning.encrypted_content"]。
//
// 真实 Codex 在 reasoning 存在时总会请求加密推理内容（ChatGPT/store=false 场景下用于上下文回放）。
// 该函数为加法式、幂等：仅在 include 缺失或未包含该项时追加；对非数组的异常 include 不做破坏性改写。
func ensureCodexReasoningInclude(reqBody map[string]any) bool {
	reasoning, ok := reqBody["reasoning"].(map[string]any)
	if !ok || len(reasoning) == 0 {
		return false
	}
	const encrypted = "reasoning.encrypted_content"
	switch existing := reqBody["include"].(type) {
	case nil:
		reqBody["include"] = []any{encrypted}
		return true
	case []any:
		for _, v := range existing {
			if s, ok := v.(string); ok && s == encrypted {
				return false
			}
		}
		reqBody["include"] = append(existing, encrypted)
		return true
	default:
		// include 为非预期类型时保持原样，避免破坏调用方意图。
		return false
	}
}

// applyCodexClientMetadata 在请求体补齐 client_metadata["x-codex-installation-id"]，
// 取值为账号真实的 openai_device_id（最新 Codex 在请求体携带的安装标识）。
//
// 加法式、幂等：仅在账号存在 device_id 且该键缺失时注入，绝不覆盖既有 client_metadata
// （如 turn metadata），也不伪造——无 device_id 时不写入。
func applyCodexClientMetadata(reqBody map[string]any, account *Account) bool {
	if account == nil {
		return false
	}
	deviceID := strings.TrimSpace(account.GetOpenAIDeviceID())
	if deviceID == "" {
		return false
	}
	const key = "x-codex-installation-id"
	switch existing := reqBody["client_metadata"].(type) {
	case map[string]any:
		if v, ok := existing[key].(string); ok && strings.TrimSpace(v) != "" {
			return false
		}
		existing[key] = deviceID
		reqBody["client_metadata"] = existing
		return true
	case map[string]string:
		if strings.TrimSpace(existing[key]) != "" {
			return false
		}
		next := make(map[string]any, len(existing)+1)
		for k, v := range existing {
			next[k] = v
		}
		next[key] = deviceID
		reqBody["client_metadata"] = next
		return true
	case nil:
		reqBody["client_metadata"] = map[string]any{key: deviceID}
		return true
	default:
		return false
	}
}

// applyInstructions 处理 instructions 字段：仅在 instructions 为空时填充默认值。
func applyInstructions(reqBody map[string]any, isCodexCLI bool) bool {
	if !isInstructionsEmpty(reqBody) {
		return false
	}
	model, _ := reqBody["model"].(string)
	reqBody["instructions"] = defaultCodexSynthInstructions(model)
	return true
}

// isInstructionsEmpty 检查 instructions 字段是否为空
// 处理以下情况：字段不存在、nil、空字符串、纯空白字符串
func isInstructionsEmpty(reqBody map[string]any) bool {
	val, exists := reqBody["instructions"]
	if !exists {
		return true
	}
	if val == nil {
		return true
	}
	str, ok := val.(string)
	if !ok {
		return true
	}
	return strings.TrimSpace(str) == ""
}

type codexInputFilterOptions struct {
	PreserveReferences bool
	PreserveCallIDs    bool
}

// filterCodexInput 按需过滤 item_reference 与 id。
// preserveReferences 为 true 时保持引用与 id，以满足续链请求对上下文的依赖。
func filterCodexInput(input []any, preserveReferences bool) []any {
	return filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: preserveReferences,
	})
}

func filterCodexInputWithOptions(input []any, opts codexInputFilterOptions) []any {
	filtered := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		typ, _ := m["type"].(string)

		// chatgpt.com codex (OAuth path) runs with store=false (forced by
		// applyCodexOAuthTransform). Replaying a reasoning item with its rs_*
		// id but no encrypted_content 404s upstream ("Item with id 'rs_...'
		// not found") — the 404 is triggered by the id lookup, not by the
		// reasoning item itself. So strip the id (always, independent of
		// PreserveReferences) yet keep the item: under store=false
		// encrypted_content is the official channel for carrying reasoning
		// context across turns, and dropping the whole item silently degrades
		// multi-turn agent reasoning. Preserve encrypted_content/content/
		// summary and every other field verbatim. Upstream additionally
		// requires a summary field — a missing one is rejected with 400
		// "Missing required parameter 'input[N].summary'" — so backfill an
		// empty array when it is absent. Contracts verified end-to-end against
		// chatgpt.com codex (gpt-5.5); see issue #1957.
		// compaction_summary items (cmp_*) are the other encrypted_content
		// carrier. Verified against the live backend: they require
		// encrypted_content (a missing one is rejected with 400), and with it
		// present the cmp_* id does not 404 whether kept or stripped. Being
		// neither reasoning nor tool calls, they flow through the generic path
		// below (id stripped when !PreserveReferences, encrypted_content
		// preserved either way), which is safe and needs no special-casing.
		if typ == "reasoning" {
			newItem := make(map[string]any, len(m))
			for key, value := range m {
				if key == "id" {
					// rs_* id replayed under store=false 404s; strip it.
					continue
				}
				newItem[key] = value
			}
			if summary, ok := newItem["summary"]; !ok || summary == nil {
				// Upstream requires a summary field; an empty array satisfies it.
				newItem["summary"] = []any{}
			}
			filtered = append(filtered, newItem)
			continue
		}

		// 仅修正真正的 tool/function call 标识，避免误改普通 message/reasoning id；
		// 若 item_reference 指向 legacy call_* 标识，则仅修正该引用本身。
		fixCallIDPrefix := func(id string) string {
			if opts.PreserveCallIDs {
				return id
			}
			if id == "" || strings.HasPrefix(id, "fc") {
				return id
			}
			if strings.HasPrefix(id, "call_") {
				return "fc_" + strings.TrimPrefix(id, "call_")
			}
			return "fc_" + id
		}

		if typ == "item_reference" {
			if !opts.PreserveReferences {
				continue
			}
			newItem := make(map[string]any, len(m))
			for key, value := range m {
				newItem[key] = value
			}
			if id, ok := newItem["id"].(string); ok && strings.HasPrefix(id, "call_") {
				newItem["id"] = fixCallIDPrefix(id)
			}
			filtered = append(filtered, newItem)
			continue
		}

		newItem := m
		copied := false
		// 仅在需要修改字段时创建副本，避免直接改写原始输入。
		ensureCopy := func() {
			if copied {
				return
			}
			newItem = make(map[string]any, len(m))
			for key, value := range m {
				newItem[key] = value
			}
			copied = true
		}

		if isCodexToolCallItemType(typ) {
			callID, ok := m["call_id"].(string)
			if !ok || strings.TrimSpace(callID) == "" {
				if id, ok := m["id"].(string); ok && strings.TrimSpace(id) != "" {
					callID = id
					ensureCopy()
					newItem["call_id"] = callID
				}
			}

			if callID != "" {
				fixedCallID := fixCallIDPrefix(callID)
				if fixedCallID != callID {
					ensureCopy()
					newItem["call_id"] = fixedCallID
				}
			}
		}

		if !isCodexToolCallItemType(typ) {
			ensureCopy()
			delete(newItem, "call_id")
		}

		if codexInputItemRequiresName(typ) {
			if strings.TrimSpace(firstNonEmptyString(m["name"])) == "" {
				name := firstNonEmptyString(m["tool_name"])
				if name == "" {
					if function, ok := m["function"].(map[string]any); ok {
						name = firstNonEmptyString(function["name"])
					}
				}
				if name == "" {
					name = "tool"
				}
				ensureCopy()
				newItem["name"] = name
			}
		}

		if !opts.PreserveReferences {
			ensureCopy()
			delete(newItem, "id")
		} else if isCodexToolCallInputType(typ) {
			// 续链模式下保留 id 以维持上下文引用，但 function_call 等
			// call-input 类 item 的 id 必须以 "fc" 开头（上游校验
			// "Expected an ID that begins with 'fc'"）。item_* 形式的 id
			// 来自客户端回放，需要删除。
			// 注意：function_call_output 等 output 类的 id 无此约束，不动。
			if id, ok := m["id"].(string); ok && id != "" && !strings.HasPrefix(id, "fc") {
				ensureCopy()
				delete(newItem, "id")
			}
		}

		filtered = append(filtered, newItem)
	}
	return filtered
}

func isCodexToolCallItemType(typ string) bool {
	switch typ {
	case "function_call",
		"tool_call",
		"local_shell_call",
		"tool_search_call",
		"custom_tool_call",
		"mcp_tool_call",
		"function_call_output",
		"mcp_tool_call_output",
		"custom_tool_call_output",
		"tool_search_output":
		return true
	default:
		return false
	}
}

// isCodexToolCallInputType 仅匹配 call-input 类型（不含 output），这些类型的
// id 必须以 "fc" 开头，上游会校验 "Expected an ID that begins with 'fc'."。
func isCodexToolCallInputType(typ string) bool {
	switch typ {
	case "function_call",
		"tool_call",
		"local_shell_call",
		"tool_search_call",
		"custom_tool_call",
		"mcp_tool_call":
		return true
	default:
		return false
	}
}

func codexInputItemRequiresName(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "function_call", "custom_tool_call", "mcp_tool_call":
		return true
	default:
		return false
	}
}

func normalizeCodexTools(reqBody map[string]any) bool {
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
	}
	tools, ok := rawTools.([]any)
	if !ok {
		return false
	}

	modified := false
	validTools := make([]any, 0, len(tools))

	for _, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			// Keep unknown structure as-is to avoid breaking upstream behavior.
			validTools = append(validTools, tool)
			continue
		}

		toolType, _ := toolMap["type"].(string)
		toolType = strings.TrimSpace(toolType)
		if toolType != "function" {
			validTools = append(validTools, toolMap)
			continue
		}

		// OpenAI Responses-style tools use top-level name/parameters.
		if name, ok := toolMap["name"].(string); ok && strings.TrimSpace(name) != "" {
			validTools = append(validTools, toolMap)
			continue
		}

		// ChatCompletions-style tools use {type:"function", function:{...}}.
		functionValue, hasFunction := toolMap["function"]
		function, ok := functionValue.(map[string]any)
		if !hasFunction || functionValue == nil || !ok || function == nil {
			// Drop invalid function tools.
			modified = true
			continue
		}

		if _, ok := toolMap["name"]; !ok {
			if name, ok := function["name"].(string); ok && strings.TrimSpace(name) != "" {
				toolMap["name"] = name
				modified = true
			}
		}
		if _, ok := toolMap["description"]; !ok {
			if desc, ok := function["description"].(string); ok && strings.TrimSpace(desc) != "" {
				toolMap["description"] = desc
				modified = true
			}
		}
		if _, ok := toolMap["parameters"]; !ok {
			if params, ok := function["parameters"]; ok {
				toolMap["parameters"] = params
				modified = true
			}
		}
		if _, ok := toolMap["strict"]; !ok {
			if strict, ok := function["strict"]; ok {
				toolMap["strict"] = strict
				modified = true
			}
		}

		validTools = append(validTools, toolMap)
	}

	if modified {
		reqBody["tools"] = validTools
	}

	return modified
}
