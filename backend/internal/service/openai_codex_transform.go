package service

import (
	"strings"
)

var codexModelMap = map[string]string{
	"gpt-5.4":                    "gpt-5.4",
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
	"gpt-5.3-codex-spark":        "gpt-5.3-codex",
	"gpt-5.3-codex-spark-low":    "gpt-5.3-codex",
	"gpt-5.3-codex-spark-medium": "gpt-5.3-codex",
	"gpt-5.3-codex-spark-high":   "gpt-5.3-codex",
	"gpt-5.3-codex-spark-xhigh":  "gpt-5.3-codex",
	"gpt-5.3-codex-low":          "gpt-5.3-codex",
	"gpt-5.3-codex-medium":       "gpt-5.3-codex",
	"gpt-5.3-codex-high":         "gpt-5.3-codex",
	"gpt-5.3-codex-xhigh":        "gpt-5.3-codex",
	"gpt-5.1-codex":              "gpt-5.1-codex",
	"gpt-5.1-codex-low":          "gpt-5.1-codex",
	"gpt-5.1-codex-medium":       "gpt-5.1-codex",
	"gpt-5.1-codex-high":         "gpt-5.1-codex",
	"gpt-5.1-codex-max":          "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-low":      "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-medium":   "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-high":     "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-xhigh":    "gpt-5.1-codex-max",
	"gpt-5.2":                    "gpt-5.2",
	"gpt-5.2-none":               "gpt-5.2",
	"gpt-5.2-low":                "gpt-5.2",
	"gpt-5.2-medium":             "gpt-5.2",
	"gpt-5.2-high":               "gpt-5.2",
	"gpt-5.2-xhigh":              "gpt-5.2",
	"gpt-5.2-codex":              "gpt-5.2-codex",
	"gpt-5.2-codex-low":          "gpt-5.2-codex",
	"gpt-5.2-codex-medium":       "gpt-5.2-codex",
	"gpt-5.2-codex-high":         "gpt-5.2-codex",
	"gpt-5.2-codex-xhigh":        "gpt-5.2-codex",
	"gpt-5.1-codex-mini":         "gpt-5.1-codex-mini",
	"gpt-5.1-codex-mini-medium":  "gpt-5.1-codex-mini",
	"gpt-5.1-codex-mini-high":    "gpt-5.1-codex-mini",
	"gpt-5.1":                    "gpt-5.1",
	"gpt-5.1-none":               "gpt-5.1",
	"gpt-5.1-low":                "gpt-5.1",
	"gpt-5.1-medium":             "gpt-5.1",
	"gpt-5.1-high":               "gpt-5.1",
	"gpt-5.1-chat-latest":        "gpt-5.1",
	"gpt-5-codex":                "gpt-5.1-codex",
	"codex-mini-latest":          "gpt-5.1-codex-mini",
	"gpt-5-codex-mini":           "gpt-5.1-codex-mini",
	"gpt-5-codex-mini-medium":    "gpt-5.1-codex-mini",
	"gpt-5-codex-mini-high":      "gpt-5.1-codex-mini",
	"gpt-5":                      "gpt-5.1",
	"gpt-5-mini":                 "gpt-5.1",
	"gpt-5-nano":                 "gpt-5.1",
REDACTED

type codexTransformResult struct {
	Modified        bool
	NormalizedModel string
	PromptCacheKey  string
REDACTED

func applyCodexOAuthTransform(reqBody map[string]any, isCodexCLI bool, isCompact bool) codexTransformResult {
	result := codexTransformResult{REDACTED
	// 工具续链需求会影响存储策略与 input 过滤逻辑。
	needsToolContinuation := NeedsToolContinuation(reqBody)

	model := ""
	if v, ok := reqBody["model"].(string); ok {
		model = v
REDACTED
	normalizedModel := normalizeCodexModel(model)
	if normalizedModel != "" {
		if model != normalizedModel {
			reqBody["model"] = normalizedModel
			result.Modified = true
	REDACTED
		result.NormalizedModel = normalizedModel
REDACTED

	if isCompact {
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

	// Strip parameters unsupported by codex models via the Responses API.
	for _, key := range []string{
		"max_output_tokens",
		"max_completion_tokens",
		"temperature",
		"top_p",
		"frequency_penalty",
		"presence_penalty",
REDACTED {
		if _, ok := reqBody[key]; ok {
			delete(reqBody, key)
			result.Modified = true
	REDACTED
REDACTED

	if normalizeCodexTools(reqBody) {
		result.Modified = true
REDACTED

	if v, ok := reqBody["prompt_cache_key"].(string); ok {
		result.PromptCacheKey = strings.TrimSpace(v)
REDACTED

	// instructions 处理逻辑：根据是否是 Codex CLI 分别调用不同方法
	if applyInstructions(reqBody, isCodexCLI) {
		result.Modified = true
REDACTED

	// 续链场景保留 item_reference 与 id，避免 call_id 上下文丢失。
	if input, ok := reqBody["input"].([]any); ok {
		input = filterCodexInput(input, needsToolContinuation)
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

func normalizeCodexModel(model string) string {
	if model == "" {
		return "gpt-5.1"
REDACTED

	modelID := model
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
REDACTED

	if mapped := getNormalizedCodexModel(modelID); mapped != "" {
		return mapped
REDACTED

	normalized := strings.ToLower(modelID)

	if strings.Contains(normalized, "gpt-5.4") || strings.Contains(normalized, "gpt 5.4") {
		return "gpt-5.4"
REDACTED
	if strings.Contains(normalized, "gpt-5.2-codex") || strings.Contains(normalized, "gpt 5.2 codex") {
		return "gpt-5.2-codex"
REDACTED
	if strings.Contains(normalized, "gpt-5.2") || strings.Contains(normalized, "gpt 5.2") {
		return "gpt-5.2"
REDACTED
	if strings.Contains(normalized, "gpt-5.3-codex") || strings.Contains(normalized, "gpt 5.3 codex") {
		return "gpt-5.3-codex"
REDACTED
	if strings.Contains(normalized, "gpt-5.3") || strings.Contains(normalized, "gpt 5.3") {
		return "gpt-5.3-codex"
REDACTED
	if strings.Contains(normalized, "gpt-5.1-codex-max") || strings.Contains(normalized, "gpt 5.1 codex max") {
		return "gpt-5.1-codex-max"
REDACTED
	if strings.Contains(normalized, "gpt-5.1-codex-mini") || strings.Contains(normalized, "gpt 5.1 codex mini") {
		return "gpt-5.1-codex-mini"
REDACTED
	if strings.Contains(normalized, "codex-mini-latest") ||
		strings.Contains(normalized, "gpt-5-codex-mini") ||
		strings.Contains(normalized, "gpt 5 codex mini") {
		return "codex-mini-latest"
REDACTED
	if strings.Contains(normalized, "gpt-5.1-codex") || strings.Contains(normalized, "gpt 5.1 codex") {
		return "gpt-5.1-codex"
REDACTED
	if strings.Contains(normalized, "gpt-5.1") || strings.Contains(normalized, "gpt 5.1") {
		return "gpt-5.1"
REDACTED
	if strings.Contains(normalized, "codex") {
		return "gpt-5.1-codex"
REDACTED
	if strings.Contains(normalized, "gpt-5") || strings.Contains(normalized, "gpt 5") {
		return "gpt-5.1"
REDACTED

	return "gpt-5.1"
REDACTED

func getNormalizedCodexModel(modelID string) string {
	if modelID == "" {
		return ""
REDACTED
	if mapped, ok := codexModelMap[modelID]; ok {
		return mapped
REDACTED
	lower := strings.ToLower(modelID)
	for key, value := range codexModelMap {
		if strings.ToLower(key) == lower {
			return value
	REDACTED
REDACTED
	return ""
REDACTED

// applyInstructions 处理 instructions 字段：仅在 instructions 为空时填充默认值。
func applyInstructions(reqBody map[string]any, isCodexCLI bool) bool {
	if !isInstructionsEmpty(reqBody) {
		return false
REDACTED
	reqBody["instructions"] = "You are a helpful coding assistant."
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

// filterCodexInput 按需过滤 item_reference 与 id。
// preserveReferences 为 true 时保持引用与 id，以满足续链请求对上下文的依赖。
func filterCodexInput(input []any, preserveReferences bool) []any {
	filtered := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
	REDACTED
		typ, _ := m["type"].(string)
		if typ == "item_reference" {
			if !preserveReferences {
				continue
		REDACTED
			newItem := make(map[string]any, len(m))
			for key, value := range m {
				newItem[key] = value
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
			if callID, ok := m["call_id"].(string); !ok || strings.TrimSpace(callID) == "" {
				if id, ok := m["id"].(string); ok && strings.TrimSpace(id) != "" {
					ensureCopy()
					newItem["call_id"] = id
			REDACTED
		REDACTED
	REDACTED

		if !preserveReferences {
			ensureCopy()
			delete(newItem, "id")
			if !isCodexToolCallItemType(typ) {
				delete(newItem, "call_id")
		REDACTED
	REDACTED

		filtered = append(filtered, newItem)
REDACTED
	return filtered
REDACTED

func isCodexToolCallItemType(typ string) bool {
	if typ == "" {
		return false
REDACTED
	return strings.HasSuffix(typ, "_call") || strings.HasSuffix(typ, "_call_output")
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
