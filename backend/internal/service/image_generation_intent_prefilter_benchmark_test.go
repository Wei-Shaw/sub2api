package service

import (
	"strings"
	"testing"
)

// buildImageIntentPrefilterBenchmarkBody 生成形状接近 Codex 客户端的大请求体：
// tools 在 input 之前、input 内含大量 items；withImageGen 控制是否带上 Codex
// 默认注入的 image_gen namespace 声明。
func buildImageIntentPrefilterBenchmarkBody(targetBytes int, withImageGen bool) []byte {
	var builder strings.Builder
	builder.Grow(targetBytes + 512)
	_, _ = builder.WriteString(`{"model":"gpt-5.5","instructions":"`)
	_, _ = builder.WriteString(strings.Repeat("z", targetBytes/8))
	_, _ = builder.WriteString(`","tools":[{"type":"function","name":"shell","description":"`)
	_, _ = builder.WriteString(strings.Repeat("y", targetBytes/16))
	_, _ = builder.WriteString(`"}`)
	if withImageGen {
		_, _ = builder.WriteString(`,{"type":"namespace","name":"image_gen"}`)
	}
	_, _ = builder.WriteString(`],"input":[`)
	item := `{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", 4000) + `"}]}`
	for i := 0; builder.Len() < targetBytes; i++ {
		if i > 0 {
			_, _ = builder.WriteString(`,`)
		}
		_, _ = builder.WriteString(item)
	}
	_, _ = builder.WriteString(`]}`)
	return []byte(builder.String())
}

// 字节级预检查把「没有生图声明」的请求整段跳过校验与根对象遍历；
// scan_without_prefilter 是修复前的等价路径，两者同一次运行即可对比。
func BenchmarkImageGenerationIntentPrefilter(b *testing.B) {
	const targetBytes = 1 << 20
	plain := buildImageIntentPrefilterBenchmarkBody(targetBytes, false)
	codex := buildImageIntentPrefilterBenchmarkBody(targetBytes, true)

	b.Run("prefilter_only/plain", func(b *testing.B) {
		b.SetBytes(int64(len(plain)))
		b.ReportAllocs()
		for range b.N {
			if openAIBodyMayDeclareImageGeneration(plain) {
				b.Fatal("plain body must not declare image generation")
			}
		}
	})

	b.Run("scan_without_prefilter/plain", func(b *testing.B) {
		b.SetBytes(int64(len(plain)))
		b.ReportAllocs()
		for range b.N {
			if referenceImageGenerationIntent(plain) {
				b.Fatal("plain body must not have image intent")
			}
		}
	})

	b.Run("intent/plain", func(b *testing.B) {
		b.SetBytes(int64(len(plain)))
		b.ReportAllocs()
		for range b.N {
			if IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.5", plain) {
				b.Fatal("plain body must not have image intent")
			}
		}
	})

	b.Run("scan_without_prefilter/codex", func(b *testing.B) {
		b.SetBytes(int64(len(codex)))
		b.ReportAllocs()
		for range b.N {
			if !referenceImageGenerationIntent(codex) {
				b.Fatal("codex namespace body must have image intent")
			}
		}
	})

	// Codex 在普通请求里就带 image_gen namespace（见 IsImageGenerationIntentForPlatform
	// 注释），预检查必然放行，这类流量仍需完整扫描，本项用于跟踪它的成本。
	b.Run("intent/codex_namespace", func(b *testing.B) {
		b.SetBytes(int64(len(codex)))
		b.ReportAllocs()
		for range b.N {
			if !IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.5", codex) {
				b.Fatal("codex namespace body must have image intent")
			}
		}
	})
}
