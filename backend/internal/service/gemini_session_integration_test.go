package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// mockGeminiSessionCache 模拟 Redis 缓存
type mockGeminiSessionCache struct {
	sessions map[string]string // key -> value
REDACTED

func newMockGeminiSessionCache() *mockGeminiSessionCache {
	return &mockGeminiSessionCache{sessions: make(map[string]string)REDACTED
REDACTED

func (m *mockGeminiSessionCache) Save(groupID int64, prefixHash, digestChain, uuid string, accountID int64) {
	key := BuildGeminiSessionKey(groupID, prefixHash, digestChain)
	value := FormatGeminiSessionValue(uuid, accountID)
	m.sessions[key] = value
REDACTED

func (m *mockGeminiSessionCache) Find(groupID int64, prefixHash, digestChain string) (uuid string, accountID int64, found bool) {
	prefixes := GenerateDigestChainPrefixes(digestChain)
	for _, p := range prefixes {
		key := BuildGeminiSessionKey(groupID, prefixHash, p)
		if val, ok := m.sessions[key]; ok {
			return ParseGeminiSessionValue(val)
	REDACTED
REDACTED
	return "", 0, false
REDACTED

// TestGeminiSessionContinuousConversation 测试连续会话的摘要链匹配
func TestGeminiSessionContinuousConversation(t *testing.T) {
	cache := newMockGeminiSessionCache()
	groupID := int64(1)
	prefixHash := "test_prefix_hash"
	sessionUUID := "session-uuid-12345"
	accountID := int64(100)

	// 模拟第一轮对话
	req1 := &antigravity.GeminiRequest{
		SystemInstruction: &antigravity.GeminiContent{
			Parts: []antigravity.GeminiPart{{Text: "You are a helpful assistant"REDACTEDREDACTED,
	REDACTED,
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Hello, what's your name?"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED
	chain1 := BuildGeminiDigestChain(req1)
	t.Logf("Round 1 chain: %s", chain1)

	// 第一轮：没有找到会话，创建新会话
	_, _, found := cache.Find(groupID, prefixHash, chain1)
	if found {
		t.Error("Round 1: should not find existing session")
REDACTED

	// 保存第一轮会话
	cache.Save(groupID, prefixHash, chain1, sessionUUID, accountID)

	// 模拟第二轮对话（用户继续对话）
	req2 := &antigravity.GeminiRequest{
		SystemInstruction: &antigravity.GeminiContent{
			Parts: []antigravity.GeminiPart{{Text: "You are a helpful assistant"REDACTEDREDACTED,
	REDACTED,
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Hello, what's your name?"REDACTEDREDACTEDREDACTED,
			{Role: "model", Parts: []antigravity.GeminiPart{{Text: "I'm Claude, nice to meet you!"REDACTEDREDACTEDREDACTED,
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "What can you do?"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED
	chain2 := BuildGeminiDigestChain(req2)
	t.Logf("Round 2 chain: %s", chain2)

	// 第二轮：应该能找到会话（通过前缀匹配）
	foundUUID, foundAccID, found := cache.Find(groupID, prefixHash, chain2)
	if !found {
		t.Error("Round 2: should find session via prefix matching")
REDACTED
	if foundUUID != sessionUUID {
		t.Errorf("Round 2: expected UUID %s, got %s", sessionUUID, foundUUID)
REDACTED
	if foundAccID != accountID {
		t.Errorf("Round 2: expected accountID %d, got %d", accountID, foundAccID)
REDACTED

	// 保存第二轮会话
	cache.Save(groupID, prefixHash, chain2, sessionUUID, accountID)

	// 模拟第三轮对话
	req3 := &antigravity.GeminiRequest{
		SystemInstruction: &antigravity.GeminiContent{
			Parts: []antigravity.GeminiPart{{Text: "You are a helpful assistant"REDACTEDREDACTED,
	REDACTED,
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Hello, what's your name?"REDACTEDREDACTEDREDACTED,
			{Role: "model", Parts: []antigravity.GeminiPart{{Text: "I'm Claude, nice to meet you!"REDACTEDREDACTEDREDACTED,
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "What can you do?"REDACTEDREDACTEDREDACTED,
			{Role: "model", Parts: []antigravity.GeminiPart{{Text: "I can help with coding, writing, and more!"REDACTEDREDACTEDREDACTED,
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Great, help me write some Go code"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED
	chain3 := BuildGeminiDigestChain(req3)
	t.Logf("Round 3 chain: %s", chain3)

	// 第三轮：应该能找到会话（通过第二轮的前缀匹配）
	foundUUID, foundAccID, found = cache.Find(groupID, prefixHash, chain3)
	if !found {
		t.Error("Round 3: should find session via prefix matching")
REDACTED
	if foundUUID != sessionUUID {
		t.Errorf("Round 3: expected UUID %s, got %s", sessionUUID, foundUUID)
REDACTED
	if foundAccID != accountID {
		t.Errorf("Round 3: expected accountID %d, got %d", accountID, foundAccID)
REDACTED

	t.Log("✓ Continuous conversation session matching works correctly!")
REDACTED

// TestGeminiSessionDifferentConversations 测试不同会话不会错误匹配
func TestGeminiSessionDifferentConversations(t *testing.T) {
	cache := newMockGeminiSessionCache()
	groupID := int64(1)
	prefixHash := "test_prefix_hash"

	// 第一个会话
	req1 := &antigravity.GeminiRequest{
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Tell me about Go programming"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED
	chain1 := BuildGeminiDigestChain(req1)
	cache.Save(groupID, prefixHash, chain1, "session-1", 100)

	// 第二个完全不同的会话
	req2 := &antigravity.GeminiRequest{
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "What's the weather today?"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED
	chain2 := BuildGeminiDigestChain(req2)

	// 不同会话不应该匹配
	_, _, found := cache.Find(groupID, prefixHash, chain2)
	if found {
		t.Error("Different conversations should not match")
REDACTED

	t.Log("✓ Different conversations are correctly isolated!")
REDACTED

// TestGeminiSessionPrefixMatchingOrder 测试前缀匹配的优先级（最长匹配优先）
func TestGeminiSessionPrefixMatchingOrder(t *testing.T) {
	cache := newMockGeminiSessionCache()
	groupID := int64(1)
	prefixHash := "test_prefix_hash"

	// 创建一个三轮对话
	req := &antigravity.GeminiRequest{
		SystemInstruction: &antigravity.GeminiContent{
			Parts: []antigravity.GeminiPart{{Text: "System prompt"REDACTEDREDACTED,
	REDACTED,
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Q1"REDACTEDREDACTEDREDACTED,
			{Role: "model", Parts: []antigravity.GeminiPart{{Text: "A1"REDACTEDREDACTEDREDACTED,
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Q2"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED
	fullChain := BuildGeminiDigestChain(req)
	prefixes := GenerateDigestChainPrefixes(fullChain)

	t.Logf("Full chain: %s", fullChain)
	t.Logf("Prefixes (longest first): %v", prefixes)

	// 验证前缀生成顺序（从长到短）
	if len(prefixes) != 4 {
		t.Errorf("Expected 4 prefixes, got %d", len(prefixes))
REDACTED

	// 保存不同轮次的会话到不同账号
	// 第一轮（最短前缀）-> 账号 1
	cache.Save(groupID, prefixHash, prefixes[3], "session-round1", 1)
	// 第二轮 -> 账号 2
	cache.Save(groupID, prefixHash, prefixes[2], "session-round2", 2)
	// 第三轮（最长前缀，完整链）-> 账号 3
	cache.Save(groupID, prefixHash, prefixes[0], "session-round3", 3)

	// 查找应该返回最长匹配（账号 3）
	_, accID, found := cache.Find(groupID, prefixHash, fullChain)
	if !found {
		t.Error("Should find session")
REDACTED
	if accID != 3 {
		t.Errorf("Should match longest prefix (account 3), got account %d", accID)
REDACTED

	t.Log("✓ Longest prefix matching works correctly!")
REDACTED

// 确保 context 包被使用（避免未使用的导入警告）
var _ = context.Background
