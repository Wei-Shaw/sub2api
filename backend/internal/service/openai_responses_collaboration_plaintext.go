package service

// 本文件把 apicompat 的 opt-in collaboration plaintext 适配器接入服务转发链：
// HTTP/SSE 走 gin context 的 per-request mapping，WS 长会话走 session 状态对象
// （mapping + 最近一次客户端 tools 原文，供省略 tools 的 follow-up 继承）。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

const (
	openAIResponsesCollabPlaintextMappingContextKey = "openai_responses_collaboration_plaintext_mapping"
	openAIResponsesCollabPlaintextSessionContextKey = "openai_responses_collaboration_plaintext_session"
)

// collaborationPlaintextAliasNeedle 是降级别名的稳定前缀（collaboration__<tool>），
// 用于响应体不含别名时跳过 decode 的快路径。
var collaborationPlaintextAliasNeedle = []byte(`"collaboration__`)

var collaborationPlaintextNamespaceNeedle = []byte(`"collaboration"`)
var collaborationPlaintextEscapeNeedle = []byte(`\u`)

func hasOpenAIResponsesCollabPlaintextMapping(mapping apicompat.ResponsesCollaborationPlaintextMapping) bool {
	return len(mapping.Calls) > 0
}

func openAIResponsesCollabPlaintextMapping(c *gin.Context) (apicompat.ResponsesCollaborationPlaintextMapping, bool) {
	if c == nil {
		return apicompat.ResponsesCollaborationPlaintextMapping{}, false
	}
	value, ok := c.Get(openAIResponsesCollabPlaintextMappingContextKey)
	mapping, typed := value.(apicompat.ResponsesCollaborationPlaintextMapping)
	return mapping, ok && typed && hasOpenAIResponsesCollabPlaintextMapping(mapping)
}

func setOpenAIResponsesCollabPlaintextMapping(c *gin.Context, mapping apicompat.ResponsesCollaborationPlaintextMapping) {
	if c == nil {
		return
	}
	if !hasOpenAIResponsesCollabPlaintextMapping(mapping) {
		clearOpenAIResponsesCollabPlaintextMapping(c)
		return
	}
	c.Set(openAIResponsesCollabPlaintextMappingContextKey, mapping)
}

// clearOpenAIResponsesCollabPlaintextMapping 清除上一转发尝试残留的 mapping；
// Forward 在同一 gin context 上按账号重试。
func clearOpenAIResponsesCollabPlaintextMapping(c *gin.Context) {
	if c == nil {
		return
	}
	if _, exists := c.Get(openAIResponsesCollabPlaintextMappingContextKey); exists {
		c.Set(openAIResponsesCollabPlaintextMappingContextKey, apicompat.ResponsesCollaborationPlaintextMapping{})
	}
}

// openAIResponsesCollabPlaintextMayNeedAdaptation 是廉价的字节级预检：
// 请求完全不含 collaboration 标识时跳过 decode，保持热路径零开销。
func openAIResponsesCollabPlaintextMayNeedAdaptation(body []byte) bool {
	return bytes.Contains(body, collaborationPlaintextNamespaceNeedle) || bytes.Contains(body, collaborationPlaintextEscapeNeedle)
}

func openAIResponsesCollabPlaintextMayNeedRestore(payload []byte) bool {
	return bytes.Contains(payload, collaborationPlaintextAliasNeedle) || bytes.Contains(payload, collaborationPlaintextEscapeNeedle)
}

func openAIResponsesCollabPlaintextMalformedAliasError(payload []byte, mapping apicompat.ResponsesCollaborationPlaintextMapping) error {
	for alias := range mapping.Calls {
		for start := range payload {
			position := start
			for index := range len(alias) {
				if position >= len(payload) {
					break
				}
				if payload[position] == alias[index] {
					position++
				} else if position+6 <= len(payload) && payload[position] == '\\' && payload[position+1] == 'u' &&
					payload[position+2] == '0' && payload[position+3] == '0' &&
					openAIResponsesCollabPlaintextHexByte(payload[position+4], payload[position+5]) == alias[index] {
					position += 6
				} else {
					break
				}
				if index == len(alias)-1 {
					return errors.New("mapped collaboration plaintext alias present in malformed upstream payload")
				}
			}
		}
	}
	return nil
}

