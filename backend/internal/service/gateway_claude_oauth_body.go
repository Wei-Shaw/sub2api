package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/anthropicfp"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/gin-gonic/gin"
)

type anthropicCacheControlPayload struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type anthropicSystemTextBlockPayload struct {
	Type         string                        `json:"type"`
	Text         string                        `json:"text"`
	CacheControl *anthropicCacheControlPayload `json:"cache_control,omitempty"`
}

type anthropicMetadataPayload struct {
	UserID string `json:"user_id"`
}

// replaceModelInBody 替换请求体中的model字段
// 优先使用定点修改，尽量保持客户端原始字段顺序。
func (s *GatewayService) replaceModelInBody(body []byte, newModel string) []byte {
	return ReplaceModelInBody(body, newModel)
}

type claudeOAuthNormalizeOptions struct {
	injectMetadata          bool
	metadataUserID          string
	stripSystemCacheControl bool
}

// sanitizeSystemText rewrites only the fixed OpenCode identity sentence (if present).
// We intentionally avoid broad keyword replacement in system prompts to prevent
// accidentally changing user-provided instructions.
func sanitizeSystemText(text string) string {
	if text == "" {
		return text
	}
	// Some clients include a fixed OpenCode identity sentence. Anthropic may treat
	// this as a non-Claude-Code fingerprint, so rewrite it to the canonical
	// Claude Code banner before generic "OpenCode"/"opencode" replacements.
	text = strings.ReplaceAll(
		text,
		"You are OpenCode, the best coding agent on the planet.",
		strings.TrimSpace(claudeCodeSystemPrompt),
	)
	return text
}

func marshalAnthropicSystemTextBlock(text string, includeCacheControl bool) ([]byte, error) {
	block := anthropicSystemTextBlockPayload{
		Type: "text",
		Text: text,
	}
	if includeCacheControl {
		block.CacheControl = &anthropicCacheControlPayload{
			Type: "ephemeral",
			TTL:  claude.DefaultCacheControlTTL,
		}
	}
	return json.Marshal(block)
}

func marshalAnthropicSystemTextBlockWithCacheControl(text string, cacheControl any) ([]byte, error) {
	block := map[string]any{
		"type": "text",
		"text": text,
	}
	if cacheControl != nil {
		block["cache_control"] = cacheControl
	}
	return json.Marshal(block)
}

func marshalAnthropicMetadata(userID string) ([]byte, error) {
	return json.Marshal(anthropicMetadataPayload{UserID: userID})
}

func buildJSONArrayRaw(items [][]byte) []byte {
	if len(items) == 0 {
		return []byte("[]")
	}

	total := 2
	for _, item := range items {
		total += len(item)
	}
	total += len(items) - 1

	buf := make([]byte, 0, total)
	buf = append(buf, '[')
	for i, item := range items {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, item...)
	}
	buf = append(buf, ']')
	return buf
}

func setJSONValueBytes(body []byte, path string, value any) ([]byte, bool) {
	next, err := sjson.SetBytes(body, path, value)
	if err != nil {
		return body, false
	}
	return next, true
}

func setJSONRawBytes(body []byte, path string, raw []byte) ([]byte, bool) {
	next, err := sjson.SetRawBytes(body, path, raw)
	if err != nil {
		return body, false
	}
	return next, true
}

func deleteJSONPathBytes(body []byte, path string) ([]byte, bool) {
	next, err := sjson.DeleteBytes(body, path)
	if err != nil {
		return body, false
	}
	return next, true
}

func normalizeClaudeOAuthSystemBody(body []byte, opts claudeOAuthNormalizeOptions) ([]byte, bool) {
	sys := gjson.GetBytes(body, "system")
	if !sys.Exists() {
		return body, false
	}

	out := body
	modified := false

	switch {
	case sys.Type == gjson.String:
		sanitized := sanitizeSystemText(sys.String())
		if sanitized != sys.String() {
			if next, ok := setJSONValueBytes(out, "system", sanitized); ok {
				out = next
				modified = true
			}
		}
	case sys.IsArray():
		index := 0
		sys.ForEach(func(_, item gjson.Result) bool {
			if item.Get("type").String() == "text" {
				textResult := item.Get("text")
				if textResult.Exists() && textResult.Type == gjson.String {
					text := textResult.String()
					sanitized := sanitizeSystemText(text)
					if sanitized != text {
						if next, ok := setJSONValueBytes(out, fmt.Sprintf("system.%d.text", index), sanitized); ok {
							out = next
							modified = true
						}
					}
				}
			}

			if opts.stripSystemCacheControl && item.Get("cache_control").Exists() {
				if next, ok := deleteJSONPathBytes(out, fmt.Sprintf("system.%d.cache_control", index)); ok {
					out = next
					modified = true
				}
			}

			index++
			return true
		})
	}

	return out, modified
}

