package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const defaultBedrockRegion = "us-east-1"

var bedrockCrossRegionPrefixes = []string{"us.", "eu.", "apac.", "jp.", "au.", "us-gov.", "global."}

// BedrockCrossRegionPrefix 根据 AWS Region 返回 Bedrock 跨区域推理的模型 ID 前缀
// 参考: https://docs.aws.amazon.com/bedrock/latest/userguide/inference-profiles-support.html
func BedrockCrossRegionPrefix(region string) string {
	switch {
	case strings.HasPrefix(region, "us-gov"):
		return "us-gov" // GovCloud 使用独立的 us-gov 前缀
	case strings.HasPrefix(region, "us-"):
		return "us"
	case strings.HasPrefix(region, "eu-"):
		return "eu"
	case region == "ap-northeast-1":
		return "jp" // 日本区域使用独立的 jp 前缀（AWS 官方定义）
	case region == "ap-southeast-2":
		return "au" // 澳大利亚区域使用独立的 au 前缀（AWS 官方定义）
	case strings.HasPrefix(region, "ap-"):
		return "apac" // 其余亚太区域使用通用 apac 前缀
	case strings.HasPrefix(region, "ca-"):
		return "us" // 加拿大区域使用 us 前缀的跨区域推理
	case strings.HasPrefix(region, "sa-"):
		return "us" // 南美区域使用 us 前缀的跨区域推理
	default:
		return "us"
	}
}

// AdjustBedrockModelRegionPrefix 将模型 ID 的区域前缀替换为与当前 AWS Region 匹配的前缀
// 例如 region=eu-west-1 时，"us.anthropic.claude-opus-4-6-v1" → "eu.anthropic.claude-opus-4-6-v1"
// 特殊值 region="global" 强制使用 global. 前缀
func AdjustBedrockModelRegionPrefix(modelID, region string) string {
	var targetPrefix string
	if region == "global" {
		targetPrefix = "global"
	} else {
		targetPrefix = BedrockCrossRegionPrefix(region)
	}

	for _, p := range bedrockCrossRegionPrefixes {
		if strings.HasPrefix(modelID, p) {
			if p == targetPrefix+"." {
				return modelID // 前缀已匹配，无需替换
			}
			return targetPrefix + "." + modelID[len(p):]
		}
	}

	// 模型 ID 没有已知区域前缀（如 "anthropic.claude-..."），不做修改
	return modelID
}

func bedrockRuntimeRegion(account *Account) string {
	if account == nil {
		return defaultBedrockRegion
	}
	if region := account.GetCredential("aws_region"); region != "" {
		return region
	}
	return defaultBedrockRegion
}

func shouldForceBedrockGlobal(account *Account) bool {
	return account != nil && account.GetCredential("aws_force_global") == "true"
}

func isRegionalBedrockModelID(modelID string) bool {
	for _, prefix := range bedrockCrossRegionPrefixes {
		if strings.HasPrefix(modelID, prefix) {
			return true
		}
	}
	return false
}

func isLikelyBedrockModelID(modelID string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "arn:") {
		return true
	}
	for _, prefix := range []string{
		"anthropic.",
		"amazon.",
		"meta.",
		"mistral.",
		"cohere.",
		"ai21.",
		"deepseek.",
		"stability.",
		"writer.",
		"nova.",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return isRegionalBedrockModelID(lower)
}

func normalizeBedrockModelID(modelID string) (normalized string, shouldAdjustRegion bool, ok bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "", false, false
	}
	if mapped, exists := domain.DefaultBedrockModelMapping[modelID]; exists {
		return mapped, true, true
	}
	if isRegionalBedrockModelID(modelID) {
		return modelID, true, true
	}
	if isLikelyBedrockModelID(modelID) {
		return modelID, false, true
	}
	return "", false, false
}

// ResolveBedrockModelID resolves a requested Claude model into a Bedrock model ID.
// It applies account model_mapping first, then default Bedrock aliases, and finally
// adjusts Anthropic cross-region prefixes to match the account region.
func ResolveBedrockModelID(account *Account, requestedModel string) (string, bool) {
	if account == nil {
		return "", false
	}

	mappedModel := account.GetMappedModel(requestedModel)
	modelID, shouldAdjustRegion, ok := normalizeBedrockModelID(mappedModel)
	if !ok {
		return "", false
	}
	if shouldAdjustRegion {
		targetRegion := bedrockRuntimeRegion(account)
		if shouldForceBedrockGlobal(account) {
			targetRegion = "global"
		}
		modelID = AdjustBedrockModelRegionPrefix(modelID, targetRegion)
	}
	return modelID, true
}

