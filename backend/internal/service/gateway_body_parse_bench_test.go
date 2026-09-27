//go:build unit

package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/gin-gonic/gin"
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
	seen := ""
	total := 0
	for _, chunk := range chunks {
		var delta string
		delta, seen = computeGeminiTextDelta(seen, chunk)
		total += len(delta)
	}
	return total
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
