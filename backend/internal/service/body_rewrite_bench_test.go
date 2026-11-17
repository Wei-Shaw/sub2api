package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// benchmarkResponsesNamespaceBody builds a Codex-sized request: 20 namespaced
// tool declarations plus ~800 KiB of history that the whole-body rewrite used
// to decode into map[string]any for every request.
func benchmarkResponsesNamespaceBody(tb testing.TB) []byte {
	tb.Helper()
	var b strings.Builder
	_, _ = b.WriteString(`{"model":"gpt-6.1","tools":[`)
	for i := 0; i < 20; i++ {
		if i > 0 {
			_ = b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"type":"namespace","name":"ns%d","tools":[`+
			`{"type":"function","name":"tool_a","parameters":{"type":"object","properties":{"q":{"type":"string"}}}},`+
			`{"type":"function","name":"tool_b","parameters":{"type":"object"}}]}`, i)
	}
	_, _ = b.WriteString(`],"input":[`)
	history := strings.Repeat("x", 2048)
	for i := 0; i < 400; i++ {
		if i > 0 {
			_ = b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"type":"function_call","namespace":"ns%d","name":"tool_a","call_id":"c%d","arguments":"{}"},`+
			`{"type":"message","role":"assistant","content":[{"type":"output_text","text":%q}]}`, i%20, i, history)
	}
	_, _ = b.WriteString(`],"tool_choice":{"type":"function_call","namespace":"ns3","name":"tool_a"}}`)
	return []byte(b.String())
}

func BenchmarkFlattenOpenAIResponsesNamespaces(b *testing.B) {
	gin.SetMode(gin.TestMode)
	body := benchmarkResponsesNamespaceBody(b)
	b.Logf("body size: %d bytes", len(body))

	b.Run("legacy_whole_body", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			if _, err := legacyFlattenOpenAIResponsesNamespaces(ctx, body); err != nil {
				b.Fatalf("flatten: %v", err)
			}
		}
	})

	b.Run("span_edits", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			if _, err := flattenOpenAIResponsesNamespaces(ctx, body); err != nil {
				b.Fatalf("flatten: %v", err)
			}
		}
	})
}

// benchmarkCacheControlBody builds 200 cache_control-carrying blocks, the shape
// that made the per-path sjson loops copy the whole body once per block.
func benchmarkCacheControlBody(tb testing.TB) []byte {
	tb.Helper()
	text := strings.Repeat("y", 512)
	var b strings.Builder
	_, _ = b.WriteString(`{"model":"claude-sonnet-5-5","system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"}}],"tools":[`)
	for i := 0; i < 8; i++ {
		if i > 0 {
			_ = b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"name":"t%d","cache_control":{"type":"ephemeral","ttl":"5m"}}`, i)
	}
	_, _ = b.WriteString(`],"messages":[`)
	for i := 0; i < 200; i++ {
		if i > 0 {
			_ = b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"role":"user","content":[{"type":"text","text":%q,"cache_control":{"type":"ephemeral","ttl":"5m"}}]}`, text)
	}
	_, _ = b.WriteString(`]}`)
	return []byte(b.String())
}

func BenchmarkForceEphemeralCacheControlTTL(b *testing.B) {
	body := benchmarkCacheControlBody(b)
	b.Logf("body size: %d bytes", len(body))

	b.Run("legacy_per_path", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			legacyForceEphemeralCacheControlTTL(body, "1h")
		}
	})

	b.Run("span_edits", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			forceEphemeralCacheControlTTL(body, "1h")
		}
	})
}

func BenchmarkStripMessageCacheControl(b *testing.B) {
	body := benchmarkCacheControlBody(b)
	b.Logf("body size: %d bytes", len(body))

	b.Run("legacy_per_path", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			legacyStripMessageCacheControl(body)
		}
	})

	b.Run("span_edits", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			stripMessageCacheControl(body)
		}
	})
}
