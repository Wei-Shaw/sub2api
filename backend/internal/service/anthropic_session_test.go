//go:build unit

package service

import (
	"strings"
	"testing"
)

func mustParseAnthropicDigestRequest(t *testing.T, body string) *ParsedRequest {
	t.Helper()
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), "")
	if err != nil {
		t.Fatalf("ParseGatewayRequest failed: %v", err)
	}
	return parsed
}

func TestBuildAnthropicDigestChain_NilRequest(t *testing.T) {
	result := BuildAnthropicDigestChain(nil)
	if result != "" {
		t.Errorf("expected empty string for nil request, got: %s", result)
	}
}

func TestBuildAnthropicDigestChain_EmptyMessages(t *testing.T) {
	parsed := mustParseAnthropicDigestRequest(t, `{"messages":[]}`)
	result := BuildAnthropicDigestChain(parsed)
	if result != "" {
		t.Errorf("expected empty string for empty messages, got: %s", result)
	}
}

func TestBuildAnthropicDigestChain_SingleUserMessage(t *testing.T) {
	parsed := mustParseAnthropicDigestRequest(t, `{"messages":[{"role":"user","content":"hello"}]}`)
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d: %s", len(parts), result)
	}
	if !strings.HasPrefix(parts[0], "u:") {
		t.Errorf("expected prefix 'u:', got: %s", parts[0])
	}
}

func TestBuildAnthropicDigestChain_UserAndAssistant(t *testing.T) {
	parsed := mustParseAnthropicDigestRequest(t, `{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi there"}]}`)
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %s", len(parts), result)
	}
	if !strings.HasPrefix(parts[0], "u:") {
		t.Errorf("part[0] expected prefix 'u:', got: %s", parts[0])
	}
	if !strings.HasPrefix(parts[1], "a:") {
		t.Errorf("part[1] expected prefix 'a:', got: %s", parts[1])
	}
}

func TestBuildAnthropicDigestChain_WithSystemString(t *testing.T) {
	parsed := mustParseAnthropicDigestRequest(t, `{"system":"You are a helpful assistant","messages":[{"role":"user","content":"hello"}]}`)
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (s + u), got %d: %s", len(parts), result)
	}
	if !strings.HasPrefix(parts[0], "s:") {
		t.Errorf("part[0] expected prefix 's:', got: %s", parts[0])
	}
	if !strings.HasPrefix(parts[1], "u:") {
		t.Errorf("part[1] expected prefix 'u:', got: %s", parts[1])
	}
}

func TestBuildAnthropicDigestChain_WithSystemContentBlocks(t *testing.T) {
	parsed := mustParseAnthropicDigestRequest(t, `{"system":[{"type":"text","text":"You are a helpful assistant"}],"messages":[{"role":"user","content":"hello"}]}`)
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (s + u), got %d: %s", len(parts), result)
	}
	if !strings.HasPrefix(parts[0], "s:") {
		t.Errorf("part[0] expected prefix 's:', got: %s", parts[0])
	}
}

func TestBuildAnthropicDigestChain_ConversationPrefixRelationship(t *testing.T) {
	// 核心测试：验证对话增长时链的前缀关系
	// 上一轮的完整链一定是下一轮链的前缀
	round1 := mustParseAnthropicDigestRequest(t, `{"system":"You are a helpful assistant","messages":[{"role":"user","content":"hello"}]}`)
	chain1 := BuildAnthropicDigestChain(round1)

	round2 := mustParseAnthropicDigestRequest(t, `{"system":"You are a helpful assistant","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi there"},{"role":"user","content":"how are you?"}]}`)
	chain2 := BuildAnthropicDigestChain(round2)

	round3 := mustParseAnthropicDigestRequest(t, `{"system":"You are a helpful assistant","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi there"},{"role":"user","content":"how are you?"},{"role":"assistant","content":"I'm doing well"},{"role":"user","content":"great"}]}`)
	chain3 := BuildAnthropicDigestChain(round3)

	t.Logf("Chain1: %s", chain1)
	t.Logf("Chain2: %s", chain2)
	t.Logf("Chain3: %s", chain3)

	if !strings.HasPrefix(chain2, chain1) {
		t.Errorf("chain1 should be prefix of chain2:\n  chain1: %s\n  chain2: %s", chain1, chain2)
	}
	if !strings.HasPrefix(chain3, chain2) {
		t.Errorf("chain2 should be prefix of chain3:\n  chain2: %s\n  chain3: %s", chain2, chain3)
	}
	if !strings.HasPrefix(chain3, chain1) {
		t.Errorf("chain1 should be prefix of chain3:\n  chain1: %s\n  chain3: %s", chain1, chain3)
	}
}

