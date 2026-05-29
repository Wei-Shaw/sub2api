//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ============ 基础优先级测试 ============

func TestGenerateSessionHash_NilParsedRequest(t *testing.T) {
	svc := &GatewayService{REDACTED
	require.Empty(t, svc.GenerateSessionHash(nil))
REDACTED

func TestGenerateSessionHash_EmptyRequest(t *testing.T) {
	svc := &GatewayService{REDACTED
	require.Empty(t, svc.GenerateSessionHash(&ParsedRequest{REDACTED))
REDACTED

func TestGenerateSessionHash_MetadataHasHighestPriority(t *testing.T) {
	svc := &GatewayService{REDACTED

	parsed := &ParsedRequest{
		MetadataUserID: "user_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2_account__session_123e4567-e89b-12d3-a456-426614174000",
		System:         "You are a helpful assistant.",
		HasSystem:      true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED

	hash := svc.GenerateSessionHash(parsed)
	require.Equal(t, "123e4567-e89b-12d3-a456-426614174000", hash, "metadata session_id should have highest priority")
REDACTED

// ============ System + Messages 基础测试 ============

func TestGenerateSessionHash_SystemPlusMessages(t *testing.T) {
	svc := &GatewayService{REDACTED

	withSystem := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED
	withoutSystem := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(withSystem)
	h2 := svc.GenerateSessionHash(withoutSystem)
	require.NotEmpty(t, h1)
	require.NotEmpty(t, h2)
	require.NotEqual(t, h1, h2, "system prompt should be part of digest, producing different hash")
REDACTED

func TestGenerateSessionHash_SystemOnlyProducesHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	parsed := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
REDACTED
	hash := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, hash, "system prompt alone should produce a hash as part of full digest")
REDACTED

func TestGenerateSessionHash_DifferentSystemsSameMessages(t *testing.T) {
	svc := &GatewayService{REDACTED

	parsed1 := &ParsedRequest{
		System:    "You are assistant A.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED
	parsed2 := &ParsedRequest{
		System:    "You are assistant B.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(parsed1)
	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEqual(t, h1, h2, "different system prompts with same messages should produce different hashes")
REDACTED

func TestGenerateSessionHash_SameSystemSameMessages(t *testing.T) {
	svc := &GatewayService{REDACTED

	mk := func() *ParsedRequest {
		return &ParsedRequest{
			System:    "You are a helpful assistant.",
			HasSystem: true,
			Messages: []any{
				map[string]any{"role": "user", "content": "hello"REDACTED,
				map[string]any{"role": "assistant", "content": "hi"REDACTED,
		REDACTED,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(mk())
	h2 := svc.GenerateSessionHash(mk())
	require.Equal(t, h1, h2, "same system + same messages should produce identical hash")
REDACTED

func TestGenerateSessionHash_DifferentMessagesProduceDifferentHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	parsed1 := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "help me with Go"REDACTED,
	REDACTED,
REDACTED
	parsed2 := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "help me with Python"REDACTED,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(parsed1)
	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEqual(t, h1, h2, "same system but different messages should produce different hashes")
REDACTED

// ============ SessionContext 核心测试 ============

func TestGenerateSessionHash_DifferentSessionContextProducesDifferentHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	// 相同消息 + 不同 SessionContext → 不同 hash（解决碰撞问题的核心场景）
	parsed1 := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "192.168.1.1",
			UserAgent: "Mozilla/5.0",
			APIKeyID:  100,
	REDACTED,
REDACTED
	parsed2 := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "10.0.0.1",
			UserAgent: "curl/7.0",
			APIKeyID:  200,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(parsed1)
	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEmpty(t, h1)
	require.NotEmpty(t, h2)
	require.NotEqual(t, h1, h2, "same messages but different SessionContext should produce different hashes")
REDACTED

func TestGenerateSessionHash_SameSessionContextProducesSameHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	mk := func() *ParsedRequest {
		return &ParsedRequest{
			Messages: []any{
				map[string]any{"role": "user", "content": "hello"REDACTED,
		REDACTED,
			SessionContext: &SessionContext{
				ClientIP:  "192.168.1.1",
				UserAgent: "Mozilla/5.0",
				APIKeyID:  100,
		REDACTED,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(mk())
	h2 := svc.GenerateSessionHash(mk())
	require.Equal(t, h1, h2, "same messages + same SessionContext should produce identical hash")
REDACTED

func TestGenerateSessionHash_MetadataOverridesSessionContext(t *testing.T) {
	svc := &GatewayService{REDACTED

	parsed := &ParsedRequest{
		MetadataUserID: "user_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2_account__session_123e4567-e89b-12d3-a456-426614174000",
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "192.168.1.1",
			UserAgent: "Mozilla/5.0",
			APIKeyID:  100,
	REDACTED,
REDACTED

	hash := svc.GenerateSessionHash(parsed)
	require.Equal(t, "123e4567-e89b-12d3-a456-426614174000", hash,
		"metadata session_id should take priority over SessionContext")
REDACTED

func TestGenerateSessionHash_MetadataJSON_HasHighestPriority(t *testing.T) {
	svc := &GatewayService{REDACTED

	parsed := &ParsedRequest{
		MetadataUserID: `{"device_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2","account_uuid":"","session_id":"c72554f2-1234-5678-abcd-123456789abc"REDACTED`,
		System:         "You are a helpful assistant.",
		HasSystem:      true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED

	hash := svc.GenerateSessionHash(parsed)
	require.Equal(t, "c72554f2-1234-5678-abcd-123456789abc", hash, "JSON format metadata session_id should have highest priority")
REDACTED

func TestGenerateSessionHash_NilSessionContextBackwardCompatible(t *testing.T) {
	svc := &GatewayService{REDACTED

	withCtx := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: nil,
REDACTED
	withoutCtx := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(withCtx)
	h2 := svc.GenerateSessionHash(withoutCtx)
	require.Equal(t, h1, h2, "nil SessionContext should produce same hash as no SessionContext")
REDACTED

// ============ 多轮连续会话测试 ============

func TestGenerateSessionHash_ContinuousConversation_HashChangesWithMessages(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1REDACTED

	// 模拟连续会话：每增加一轮对话，hash 应该不同（内容累积变化）
	round1 := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	round2 := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
			map[string]any{"role": "assistant", "content": "Hi there!"REDACTED,
			map[string]any{"role": "user", "content": "How are you?"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	round3 := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
			map[string]any{"role": "assistant", "content": "Hi there!"REDACTED,
			map[string]any{"role": "user", "content": "How are you?"REDACTED,
			map[string]any{"role": "assistant", "content": "I'm doing well!"REDACTED,
			map[string]any{"role": "user", "content": "Tell me a joke"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h1 := svc.GenerateSessionHash(round1)
	h2 := svc.GenerateSessionHash(round2)
	h3 := svc.GenerateSessionHash(round3)

	require.NotEmpty(t, h1)
	require.NotEmpty(t, h2)
	require.NotEmpty(t, h3)
	require.NotEqual(t, h1, h2, "different conversation rounds should produce different hashes")
	require.NotEqual(t, h2, h3, "each new round should produce a different hash")
	require.NotEqual(t, h1, h3, "round 1 and round 3 should differ")
REDACTED

func TestGenerateSessionHash_ContinuousConversation_SameRoundSameHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1REDACTED

	// 同一轮对话重复请求（如重试）应产生相同 hash
	mk := func() *ParsedRequest {
		return &ParsedRequest{
			System:    "You are a helpful assistant.",
			HasSystem: true,
			Messages: []any{
				map[string]any{"role": "user", "content": "hello"REDACTED,
				map[string]any{"role": "assistant", "content": "Hi there!"REDACTED,
				map[string]any{"role": "user", "content": "How are you?"REDACTED,
		REDACTED,
			SessionContext: ctx,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(mk())
	h2 := svc.GenerateSessionHash(mk())
	require.Equal(t, h1, h2, "same conversation state should produce identical hash on retry")
REDACTED

// ============ 消息回退测试 ============

func TestGenerateSessionHash_MessageRollback(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1REDACTED

	// 模拟消息回退：用户删掉最后一轮再重发
	original := &ParsedRequest{
		System:    "System prompt",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "msg1"REDACTED,
			map[string]any{"role": "assistant", "content": "reply1"REDACTED,
			map[string]any{"role": "user", "content": "msg2"REDACTED,
			map[string]any{"role": "assistant", "content": "reply2"REDACTED,
			map[string]any{"role": "user", "content": "msg3"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	// 回退到 msg2 后，用新的 msg3 替代
	rollback := &ParsedRequest{
		System:    "System prompt",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "msg1"REDACTED,
			map[string]any{"role": "assistant", "content": "reply1"REDACTED,
			map[string]any{"role": "user", "content": "msg2"REDACTED,
			map[string]any{"role": "assistant", "content": "reply2"REDACTED,
			map[string]any{"role": "user", "content": "different msg3"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	hOrig := svc.GenerateSessionHash(original)
	hRollback := svc.GenerateSessionHash(rollback)
	require.NotEqual(t, hOrig, hRollback, "rollback with different last message should produce different hash")
REDACTED

func TestGenerateSessionHash_MessageRollbackSameContent(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1REDACTED

	// 回退后重新发送相同内容 → 相同 hash（合理的粘性恢复）
	mk := func() *ParsedRequest {
		return &ParsedRequest{
			System:    "System prompt",
			HasSystem: true,
			Messages: []any{
				map[string]any{"role": "user", "content": "msg1"REDACTED,
				map[string]any{"role": "assistant", "content": "reply1"REDACTED,
				map[string]any{"role": "user", "content": "msg2"REDACTED,
		REDACTED,
			SessionContext: ctx,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(mk())
	h2 := svc.GenerateSessionHash(mk())
	require.Equal(t, h1, h2, "rollback and resend same content should produce same hash")
REDACTED

// ============ 相同 System、不同用户消息 ============

func TestGenerateSessionHash_SameSystemDifferentUsers(t *testing.T) {
	svc := &GatewayService{REDACTED

	// 两个不同用户使用相同 system prompt 但发送不同消息
	user1 := &ParsedRequest{
		System:    "You are a code reviewer.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "Review this Go code"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "1.1.1.1",
			UserAgent: "vscode",
			APIKeyID:  1,
	REDACTED,
REDACTED
	user2 := &ParsedRequest{
		System:    "You are a code reviewer.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "Review this Python code"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "2.2.2.2",
			UserAgent: "vscode",
			APIKeyID:  2,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(user1)
	h2 := svc.GenerateSessionHash(user2)
	require.NotEqual(t, h1, h2, "different users with different messages should get different hashes")
REDACTED

func TestGenerateSessionHash_SameSystemSameMessageDifferentContext(t *testing.T) {
	svc := &GatewayService{REDACTED

	// 这是修复的核心场景：两个不同用户发送完全相同的 system + messages（如 "hello"）
	// 有了 SessionContext 后应该产生不同 hash
	user1 := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "1.1.1.1",
			UserAgent: "Mozilla/5.0",
			APIKeyID:  10,
	REDACTED,
REDACTED
	user2 := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "2.2.2.2",
			UserAgent: "Mozilla/5.0",
			APIKeyID:  20,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(user1)
	h2 := svc.GenerateSessionHash(user2)
	require.NotEqual(t, h1, h2, "CRITICAL: same system+messages but different users should get different hashes")
REDACTED

// ============ SessionContext 各字段独立影响测试 ============

func TestGenerateSessionHash_SessionContext_IPDifference(t *testing.T) {
	svc := &GatewayService{REDACTED

	base := func(ip string) *ParsedRequest {
		return &ParsedRequest{
			Messages: []any{
				map[string]any{"role": "user", "content": "test"REDACTED,
		REDACTED,
			SessionContext: &SessionContext{
				ClientIP:  ip,
				UserAgent: "same-ua",
				APIKeyID:  1,
		REDACTED,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(base("1.1.1.1"))
	h2 := svc.GenerateSessionHash(base("2.2.2.2"))
	require.NotEqual(t, h1, h2, "different IP should produce different hash")
REDACTED

func TestGenerateSessionHash_SessionContext_UADifference(t *testing.T) {
	svc := &GatewayService{REDACTED

	base := func(ua string) *ParsedRequest {
		return &ParsedRequest{
			Messages: []any{
				map[string]any{"role": "user", "content": "test"REDACTED,
		REDACTED,
			SessionContext: &SessionContext{
				ClientIP:  "1.1.1.1",
				UserAgent: ua,
				APIKeyID:  1,
		REDACTED,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(base("Mozilla/5.0"))
	h2 := svc.GenerateSessionHash(base("curl/7.0"))
	require.NotEqual(t, h1, h2, "different User-Agent should produce different hash")
REDACTED

func TestGenerateSessionHash_SessionContext_UAVersionNoiseIgnored(t *testing.T) {
	svc := &GatewayService{REDACTED

	base := func(ua string) *ParsedRequest {
		return &ParsedRequest{
			Messages: []any{
				map[string]any{"role": "user", "content": "test"REDACTED,
		REDACTED,
			SessionContext: &SessionContext{
				ClientIP:  "1.1.1.1",
				UserAgent: ua,
				APIKeyID:  1,
		REDACTED,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(base("Mozilla/5.0 codex_cli_rs/0.1.0"))
	h2 := svc.GenerateSessionHash(base("Mozilla/5.0 codex_cli_rs/0.1.1"))
	require.Equal(t, h1, h2, "version-only User-Agent changes should not perturb the sticky session hash")
REDACTED

func TestGenerateSessionHash_SessionContext_FreeformUAVersionNoiseIgnored(t *testing.T) {
	svc := &GatewayService{REDACTED

	base := func(ua string) *ParsedRequest {
		return &ParsedRequest{
			Messages: []any{
				map[string]any{"role": "user", "content": "test"REDACTED,
		REDACTED,
			SessionContext: &SessionContext{
				ClientIP:  "1.1.1.1",
				UserAgent: ua,
				APIKeyID:  1,
		REDACTED,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(base("Codex CLI 0.1.0"))
	h2 := svc.GenerateSessionHash(base("Codex CLI 0.1.1"))
	require.Equal(t, h1, h2, "free-form version-only User-Agent changes should not perturb the sticky session hash")
REDACTED

func TestGenerateSessionHash_SessionContext_APIKeyIDDifference(t *testing.T) {
	svc := &GatewayService{REDACTED

	base := func(keyID int64) *ParsedRequest {
		return &ParsedRequest{
			Messages: []any{
				map[string]any{"role": "user", "content": "test"REDACTED,
		REDACTED,
			SessionContext: &SessionContext{
				ClientIP:  "1.1.1.1",
				UserAgent: "same-ua",
				APIKeyID:  keyID,
		REDACTED,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(base(1))
	h2 := svc.GenerateSessionHash(base(2))
	require.NotEqual(t, h1, h2, "different APIKeyID should produce different hash")
REDACTED

// ============ 多用户并发相同消息场景 ============

func TestGenerateSessionHash_MultipleUsersSameFirstMessage(t *testing.T) {
	svc := &GatewayService{REDACTED

	// 模拟 5 个不同用户同时发送 "hello" → 应该产生 5 个不同的 hash
	hashes := make(map[string]bool)
	for i := 0; i < 5; i++ {
		parsed := &ParsedRequest{
			Messages: []any{
				map[string]any{"role": "user", "content": "hello"REDACTED,
		REDACTED,
			SessionContext: &SessionContext{
				ClientIP:  "192.168.1." + string(rune('1'+i)),
				UserAgent: "client-" + string(rune('A'+i)),
				APIKeyID:  int64(i + 1),
		REDACTED,
	REDACTED
		h := svc.GenerateSessionHash(parsed)
		require.NotEmpty(t, h)
		require.False(t, hashes[h], "hash collision detected for user %d", i)
		hashes[h] = true
REDACTED
	require.Len(t, hashes, 5, "5 different users should produce 5 unique hashes")
REDACTED

// ============ 连续会话粘性：多轮对话同一用户 ============

func TestGenerateSessionHash_SameUserGrowingConversation(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "browser", APIKeyID: 42REDACTED

	// 模拟同一用户的连续会话，每轮 hash 不同但同用户重试保持一致
	messages := []map[string]any{
		{"role": "user", "content": "msg1"REDACTED,
		{"role": "assistant", "content": "reply1"REDACTED,
		{"role": "user", "content": "msg2"REDACTED,
		{"role": "assistant", "content": "reply2"REDACTED,
		{"role": "user", "content": "msg3"REDACTED,
		{"role": "assistant", "content": "reply3"REDACTED,
		{"role": "user", "content": "msg4"REDACTED,
REDACTED

	prevHash := ""
	for round := 1; round <= len(messages); round += 2 {
		// 构建前 round 条消息
		msgs := make([]any, round)
		for j := 0; j < round; j++ {
			msgs[j] = messages[j]
	REDACTED
		parsed := &ParsedRequest{
			System:         "System",
			HasSystem:      true,
			Messages:       msgs,
			SessionContext: ctx,
	REDACTED
		h := svc.GenerateSessionHash(parsed)
		require.NotEmpty(t, h, "round %d hash should not be empty", round)

		if prevHash != "" {
			require.NotEqual(t, prevHash, h, "round %d hash should differ from previous round", round)
	REDACTED
		prevHash = h

		// 同一轮重试应该相同
		h2 := svc.GenerateSessionHash(parsed)
		require.Equal(t, h, h2, "retry of round %d should produce same hash", round)
REDACTED
REDACTED

// ============ 多轮消息内容结构化测试 ============

func TestGenerateSessionHash_MultipleUserMessages(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1REDACTED

	// 5 条用户消息（无 assistant 回复）
	parsed := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "first"REDACTED,
			map[string]any{"role": "user", "content": "second"REDACTED,
			map[string]any{"role": "user", "content": "third"REDACTED,
			map[string]any{"role": "user", "content": "fourth"REDACTED,
			map[string]any{"role": "user", "content": "fifth"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, h)

	// 修改中间一条消息应该改变 hash
	parsed2 := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "first"REDACTED,
			map[string]any{"role": "user", "content": "CHANGED"REDACTED,
			map[string]any{"role": "user", "content": "third"REDACTED,
			map[string]any{"role": "user", "content": "fourth"REDACTED,
			map[string]any{"role": "user", "content": "fifth"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEqual(t, h, h2, "changing any message should change the hash")
REDACTED

func TestGenerateSessionHash_MessageOrderMatters(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1REDACTED

	parsed1 := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "alpha"REDACTED,
			map[string]any{"role": "user", "content": "beta"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED
	parsed2 := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "beta"REDACTED,
			map[string]any{"role": "user", "content": "alpha"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h1 := svc.GenerateSessionHash(parsed1)
	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEqual(t, h1, h2, "message order should affect the hash")
REDACTED

// ============ 复杂内容格式测试 ============

func TestGenerateSessionHash_StructuredContent(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1REDACTED

	// 结构化 content（数组形式）
	parsed := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "Look at this"REDACTED,
					map[string]any{"type": "text", "text": "And this too"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, h, "structured content should produce a hash")
REDACTED

func TestGenerateSessionHash_ArraySystemPrompt(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1REDACTED

	// 数组格式的 system prompt
	parsed := &ParsedRequest{
		System: []any{
			map[string]any{"type": "text", "text": "You are a helpful assistant."REDACTED,
			map[string]any{"type": "text", "text": "Be concise."REDACTED,
	REDACTED,
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, h, "array system prompt should produce a hash")
REDACTED

// ============ SessionContext 与 cache_control 优先级 ============

func TestGenerateSessionHash_CacheControlOverridesSessionContext(t *testing.T) {
	svc := &GatewayService{REDACTED

	// 当有 cache_control: ephemeral 时，使用第 2 级优先级
	// SessionContext 不应影响结果
	parsed1 := &ParsedRequest{
		System: []any{
			map[string]any{
				"type":          "text",
				"text":          "You are a tool-specific assistant.",
				"cache_control": map[string]any{"type": "ephemeral"REDACTED,
		REDACTED,
	REDACTED,
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "1.1.1.1",
			UserAgent: "ua1",
			APIKeyID:  100,
	REDACTED,
REDACTED
	parsed2 := &ParsedRequest{
		System: []any{
			map[string]any{
				"type":          "text",
				"text":          "You are a tool-specific assistant.",
				"cache_control": map[string]any{"type": "ephemeral"REDACTED,
		REDACTED,
	REDACTED,
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "2.2.2.2",
			UserAgent: "ua2",
			APIKeyID:  200,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(parsed1)
	h2 := svc.GenerateSessionHash(parsed2)
	require.Equal(t, h1, h2, "cache_control ephemeral has higher priority, SessionContext should not affect result")
REDACTED

// ============ 边界情况 ============

func TestGenerateSessionHash_EmptyMessages(t *testing.T) {
	svc := &GatewayService{REDACTED

	parsed := &ParsedRequest{
		Messages: []any{REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "1.1.1.1",
			UserAgent: "test",
			APIKeyID:  1,
	REDACTED,
REDACTED

	// 空 messages + 只有 SessionContext 时，combined.Len() > 0 因为有 context 写入
	h := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, h, "empty messages with SessionContext should still produce a hash from context")
REDACTED

func TestGenerateSessionHash_EmptyMessagesNoContext(t *testing.T) {
	svc := &GatewayService{REDACTED

	parsed := &ParsedRequest{
		Messages: []any{REDACTED,
REDACTED

	h := svc.GenerateSessionHash(parsed)
	require.Empty(t, h, "empty messages without SessionContext should produce empty hash")
REDACTED

func TestGenerateSessionHash_SessionContextWithEmptyFields(t *testing.T) {
	svc := &GatewayService{REDACTED

	// SessionContext 字段为空字符串和零值时仍应影响 hash
	withEmptyCtx := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "test"REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "",
			UserAgent: "",
			APIKeyID:  0,
	REDACTED,
REDACTED
	withoutCtx := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "test"REDACTED,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(withEmptyCtx)
	h2 := svc.GenerateSessionHash(withoutCtx)
	// 有 SessionContext（即使字段为空）仍然会写入分隔符 "::" 等
	require.NotEqual(t, h1, h2, "empty-field SessionContext should still differ from nil SessionContext")
REDACTED

// ============ 长对话历史测试 ============

func TestGenerateSessionHash_LongConversation(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "test", APIKeyID: 1REDACTED

	// 构建 20 轮对话
	messages := make([]any, 0, 40)
	for i := 0; i < 20; i++ {
		messages = append(messages, map[string]any{
			"role":    "user",
			"content": "user message " + string(rune('A'+i)),
	REDACTED)
		messages = append(messages, map[string]any{
			"role":    "assistant",
			"content": "assistant reply " + string(rune('A'+i)),
	REDACTED)
REDACTED

	parsed := &ParsedRequest{
		System:         "System prompt",
		HasSystem:      true,
		Messages:       messages,
		SessionContext: ctx,
REDACTED

	h := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, h)

	// 再加一轮应该不同
	moreMessages := make([]any, len(messages)+2)
	copy(moreMessages, messages)
	moreMessages[len(messages)] = map[string]any{"role": "user", "content": "one more"REDACTED
	moreMessages[len(messages)+1] = map[string]any{"role": "assistant", "content": "ok"REDACTED

	parsed2 := &ParsedRequest{
		System:         "System prompt",
		HasSystem:      true,
		Messages:       moreMessages,
		SessionContext: ctx,
REDACTED

	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEqual(t, h, h2, "adding more messages to long conversation should change hash")
REDACTED

// ============ Gemini 原生格式 session hash 测试 ============

func TestGenerateSessionHash_GeminiContentsProducesHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	// Gemini 格式: contents[].parts[].text
	parsed := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Hello from Gemini"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "1.2.3.4",
			UserAgent: "gemini-cli",
			APIKeyID:  1,
	REDACTED,
REDACTED

	h := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, h, "Gemini contents with parts should produce a non-empty hash")
REDACTED

func TestGenerateSessionHash_GeminiDifferentContentsDifferentHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "gemini-cli", APIKeyID: 1REDACTED

	parsed1 := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Hello"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED
	parsed2 := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Goodbye"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h1 := svc.GenerateSessionHash(parsed1)
	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEqual(t, h1, h2, "different Gemini contents should produce different hashes")
REDACTED

func TestGenerateSessionHash_GeminiSameContentsSameHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "gemini-cli", APIKeyID: 1REDACTED

	mk := func() *ParsedRequest {
		return &ParsedRequest{
			Messages: []any{
				map[string]any{
					"role": "user",
					"parts": []any{
						map[string]any{"text": "Hello"REDACTED,
				REDACTED,
			REDACTED,
				map[string]any{
					"role": "model",
					"parts": []any{
						map[string]any{"text": "Hi there!"REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			SessionContext: ctx,
	REDACTED
REDACTED

	h1 := svc.GenerateSessionHash(mk())
	h2 := svc.GenerateSessionHash(mk())
	require.Equal(t, h1, h2, "same Gemini contents should produce identical hash")
REDACTED

func TestGenerateSessionHash_GeminiMultiTurnHashChanges(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "gemini-cli", APIKeyID: 1REDACTED

	round1 := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "hello"REDACTEDREDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	round2 := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "hello"REDACTEDREDACTED,
		REDACTED,
			map[string]any{
				"role":  "model",
				"parts": []any{map[string]any{"text": "Hi!"REDACTEDREDACTED,
		REDACTED,
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "How are you?"REDACTEDREDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h1 := svc.GenerateSessionHash(round1)
	h2 := svc.GenerateSessionHash(round2)
	require.NotEmpty(t, h1)
	require.NotEmpty(t, h2)
	require.NotEqual(t, h1, h2, "Gemini multi-turn should produce different hashes per round")
REDACTED

func TestGenerateSessionHash_GeminiDifferentUsersSameContentDifferentHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	// 核心场景：两个不同用户发送相同 Gemini 格式消息应得到不同 hash
	user1 := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "hello"REDACTEDREDACTED,
		REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "1.1.1.1",
			UserAgent: "gemini-cli",
			APIKeyID:  10,
	REDACTED,
REDACTED
	user2 := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "hello"REDACTEDREDACTED,
		REDACTED,
	REDACTED,
		SessionContext: &SessionContext{
			ClientIP:  "2.2.2.2",
			UserAgent: "gemini-cli",
			APIKeyID:  20,
	REDACTED,
REDACTED

	h1 := svc.GenerateSessionHash(user1)
	h2 := svc.GenerateSessionHash(user2)
	require.NotEqual(t, h1, h2, "CRITICAL: different Gemini users with same content must get different hashes")
REDACTED

func TestGenerateSessionHash_GeminiSystemInstructionAffectsHash(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "gemini-cli", APIKeyID: 1REDACTED

	// systemInstruction 经 ParseGatewayRequest 解析后存入 parsed.System
	withSys := &ParsedRequest{
		System: []any{
			map[string]any{"text": "You are a coding assistant."REDACTED,
	REDACTED,
		Messages: []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "hello"REDACTEDREDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED
	withoutSys := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "hello"REDACTEDREDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h1 := svc.GenerateSessionHash(withSys)
	h2 := svc.GenerateSessionHash(withoutSys)
	require.NotEqual(t, h1, h2, "systemInstruction should affect the hash")
REDACTED

func TestGenerateSessionHash_GeminiMultiPartMessage(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "gemini-cli", APIKeyID: 1REDACTED

	// 多 parts 的消息
	parsed := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Part 1"REDACTED,
					map[string]any{"text": "Part 2"REDACTED,
					map[string]any{"text": "Part 3"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, h, "multi-part Gemini message should produce a hash")

	// 不同内容的多 parts
	parsed2 := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Part 1"REDACTED,
					map[string]any{"text": "CHANGED"REDACTED,
					map[string]any{"text": "Part 3"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEqual(t, h, h2, "changing a part should change the hash")
REDACTED

func TestGenerateSessionHash_GeminiNonTextPartsIgnored(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "gemini-cli", APIKeyID: 1REDACTED

	// 含非 text 类型 parts（如 inline_data），应被跳过但不报错
	parsed := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Describe this image"REDACTED,
					map[string]any{"inline_data": map[string]any{"mime_type": "image/png", "data": "base64..."REDACTEDREDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		SessionContext: ctx,
REDACTED

	h := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, h, "Gemini message with mixed parts should still produce a hash from text parts")
REDACTED

func TestGenerateSessionHash_GeminiMultiTurnHashNotSticky(t *testing.T) {
	svc := &GatewayService{REDACTED

	ctx := &SessionContext{ClientIP: "10.0.0.1", UserAgent: "gemini-cli", APIKeyID: 42REDACTED

	// 模拟同一 Gemini 会话的三轮请求，每轮 contents 累积增长。
	// 验证预期行为：每轮 hash 都不同，即 GenerateSessionHash 不具备跨轮粘性。
	// 这是 by-design 的——Gemini 的跨轮粘性由 Digest Fallback（BuildGeminiDigestChain）负责。
	round1Body := []byte(`{
		"systemInstruction": {"parts": [{"text": "You are a coding assistant."REDACTED]REDACTED,
		"contents": [
			{"role": "user", "parts": [{"text": "Write a Go function"REDACTED]REDACTED
		]
REDACTED`)
	round2Body := []byte(`{
		"systemInstruction": {"parts": [{"text": "You are a coding assistant."REDACTED]REDACTED,
		"contents": [
			{"role": "user", "parts": [{"text": "Write a Go function"REDACTED]REDACTED,
			{"role": "model", "parts": [{"text": "func hello() {REDACTED"REDACTED]REDACTED,
			{"role": "user", "parts": [{"text": "Add error handling"REDACTED]REDACTED
		]
REDACTED`)
	round3Body := []byte(`{
		"systemInstruction": {"parts": [{"text": "You are a coding assistant."REDACTED]REDACTED,
		"contents": [
			{"role": "user", "parts": [{"text": "Write a Go function"REDACTED]REDACTED,
			{"role": "model", "parts": [{"text": "func hello() {REDACTED"REDACTED]REDACTED,
			{"role": "user", "parts": [{"text": "Add error handling"REDACTED]REDACTED,
			{"role": "model", "parts": [{"text": "func hello() error { return nil REDACTED"REDACTED]REDACTED,
			{"role": "user", "parts": [{"text": "Now add tests"REDACTED]REDACTED
		]
REDACTED`)

	hashes := make([]string, 3)
	for i, body := range [][]byte{round1Body, round2Body, round3BodyREDACTED {
		parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "gemini")
	REDACTED
		parsed.SessionContext = ctx
		hashes[i] = svc.GenerateSessionHash(parsed)
		require.NotEmpty(t, hashes[i], "round %d hash should not be empty", i+1)
REDACTED

	// 每轮 hash 都不同——这是预期行为
	require.NotEqual(t, hashes[0], hashes[1], "round 1 vs 2 hash should differ (contents grow)")
	require.NotEqual(t, hashes[1], hashes[2], "round 2 vs 3 hash should differ (contents grow)")
	require.NotEqual(t, hashes[0], hashes[2], "round 1 vs 3 hash should differ")

	// 同一轮重试应产生相同 hash
	parsed1Again, err := ParseGatewayRequest(NewRequestBodyRef(round2Body), "gemini")
REDACTED
	parsed1Again.SessionContext = ctx
	h2Again := svc.GenerateSessionHash(parsed1Again)
	require.Equal(t, hashes[1], h2Again, "retry of same round should produce same hash")
REDACTED

func TestGenerateSessionHash_GeminiEndToEnd(t *testing.T) {
	svc := &GatewayService{REDACTED

	// 端到端测试：模拟 ParseGatewayRequest + GenerateSessionHash 完整流程
	body := []byte(`{
		"model": "gemini-2.5-pro",
		"systemInstruction": {
			"parts": [{"text": "You are a coding assistant."REDACTED]
	REDACTED,
		"contents": [
			{"role": "user", "parts": [{"text": "Write a Go function"REDACTED]REDACTED,
			{"role": "model", "parts": [{"text": "Here is a function..."REDACTED]REDACTED,
			{"role": "user", "parts": [{"text": "Now add error handling"REDACTED]REDACTED
		]
REDACTED`)

	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "gemini")
REDACTED
	parsed.SessionContext = &SessionContext{
		ClientIP:  "10.0.0.1",
		UserAgent: "gemini-cli/1.0",
		APIKeyID:  42,
REDACTED

	h := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, h, "end-to-end Gemini flow should produce a hash")

	// 同一请求再次解析应产生相同 hash
	parsed2, err := ParseGatewayRequest(NewRequestBodyRef(body), "gemini")
REDACTED
	parsed2.SessionContext = &SessionContext{
		ClientIP:  "10.0.0.1",
		UserAgent: "gemini-cli/1.0",
		APIKeyID:  42,
REDACTED

	h2 := svc.GenerateSessionHash(parsed2)
	require.Equal(t, h, h2, "same request should produce same hash")

	// 不同用户发送相同请求应产生不同 hash
	parsed3, err := ParseGatewayRequest(NewRequestBodyRef(body), "gemini")
REDACTED
	parsed3.SessionContext = &SessionContext{
		ClientIP:  "10.0.0.2",
		UserAgent: "gemini-cli/1.0",
		APIKeyID:  99,
REDACTED

	h3 := svc.GenerateSessionHash(parsed3)
	require.NotEqual(t, h, h3, "different user with same Gemini request should get different hash")
REDACTED
