package service

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

var (
	benchmarkStringSink string
	benchmarkIntSink    int
)

// BenchmarkGenerateSessionHash_Metadata 关注 JSON 解析与正则匹配开销。
func BenchmarkGenerateSessionHash_Metadata(b *testing.B) {
	svc := &GatewayService{}
	body := []byte(`{"metadata":{"user_id":"session_123e4567-e89b-12d3-a456-426614174000"},"messages":[{"content":"hello"}]}`)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
		if err != nil {
			b.Fatalf("解析请求失败: %v", err)
		}
		benchmarkStringSink = svc.GenerateSessionHash(parsed)
	}
}

func BenchmarkParseGatewayRequest_LargeAnthropicMessages(b *testing.B) {
	for _, size := range benchmarkBodySizes() {
		b.Run(size.name, func(b *testing.B) {
			body := buildLargeAnthropicMessagesBody(size.bytes, false)

			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
				if err != nil {
					b.Fatalf("解析 Anthropic 请求失败: %v", err)
				}
				benchmarkIntSink = len(parsed.MessagesRaw())
			}
		})
	}
}

func BenchmarkParseGatewayRequest_LargeGeminiContents(b *testing.B) {
	for _, size := range benchmarkBodySizes() {
		b.Run(size.name, func(b *testing.B) {
			body := buildLargeGeminiContentsBody(size.bytes)

			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformGemini)
				if err != nil {
					b.Fatalf("解析 Gemini 请求失败: %v", err)
				}
				benchmarkIntSink = len(parsed.MessagesRaw())
			}
		})
	}
}

func BenchmarkGenerateSessionHash_LargeAnthropicMessages(b *testing.B) {
	svc := &GatewayService{}
	for _, size := range benchmarkBodySizes() {
		b.Run(size.name, func(b *testing.B) {
			body := buildLargeAnthropicMessagesBody(size.bytes, true)
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
			if err != nil {
				b.Fatalf("解析请求失败: %v", err)
			}

			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkStringSink = svc.GenerateSessionHash(parsed)
			}
		})
	}
}

func BenchmarkOpenAIResponses_LargeInputMeta(b *testing.B) {
	for _, size := range benchmarkBodySizes() {
		b.Run(size.name, func(b *testing.B) {
			body := buildLargeOpenAIResponsesBody(size.bytes)

			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				model, stream, promptCacheKey := extractOpenAIRequestMetaFromBody(body)
				benchmarkStringSink = model + promptCacheKey
				if stream {
					benchmarkIntSink++
				}
			}
		})
	}
}

func BenchmarkOpenAIResponses_LargeInputDecodeMap(b *testing.B) {
	for _, size := range benchmarkBodySizes() {
		b.Run(size.name, func(b *testing.B) {
			body := buildLargeOpenAIResponsesBody(size.bytes)

			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reqBody, err := getOpenAIRequestBodyMap(nil, body)
				if err != nil {
					b.Fatalf("解析 OpenAI 请求失败: %v", err)
				}
				benchmarkIntSink = len(reqBody)
			}
		})
	}
}

func BenchmarkOpenAIResponses_LargeInputRawPatch(b *testing.B) {
	for _, size := range benchmarkBodySizes() {
		b.Run(size.name, func(b *testing.B) {
			body := buildLargeOpenAIResponsesBody(size.bytes)

			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				view := newOpenAIRequestView(body)
				view.MarkPatchSet("instructions", "You are a helpful coding assistant.")
				view.MarkPatchSet("reasoning.effort", "none")
				patched, err := view.ApplyPatches()
				if err != nil {
					b.Fatalf("应用 OpenAI raw patch 失败: %v", err)
				}
				benchmarkIntSink = len(patched)
			}
		})
	}
}

func BenchmarkOpenAIResponses_LargeInputImageBillingRaw(b *testing.B) {
	for _, size := range benchmarkBodySizes() {
		b.Run(size.name, func(b *testing.B) {
			body := buildLargeOpenAIResponsesImageToolBody(size.bytes)

			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cfg, err := resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body, "gpt-5.4")
				if err != nil {
					b.Fatalf("解析 OpenAI 图片计费配置失败: %v", err)
				}
				benchmarkStringSink = cfg.Model + cfg.SizeTier + cfg.InputSize
			}
		})
	}
}

func BenchmarkOpenAIResponses_LargeInputEmptyBase64Guard(b *testing.B) {
	for _, size := range benchmarkBodySizes() {
		b.Run(size.name, func(b *testing.B) {
			body := buildLargeOpenAIResponsesBody(size.bytes)

			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if openAIRequestBodyMayContainEmptyBase64InputImage(body) {
					benchmarkIntSink++
				}
			}
		})
	}
}

func BenchmarkOpenAIResponses_LargeInputFunctionCallValidation(b *testing.B) {
	for _, size := range benchmarkBodySizes() {
		b.Run(size.name, func(b *testing.B) {
			body := buildLargeOpenAIResponsesToolContinuationBody(size.bytes)

			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				validation := ValidateFunctionCallOutputContextBytes(body)
				if !validation.HasFunctionCallOutput || !validation.HasItemReferenceForAllCallIDs {
					b.Fatalf("工具续链校验结果异常: %+v", validation)
				}
				benchmarkIntSink++
			}
		})
	}
}

// BenchmarkExtractCacheableContent_System 关注字符串拼接路径的性能。
func BenchmarkExtractCacheableContent_System(b *testing.B) {
	svc := &GatewayService{}
	req := buildSystemCacheableRequest(12)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchmarkStringSink = svc.extractCacheableContent(req)
	}
}

