package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unsafe"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
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

type jsonRange struct {
	start int        // 原始请求体中的起始偏移（闭区间）
	end   int        // 原始请求体中的结束偏移（开区间）
	kind  gjson.Type // JSON 值类型，用于调用方做轻量分支
}

type RequestBodyRef struct {
	data []byte
}

func NewRequestBodyRef(data []byte) *RequestBodyRef {
	return &RequestBodyRef{data: data}
}

func (b *RequestBodyRef) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.data
}

func (b *RequestBodyRef) Len() int {
	if b == nil {
		return 0
	}
	return len(b.data)
}

func (b *RequestBodyRef) Replace(data []byte) {
	if b == nil {
		return
	}
	b.data = data
}

func missingJSONRange() jsonRange {
	return jsonRange{start: -1, end: -1}
}

func rangeFromResult(r gjson.Result) jsonRange {
	if r.Raw == "" || r.Index <= 0 {
		return missingJSONRange()
	}
	end := r.Index + len(r.Raw)
	if end < r.Index {
		return missingJSONRange()
	}
	return jsonRange{start: r.Index, end: end, kind: r.Type}
}

func (r jsonRange) exists() bool {
	return r.start >= 0 && r.end >= r.start
}

// clearGatewayRequestDerivedState 清空绑定当前 body 的轻量派生字段，防止 ReplaceBody 后读到旧值。
func clearGatewayRequestDerivedState(parsed *ParsedRequest) {
	if parsed == nil {
		return
	}
	parsed.Model = ""
	parsed.Stream = false
	parsed.MetadataUserID = ""
	parsed.HasSystem = false
	parsed.ThinkingEnabled = false
	parsed.OutputEffort = ""
	parsed.MaxTokens = 0
	parsed.systemRange = missingJSONRange()
	parsed.messagesRange = missingJSONRange()
	parsed.inputRange = missingJSONRange()
}

func clearGatewayRequestRanges(parsed *ParsedRequest) {
	if parsed == nil {
		return
	}
	parsed.HasSystem = false
	parsed.systemRange = missingJSONRange()
	parsed.messagesRange = missingJSONRange()
	parsed.inputRange = missingJSONRange()
}

func setGatewayRequestRanges(parsed *ParsedRequest, protocol string, jsonStr string) {
	if parsed == nil {
		return
	}
	switch protocol {
	case domain.PlatformGemini:
		if sysParts := gjson.Get(jsonStr, "systemInstruction.parts"); sysParts.Exists() && sysParts.IsArray() {
			parsed.systemRange = rangeFromResult(sysParts)
		}
		if contents := gjson.Get(jsonStr, "contents"); contents.Exists() && contents.IsArray() {
			parsed.messagesRange = rangeFromResult(contents)
		}
	default:
		if sys := gjson.Get(jsonStr, "system"); sys.Exists() {
			parsed.HasSystem = true
			parsed.systemRange = rangeFromResult(sys)
		}
		if msgs := gjson.Get(jsonStr, "messages"); msgs.Exists() && msgs.IsArray() {
			parsed.messagesRange = rangeFromResult(msgs)
		}
		if protocol == "responses" {
			if input := gjson.Get(jsonStr, "input"); input.Exists() {
				parsed.inputRange = rangeFromResult(input)
			}
		}
	}
}

