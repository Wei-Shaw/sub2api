package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OpenCode Zen 免费层门禁伪装。
//
// 上游对免费层模型（模型 ID 以 -free 结尾）实施「仅允许官方 OpenCode CLI
// 发起」的客户端身份门禁，判定是四组独立信号的与，缺一即 403 FreeTierError
// "OpenCode's free tier can only be used from within OpenCode"：
//
//  1. X-Opencode-Session 必须匹配 ses_ 规范形状（UUID / 短串一律拒绝）
//  2. User-Agent 必须为 opencode/<version> 且 version >= 1.18.0
//     （1.17.9 返回 426 UpgradeRequired）
//  3. body 的 tools 必须同时含 bash 与 read（只含其一仍 403）
//  4. body 的 stream 必须为 true（false 返回 403）
//
// 以上四条均为对 https://opencode.ai/zen/v1/chat/completions 的实测结论
// （Authorization: Bearer public，模型 mimo-v2.6-flash-free，2026-09-30）。
// X-Opencode-Request / Client / Project 经实测非必需，故不发送以免无谓地
// 放大与官方流量的形态差异。
//
// 注入的 bash / read 工具对模型可见，模型理论上可能发起同名工具调用，因此
// 伪装默认只作用于 -free 模型（见 free_tier_gate=auto），并可被账号级开关
// 收窄或关闭。

const (
	// openCodeFreeTierGateCredential 是账号级门禁开关的凭据键。
	openCodeFreeTierGateCredential = "free_tier_gate"

	// openCodeGateModeAuto 仅对 -free 模型伪装（缺省）。
	openCodeGateModeAuto = "auto"
	// openCodeGateModeAlways 对该账号全部 OpenCode 请求伪装，用于上游规则
	// 收紧时的应急开关。
	openCodeGateModeAlways = "always"
	// openCodeGateModeOff 完全关闭伪装。
	openCodeGateModeOff = "off"

	// 门禁 body 方言。三条出站 funnel 各自明确知道自己转发的是哪种协议，
	// 因此方言由调用方显式传入，不做启发式嗅探。
	openCodeGateDialectChat      = "chat"
	openCodeGateDialectResponses = "responses"
	openCodeGateDialectAnthropic = "anthropic"
)

// openCodeCanonicalSessionRe 是上游接受的会话 ID 形状：ses_ + 12 位小写 hex
// + 14 位字母数字，总长恒为 30。
var openCodeCanonicalSessionRe = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

// openCodeGateToolNames 是上游要求同时存在的工具名，顺序即注入顺序。
var openCodeGateToolNames = []string{"bash", "read"}

// canonicalOpenCodeID 把任意来源字符串确定性映射到上游规范 ID 形状
// （<prefix>_ + 12 位小写 hex + 14 位字母数字，共 30 字符）。
//
// 确定性是硬要求而非优化：会话 ID 每轮随机会让上游 prompt cache 永不命中，
// 因此同一来源必须恒等映射。
func canonicalOpenCodeID(prefix, source string) string {
	sum := sha256.Sum256([]byte(source))
	hexPart := hex.EncodeToString(sum[:6])
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	suffix := make([]byte, 14)
	for i := 0; i < 14; i++ {
		suffix[i] = alphabet[int(sum[6+i])%len(alphabet)]
	}
	return prefix + "_" + hexPart + string(suffix)
}

// isOpenCodeFreeTierModel 判断模型是否为免费层模型。判定大小写不敏感，并
// 先剥离 opencode-go/ 等路由前缀。
func isOpenCodeFreeTierModel(model string) bool {
	model = normalizeOpenCodeGoModelID(model)
	return model != "" && strings.HasSuffix(model, "-free")
}

// openCodeFreeTierGateMode 返回账号的门禁模式，缺省 auto，非法值回落 auto。
// 非法值不应出现（写入路径已校验），此处兜底是为了让历史数据保持可用。
func openCodeFreeTierGateMode(account *Account) string {
	if account == nil {
		return openCodeGateModeAuto
	}
	switch mode := strings.ToLower(strings.TrimSpace(account.GetCredential(openCodeFreeTierGateCredential))); mode {
	case openCodeGateModeAlways:
		return openCodeGateModeAlways
	case openCodeGateModeOff:
		return openCodeGateModeOff
	default:
		return openCodeGateModeAuto
	}
}

// shouldApplyOpenCodeFreeTierGate 判定本次请求是否需要门禁伪装。
// model 为空时 auto 模式返回 false：无法确认是否免费层，宁可不伪装。
func shouldApplyOpenCodeFreeTierGate(account *Account, model string) bool {
	if account == nil || !account.IsOpenCodeGo() {
		return false
	}
	switch openCodeFreeTierGateMode(account) {
	case openCodeGateModeOff:
		return false
	case openCodeGateModeAlways:
		return true
	default:
		return isOpenCodeFreeTierModel(model)
	}
}

