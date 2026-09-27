//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// buildBodyParseBenchBody 构造约 200KB、带 tools 与已签名 thinking 的 Claude Code 风格请求体。
// unsignedLast 为 true 时最后一个 assistant thinking 缺少签名（需要过滤）。
func buildBodyParseBenchBody(unsignedLast bool) []byte {
	const turns = 64
	var sb strings.Builder
	sb.WriteString(`{"model":"claude-sonnet-4-5","max_tokens":32000,"stream":true,"thinking":{"type":"enabled","budget_tokens":16000},"metadata":{"user_id":"user_bench_account__session_bench"},"system":[{"type":"text","text":"You are Claude Code.","cache_control":{"type":"ephemeral"}}],"tools":[`)
	for i := 0; i < 20; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"name":"tool_%d","description":"%s","input_schema":{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path"]}}`, i, strings.Repeat("describe the tool ", 20))
	}
	sb.WriteString(`],"messages":[`)
	for i := 0; i < turns; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		signature := fmt.Sprintf("EqQBCkYIBxgCKkBbenchsignature%04d", i)
		if unsignedLast && i == turns-1 {
			signature = ""
		}
		fmt.Fprintf(&sb, `{"role":"user","content":[{"type":"text","text":"%s"}]},`, strings.Repeat("please edit the file ", 40))
		fmt.Fprintf(&sb, `{"role":"assistant","content":[{"type":"thinking","thinking":"%s","signature":"%s"},{"type":"text","text":"%s"},{"type":"tool_use","id":"toolu_%d","name":"tool_%d","input":{"path":"/src/file_%d.go"}}]},`,
			strings.Repeat("let me think about it ", 40), signature, strings.Repeat("sure ", 40), i, i%20, i)
		fmt.Fprintf(&sb, `{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_%d","content":"%s"}]}`, i, strings.Repeat("file content line ", 30))
	}
	sb.WriteString(`]}`)
	return []byte(sb.String())
}

