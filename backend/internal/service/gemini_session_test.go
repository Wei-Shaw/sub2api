package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

func TestShortHash(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte{}},
		{"simple", []byte("hello world")},
		{"json", []byte(`{"role":"user","parts":[{"text":"hello"}]}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shortHash(tt.input)
			// Base36 编码的 uint64 最长 13 个字符
			if len(result) > 13 {
				t.Errorf("shortHash result too long: %d characters", len(result))
			}
			// 相同输入应该产生相同输出
			result2 := shortHash(tt.input)
			if result != result2 {
				t.Errorf("shortHash not deterministic: %s vs %s", result, result2)
			}
		})
	}
}

func TestBuildGeminiDigestChain(t *testing.T) {
	tests := []struct {
		name     string
		req      *antigravity.GeminiRequest
		wantLen  int  // 预期的分段数量
		hasEmpty bool // 是否应该是空字符串
	}{
		{
			name:     "nil request",
			req:      nil,
			hasEmpty: true,
		},
		{
			name: "empty contents",
			req: &antigravity.GeminiRequest{
				Contents: []antigravity.GeminiContent{},
			},
			hasEmpty: true,
		},
		{
			name: "single user message",
			req: &antigravity.GeminiRequest{
				Contents: []antigravity.GeminiContent{
					{Role: "user", Parts: []antigravity.GeminiPart{{Text: "hello"}}},
				},
			},
			wantLen: 1, // u:<hash>
		},
		{
			name: "user and model messages",
			req: &antigravity.GeminiRequest{
				Contents: []antigravity.GeminiContent{
					{Role: "user", Parts: []antigravity.GeminiPart{{Text: "hello"}}},
					{Role: "model", Parts: []antigravity.GeminiPart{{Text: "hi there"}}},
				},
			},
			wantLen: 2, // u:<hash>-m:<hash>
		},
		{
			name: "with system instruction",
			req: &antigravity.GeminiRequest{
				SystemInstruction: &antigravity.GeminiContent{
					Role:  "user",
					Parts: []antigravity.GeminiPart{{Text: "You are a helpful assistant"}},
				},
				Contents: []antigravity.GeminiContent{
					{Role: "user", Parts: []antigravity.GeminiPart{{Text: "hello"}}},
				},
			},
			wantLen: 2, // s:<hash>-u:<hash>
		},
		{
			name: "conversation with system",
			req: &antigravity.GeminiRequest{
				SystemInstruction: &antigravity.GeminiContent{
					Role:  "user",
					Parts: []antigravity.GeminiPart{{Text: "System prompt"}},
				},
				Contents: []antigravity.GeminiContent{
					{Role: "user", Parts: []antigravity.GeminiPart{{Text: "hello"}}},
					{Role: "model", Parts: []antigravity.GeminiPart{{Text: "hi"}}},
					{Role: "user", Parts: []antigravity.GeminiPart{{Text: "how are you?"}}},
				},
			},
			wantLen: 4, // s:<hash>-u:<hash>-m:<hash>-u:<hash>
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildGeminiDigestChain(tt.req)

			if tt.hasEmpty {
				if result != "" {
					t.Errorf("expected empty string, got: %s", result)
				}
				return
			}

			// 检查分段数量
			parts := splitChain(result)
			if len(parts) != tt.wantLen {
				t.Errorf("expected %d parts, got %d: %s", tt.wantLen, len(parts), result)
			}

			// 验证每个分段的格式
			for _, part := range parts {
				if len(part) < 3 || part[1] != ':' {
					t.Errorf("invalid part format: %s", part)
				}
				prefix := part[0]
				if prefix != 's' && prefix != 'u' && prefix != 'm' {
					t.Errorf("invalid prefix: %c", prefix)
				}
			}
		})
	}
}

func TestGenerateGeminiPrefixHash(t *testing.T) {
	hash1 := GenerateGeminiPrefixHash(1, 100, "192.168.1.1", "Mozilla/5.0", "antigravity", "gemini-2.5-pro")
	hash2 := GenerateGeminiPrefixHash(1, 100, "192.168.1.1", "Mozilla/5.0", "antigravity", "gemini-2.5-pro")
	hash3 := GenerateGeminiPrefixHash(2, 100, "192.168.1.1", "Mozilla/5.0", "antigravity", "gemini-2.5-pro")

	// 相同输入应该产生相同输出
	if hash1 != hash2 {
		t.Errorf("GenerateGeminiPrefixHash not deterministic: %s vs %s", hash1, hash2)
	}

	// 不同输入应该产生不同输出
	if hash1 == hash3 {
		t.Errorf("GenerateGeminiPrefixHash collision for different inputs")
	}

	// Base64 URL 编码的 12 字节正好是 16 字符
	if len(hash1) != 16 {
		t.Errorf("expected 16 characters, got %d: %s", len(hash1), hash1)
	}
}

func TestGenerateGeminiPrefixHash_IgnoresUserAgentVersionNoise(t *testing.T) {
	hash1 := GenerateGeminiPrefixHash(1, 100, "192.168.1.1", "Mozilla/5.0 codex_cli_rs/0.1.0", "antigravity", "gemini-2.5-pro")
	hash2 := GenerateGeminiPrefixHash(1, 100, "192.168.1.1", "Mozilla/5.0 codex_cli_rs/0.1.1", "antigravity", "gemini-2.5-pro")

	if hash1 != hash2 {
		t.Fatalf("version-only User-Agent changes should not perturb Gemini prefix hash: %s vs %s", hash1, hash2)
	}
}

func TestGenerateGeminiPrefixHash_IgnoresFreeformUserAgentVersionNoise(t *testing.T) {
	hash1 := GenerateGeminiPrefixHash(1, 100, "192.168.1.1", "Codex CLI 0.1.0", "antigravity", "gemini-2.5-pro")
	hash2 := GenerateGeminiPrefixHash(1, 100, "192.168.1.1", "Codex CLI 0.1.1", "antigravity", "gemini-2.5-pro")

	if hash1 != hash2 {
		t.Fatalf("free-form version-only User-Agent changes should not perturb Gemini prefix hash: %s vs %s", hash1, hash2)
	}
}

func TestParseGeminiSessionValue(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantUUID  string
		wantAccID int64
		wantOK    bool
	}{
		{
			name:   "empty",
			value:  "",
			wantOK: false,
		},
		{
			name:   "no colon",
			value:  "abc123",
			wantOK: false,
		},
		{
			name:      "valid",
			value:     "uuid-1234:100",
			wantUUID:  "uuid-1234",
			wantAccID: 100,
			wantOK:    true,
		},
		{
			name:      "uuid with colon",
			value:     "a:b:c:123",
			wantUUID:  "a:b:c",
			wantAccID: 123,
			wantOK:    true,
		},
		{
			name:   "invalid account id",
			value:  "uuid:abc",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uuid, accID, ok := ParseGeminiSessionValue(tt.value)

			if ok != tt.wantOK {
				t.Errorf("ok: expected %v, got %v", tt.wantOK, ok)
			}

			if tt.wantOK {
				if uuid != tt.wantUUID {
					t.Errorf("uuid: expected %s, got %s", tt.wantUUID, uuid)
				}
				if accID != tt.wantAccID {
					t.Errorf("accountID: expected %d, got %d", tt.wantAccID, accID)
				}
			}
		})
	}
}

func TestFormatGeminiSessionValue(t *testing.T) {
	result := FormatGeminiSessionValue("test-uuid", 123)
	expected := "test-uuid:123"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}

	// 验证往返一致性
	uuid, accID, ok := ParseGeminiSessionValue(result)
	if !ok {
		t.Error("ParseGeminiSessionValue failed on formatted value")
	}
	if uuid != "test-uuid" || accID != 123 {
		t.Errorf("round-trip failed: uuid=%s, accID=%d", uuid, accID)
	}
}

// splitChain 辅助函数：按 "-" 分割摘要链
func splitChain(chain string) []string {
	if chain == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(chain); i++ {
		if chain[i] == '-' {
			parts = append(parts, chain[start:i])
			start = i + 1
		}
	}
	if start < len(chain) {
		parts = append(parts, chain[start:])
	}
	return parts
}

func TestDigestChainDifferentSysInstruction(t *testing.T) {
	req1 := &antigravity.GeminiRequest{
		SystemInstruction: &antigravity.GeminiContent{
			Parts: []antigravity.GeminiPart{{Text: "SYS_ORIGINAL"}},
		},
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "hello"}}},
		},
	}

	req2 := &antigravity.GeminiRequest{
		SystemInstruction: &antigravity.GeminiContent{
			Parts: []antigravity.GeminiPart{{Text: "SYS_MODIFIED"}},
		},
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "hello"}}},
		},
	}

	chain1 := BuildGeminiDigestChain(req1)
	chain2 := BuildGeminiDigestChain(req2)

	t.Logf("Chain1: %s", chain1)
	t.Logf("Chain2: %s", chain2)

	if chain1 == chain2 {
		t.Error("Different systemInstruction should produce different chains")
	}
}

func TestDigestChainTamperedMiddleContent(t *testing.T) {
	req1 := &antigravity.GeminiRequest{
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "hello"}}},
			{Role: "model", Parts: []antigravity.GeminiPart{{Text: "ORIGINAL_REPLY"}}},
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "next"}}},
		},
	}

	req2 := &antigravity.GeminiRequest{
		Contents: []antigravity.GeminiContent{
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "hello"}}},
			{Role: "model", Parts: []antigravity.GeminiPart{{Text: "TAMPERED_REPLY"}}},
			{Role: "user", Parts: []antigravity.GeminiPart{{Text: "next"}}},
		},
	}

	chain1 := BuildGeminiDigestChain(req1)
	chain2 := BuildGeminiDigestChain(req2)

	t.Logf("Chain1: %s", chain1)
	t.Logf("Chain2: %s", chain2)

	if chain1 == chain2 {
		t.Error("Tampered middle content should produce different chains")
	}

	// 验证第一个 user 的 hash 相同
	parts1 := splitChain(chain1)
	parts2 := splitChain(chain2)

	if parts1[0] != parts2[0] {
		t.Error("First user message hash should be the same")
	}
	if parts1[1] == parts2[1] {
		t.Error("Model reply hash should be different")
	}
}

func TestGenerateGeminiDigestSessionKey(t *testing.T) {
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
			want:       "gemini:digest:abcdefgh:550e8400",
		},
		{
			name:       "exactly 8 chars prefix and uuid",
			prefixHash: "12345678",
			uuid:       "abcdefgh",
			want:       "gemini:digest:12345678:abcdefgh",
		},
		{
			name:       "short hash and short uuid (less than 8)",
			prefixHash: "abc",
			uuid:       "xyz",
			want:       "gemini:digest:abc:xyz",
		},
		{
			name:       "empty hash and uuid",
			prefixHash: "",
			uuid:       "",
			want:       "gemini:digest::",
		},
		{
			name:       "normal prefix with short uuid",
			prefixHash: "abcdefgh12345678",
			uuid:       "short",
			want:       "gemini:digest:abcdefgh:short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateGeminiDigestSessionKey(tt.prefixHash, tt.uuid)
			if got != tt.want {
				t.Errorf("GenerateGeminiDigestSessionKey(%q, %q) = %q, want %q", tt.prefixHash, tt.uuid, got, tt.want)
			}
		})
	}

	// 验证确定性：相同输入产生相同输出
	t.Run("deterministic", func(t *testing.T) {
		hash := "testprefix123456"
		uuid := "test-uuid-12345"
		result1 := GenerateGeminiDigestSessionKey(hash, uuid)
		result2 := GenerateGeminiDigestSessionKey(hash, uuid)
		if result1 != result2 {
			t.Errorf("GenerateGeminiDigestSessionKey not deterministic: %s vs %s", result1, result2)
		}
	})

	// 验证不同 uuid 产生不同 sessionKey（负载均衡核心逻辑）
	t.Run("different uuid different key", func(t *testing.T) {
		hash := "sameprefix123456"
		uuid1 := "uuid0001-session-a"
		uuid2 := "uuid0002-session-b"
		result1 := GenerateGeminiDigestSessionKey(hash, uuid1)
		result2 := GenerateGeminiDigestSessionKey(hash, uuid2)
		if result1 == result2 {
			t.Errorf("Different UUIDs should produce different session keys: %s vs %s", result1, result2)
		}
	})
}

func TestGenerateGeminiContentSessionHash_StableAcrossTurns(t *testing.T) {
	const prefix = `"tools":[{"functionDeclarations":[{"name":"lookup","parameters":{"type":"object"}}]}],"systemInstruction":{"parts":[{"text":"You are Aria."}]}`
	const first = `{"role":"user","parts":[{"text":"[Story summary] Kim meets Aria."}]},{"role":"model","parts":[{"text":"Noted."}]}`
	turn1 := `{` + prefix + `,"contents":[` + first + `,{"role":"user","parts":[{"text":"guide v1"}]},{"role":"user","parts":[{"text":"msg1"},{"text":"notes v1"}]}]}`
	appended := `{` + prefix + `,"contents":[` + first + `,{"role":"user","parts":[{"text":"guide v1"}]},{"role":"user","parts":[{"text":"msg1"},{"text":"notes v1"}]},{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{}},"thoughtSignature":"c2ln"}]},{"role":"user","parts":[{"functionResponse":{"name":"lookup","response":{}}}]}]}`
	turnContext := `{` + prefix + `,"contents":[` + first + `,{"role":"user","parts":[{"text":"msg1"}]},{"role":"model","parts":[{"text":"reply1"}]},{"role":"user","parts":[{"text":"guide v2"}]},{"role":"user","parts":[{"text":"msg2"},{"text":"notes v2"}]}]}`
	edited := `{` + prefix + `,"contents":[` + first + `,{"role":"user","parts":[{"text":"guide v1"}]},{"role":"user","parts":[{"text":"msg1 edited"}]}],"generationConfig":{"temperature":0.2}}`

	want := GenerateGeminiContentSessionHash(7, "gemini-4-argon", []byte(turn1))
	if want == "" {
		t.Fatal("expected non-empty hash")
	}
	for name, body := range map[string]string{"appended": appended, "turn_context": turnContext, "edited_last": edited} {
		if got := GenerateGeminiContentSessionHash(7, "gemini-4-argon", []byte(body)); got != want {
			t.Errorf("%s: hash %q, want %q", name, got, want)
		}
	}
}

func TestGenerateGeminiContentSessionHash_SeedFormat(t *testing.T) {
	body := `{"tools":[{"googleSearch":{}}],"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
	want := DeriveSessionHashFromSeed(`gemini_cs_key=3|model=gemini-3.8-flash|tools=[{"googleSearch":{}}]|system={"parts":[{"text":"sys"}]}|first_user=[{"text":"hi"}]`)
	if got := GenerateGeminiContentSessionHash(3, " gemini-3.8-flash ", []byte(body)); got != want {
		t.Fatalf("hash %q, want %q", got, want)
	}
}