func ensureClaudeOAuthMetadataUserID(body []byte, userID string) ([]byte, bool) {
	if strings.TrimSpace(userID) == "" {
		return body, false
	}

	metadata := gjson.GetBytes(body, "metadata")
	if !metadata.Exists() || metadata.Type == gjson.Null {
		raw, err := marshalAnthropicMetadata(userID)
		if err != nil {
			return body, false
		}
		return setJSONRawBytes(body, "metadata", raw)
	}

	trimmedRaw := strings.TrimSpace(metadata.Raw)
	if strings.HasPrefix(trimmedRaw, "{") {
		existing := metadata.Get("user_id")
		if existing.Exists() && existing.Type == gjson.String && existing.String() != "" {
			return body, false
		}
		return setJSONValueBytes(body, "metadata.user_id", userID)
	}

	raw, err := marshalAnthropicMetadata(userID)
	if err != nil {
		return body, false
	}
	return setJSONRawBytes(body, "metadata", raw)
}

func normalizeClaudeOAuthRequestBody(body []byte, modelID string, opts claudeOAuthNormalizeOptions) ([]byte, string) {
	if len(body) == 0 {
		return body, modelID
	}

	out := body
	modified := false

	if next, changed := normalizeClaudeOAuthSystemBody(out, opts); changed {
		out = next
		modified = true
	}

	rawModel := gjson.GetBytes(out, "model")
	if rawModel.Exists() && rawModel.Type == gjson.String {
		normalized := claude.NormalizeModelID(rawModel.String())
		if normalized != rawModel.String() {
			if next, ok := setJSONValueBytes(out, "model", normalized); ok {
				out = next
				modified = true
			}
			modelID = normalized
		}
	}

	// 确保 tools 字段存在（即使为空数组）
	if !gjson.GetBytes(out, "tools").Exists() {
		if next, ok := setJSONRawBytes(out, "tools", []byte("[]")); ok {
			out = next
			modified = true
		}
	}

	if opts.injectMetadata && opts.metadataUserID != "" {
		if next, changed := ensureClaudeOAuthMetadataUserID(out, opts.metadataUserID); changed {
			out = next
			modified = true
		}
	}

	// temperature：真实 Claude Code CLI 总是发送 temperature（默认 1，客户端可覆盖）。
	// 之前的实现直接 delete 会导致 payload 缺字段，与真实 CLI 字节级不一致。
	// 策略：客户端传了什么就透传；没传则补默认 1。
	if !gjson.GetBytes(out, "temperature").Exists() {
		if next, ok := setJSONValueBytes(out, "temperature", 1); ok {
			out = next
			modified = true
		}
	}

	// max_tokens：真实 CLI 的默认值是 128000。缺失时补齐以对齐指纹。
	if !gjson.GetBytes(out, "max_tokens").Exists() {
		if next, ok := setJSONValueBytes(out, "max_tokens", 128000); ok {
			out = next
			modified = true
		}
	}

	// context_management：thinking.type 为 enabled/adaptive 时，真实 CLI 会自动
	// 附带 {"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}。
	// 客户端显式传了就透传；否则按 CLI 行为补齐。
	//
	// 注：本函数不按 model 名决定是否保留 context_management。“最终 beta
	// header 不含 context-management-2025-06-27 时 strip 字段”的能力维度
	// 对称约束由 sanitizeAnthropicBodyForBetaTokens 在 buildUpstreamRequest /
	// buildCountTokensRequest 层统一执行，与 Bedrock 路径的
	// sanitizeBedrockFieldsForBetaTokens 对称。
	if !gjson.GetBytes(out, "context_management").Exists() {
		thinkingType := gjson.GetBytes(out, "thinking.type").String()
		if thinkingType == "enabled" || thinkingType == "adaptive" {
			const cmDefault = `{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}`
			if next, ok := setJSONRawBytes(out, "context_management", []byte(cmDefault)); ok {
				out = next
				modified = true
			}
		}
	}

	// tool_choice：与 Parrot 对齐，不再无条件删除。
	// - 客户端传了 {"type":"tool","name":"X"} → 保留结构，name 由
	//   applyToolNameRewriteToBody 同步映射为假名
	// - 其他形态（auto/any/none）原样透传
	// 如果 body 里完全没有 tools（空数组），tool_choice 没意义时才删除
	if !gjson.GetBytes(out, "tools").IsArray() || len(gjson.GetBytes(out, "tools").Array()) == 0 {
		if gjson.GetBytes(out, "tool_choice").Exists() {
			if next, ok := deleteJSONPathBytes(out, "tool_choice"); ok {
				out = next
				modified = true
			}
		}
	}

	if !modified {
		return body, modelID
	}

	return out, modelID
}

func (s *GatewayService) buildOAuthMetadataUserID(parsed *ParsedRequest, account *Account, fp *Fingerprint) string {
	if parsed == nil || account == nil {
		return ""
	}
	if parsed.MetadataUserID != "" {
		return ""
	}

	userID := strings.TrimSpace(account.GetClaudeUserID())
	if userID == "" && fp != nil {
		userID = fp.ClientID
	}
	if userID == "" {
		// Fall back to a random, well-formed client id so we can still satisfy
		// Claude Code OAuth requirements when account metadata is incomplete.
		userID = generateClientID()
	}

	// session_id 用"会话级稳定种子"派生（账号 + 客户端区分因子 + 首条 user 文本）：
	// 随对话在尾部追加 messages 时保持不变，贴近真实 CC 进程级稳定的 session_id。
	// 不复用 GenerateSessionHash —— 后者是粘性路由键、按设计逐轮变化（见其测试）。
	var firstUserText string
	if parsed.Body != nil {
		firstUserText = extractFirstUserText(parsed.Body.Bytes())
	}
	seed := buildStableSessionSeed(account.ID, sessionContextDiscriminator(parsed.SessionContext), firstUserText)
	sessionID := generateSessionUUID(seed)

	// 根据指纹 UA 版本选择输出格式
	var uaVersion string
	if fp != nil {
		uaVersion = ExtractCLIVersion(fp.UserAgent)
	}
	accountUUID := strings.TrimSpace(account.GetExtraString("account_uuid"))
	return FormatMetadataUserID(userID, accountUUID, sessionID, uaVersion)
}