func BenchmarkReplaceBody(b *testing.B) {
	body := buildBodyParseBenchBody(false)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("same_slice", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		for i := 0; i < b.N; i++ {
			if err := parsed.ReplaceBody(parsed.Body.Bytes()); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("clone_same_slice", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		for i := 0; i < b.N; i++ {
			if _, err := parsed.CloneForBody(body); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestReplaceBody_SameSliceSkipsReparse(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","speed":"FAST","stream":true,"max_tokens":1024,"metadata":{"user_id":"u1"},"system":"sys","messages":[{"role":"user","content":"hi"}]}`)
	golden := append([]byte(nil), body...)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
	require.NoError(t, err)
	want := *parsed

	allocs := testing.AllocsPerRun(20, func() {
		if err := parsed.ReplaceBody(parsed.Body.Bytes()); err != nil {
			panic(err)
		}
	})
	require.Zero(t, allocs, "an unchanged body must not be re-parsed")
	require.Equal(t, golden, parsed.Body.Bytes())
	require.Same(t, &body[0], &parsed.Body.Bytes()[0])
	require.Equal(t, want.Model, parsed.Model)
	require.Equal(t, want.Stream, parsed.Stream)
	require.Equal(t, "fast", parsed.Speed)
	require.Equal(t, want.MaxTokens, parsed.MaxTokens)
	require.Equal(t, want.MetadataUserID, parsed.MetadataUserID)
	require.Equal(t, `"sys"`, string(parsed.SystemRaw()))
	require.Equal(t, `[{"role":"user","content":"hi"}]`, string(parsed.MessagesRaw()))

	// 不同切片（哪怕只改一个字段）仍然重新解析。
	edited := bytes.Replace(body, []byte(`"stream":true`), []byte(`"stream":false`), 1)
	require.NoError(t, parsed.ReplaceBody(edited))
	require.False(t, parsed.Stream)
	require.Equal(t, `[{"role":"user","content":"hi"}]`, string(parsed.MessagesRaw()))
}

func TestCloneForBody_SameSliceSkipsReparse(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","speed":"FAST","stream":true,"system":"sys","messages":[{"role":"user","content":"hi"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
	require.NoError(t, err)
	parsed.OnUpstreamAccepted = func() {}

	clone, err := parsed.CloneForBody(body)
	require.NoError(t, err)
	require.NotSame(t, parsed.Body, clone.Body)
	require.Nil(t, clone.OnUpstreamAccepted)
	require.Equal(t, parsed.Model, clone.Model)
	require.Equal(t, "fast", clone.Speed)
	require.True(t, clone.Stream)
	require.Equal(t, `"sys"`, string(clone.SystemRaw()))
	require.Equal(t, `[{"role":"user","content":"hi"}]`, string(clone.MessagesRaw()))

	copied := append([]byte(nil), body...)
	sameAllocs := testing.AllocsPerRun(20, func() { _, _ = parsed.CloneForBody(body) })
	copyAllocs := testing.AllocsPerRun(20, func() { _, _ = parsed.CloneForBody(copied) })
	require.Less(t, sameAllocs, copyAllocs, "cloning onto the same body slice must skip the re-parse")
}

func BenchmarkFilterThinkingBlocks(b *testing.B) {
	for _, tc := range []struct {
		name         string
		unsignedLast bool
	}{
		{name: "signed_unchanged"},
		{name: "one_unsigned_filtered", unsignedLast: true},
	} {
		body := buildBodyParseBenchBody(tc.unsignedLast)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				_ = FilterThinkingBlocks(body, "claude-sonnet-4-5")
			}
		})
	}
}

// legacyFilterThinkingBlocks 是加入 gjson 预扫描之前 filterThinkingBlocksInternal 的 map 路径，
// 作为等价性对照。
func legacyFilterThinkingBlocks(body []byte, alwaysThinking bool) []byte {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}
	thinkingEnabled := alwaysThinking
	if thinking, ok := req["thinking"].(map[string]any); ok {
		if thinkType, ok := thinking["type"].(string); ok && (thinkType == "enabled" || thinkType == "adaptive") {
			thinkingEnabled = true
		}
	}
	messages, ok := req["messages"].([]any)
	if !ok {
		return body
	}
	filtered := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		content, ok := msgMap["content"].([]any)
		if !ok {
			continue
		}
		newContent := make([]any, 0, len(content))
		filteredThisMessage := false
		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok {
				newContent = append(newContent, block)
				continue
			}
			blockType, _ := blockMap["type"].(string)
			if blockType == "thinking" || blockType == "redacted_thinking" {
				if thinkingEnabled && role == "assistant" {
					if alwaysThinking && blockType == "redacted_thinking" {
						if data, ok := blockMap["data"].(string); ok && data != "" {
							newContent = append(newContent, block)
							continue
						}
					}
					signature, _ := blockMap["signature"].(string)
					if signature != "" && signature != antigravity.DummyThoughtSignature {
						newContent = append(newContent, block)
						continue
					}
				}
				filtered = true
				filteredThisMessage = true
				continue
			}
			if blockType == "" {
				if _, hasThinking := blockMap["thinking"]; hasThinking {
					filtered = true
					filteredThisMessage = true
					continue
				}
			}
			newContent = append(newContent, block)
		}
		if filteredThisMessage {
			msgMap["content"] = newContent
		}
	}
	if !filtered {
		return body
	}
	newBody, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return newBody
}

