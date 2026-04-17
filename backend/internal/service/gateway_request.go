package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unsafe"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var (
	// 这些字节模式用于 fast-path 判断，避免每次 []byte("...") 产生临时分配。
	patternTypeThinking         = []byte(`"type":"thinking"`)
	patternTypeThinkingSpaced   = []byte(`"type": "thinking"`)
	patternTypeRedactedThinking = []byte(`"type":"redacted_thinking"`)
	patternTypeRedactedSpaced   = []byte(`"type": "redacted_thinking"`)

	patternThinkingField       = []byte(`"thinking":`)
	patternThinkingFieldSpaced = []byte(`"thinking" :`)

	patternEmptyContent       = []byte(`"content":[]`)
	patternEmptyContentSpaced = []byte(`"content": []`)
	patternEmptyContentSp1    = []byte(`"content" : []`)
	patternEmptyContentSp2    = []byte(`"content" :[]`)

	// Fast-path patterns for empty text blocks: {"type":"text","text":""}
	patternEmptyText       = []byte(`"text":""`)
	patternEmptyTextSpaced = []byte(`"text": ""`)
	patternEmptyTextSp1    = []byte(`"text" : ""`)
	patternEmptyTextSp2    = []byte(`"text" :""`)

	sessionUserAgentProductPattern = regexp.MustCompile(`([A-Za-z0-9._-]+)/[A-Za-z0-9._-]+`)
	sessionUserAgentVersionPattern = regexp.MustCompile(`\bv?\d+(?:\.\d+){1,3}\b`)
)

// SessionContext 粘性会话上下文，用于区分不同来源的请求。
// 仅在 GenerateSessionHash 第 3 级 fallback（消息内容 hash）时混入，
// 避免不同用户发送相同消息产生相同 hash 导致账号集中。
type SessionContext struct {
	ClientIP  string
	UserAgent string
	APIKeyID  int64
}

// ParsedRequest 保存网关请求的预解析结果
//
// 性能优化说明：
// 原实现在多个位置重复解析请求体（Handler、Service 各解析一次）：
// 1. gateway_handler.go 解析获取 model 和 stream
// 2. gateway_service.go 再次解析获取 system、messages、metadata
// 3. GenerateSessionHash 又一次解析获取会话哈希所需字段
//
// 新实现一次解析，多处复用：
// 1. 在 Handler 层统一调用 ParseGatewayRequest 一次性解析
// 2. 将解析结果 ParsedRequest 传递给 Service 层
// 3. 避免重复 json.Unmarshal，减少 CPU 和内存开销
type ParsedRequest struct {
	Body            []byte          // 原始请求体（保留用于转发）
	Model           string          // 请求的模型名称
	Stream          bool            // 是否为流式请求
	MetadataUserID  string          // metadata.user_id（用于会话亲和）
	System          any             // system 字段内容
	Messages        []any           // messages 数组
	HasSystem       bool            // 是否包含 system 字段（包含 null 也视为显式传入）
	ThinkingEnabled bool            // 是否开启 thinking（部分平台会影响最终模型名）
	OutputEffort    string          // output_config.effort（Claude API 的推理强度控制）
	MaxTokens       int             // max_tokens 值（用于探测请求拦截）
	SessionContext  *SessionContext // 可选：请求上下文区分因子（nil 时行为不变）

	// GroupID 请求所属分组 ID（来自 API Key）
	GroupID *int64

	// OnUpstreamAccepted 上游接受请求后立即调用（用于提前释放串行锁）
	// 流式请求在收到 2xx 响应头后调用，避免持锁等流完成
	OnUpstreamAccepted func()
}

// NormalizeSessionUserAgent reduces UA noise for sticky-session and digest hashing.
// It preserves the set of product names from Product/Version tokens while
// discarding version-only changes and incidental comments.
func NormalizeSessionUserAgent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	matches := sessionUserAgentProductPattern.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return normalizeSessionUserAgentFallback(raw)
	}

	products := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		product := strings.ToLower(strings.TrimSpace(match[1]))
		if product == "" {
			continue
		}
		if _, exists := seen[product]; exists {
			continue
		}
		seen[product] = struct{}{}
		products = append(products, product)
	}
	if len(products) == 0 {
		return normalizeSessionUserAgentFallback(raw)
	}
	sort.Strings(products)
	return strings.Join(products, "+")
}