// applyClaudeCodeOAuthMimicryToBody 将"非 Claude Code 客户端 + Claude OAuth 账号"
// 路径上原本只在 /v1/messages 里做的完整伪装应用到任意 body 上。
//
// 这是 /v1/messages 主路径上 rewriteSystemForNonClaudeCode +
// normalizeClaudeOAuthRequestBody 流程的通用版，供 OpenAI 协议兼容层
// (ForwardAsChatCompletions / ForwardAsResponses) 复用。
//
// 未抽离之前，OpenAI 协议兼容层仅做 injectClaudeCodePrompt（前置追加），
// 而仓内 /v1/messages 路径自己的注释明确说过"仅前置追加无法通过 Anthropic
// 第三方检测"；那条注释就是本函数存在的根因。
//
// 参数：
//   - ctx / c：用于读取指纹和 gateway settings；c 可为 nil（如 count_tokens）。
//   - account：必须是 OAuth 账号，且调用方已判断不是 Claude Code 客户端。
//   - body：已经 marshal 成 Anthropic /v1/messages 格式的请求体。
//   - systemRaw：body 中原始 system 字段（用于判断是否需要 rewrite）。
//   - model：最终会发给上游的模型 ID（用于模型规范化 + metadata 版本选择）。
//
// 返回：改写后的 body。即使中间任何一步失败，也会退化成原 body（不会 panic）。
func (s *GatewayService) applyClaudeCodeOAuthMimicryToBody(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	systemRaw any,
	model string,
) []byte {
	if account == nil || !account.IsOAuth() || len(body) == 0 {
		return body
	}

	systemPromptInjectionEnabled, systemPrompt, systemPromptBlocks := s.claudeOAuthSystemPromptInjectionSettings(ctx)
	systemRewritten := false
	if systemPromptInjectionEnabled {
		body = rewriteSystemForNonClaudeCodeWithPromptBlocks(body, normalizeSystemParam(systemRaw), systemPrompt, systemPromptBlocks)
		systemRewritten = true
	}

	normalizeOpts := claudeOAuthNormalizeOptions{stripSystemCacheControl: !systemRewritten}

	if s.identityService != nil && c != nil && c.Request != nil {
		if fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header); err == nil && fp != nil {
			mimicMPT := false
			if s.settingService != nil {
				_, mimicMPT, _ = s.settingService.GetGatewayForwardingSettings(ctx)
			}
			if !mimicMPT {
				if uid := s.buildOAuthMetadataUserIDFromBody(ctx, account, fp, body); uid != "" {
					normalizeOpts.injectMetadata = true
					normalizeOpts.metadataUserID = uid
				}
			}
		}
	}

	body, _ = normalizeClaudeOAuthRequestBody(body, model, normalizeOpts)

	// Phase D+E+F: messages cache 策略 + 工具名混淆 + tools[-1] 断点
	// 对齐 Parrot transform_request 里剩余的字段级改写。顺序有语义约束：
	//   1) messages cache：仅在配置开启时清除客户端断点并注入代理断点
	//   2) tool rewrite：最后改 tools[*].name / tool_choice.name 并在 tools[-1]
	//      上打断点；mapping 存入 gin.Context 供响应侧 bytes.Replace 还原。
	body = s.rewriteMessageCacheControlIfEnabled(ctx, body)

	if rw := buildToolNameRewriteFromBody(body); rw != nil {
		body = applyToolNameRewriteToBody(body, rw)
		if c != nil {
			c.Set(toolNameRewriteKey, rw)
		}
	} else {
		body = applyToolsLastCacheBreakpoint(body)
	}

	return body
}

// buildOAuthMetadataUserIDFromBody 是 buildOAuthMetadataUserID 的变体，
// 适用于调用方手上没有 ParsedRequest 的场景（如 OpenAI 协议兼容层）。
//
// 与 buildOAuthMetadataUserID 的唯一区别：
//   - session hash 从 body 本体按同样规则重算，而不是读取 ParsedRequest 缓存值。
//   - 如果 body 里已经存在 metadata.user_id，则返回空（由 ensureClaudeOAuthMetadataUserID
//     自行决定是否覆盖）。
func (s *GatewayService) buildOAuthMetadataUserIDFromBody(
	ctx context.Context,
	account *Account,
	fp *Fingerprint,
	body []byte,
) string {
	_ = ctx
	if account == nil {
		return ""
	}
	if existing := gjson.GetBytes(body, "metadata.user_id").String(); existing != "" {
		return ""
	}

	userID := strings.TrimSpace(account.GetClaudeUserID())
	if userID == "" && fp != nil {
		userID = fp.ClientID
	}
	if userID == "" {
		userID = generateClientID()
	}

	// 与 buildOAuthMetadataUserID 一致：用会话级稳定种子，避免整 body 哈希导致
	// 每轮（甚至每个 token 变化）都重算出不同的 session_id。
	var clientDiscriminator string
	if fp != nil {
		clientDiscriminator = fp.ClientID
	}
	seed := buildStableSessionSeed(account.ID, clientDiscriminator, extractFirstUserText(body))
	sessionID := generateSessionUUID(seed)

	var uaVersion string
	if fp != nil {
		uaVersion = ExtractCLIVersion(fp.UserAgent)
	}
	accountUUID := strings.TrimSpace(account.GetExtraString("account_uuid"))
	return FormatMetadataUserID(userID, accountUUID, sessionID, uaVersion)
}

