package service

import (
	"fmt"
	"strings"
	"testing"
)

// buildToolSchemaBenchmarkBody 造一份贴近真实 Codex 请求的 body：小 tools 数组 +
// 大 input 历史。description 里带括号，因为真实工具的说明几乎都有。
func buildToolSchemaBenchmarkBody(targetBytes, tools int, withLookaround bool) []byte {
	var sb strings.Builder
	sb.Grow(targetBytes + 4096)
	write := func(s string) { _, _ = sb.WriteString(s) }
	write(`{"model":"gpt-5.6","stream":true,"instructions":"You are Codex, a coding agent.","tools":[`)
	for i := 0; i < tools; i++ {
		if i > 0 {
			write(",")
		}
		pattern := ""
		if withLookaround && i == 0 {
			pattern = `,"pattern":"(?=x).*"`
		}
		write(fmt.Sprintf(
			`{"type":"function","name":"tool_%d","description":"Runs a shell command (supports pipes).","parameters":{"type":"object","properties":{"query":{"type":"string"%s},"limit":{"type":"integer","minimum":1}},"required":["query"]}}`,
			i, pattern))
	}
	write(`],"input":[`)
	item := `{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", 4000) + `"}]}`
	for i := 0; sb.Len() < targetBytes; i++ {
		if i > 0 {
			write(",")
		}
		write(item)
	}
	write(`]}`)
	return []byte(sb.String())
}

// ungated 是改动前入口在这类 body 上的实际走向：旧的预检查是「body 里有 (」，
// 而工具说明几乎都带括号，所以请求无一例外走整趟解析。
func benchmarkToolSchemaEntry(
	b *testing.B, body []byte,
	ungated func([]byte) ([]byte, bool, error),
	gated func([]byte) ([]byte, bool, error),
) {
	b.Helper()
	b.Run("ungated", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		b.ReportAllocs()
		for range b.N {
			_, _, _ = ungated(body)
		}
	})
	b.Run("gated", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		b.ReportAllocs()
		for range b.N {
			_, _, _ = gated(body)
		}
	})
}

func BenchmarkSanitizeOpenAIResponsesToolSchemaPatternsClean1MiB(b *testing.B) {
	body := buildToolSchemaBenchmarkBody(1<<20, 30, false)
	benchmarkToolSchemaEntry(b, body,
		func(in []byte) ([]byte, bool, error) {
			return sanitizeOpenAIResponsesToolSchemas(in, openAIResponsesToolSchemaOptions{removeUnsupportedRegexPatterns: true})
		},
		sanitizeOpenAIResponsesToolSchemaPatterns,
	)
}

func BenchmarkSanitizeOpenAIResponsesToolParameterTypesClean1MiB(b *testing.B) {
	body := buildToolSchemaBenchmarkBody(1<<20, 30, false)
	benchmarkToolSchemaEntry(b, body,
		func(in []byte) ([]byte, bool, error) {
			return sanitizeOpenAIResponsesToolSchemas(in, openAIResponsesToolSchemaOptions{
				replaceNullParameterTypes:       true,
				injectObjectUnionRootObjectType: true,
				dropNullRequired:                true,
			})
		},
		sanitizeOpenAIResponsesToolParameterTypes,
	)
}

func BenchmarkOpenAIResponsesToolSchemaGates(b *testing.B) {
	body := buildToolSchemaBenchmarkBody(1<<20, 30, false)
	b.Run("unsupported_regex", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		for range b.N {
			_ = openAIResponsesBodyMayContainUnsupportedRegex(body)
		}
	})
	b.Run("parameter_repair", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		for range b.N {
			_ = openAIResponsesBodyMayNeedToolParameterRepair(body)
		}
	})
}

// 字符串扫描是解析里最重的一段：请求体的大头是长内容字符串。
func BenchmarkOpenAIResponsesJSONStringScan(b *testing.B) {
	body := []byte(`"` + strings.Repeat("abcdefgh", 1<<14) + `"`)
	b.Run("legacy", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		for range b.N {
			_, _, _, _ = legacyParseOpenAIResponsesJSONString(body, 0)
		}
	})
	b.Run("current", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		for range b.N {
			_ = currentParseOpenAIResponsesJSONString(body, 0)
		}
	})
}

// 平台入口就是三个调用点（HTTP 转发、WS 归一化、/v1/messages 转换）实际走的路径。
func BenchmarkSanitizeOpenAIResponsesToolSchemasForPlatformClean1MiB(b *testing.B) {
	body := buildToolSchemaBenchmarkBody(1<<20, 30, false)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for range b.N {
		_, _, _ = sanitizeOpenAIResponsesToolSchemasForPlatform(body, PlatformOpenAI)
	}
}
