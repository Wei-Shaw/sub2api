package service

import (
	"strings"
	"testing"
)

func TestBuildAnthropicDigestChain_NilRequest(t *testing.T) {
	result := BuildAnthropicDigestChain(nil)
	if result != "" {
		t.Errorf("expected empty string for nil request, got: %s", result)
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_EmptyMessages(t *testing.T) {
	parsed := &ParsedRequest{
		Messages: []any{REDACTED,
REDACTED
	result := BuildAnthropicDigestChain(parsed)
	if result != "" {
		t.Errorf("expected empty string for empty messages, got: %s", result)
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_SingleUserMessage(t *testing.T) {
	parsed := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d: %s", len(parts), result)
REDACTED
	if !strings.HasPrefix(parts[0], "u:") {
		t.Errorf("expected prefix 'u:', got: %s", parts[0])
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_UserAndAssistant(t *testing.T) {
	parsed := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
			map[string]any{"role": "assistant", "content": "hi there"REDACTED,
	REDACTED,
REDACTED
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %s", len(parts), result)
REDACTED
	if !strings.HasPrefix(parts[0], "u:") {
		t.Errorf("part[0] expected prefix 'u:', got: %s", parts[0])
REDACTED
	if !strings.HasPrefix(parts[1], "a:") {
		t.Errorf("part[1] expected prefix 'a:', got: %s", parts[1])
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_WithSystemString(t *testing.T) {
	parsed := &ParsedRequest{
		System: "You are a helpful assistant",
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (s + u), got %d: %s", len(parts), result)
REDACTED
	if !strings.HasPrefix(parts[0], "s:") {
		t.Errorf("part[0] expected prefix 's:', got: %s", parts[0])
REDACTED
	if !strings.HasPrefix(parts[1], "u:") {
		t.Errorf("part[1] expected prefix 'u:', got: %s", parts[1])
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_WithSystemContentBlocks(t *testing.T) {
	parsed := &ParsedRequest{
		System: []any{
			map[string]any{"type": "text", "text": "You are a helpful assistant"REDACTED,
	REDACTED,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (s + u), got %d: %s", len(parts), result)
REDACTED
	if !strings.HasPrefix(parts[0], "s:") {
		t.Errorf("part[0] expected prefix 's:', got: %s", parts[0])
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_ConversationPrefixRelationship(t *testing.T) {
	// 核心测试：验证对话增长时链的前缀关系
	// 上一轮的完整链一定是下一轮链的前缀
	system := "You are a helpful assistant"

	// 第 1 轮: system + user
	round1 := &ParsedRequest{
		System: system,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED
	chain1 := BuildAnthropicDigestChain(round1)

	// 第 2 轮: system + user + assistant + user
	round2 := &ParsedRequest{
		System: system,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
			map[string]any{"role": "assistant", "content": "hi there"REDACTED,
			map[string]any{"role": "user", "content": "how are you?"REDACTED,
	REDACTED,
REDACTED
	chain2 := BuildAnthropicDigestChain(round2)

	// 第 3 轮: system + user + assistant + user + assistant + user
	round3 := &ParsedRequest{
		System: system,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
			map[string]any{"role": "assistant", "content": "hi there"REDACTED,
			map[string]any{"role": "user", "content": "how are you?"REDACTED,
			map[string]any{"role": "assistant", "content": "I'm doing well"REDACTED,
			map[string]any{"role": "user", "content": "great"REDACTED,
	REDACTED,
REDACTED
	chain3 := BuildAnthropicDigestChain(round3)

	t.Logf("Chain1: %s", chain1)
	t.Logf("Chain2: %s", chain2)
	t.Logf("Chain3: %s", chain3)

	// chain1 是 chain2 的前缀
	if !strings.HasPrefix(chain2, chain1) {
		t.Errorf("chain1 should be prefix of chain2:\n  chain1: %s\n  chain2: %s", chain1, chain2)
REDACTED

	// chain2 是 chain3 的前缀
	if !strings.HasPrefix(chain3, chain2) {
		t.Errorf("chain2 should be prefix of chain3:\n  chain2: %s\n  chain3: %s", chain2, chain3)
REDACTED

	// chain1 也是 chain3 的前缀（传递性）
	if !strings.HasPrefix(chain3, chain1) {
		t.Errorf("chain1 should be prefix of chain3:\n  chain1: %s\n  chain3: %s", chain1, chain3)
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_DifferentSystemProducesDifferentChain(t *testing.T) {
	parsed1 := &ParsedRequest{
		System: "System A",
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED
	parsed2 := &ParsedRequest{
		System: "System B",
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
	REDACTED,
REDACTED

	chain1 := BuildAnthropicDigestChain(parsed1)
	chain2 := BuildAnthropicDigestChain(parsed2)

	if chain1 == chain2 {
		t.Error("Different system prompts should produce different chains")
REDACTED

	// 但 user 部分的 hash 应该相同
	parts1 := splitChain(chain1)
	parts2 := splitChain(chain2)
	if parts1[1] != parts2[1] {
		t.Error("Same user message should produce same hash regardless of system")
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_DifferentContentProducesDifferentChain(t *testing.T) {
	parsed1 := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
			map[string]any{"role": "assistant", "content": "ORIGINAL reply"REDACTED,
			map[string]any{"role": "user", "content": "next"REDACTED,
	REDACTED,
REDACTED
	parsed2 := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
			map[string]any{"role": "assistant", "content": "TAMPERED reply"REDACTED,
			map[string]any{"role": "user", "content": "next"REDACTED,
	REDACTED,
REDACTED

	chain1 := BuildAnthropicDigestChain(parsed1)
	chain2 := BuildAnthropicDigestChain(parsed2)

	if chain1 == chain2 {
		t.Error("Different content should produce different chains")
REDACTED

	parts1 := splitChain(chain1)
	parts2 := splitChain(chain2)
	// 第一个 user message hash 应该相同
	if parts1[0] != parts2[0] {
		t.Error("First user message hash should be the same")
REDACTED
	// assistant reply hash 应该不同
	if parts1[1] == parts2[1] {
		t.Error("Assistant reply hash should differ")
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_Deterministic(t *testing.T) {
	parsed := &ParsedRequest{
		System: "test system",
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"REDACTED,
			map[string]any{"role": "assistant", "content": "hi"REDACTED,
	REDACTED,
REDACTED

	chain1 := BuildAnthropicDigestChain(parsed)
	chain2 := BuildAnthropicDigestChain(parsed)

	if chain1 != chain2 {
		t.Errorf("BuildAnthropicDigestChain not deterministic: %s vs %s", chain1, chain2)
REDACTED
REDACTED

func TestBuildAnthropicTrieKey(t *testing.T) {
	tests := []struct {
		name       string
		groupID    int64
		prefixHash string
		want       string
REDACTED{
		{
			name:       "normal",
			groupID:    123,
			prefixHash: "abcdef12",
			want:       "anthropic:trie:123:abcdef12",
	REDACTED,
		{
			name:       "zero group",
			groupID:    0,
			prefixHash: "xyz",
			want:       "anthropic:trie:0:xyz",
	REDACTED,
		{
			name:       "empty prefix",
			groupID:    1,
			prefixHash: "",
			want:       "anthropic:trie:1:",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildAnthropicTrieKey(tt.groupID, tt.prefixHash)
			if got != tt.want {
				t.Errorf("BuildAnthropicTrieKey(%d, %q) = %q, want %q", tt.groupID, tt.prefixHash, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGenerateAnthropicDigestSessionKey(t *testing.T) {
	tests := []struct {
		name       string
		prefixHash string
		uuid       string
		want       string
REDACTED{
		{
			name:       "normal 16 char hash with uuid",
			prefixHash: "abcdefgh12345678",
			uuid:       "550e8400-e29b-41d4-a716-446655440000",
			want:       "anthropic:digest:abcdefgh:550e8400",
	REDACTED,
		{
			name:       "exactly 8 chars",
			prefixHash: "12345678",
			uuid:       "abcdefgh",
			want:       "anthropic:digest:12345678:abcdefgh",
	REDACTED,
		{
			name:       "short values",
			prefixHash: "abc",
			uuid:       "xyz",
			want:       "anthropic:digest:abc:xyz",
	REDACTED,
		{
			name:       "empty values",
			prefixHash: "",
			uuid:       "",
			want:       "anthropic:digest::",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateAnthropicDigestSessionKey(tt.prefixHash, tt.uuid)
			if got != tt.want {
				t.Errorf("GenerateAnthropicDigestSessionKey(%q, %q) = %q, want %q", tt.prefixHash, tt.uuid, got, tt.want)
		REDACTED
	REDACTED)
REDACTED

	// 验证不同 uuid 产生不同 sessionKey
	t.Run("different uuid different key", func(t *testing.T) {
		hash := "sameprefix123456"
		result1 := GenerateAnthropicDigestSessionKey(hash, "uuid0001-session-a")
		result2 := GenerateAnthropicDigestSessionKey(hash, "uuid0002-session-b")
		if result1 == result2 {
			t.Errorf("Different UUIDs should produce different session keys: %s vs %s", result1, result2)
	REDACTED
REDACTED)
REDACTED

func TestAnthropicSessionTTL(t *testing.T) {
	ttl := AnthropicSessionTTL()
	if ttl.Seconds() != 300 {
		t.Errorf("expected 300 seconds, got: %v", ttl.Seconds())
REDACTED
REDACTED

func TestBuildAnthropicDigestChain_ContentBlocks(t *testing.T) {
	// 测试 content 为 content blocks 数组的情况
	parsed := &ParsedRequest{
		Messages: []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "describe this image"REDACTED,
					map[string]any{"type": "image", "source": map[string]any{"type": "base64"REDACTEDREDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d: %s", len(parts), result)
REDACTED
	if !strings.HasPrefix(parts[0], "u:") {
		t.Errorf("expected prefix 'u:', got: %s", parts[0])
REDACTED
REDACTED