func TestGenerateGeminiContentSessionHash_DistinguishesEachSeedField(t *testing.T) {
	const (
		tools    = `"tools":[{"functionDeclarations":[{"name":"a"}]}],`
		system   = `"systemInstruction":{"parts":[{"text":"sys A"}]},`
		first    = `{"role":"user","parts":[{"text":"first A"}]}`
		trailing = `,{"role":"model","parts":[{"text":"reply"}]},{"role":"user","parts":[{"text":"tail"}]}`
	)
	base := `{` + tools + system + `"contents":[` + first + trailing + `]}`
	h := GenerateGeminiContentSessionHash(1, "m1", []byte(base))
	cases := map[string]string{
		"other_key":    GenerateGeminiContentSessionHash(2, "m1", []byte(base)),
		"other_model":  GenerateGeminiContentSessionHash(1, "m2", []byte(base)),
		"no_model":     GenerateGeminiContentSessionHash(1, "", []byte(base)),
		"other_tools":  GenerateGeminiContentSessionHash(1, "m1", []byte(`{"tools":[{"functionDeclarations":[{"name":"b"}]}],`+system+`"contents":[`+first+trailing+`]}`)),
		"no_tools":     GenerateGeminiContentSessionHash(1, "m1", []byte(`{`+system+`"contents":[`+first+trailing+`]}`)),
		"other_system": GenerateGeminiContentSessionHash(1, "m1", []byte(`{`+tools+`"systemInstruction":{"parts":[{"text":"sys B"}]},"contents":[`+first+trailing+`]}`)),
		"no_system":    GenerateGeminiContentSessionHash(1, "m1", []byte(`{`+tools+`"contents":[`+first+trailing+`]}`)),
		"other_first":  GenerateGeminiContentSessionHash(1, "m1", []byte(`{`+tools+system+`"contents":[{"role":"user","parts":[{"text":"first B"}]}`+trailing+`]}`)),
		"inline_first": GenerateGeminiContentSessionHash(1, "m1", []byte(`{`+tools+system+`"contents":[{"role":"user","parts":[{"text":"first A"},{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}]}`+trailing+`]}`)),
	}
	for name, got := range cases {
		if got == "" {
			t.Errorf("%s: expected non-empty hash", name)
		}
		if got == h {
			t.Errorf("%s: hash should differ from base", name)
		}
	}
}