// parseGatewayRequestCurrentBody 只做标量和 raw range 轻量解析，不恢复 system/messages 对象图。
func parseGatewayRequestCurrentBody(parsed *ParsedRequest, protocol string) error {
	if parsed == nil || parsed.Body == nil {
		return fmt.Errorf("empty request body")
	}

	bodyBytes := parsed.Body.Bytes()
	if !gjson.ValidBytes(bodyBytes) {
		return DescribeInvalidJSON(bodyBytes)
	}

	// 只在当前函数内零拷贝读取 JSON 字段；ReplaceBody 后必须重新进入本函数刷新派生状态。
	jsonStr := *(*string)(unsafe.Pointer(&bodyBytes))
	clearGatewayRequestDerivedState(parsed)
	parsed.protocol = protocol

	modelResult := gjson.Get(jsonStr, "model")
	if modelResult.Exists() {
		if modelResult.Type != gjson.String {
			return fmt.Errorf("invalid model field type")
		}
		parsed.Model = modelResult.String()
	}

	streamResult := gjson.Get(jsonStr, "stream")
	if streamResult.Exists() {
		if streamResult.Type != gjson.True && streamResult.Type != gjson.False {
			return fmt.Errorf("invalid stream field type")
		}
		parsed.Stream = streamResult.Bool()
	}

	parsed.MetadataUserID = gjson.Get(jsonStr, "metadata.user_id").String()

	thinkingType := gjson.Get(jsonStr, "thinking.type").String()
	parsed.ThinkingEnabled = thinkingType == "enabled" || thinkingType == "adaptive"

	parsed.OutputEffort = strings.TrimSpace(gjson.Get(jsonStr, "output_config.effort").String())

	maxTokensResult := gjson.Get(jsonStr, "max_tokens")
	if maxTokensResult.Exists() && maxTokensResult.Type == gjson.Number {
		f := maxTokensResult.Float()
		if !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f) &&
			f <= float64(math.MaxInt) && f >= float64(math.MinInt) {
			parsed.MaxTokens = int(f)
		}
	}

	setGatewayRequestRanges(parsed, protocol, jsonStr)
	return nil
}

func refreshGatewayRequestRanges(parsed *ParsedRequest, protocol string) error {
	return parseGatewayRequestCurrentBody(parsed, protocol)
}