// BuildBedrockURL 构建 Bedrock InvokeModel 的 URL
// stream=true 时使用 invoke-with-response-stream 端点
// modelID 中的特殊字符会被 URL 编码（与 litellm 的 urllib.parse.quote(safe="") 对齐）
func BuildBedrockURL(region, modelID string, stream bool) string {
	if region == "" {
		region = defaultBedrockRegion
	}
	encodedModelID := url.PathEscape(modelID)
	// url.PathEscape 不编码冒号（RFC 允许 path 中出现 ":"），
	// 但 AWS Bedrock 期望模型 ID 中的冒号被编码为 %3A
	encodedModelID = strings.ReplaceAll(encodedModelID, ":", "%3A")
	if stream {
		return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke-with-response-stream", region, encodedModelID)
	}
	return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke", region, encodedModelID)
}

// PrepareBedrockRequestBody 处理请求体以适配 Bedrock API
//  1. 注入 anthropic_version
//  2. 注入 anthropic_beta（从客户端 anthropic-beta 头解析）
//  3. 移除 Bedrock 不支持的字段（model, stream, output_format, output_config）
//  4. 移除工具定义中的 custom 字段（Claude Code 会发送 custom: {defer_loading: true}）
//  5. 清理 cache_control 中 Bedrock 不支持的字段（scope, ttl）
func PrepareBedrockRequestBody(body []byte, modelID string, betaHeader string) ([]byte, error) {
	betaTokens := ResolveBedrockBetaTokens(betaHeader, body, modelID)
	return PrepareBedrockRequestBodyWithTokens(body, modelID, betaTokens)
}

// PrepareBedrockRequestBodyWithTokens prepares a Bedrock request using pre-resolved beta tokens.
func PrepareBedrockRequestBodyWithTokens(body []byte, modelID string, betaTokens []string) ([]byte, error) {
	var err error

	// 注入 anthropic_version（Bedrock 要求）
	body, err = sjson.SetBytes(body, "anthropic_version", "bedrock-2023-05-31")
	if err != nil {
		return nil, fmt.Errorf("inject anthropic_version: %w", err)
	}

	// 注入 anthropic_beta（Bedrock Invoke 通过请求体传递 beta 头，而非 HTTP 头）
	// 1. 从客户端 anthropic-beta header 解析
	// 2. 根据请求体内容自动补齐必要的 beta token
	//    参考 litellm: AnthropicModelInfo.get_anthropic_beta_list() + _get_tool_search_beta_header_for_bedrock()
	if len(betaTokens) > 0 {
		body, err = sjson.SetBytes(body, "anthropic_beta", betaTokens)
		if err != nil {
			return nil, fmt.Errorf("inject anthropic_beta: %w", err)
		}
	}

	// 移除 model 字段（Bedrock 通过 URL 指定模型）
	body, err = sjson.DeleteBytes(body, "model")
	if err != nil {
		return nil, fmt.Errorf("remove model field: %w", err)
	}

	// 移除 stream 字段（Bedrock 通过不同端点控制流式，不接受请求体中的 stream 字段）
	body, err = sjson.DeleteBytes(body, "stream")
	if err != nil {
		return nil, fmt.Errorf("remove stream field: %w", err)
	}

	// 转换 output_format（Bedrock Invoke 不支持此字段，但可将 schema 内联到最后一条 user message）
	// 参考 litellm: _convert_output_format_to_inline_schema()
	body = convertOutputFormatToInlineSchema(body)

	// 移除 output_config 字段（Bedrock Invoke 不支持）
	body, err = sjson.DeleteBytes(body, "output_config")
	if err != nil {
		return nil, fmt.Errorf("remove output_config field: %w", err)
	}

	// 移除工具定义中的 custom 字段
	// Claude Code (v2.1.69+) 在 tool 定义中发送 custom: {defer_loading: true}，
	// Anthropic API 接受但 Bedrock 会拒绝并报 "Extra inputs are not permitted"
	body = removeCustomFieldFromTools(body)

	// 清理 cache_control 中 Bedrock 不支持的字段
	body = sanitizeBedrockCacheControl(body, modelID)

	return body, nil
}