func normalizeSessionUserAgentFallback(raw string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(raw), " "))
	normalized = sessionUserAgentVersionPattern.ReplaceAllString(normalized, "")
	return strings.Join(strings.Fields(normalized), " ")
}

// ParseGatewayRequest 解析网关请求体并返回结构化结果。
// protocol 指定请求协议格式（domain.PlatformAnthropic / domain.PlatformGemini），
// 不同协议使用不同的 system/messages 字段名。
func ParseGatewayRequest(body []byte, protocol string) (*ParsedRequest, error) {
	// 保持与旧实现一致：请求体必须是合法 JSON。
	// 注意：gjson.GetBytes 对非法 JSON 不会报错，因此需要显式校验。
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid json")
	}

	// 性能：
	// - gjson.GetBytes 会把匹配的 Raw/Str 安全复制成 string（对于巨大 messages 会产生额外拷贝）。
	// - 这里将 body 通过 unsafe 零拷贝视为 string，仅在本函数内使用，且 body 不会被修改。
	jsonStr := *(*string)(unsafe.Pointer(&body))

	parsed := &ParsedRequest{
		Body: body,
	}

	// --- gjson 提取简单字段（避免完整 Unmarshal） ---

	// model: 需要严格类型校验，非 string 返回错误
	modelResult := gjson.Get(jsonStr, "model")
	if modelResult.Exists() {
		if modelResult.Type != gjson.String {
			return nil, fmt.Errorf("invalid model field type")
		}
		parsed.Model = modelResult.String()
	}

	// stream: 需要严格类型校验，非 bool 返回错误
	streamResult := gjson.Get(jsonStr, "stream")
	if streamResult.Exists() {
		if streamResult.Type != gjson.True && streamResult.Type != gjson.False {
			return nil, fmt.Errorf("invalid stream field type")
		}
		parsed.Stream = streamResult.Bool()
	}

	// metadata.user_id: 直接路径提取，不需要严格类型校验
	parsed.MetadataUserID = gjson.Get(jsonStr, "metadata.user_id").String()

	// thinking.type: enabled/adaptive 都视为开启
	thinkingType := gjson.Get(jsonStr, "thinking.type").String()
	if thinkingType == "enabled" || thinkingType == "adaptive" {
		parsed.ThinkingEnabled = true
	}

	// output_config.effort: Claude API 的推理强度控制参数
	parsed.OutputEffort = strings.TrimSpace(gjson.Get(jsonStr, "output_config.effort").String())

	// max_tokens: 仅接受整数值
	maxTokensResult := gjson.Get(jsonStr, "max_tokens")
	if maxTokensResult.Exists() && maxTokensResult.Type == gjson.Number {
		f := maxTokensResult.Float()
		if !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f) &&
			f <= float64(math.MaxInt) && f >= float64(math.MinInt) {
			parsed.MaxTokens = int(f)
		}
	}

	// --- system/messages 提取 ---
	// 避免把整个 body Unmarshal 到 map（会产生大量 map/接口分配）。
	// 使用 gjson 抽取目标字段的 Raw，再对该子树进行 Unmarshal。

	switch protocol {
	case domain.PlatformGemini:
		// Gemini 原生格式: systemInstruction.parts / contents
		if sysParts := gjson.Get(jsonStr, "systemInstruction.parts"); sysParts.Exists() && sysParts.IsArray() {
			var parts []any
			if err := json.Unmarshal(sliceRawFromBody(body, sysParts), &parts); err != nil {
				return nil, err
			}
			parsed.System = parts
		}

		if contents := gjson.Get(jsonStr, "contents"); contents.Exists() && contents.IsArray() {
			var msgs []any
			if err := json.Unmarshal(sliceRawFromBody(body, contents), &msgs); err != nil {
				return nil, err
			}
			parsed.Messages = msgs
		}
	default:
		// Anthropic / OpenAI 格式: system / messages
		// system 字段只要存在就视为显式提供（即使为 null），
		// 以避免客户端传 null 时被默认 system 误注入。
		if sys := gjson.Get(jsonStr, "system"); sys.Exists() {
			parsed.HasSystem = true
			switch sys.Type {
			case gjson.Null:
				parsed.System = nil
			case gjson.String:
				// 与 encoding/json 的 Unmarshal 行为一致：返回解码后的字符串。
				parsed.System = sys.String()
			default:
				var system any
				if err := json.Unmarshal(sliceRawFromBody(body, sys), &system); err != nil {
					return nil, err
				}
				parsed.System = system
			}
		}

		if msgs := gjson.Get(jsonStr, "messages"); msgs.Exists() && msgs.IsArray() {
			var messages []any
			if err := json.Unmarshal(sliceRawFromBody(body, msgs), &messages); err != nil {
				return nil, err
			}
			parsed.Messages = messages
		}
	}

	return parsed, nil
}