func benchmarkBodySizes() []struct {
	name  string
	bytes int
} {
	return []struct {
		name  string
		bytes int
	}{
		{name: "4MB", bytes: 4 << 20},
		{name: "8MB", bytes: 8 << 20},
		{name: "16MB", bytes: 16 << 20},
		{name: "32MB", bytes: 32 << 20},
	}
}

func buildSystemCacheableRequest(parts int) *ParsedRequest {
	var builder strings.Builder
	_, _ = builder.WriteString(`{"system":[`)
	for i := 0; i < parts; i++ {
		if i > 0 {
			_ = builder.WriteByte(',')
		}
		_, _ = builder.WriteString(`{"text":"system_part_`)
		_, _ = builder.WriteString(strconv.Itoa(i))
		_, _ = builder.WriteString(`","cache_control":{"type":"ephemeral"}}`)
	}
	_, _ = builder.WriteString(`]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(builder.String())), "")
	if err != nil {
		panic(err)
	}
	return parsed
}

func buildLargeAnthropicMessagesBody(targetBytes int, includeCacheControl bool) []byte {
	var builder strings.Builder
	builder.Grow(targetBytes + 1024)
	_, _ = builder.WriteString(`{"model":"claude-sonnet-4-5","stream":true,"system":[{"type":"text","text":"system seed"}],"messages":[`)
	for i := 0; builder.Len() < targetBytes; i++ {
		if i > 0 {
			_ = builder.WriteByte(',')
		}
		_, _ = builder.WriteString(`{"role":"user","content":[{"type":"text","text":"`)
		_, _ = builder.WriteString(strings.Repeat("anthropic payload ", 64))
		_, _ = builder.WriteString(strconv.Itoa(i))
		_ = builder.WriteByte('"')
		if includeCacheControl && i%32 == 0 {
			_, _ = builder.WriteString(`,"cache_control":{"type":"ephemeral"}`)
		}
		_, _ = builder.WriteString(`}]}`)
	}
	_, _ = builder.WriteString(`]}`)
	return []byte(builder.String())
}

func buildLargeGeminiContentsBody(targetBytes int) []byte {
	var builder strings.Builder
	builder.Grow(targetBytes + 1024)
	_, _ = builder.WriteString(`{"model":"gemini-2.5-pro","systemInstruction":{"parts":[{"text":"system seed"}]},"contents":[`)
	for i := 0; builder.Len() < targetBytes; i++ {
		if i > 0 {
			_ = builder.WriteByte(',')
		}
		_, _ = builder.WriteString(`{"role":"user","parts":[{"text":"`)
		_, _ = builder.WriteString(strings.Repeat("gemini payload ", 64))
		_, _ = builder.WriteString(strconv.Itoa(i))
		_, _ = builder.WriteString(`"}]}`)
	}
	_, _ = builder.WriteString(`]}`)
	return []byte(builder.String())
}

func buildLargeOpenAIResponsesBody(targetBytes int) []byte {
	var builder strings.Builder
	builder.Grow(targetBytes + 1024)
	_, _ = builder.WriteString(`{"model":"gpt-5.4","stream":true,"prompt_cache_key":"session-benchmark","input":[`)
	for i := 0; builder.Len() < targetBytes; i++ {
		if i > 0 {
			_ = builder.WriteByte(',')
		}
		_, _ = builder.WriteString(`{"type":"message","role":"user","content":[{"type":"input_text","text":"`)
		_, _ = builder.WriteString(strings.Repeat("openai responses payload ", 48))
		_, _ = builder.WriteString(strconv.Itoa(i))
		_, _ = builder.WriteString(`"}]}`)
	}
	_, _ = builder.WriteString(`],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"query":{"type":"string"}}}}]}`)
	return []byte(builder.String())
}

func buildLargeOpenAIResponsesToolContinuationBody(targetBytes int) []byte {
	var builder strings.Builder
	builder.Grow(targetBytes + 1024)
	_, _ = builder.WriteString(`{"model":"gpt-5.4","stream":true,"previous_response_id":"resp_benchmark","input":[`)
	for i := 0; builder.Len() < targetBytes; i++ {
		if i > 0 {
			_ = builder.WriteByte(',')
		}
		callID := "call_" + strconv.Itoa(i)
		_, _ = builder.WriteString(`{"type":"item_reference","id":"`)
		_, _ = builder.WriteString(callID)
		_, _ = builder.WriteString(`"},{"type":"function_call_output","call_id":"`)
		_, _ = builder.WriteString(callID)
		_, _ = builder.WriteString(`","output":"`)
		_, _ = builder.WriteString(strings.Repeat("tool output payload ", 48))
		_, _ = builder.WriteString(strconv.Itoa(i))
		_, _ = builder.WriteString(`"}`)
	}
	_, _ = builder.WriteString(`]}`)
	return []byte(builder.String())
}

func buildLargeOpenAIResponsesImageToolBody(targetBytes int) []byte {
	var builder strings.Builder
	builder.Grow(targetBytes + 1024)
	_, _ = builder.WriteString(`{"model":"gpt-5.4","stream":false,"tools":[{"type":"image_generation","model":"gpt-image-2","size":"2048x1152"}],"input":[`)
	for i := 0; builder.Len() < targetBytes; i++ {
		if i > 0 {
			_ = builder.WriteByte(',')
		}
		_, _ = builder.WriteString(`{"type":"message","role":"user","content":[{"type":"input_text","text":"`)
		_, _ = builder.WriteString(strings.Repeat("openai image billing payload ", 48))
		_, _ = builder.WriteString(strconv.Itoa(i))
		_, _ = builder.WriteString(`"}]}`)
	}
	_, _ = builder.WriteString(`]}`)
	return []byte(builder.String())
}
