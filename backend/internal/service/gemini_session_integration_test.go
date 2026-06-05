package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// TestGeminiSessionContinuousConversation 测试连续会话的摘要链匹配
func TestGeminiSessionContinuousConversation(t *testing.T) {
	store := NewDigestSessionStore()
	groupID := int64(1)
	prefixHash := "test_prefix_hash"
	sessionUUID := "session-uuid-12345"
	accountID := int64(100)

	// 模拟第一轮对话
	req1 := &antigravity.GeminiRequest{
		SystemInstruction: &antigravity.GeminiContent{
			Parts: []antigravity.GeminiPart{{Text: "You are a helpful assistant"}},
		},
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Hello, what's your name?"}}},
		},
	}
	chain1 := BuildGeminiDigestChain(req1)
	t.Logf("Round 1 chain: %s", chain1)

	// 第一轮：没有找到会话，创建新会话
	_, _, _, found := store.Find(groupID, prefixHash, chain1)
	if found {
		t.Error("Round 1: should not find existing session")
	}

	// 保存第一轮会话（首轮无旧 chain）
	store.Save(groupID, prefixHash, chain1, sessionUUID, accountID, "")

	// 模拟第二轮对话（用户继续对话）
	req2 := &antigravity.GeminiRequest{
		SystemInstruction: &antigravity.GeminiContent{
			Parts: []antigravity.GeminiPart{{Text: "You are a helpful assistant"}},
		},
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Hello, what's your name?"}}},
			{Role: "model", Parts: []antigravity.GeminiPart{{Text: "I'm Claude, nice to meet you!"}}},
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "What can you do?"}}},
		},
	}
	chain2 := BuildGeminiDigestChain(req2)
	t.Logf("Round 2 chain: %s", chain2)

	// 第二轮：应该能找到会话（通过前缀匹配）
	foundUUID, foundAccID, matchedChain, found := store.Find(groupID, prefixHash, chain2)
	if !found {
		t.Error("Round 2: should find session via prefix matching")
	}
	if foundUUID != sessionUUID {
		t.Errorf("Round 2: expected UUID %s, got %s", sessionUUID, foundUUID)
	}
	if foundAccID != accountID {
		t.Errorf("Round 2: expected accountID %d, got %d", accountID, foundAccID)
	}

	// 保存第二轮会话，传入 Find 返回的 matchedChain 以删旧 key
	store.Save(groupID, prefixHash, chain2, sessionUUID, accountID, matchedChain)

	// 模拟第三轮对话
	req3 := &antigravity.GeminiRequest{
		SystemInstruction: &antigravity.GeminiContent{
			Parts: []antigravity.GeminiPart{{Text: "You are a helpful assistant"}},
		},
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Hello, what's your name?"}}},
			{Role: "model", Parts: []antigravity.GeminiPart{{Text: "I'm Claude, nice to meet you!"}}},
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "What can you do?"}}},
			{Role: "model", Parts: []antigravity.GeminiPart{{Text: "I can help with coding, writing, and more!"}}},
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Great, help me write some Go code"}}},
		},
	}
	chain3 := BuildGeminiDigestChain(req3)
	t.Logf("Round 3 chain: %s", chain3)

	// 第三轮：应该能找到会话（通过第二轮的前缀匹配）
	foundUUID, foundAccID, _, found = store.Find(groupID, prefixHash, chain3)
	if !found {
		t.Error("Round 3: should find session via prefix matching")
	}
	if foundUUID != sessionUUID {
		t.Errorf("Round 3: expected UUID %s, got %s", sessionUUID, foundUUID)
	}
	if foundAccID != accountID {
		t.Errorf("Round 3: expected accountID %d, got %d", accountID, foundAccID)
	}
}

// TestGeminiSessionDifferentConversations 测试不同会话不会错误匹配
func TestGeminiSessionDifferentConversations(t *testing.T) {
	store := NewDigestSessionStore()
	groupID := int64(1)
	prefixHash := "test_prefix_hash"

	// 第一个会话
	req1 := &antigravity.GeminiRequest{
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "Tell me about Go programming"}}},
		},
	}
	chain1 := BuildGeminiDigestChain(req1)
	store.Save(groupID, prefixHash, chain1, "session-1", 100, "")

	// 第二个完全不同的会话
	req2 := &antigravity.GeminiRequest{
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "What's the weather today?"}}},
		},
	}
	chain2 := BuildGeminiDigestChain(req2)

	// 不同会话不应该匹配
	_, _, _, found := store.Find(groupID, prefixHash, chain2)
	if found {
		t.Error("Different conversations should not match")
	}
}

// TestGeminiSessionPrefixMatchingOrder 测试前缀匹配的优先级（最长匹配优先）
func TestGeminiSessionPrefixMatchingOrder(t *testing.T) {
	store := NewDigestSessionStore()
	groupID := int64(1)
	prefixHash := "test_prefix_hash"

	// 保存不同轮次的会话到不同账号
	store.Save(groupID, prefixHash, "s:sys-u:q1", "session-round1", 1, "")
	store.Save(groupID, prefixHash, "s:sys-u:q1-m:a1", "session-round2", 2, "")
	store.Save(groupID, prefixHash, "s:sys-u:q1-m:a1-u:q2", "session-round3", 3, "")

	// 查找更长的链，应该返回最长匹配（账号 3）
	_, accID, _, found := store.Find(groupID, prefixHash, "s:sys-u:q1-m:a1-u:q2-m:a2")
	if !found {
		t.Error("Should find session")
	}
	if accID != 3 {
		t.Errorf("Should match longest prefix (account 3), got account %d", accID)
	}
}