// sliceRawFromBody 返回 Result.Raw 对应的原始字节切片。
// 优先使用 Result.Index 直接从 body 切片，避免对大字段（如 messages）产生额外拷贝。
// 当 Index 不可用时，退化为复制（理论上极少发生）。
func sliceRawFromBody(body []byte, r gjson.Result) []byte {
	if r.Index > 0 {
		end := r.Index + len(r.Raw)
		if end <= len(body) {
			return body[r.Index:end]
		}
	}
	// fallback: 不影响正确性，但会产生一次拷贝
	return []byte(r.Raw)
}

// stripEmptyTextBlocksFromSlice removes empty text blocks from a content slice (including nested tool_result content).
// Returns (cleaned slice, true) if any blocks were removed, or (original, false) if unchanged.
func stripEmptyTextBlocksFromSlice(blocks []any) ([]any, bool) {
	var result []any
	changed := false
	for i, block := range blocks {
		blockMap, ok := block.(map[string]any)
		if !ok {
			if result != nil {
				result = append(result, block)
			}
			continue
		}
		blockType, _ := blockMap["type"].(string)

		// Strip empty text blocks
		if blockType == "text" {
			if txt, _ := blockMap["text"].(string); txt == "" {
				if result == nil {
					result = make([]any, 0, len(blocks))
					result = append(result, blocks[:i]...)
				}
				changed = true
				continue
			}
		}

		// Recurse into tool_result nested content
		if blockType == "tool_result" {
			if nestedContent, ok := blockMap["content"].([]any); ok {
				if cleaned, nestedChanged := stripEmptyTextBlocksFromSlice(nestedContent); nestedChanged {
					if result == nil {
						result = make([]any, 0, len(blocks))
						result = append(result, blocks[:i]...)
					}
					changed = true
					blockCopy := make(map[string]any, len(blockMap))
					for k, v := range blockMap {
						blockCopy[k] = v
					}
					blockCopy["content"] = cleaned
					result = append(result, blockCopy)
					continue
				}
			}
		}

		if result != nil {
			result = append(result, block)
		}
	}
	if !changed {
		return blocks, false
	}
	return result, true
}

// StripEmptyTextBlocks removes empty text blocks from the request body (including nested tool_result content).
// This is a lightweight pre-filter for the initial request path to prevent upstream 400 errors.
// Returns the original body unchanged if no empty text blocks are found.
func StripEmptyTextBlocks(body []byte) []byte {
	// Fast path: check if body contains empty text patterns
	hasEmptyTextBlock := bytes.Contains(body, patternEmptyText) ||
		bytes.Contains(body, patternEmptyTextSpaced) ||
		bytes.Contains(body, patternEmptyTextSp1) ||
		bytes.Contains(body, patternEmptyTextSp2)
	if !hasEmptyTextBlock {
		return body
	}

	jsonStr := *(*string)(unsafe.Pointer(&body))
	msgsRes := gjson.Get(jsonStr, "messages")
	if !msgsRes.Exists() || !msgsRes.IsArray() {
		return body
	}

	var messages []any
	if err := json.Unmarshal(sliceRawFromBody(body, msgsRes), &messages); err != nil {
		return body
	}

	modified := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		content, ok := msgMap["content"].([]any)
		if !ok {
			continue
		}
		if cleaned, changed := stripEmptyTextBlocksFromSlice(content); changed {
			modified = true
			msgMap["content"] = cleaned
		}
	}

	if !modified {
		return body
	}

	msgsBytes, err := json.Marshal(messages)
	if err != nil {
		return body
	}
	out, err := sjson.SetRawBytes(body, "messages", msgsBytes)
	if err != nil {
		return body
	}
	return out
}