func openAIResponsesCollabPlaintextHexByte(high, low byte) byte {
	hex := func(value byte) byte {
		switch {
		case value >= '0' && value <= '9':
			return value - '0'
		case value >= 'a' && value <= 'f':
			return value - 'a' + 10
		case value >= 'A' && value <= 'F':
			return value - 'A' + 10
		default:
			return 0xff
		}
	}
	return hex(high)<<4 | hex(low)
}

// errOpenAIResponsesCollabPlaintextUnsupported 是 opt-in 账号配置了不支持
// 的出站模式时的确定性校验错误。所有入口统一拒绝而不是静默改道或丢弃密文。
var errOpenAIResponsesCollabPlaintextUnsupported = errors.New(
	"openai_responses_plaintext_collaboration is only supported on OpenAI API-key/OAuth-like accounts forwarding via native Responses; the configured outbound mode cannot carry the adaptation")

// openAIResponsesCollabPlaintextSupportedOutbound 限定该适配器的合法出口：
// OpenAI 平台 API-key 与 OAuth-like（含 SetupToken）账号的 native Responses
// 出口。force_chat_completions/Anthropic 协议等其他出站模式不支持该降级。
func openAIResponsesCollabPlaintextSupportedOutbound(account *Account) bool {
	if !account.IsOpenAIResponsesPlaintextCollaborationEnabled() {
		return false
	}
	if !account.IsOpenAIApiKey() && !account.IsOpenAIOAuthLike() {
		return false
	}
	if account.IsAnthropicProtocol() {
		return false
	}
	if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
		return false
	}
	return true
}

// adaptOpenAIResponsesCollabPlaintextBody 在账号 opt-in 且请求含 collaboration
// 标识时执行降级，并把 mapping 存入 context 供响应侧还原。未发生实际改写时
// 返回原始字节。同一 context 上已有非空 mapping 时视为本 attempt 已完成适配
// （body 已是降级形态），直接返回以保住既有 mapping——重复适配会把别名集合
// 重置为空并静默放行上游别名。
func adaptOpenAIResponsesCollabPlaintextBody(c *gin.Context, account *Account, body []byte) ([]byte, error) {
	if !account.IsOpenAIResponsesPlaintextCollaborationEnabled() {
		return body, nil
	}
	if !openAIResponsesCollabPlaintextSupportedOutbound(account) {
		return body, errOpenAIResponsesCollabPlaintextUnsupported
	}
	if _, adapted := openAIResponsesCollabPlaintextMapping(c); adapted {
		return body, nil
	}
	if !openAIResponsesCollabPlaintextMayNeedAdaptation(body) {
		return body, nil
	}
	var requestBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &requestBody); err != nil {
		return body, fmt.Errorf("decode OpenAI collaboration plaintext request: %w", err)
	}
	mapping, changed, err := apicompat.AdaptResponsesCollaborationPlaintext(requestBody)
	if err != nil {
		return body, err
	}
	setOpenAIResponsesCollabPlaintextMapping(c, mapping)
	if !changed {
		return body, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, fmt.Errorf("encode OpenAI collaboration plaintext request: %w", err)
	}
	return rebuilt, nil
}

// restoreOpenAIResponsesCollabPlaintextPayload 用请求级 mapping 还原一个上游
// JSON 载荷（非流式响应体或单个 SSE/WS 事件）。无 mapping、载荷不含别名、
// 或载荷不是合法 JSON 时原样返回。
func restoreOpenAIResponsesCollabPlaintextPayload(c *gin.Context, payload []byte) ([]byte, error) {
	mapping, ok := openAIResponsesCollabPlaintextMapping(c)
	if !ok {
		return payload, nil
	}
	// mapping 存在但载荷不是合法 JSON 时不能静默放行：payload 里的
	// collaboration__ 别名会原样泄漏给客户端。[DONE]/SSE 注释等合法
	// 非 JSON 帧不含别名 needle，不会被这条路径拦下。
	if !json.Valid(payload) {
		if err := openAIResponsesCollabPlaintextMalformedAliasError(payload, mapping); err != nil {
			return nil, err
		}
		return payload, nil
	}
	if !openAIResponsesCollabPlaintextMayNeedRestore(payload) {
		return payload, nil
	}
	restored, _, err := apicompat.RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
	return restored, err
}