// DescribeInvalidJSON returns a diagnostic error for a request body that
// failed JSON validation. It re-parses with encoding/json (failure path only)
// to pinpoint the first offending byte, so operators can distinguish genuinely
// invalid JSON from a truncated / partially consumed body. The error carries
// only length/offset/character information — never body content — so callers
// may safely wrap or log it.
func DescribeInvalidJSON(body []byte) error {
	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			return fmt.Errorf("invalid json (len=%d, offset=%d): %s", len(body), syntaxErr.Offset, syntaxErr.Error())
		}
		return fmt.Errorf("invalid json (len=%d): %w", len(body), err)
	}
	// gjson rejected the body but encoding/json accepted it (divergent edge
	// cases, e.g. certain malformed UTF-8 sequences); report the basics.
	return fmt.Errorf("invalid json (len=%d)", len(body))
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
	Body            *RequestBodyRef // 原始请求体引用（保留用于转发）；替换内容请走 ReplaceBody
	Model           string          // 请求的模型名称
	Stream          bool            // 是否为流式请求
	MetadataUserID  string          // metadata.user_id（用于会话亲和）
	HasSystem       bool            // 是否包含 system 字段（包含 null 也视为显式传入）
	ThinkingEnabled bool            // 是否开启 thinking（部分平台会影响最终模型名）
	OutputEffort    string          // output_config.effort（Claude API 的推理强度控制）
	MaxTokens       int             // max_tokens 值（用于探测请求拦截）
	SessionContext  *SessionContext // 可选：请求上下文区分因子（nil 时行为不变）

	protocol      string    // 当前 Body 的协议格式，用于 Body 替换后刷新 raw range
	systemRange   jsonRange // system/systemInstruction.parts 的 raw JSON 范围，绑定 Body 当前内容
	messagesRange jsonRange // messages/contents 的 raw JSON 范围，绑定 Body 当前内容
	inputRange    jsonRange // Responses API input 的 raw JSON 范围，绑定 Body 当前内容

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
func ParseGatewayRequest(body *RequestBodyRef, protocol string) (*ParsedRequest, error) {
	parsed := &ParsedRequest{Body: body}
	if err := parseGatewayRequestCurrentBody(parsed, protocol); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (p *ParsedRequest) raw(r jsonRange) []byte {
	if p == nil || p.Body == nil || !r.exists() {
		return nil
	}
	body := p.Body.Bytes()
	if r.end > len(body) {
		return nil
	}
	return body[r.start:r.end]
}

func (p *ParsedRequest) SystemRaw() []byte {
	return p.raw(p.systemRange)
}

func (p *ParsedRequest) MessagesRaw() []byte {
	return p.raw(p.messagesRange)
}

func (p *ParsedRequest) InputRaw() []byte {
	return p.raw(p.inputRange)
}

func (p *ParsedRequest) DecodeSystem(dst any) error {
	raw := p.SystemRaw()
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func (p *ParsedRequest) DecodeMessages(dst any) error {
	raw := p.MessagesRaw()
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func (p *ParsedRequest) SystemValue() (any, bool) {
	raw := p.SystemRaw()
	if len(raw) == 0 {
		return nil, false
	}
	var system any
	if err := json.Unmarshal(raw, &system); err != nil {
		return nil, false
	}
	return system, true
}

// CloneForBody 为单次账号尝试创建独立 body 视图，避免 failover 复用已改写的 ParsedRequest。
func (p *ParsedRequest) CloneForBody(body []byte) (*ParsedRequest, error) {
	if p == nil {
		return nil, fmt.Errorf("parse request: empty request")
	}
	clone := *p
	clone.Body = NewRequestBodyRef(body)
	clone.OnUpstreamAccepted = nil
	if err := refreshGatewayRequestRanges(&clone, clone.protocol); err != nil {
		return nil, err
	}
	return &clone, nil
}

// ReplaceBody 统一刷新当前 body 和 raw range，保证后续 helper 读取的是最新请求体。
func (p *ParsedRequest) ReplaceBody(data []byte) error {
	if p == nil {
		return fmt.Errorf("parse request: empty request")
	}
	if p.Body == nil {
		p.Body = NewRequestBodyRef(data)
	} else {
		p.Body.Replace(data)
	}
	if err := refreshGatewayRequestRanges(p, p.protocol); err != nil {
		clearGatewayRequestRanges(p)
		return err
	}
	return nil
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
// This prevents 400 errors from invalid thinking block signatures.
//
// mappedModel 是「实际发给上游的模型 ID」(after account model mapping)，用于按
// 协议族分流。仅 anthropic-strict 走原过滤逻辑；passback-required 与 unknown
// 一律保留全部 thinking block，避免误伤第三方兼容上游
// (DeepSeek `/anthropic`、Kimi `/coding`、GLM、Moonshot 等)，详见
// .pensieve/short-term/knowledge/thinking-block-filter-third-party-upstream-inversion/。
//
// 策略 (anthropic-strict only)：
//   - 当 thinking.type 不是 "enabled"/"adaptive"：移除所有 thinking 相关块
//   - 当 thinking.type 是 "enabled"/"adaptive"：仅移除缺失/无效 signature 的 thinking 块（避免 400）
//     (blocks with missing/empty/dummy signatures that would cause 400 errors)
func FilterThinkingBlocks(body []byte, mappedModel string) []byte {
	if !ShouldPreFilterThinkingBlocks(mappedModel) {
		return body
	}
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
//
// mappedModel 用于按协议族分流：仅 anthropic-strict 执行上述变形；
// passback-required (DeepSeek/Kimi/GLM 等) 与 unknown 一律返回原 body，
// 因为这类上游的契约就是「thinking block 原样回传」（或我们不了解），
// retry 任何变形都不会修好 400，反而破坏契约。详见 thinking_protocol.go。
func FilterThinkingBlocksForRetry(body []byte, mappedModel string) []byte {
	// 仅 anthropic-strict 走整流；passback-required 与 unknown 都返回原 body。
	if !ShouldApplyRetryFilters(mappedModel) {
		return body
	}

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

// anthropicBetaContextManagementToken 是 context_management 字段受的 beta token。
// 与 claude.BetaContextManagement 保持一致；在本文件本地定义以避免震荡
// claude package 的该常量含义。
const anthropicBetaContextManagementToken = "context-management-2025-06-27"

// sanitizeAnthropicBodyForBetaTokens 是对 Anthropic 直连路径上 body↔beta header
// **能力维度**对称约束的统一实现，与 Bedrock 路径的
// `sanitizeBedrockFieldsForBetaTokens` 对称。
//
// 问题场景：
//   - context_management 是 Claude Code CLI 2.1.87+ 默认携带的 beta 字段
//     （含 clear_thinking_20251015 等清理策略）
//   - 其被 Anthropic 上游接受的前提是 anthropic-beta header 含
//     `context-management-2025-06-27`
//   - 若两侧不一致上游 Pydantic schema 拒收：
//     "context_management: Extra inputs are not permitted"
//
// 本函数按最终发送的 anthropic-beta header 决定是否保留 body 中的
// context_management 字段：缺 beta token → strip。这将限制完全建立在
// "能力维度" 上，与 model 名 / token type / mimicry 子路径无关。
//
// 调用约束：必须在 CCH 签名之前调用，否则签名 hash 与最终 body
// 不一致，上游会以 third-party 拒收。
//
// 返回 (sanitized, changed)：changed 表示是否发生实际删除，供调用方决定
// 是否重用原 body 引用。
func sanitizeAnthropicBodyForBetaTokens(body []byte, anthropicBetaHeader string) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}
	if !gjson.GetBytes(body, "context_management").Exists() {
		return body, false
	}
	if anthropicBetaTokensContains(anthropicBetaHeader, anthropicBetaContextManagementToken) {
		return body, false
	}
	if b, err := sjson.DeleteBytes(body, "context_management"); err == nil {
		return b, true
	} else {
		// 不应发生：gjson 刚验证过字段存在 + body 是合法 JSON。如果 sjson 仍报错，
		// 调用方会拿到 (body, false)，但此前 computeFinalAnthropicBeta 已按“strip 后”
		// 计算了 finalBeta——两侧会不一致。记录 warning 最小限度提醒运维。
		logger.LegacyPrintf("service.gateway",
			"[CtxMgmtSanitize] sjson.DeleteBytes failed unexpectedly: %v (body len=%d). "+
				"body and final anthropic-beta header may be out of sync.", err, len(body))
	}
	return body, false
}

// anthropicBetaTokensContains 检测逗号分隔的 anthropic-beta header 是否含指定 token。
// 宋体空格宽容；区分大小写（Anthropic beta token 始终是小写）。
func anthropicBetaTokensContains(header, token string) bool {
	if header == "" || token == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		if strings.TrimSpace(part) == token {
			return true
		}
	}
	return false
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
//
// mappedModel 同 FilterThinkingBlocksForRetry：仅 anthropic-strict 执行变形；
// passback-required 与 unknown 都返回原 body，避免在不熟悉的上游上盲目变形。
func FilterSignatureSensitiveBlocksForRetry(body []byte, mappedModel string) []byte {
	if !ShouldApplyRetryFilters(mappedModel) {
		return body
	}

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
	case "low", "medium", "high", "xhigh", "max":
		return &value
	default:
		return nil
	}
}

// DefaultEffortForThinkingEnabled 给"开启了 thinking 但协议层没有 effort 档位概念"
// 的国产模型族返回一个默认 effort 字符串（"high"），用于 usage_log.reasoning_effort
// 字段，避免该字段长期为 NULL 导致用量分析无法区分 thinking 开/关。
//
// 适用范围（按 ResolveThinkingProtocol 的 PassbackRequired 集合做白名单过滤）：
//   - Kimi (kimi-* / moonshot-*)
//   - GLM (glm-*)
//   - MiniMax (minimax-m*)
//   - Qwen thinking 变体 (qwen[1-4]?-*-thinking)
//
// **排除 DeepSeek**：DeepSeek 原生支持 reasoning_effort: high/max，客户端可显式指定，
// 网关不应注入默认值覆盖客户端意图（即便客户端没发，DeepSeek 上游自己会用 high default
// ——但那是上游行为，不是我们的语义注入）。
//
// 适用场景由调用方守卫：仅当 (1) ResolveThinkingProtocol == PassbackRequired
// (2) 已确认 thinking 启用（Anthropic: parsed.ThinkingEnabled；OpenAI: 见
// OpenAIBodyHasThinkingEnabled) (3) 已有 effort 解析返回 nil 三者同时成立时调用。
//
// 返回值固定指向 "high"。理由：Kimi/GLM/MiniMax 启用 thinking 都是"深度推理模式"，
// 等同 Claude/OpenAI 的 high 档位语义；用 high 比 medium/normal 更贴近实际行为，
// 也与 DeepSeek thinking-enabled 的默认 effort 一致。
//
// 未来兼容性：如果这些厂商后续加入真实 effort 档位（如 Kimi 跟进 DeepSeek 的
// reasoning_effort: high/max），客户端开始显式发 effort 值时，调用方的守卫条件 (3)
// 会因 extractor 返回非 nil 而不触发本函数，自动让出。
func DefaultEffortForThinkingEnabled(mappedModel string) *string {
	if ResolveThinkingProtocol(mappedModel) != ThinkingProtocolPassbackRequired {
		return nil
	}
	// DeepSeek 在 PassbackRequired 集合里但有原生 effort 支持，排除。
	if strings.HasPrefix(strings.ToLower(mappedModel), "deepseek-") {
		return nil
	}
	effort := "high"
	return &effort
}

// OpenAIBodyHasThinkingEnabled 检测 OpenAI 协议的请求体里是否启用了 thinking。
//
// 国产 OpenAI-兼容上游（GLM via thinkingFormat=zai / Kimi 等）在请求体里用
// `thinking: {type: "enabled"}` 或 `thinking: {type: "adaptive"}` 表达启用。
// 仅 "enabled" / "adaptive" 视为开启；"disabled" 或缺省 → 视为关闭。
//
// 配合 DefaultEffortForThinkingEnabled 使用：OpenAI 路径上 reasoning_effort 解析为空
// 但本函数返回 true 时，给 usage_log 填默认 effort。
func OpenAIBodyHasThinkingEnabled(body []byte) bool {
	thinkingType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String()))
	return thinkingType == "enabled" || thinkingType == "adaptive"
}

// ApplyThinkingEnabledFallback 补丁已解析出的 effort，仅在 effort 为 nil 且
// 检测到 body 里 thinking 启用 + mappedModel 属于国产 passback-required 上游时，
// 返回 DefaultEffortForThinkingEnabled 的默认值（"high"）。不覆盖已解析出的值。
//
// 适用于 OpenAI 网关的多条路径调用方（避免重复的 if-nil 表达式）。
func ApplyThinkingEnabledFallback(effort *string, body []byte, mappedModel string) *string {
	if effort != nil {
		return effort
	}
	if !OpenAIBodyHasThinkingEnabled(body) {
		return nil
	}
	return DefaultEffortForThinkingEnabled(mappedModel)
}

// NormalizeGLMOpenAIReasoningEffort rewrites OpenAI Chat Completions
// reasoning_effort values to the GLM native scale used by z.ai: high/max.
// It only applies to glm-* mapped models and leaves all other providers untouched.
func NormalizeGLMOpenAIReasoningEffort(body []byte, mappedModel string) ([]byte, bool) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mappedModel)), "glm-") {
		return body, false
	}

	path := "reasoning.effort"
	raw := strings.TrimSpace(gjson.GetBytes(body, path).String())
	if raw == "" {
		path = "reasoning_effort"
		raw = strings.TrimSpace(gjson.GetBytes(body, path).String())
	}
	if raw == "" {
		return body, false
	}

	mapped := normalizeGLMOpenAIReasoningEffort(raw)
	if mapped == "" || mapped == raw {
		return body, false
	}

	modified, err := sjson.SetBytes(body, path, mapped)
	if err != nil {
		return body, false
	}
	return modified, true
}