// FilterThinkingBlocks removes thinking blocks from request body
// Returns filtered body or original body if filtering fails (fail-safe)
// This prevents 400 errors from invalid thinking block signatures
//
// 策略：
//   - 当 thinking.type 不是 "enabled"/"adaptive"：移除所有 thinking 相关块
//   - 当 thinking.type 是 "enabled"/"adaptive"：仅移除缺失/无效 signature 的 thinking 块（避免 400）
//     (blocks with missing/empty/dummy signatures that would cause 400 errors)
func FilterThinkingBlocks(body []byte) []byte {
	return filterThinkingBlocksInternal(body, false)
}

// FilterThinkingBlocksForRetry strips thinking-related constructs for retry scenarios.
//
// Why:
//   - Upstreams may reject historical `thinking`/`redacted_thinking` blocks due to invalid/missing signatures.
//   - Anthropic extended thinking has a structural constraint: when top-level `thinking` is enabled and the
//     final message is an assistant prefill, the assistant content must start with a thinking block.
//   - If we remove thinking blocks but keep top-level `thinking` enabled, we can trigger:
//     "Expected `thinking` or `redacted_thinking`, but found `text`"
//
// Strategy (B: preserve content as text):
//   - Disable top-level `thinking` (remove `thinking` field).
//   - Convert `thinking` blocks to `text` blocks (preserve the thinking content).
//   - Remove `redacted_thinking` blocks (cannot be converted to text).
//   - Ensure no message ends up with empty content.
func FilterThinkingBlocksForRetry(body []byte) []byte {
	hasThinkingContent := bytes.Contains(body, patternTypeThinking) ||
		bytes.Contains(body, patternTypeThinkingSpaced) ||
		bytes.Contains(body, patternTypeRedactedThinking) ||
		bytes.Contains(body, patternTypeRedactedSpaced) ||
		bytes.Contains(body, patternThinkingField) ||
		bytes.Contains(body, patternThinkingFieldSpaced)

	// Also check for empty content arrays and empty text blocks that need fixing.
	// Note: This is a heuristic check; the actual empty content handling is done below.
	hasEmptyContent := bytes.Contains(body, patternEmptyContent) ||
		bytes.Contains(body, patternEmptyContentSpaced) ||
		bytes.Contains(body, patternEmptyContentSp1) ||
		bytes.Contains(body, patternEmptyContentSp2)

	// Check for empty text blocks: {"type":"text","text":""}
	// These cause upstream 400: "text content blocks must be non-empty"
	hasEmptyTextBlock := bytes.Contains(body, patternEmptyText) ||
		bytes.Contains(body, patternEmptyTextSpaced) ||
		bytes.Contains(body, patternEmptyTextSp1) ||
		bytes.Contains(body, patternEmptyTextSp2)

	// Fast path: nothing to process
	if !hasThinkingContent && !hasEmptyContent && !hasEmptyTextBlock {
		return body
	}

	// 尽量避免把整个 body Unmarshal 成 map（会产生大量 map/接口分配）。
	// 这里先用 gjson 把 messages 子树摘出来，后续只对 messages 做 Unmarshal/Marshal。
	jsonStr := *(*string)(unsafe.Pointer(&body))
	msgsRes := gjson.Get(jsonStr, "messages")
	if !msgsRes.Exists() || !msgsRes.IsArray() {
		return body
	}

	// Fast path：只需要删除顶层 thinking，不需要改 messages。
	// 注意：patternThinkingField 可能来自嵌套字段（如 tool_use.input.thinking），因此必须用 gjson 判断顶层字段是否存在。
	containsThinkingBlocks := bytes.Contains(body, patternTypeThinking) ||
		bytes.Contains(body, patternTypeThinkingSpaced) ||
		bytes.Contains(body, patternTypeRedactedThinking) ||
		bytes.Contains(body, patternTypeRedactedSpaced) ||
		bytes.Contains(body, patternThinkingFieldSpaced)
	if !hasEmptyContent && !hasEmptyTextBlock && !containsThinkingBlocks {
		if topThinking := gjson.Get(jsonStr, "thinking"); topThinking.Exists() {
			if out, err := sjson.DeleteBytes(body, "thinking"); err == nil {
				out = removeThinkingDependentContextStrategies(out)
				return out
			}
			return body
		}
		return body
	}

	var messages []any
	if err := json.Unmarshal(sliceRawFromBody(body, msgsRes), &messages); err != nil {
		return body
	}

	modified := false

	// Disable top-level thinking mode for retry to avoid structural/signature constraints upstream.
	deleteTopLevelThinking := gjson.Get(jsonStr, "thinking").Exists()

	for i := 0; i < len(messages); i++ {
		msgMap, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}

		role, _ := msgMap["role"].(string)
		content, ok := msgMap["content"].([]any)
		if !ok {
			// String content or other format - keep as is
			continue
		}

		// 延迟分配：只有检测到需要修改的块，才构建新 slice。
		var newContent []any
		modifiedThisMsg := false

		ensureNewContent := func(prefixLen int) {
			if newContent != nil {
				return
			}
			newContent = make([]any, 0, len(content))
			if prefixLen > 0 {
				newContent = append(newContent, content[:prefixLen]...)
			}
		}

		for bi := 0; bi < len(content); bi++ {
			block := content[bi]
			blockMap, ok := block.(map[string]any)
			if !ok {
				if newContent != nil {
					newContent = append(newContent, block)
				}
				continue
			}

			blockType, _ := blockMap["type"].(string)

			// Strip empty text blocks: {"type":"text","text":""}
			// Upstream rejects these with 400: "text content blocks must be non-empty"
			if blockType == "text" {
				if txt, _ := blockMap["text"].(string); txt == "" {
					modifiedThisMsg = true
					ensureNewContent(bi)
					continue
				}
			}

			// Convert thinking blocks to text (preserve content) and drop redacted_thinking.
			switch blockType {
			case "thinking":
				modifiedThisMsg = true
				ensureNewContent(bi)
				thinkingText, _ := blockMap["thinking"].(string)
				if thinkingText != "" {
					newContent = append(newContent, map[string]any{"type": "text", "text": thinkingText})
				}
				continue
			case "redacted_thinking":
				modifiedThisMsg = true
				ensureNewContent(bi)
				continue
			}

			// Handle blocks without type discriminator but with a "thinking" field.
			if blockType == "" {
				if rawThinking, hasThinking := blockMap["thinking"]; hasThinking {
					modifiedThisMsg = true
					ensureNewContent(bi)
					switch v := rawThinking.(type) {
					case string:
						if v != "" {
							newContent = append(newContent, map[string]any{"type": "text", "text": v})
						}
					default:
						if b, err := json.Marshal(v); err == nil && len(b) > 0 {
							newContent = append(newContent, map[string]any{"type": "text", "text": string(b)})
						}
					}
					continue
				}
			}

			// Recursively strip empty text blocks from tool_result nested content.
			if blockType == "tool_result" {
				if nestedContent, ok := blockMap["content"].([]any); ok {
					if cleaned, changed := stripEmptyTextBlocksFromSlice(nestedContent); changed {
						modifiedThisMsg = true
						ensureNewContent(bi)
						blockCopy := make(map[string]any, len(blockMap))
						for k, v := range blockMap {
							blockCopy[k] = v
						}
						blockCopy["content"] = cleaned
						newContent = append(newContent, blockCopy)
						continue
					}
				}
			}

			if newContent != nil {
				newContent = append(newContent, block)
			}
		}

		// Handle empty content: either from filtering or originally empty
		if newContent == nil {
			if len(content) == 0 {
				modified = true
				placeholder := "(content removed)"
				if role == "assistant" {
					placeholder = "(assistant content removed)"
				}
				msgMap["content"] = []any{map[string]any{"type": "text", "text": placeholder}}
			}
			continue
		}

		if len(newContent) == 0 {
			modified = true
			placeholder := "(content removed)"
			if role == "assistant" {
				placeholder = "(assistant content removed)"
			}
			msgMap["content"] = []any{map[string]any{"type": "text", "text": placeholder}}
			continue
		}

		if modifiedThisMsg {
			modified = true
			msgMap["content"] = newContent
		}
	}

	if !modified && !deleteTopLevelThinking {
		// Avoid rewriting JSON when no changes are needed.
		return body
	}

	out := body
	if deleteTopLevelThinking {
		if b, err := sjson.DeleteBytes(out, "thinking"); err == nil {
			out = b
		} else {
			return body
		}
		// Removing "thinking" makes any context_management strategy that requires it invalid
		// (e.g. clear_thinking_20251015).  Strip those entries so the retry request does not
		// receive a 400 "strategy requires thinking to be enabled or adaptive".
		out = removeThinkingDependentContextStrategies(out)
	}
	if modified {
		msgsBytes, err := json.Marshal(messages)
		if err != nil {
			return body
		}
		out, err = sjson.SetRawBytes(out, "messages", msgsBytes)
		if err != nil {
			return body
		}
	}
	return out
}