func TestGenerateGeminiContentSessionHash_FirstUserSkipsLeadingModelContent(t *testing.T) {
	body := func(firstUser string) []byte {
		return []byte(`{"systemInstruction":{"parts":[{"text":"You are Aria."}]},"contents":[{"role":"model","parts":[{"text":"Hi! I'm Aria."}]},{"role":"user","parts":[{"text":"` + firstUser + `"}]},{"role":"model","parts":[{"text":"reply"}]},{"role":"user","parts":[{"text":"next"}]}]}`)
	}
	kim := GenerateGeminiContentSessionHash(1, "m", body("I'm Kim."))
	lee := GenerateGeminiContentSessionHash(1, "m", body("I'm Lee."))
	if kim == "" || lee == "" {
		t.Fatal("expected non-empty hashes")
	}
	if kim == lee {
		t.Error("conversations sharing only the leading model greeting should not share a hash")
	}

	userFirst := `{"systemInstruction":{"parts":[{"text":"You are Aria."}]},"contents":[{"role":"user","parts":[{"text":"I'm Kim."}]}]}`
	if got := GenerateGeminiContentSessionHash(1, "m", []byte(userFirst)); got != kim {
		t.Errorf("leading model content should be skipped: hash %q, want %q", got, kim)
	}
}

func TestGenerateGeminiContentSessionHash_RoleOmittedCountsAsUser(t *testing.T) {
	withRole := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
	withoutRole := `{"contents":[{"parts":[{"text":"hi"}]}]}`
	if GenerateGeminiContentSessionHash(1, "m", []byte(withoutRole)) == "" {
		t.Fatal("expected non-empty hash when role is omitted")
	}
	if GenerateGeminiContentSessionHash(1, "m", []byte(withRole)) != GenerateGeminiContentSessionHash(1, "m", []byte(withoutRole)) {
		t.Error("omitted role and explicit user role should share a hash")
	}
}

