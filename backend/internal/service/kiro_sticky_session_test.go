//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// kiroGroup 返回一个 Kiro 平台分组，用于 sticky session 测试。
func kiroGroup() *Group {
	return &Group{Platform: PlatformKiro, KiroAutoStickyEnabled: true}
}

// nonKiroGroup 返回一个 Anthropic 平台分组，用于对比测试。
func nonKiroGroup() *Group {
	return &Group{Platform: PlatformAnthropic}
}

// newKiroRequestBody 构建一个包含 system prompt 的 Anthropic 格式请求体。
// turn 控制 messages 数组长度，模拟对话的不同轮次。
func newKiroRequestBody(systemPrompt string, turns int) []byte {
	msgs := `[{"role":"user","content":"hello"}`
	for i := 1; i < turns; i++ {
		msgs += `,{"role":"assistant","content":"reply"},{"role":"user","content":"next turn"}`
	}
	msgs += `]`
	body := `{"model":"claude-sonnet-4-5","system":"` + systemPrompt + `","messages":` + msgs + `}`
	return []byte(body)
}

func newKiroRequestBodyWithoutSystem(firstUserMessage string, turns int) []byte {
	msgs := `[{"role":"user","content":"` + firstUserMessage + `"}`
	for i := 1; i < turns; i++ {
		msgs += `,{"role":"assistant","content":"reply"},{"role":"user","content":"next turn"}`
	}
	msgs += `]`
	body := `{"model":"claude-sonnet-4-5","messages":` + msgs + `}`
	return []byte(body)
}

func newKiroRequestBodyWithBodySession(systemPrompt, sessionField, sessionID string) []byte {
	body := `{"model":"claude-sonnet-4-5","` + sessionField + `":"` + sessionID + `","system":"` + systemPrompt + `","messages":[{"role":"user","content":"hello"}]}`
	return []byte(body)
}

// TestKiroStickySession_SystemPromptHashStableAcrossRounds 验证：
// Kiro 分组的请求，随着 messages 增加（多轮对话），session hash 保持不变。
func TestKiroStickySession_SystemPromptHashStableAcrossRounds(t *testing.T) {
	svc := &GatewayService{}
	systemPrompt := "You are a helpful assistant for Kiro."

	ctx := &SessionContext{APIKeyID: 42}

	makeHash := func(turns int) string {
		body := newKiroRequestBody(systemPrompt, turns)
		ref := NewRequestBodyRef(body)
		parsed, err := ParseGatewayRequest(ref, "anthropic")
		require.NoError(t, err)
		parsed.Group = kiroGroup()
		parsed.SessionContext = ctx
		return svc.GenerateSessionHash(parsed)
	}

	hash1 := makeHash(1) // 第 1 轮
	hash2 := makeHash(2) // 第 2 轮（messages 增加）
	hash3 := makeHash(5) // 第 5 轮

	require.NotEmpty(t, hash1, "第 1 轮 hash 不应为空")
	require.Equal(t, hash1, hash2, "第 2 轮 hash 应与第 1 轮相同（system prompt 未变）")
	require.Equal(t, hash1, hash3, "第 5 轮 hash 应与第 1 轮相同（system prompt 未变）")
}

// TestKiroStickySession_DifferentSystemPromptsDifferentHash 验证：
// 不同 system prompt 产生不同 hash（避免不同客户端会话被误路由到同一账号）。
func TestKiroStickySession_DifferentSystemPromptsDifferentHash(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{APIKeyID: 42}

	makeHash := func(systemPrompt string) string {
		body := newKiroRequestBody(systemPrompt, 1)
		ref := NewRequestBodyRef(body)
		parsed, err := ParseGatewayRequest(ref, "anthropic")
		require.NoError(t, err)
		parsed.Group = kiroGroup()
		parsed.SessionContext = ctx
		return svc.GenerateSessionHash(parsed)
	}

	hash1 := makeHash("System prompt A")
	hash2 := makeHash("System prompt B")

	require.NotEmpty(t, hash1)
	require.NotEmpty(t, hash2)
	require.NotEqual(t, hash1, hash2, "不同 system prompt 应产生不同 hash")
}

// TestKiroStickySession_DifferentAPIKeysDifferentHash 验证：
// 相同 system prompt 但不同 API Key，hash 不同（不同用户的会话不共享粘性绑定）。
func TestKiroStickySession_DifferentAPIKeysDifferentHash(t *testing.T) {
	svc := &GatewayService{}
	systemPrompt := "Shared system prompt"

	makeHash := func(apiKeyID int64) string {
		body := newKiroRequestBody(systemPrompt, 1)
		ref := NewRequestBodyRef(body)
		parsed, err := ParseGatewayRequest(ref, "anthropic")
		require.NoError(t, err)
		parsed.Group = kiroGroup()
		parsed.SessionContext = &SessionContext{APIKeyID: apiKeyID}
		return svc.GenerateSessionHash(parsed)
	}

	hash1 := makeHash(1)
	hash2 := makeHash(2)

	require.NotEmpty(t, hash1)
	require.NotEmpty(t, hash2)
	require.NotEqual(t, hash1, hash2, "不同 API Key 应产生不同 hash，避免用户间粘性泄漏")
}