// openAIWSCollabPlaintextSession 维护 WS 会话级适配状态：当前 mapping 与最近
// 一个显式声明 tools 的帧里的客户端原始 tools。follow-up 帧省略 tools 时按
// 契约继承上一份有效声明：先注入原文再重新适配，使 input 中的 namespace
// function_call 仍能由本帧派生的 mapping 改写为别名。
// filter goroutine（客户端帧）与 WriteFrame goroutine（上游帧）并发访问，须加锁。
type openAIWSCollabPlaintextSession struct {
	mu            sync.Mutex
	accountID     int64
	enabled       bool
	mapping       apicompat.ResponsesCollaborationPlaintextMapping
	toolsRaw      json.RawMessage
	toolsDeclared bool
}

type openAIWSCollabPlaintextPrepared struct {
	payload       []byte
	toolsRaw      json.RawMessage
	toolsDeclared bool
	mapping       apicompat.ResponsesCollaborationPlaintextMapping
}

// bindAccount 将会话绑定到当前账号。mapping 是 per-account 的适配产物，
// 账号切换必须作废；toolsRaw 是客户端在自己的连接上声明的原始 tools，
// 属客户端会话状态，随连接跨账号保留（与 openAIWSHTTPBridgeToolState 的
// 既有重分配契约一致）。
func (s *openAIWSCollabPlaintextSession) bindAccount(accountID int64, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountID != accountID || s.enabled != enabled {
		s.accountID = accountID
		s.enabled = enabled
		s.mapping = apicompat.ResponsesCollaborationPlaintextMapping{}
	}
}

func (s *openAIWSCollabPlaintextSession) declaredTools() json.RawMessage {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.toolsDeclared {
		return nil
	}
	return append(json.RawMessage(nil), s.toolsRaw...)
}

// Off accounts retain only the client's accepted declarations. Their payloads
// and response events never pass through the plaintext adapter.
func observeOpenAIWSCollabPlaintextClientTools(c *gin.Context, account *Account, payload []byte) {
	if c == nil || account == nil || account.IsOpenAIResponsesPlaintextCollaborationEnabled() {
		return
	}
	prepared, _ := prepareOpenAIWSCollabPlaintextPayload(nil, payload, false)
	commitOpenAIWSCollabPlaintextPayload(c, account, prepared)
}

func prepareOpenAIWSCollabPlaintextPayload(s *openAIWSCollabPlaintextSession, payload []byte, enabled bool) (openAIWSCollabPlaintextPrepared, error) {
	prepared := openAIWSCollabPlaintextPrepared{payload: payload}
	if len(payload) == 0 || !json.Valid(payload) {
		return prepared, nil
	}
	if tools, present := openAIWSHTTPBridgeRawField(payload, "tools"); present {
		trimmed := bytes.TrimSpace(tools)
		if len(trimmed) > 0 && (trimmed[0] == '[' || bytes.Equal(trimmed, []byte("null"))) {
			prepared.toolsRaw = tools
			prepared.toolsDeclared = true
		} else if enabled {
			return prepared, errors.New("websocket collaboration plaintext tools must be an array or null")
		}
	}
	if !enabled {
		return prepared, nil
	}
	if s == nil {
		return prepared, errors.New("websocket collaboration plaintext session is nil")
	}
	if !prepared.toolsDeclared {
		if inherited := s.declaredTools(); len(inherited) > 0 {
			next, err := sjson.SetRawBytes(payload, "tools", inherited)
			if err != nil {
				return prepared, fmt.Errorf("inherit collaboration plaintext tools: %w", err)
			}
			prepared.payload = next
		} else if !openAIResponsesCollabPlaintextMayNeedAdaptation(payload) {
			return prepared, nil
		}
	}
	var requestBody map[string]any
	if err := decodeOpenAIJSONUseNumber(prepared.payload, &requestBody); err != nil {
		return prepared, fmt.Errorf("decode websocket collaboration plaintext request: %w", err)
	}
	mapping, changed, err := apicompat.AdaptResponsesCollaborationPlaintext(requestBody)
	if err != nil {
		return prepared, err
	}
	prepared.mapping = mapping
	if changed {
		rebuilt, marshalErr := marshalOpenAIUpstreamJSON(requestBody)
		if marshalErr != nil {
			return prepared, fmt.Errorf("encode websocket collaboration plaintext request: %w", marshalErr)
		}
		prepared.payload = rebuilt
	}
	return prepared, nil
}

