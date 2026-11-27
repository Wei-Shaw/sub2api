package service

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// claudeCodeLikeBody builds a ~32KB request shaped like a Claude Code turn:
// 20 tools, a system prompt, and 40 messages with 4 content blocks each.
func claudeCodeLikeBody(cacheControls int, with1h bool) []byte {
	return claudeCodeLikeBodyWith(claudeCodeLikeBodyOptions{cacheControls: cacheControls, with1h: with1h})
}

// claudeCodeLikeBodyWithThinking builds the same shape with extended thinking
// enabled: the top-level thinking config plus a thinking block in the last
// assistant turn. Either occurrence makes the byte-level precheck in
// enforceCacheControlLimit inapplicable, so this is the shape that decides
// whether the cache_control walks can be skipped for real Claude Code traffic.
func claudeCodeLikeBodyWithThinking(with1h bool) []byte {
	return claudeCodeLikeBodyWith(claudeCodeLikeBodyOptions{cacheControls: 4, with1h: with1h, thinking: true})
}

type claudeCodeLikeBodyOptions struct {
	cacheControls int
	with1h        bool
	thinking      bool
}

func claudeCodeLikeBodyWith(opts claudeCodeLikeBodyOptions) []byte {
	cacheControls := opts.cacheControls
	var b strings.Builder
	_, _ = b.WriteString(`{"model":"claude-sonnet-5-5","max_tokens":32000,"stream":true`)
	if opts.thinking {
		_, _ = b.WriteString(`,"thinking":{"type":"enabled","budget_tokens":31999}`)
	}
	_, _ = b.WriteString(`,"tools":[`)
	for i := 0; i < 20; i++ {
		if i > 0 {
			_ = b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"name":"tool_%d","description":"%s","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}`, i, strings.Repeat("d", 40))
		if i == 19 && cacheControls > 0 {
			_, _ = b.WriteString(`,"cache_control":{"type":"ephemeral"}`)
			cacheControls--
		}
		_ = b.WriteByte('}')
	}
	_, _ = b.WriteString(`],"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."`)
	if cacheControls > 0 {
		_, _ = b.WriteString(`,"cache_control":{"type":"ephemeral"}`)
		cacheControls--
	}
	_, _ = b.WriteString(`}],"messages":[`)
	for m := 0; m < 40; m++ {
		if m > 0 {
			_ = b.WriteByte(',')
		}
		role := "user"
		if m%2 == 1 {
			role = "assistant"
		}
		fmt.Fprintf(&b, `{"role":%q,"content":[`, role)
		for c := 0; c < 4; c++ {
			if c > 0 {
				_ = b.WriteByte(',')
			}
			if opts.thinking && m == 39 && c == 0 {
				_, _ = b.WriteString(`{"type":"thinking","thinking":"` + strings.Repeat("t", 80) + `","signature":"` + strings.Repeat("s", 64) + `"}`)
				continue
			}
			fmt.Fprintf(&b, `{"type":"text","text":"%s"`, strings.Repeat("x", 120))
			if m >= 38 && c == 3 && cacheControls > 0 {
				ttl := ""
				if opts.with1h && m == 39 {
					ttl = `,"ttl":"1h"`
				}
				fmt.Fprintf(&b, `,"cache_control":{"type":"ephemeral"%s}`, ttl)
				cacheControls--
			}
			_ = b.WriteByte('}')
		}
		_, _ = b.WriteString(`]}`)
	}
	_, _ = b.WriteString(`]}`)
	return []byte(b.String())
}

func BenchmarkEnforceCacheControlLimit(b *testing.B) {
	cases := []struct {
		name string
		body []byte
	}{
		{"common_4cc_no1h", claudeCodeLikeBody(4, false)},
		{"with_1h", claudeCodeLikeBody(4, true)},
		{"thinking_4cc_no1h", claudeCodeLikeBodyWithThinking(false)},
		{"thinking_4cc_1h", claudeCodeLikeBodyWithThinking(true)},
	}
	for _, tc := range cases {
		b.Run(tc.name+"/legacy", func(b *testing.B) {
			b.SetBytes(int64(len(tc.body)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = legacyEnforceCacheControlLimit(tc.body)
			}
		})
		b.Run(tc.name+"/optimized", func(b *testing.B) {
			b.SetBytes(int64(len(tc.body)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = enforceCacheControlLimit(tc.body)
			}
		})
	}
}

func BenchmarkRequestToolHelpers(b *testing.B) {
	body := claudeCodeLikeBody(4, false)
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = legacyRequestNeedsBetaFeatures(body)
			_ = legacyFilterSonnet55ToolsetBeta("fine-grained-tool-streaming-2025-05-14", body, "claude-sonnet-5-5")
			_ = legacyIsOnlyWebSearchToolInBody(body)
			_ = legacyBuildToolNameRewriteFromBody(body)
			_ = legacyApplyToolsLastCacheBreakpoint(body)
			_ = legacyExtractSystemPreviewFromBody(body)
		}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = requestNeedsBetaFeatures(body)
			_ = filterSonnet55ToolsetBeta("fine-grained-tool-streaming-2025-05-14", body, "claude-sonnet-5-5")
			_ = isOnlyWebSearchToolInBody(body)
			_ = buildToolNameRewriteFromBody(body)
			_ = applyToolsLastCacheBreakpoint(body)
			_ = extractSystemPreviewFromBody(body)
		}
	})
}

func BenchmarkClaudeMimicDebugLine(b *testing.B) {
	req, _ := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)
	for _, h := range []string{"User-Agent", "X-App", "Anthropic-Version", "Anthropic-Beta", "X-Stainless-Lang", "X-Stainless-Os", "Authorization", "Content-Type", "Accept"} {
		req.Header.Set(h, "value-"+h)
	}
	body := claudeCodeLikeBody(4, false)
	account := &Account{ID: 1, Name: "acc"}
	b.Run("eager", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = buildClaudeMimicDebugLine(req, body, account, "oauth", true)
		}
	})
	b.Run("lazy_unused", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = newClaudeMimicDebugLine(req, body, account, "oauth", true)
		}
	})
}