// buildStableSessionSeed 为伪装路径合成的 metadata.user_id session_id 生成"会话级稳定"种子。
//
// 真实 Claude Code 的 session_id 是进程级随机 UUID，在一段会话内跨请求保持不变。无状态代理
// 无法恢复该值，这里用"会话内不变的锚点"近似：账号 ID + 客户端区分因子 + 首条 user 消息文本。
// 对话在尾部追加 messages 时这三者都不变，因此 generateSessionUUID(seed) 跨轮稳定。
//
// 注意：粘性路由键 GenerateSessionHash 按设计逐轮变化（见其测试），本函数与之独立、互不影响。
// accountID 恒存在，故 seed 永不为空 —— 输出始终是确定性 UUID，而非随机值。
func buildStableSessionSeed(accountID int64, clientDiscriminator, firstUserText string) string {
	var b strings.Builder
	_, _ = b.WriteString(strconv.FormatInt(accountID, 10))
	_, _ = b.WriteString("::")
	_, _ = b.WriteString(clientDiscriminator)
	_, _ = b.WriteString("::")
	_, _ = b.WriteString(firstUserText)
	return b.String()
}

// sessionContextDiscriminator 把请求上下文（客户端 IP / 归一化 UA / API Key ID）拼成
// 一个跨客户端的区分因子，避免不同用户的相同首条消息派生出相同 session_id。
func sessionContextDiscriminator(sc *SessionContext) string {
	if sc == nil {
		return ""
	}
	return sc.ClientIP + ":" + NormalizeSessionUserAgent(sc.UserAgent) + ":" + strconv.FormatInt(sc.APIKeyID, 10)
}

// GenerateSessionUUID creates a deterministic UUID4 from a seed string.
func GenerateSessionUUID(seed string) string {
	return generateSessionUUID(seed)
}

