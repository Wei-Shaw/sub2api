//go:build unit

package service

import (
	"strings"
	"testing"
)

var benchmarkConvertOpenAICompactSink []byte

func BenchmarkConvertOpenAICompactInputsForGrok(b *testing.B) {
	largeText := strings.Repeat("payload-chunk-", 1<<12)
	noCompact := []byte(`{"model":"grok-4.5","input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"` + largeText + `"}]},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + largeText + `"}]},` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"follow up"}]}` +
		`],"tools":[{"type":"function","name":"read_file","parameters":{"type":"object"}}]}`)
	withCompact := []byte(`{"model":"grok-4.5","input":[` +
		`{"type":"compaction","encrypted_content":"enc-blob","summary":[{"type":"summary_text","text":"prior summary"}]},` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"` + largeText + `"}]}` +
		`]}`)

	b.Run("NoCompactionFastPath", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(noCompact)))
		for range b.N {
			out, err := convertOpenAICompactInputsForGrok(noCompact)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkConvertOpenAICompactSink = out
		}
	})
	b.Run("WithCompactionRewrite", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(withCompact)))
		for range b.N {
			out, err := convertOpenAICompactInputsForGrok(withCompact)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkConvertOpenAICompactSink = out
		}
	})
}