// openCodeGateModelFromBody 读取出站 body 的模型名。取的是出站值而非客户端
// 原始值：门禁由上游对出站模型实施，因此判定必须与出站模型一致。
func openCodeGateModelFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	return strings.TrimSpace(gjson.GetBytes(body, "model").String())
}

// canonicalOpenCodeSessionID 把已解析出的会话来源规范化为上游接受的形状。
// 客户端已经提供的合法 ses_ 值必须原样保留——重新铸造会破坏调用方自建的
// 会话连续性。
func canonicalOpenCodeSessionID(source string) string {
	source = strings.TrimSpace(source)
	if openCodeCanonicalSessionRe.MatchString(source) {
		return source
	}
	if source == "" {
		return ""
	}
	return canonicalOpenCodeID("ses", source)
}

// applyOpenCodeFreeTierGateSession 在 applyOpenCodeSessionHeader 之后调用，
// 把已经落地的会话值改写为规范形状。复用既有解析链（客户端头 →
// prompt_cache_key / metadata.user_id → 账号覆写）保证跨轮次稳定。
//
// 必须在 applyOpenCodeSessionHeader 之后调用，原因见 openCodeGateSessionSeed：
// Zen 模式下既有的自动生成条件不成立，会话头可能压根不存在，此时本函数负责
// 补一个，否则免费模型必然 403。
func applyOpenCodeFreeTierGateSession(account *Account, headers http.Header, body []byte) {
	if headers == nil {
		return
	}
	if !shouldApplyOpenCodeFreeTierGate(account, openCodeGateModelFromBody(body)) {
		return
	}
	current := existingOpenCodeSessionHeader(headers)
	if current == "" {
		current = openCodeGateSessionSeed(body)
	}
	canonical := canonicalOpenCodeSessionID(current)
	if canonical == "" {
		// 既无客户端会话标识、也无法从请求体推导稳定种子时的最后兜底：
		// 随机值会让 prompt cache 不命中，但至少满足上游形状要求。
		canonical = canonicalOpenCodeID("ses", uuid.NewString())
	}
	for key := range headers {
		if strings.EqualFold(key, openCodeSessionHeader) {
			delete(headers, key)
		}
	}
	headers.Set(openCodeSessionHeader, canonical)
}

// openCodeGateSessionSeed 在既有一切会话来源都缺失时，从请求体推导一个跨轮次
// 稳定的会话种子。
//
// 为什么需要它：applyOpenCodeSessionHeader 只在 shouldGenerateOpenCodeSession
// 为真时生成兜底值，而该条件对 Zen 账号不成立——它要求目标路径含 /zen/go，
// Zen 的路径是 /zen/v1。因此「Zen 账号 + 客户端未提供会话标识」这一组合下，
// 出站请求完全没有 X-Opencode-Session，免费层门禁必然拒绝。
//
// 种子取模型名 + 首条用户消息，与 CPA 插件的 content fallback 同构：同一会话
// 跨轮次稳定（首条用户消息不变），从而不击穿上游 prompt cache。
func openCodeGateSessionSeed(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if id := strings.TrimSpace(openCodeSessionIDFromPayload(body)); id != "" {
		return id
	}
	text := openCodeGateFirstUserText(body)
	if text == "" {
		return ""
	}
	return "content:" + openCodeGateModelFromBody(body) + "\x00" + text
}

// openCodeGateFirstUserText 取首条用户消息的文本，兼容 chat / responses 两种
// 载荷形状（content 为字符串或 parts 数组，responses 的 input 还可为裸字符串）。
func openCodeGateFirstUserText(body []byte) string {
	for _, path := range []string{"messages", "input"} {
		node := gjson.GetBytes(body, path)
		if node.Type == gjson.String {
			if text := strings.TrimSpace(node.String()); text != "" {
				return text
			}
			continue
		}
		if !node.IsArray() {
			continue
		}
		for _, item := range node.Array() {
			if item.Get("role").String() != "user" {
				continue
			}
			if text := openCodeGateContentText(item.Get("content")); text != "" {
				return text
			}
		}
	}
	return ""
}

func openCodeGateContentText(content gjson.Result) string {
	if content.Type == gjson.String {
		return strings.TrimSpace(content.String())
	}
	if content.IsArray() {
		for _, part := range content.Array() {
			if text := strings.TrimSpace(part.Get("text").String()); text != "" {
				return text
			}
		}
	}
	return ""
}