func TestFilterThinkingBlocksInternal_MatchesLegacyMapPath(t *testing.T) {
	const on = `"thinking":{"type":"enabled","budget_tokens":1024},`
	signed := `{"type":"thinking","thinking":"t","signature":"sig"}`
	cases := map[string]string{
		"signed kept":                  `{` + on + `"messages":[{"role":"assistant","content":[` + signed + `,{"type":"text","text":"a"}]}]}`,
		"adaptive signed kept":         `{"thinking":{"type":"adaptive"},"messages":[{"role":"assistant","content":[` + signed + `]}]}`,
		"escaped enabled":              `{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[` + signed + `]}]}`,
		"missing signature":            `{` + on + `"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t"},{"type":"text","text":"a"}]}]}`,
		"empty signature":              `{` + on + `"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":""}]}]}`,
		"non-string signature":         `{` + on + `"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":123}]}]}`,
		"dummy signature":              `{` + on + `"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"skip_thought_signature_validator"}]}]}`,
		"thinking disabled":            `{"thinking":{"type":"disabled"},"messages":[{"role":"assistant","content":[` + signed + `]}]}`,
		"no top-level thinking":        `{"messages":[{"role":"assistant","content":[` + signed + `]}]}`,
		"thinking in user message":     `{` + on + `"messages":[{"role":"user","content":[` + signed + `]}]}`,
		"redacted with data":           `{` + on + `"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"d"}]}]}`,
		"redacted signed":              `{` + on + `"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"d","signature":"sig"}]}]}`,
		"untyped thinking key":         `{` + on + `"messages":[{"role":"assistant","content":[{"thinking":"t","signature":"sig"}]}]}`,
		"non-string type":              `{` + on + `"messages":[{"role":"assistant","content":[{"type":1,"thinking":"t"}]}]}`,
		"text block only":              `{` + on + `"messages":[{"role":"assistant","content":[{"type":"text","text":"a","thinking":"x"}]}]}`,
		"duplicate block type":         `{` + on + `"messages":[{"role":"assistant","content":[{"type":"text","type":"thinking","thinking":"t"}]}]}`,
		"duplicate signature":          `{` + on + `"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"sig","signature":""}]}]}`,
		"duplicate top-level thinking": `{` + on + `"messages":[{"role":"assistant","content":[` + signed + `]}],"thinking":{"type":"disabled"}}`,
		"duplicate messages last ok":   `{` + on + `"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t"}]}],"messages":[{"role":"assistant","content":[` + signed + `]}]}`,
		"duplicate messages last bad":  `{` + on + `"messages":[{"role":"assistant","content":[` + signed + `]}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t"}]}]}`,
		"duplicate role":               `{` + on + `"messages":[{"role":"user","role":"assistant","content":[` + signed + `]}]}`,
		"escaped key":                  `{` + on + `"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"t"}]}]}`,
		"pretty printed":               "{\n  \"thinking\" : {\"type\" : \"enabled\"},\n  \"messages\" : [ { \"role\" : \"assistant\", \"content\" : [ { \"type\" : \"thinking\", \"thinking\" : \"t\" } ] } ]\n}",
		"string content":               `{` + on + `"messages":[{"role":"assistant","content":"thinking"}]}`,
		"non-object entries":           `{` + on + `"messages":[null,"x",{"role":"assistant","content":[null,"thinking",` + signed + `]}]}`,
		"messages not array":           `{` + on + `"messages":{"role":"assistant"}}`,
		"invalid json":                 `{` + on + `"messages":[{"role":"assistant","content":[{"type":"thinking"}]}`,
		"top-level array":              `[{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking"}]}]}]`,
	}
	for name, raw := range cases {
		for _, alwaysThinking := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/always=%v", name, alwaysThinking), func(t *testing.T) {
				body := []byte(raw)
				want := legacyFilterThinkingBlocks(body, alwaysThinking)
				got := filterThinkingBlocksInternal(body, alwaysThinking)
				require.Equal(t, string(want), string(got))
				require.Equal(t, sameBodySlice(body, want), sameBodySlice(body, got), "unchanged bodies must be returned as the same slice")
			})
		}
	}
}

func TestFilterThinkingBlocks_SignedBodySkipsMapDecode(t *testing.T) {
	body := buildBodyParseBenchBody(false)
	require.Same(t, &body[0], &FilterThinkingBlocks(body, "claude-sonnet-4-5")[0])
	allocs := testing.AllocsPerRun(5, func() { _ = FilterThinkingBlocks(body, "claude-sonnet-4-5") })
	require.Zero(t, allocs, "signed thinking history must not be decoded into a map")

	unsigned := buildBodyParseBenchBody(true)
	require.Equal(t, string(legacyFilterThinkingBlocks(unsigned, false)), string(FilterThinkingBlocks(unsigned, "claude-sonnet-4-5")))
	require.NotEqual(t, string(unsigned), string(FilterThinkingBlocks(unsigned, "claude-sonnet-4-5")))
}

