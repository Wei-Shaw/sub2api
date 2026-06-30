package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

var codexModelMap = map[string]string{
	"gpt-5.5":                    "gpt-5.5",
	"gpt-5.5-pro":                "gpt-5.5-pro",
	"codex-auto-review":          "codex-auto-review",
	"gpt-5.4":                    "gpt-5.4",
	"gpt-5.4-mini":               "gpt-5.4-mini",
	"gpt-5.4-none":               "gpt-5.4",
	"gpt-5.4-low":                "gpt-5.4",
	"gpt-5.4-medium":             "gpt-5.4",
	"gpt-5.4-high":               "gpt-5.4",
	"gpt-5.4-xhigh":              "gpt-5.4",
	"gpt-5.4-chat-latest":        "gpt-5.4",
	"gpt-5.3":                    "gpt-5.3-codex",
	"gpt-5.3-none":               "gpt-5.3-codex",
	"gpt-5.3-low":                "gpt-5.3-codex",
	"gpt-5.3-medium":             "gpt-5.3-codex",
	"gpt-5.3-high":               "gpt-5.3-codex",
	"gpt-5.3-xhigh":              "gpt-5.3-codex",
	"gpt-5.3-codex":              "gpt-5.3-codex",
	"gpt-5.3-codex-spark":        "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-low":    "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-medium": "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-high":   "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-xhigh":  "gpt-5.3-codex-spark",
	"gpt-5.3-codex-low":          "gpt-5.3-codex",
	"gpt-5.3-codex-medium":       "gpt-5.3-codex",
	"gpt-5.3-codex-high":         "gpt-5.3-codex",
	"gpt-5.3-codex-xhigh":        "gpt-5.3-codex",
	"gpt-5.2":                    "gpt-5.2",
	"gpt-5.2-none":               "gpt-5.2",
	"gpt-5.2-low":                "gpt-5.2",
	"gpt-5.2-medium":             "gpt-5.2",
	"gpt-5.2-high":               "gpt-5.2",
	"gpt-5.2-xhigh":              "gpt-5.2",
	"gpt-5":                      "gpt-5.4",
	"gpt-5-mini":                 "gpt-5.4",
	"gpt-5-nano":                 "gpt-5.4",
	"gpt-5.1":                    "gpt-5.4",
	"gpt-5.1-codex":              "gpt-5.3-codex",
	"gpt-5.1-codex-max":          "gpt-5.3-codex",
	"gpt-5.1-codex-mini":         "gpt-5.3-codex",
	"gpt-5.2-codex":              "gpt-5.2",
	"codex-mini-latest":          "gpt-5.3-codex",
	"gpt-5-codex":                "gpt-5.3-codex",
REDACTED

var codexVersionModelPrefixes = []struct {
	prefix string
	target string
REDACTED{
	{prefix: "gpt-5.3-codex-spark", target: "gpt-5.3-codex-spark"REDACTED,
	{prefix: "gpt-5.3-codex", target: "gpt-5.3-codex"REDACTED,
	{prefix: "gpt-5.4-mini", target: "gpt-5.4-mini"REDACTED,
	{prefix: "gpt-5.4-nano", target: "gpt-5.4-nano"REDACTED,
	{prefix: "gpt-5.5-pro", target: "gpt-5.5-pro"REDACTED,
	{prefix: "gpt-5.5", target: "gpt-5.5"REDACTED,
	{prefix: "gpt-5.4", target: "gpt-5.4"REDACTED,
	{prefix: "gpt-5.2", target: "gpt-5.2"REDACTED,
REDACTED

type codexTransformResult struct {
	Modified        bool
	NormalizedModel string
	PromptCacheKey  string
REDACTED

type codexOAuthTransformOptions struct {
	IsCodexCLI              bool
	IsCompact               bool
	SkipDefaultInstructions bool
	PreserveToolCallIDs     bool
REDACTED

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
REDACTED

var openAICodexOAuthUnsupportedFields = append([]string{
	"max_output_tokens",
	"max_completion_tokens",
	"temperature",
	"top_p",
	"frequency_penalty",
	"presence_penalty",
REDACTED, openAIChatGPTInternalUnsupportedFields...)

func applyCodexOAuthTransform(reqBody map[string]any, isCodexCLI bool, isCompact bool) codexTransformResult {
	return applyCodexOAuthTransformWithOptions(reqBody, codexOAuthTransformOptions{
		IsCodexCLI: isCodexCLI,
		IsCompact:  isCompact,
REDACTED)
REDACTED

func applyCodexOAuthTransformWithOptions(reqBody map[string]any, opts codexOAuthTransformOptions) codexTransformResult {
	result := codexTransformResult{REDACTED
	// 工具续链需求会影响存储策略与 input 过滤逻辑。
	needsToolContinuation := NeedsToolContinuation(reqBody)

	model := ""
	if v, ok := reqBody["model"].(string); ok {
		model = v
REDACTED
	normalizedModel := strings.TrimSpace(model)
	if normalizedModel != "" {
		if model != normalizedModel {
			reqBody["model"] = normalizedModel
			result.Modified = true
	REDACTED
		result.NormalizedModel = normalizedModel
REDACTED

	if opts.IsCompact {
		if _, ok := reqBody["store"]; ok {
			delete(reqBody, "store")
			result.Modified = true
	REDACTED
		if _, ok := reqBody["stream"]; ok {
			delete(reqBody, "stream")
			result.Modified = true
	REDACTED
REDACTED else {
		// OAuth 走 ChatGPT internal API 时，store 必须为 false；显式 true 也会强制覆盖。
		// 避免上游返回 "Store must be set to false"。
		if v, ok := reqBody["store"].(bool); !ok || v {
			reqBody["store"] = false
			result.Modified = true
	REDACTED
		if v, ok := reqBody["stream"].(bool); !ok || !v {
			reqBody["stream"] = true
			result.Modified = true
	REDACTED
REDACTED

	// Strip parameters unsupported by ChatGPT internal Codex endpoint.
	for _, key := range openAICodexOAuthUnsupportedFields {
		if _, ok := reqBody[key]; ok {
			delete(reqBody, key)
			result.Modified = true
	REDACTED
REDACTED

	// 请求带 reasoning 时补齐 include:["reasoning.encrypted_content"]，与真实 Codex 对齐
	// （compact 端点形态不同，单独处理，此处跳过）。
	if !opts.IsCompact && ensureCodexReasoningInclude(reqBody) {
		result.Modified = true
REDACTED

	// 兼容遗留的 functions 和 function_call，转换为 tools 和 tool_choice
	if functionsRaw, ok := reqBody["functions"]; ok {
		if functions, k := functionsRaw.([]any); k {
			tools := make([]any, 0, len(functions))
			for _, f := range functions {
				tools = append(tools, map[string]any{
					"type":     "function",
					"function": f,
			REDACTED)
		REDACTED
			reqBody["tools"] = tools
	REDACTED
		delete(reqBody, "functions")
		result.Modified = true
REDACTED

	if fcRaw, ok := reqBody["function_call"]; ok {
		if fcStr, ok := fcRaw.(string); ok {
			// e.g. "auto", "none"
			reqBody["tool_choice"] = fcStr
	REDACTED else if fcObj, ok := fcRaw.(map[string]any); ok {
			// e.g. {"name": "my_func"REDACTED
			if name, ok := fcObj["name"].(string); ok && strings.TrimSpace(name) != "" {
				reqBody["tool_choice"] = map[string]any{
					"type": "function",
					"name": name,
			REDACTED
		REDACTED
	REDACTED
		delete(reqBody, "function_call")
		result.Modified = true
REDACTED

	if normalizeCodexTools(reqBody) {
		result.Modified = true
REDACTED
	if normalizeCodexToolChoice(reqBody) {
		result.Modified = true
REDACTED

	if v, ok := reqBody["prompt_cache_key"].(string); ok {
		result.PromptCacheKey = strings.TrimSpace(v)
		if isOpenAICompatMessagesBridgeRequestBody(reqBody) {
			delete(reqBody, "prompt_cache_key")
			result.Modified = true
	REDACTED
REDACTED

	// ChatGPT internal Codex endpoint does not accept role:"system".
	// Keep the guidance in input as developer for Responses JSON mode, and
	// also mirror it into instructions because Codex OAuth requires it.
	if extractSystemMessagesFromInput(reqBody) {
		result.Modified = true
REDACTED

	// instructions 处理逻辑：根据是否是 Codex CLI 分别调用不同方法
	if !opts.SkipDefaultInstructions && applyInstructions(reqBody, opts.IsCodexCLI) {
		result.Modified = true
REDACTED
	if isCodexSparkModel(normalizedModel) && applyCodexSparkImageUnsupportedInstructions(reqBody) {
		result.Modified = true
REDACTED
	// gpt-5.3-codex-spark rejects the image_generation tool upstream (HTTP 400,
	// param=tools); Codex CLI advertises it by default, so strip it for spark.
	if isCodexSparkModel(normalizedModel) && stripCodexSparkImageGenerationTools(reqBody) {
		result.Modified = true
REDACTED

	// 续链场景保留 item_reference 与 id，避免 call_id 上下文丢失。
	if input, ok := reqBody["input"].([]any); ok {
		if normalizedInput, modified := normalizeCodexToolRoleMessages(input); modified {
			input = normalizedInput
			result.Modified = true
	REDACTED
		if normalizedInput, modified := normalizeCodexMessageContentText(input); modified {
			input = normalizedInput
			result.Modified = true
	REDACTED
		input = filterCodexInputWithOptions(input, codexInputFilterOptions{
			PreserveReferences: needsToolContinuation,
			PreserveCallIDs:    opts.PreserveToolCallIDs,
	REDACTED)
		reqBody["input"] = input
		result.Modified = true
REDACTED else if inputStr, ok := reqBody["input"].(string); ok {
		// ChatGPT codex endpoint requires input to be a list, not a string.
		// Convert string input to the expected message array format.
		trimmed := strings.TrimSpace(inputStr)
		if trimmed != "" {
			reqBody["input"] = []any{
				map[string]any{
					"type":    "message",
					"role":    "user",
					"content": inputStr,
			REDACTED,
		REDACTED
	REDACTED else {
			reqBody["input"] = []any{REDACTED
	REDACTED
		result.Modified = true
REDACTED

	return result
REDACTED

func normalizeCodexToolChoice(reqBody map[string]any) bool {
	choice, ok := reqBody["tool_choice"]
	if !ok || choice == nil {
		return false
REDACTED
	choiceMap, ok := choice.(map[string]any)
	if !ok {
		return false
REDACTED
	choiceType := strings.TrimSpace(firstNonEmptyString(choiceMap["type"]))
	if choiceType == "" {
		return false
REDACTED
	modified := false
	if choiceType == "function" {
		name := strings.TrimSpace(firstNonEmptyString(choiceMap["name"]))
		if name == "" {
			if function, ok := choiceMap["function"].(map[string]any); ok {
				name = strings.TrimSpace(firstNonEmptyString(function["name"]))
		REDACTED
	REDACTED
		if name == "" {
			reqBody["tool_choice"] = "auto"
			return true
	REDACTED
		if strings.TrimSpace(firstNonEmptyString(choiceMap["name"])) != name {
			choiceMap["name"] = name
			modified = true
	REDACTED
		if _, ok := choiceMap["function"]; ok {
			delete(choiceMap, "function")
			modified = true
	REDACTED
		if !codexToolsContainFunctionName(reqBody["tools"], name) {
			reqBody["tool_choice"] = "auto"
			return true
	REDACTED
		return modified
REDACTED
	if codexToolsContainType(reqBody["tools"], choiceType) {
		return modified
REDACTED
	reqBody["tool_choice"] = "auto"
	return true
REDACTED

func codexToolsContainType(rawTools any, toolType string) bool {
	tools, ok := rawTools.([]any)
	if !ok || strings.TrimSpace(toolType) == "" {
		return false
REDACTED
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
	REDACTED
		if strings.TrimSpace(firstNonEmptyString(tool["type"])) == toolType {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func codexToolsContainFunctionName(rawTools any, name string) bool {
	tools, ok := rawTools.([]any)
	if !ok || strings.TrimSpace(name) == "" {
		return false
REDACTED
	normalizedName := strings.TrimSpace(name)
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
	REDACTED
		if strings.TrimSpace(firstNonEmptyString(tool["type"])) != "function" {
			continue
	REDACTED
		toolName := strings.TrimSpace(firstNonEmptyString(tool["name"]))
		if toolName == "" {
			if function, ok := tool["function"].(map[string]any); ok {
				toolName = strings.TrimSpace(firstNonEmptyString(function["name"]))
		REDACTED
	REDACTED
		if toolName == normalizedName {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func normalizeCodexToolRoleMessages(input []any) ([]any, bool) {
	if len(input) == 0 {
		return input, false
REDACTED

	modified := false
	normalized := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			normalized = append(normalized, item)
			continue
	REDACTED
		role, _ := m["role"].(string)
		if strings.TrimSpace(role) != "tool" {
			normalized = append(normalized, item)
			continue
	REDACTED

		callID := firstNonEmptyString(m["call_id"], m["tool_call_id"], m["id"])
		callID = strings.TrimSpace(callID)
		if callID == "" {
			// Responses does not accept role:"tool". If no call id is available,
			// preserve the text as a user message instead of sending invalid input.
			fallback := make(map[string]any, len(m))
			for key, value := range m {
				fallback[key] = value
		REDACTED
			fallback["role"] = "user"
			delete(fallback, "tool_call_id")
			normalized = append(normalized, fallback)
			modified = true
			continue
	REDACTED

		output := extractTextFromContent(m["content"])
		if output == "" {
			if value, ok := m["output"].(string); ok {
				output = value
		REDACTED
	REDACTED
		if output == "" && m["content"] != nil {
			if b, err := json.Marshal(m["content"]); err == nil {
				output = string(b)
		REDACTED
	REDACTED

		normalized = append(normalized, map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  output,
	REDACTED)
		modified = true
REDACTED
	if !modified {
		return input, false
REDACTED
	return normalized, true
REDACTED

func normalizeCodexMessageContentText(input []any) ([]any, bool) {
	if len(input) == 0 {
		return input, false
REDACTED

	modified := false
	normalized := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(firstNonEmptyString(m["type"])) != "message" {
			normalized = append(normalized, item)
			continue
	REDACTED
		parts, ok := m["content"].([]any)
		if !ok {
			normalized = append(normalized, item)
			continue
	REDACTED

		var newItem map[string]any
		var newParts []any
		ensureItemCopy := func() {
			if newItem != nil {
				return
		REDACTED
			newItem = make(map[string]any, len(m))
			for key, value := range m {
				newItem[key] = value
		REDACTED
			newParts = make([]any, len(parts))
			copy(newParts, parts)
	REDACTED

		for i, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
		REDACTED
			text, hasText := part["text"]
			if !hasText {
				continue
		REDACTED
			if _, ok := text.(string); ok {
				continue
		REDACTED

			ensureItemCopy()
			newPart := make(map[string]any, len(part))
			for key, value := range part {
				newPart[key] = value
		REDACTED
			newPart["text"] = stringifyCodexContentText(text)
			newParts[i] = newPart
			modified = true
	REDACTED

		if newItem != nil {
			newItem["content"] = newParts
			normalized = append(normalized, newItem)
			continue
	REDACTED
		normalized = append(normalized, item)
REDACTED
	if !modified {
		return input, false
REDACTED
	return normalized, true
REDACTED

func stringifyCodexContentText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
	REDACTED
		return fmt.Sprint(v)
REDACTED
REDACTED

func normalizeCodexModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "gpt-5.4"
REDACTED
	if mapped, ok := normalizeKnownCodexModel(model); ok {
		return mapped
REDACTED
	return model
REDACTED

func normalizeKnownCodexModel(model string) (string, bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", false
REDACTED
	if isOpenAIImageGenerationModel(model) {
		return model, true
REDACTED

	modelID := lastOpenAIModelSegment(model)

	if normalized := canonicalizeOpenAIModelAliasSpelling(modelID); normalized != "" {
		modelID = normalized
REDACTED
	if mapped := normalizeKnownOpenAICodexModel(modelID); mapped != "" {
		return mapped, true
REDACTED
	key := codexModelLookupKey(modelID)
	if key == "" {
		return "", false
REDACTED
	if mapped := getNormalizedCodexModel(key); mapped != "" {
		return mapped, true
REDACTED
	for _, item := range codexVersionModelPrefixes {
		if key == item.prefix {
			return item.target, true
	REDACTED
		suffix, ok := strings.CutPrefix(key, item.prefix+"-")
		if ok && isKnownCodexModelSuffix(suffix) {
			return item.target, true
	REDACTED
REDACTED
	return "", false
REDACTED

func codexModelLookupKey(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
REDACTED
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
REDACTED
	return strings.ToLower(strings.Join(strings.Fields(modelID), "-"))
REDACTED

func isKnownCodexModelSuffix(suffix string) bool {
	switch suffix {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return true
REDACTED
	return isCodexDateSuffix(suffix)
REDACTED

func isCodexDateSuffix(suffix string) bool {
	parts := strings.Split(suffix, "-")
	if len(parts) != 3 || len(parts[0]) != 4 || len(parts[1]) != 2 || len(parts[2]) != 2 {
		return false
REDACTED
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
		REDACTED
	REDACTED
REDACTED
	return true
REDACTED

func isCodexSparkModel(model string) bool {
	return normalizeCodexModel(model) == "gpt-5.3-codex-spark"
REDACTED

func hasOpenAIImageGenerationTool(reqBody map[string]any) bool {
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
REDACTED
	tools, ok := rawTools.([]any)
	if !ok {
		return false
REDACTED
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if !ok {
			continue
	REDACTED
		if strings.TrimSpace(firstNonEmptyString(toolMap["type"])) == "image_generation" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// stripCodexSparkImageGenerationTools removes image_generation tool entries from
// reqBody["tools"]. gpt-5.3-codex-spark rejects that tool upstream with HTTP 400
// (invalid_request_error, param=tools), and Codex CLI advertises it by default, so
// it must be dropped for spark. When the tools list becomes empty the key is removed.
// Returns true when the body was modified.
func stripCodexSparkImageGenerationTools(reqBody map[string]any) bool {
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
REDACTED
	tools, ok := rawTools.([]any)
	if !ok {
		return false
REDACTED
	filtered := make([]any, 0, len(tools))
	removed := false
	for _, rawTool := range tools {
		if toolMap, ok := rawTool.(map[string]any); ok &&
			strings.TrimSpace(firstNonEmptyString(toolMap["type"])) == "image_generation" {
			removed = true
			continue
	REDACTED
		filtered = append(filtered, rawTool)
REDACTED
	if !removed {
		return false
REDACTED
	if len(filtered) == 0 {
		delete(reqBody, "tools")
REDACTED else {
		reqBody["tools"] = filtered
REDACTED
	return true
REDACTED

func hasOpenAIInputImage(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
REDACTED
	return hasOpenAIInputImageValue(reqBody["input"]) || hasOpenAIInputImageValue(reqBody["messages"])
REDACTED

func hasOpenAIInputImageValue(value any) bool {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if hasOpenAIInputImageValue(item) {
				return true
		REDACTED
	REDACTED
	case map[string]any:
		if strings.TrimSpace(firstNonEmptyString(v["type"])) == "input_image" {
			return true
	REDACTED
		if _, ok := v["image_url"]; ok {
			return true
	REDACTED
		return hasOpenAIInputImageValue(v["content"])
REDACTED
	return false
REDACTED

func validateCodexSparkInput(reqBody map[string]any, model string) error {
	if !isCodexSparkModel(model) || !hasOpenAIInputImage(reqBody) {
		return nil
REDACTED
	return fmt.Errorf("model %q does not support image input", strings.TrimSpace(model))
REDACTED

func normalizeOpenAIResponsesImageGenerationTools(reqBody map[string]any) bool {
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
REDACTED
	tools, ok := rawTools.([]any)
	if !ok {
		return false
REDACTED

	modified := false
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if !ok || strings.TrimSpace(firstNonEmptyString(toolMap["type"])) != "image_generation" {
			continue
	REDACTED
		if _, ok := toolMap["output_format"]; !ok {
			if value := strings.TrimSpace(firstNonEmptyString(toolMap["format"])); value != "" {
				toolMap["output_format"] = value
				modified = true
		REDACTED
	REDACTED
		if _, ok := toolMap["output_compression"]; !ok {
			if value, exists := toolMap["compression"]; exists && value != nil {
				toolMap["output_compression"] = value
				modified = true
		REDACTED
	REDACTED
		if _, ok := toolMap["format"]; ok {
			delete(toolMap, "format")
			modified = true
	REDACTED
		if _, ok := toolMap["compression"]; ok {
			delete(toolMap, "compression")
			modified = true
	REDACTED
REDACTED
	return modified
REDACTED

func ensureOpenAIResponsesImageGenerationTool(reqBody map[string]any) bool {
	if len(reqBody) == 0 {
		return false
REDACTED
	if isCodexSparkModel(firstNonEmptyString(reqBody["model"])) {
		return false
REDACTED

	tool := map[string]any{
		"type":          "image_generation",
		"output_format": "png",
REDACTED

	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		reqBody["tools"] = []any{toolREDACTED
		return true
REDACTED

	tools, ok := rawTools.([]any)
	if !ok {
		reqBody["tools"] = []any{toolREDACTED
		return true
REDACTED
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if !ok {
			continue
	REDACTED
		if strings.TrimSpace(firstNonEmptyString(toolMap["type"])) == "image_generation" {
			return false
	REDACTED
REDACTED

	reqBody["tools"] = append(tools, tool)
	return true
REDACTED

func ensureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody map[string]any) bool {
	if len(reqBody) == 0 || !hasOpenAIImageGenerationTool(reqBody) {
		return false
REDACTED
	if isCodexSparkModel(firstNonEmptyString(reqBody["model"])) {
		return false
REDACTED
	if _, ok := reqBody["tool_choice"]; ok {
		return false
REDACTED
	reqBody["tool_choice"] = "auto"
	return true
REDACTED

func applyCodexImageGenerationBridgeInstructions(reqBody map[string]any) bool {
	if len(reqBody) == 0 || !hasOpenAIImageGenerationTool(reqBody) {
		return false
REDACTED
	if isCodexSparkModel(firstNonEmptyString(reqBody["model"])) {
		return false
REDACTED

	existing, _ := reqBody["instructions"].(string)
	if strings.Contains(existing, codexImageGenerationBridgeMarker) {
		return false
REDACTED

	existing = strings.TrimRight(existing, " \t\r\n")
	if strings.TrimSpace(existing) == "" {
		reqBody["instructions"] = codexImageGenerationBridgeText
		return true
REDACTED

	reqBody["instructions"] = existing + "\n\n" + codexImageGenerationBridgeText
	return true
REDACTED

func applyCodexSparkImageUnsupportedInstructions(reqBody map[string]any) bool {
	if len(reqBody) == 0 {
		return false
REDACTED
	existing, _ := reqBody["instructions"].(string)
	if strings.Contains(existing, codexSparkImageUnsupportedMarker) {
		return false
REDACTED
	existing = strings.TrimRight(existing, " \t\r\n")
	if strings.TrimSpace(existing) == "" {
		reqBody["instructions"] = codexSparkImageUnsupportedText
		return true
REDACTED
	reqBody["instructions"] = existing + "\n\n" + codexSparkImageUnsupportedText
	return true
REDACTED

func validateOpenAIResponsesImageModel(reqBody map[string]any, model string) error {
	if !hasOpenAIImageGenerationTool(reqBody) {
		return nil
REDACTED
	model = strings.TrimSpace(model)
	if !isOpenAIImageGenerationModel(model) {
		return nil
REDACTED
	return fmt.Errorf("/v1/responses image_generation requests require a Responses-capable text model; image-only model %q is not allowed", model)
REDACTED

func normalizeOpenAIResponsesImageOnlyModel(reqBody map[string]any) bool {
	if len(reqBody) == 0 {
		return false
REDACTED
	imageModel := strings.TrimSpace(firstNonEmptyString(reqBody["model"]))
	if !isOpenAIImageGenerationModel(imageModel) {
		return false
REDACTED

	modified := false
	tools, _ := reqBody["tools"].([]any)
	imageToolIndex := -1
	for i, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if !ok {
			continue
	REDACTED
		if strings.TrimSpace(firstNonEmptyString(toolMap["type"])) == "image_generation" {
			imageToolIndex = i
			break
	REDACTED
REDACTED
	if imageToolIndex < 0 {
		tools = append(tools, map[string]any{
			"type":  "image_generation",
			"model": imageModel,
	REDACTED)
		imageToolIndex = len(tools) - 1
		reqBody["tools"] = tools
		modified = true
REDACTED

	if toolMap, ok := tools[imageToolIndex].(map[string]any); ok {
		if strings.TrimSpace(firstNonEmptyString(toolMap["model"])) == "" {
			toolMap["model"] = imageModel
			modified = true
	REDACTED
		for _, key := range []string{
			"size",
			"quality",
			"background",
			"output_format",
			"output_compression",
			"moderation",
			"style",
			"partial_images",
	REDACTED {
			if value, exists := reqBody[key]; exists && value != nil {
				if _, toolHas := toolMap[key]; !toolHas {
					toolMap[key] = value
			REDACTED
				delete(reqBody, key)
				modified = true
		REDACTED
	REDACTED
REDACTED

	if prompt := strings.TrimSpace(firstNonEmptyString(reqBody["prompt"])); prompt != "" {
		if _, hasInput := reqBody["input"]; !hasInput {
			reqBody["input"] = prompt
	REDACTED
		delete(reqBody, "prompt")
		modified = true
REDACTED

	if _, ok := reqBody["tool_choice"]; !ok {
		reqBody["tool_choice"] = map[string]any{"type": "image_generation"REDACTED
		modified = true
REDACTED
	if imageModel != openAIImagesResponsesMainModel {
		modified = true
REDACTED
	reqBody["model"] = openAIImagesResponsesMainModel
	return modified
REDACTED

func normalizeOpenAIModelForUpstream(account *Account, model string) string {
	if account == nil || account.Type == AccountTypeOAuth {
		return normalizeCodexModel(model)
REDACTED
	return strings.TrimSpace(model)
REDACTED

func SupportsVerbosity(model string) bool {
	if !strings.HasPrefix(model, "gpt-") {
		return true
REDACTED

	var major, minor int
	n, _ := fmt.Sscanf(model, "gpt-%d.%d", &major, &minor)

	if major > 5 {
		return true
REDACTED
	if major < 5 {
		return false
REDACTED

	// gpt-5
	if n == 1 {
		return true
REDACTED

	return minor >= 3
REDACTED

func getNormalizedCodexModel(modelID string) string {
	key := codexModelLookupKey(modelID)
	if key == "" {
		return ""
REDACTED
	if mapped, ok := codexModelMap[key]; ok {
		return mapped
REDACTED
	return ""
REDACTED

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
		REDACTED
			switch t, _ := m["type"].(string); t {
			case "text", "input_text", "output_text":
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
			REDACTED
		REDACTED
	REDACTED
		return strings.Join(parts, "")
	default:
		return ""
REDACTED
REDACTED

// extractSystemMessagesFromInput scans input for role=="system", maps those
// items to developer, and mirrors their text into reqBody["instructions"].
// It preserves the input items so Responses JSON mode can still see JSON
// instructions in input messages.
func extractSystemMessagesFromInput(reqBody map[string]any) bool {
	input, ok := reqBody["input"].([]any)
	if !ok || len(input) == 0 {
		return false
REDACTED

	var systemTexts []string
	modified := false
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		if role, _ := m["role"].(string); role != "system" {
			continue
	REDACTED
		m["role"] = "developer"
		modified = true
		if text := extractTextFromContent(m["content"]); text != "" {
			systemTexts = append(systemTexts, text)
	REDACTED
REDACTED

	if len(systemTexts) == 0 {
		return modified
REDACTED

	extracted := strings.Join(systemTexts, "\n\n")
	if existing, ok := reqBody["instructions"].(string); ok && strings.TrimSpace(existing) != "" {
		reqBody["instructions"] = extracted + "\n\n" + existing
REDACTED else {
		reqBody["instructions"] = extracted
REDACTED
	return true
REDACTED

func extractPromptLikeInstructionsFromInput(reqBody map[string]any) string {
	input, ok := reqBody["input"].([]any)
	if !ok || len(input) == 0 {
		return ""
REDACTED
	var texts []string
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		role, _ := m["role"].(string)
		switch role {
		case "developer", "system":
			if text := strings.TrimSpace(extractTextFromContent(m["content"])); text != "" {
				texts = append(texts, text)
		REDACTED
	REDACTED
REDACTED
	return strings.Join(texts, "\n\n")
REDACTED

// defaultCodexSynthInstructions 返回合成路径在 instructions 为空时应填入的默认提示词。
//
// 按 model 选择真实 Codex CLI 的 base instructions（codex 系→GPT-5-Codex，
// gpt-5.2→GPT-5.2，gpt-5.1/gpt-5→GPT-5.1），使合成请求在提示词层面贴近真实 Codex 行为；
// 若内嵌 prompt 意外为空，回退到最小占位符以保证字段非空。
func defaultCodexSynthInstructions(model string) string {
	if instructions := strings.TrimSpace(openai.CodexBaseInstructionsForModel(model)); instructions != "" {
		return instructions
REDACTED
	return "You are a helpful coding assistant."
REDACTED

// ensureCodexReasoningInclude 在请求带 reasoning 时补齐 include:["reasoning.encrypted_content"]。
//
// 真实 Codex 在 reasoning 存在时总会请求加密推理内容（ChatGPT/store=false 场景下用于上下文回放）。
// 该函数为加法式、幂等：仅在 include 缺失或未包含该项时追加；对非数组的异常 include 不做破坏性改写。
func ensureCodexReasoningInclude(reqBody map[string]any) bool {
	reasoning, ok := reqBody["reasoning"].(map[string]any)
	if !ok || len(reasoning) == 0 {
		return false
REDACTED
	const encrypted = "reasoning.encrypted_content"
	switch existing := reqBody["include"].(type) {
	case nil:
		reqBody["include"] = []any{encryptedREDACTED
		return true
	case []any:
		for _, v := range existing {
			if s, ok := v.(string); ok && s == encrypted {
				return false
		REDACTED
	REDACTED
		reqBody["include"] = append(existing, encrypted)
		return true
	default:
		// include 为非预期类型时保持原样，避免破坏调用方意图。
		return false
REDACTED
REDACTED

// applyCodexClientMetadata 在请求体补齐 client_metadata["x-codex-installation-id"]，
// 取值为账号真实的 openai_device_id（最新 Codex 在请求体携带的安装标识）。
//
// 加法式、幂等：仅在账号存在 device_id 且该键缺失时注入，绝不覆盖既有 client_metadata
// （如 turn metadata），也不伪造——无 device_id 时不写入。
func applyCodexClientMetadata(reqBody map[string]any, account *Account) bool {
	if account == nil {
		return false
REDACTED
	deviceID := strings.TrimSpace(account.GetOpenAIDeviceID())
	if deviceID == "" {
		return false
REDACTED
	const key = "x-codex-installation-id"
	switch existing := reqBody["client_metadata"].(type) {
	case map[string]any:
		if v, ok := existing[key].(string); ok && strings.TrimSpace(v) != "" {
			return false
	REDACTED
		existing[key] = deviceID
		reqBody["client_metadata"] = existing
		return true
	case map[string]string:
		if strings.TrimSpace(existing[key]) != "" {
			return false
	REDACTED
		next := make(map[string]any, len(existing)+1)
		for k, v := range existing {
			next[k] = v
	REDACTED
		next[key] = deviceID
		reqBody["client_metadata"] = next
		return true
	case nil:
		reqBody["client_metadata"] = map[string]any{key: deviceIDREDACTED
		return true
	default:
		return false
REDACTED
REDACTED

// applyInstructions 处理 instructions 字段：仅在 instructions 为空时填充默认值。
func applyInstructions(reqBody map[string]any, isCodexCLI bool) bool {
	if !isInstructionsEmpty(reqBody) {
		return false
REDACTED
	model, _ := reqBody["model"].(string)
	reqBody["instructions"] = defaultCodexSynthInstructions(model)
	return true
REDACTED

// isInstructionsEmpty 检查 instructions 字段是否为空
// 处理以下情况：字段不存在、nil、空字符串、纯空白字符串
func isInstructionsEmpty(reqBody map[string]any) bool {
	val, exists := reqBody["instructions"]
	if !exists {
		return true
REDACTED
	if val == nil {
		return true
REDACTED
	str, ok := val.(string)
	if !ok {
		return true
REDACTED
	return strings.TrimSpace(str) == ""
REDACTED

type codexInputFilterOptions struct {
	PreserveReferences bool
	PreserveCallIDs    bool
REDACTED

// filterCodexInput 按需过滤 item_reference 与 id。
// preserveReferences 为 true 时保持引用与 id，以满足续链请求对上下文的依赖。
func filterCodexInput(input []any, preserveReferences bool) []any {
	return filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: preserveReferences,
REDACTED)
REDACTED

func filterCodexInputWithOptions(input []any, opts codexInputFilterOptions) []any {
	filtered := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
	REDACTED
		typ, _ := m["type"].(string)

		// chatgpt.com codex backend (OAuth path) does not persist reasoning
		// items because applyCodexOAuthTransform forces store=false. Any rs_*
		// reference replayed in input is guaranteed to 404 upstream
		// ("Item with id 'rs_...' not found"). Drop reasoning items entirely.
		if typ == "reasoning" {
			continue
	REDACTED

		// 仅修正真正的 tool/function call 标识，避免误改普通 message/reasoning id；
		// 若 item_reference 指向 legacy call_* 标识，则仅修正该引用本身。
		fixCallIDPrefix := func(id string) string {
			if opts.PreserveCallIDs {
				return id
		REDACTED
			if id == "" || strings.HasPrefix(id, "fc") {
				return id
		REDACTED
			if strings.HasPrefix(id, "call_") {
				return "fc_" + strings.TrimPrefix(id, "call_")
		REDACTED
			return "fc_" + id
	REDACTED

		if typ == "item_reference" {
			if !opts.PreserveReferences {
				continue
		REDACTED
			newItem := make(map[string]any, len(m))
			for key, value := range m {
				newItem[key] = value
		REDACTED
			if id, ok := newItem["id"].(string); ok && strings.HasPrefix(id, "call_") {
				newItem["id"] = fixCallIDPrefix(id)
		REDACTED
			filtered = append(filtered, newItem)
			continue
	REDACTED

		newItem := m
		copied := false
		// 仅在需要修改字段时创建副本，避免直接改写原始输入。
		ensureCopy := func() {
			if copied {
				return
		REDACTED
			newItem = make(map[string]any, len(m))
			for key, value := range m {
				newItem[key] = value
		REDACTED
			copied = true
	REDACTED

		if isCodexToolCallItemType(typ) {
			callID, ok := m["call_id"].(string)
			if !ok || strings.TrimSpace(callID) == "" {
				if id, ok := m["id"].(string); ok && strings.TrimSpace(id) != "" {
					callID = id
					ensureCopy()
					newItem["call_id"] = callID
			REDACTED
		REDACTED

			if callID != "" {
				fixedCallID := fixCallIDPrefix(callID)
				if fixedCallID != callID {
					ensureCopy()
					newItem["call_id"] = fixedCallID
			REDACTED
		REDACTED
	REDACTED

		if !isCodexToolCallItemType(typ) {
			ensureCopy()
			delete(newItem, "call_id")
	REDACTED

		if codexInputItemRequiresName(typ) {
			if strings.TrimSpace(firstNonEmptyString(m["name"])) == "" {
				name := firstNonEmptyString(m["tool_name"])
				if name == "" {
					if function, ok := m["function"].(map[string]any); ok {
						name = firstNonEmptyString(function["name"])
				REDACTED
			REDACTED
				if name == "" {
					name = "tool"
			REDACTED
				ensureCopy()
				newItem["name"] = name
		REDACTED
	REDACTED

		if !opts.PreserveReferences {
			ensureCopy()
			delete(newItem, "id")
	REDACTED

		filtered = append(filtered, newItem)
REDACTED
	return filtered
REDACTED

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
REDACTED
REDACTED

func codexInputItemRequiresName(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "function_call", "custom_tool_call", "mcp_tool_call":
		return true
	default:
		return false
REDACTED
REDACTED

func normalizeCodexTools(reqBody map[string]any) bool {
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
REDACTED
	tools, ok := rawTools.([]any)
	if !ok {
		return false
REDACTED

	modified := false
	validTools := make([]any, 0, len(tools))

	for _, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			// Keep unknown structure as-is to avoid breaking upstream behavior.
			validTools = append(validTools, tool)
			continue
	REDACTED

		toolType, _ := toolMap["type"].(string)
		toolType = strings.TrimSpace(toolType)
		if toolType != "function" {
			validTools = append(validTools, toolMap)
			continue
	REDACTED

		// OpenAI Responses-style tools use top-level name/parameters.
		if name, ok := toolMap["name"].(string); ok && strings.TrimSpace(name) != "" {
			validTools = append(validTools, toolMap)
			continue
	REDACTED

		// ChatCompletions-style tools use {type:"function", function:{...REDACTEDREDACTED.
		functionValue, hasFunction := toolMap["function"]
		function, ok := functionValue.(map[string]any)
		if !hasFunction || functionValue == nil || !ok || function == nil {
			// Drop invalid function tools.
			modified = true
			continue
	REDACTED

		if _, ok := toolMap["name"]; !ok {
			if name, ok := function["name"].(string); ok && strings.TrimSpace(name) != "" {
				toolMap["name"] = name
				modified = true
		REDACTED
	REDACTED
		if _, ok := toolMap["description"]; !ok {
			if desc, ok := function["description"].(string); ok && strings.TrimSpace(desc) != "" {
				toolMap["description"] = desc
				modified = true
		REDACTED
	REDACTED
		if _, ok := toolMap["parameters"]; !ok {
			if params, ok := function["parameters"]; ok {
				toolMap["parameters"] = params
				modified = true
		REDACTED
	REDACTED
		if _, ok := toolMap["strict"]; !ok {
			if strict, ok := function["strict"]; ok {
				toolMap["strict"] = strict
				modified = true
		REDACTED
	REDACTED

		validTools = append(validTools, toolMap)
REDACTED

	if modified {
		reqBody["tools"] = validTools
REDACTED

	return modified
REDACTED
