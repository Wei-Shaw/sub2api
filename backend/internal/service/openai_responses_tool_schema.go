package service

import (
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	// 工具定义在多轮历史里最多再嵌套一层 tools，留出余量后截断，避免畸形请求体
	// 造成无界递归。
	openAIResponsesToolSchemaMaxDepth = 4
	// JSON Schema 里 type 只能是字符串或字符串数组；显式 null 无论哪个方言都非法，
	// 补成 object 与 upstream 对该工具的实际期望一致。
	openAIResponsesToolSchemaFallbackType = "object"
)

// sanitizeOpenAIResponsesToolParameterTypes 修正请求体中显式为 null 的
// tools[].parameters.type。
//
// Codex Desktop 内置的 automation_update 工具会带 parameters.type = null，
// OpenAI 直接回 400 invalid_function_parameters，而网关把该状态归一成可重试的
// 502 upstream_error；该工具定义又会沉进多轮历史，导致之后每一轮继续失败并在
// 账号池里反复重放同一份坏 Schema。
//
// 只修正显式 null：缺失 type 的 Schema 本身合法（等价于不约束），补写会收窄
// 客户端语义，因此保持原样。
func sanitizeOpenAIResponsesToolParameterTypes(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
REDACTED

	paths := make([]string, 0, 2)
	collectOpenAIResponsesToolSchemaNullTypePaths(gjson.GetBytes(body, "tools"), "tools", 0, &paths)
	if input := gjson.GetBytes(body, "input"); input.IsArray() {
		index := 0
		input.ForEach(func(_, item gjson.Result) bool {
			itemPath := fmt.Sprintf("input.%d.tools", index)
			index++
			if item.IsObject() {
				collectOpenAIResponsesToolSchemaNullTypePaths(item.Get("tools"), itemPath, 0, &paths)
		REDACTED
			return true
	REDACTED)
REDACTED
	if len(paths) == 0 {
		return body, false, nil
REDACTED

	sanitized := body
	for _, path := range paths {
		next, err := sjson.SetBytes(sanitized, path, openAIResponsesToolSchemaFallbackType)
		if err != nil {
			return body, false, fmt.Errorf("normalize %s: %w", path, err)
	REDACTED
		sanitized = next
REDACTED
	return sanitized, true, nil
REDACTED

// collectOpenAIResponsesToolSchemaNullTypePaths 收集一个 tools 数组里所有需要修正的
// parameters.type 路径。不按 tool type 过滤：null 的 schema type 在 function、
// custom 以及任何 hosted 工具上都同样非法。
func collectOpenAIResponsesToolSchemaNullTypePaths(tools gjson.Result, basePath string, depth int, paths *[]string) {
	if depth > openAIResponsesToolSchemaMaxDepth || !tools.IsArray() {
		return
REDACTED
	index := 0
	tools.ForEach(func(_, tool gjson.Result) bool {
		toolPath := fmt.Sprintf("%s.%d", basePath, index)
		index++
		if !tool.IsObject() {
			return true
	REDACTED
		// Responses 形态用顶层 parameters，ChatCompletions 形态用 function.parameters，
		// 两种都可能出现在 Responses 请求里（见 normalizeCodexTools）。
		for _, suffix := range []string{"parameters", "function.parameters"REDACTED {
			params := tool.Get(suffix)
			if !params.IsObject() {
				continue
		REDACTED
			if typ := params.Get("type"); typ.Exists() && typ.Type == gjson.Null {
				*paths = append(*paths, toolPath+"."+suffix+".type")
		REDACTED
	REDACTED
		// 历史输入里的工具定义会再嵌套一层 tools（upstream 报错路径形如
		// input[234].tools[0].tools[3].parameters）。
		collectOpenAIResponsesToolSchemaNullTypePaths(tool.Get("tools"), toolPath+".tools", depth+1, paths)
		return true
REDACTED)
REDACTED