func commitOpenAIWSCollabPlaintextPayload(c *gin.Context, account *Account, prepared openAIWSCollabPlaintextPrepared) {
	if c == nil || account == nil {
		return
	}
	session := openAIWSCollabPlaintextSessionFromContext(c)
	if session == nil {
		if !prepared.toolsDeclared {
			return
		}
		session = &openAIWSCollabPlaintextSession{accountID: account.ID, enabled: account.IsOpenAIResponsesPlaintextCollaborationEnabled()}
		setOpenAIWSCollabPlaintextSession(c, session)
	}
	session.commitPayload(prepared, account.IsOpenAIResponsesPlaintextCollaborationEnabled())
}

func (s *openAIWSCollabPlaintextSession) commitPayload(prepared openAIWSCollabPlaintextPrepared, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prepared.toolsDeclared {
		s.toolsRaw = append(s.toolsRaw[:0], prepared.toolsRaw...)
		s.toolsDeclared = true
	}
	if enabled {
		s.mapping = prepared.mapping
	}
}

func openAIWSCollabPlaintextSessionFromContext(c *gin.Context) *openAIWSCollabPlaintextSession {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIResponsesCollabPlaintextSessionContextKey)
	if !ok {
		return nil
	}
	session, _ := value.(*openAIWSCollabPlaintextSession)
	return session
}

func setOpenAIWSCollabPlaintextSession(c *gin.Context, session *openAIWSCollabPlaintextSession) {
	if c == nil {
		return
	}
	c.Set(openAIResponsesCollabPlaintextSessionContextKey, session)
}

// openAIWSCollabPlaintextSessionForAccount 返回该账号在当前 context 上活跃的
// 会话状态。账号未 opt-in 返回 (nil, nil)；opt-in 但出站模式不支持返回
// errOpenAIResponsesCollabPlaintextUnsupported——入口必须把它变成明确的
// 拒绝（HTTP 400 / WS close），而不是静默按原样转发。
// 已有会话属于别的账号时保留客户端声明的 toolsRaw、仅重置 per-account
// mapping，保证 A 的别名不会用于 B 的响应还原。
func openAIWSCollabPlaintextSessionForAccount(c *gin.Context, account *Account) (*openAIWSCollabPlaintextSession, error) {
	if c == nil || account == nil {
		return nil, nil
	}
	enabled := account.IsOpenAIResponsesPlaintextCollaborationEnabled()
	if !enabled {
		if existing := openAIWSCollabPlaintextSessionFromContext(c); existing != nil {
			existing.bindAccount(account.ID, false)
		}
		return nil, nil
	}
	if !openAIResponsesCollabPlaintextSupportedOutbound(account) {
		return nil, errOpenAIResponsesCollabPlaintextUnsupported
	}
	if existing := openAIWSCollabPlaintextSessionFromContext(c); existing != nil {
		existing.bindAccount(account.ID, true)
		return existing, nil
	}
	session := &openAIWSCollabPlaintextSession{accountID: account.ID, enabled: true}
	setOpenAIWSCollabPlaintextSession(c, session)
	return session, nil
}

// adaptPayload 处理一个发往上游的 response.create 帧：显式 tools（含空数组/
// null）视为整体替换并保存原文；省略 tools 且此前声明过时注入缓存原文。
// 映射随每个声明帧重算并整体替换，保证上一帧的别名不会泄漏到本帧。
func (s *openAIWSCollabPlaintextSession) adaptPayload(payload []byte) ([]byte, error) {
	if s == nil {
		return payload, nil
	}
	prepared, err := prepareOpenAIWSCollabPlaintextPayload(s, payload, true)
	if err != nil {
		return payload, err
	}
	s.commitPayload(prepared, true)
	return prepared.payload, nil
}

// restore 用当前 mapping 还原一个上游→客户端帧；别名缺失时原样返回，
// 还原错误（如非空/畸形的 encrypted_function_args）原样上抛。mapping
// 存在而载荷不是合法 JSON 时返回错误，避免别名原样泄漏给客户端。
func (s *openAIWSCollabPlaintextSession) restore(payload []byte) ([]byte, error) {
	if s == nil {
		return payload, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !hasOpenAIResponsesCollabPlaintextMapping(s.mapping) {
		return payload, nil
	}
	if !json.Valid(payload) {
		if err := openAIResponsesCollabPlaintextMalformedAliasError(payload, s.mapping); err != nil {
			return nil, err
		}
		return payload, nil
	}
	if !openAIResponsesCollabPlaintextMayNeedRestore(payload) {
		return payload, nil
	}
	restored, _, err := apicompat.RestoreResponsesCollaborationPlaintextPayload(payload, s.mapping)
	return restored, err
}