func TestGenerateGeminiContentSessionHash_NonTextPartAnchors(t *testing.T) {
	inlineOnly := `{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}]}]}`
	blankThenInline := `{"contents":[{"role":"user","parts":[{"text":" "},{"fileData":{"fileUri":"gs://b/f.pdf"}}]}]}`
	for name, body := range map[string]string{"inline_only": inlineOnly, "blank_then_file": blankThenInline} {
		if GenerateGeminiContentSessionHash(1, "m", []byte(body)) == "" {
			t.Errorf("%s: expected non-empty hash", name)
		}
	}
}

func TestGenerateGeminiContentSessionHash_NormalizesJSONFormatting(t *testing.T) {
	compact := `{"tools":[{"googleSearch":{}}],"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"first"}]}]}`
	reordered := `{ "contents" : [ { "parts" : [ { "text" : "first" } ], "role" : "user" } ], "systemInstruction" : { "parts" : [ { "text" : "sys" } ] }, "tools" : [ { "googleSearch" : { } } ] }`
	emptyToolsBase := `{"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"first"}]}]}`
	emptyTools := `{"tools":[],"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"first"}]}]}`
	if GenerateGeminiContentSessionHash(1, "m", []byte(emptyTools)) != GenerateGeminiContentSessionHash(1, "m", []byte(emptyToolsBase)) {
		t.Error("empty tools should hash like missing tools")
	}
	snakeCase := `{"tools":[{"googleSearch":{}}],"system_instruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"first"}]}]}`
	bothSystems := `{"tools":[{"googleSearch":{}}],"system_instruction":{"parts":[{"text":"other"}]},"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"first"}]}]}`

	want := GenerateGeminiContentSessionHash(1, "m", []byte(compact))
	for name, body := range map[string]string{"reordered": reordered, "snake_case_system": snakeCase, "camel_case_system_wins": bothSystems} {
		if got := GenerateGeminiContentSessionHash(1, "m", []byte(body)); got != want {
			t.Errorf("%s: hash %q, want %q", name, got, want)
		}
	}
}

func TestGenerateGeminiContentSessionHash_EmptyWithoutFirstUserContent(t *testing.T) {
	for name, body := range map[string]string{
		"empty_body":       ``,
		"non_object_body":  `[1,2]`,
		"no_contents":      `{"systemInstruction":{"parts":[{"text":"sys"}]}}`,
		"contents_object":  `{"contents":{"role":"user","parts":[{"text":"hi"}]}}`,
		"empty_contents":   `{"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[]}`,
		"only_model":       `{"contents":[{"role":"model","parts":[{"text":"Hi!"}]}]}`,
		"empty_parts":      `{"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[]}]}`,
		"missing_parts":    `{"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user"}]}`,
		"blank_text":       `{"contents":[{"role":"user","parts":[{"text":"   "}]}]}`,
		"empty_part":       `{"contents":[{"role":"user","parts":[{}]}]}`,
		"null_part":        `{"contents":[{"role":"user","parts":[null]}]}`,
		"non_object_first": `{"contents":["hello",{"role":"user","parts":[{"text":"hi"}]}]}`,
	} {
		if got := GenerateGeminiContentSessionHash(1, "m", []byte(body)); got != "" {
			t.Errorf("%s: expected empty hash, got %q", name, got)
		}
	}
}