// removeThinkingDependentContextStrategies 从 context_management.edits 中移除
// 需要 thinking 启用的策略（如 clear_thinking_20251015）。
// 当顶层 "thinking" 字段被禁用时必须调用，否则上游会返回
// "strategy requires thinking to be enabled or adaptive"。
func removeThinkingDependentContextStrategies(body []byte) []byte {
	jsonStr := *(*string)(unsafe.Pointer(&body))
	editsRes := gjson.Get(jsonStr, "context_management.edits")
	if !editsRes.Exists() || !editsRes.IsArray() {
		return body
	}

	var filtered []json.RawMessage
	hasRemoved := false
	editsRes.ForEach(func(_, v gjson.Result) bool {
		if v.Get("type").String() == "clear_thinking_20251015" {
			hasRemoved = true
			return true
		}
		filtered = append(filtered, json.RawMessage(v.Raw))
		return true
	})

	if !hasRemoved {
		return body
	}

	if len(filtered) == 0 {
		if b, err := sjson.DeleteBytes(body, "context_management.edits"); err == nil {
			return b
		}
		return body
	}

	filteredBytes, err := json.Marshal(filtered)
	if err != nil {
		return body
	}
	if b, err := sjson.SetRawBytes(body, "context_management.edits", filteredBytes); err == nil {
		return b
	}
	return body
}