func BenchmarkRestoreToolNames(b *testing.B) {
	chunk := []byte(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"` + strings.Repeat(`{\"path\":\"/src/main.go\"} `, 36) + `"}}`)
	rw := &ToolNameRewrite{ReverseOrdered: [][2]string{
		{"analyze_a1b2c3", "read_file"},
		{"compute_d4e5f6", "write_file"},
		{"fetch_g7h8i9", "list_dir"},
	}}
	b.Run("no_match", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(chunk)))
		for i := 0; i < b.N; i++ {
			_ = restoreToolNamesInBytes(chunk, rw)
		}
	})
	matched := append(append([]byte(nil), chunk[:len(chunk)-3]...), []byte(`fetch_g7h8i9"}}`)...)
	b.Run("match", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(matched)))
		for i := 0; i < b.N; i++ {
			_ = restoreToolNamesInBytes(matched, rw)
		}
	})
}

func TestRestoreToolNamesInBytes_NoMatchDoesNotCopyChunk(t *testing.T) {
	chunk := []byte(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"` + strings.Repeat(`{\"path\":\"/src/main.go\"} `, 36) + `"}}`)
	rw := &ToolNameRewrite{ReverseOrdered: [][2]string{{"fetch_g7h8i9", "list_dir"}}}

	require.Same(t, &chunk[0], &restoreToolNamesInBytes(chunk, rw)[0])
	allocs := testing.AllocsPerRun(20, func() { _ = restoreToolNamesInBytes(chunk, rw) })
	require.Zero(t, allocs, "a chunk without fake tool names must not be copied")

	matched := []byte(`{"name":"fetch_g7h8i9","other":"cc_sess_x","more":"cc_ses_y","again":"fetch_g7h8i9"}`)
	golden := append([]byte(nil), matched...)
	require.Equal(t, `{"name":"list_dir","other":"sessions_x","more":"session_y","again":"list_dir"}`, string(restoreToolNamesInBytes(matched, rw)))
	require.Equal(t, golden, matched, "the input chunk must not be modified in place")
}

func BenchmarkChatCompletionsPayload(b *testing.B) {
	gin.SetMode(gin.TestMode)
	var sb strings.Builder
	sb.WriteString("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_bench\",\"model\":\"gpt-5.4\",\"status\":\"in_progress\"}}\n\n")
	for i := 0; i < 2000; i++ {
		sb.WriteString("data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_bench\",\"output_index\":0,\"content_index\":0,\"delta\":\"token \"}\n\n")
	}
	sb.WriteString("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_bench\",\"model\":\"gpt-5.4\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2000,\"total_tokens\":2010}}}\n\n")
	stream := []byte(sb.String())
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Name: "openai-bench", Platform: PlatformOpenAI}

	b.ReportAllocs()
	b.SetBytes(int64(len(stream)))
	for i := 0; i < b.N; i++ {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewReader(stream)),
		}
		if _, err := svc.handleChatStreamingResponse(resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now(), 0); err != nil {
			b.Fatal(err)
		}
	}
}

func TestForwardAsChatCompletions_APIKeyPromptCacheKeyPreservesBodyOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":[{"role":"user","content":[{"type":"input_text","text":"a<b"}]}],"stream":false}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 99})
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 2, Name: "openai-compatible", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-compatible"},
		Extra:       map[string]any{"openai_responses_supported": true},
	}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "cache-key-1", "gpt-5.4")
	require.Error(t, err)
	sent := string(upstream.lastBody)
	require.Equal(t, "cache-key-1", gjson.Get(sent, "prompt_cache_key").String())
	// 注入 prompt_cache_key 不应重编码整个 body：原有 key 顺序与原始字符保持不变。
	require.Less(t, strings.Index(sent, `"model"`), strings.Index(sent, `"input"`))
	require.Contains(t, sent, `"text":"a<b"`)
}

// geminiDeltaBenchChunks 生成 n 个互不为前缀的 64 字节 delta 模式文本块。
func geminiDeltaBenchChunks(n int) []string {
	chunks := make([]string, n)
	for i := range chunks {
		chunks[i] = fmt.Sprintf("chunk-%05d %s", i, strings.Repeat("x", 52))
	}
	return chunks
}

// runGeminiTextDeltaStream 用 computeGeminiTextDelta 处理一整段流，返回输出的 delta 总字节数。
func runGeminiTextDeltaStream(chunks []string) int {
	var seen geminiSeenText
	total := 0
	for _, chunk := range chunks {
		total += len(computeGeminiTextDelta(&seen, chunk))
	}
	return total
}

// legacyComputeGeminiTextDelta 是改为追加写入之前的实现，作为等价性对照。
func legacyComputeGeminiTextDelta(seen, incoming string) (delta, newSeen string) {
	incoming = strings.TrimSuffix(incoming, "\u0000")
	if incoming == "" {
		return "", seen
	}
	if strings.HasPrefix(incoming, seen) {
		return strings.TrimPrefix(incoming, seen), incoming
	}
	if strings.HasPrefix(seen, incoming) {
		return "", seen
	}
	return incoming, seen + incoming
}

func TestComputeGeminiTextDelta_MatchesLegacy(t *testing.T) {
	sequences := map[string][]string{
		"cumulative":            {"He", "Hello", "Hello wor", "Hello world"},
		"delta":                 {"He", "llo", " wor", "ld"},
		"repeated prefix delta": {"ab", "ab", "abab", "b", "ab", "aba"},
		"duplicate and rewind":  {"Hello world", "Hello", "Hello world", "Hello world!"},
		"nul suffix and empty":  {"abc\u0000", "", "\u0000", "abcdef\u0000", "gh"},
		"multibyte delta":       {"你", "好", "，世界", "🙂", "é"},
		"multibyte cumulative":  {"你", "你好", "你好，世界", "你好，世界🙂"},
		"mode switches":         {"a", "b", "abc", "d", "abcde", "abcdef", "x", "y", "abcdefxyz"},
		"tool json":             {`{"a":`, `{"a":1}`, `,"b":2}`, `{}`},
	}
	for name, chunks := range sequences {
		t.Run(name, func(t *testing.T) {
			var seen geminiSeenText
			for round := 0; round < 2; round++ {
				seen = geminiSeenText{} // 与调用方关闭 tool block 时的重置方式一致
				legacySeen := ""
				for i, chunk := range chunks {
					var want string
					want, legacySeen = legacyComputeGeminiTextDelta(legacySeen, chunk)
					require.Equal(t, want, computeGeminiTextDelta(&seen, chunk), "round %d chunk %d", round, i)
					require.Equal(t, legacySeen, seen.text, "round %d chunk %d", round, i)
				}
			}
		})
	}
}

func TestComputeGeminiTextDelta_DeltaModeAppendsInsteadOfRecopying(t *testing.T) {
	chunks := geminiDeltaBenchChunks(1000)
	allocs := testing.AllocsPerRun(5, func() { _ = runGeminiTextDeltaStream(chunks) })
	legacyAllocs := testing.AllocsPerRun(5, func() {
		seen := ""
		for _, chunk := range chunks {
			_, seen = legacyComputeGeminiTextDelta(seen, chunk)
		}
	})
	// 旧实现每个 chunk 都重新拼接 seen+incoming（每 chunk 一次分配、O(n²) 拷贝）；追加写入只随容量增长分配。
	require.GreaterOrEqual(t, legacyAllocs, float64(len(chunks)-1))
	require.Less(t, allocs, float64(100))
}

func BenchmarkGeminiTextDelta(b *testing.B) {
	for _, n := range []int{500, 1000, 2000} {
		chunks := geminiDeltaBenchChunks(n)
		b.Run(fmt.Sprintf("delta_chunks_%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if runGeminiTextDeltaStream(chunks) != n*64 {
					b.Fatal("unexpected delta size")
				}
			}
		})
	}
}
