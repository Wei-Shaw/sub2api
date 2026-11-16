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
	item := `{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", 4000) + `"}]}`
	if anthropic {
		sb.WriteString(`{"model":"claude-sonnet-5-5","stream":true,"system":[{"type":"text","text":"`)
		sb.WriteString(strings.Repeat("z", targetBytes/8))
		sb.WriteString(`"}],"tools":[{"name":"shell","description":"`)
		sb.WriteString(strings.Repeat("y", targetBytes/16))
		sb.WriteString(`"}],"messages":[`)
		msg := `{"role":"user","content":[{"type":"text","text":"` + strings.Repeat("x", 4000) + `"}]}`
		for i := 0; sb.Len() < targetBytes; i++ {
			if i > 0 {
				sb.WriteString(`,`)
			}
			sb.WriteString(msg)
		}
		sb.WriteString(`]}`)
		return []byte(sb.String())
	}
	sb.WriteString(`{"model":"gpt-5.5","stream":true,"instructions":"`)
	sb.WriteString(strings.Repeat("z", targetBytes/8))
	sb.WriteString(`","tools":[{"type":"function","name":"shell","description":"`)
	sb.WriteString(strings.Repeat("y", targetBytes/16))
	sb.WriteString(`"}],"input":[`)
	for i := 0; sb.Len() < targetBytes; i++ {
		if i > 0 {
			sb.WriteString(`,`)
		}
		sb.WriteString(item)
	}
	sb.WriteString(`]}`)
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
				if err := parseGatewayRequestBody(parsed, tc.protocol); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