// TestKiroStickySession_NonKiroGroupFallsBackToMessageHash 验证：
// 非 Kiro 分组不走 kiro_system_prompt 路径，多轮对话 hash 会随消息变化（原有行为不变）。
func TestKiroStickySession_NonKiroGroupFallsBackToMessageHash(t *testing.T) {
	svc := &GatewayService{}
	systemPrompt := "You are a helpful assistant."
	ctx := &SessionContext{APIKeyID: 42}

	makeHash := func(turns int) string {
		body := newKiroRequestBody(systemPrompt, turns)
		ref := NewRequestBodyRef(body)
		parsed, err := ParseGatewayRequest(ref, "anthropic")
		require.NoError(t, err)
		parsed.Group = nonKiroGroup() // 非 Kiro 分组
		parsed.SessionContext = ctx
		return svc.GenerateSessionHash(parsed)
	}

	hash1 := makeHash(1)
	hash2 := makeHash(2) // messages 增加

	require.NotEmpty(t, hash1)
	require.NotEmpty(t, hash2)
	require.NotEqual(t, hash1, hash2, "非 Kiro 分组多轮对话 hash 应随消息内容变化")
}

// TestKiroStickySession_ExplicitSessionIDHeaderTakesPrecedence 验证：
// ExplicitSessionID（来自 X-Session-ID 请求头）优先级高于 system prompt hash，
// 且不同 system prompt 下相同 session id 仍返回相同 hash。
func TestKiroStickySession_ExplicitSessionIDHeaderTakesPrecedence(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{APIKeyID: 42}

	makeHash := func(systemPrompt, explicitID string) string {
		body := newKiroRequestBody(systemPrompt, 1)
		ref := NewRequestBodyRef(body)
		parsed, err := ParseGatewayRequest(ref, "anthropic")
		require.NoError(t, err)
		parsed.Group = kiroGroup()
		parsed.SessionContext = ctx
		parsed.ExplicitSessionID = explicitID
		return svc.GenerateSessionHash(parsed)
	}

	// 相同 session id，不同 system prompt → hash 相同（显式 ID 主导）
	hashA := makeHash("System prompt A", "my-session-123")
	hashB := makeHash("System prompt B", "my-session-123")
	require.NotEmpty(t, hashA)
	require.Equal(t, hashA, hashB, "相同 X-Session-ID 应产生相同 hash，无视 system prompt 差异")

	// 不同 session id → hash 不同
	hashC := makeHash("System prompt A", "other-session-456")
	require.NotEqual(t, hashA, hashC, "不同 X-Session-ID 应产生不同 hash")

	// 任意字符串 session id 也能工作（不要求特定格式）
	hashD := makeHash("System prompt A", "default")
	require.NotEmpty(t, hashD, "任意字符串 session id 都应有效")
}

// TestKiroStickySession_ExplicitSessionIDDifferentAPIKeys 验证：
// 相同 X-Session-ID 但不同 API Key，hash 不同（用户间隔离）。
func TestKiroStickySession_ExplicitSessionIDDifferentAPIKeys(t *testing.T) {
	svc := &GatewayService{}

	makeHash := func(apiKeyID int64) string {
		body := newKiroRequestBody("shared prompt", 1)
		ref := NewRequestBodyRef(body)
		parsed, err := ParseGatewayRequest(ref, "anthropic")
		require.NoError(t, err)
		parsed.Group = kiroGroup()
		parsed.SessionContext = &SessionContext{APIKeyID: apiKeyID}
		parsed.ExplicitSessionID = "default"
		return svc.GenerateSessionHash(parsed)
	}

	require.NotEqual(t, makeHash(1), makeHash(2), "不同 API Key 即使相同 session id 也应隔离")
}

// TestKiroStickySession_EmptySystemPromptUsesFirstUserMessage 验证：
// Kiro 分组没有 system prompt 时，使用第一条 user 消息作为稳定种子，避免多轮 messages 增长导致 hash 变化。
func TestKiroStickySession_EmptySystemPromptUsesFirstUserMessage(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{
		ClientIP: "1.2.3.4",
		APIKeyID: 42,
	}

	makeHash := func(firstUserMessage string, turns int) string {
		body := newKiroRequestBodyWithoutSystem(firstUserMessage, turns)
		ref := NewRequestBodyRef(body)
		parsed, err := ParseGatewayRequest(ref, "anthropic")
		require.NoError(t, err)
		parsed.Group = kiroGroup()
		parsed.SessionContext = ctx
		return svc.GenerateSessionHash(parsed)
	}

	hash1 := makeHash("hello", 1)
	hash2 := makeHash("hello", 2)
	hash3 := makeHash("different first prompt", 2)

	require.NotEmpty(t, hash1)
	require.Equal(t, hash1, hash2, "没有 system prompt 时，同一第一条 user 消息应保持 hash 稳定")
	require.NotEqual(t, hash1, hash3, "不同第一条 user 消息应产生不同 hash")
}