func generateSessionUUID(seed string) string {
	if seed == "" {
		return uuid.NewString()
	}
	hash := sha256.Sum256([]byte(seed))
	bytes := hash[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

// normalizeSystemParam 将 json.RawMessage 类型的 system 参数转为标准 Go 类型（string / []any / nil），
// 避免 type switch 中 json.RawMessage（底层 []byte）无法匹配 case string / case []any / case nil 的问题。
// 这是 Go 的 typed nil 陷阱：(json.RawMessage, nil) ≠ (nil, nil)。
func normalizeSystemParam(system any) any {
	raw, ok := system.(json.RawMessage)
	if !ok {
		return system
	}
	if len(raw) == 0 {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	return parsed
}

// systemIncludesClaudeCodePrompt 检查 system 中是否已包含 Claude Code 提示词
// 使用前缀匹配支持多种变体（标准版、Agent SDK 版等）
func systemIncludesClaudeCodePrompt(system any) bool {
	system = normalizeSystemParam(system)
	switch v := system.(type) {
	case string:
		return hasClaudeCodePrefix(v)
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && hasClaudeCodePrefix(text) {
					return true
				}
			}
		}
	}
	return false
}

// hasClaudeCodePrefix 检查文本是否以 Claude Code 提示词的特征前缀开头
func hasClaudeCodePrefix(text string) bool {
	for _, prefix := range claudeCodePromptPrefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// injectClaudeCodePrompt 在 system 开头注入 Claude Code 提示词
// 处理 null、字符串、数组三种格式
func injectClaudeCodePrompt(body []byte, system any) []byte {
	system = normalizeSystemParam(system)
	claudeCodeBlock, err := marshalAnthropicSystemTextBlock(claudeCodeSystemPrompt, true)
	if err != nil {
		logger.LegacyPrintf("service.gateway", "Warning: failed to build Claude Code prompt block: %v", err)
		return body
	}
	// Opencode plugin applies an extra safeguard: it not only prepends the Claude Code
	// banner, it also prefixes the next system instruction with the same banner plus
	// a blank line. This helps when upstream concatenates system instructions.
	claudeCodePrefix := strings.TrimSpace(claudeCodeSystemPrompt)

	var items [][]byte

	switch v := system.(type) {
	case nil:
		items = [][]byte{claudeCodeBlock}
	case string:
		// Be tolerant of older/newer clients that may differ only by trailing whitespace/newlines.
		if strings.TrimSpace(v) == "" || strings.TrimSpace(v) == strings.TrimSpace(claudeCodeSystemPrompt) {
			items = [][]byte{claudeCodeBlock}
		} else {
			// Mirror opencode behavior: keep the banner as a separate system entry,
			// but also prefix the next system text with the banner.
			merged := v
			if !strings.HasPrefix(v, claudeCodePrefix) {
				merged = claudeCodePrefix + "\n\n" + v
			}
			nextBlock, buildErr := marshalAnthropicSystemTextBlock(merged, false)
			if buildErr != nil {
				logger.LegacyPrintf("service.gateway", "Warning: failed to build prefixed Claude Code system block: %v", buildErr)
				return body
			}
			items = [][]byte{claudeCodeBlock, nextBlock}
		}
	case []any:
		items = make([][]byte, 0, len(v)+1)
		items = append(items, claudeCodeBlock)
		prefixedNext := false
		systemResult := gjson.GetBytes(body, "system")
		if systemResult.IsArray() {
			systemResult.ForEach(func(_, item gjson.Result) bool {
				textResult := item.Get("text")
				if textResult.Exists() && textResult.Type == gjson.String &&
					strings.TrimSpace(textResult.String()) == strings.TrimSpace(claudeCodeSystemPrompt) {
					return true
				}

				raw := []byte(item.Raw)
				// Prefix the first subsequent text system block once.
				if !prefixedNext && item.Get("type").String() == "text" && textResult.Exists() && textResult.Type == gjson.String {
					text := textResult.String()
					if strings.TrimSpace(text) != "" && !strings.HasPrefix(text, claudeCodePrefix) {
						next, setErr := sjson.SetBytes(raw, "text", claudeCodePrefix+"\n\n"+text)
						if setErr == nil {
							raw = next
							prefixedNext = true
						}
					}
				}
				items = append(items, raw)
				return true
			})
		} else {
			for _, item := range v {
				m, ok := item.(map[string]any)
				if !ok {
					raw, marshalErr := json.Marshal(item)
					if marshalErr == nil {
						items = append(items, raw)
					}
					continue
				}
				if text, ok := m["text"].(string); ok && strings.TrimSpace(text) == strings.TrimSpace(claudeCodeSystemPrompt) {
					continue
				}
				if !prefixedNext {
					if blockType, _ := m["type"].(string); blockType == "text" {
						if text, ok := m["text"].(string); ok && strings.TrimSpace(text) != "" && !strings.HasPrefix(text, claudeCodePrefix) {
							m["text"] = claudeCodePrefix + "\n\n" + text
							prefixedNext = true
						}
					}
				}
				raw, marshalErr := json.Marshal(m)
				if marshalErr == nil {
					items = append(items, raw)
				}
			}
		}
	default:
		items = [][]byte{claudeCodeBlock}
	}

	result, ok := setJSONRawBytes(body, "system", buildJSONArrayRaw(items))
	if !ok {
		logger.LegacyPrintf("service.gateway", "Warning: failed to inject Claude Code prompt")
		return body
	}
	return result
}

// rewriteSystemForNonClaudeCode 将非 Claude Code 客户端的 system prompt 迁移至 messages，
// system 字段仅保留 Claude Code 标识提示词。
// Anthropic 基于 system 参数内容检测第三方应用，仅前置追加 Claude Code 提示词
// 无法通过检测，因为后续内容仍为非 Claude Code 格式。
// 策略：将原始 system prompt 提取并注入为 user/assistant 消息对，system 仅保留 Claude Code 标识。
func rewriteSystemForNonClaudeCode(body []byte, system any) []byte {
	return rewriteSystemForNonClaudeCodeWithPromptBlocks(body, system, "", "")
}

func rewriteSystemForNonClaudeCodeWithPrompt(body []byte, system any, expansionPrompt string) []byte {
	return rewriteSystemForNonClaudeCodeWithPromptBlocks(body, system, expansionPrompt, "")
}

type claudeOAuthSystemPromptBlockConfig struct {
	Enabled      *bool           `json:"enabled,omitempty"`
	Type         string          `json:"type,omitempty"`
	Text         string          `json:"text,omitempty"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

type claudeOAuthSystemPromptBlocksEnvelope struct {
	Blocks []claudeOAuthSystemPromptBlockConfig `json:"blocks"`
}

func defaultClaudeOAuthExpansionPrompt(expansionPrompt string) string {
	expansionPrompt = strings.TrimSpace(expansionPrompt)
	if expansionPrompt == "" {
		return claudeCodeSystemPromptExpansion
	}
	return expansionPrompt
}

func parseClaudeOAuthSystemPromptBlocksConfig(raw string) ([]claudeOAuthSystemPromptBlockConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var blocks []claudeOAuthSystemPromptBlockConfig
		if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
			return nil, err
		}
		return blocks, nil
	}
	var envelope claudeOAuthSystemPromptBlocksEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, err
	}
	return envelope.Blocks, nil
}

func decodeClaudeOAuthSystemPromptCacheControl(raw json.RawMessage) (any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("false")) {
		return nil, nil
	}
	if bytes.Equal(trimmed, []byte("true")) {
		return map[string]string{
			"type": "ephemeral",
			"ttl":  claude.DefaultCacheControlTTL,
		}, nil
	}
	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("cache_control must be boolean, null, or object")
	}
	return value, nil
}

func expandClaudeOAuthSystemPromptTextTemplate(body []byte, text string, expansionPrompt string) (string, error) {
	if text == "" {
		return "", nil
	}
	expansionPrompt = defaultClaudeOAuthExpansionPrompt(expansionPrompt)
	billingText, err := buildBillingAttributionText(body, claude.CLICurrentVersion)
	if err != nil {
		return "", err
	}
	fp := computeClaudeCodeFingerprint(body, claude.CLICurrentVersion)
	replacer := strings.NewReplacer(
		"{billing_header}", billingText,
		"{cc_version}", claude.CLICurrentVersion,
		"{fp}", fp,
		"{claude_code_system_prompt}", claudeCodeSystemPrompt,
		"{claude_code_expansion_prompt}", expansionPrompt,
	)
	return replacer.Replace(text), nil
}

func defaultClaudeOAuthSystemPromptBlockConfig() []claudeOAuthSystemPromptBlockConfig {
	enabled := true
	return []claudeOAuthSystemPromptBlockConfig{
		{
			Enabled: &enabled,
			Type:    "text",
			Text:    "{billing_header}",
		},
		{
			Enabled: &enabled,
			Type:    "text",
			Text:    "{claude_code_system_prompt}",
		},
		{
			Enabled: &enabled,
			Type:    "text",
			Text:    "{claude_code_expansion_prompt}",
			CacheControl: json.RawMessage(
				fmt.Sprintf(`{"type":"ephemeral","ttl":%q}`, claude.DefaultCacheControlTTL),
			),
		},
	}
}

func buildClaudeOAuthSystemPromptBlocksJSON(body []byte, expansionPrompt string, blocksConfig string) ([][]byte, error) {
	blocks, err := parseClaudeOAuthSystemPromptBlocksConfig(blocksConfig)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		blocks = defaultClaudeOAuthSystemPromptBlockConfig()
	}

	items := make([][]byte, 0, len(blocks))
	for i, block := range blocks {
		if block.Enabled != nil && !*block.Enabled {
			continue
		}
		blockType := strings.TrimSpace(block.Type)
		if blockType == "" {
			blockType = "text"
		}
		if blockType != "text" {
			return nil, fmt.Errorf("system block %d type %q is not supported", i, block.Type)
		}
		text, err := expandClaudeOAuthSystemPromptTextTemplate(body, block.Text, expansionPrompt)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		cacheControl, err := decodeClaudeOAuthSystemPromptCacheControl(block.CacheControl)
		if err != nil {
			return nil, fmt.Errorf("system block %d cache_control: %w", i, err)
		}
		raw, err := marshalAnthropicSystemTextBlockWithCacheControl(text, cacheControl)
		if err != nil {
			return nil, err
		}
		items = append(items, raw)
	}
	return items, nil
}

func ValidateClaudeOAuthSystemPromptBlocksConfig(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	blocks, err := parseClaudeOAuthSystemPromptBlocksConfig(raw)
	if err != nil {
		return infraerrors.BadRequest("INVALID_CLAUDE_OAUTH_SYSTEM_PROMPT_BLOCKS", "claude oauth system prompt blocks must be valid JSON")
	}
	for i, block := range blocks {
		blockType := strings.TrimSpace(block.Type)
		if blockType == "" {
			blockType = "text"
		}
		if blockType != "text" {
			return infraerrors.BadRequest("INVALID_CLAUDE_OAUTH_SYSTEM_PROMPT_BLOCKS", fmt.Sprintf("system block %d type must be text", i))
		}
		if _, err := decodeClaudeOAuthSystemPromptCacheControl(block.CacheControl); err != nil {
			return infraerrors.BadRequest("INVALID_CLAUDE_OAUTH_SYSTEM_PROMPT_BLOCKS", fmt.Sprintf("system block %d cache_control is invalid", i))
		}
	}
	return nil
}

func rewriteSystemForNonClaudeCodeWithPromptBlocks(body []byte, system any, expansionPrompt string, blocksConfig string) []byte {
	system = normalizeSystemParam(system)
	expansionPrompt = defaultClaudeOAuthExpansionPrompt(expansionPrompt)

	// 1. 提取原始 system prompt 文本
	var originalSystemText string
	switch v := system.(type) {
	case string:
		originalSystemText = strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		originalSystemText = strings.Join(parts, "\n\n")
	}

	// 2. 构造 system 数组，对齐真实 Claude Code CLI 的 3-block 形态：
	//    [0] billing attribution block（cc_version={cliVer}.{fp}; cc_entrypoint=cli;）
	//    [1] "You are Claude Code..." 身份前缀 block（默认不带 cache_control）
	//    [2] 工具无关的通用提示词扩充 block（带 cache_control 作为稳定缓存断点）
	//
	//    真实 CC 的 system 在身份前缀之后还有大段提示词，仅有 2 块会在块数/体量上明显
	//    区别于真实 CLI。这里注入 claudeCodeSystemPromptExpansion（中性段落）把形态做到
	//    接近真实，同时不注入会污染被代理用户行为的工具专属指令。
	//
	//    缺失 billing block 的系统 payload 是 Anthropic 判定第三方的关键信号之一
	//    （真实 CLI 每个请求都带）。新版 CLI 已取消 cch=... 签名字段，故 block 不再注入
	//    cch（见 buildBillingAttributionText）。
	systemBlocks, blockErr := buildClaudeOAuthSystemPromptBlocksJSON(body, expansionPrompt, blocksConfig)
	if blockErr != nil {
		logger.LegacyPrintf("service.gateway", "Warning: failed to build configured Claude OAuth system blocks: %v", blockErr)
		systemBlocks, blockErr = buildClaudeOAuthSystemPromptBlocksJSON(body, expansionPrompt, "")
	}
	if blockErr != nil {
		logger.LegacyPrintf("service.gateway", "Warning: failed to build default Claude OAuth system blocks: %v", blockErr)
		return body
	}
	out, ok := setJSONRawBytes(body, "system", buildJSONArrayRaw(systemBlocks))
	if !ok {
		logger.LegacyPrintf("service.gateway", "Warning: failed to set Claude Code system prompt")
		return body
	}

	// 3. 将原始 system prompt 作为 user/assistant 消息对注入到 messages 开头
	//    模型仍通过 messages 接收完整指令，保留客户端功能
	ccPromptTrimmed := strings.TrimSpace(claudeCodeSystemPrompt)
	if originalSystemText != "" && originalSystemText != ccPromptTrimmed && !hasClaudeCodePrefix(originalSystemText) {
		instrMsg, err1 := json.Marshal(map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "[System Instructions]\n" + originalSystemText},
			},
		})
		ackMsg, err2 := json.Marshal(map[string]any{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "Understood. I will follow these instructions."},
			},
		})
		if err1 != nil || err2 != nil {
			logger.LegacyPrintf("service.gateway", "Warning: failed to marshal system-to-messages injection")
			return out
		}

		// 重建 messages 数组：[instruction, ack, ...originalMessages]
		items := [][]byte{instrMsg, ackMsg}
		messagesResult := gjson.GetBytes(out, "messages")
		if messagesResult.IsArray() {
			messagesResult.ForEach(func(_, msg gjson.Result) bool {
				items = append(items, []byte(msg.Raw))
				return true
			})
		}

		if next, setOk := setJSONRawBytes(out, "messages", buildJSONArrayRaw(items)); setOk {
			out = next
		}
	}

	return out
}

type cacheControlPath struct {
	path string
	log  string
}

func collectCacheControlPaths(body []byte) (invalidThinking []cacheControlPath, messagePaths []string, toolPaths []string, systemPaths []string) {
	system := gjson.GetBytes(body, "system")
	if system.IsArray() {
		sysIndex := 0
		system.ForEach(func(_, item gjson.Result) bool {
			if item.Get("cache_control").Exists() {
				path := fmt.Sprintf("system.%d.cache_control", sysIndex)
				if item.Get("type").String() == "thinking" {
					invalidThinking = append(invalidThinking, cacheControlPath{
						path: path,
						log:  "[Warning] Removed illegal cache_control from thinking block in system",
					})
				} else {
					systemPaths = append(systemPaths, path)
				}
			}
			sysIndex++
			return true
		})
	}

	messages := gjson.GetBytes(body, "messages")
	if messages.IsArray() {
		msgIndex := 0
		messages.ForEach(func(_, msg gjson.Result) bool {
			content := msg.Get("content")
			if content.IsArray() {
				contentIndex := 0
				content.ForEach(func(_, item gjson.Result) bool {
					if item.Get("cache_control").Exists() {
						path := fmt.Sprintf("messages.%d.content.%d.cache_control", msgIndex, contentIndex)
						if item.Get("type").String() == "thinking" {
							invalidThinking = append(invalidThinking, cacheControlPath{
								path: path,
								log:  fmt.Sprintf("[Warning] Removed illegal cache_control from thinking block in messages[%d].content[%d]", msgIndex, contentIndex),
							})
						} else {
							messagePaths = append(messagePaths, path)
						}
					}
					contentIndex++
					return true
				})
			}
			msgIndex++
			return true
		})
	}

	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		toolIndex := 0
		tools.ForEach(func(_, tool gjson.Result) bool {
			if tool.Get("cache_control").Exists() {
				toolPaths = append(toolPaths, fmt.Sprintf("tools.%d.cache_control", toolIndex))
			}
			toolIndex++
			return true
		})
	}

	return invalidThinking, messagePaths, toolPaths, systemPaths
}

// enforceCacheControlLimit 强制执行 cache_control 块数量限制（最多 4 个）
// 超限时优先移除工具断点，再移除 messages 断点，最后才移除 system 断点。
func enforceCacheControlLimit(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	invalidThinking, messagePaths, toolPaths, systemPaths := collectCacheControlPaths(body)
	out := body
	modified := false

	// 先清理 thinking 块中的非法 cache_control（thinking 块不支持该字段）
	for _, item := range invalidThinking {
		if !gjson.GetBytes(out, item.path).Exists() {
			continue
		}
		next, ok := deleteJSONPathBytes(out, item.path)
		if !ok {
			continue
		}
		out = next
		modified = true
		logger.LegacyPrintf("service.gateway", "%s", item.log)
	}

	count := len(messagePaths) + len(toolPaths) + len(systemPaths)
	if count <= maxCacheControlBlocks {
		if modified {
			return out
		}
		return body
	}

	// 超限：优先从 tools 中移除，再从 messages 中移除，最后才从 system 中移除。
	remaining := count - maxCacheControlBlocks
	for i := len(toolPaths) - 1; i >= 0 && remaining > 0; i-- {
		path := toolPaths[i]
		if !gjson.GetBytes(out, path).Exists() {
			continue
		}
		next, ok := deleteJSONPathBytes(out, path)
		if !ok {
			continue
		}
		out = next
		modified = true
		remaining--
	}

	for _, path := range messagePaths {
		if remaining <= 0 {
			break
		}
		if !gjson.GetBytes(out, path).Exists() {
			continue
		}
		next, ok := deleteJSONPathBytes(out, path)
		if !ok {
			continue
		}
		out = next
		modified = true
		remaining--
	}

	for i := len(systemPaths) - 1; i >= 0 && remaining > 0; i-- {
		path := systemPaths[i]
		if !gjson.GetBytes(out, path).Exists() {
			continue
		}
		next, ok := deleteJSONPathBytes(out, path)
		if !ok {
			continue
		}
		out = next
		modified = true
		remaining--
	}

	if modified {
		return out
	}
	return body
}

// injectAnthropicCacheControlTTL1h 将已有 ephemeral cache_control 块的 ttl 强制写为 1h。
// 仅修改已经存在的 cache_control，不新增缓存断点。
func injectAnthropicCacheControlTTL1h(body []byte) []byte {
	return forceEphemeralCacheControlTTL(body, cacheTTLTarget1h)
}

func forceEphemeralCacheControlTTL(body []byte, ttl string) []byte {
	if len(body) == 0 || ttl == "" {
		return body
	}
	out := body
	var paths []string
	addPath := func(path string, value gjson.Result) {
		cc := value.Get("cache_control")
		if !cc.Exists() || cc.Get("type").String() != "ephemeral" {
			return
		}
		if cc.Get("ttl").String() == ttl {
			return
		}
		paths = append(paths, path+".cache_control.ttl")
	}

	if topCC := gjson.GetBytes(body, "cache_control"); topCC.Exists() && topCC.Get("type").String() == "ephemeral" && topCC.Get("ttl").String() != ttl {
		paths = append(paths, "cache_control.ttl")
	}

	system := gjson.GetBytes(body, "system")
	if system.IsArray() {
		idx := -1
		system.ForEach(func(_, block gjson.Result) bool {
			idx++
			addPath(fmt.Sprintf("system.%d", idx), block)
			return true
		})
	}

	messages := gjson.GetBytes(body, "messages")
	if messages.IsArray() {
		msgIdx := -1
		messages.ForEach(func(_, msg gjson.Result) bool {
			msgIdx++
			content := msg.Get("content")
			if !content.IsArray() {
				return true
			}
			contentIdx := -1
			content.ForEach(func(_, block gjson.Result) bool {
				contentIdx++
				addPath(fmt.Sprintf("messages.%d.content.%d", msgIdx, contentIdx), block)
				return true
			})
			return true
		})
	}

	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		idx := -1
		tools.ForEach(func(_, tool gjson.Result) bool {
			idx++
			addPath(fmt.Sprintf("tools.%d", idx), tool)
			return true
		})
	}

	for _, path := range paths {
		if next, err := sjson.SetBytes(out, path, ttl); err == nil {
			out = next
		}
	}
	return out
}

func (s *GatewayService) shouldInjectAnthropicCacheTTL1h(ctx context.Context, account *Account) bool {
	if account == nil || !account.IsAnthropicOAuthOrSetupToken() || s == nil || s.settingService == nil {
		return false
	}
	return s.settingService.IsAnthropicCacheTTL1hInjectionEnabled(ctx)
}

// shouldNormalizeClientDateline reports whether the request body's client
// dateline should be normalized before forwarding to Anthropic. The switch is
// scoped to Anthropic OAuth/SetupToken accounts only; API-Key accounts and
// non-Anthropic platforms bypass this step entirely.
func (s *GatewayService) shouldNormalizeClientDateline(ctx context.Context, account *Account) bool {
	if account == nil || !account.IsAnthropicOAuthOrSetupToken() || s == nil || s.settingService == nil {
		return false
	}
	return s.settingService.IsClientDatelineNormalizationEnabled(ctx)
}

// normalizeClientDatelineIfEnabled applies dateline normalization to body when
// the switch is on and the account qualifies. Returns (nextBody, true) only
// when the body actually changed; otherwise returns (nil, false) so callers
// can skip the writeback.
func (s *GatewayService) normalizeClientDatelineIfEnabled(ctx context.Context, account *Account, body []byte) ([]byte, bool) {
	if !s.shouldNormalizeClientDateline(ctx, account) {
		return nil, false
	}
	next, _, changed := anthropicfp.NormalizeDateline(body)
	if !changed {
		return nil, false
	}
	return next, true
}

func (s *GatewayService) claudeOAuthSystemPromptInjectionSettings(ctx context.Context) (bool, string, string) {
	if s == nil || s.settingService == nil {
		return true, "", ""
	}
	return s.settingService.GetClaudeOAuthSystemPromptInjectionSettings(ctx)
}