// ResolveBedrockBetaTokens computes the final Bedrock beta token list before policy filtering.
func ResolveBedrockBetaTokens(betaHeader string, body []byte, modelID string) []string {
	betaTokens := parseAnthropicBetaHeader(betaHeader)
	betaTokens = autoInjectBedrockBetaTokens(betaTokens, body, modelID)
	return filterBedrockBetaTokens(betaTokens)
}

// convertOutputFormatToInlineSchema 将 output_format 中的 JSON schema 内联到最后一条 user message
// Bedrock Invoke 不支持 output_format 参数，litellm 的做法是将 schema 追加到用户消息中
// 参考: litellm AmazonAnthropicClaudeMessagesConfig._convert_output_format_to_inline_schema()
func convertOutputFormatToInlineSchema(body []byte) []byte {
	outputFormat := gjson.GetBytes(body, "output_format")
	if !outputFormat.Exists() || !outputFormat.IsObject() {
		return body
	}

	// 先从请求体中移除 output_format
	body, _ = sjson.DeleteBytes(body, "output_format")

	schema := outputFormat.Get("schema")
	if !schema.Exists() {
		return body
	}

	// 找到最后一条 user message
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}
	msgArr := messages.Array()
	lastUserIdx := -1
	for i := len(msgArr) - 1; i >= 0; i-- {
		if msgArr[i].Get("role").String() == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		return body
	}

	// 将 schema 序列化为 JSON 文本追加到该 message 的 content 数组
	schemaJSON, err := json.Marshal(json.RawMessage(schema.Raw))
	if err != nil {
		return body
	}

	content := msgArr[lastUserIdx].Get("content")
	basePath := fmt.Sprintf("messages.%d.content", lastUserIdx)

	if content.IsArray() {
		// 追加一个 text block 到 content 数组末尾
		idx := len(content.Array())
		body, _ = sjson.SetBytes(body, fmt.Sprintf("%s.%d.type", basePath, idx), "text")
		body, _ = sjson.SetBytes(body, fmt.Sprintf("%s.%d.text", basePath, idx), string(schemaJSON))
	} else if content.Type == gjson.String {
		// content 是纯字符串，转换为数组格式
		originalText := content.String()
		body, _ = sjson.SetBytes(body, basePath, []map[string]string{
			{"type": "text", "text": originalText},
			{"type": "text", "text": string(schemaJSON)},
		})
	}

	return body
}

// removeCustomFieldFromTools 移除 tools 数组中每个工具定义的 custom 字段
func removeCustomFieldFromTools(body []byte) []byte {
	tools := gjson.GetBytes(body, "tools")
	if !tools.Exists() || !tools.IsArray() {
		return body
	}
	var err error
	for i := range tools.Array() {
		body, err = sjson.DeleteBytes(body, fmt.Sprintf("tools.%d.custom", i))
		if err != nil {
			// 删除失败不影响整体流程，跳过
			continue
		}
	}
	return body
}

// claudeVersionRe 匹配 Claude 模型 ID 中的版本号部分
// 支持 claude-{tier}-{major}-{minor} 和 claude-{tier}-{major}.{minor} 格式
var claudeVersionRe = regexp.MustCompile(`claude-(?:haiku|sonnet|opus)-(\d+)[-.](\d+)`)

// isBedrockClaude45OrNewer 判断 Bedrock 模型 ID 是否为 Claude 4.5 或更新版本
// Claude 4.5+ 支持 cache_control 中的 ttl 字段（"5m" 和 "1h"）
func isBedrockClaude45OrNewer(modelID string) bool {
	lower := strings.ToLower(modelID)
	matches := claudeVersionRe.FindStringSubmatch(lower)
	if matches == nil {
		return false
	}
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	return major > 4 || (major == 4 && minor >= 5)
}

// sanitizeBedrockCacheControl 清理 system 和 messages 中 cache_control 里
// Bedrock 不支持的字段：
//   - scope：Bedrock 不支持（如 "global" 跨请求缓存）
//   - ttl：仅 Claude 4.5+ 支持 "5m" 和 "1h"，旧模型需要移除
func sanitizeBedrockCacheControl(body []byte, modelID string) []byte {
	isClaude45 := isBedrockClaude45OrNewer(modelID)

	// 清理 system 数组中的 cache_control
	systemArr := gjson.GetBytes(body, "system")
	if systemArr.Exists() && systemArr.IsArray() {
		for i, item := range systemArr.Array() {
			if !item.IsObject() {
				continue
			}
			cc := item.Get("cache_control")
			if !cc.Exists() || !cc.IsObject() {
				continue
			}
			body = deleteCacheControlUnsupportedFields(body, fmt.Sprintf("system.%d.cache_control", i), cc, isClaude45)
		}
	}

	// 清理 messages 中的 cache_control
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}
	for mi, msg := range messages.Array() {
		if !msg.IsObject() {
			continue
		}
		content := msg.Get("content")
		if !content.Exists() || !content.IsArray() {
			continue
		}
		for ci, block := range content.Array() {
			if !block.IsObject() {
				continue
			}
			cc := block.Get("cache_control")
			if !cc.Exists() || !cc.IsObject() {
				continue
			}
			body = deleteCacheControlUnsupportedFields(body, fmt.Sprintf("messages.%d.content.%d.cache_control", mi, ci), cc, isClaude45)
		}
	}

	return body
}