// FilterSignatureSensitiveBlocksForRetry is a stronger retry filter for cases where upstream errors indicate
// signature/thought_signature validation issues involving tool blocks.
//
// This performs everything in FilterThinkingBlocksForRetry, plus:
//   - Convert `tool_use` blocks to text (name/id/input) so we stop sending structured tool calls.
//   - Convert `tool_result` blocks to text so we keep tool results visible without tool semantics.
//
// Use this only when needed: converting tool blocks to text changes model behaviour and can increase the
// risk of prompt injection (tool output becomes plain conversation text).
func FilterSignatureSensitiveBlocksForRetry(body []byte) []byte {
	// Fast path: only run when we see likely relevant constructs.
	if !bytes.Contains(body, []byte(`"type":"thinking"`)) &&
		!bytes.Contains(body, []byte(`"type": "thinking"`)) &&
		!bytes.Contains(body, []byte(`"type":"redacted_thinking"`)) &&
		!bytes.Contains(body, []byte(`"type": "redacted_thinking"`)) &&
		!bytes.Contains(body, []byte(`"type":"tool_use"`)) &&
		!bytes.Contains(body, []byte(`"type": "tool_use"`)) &&
		!bytes.Contains(body, []byte(`"type":"tool_result"`)) &&
		!bytes.Contains(body, []byte(`"type": "tool_result"`)) &&
		!bytes.Contains(body, []byte(`"thinking":`)) &&
		!bytes.Contains(body, []byte(`"thinking" :`)) {
		return body
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}

	modified := false

	// Disable top-level thinking for retry to avoid structural/signature constraints upstream.
	if _, exists := req["thinking"]; exists {
		delete(req, "thinking")
		modified = true
		// Remove context_management strategies that require thinking to be enabled
		// (e.g. clear_thinking_20251015), otherwise upstream returns 400.
		if cm, ok := req["context_management"].(map[string]any); ok {
			if edits, ok := cm["edits"].([]any); ok {
				filtered := make([]any, 0, len(edits))
				for _, edit := range edits {
					if editMap, ok := edit.(map[string]any); ok {
						if editMap["type"] == "clear_thinking_20251015" {
							continue
						}
					}
					filtered = append(filtered, edit)
				}
				if len(filtered) != len(edits) {
					if len(filtered) == 0 {
						delete(cm, "edits")
					} else {
						cm["edits"] = filtered
					}
				}
			}
		}
	}

	messages, ok := req["messages"].([]any)
	if !ok {
		return body
	}

	newMessages := make([]any, 0, len(messages))

	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			newMessages = append(newMessages, msg)
			continue
		}

		role, _ := msgMap["role"].(string)
		content, ok := msgMap["content"].([]any)
		if !ok {
			newMessages = append(newMessages, msg)
			continue
		}

		newContent := make([]any, 0, len(content))
		modifiedThisMsg := false

		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok {
				newContent = append(newContent, block)
				continue
			}

			blockType, _ := blockMap["type"].(string)
			switch blockType {
			case "thinking":
				modifiedThisMsg = true
				thinkingText, _ := blockMap["thinking"].(string)
				if thinkingText == "" {
					continue
				}
				newContent = append(newContent, map[string]any{"type": "text", "text": thinkingText})
				continue
			case "redacted_thinking":
				modifiedThisMsg = true
				continue
			case "tool_use":
				modifiedThisMsg = true
				name, _ := blockMap["name"].(string)
				id, _ := blockMap["id"].(string)
				input := blockMap["input"]
				inputJSON, _ := json.Marshal(input)
				text := "(tool_use)"
				if name != "" {
					text += " name=" + name
				}
				if id != "" {
					text += " id=" + id
				}
				if len(inputJSON) > 0 && string(inputJSON) != "null" {
					text += " input=" + string(inputJSON)
				}
				newContent = append(newContent, map[string]any{"type": "text", "text": text})
				continue
			case "tool_result":
				modifiedThisMsg = true
				toolUseID, _ := blockMap["tool_use_id"].(string)
				isError, _ := blockMap["is_error"].(bool)
				content := blockMap["content"]
				contentJSON, _ := json.Marshal(content)
				text := "(tool_result)"
				if toolUseID != "" {
					text += " tool_use_id=" + toolUseID
				}
				if isError {
					text += " is_error=true"
				}
				if len(contentJSON) > 0 && string(contentJSON) != "null" {
					text += "\n" + string(contentJSON)
				}
				newContent = append(newContent, map[string]any{"type": "text", "text": text})
				continue
			}

			if blockType == "" {
				if rawThinking, hasThinking := blockMap["thinking"]; hasThinking {
					modifiedThisMsg = true
					switch v := rawThinking.(type) {
					case string:
						if v != "" {
							newContent = append(newContent, map[string]any{"type": "text", "text": v})
						}
					default:
						if b, err := json.Marshal(v); err == nil && len(b) > 0 {
							newContent = append(newContent, map[string]any{"type": "text", "text": string(b)})
						}
					}
					continue
				}
			}

			newContent = append(newContent, block)
		}

		if modifiedThisMsg {
			modified = true
			if len(newContent) == 0 {
				placeholder := "(content removed)"
				if role == "assistant" {
					placeholder = "(assistant content removed)"
				}
				newContent = append(newContent, map[string]any{"type": "text", "text": placeholder})
			}
			msgMap["content"] = newContent
		}

		newMessages = append(newMessages, msgMap)
	}

	if !modified {
		return body
	}

	req["messages"] = newMessages
	newBody, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return newBody
}