// applyOpenCodeFreeTierGateBody 对出站 body 施加门禁要求：强制 stream:true，
// 并按方言补齐缺失的 bash / read 工具。dialect 取 openCodeGateDialect* 之一。
//
// body 不是合法 JSON、或 tools 存在但不是数组时原样返回：门禁伪装是尽力而为
// 的上游兼容层，绝不能因为形状意外而让请求在网关侧失败。
func applyOpenCodeFreeTierGateBody(account *Account, body []byte, dialect string) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	if !shouldApplyOpenCodeFreeTierGate(account, openCodeGateModelFromBody(body)) {
		return body, nil
	}
	if !gjson.ValidBytes(body) {
		return body, nil
	}

	out, err := sjson.SetBytes(body, "stream", true)
	if err != nil {
		return body, fmt.Errorf("opencode free tier gate: force stream: %w", err)
	}

	tools, ok, err := openCodeGateToolsWithRequired(out, dialect)
	if err != nil {
		return body, err
	}
	if !ok {
		// tools 存在但不是数组：形状未知，不猜测也不改动。
		return out, nil
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return body, fmt.Errorf("opencode free tier gate: encode tools: %w", err)
	}
	out, err = sjson.SetRawBytes(out, "tools", encoded)
	if err != nil {
		return body, fmt.Errorf("opencode free tier gate: write tools: %w", err)
	}
	return out, nil
}

// openCodeGateToolsWithRequired 返回补齐 bash / read 之后的 tools 数组。
// 第二个返回值为 false 表示 tools 存在但不是数组，调用方应放弃改动。
func openCodeGateToolsWithRequired(body []byte, dialect string) ([]json.RawMessage, bool, error) {
	raw := gjson.GetBytes(body, "tools")
	tools := make([]json.RawMessage, 0, 4)
	if raw.Exists() {
		if !raw.IsArray() {
			return nil, false, nil
		}
		for _, item := range raw.Array() {
			if item.Raw == "" {
				continue
			}
			tools = append(tools, json.RawMessage(item.Raw))
		}
	}

	present := make(map[string]bool, len(tools))
	for _, item := range tools {
		if name := openCodeGateToolName(item, dialect); name != "" {
			present[name] = true
		}
	}
	for _, name := range openCodeGateToolNames {
		if present[name] {
			continue
		}
		encoded, err := openCodeGateToolJSON(name, dialect)
		if err != nil {
			return nil, false, err
		}
		tools = append(tools, encoded)
	}
	return tools, true, nil
}

// openCodeGateToolName 按方言取出工具条目名。chat 方言为 function.name，
// responses 与 anthropic 方言为顶层 name。
func openCodeGateToolName(item json.RawMessage, dialect string) string {
	if dialect == openCodeGateDialectChat {
		return strings.TrimSpace(gjson.GetBytes(item, "function.name").String())
	}
	return strings.TrimSpace(gjson.GetBytes(item, "name").String())
}

// openCodeGateToolJSON 构造最小可用的工具定义。参数 schema 固定为
// {"type":"object"}：门禁只校验工具名存在，补充真实 schema 没有收益，反而
// 会扩大与官方客户端流量的差异。
func openCodeGateToolJSON(name, dialect string) (json.RawMessage, error) {
	description := "Runs a persistent bash shell session."
	if name == "read" {
		description = "Reads a file from the local filesystem."
	}
	schema := map[string]any{"type": "object"}

	var entry map[string]any
	switch dialect {
	case openCodeGateDialectChat:
		entry = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": description,
				"parameters":  schema,
			},
		}
	case openCodeGateDialectAnthropic:
		// Anthropic 自定义工具无 type 包装，参数键为 input_schema。
		entry = map[string]any{
			"name":         name,
			"description":  description,
			"input_schema": schema,
		}
	default:
		entry = map[string]any{
			"type":        "function",
			"name":        name,
			"description": description,
			"parameters":  schema,
		}
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("opencode free tier gate: encode tool %s: %w", name, err)
	}
	return encoded, nil
}

// infraInvalidFreeTierGate 构造门禁模式非法时的 400 错误。
func infraInvalidFreeTierGate() error {
	return infraerrors.New(
		http.StatusBadRequest,
		"INVALID_OPENCODE_FREE_TIER_GATE",
		"free_tier_gate must be one of auto, always, off",
	)
}

// NormalizeOpenCodeFreeTierGateCredentials 校验并原地规范化
// credentials.free_tier_gate。未携带该字段时为 no-op，使旧账号继续按 auto
// 处理。非法取值返回 400 INVALID_OPENCODE_FREE_TIER_GATE。
func NormalizeOpenCodeFreeTierGateCredentials(credentials map[string]any) error {
	if credentials == nil {
		return nil
	}
	raw, ok := credentials[openCodeFreeTierGateCredential]
	if !ok || raw == nil {
		return nil
	}
	mode, ok := raw.(string)
	if !ok {
		return infraInvalidFreeTierGate()
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case openCodeGateModeAuto:
		credentials[openCodeFreeTierGateCredential] = openCodeGateModeAuto
	case openCodeGateModeAlways:
		credentials[openCodeFreeTierGateCredential] = openCodeGateModeAlways
	case openCodeGateModeOff:
		credentials[openCodeFreeTierGateCredential] = openCodeGateModeOff
	default:
		return infraInvalidFreeTierGate()
	}
	return nil
}