// TestIsKiroGroup 验证 isKiroGroup 辅助函数。
func TestKiroStickySession_DisabledSkipsAutoInference(t *testing.T) {
	svc := &GatewayService{}
	body := newKiroRequestBody("stable system prompt", 1)
	ref := NewRequestBodyRef(body)
	parsed, err := ParseGatewayRequest(ref, "anthropic")
	require.NoError(t, err)
	parsed.Group = &Group{Platform: PlatformKiro, KiroAutoStickyEnabled: false}
	parsed.SessionContext = &SessionContext{APIKeyID: 42}

	require.Empty(t, svc.GenerateSessionHash(parsed))
}

func TestKiroStickySession_ExplicitSessionIDWorksWhenAutoInferenceDisabled(t *testing.T) {
	svc := &GatewayService{}
	body := newKiroRequestBody("stable system prompt", 1)
	ref := NewRequestBodyRef(body)
	parsed, err := ParseGatewayRequest(ref, "anthropic")
	require.NoError(t, err)
	parsed.Group = &Group{Platform: PlatformKiro, KiroAutoStickyEnabled: false}
	parsed.SessionContext = &SessionContext{APIKeyID: 42}
	parsed.ExplicitSessionID = "manual-session"

	require.NotEmpty(t, svc.GenerateSessionHash(parsed))
}

func TestKiroStickySession_BodySessionIDTakesPrecedence(t *testing.T) {
	svc := &GatewayService{}
	ctx := &SessionContext{APIKeyID: 42}

	makeHash := func(systemPrompt, conversationID string) string {
		body := newKiroRequestBodyWithBodySession(systemPrompt, "conversation_id", conversationID)
		ref := NewRequestBodyRef(body)
		parsed, err := ParseGatewayRequest(ref, "anthropic")
		require.NoError(t, err)
		parsed.Group = kiroGroup()
		parsed.SessionContext = ctx
		return svc.GenerateSessionHash(parsed)
	}

	hashA := makeHash("System prompt A", "conv-stable")
	hashB := makeHash("System prompt B", "conv-stable")
	hashC := makeHash("System prompt A", "conv-other")

	require.NotEmpty(t, hashA)
	require.Equal(t, hashA, hashB, "body conversation_id should override system prompt differences")
	require.NotEqual(t, hashA, hashC, "different body conversation_id values should route independently")
}

func TestKiroStickySession_BodySessionIDDifferentAPIKeys(t *testing.T) {
	svc := &GatewayService{}

	makeHash := func(apiKeyID int64) string {
		body := newKiroRequestBodyWithBodySession("stable system prompt", "session_id", "shared-session")
		ref := NewRequestBodyRef(body)
		parsed, err := ParseGatewayRequest(ref, "anthropic")
		require.NoError(t, err)
		parsed.Group = kiroGroup()
		parsed.SessionContext = &SessionContext{APIKeyID: apiKeyID}
		return svc.GenerateSessionHash(parsed)
	}

	require.NotEqual(t, makeHash(1), makeHash(2), "body session_id should be isolated per API key")
}

func TestKiroStickySession_BodySessionIDWorksWhenAutoInferenceDisabled(t *testing.T) {
	svc := &GatewayService{}
	body := newKiroRequestBodyWithBodySession("stable system prompt", "thread_id", "thread-123")
	ref := NewRequestBodyRef(body)
	parsed, err := ParseGatewayRequest(ref, "anthropic")
	require.NoError(t, err)
	parsed.Group = &Group{Platform: PlatformKiro, KiroAutoStickyEnabled: false}
	parsed.SessionContext = &SessionContext{APIKeyID: 42}

	require.NotEmpty(t, svc.GenerateSessionHash(parsed))
}

func TestExtractBodySessionID(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "prompt cache key", body: `{"prompt_cache_key":"pcache-1","conversation_id":"conv-1"}`, want: "pcache-1"},
		{name: "conversation id", body: `{"conversation_id":"conv-1"}`, want: "conv-1"},
		{name: "camel session id", body: `{"sessionId":"sess-1"}`, want: "sess-1"},
		{name: "metadata session id", body: `{"metadata":{"session_id":"meta-sess-1"}}`, want: "meta-sess-1"},
		{name: "blank ignored", body: `{"session_id":"   ","thread_id":"thread-1"}`, want: "thread-1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, extractBodySessionID(tc.body))
		})
	}
}

func TestIsKiroGroup(t *testing.T) {
	require.True(t, isKiroGroup(&Group{Platform: PlatformKiro}))
	require.False(t, isKiroGroup(&Group{Platform: PlatformAnthropic}))
	require.False(t, isKiroGroup(&Group{Platform: PlatformOpenAI}))
	require.False(t, isKiroGroup(nil))
}