// filterThinkingBlocksInternal removes invalid thinking blocks from request
// 策略：
//   - 当 thinking.type 不是 "enabled"/"adaptive"：移除所有 thinking 相关块
//   - 当 thinking.type 是 "enabled"/"adaptive"：仅移除缺失/无效 signature 的 thinking 块
func filterThinkingBlocksInternal(body []byte, _ bool) []byte {
	// Fast path: if body doesn't contain "thinking", skip parsing
	if !bytes.Contains(body, []byte(`"type":"thinking"`)) &&
		!bytes.Contains(body, []byte(`"type": "thinking"`)) &&
		!bytes.Contains(body, []byte(`"type":"redacted_thinking"`)) &&
		!bytes.Contains(body, []byte(`"type": "redacted_thinking"`)) &&
		!bytes.Contains(body, []byte(`"thinking":`)) &&
		!bytes.Contains(body, []byte(`"thinking" :`)) {
		return body
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}

	// Check if thinking is enabled
	thinkingEnabled := false
	if thinking, ok := req["thinking"].(map[string]any); ok {
		if thinkType, ok := thinking["type"].(string); ok && (thinkType == "enabled" || thinkType == "adaptive") {
			thinkingEnabled = true
		}
	}

	messages, ok := req["messages"].([]any)
	if !ok {
		return body
	}

	filtered := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}

		role, _ := msgMap["role"].(string)
		content, ok := msgMap["content"].([]any)
		if !ok {
			continue
		}

		newContent := make([]any, 0, len(content))
		filteredThisMessage := false

		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok {
				newContent = append(newContent, block)
				continue
			}

			blockType, _ := blockMap["type"].(string)

			if blockType == "thinking" || blockType == "redacted_thinking" {
				// When thinking is enabled and this is an assistant message,
				// only keep thinking blocks with valid signatures
				if thinkingEnabled && role == "assistant" {
					signature, _ := blockMap["signature"].(string)
					if signature != "" && signature != antigravity.DummyThoughtSignature {
						newContent = append(newContent, block)
						continue
					}
				}
				filtered = true
				filteredThisMessage = true
				continue
			}

			// Handle blocks without type discriminator but with "thinking" key
			if blockType == "" {
				if _, hasThinking := blockMap["thinking"]; hasThinking {
					filtered = true
					filteredThisMessage = true
					continue
				}
			}

			newContent = append(newContent, block)
		}

		if filteredThisMessage {
			msgMap["content"] = newContent
		}
	}

	if !filtered {
		return body
	}

	newBody, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return newBody
}