// deleteCacheControlUnsupportedFields 删除给定 cache_control 路径下 Bedrock 不支持的字段
func deleteCacheControlUnsupportedFields(body []byte, basePath string, cc gjson.Result, isClaude45 bool) []byte {
	// Bedrock 不支持 scope（如 "global"）
	if cc.Get("scope").Exists() {
		body, _ = sjson.DeleteBytes(body, basePath+".scope")
	}

	// ttl：仅 Claude 4.5+ 支持 "5m" 和 "1h"，其余情况移除
	ttl := cc.Get("ttl")
	if ttl.Exists() {
		shouldRemove := true
		if isClaude45 {
			v := ttl.String()
			if v == "5m" || v == "1h" {
				shouldRemove = false
			}
		}
		if shouldRemove {
			body, _ = sjson.DeleteBytes(body, basePath+".ttl")
		}
	}

	return body
}

// parseAnthropicBetaHeader 解析 anthropic-beta 头的逗号分隔字符串为 token 列表
func parseAnthropicBetaHeader(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	if strings.HasPrefix(header, "[") && strings.HasSuffix(header, "]") {
		var parsed []any
		if err := json.Unmarshal([]byte(header), &parsed); err == nil {
			tokens := make([]string, 0, len(parsed))
			for _, item := range parsed {
				token := strings.TrimSpace(fmt.Sprint(item))
				if token != "" {
					tokens = append(tokens, token)
				}
			}
			return tokens
		}
	}
	var tokens []string
	for _, part := range strings.Split(header, ",") {
		t := strings.TrimSpace(part)
		if t != "" {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// bedrockSupportedBetaTokens 是 Bedrock Invoke 支持的 beta 头白名单
// 参考: litellm/litellm/llms/bedrock/common_utils.py (anthropic_beta_headers_config.json)
// 更新策略: 当 AWS Bedrock 新增支持的 beta token 时需同步更新此白名单
var bedrockSupportedBetaTokens = map[string]bool{
	"computer-use-2025-01-24":         true,
	"computer-use-2025-11-24":         true,
	"context-1m-2025-08-07":           true,
	"context-management-2025-06-27":   true,
	"compact-2026-01-12":              true,
	"interleaved-thinking-2025-05-14": true,
	"tool-search-tool-2025-10-19":     true,
	"tool-examples-2025-10-29":        true,
}

// bedrockBetaTokenTransforms 定义 Bedrock Invoke 特有的 beta 头转换规则
// Anthropic 直接 API 使用通用头，Bedrock Invoke 需要特定的替代头
var bedrockBetaTokenTransforms = map[string]string{
	"advanced-tool-use-2025-11-20": "tool-search-tool-2025-10-19",
}

// autoInjectBedrockBetaTokens 根据请求体内容自动补齐必要的 beta token
// 参考 litellm: AnthropicModelInfo.get_anthropic_beta_list() 和
// AmazonAnthropicClaudeMessagesConfig._get_tool_search_beta_header_for_bedrock()
//
// 客户端（特别是非 Claude Code 客户端）可能只在 body 中启用了功能而不在 header 中带对应 beta token，
// 这里通过检测请求体特征自动补齐，确保 Bedrock Invoke 不会因缺少必要 beta 头而 400。
func autoInjectBedrockBetaTokens(tokens []string, body []byte, modelID string) []string {
	seen := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		seen[t] = true
	}

	inject := func(token string) {
		if !seen[token] {
			tokens = append(tokens, token)
			seen[token] = true
		}
	}

	// 检测 thinking / interleaved thinking
	// 请求体中有 "thinking" 字段 → 需要 interleaved-thinking beta
	if gjson.GetBytes(body, "thinking").Exists() {
		inject("interleaved-thinking-2025-05-14")
	}

	// 检测 computer_use 工具
	// tools 中有 type="computer_20xxxxxx" 的工具 → 需要 computer-use beta
	tools := gjson.GetBytes(body, "tools")
	if tools.Exists() && tools.IsArray() {
		toolSearchUsed := false
		programmaticToolCallingUsed := false
		inputExamplesUsed := false
		for _, tool := range tools.Array() {
			toolType := tool.Get("type").String()
			if strings.HasPrefix(toolType, "computer_20") {
				inject("computer-use-2025-11-24")
			}
			if isBedrockToolSearchType(toolType) {
				toolSearchUsed = true
			}
			if hasCodeExecutionAllowedCallers(tool) {
				programmaticToolCallingUsed = true
			}
			if hasInputExamples(tool) {
				inputExamplesUsed = true
			}
		}
		if programmaticToolCallingUsed || inputExamplesUsed {
			// programmatic tool calling 和 input examples 需要 advanced-tool-use，
			// 后续 filterBedrockBetaTokens 会将其转换为 Bedrock 特定的 tool-search-tool
			inject("advanced-tool-use-2025-11-20")
		}
		if toolSearchUsed && bedrockModelSupportsToolSearch(modelID) {
			// 纯 tool search（无 programmatic/inputExamples）时直接注入 Bedrock 特定头，
			// 跳过 advanced-tool-use → tool-search-tool 的转换步骤（与 litellm 对齐）
			if !programmaticToolCallingUsed && !inputExamplesUsed {
				inject("tool-search-tool-2025-10-19")
			} else {
				inject("advanced-tool-use-2025-11-20")
			}
		}
	}

	return tokens
}

func isBedrockToolSearchType(toolType string) bool {
	return toolType == "tool_search_tool_regex_20251119" || toolType == "tool_search_tool_bm25_20251119"
}

func hasCodeExecutionAllowedCallers(tool gjson.Result) bool {
	allowedCallers := tool.Get("allowed_callers")
	if containsStringInJSONArray(allowedCallers, "code_execution_20250825") {
		return true
	}
	return containsStringInJSONArray(tool.Get("function.allowed_callers"), "code_execution_20250825")
}

func hasInputExamples(tool gjson.Result) bool {
	if arr := tool.Get("input_examples"); arr.Exists() && arr.IsArray() && len(arr.Array()) > 0 {
		return true
	}
	arr := tool.Get("function.input_examples")
	return arr.Exists() && arr.IsArray() && len(arr.Array()) > 0
}

func containsStringInJSONArray(result gjson.Result, target string) bool {
	if !result.Exists() || !result.IsArray() {
		return false
	}
	for _, item := range result.Array() {
		if item.String() == target {
			return true
		}
	}
	return false
}

// bedrockModelSupportsToolSearch 判断 Bedrock 模型是否支持 tool search
// 目前仅 Claude Opus/Sonnet 4.5+ 支持，Haiku 不支持
func bedrockModelSupportsToolSearch(modelID string) bool {
	lower := strings.ToLower(modelID)
	matches := claudeVersionRe.FindStringSubmatch(lower)
	if matches == nil {
		return false
	}
	// Haiku 不支持 tool search
	if strings.Contains(lower, "haiku") {
		return false
	}
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	return major > 4 || (major == 4 && minor >= 5)
}

// filterBedrockBetaTokens 过滤并转换 beta token 列表，仅保留 Bedrock Invoke 支持的 token
// 1. 应用转换规则（如 advanced-tool-use → tool-search-tool）
// 2. 过滤掉 Bedrock 不支持的 token（如 output-128k, files-api, structured-outputs 等）
// 3. 自动关联 tool-examples（当 tool-search-tool 存在时）
func filterBedrockBetaTokens(tokens []string) []string {
	seen := make(map[string]bool, len(tokens))
	var result []string

	for _, t := range tokens {
		// 应用转换规则
		if replacement, ok := bedrockBetaTokenTransforms[t]; ok {
			t = replacement
		}
		// 只保留白名单中的 token，且去重
		if bedrockSupportedBetaTokens[t] && !seen[t] {
			result = append(result, t)
			seen[t] = true
		}
	}

	// 自动关联: tool-search-tool 存在时，确保 tool-examples 也存在
	if seen["tool-search-tool-2025-10-19"] && !seen["tool-examples-2025-10-29"] {
		result = append(result, "tool-examples-2025-10-29")
	}

	return result
}
