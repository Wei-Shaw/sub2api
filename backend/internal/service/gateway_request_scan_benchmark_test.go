package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// buildGatewayRequestBenchmarkBody 生成 Codex / Claude Code 两种形态的大请求体。
func buildGatewayRequestBenchmarkBody(targetBytes int, anthropic bool) []byte {
	var sb strings.Builder
	sb.Grow(targetBytes + 1024)
	// strings.Builder.WriteString 的 error 恒为 nil，errcheck 不接受裸忽略，收口在这里。
	write := func(s string) { _, _ = sb.WriteString(s) }
	item := `{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", 4000) + `"}]}`
	if anthropic {
		write(`{"model":"claude-sonnet-5-5","stream":true,"system":[{"type":"text","text":"`)
		write(strings.Repeat("z", targetBytes/8))
		write(`"}],"tools":[{"name":"shell","description":"`)
		write(strings.Repeat("y", targetBytes/16))
		write(`"}],"messages":[`)
		msg := `{"role":"user","content":[{"type":"text","text":"` + strings.Repeat("x", 4000) + `"}]}`
		for i := 0; sb.Len() < targetBytes; i++ {
			if i > 0 {
				write(`,`)
			}
			write(msg)
		}
		write(`]}`)
		return []byte(sb.String())
	}
	write(`{"model":"gpt-5.5","stream":true,"instructions":"`)
	write(strings.Repeat("z", targetBytes/8))
	write(`","tools":[{"type":"function","name":"shell","description":"`)
	write(strings.Repeat("y", targetBytes/16))
	write(`"}],"input":[`)
	for i := 0; sb.Len() < targetBytes; i++ {
		if i > 0 {
			write(`,`)
		}
		write(item)
	}
	write(`]}`)
	return []byte(sb.String())
}

func BenchmarkParseGatewayRequestScan(b *testing.B) {
	cases := []struct {
		name      string
		size      int
		protocol  string
		anthropic bool
	}{
		{"responses_128KiB", 128 << 10, "responses", false},
		{"responses_1MiB", 1 << 20, "responses", false},
		{"anthropic_1MiB", 1 << 20, domain.PlatformAnthropic, true},
	}

	for _, tc := range cases {
		body := buildGatewayRequestBenchmarkBody(tc.size, tc.anthropic)
		b.Run(tc.name+"/legacy", func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for range b.N {
				parsed := &ParsedRequest{Body: NewRequestBodyRef(body)}
				if err := legacyParseGatewayRequestCurrentBody(parsed, tc.protocol); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(tc.name+"/scan", func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for range b.N {
				parsed := &ParsedRequest{Body: NewRequestBodyRef(body)}
				if err := parseGatewayRequestBody(parsed, tc.protocol, true); err != nil {
					b.Fatal(err)
				}
			}
		})
		// 调用方已经校验过同一份字节时（handler 已做 gjson.ValidBytes，且之后只经 sjson 改写）。
		b.Run(tc.name+"/scan_validated", func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for range b.N {
				parsed := &ParsedRequest{Body: NewRequestBodyRef(body)}
				if err := parseGatewayRequestBody(parsed, tc.protocol, false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