func TestBuildAnthropicDigestChain_DifferentSystemProducesDifferentChain(t *testing.T) {
	parsed1 := mustParseAnthropicDigestRequest(t, `{"system":"System A","messages":[{"role":"user","content":"hello"}]}`)
	parsed2 := mustParseAnthropicDigestRequest(t, `{"system":"System B","messages":[{"role":"user","content":"hello"}]}`)

	chain1 := BuildAnthropicDigestChain(parsed1)
	chain2 := BuildAnthropicDigestChain(parsed2)

	if chain1 == chain2 {
		t.Error("Different system prompts should produce different chains")
	}

	parts1 := splitChain(chain1)
	parts2 := splitChain(chain2)
	if parts1[1] != parts2[1] {
		t.Error("Same user message should produce same hash regardless of system")
	}
}

func TestBuildAnthropicDigestChain_DifferentContentProducesDifferentChain(t *testing.T) {
	parsed1 := mustParseAnthropicDigestRequest(t, `{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"ORIGINAL reply"},{"role":"user","content":"next"}]}`)
	parsed2 := mustParseAnthropicDigestRequest(t, `{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"TAMPERED reply"},{"role":"user","content":"next"}]}`)

	chain1 := BuildAnthropicDigestChain(parsed1)
	chain2 := BuildAnthropicDigestChain(parsed2)

	if chain1 == chain2 {
		t.Error("Different content should produce different chains")
	}

	parts1 := splitChain(chain1)
	parts2 := splitChain(chain2)
	if parts1[0] != parts2[0] {
		t.Error("First user message hash should be the same")
	}
	if parts1[1] == parts2[1] {
		t.Error("Assistant reply hash should differ")
	}
}

func TestBuildAnthropicDigestChain_Deterministic(t *testing.T) {
	parsed := mustParseAnthropicDigestRequest(t, `{"system":"test system","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]}`)

	chain1 := BuildAnthropicDigestChain(parsed)
	chain2 := BuildAnthropicDigestChain(parsed)

	if chain1 != chain2 {
		t.Errorf("BuildAnthropicDigestChain not deterministic: %s vs %s", chain1, chain2)
	}
}

func TestBuildAnthropicDigestChain_CanonicalJSON(t *testing.T) {
	parsed1 := mustParseAnthropicDigestRequest(t, `{"system":[{"type":"text","text":"system"}],"messages":[{"role":"user","content":{"type":"text","text":"hello"}}]}`)
	parsed2 := mustParseAnthropicDigestRequest(t, `{"system":[{"text":"system","type":"text"}],"messages":[{"role":"user","content":{"text":"hello","type":"text"}}]}`)

	chain1 := BuildAnthropicDigestChain(parsed1)
	chain2 := BuildAnthropicDigestChain(parsed2)

	if chain1 != chain2 {
		t.Errorf("semantically equivalent JSON should produce same chain: %s vs %s", chain1, chain2)
	}
}

func TestGenerateAnthropicDigestSessionKey(t *testing.T) {
	tests := []struct {
		name       string
		prefixHash string
		uuid       string
		want       string
	}{
		{
			name:       "normal 16 char hash with uuid",
			prefixHash: "abcdefgh12345678",
			uuid:       "550e8400-e29b-41d4-a716-446655440000",
			want:       "anthropic:digest:abcdefgh:550e8400",
		},
		{
			name:       "exactly 8 chars",
			prefixHash: "12345678",
			uuid:       "abcdefgh",
			want:       "anthropic:digest:12345678:abcdefgh",
		},
		{
			name:       "short values",
			prefixHash: "abc",
			uuid:       "xyz",
			want:       "anthropic:digest:abc:xyz",
		},
		{
			name:       "empty values",
			prefixHash: "",
			uuid:       "",
			want:       "anthropic:digest::",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateAnthropicDigestSessionKey(tt.prefixHash, tt.uuid)
			if got != tt.want {
				t.Errorf("GenerateAnthropicDigestSessionKey(%q, %q) = %q, want %q", tt.prefixHash, tt.uuid, got, tt.want)
			}
		})
	}

	t.Run("different uuid different key", func(t *testing.T) {
		hash := "sameprefix123456"
		result1 := GenerateAnthropicDigestSessionKey(hash, "uuid0001-session-a")
		result2 := GenerateAnthropicDigestSessionKey(hash, "uuid0002-session-b")
		if result1 == result2 {
			t.Errorf("Different UUIDs should produce different session keys: %s vs %s", result1, result2)
		}
	})
}

func TestAnthropicSessionTTL(t *testing.T) {
	ttl := AnthropicSessionTTL()
	if ttl.Seconds() != 300 {
		t.Errorf("expected 300 seconds, got: %v", ttl.Seconds())
	}
}

func TestBuildAnthropicDigestChain_ContentBlocks(t *testing.T) {
	parsed := mustParseAnthropicDigestRequest(t, `{"messages":[{"role":"user","content":[{"type":"text","text":"describe this image"},{"type":"image","source":{"type":"base64"}}]}]}`)
	result := BuildAnthropicDigestChain(parsed)
	parts := splitChain(result)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d: %s", len(parts), result)
	}
	if !strings.HasPrefix(parts[0], "u:") {
		t.Errorf("expected prefix 'u:', got: %s", parts[0])
	}
}