// NormalizeClaudeOutputEffort normalizes Claude's output_config.effort value.
// Returns nil for empty or unrecognized values.
func NormalizeClaudeOutputEffort(raw string) *string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return nil
	}
	switch value {
	case "low", "medium", "high", "max":
		return &value
	default:
		return nil
	}
}

// =========================
// Thinking Budget Rectifier
// =========================

const (
	// BudgetRectifyBudgetTokens is the budget_tokens value to set when rectifying.
	BudgetRectifyBudgetTokens = 32000
	// BudgetRectifyMaxTokens is the max_tokens value to set when rectifying.
	BudgetRectifyMaxTokens = 64000
	// BudgetRectifyMinMaxTokens is the minimum max_tokens that must exceed budget_tokens.
	BudgetRectifyMinMaxTokens = 32001
)

// isThinkingBudgetConstraintError detects whether an upstream error message indicates
// a budget_tokens constraint violation (e.g. "budget_tokens >= 1024").
// Matches three conditions (all must be true):
//  1. Contains "budget_tokens" or "budget tokens"
//  2. Contains "thinking"
//  3. Contains ">= 1024" or "greater than or equal to 1024" or ("1024" + "input should be")
func isThinkingBudgetConstraintError(errMsg string) bool {
	m := strings.ToLower(errMsg)

	// Condition 1: budget_tokens or budget tokens
	hasBudget := strings.Contains(m, "budget_tokens") || strings.Contains(m, "budget tokens")
	if !hasBudget {
		return false
	}

	// Condition 2: thinking
	if !strings.Contains(m, "thinking") {
		return false
	}

	// Condition 3: constraint indicator
	if strings.Contains(m, ">= 1024") || strings.Contains(m, "greater than or equal to 1024") {
		return true
	}
	if strings.Contains(m, "1024") && strings.Contains(m, "input should be") {
		return true
	}

	return false
}

// RectifyThinkingBudget modifies the request body to fix budget_tokens constraint errors.
// It sets thinking.budget_tokens = 32000, thinking.type = "enabled" (unless adaptive),
// and ensures max_tokens >= 32001.
// Returns (modified body, true) if changes were applied, or (original body, false) if not.
func RectifyThinkingBudget(body []byte) ([]byte, bool) {
	// If thinking type is "adaptive", skip rectification entirely
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if thinkingType == "adaptive" {
		return body, false
	}

	modified := body
	changed := false

	// Set thinking.type = "enabled"
	if thinkingType != "enabled" {
		if result, err := sjson.SetBytes(modified, "thinking.type", "enabled"); err == nil {
			modified = result
			changed = true
		}
	}

	// Set thinking.budget_tokens = 32000
	currentBudget := gjson.GetBytes(modified, "thinking.budget_tokens").Int()
	if currentBudget != BudgetRectifyBudgetTokens {
		if result, err := sjson.SetBytes(modified, "thinking.budget_tokens", BudgetRectifyBudgetTokens); err == nil {
			modified = result
			changed = true
		}
	}

	// Ensure max_tokens >= BudgetRectifyMinMaxTokens
	maxTokens := gjson.GetBytes(modified, "max_tokens").Int()
	if maxTokens < int64(BudgetRectifyMinMaxTokens) {
		if result, err := sjson.SetBytes(modified, "max_tokens", BudgetRectifyMaxTokens); err == nil {
			modified = result
			changed = true
		}
	}

	return modified, changed
}