func normalizeGLMOpenAIReasoningEffort(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	value = strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)

	switch value {
	case "low", "medium", "high":
		return "high"
	case "xhigh", "extrahigh", "max", "ultracode":
		return "max"
	default:
		return ""
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

// NormalizeChineseLLMThinking rewrites the top-level `thinking` object for Chinese
// LLM providers that use Anthropic-compatible endpoints but have different accepted
// values for `thinking.type`. Currently scoped to:
//   - MiniMax M-series (`MiniMax-m*`, covering M2.x / M3 / M3.x): official docs accept
//     only `thinking.type` of "adaptive" or "disabled"; "enabled" is not a valid value
//     and may be rejected/ignored. Pi-ai and other Anthropic-SDK clients default to
//     "enabled" (Anthropic-original) and never auto-rewrite for non-Anthropic models.
//
// Non-MiniMax models (Kimi/GLM/DeepSeek) currently accept "enabled" as-is, so this
// function is intentionally a no-op for them. New Chinese LLM quirks should be
// added here as separate case branches.
//
// Returns (modified body, true) if a rewrite was applied, or (original body, false)
// if no rewrite was needed. Caller should be on the Anthropic forward path AFTER
// FilterThinkingBlocks and BEFORE building the upstream request, only for
// passback-required models (ResolveThinkingProtocol == PassbackRequired).
func NormalizeChineseLLMThinking(body []byte, mappedModel string) ([]byte, bool) {
	modelLower := strings.ToLower(mappedModel)
	if !strings.HasPrefix(modelLower, "minimax-m") {
		return body, false
	}
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if thinkingType != "enabled" {
		return body, false
	}
	modified, err := sjson.SetBytes(body, "thinking.type", "adaptive")
	if err != nil {
		return body, false
	}
	return modified, true
}
