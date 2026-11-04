package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

var (
	round2StringSink string
	round2ResultSink gjson.Result
	round2BoolSink   bool
	round2IntSink    int
	round2BytesSink  []byte
)

func round2CompletedEvent(size int) []byte {
	return []byte(`{"type":"response.completed","response":{"id":"resp_round2","model":"gpt-5.1","output":[{"type":"message","content":[{"type":"output_text","text":"` + strings.Repeat("x", size) + `"}]}],"usage":{"input_tokens":100,"output_tokens":50,"input_tokens_details":{"cached_tokens":20}}}}`)
}

func BenchmarkRound2IntegritySnapshot(b *testing.B) {
	for _, size := range []int{2048, 1 << 20, 16 << 20} {
		body := bytes.Repeat([]byte{'x'}, size)
		b.Run(fmt.Sprintf("%dKiB/unconditional_clone", size>>10), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for range b.N {
				round2BytesSink = bytes.Clone(body)
			}
		})
	}
}

// Exploration-only equivalent for the valid envelope fixtures. Small retained
// strings are cloned to avoid keeping a multi-megabyte event alive via an ID.
func round2ViewEnvelope(body []byte) (string, string, gjson.Result) {
	root := parseRawJSONView(body)
	response := root.Get("response")
	id := strings.TrimSpace(response.Get("id").String())
	if id == "" {
		id = strings.TrimSpace(root.Get("id").String())
	}
	return strings.Clone(strings.TrimSpace(root.Get("type").String())), strings.Clone(id), response
}

func TestRound2EnvelopeProbeFixtures(t *testing.T) {
	for _, payload := range [][]byte{
		round2CompletedEvent(1 << 20),
		[]byte(`{"id":"evt_1","type":"response.output_text.delta","delta":"你好"}`),
		[]byte(`{"type":"response.completed","response":{"id":" escaped\nID "}}`),
	} {
		wantType, wantID, wantResponse := parseOpenAIWSEventEnvelope(payload)
		gotType, gotID, gotResponse := round2ViewEnvelope(payload)
		if wantType != gotType || wantID != gotID || wantResponse.Raw != gotResponse.Raw {
			t.Fatal("envelope fixture mismatch")
		}
	}
}

func BenchmarkRound2WSEnvelope(b *testing.B) {
	for _, size := range []int{1024, 256 << 10, 1 << 20, 8 << 20} {
		payload := round2CompletedEvent(size)
		for _, probe := range []struct {
			name string
			fn   func([]byte) (string, string, gjson.Result)
		}{{"current", parseOpenAIWSEventEnvelope}, {"view_probe", round2ViewEnvelope}} {
			b.Run(fmt.Sprintf("%dKiB/%s", size>>10, probe.name), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))
				for range b.N {
					round2StringSink, _, round2ResultSink = probe.fn(payload)
				}
			})
		}
	}
}

func BenchmarkRound2WSValidation(b *testing.B) {
	for _, size := range []int{1024, 256 << 10, 1 << 20} {
		payload := round2CompletedEvent(size)
		b.Run(fmt.Sprintf("%dKiB/current_two_validations", size>>10), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			for range b.N {
				_, repaired := splitOpenAIConcatenatedJSONDocuments(payload)
				round2BoolSink = json.Valid(payload) && !repaired
			}
		})
		b.Run(fmt.Sprintf("%dKiB/reused_validity_probe", size>>10), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			for range b.N {
				_, repaired, valid := classifyOpenAIConcatenatedJSONDocuments(payload)
				round2BoolSink = valid && !repaired
			}
		})
	}
}

func BenchmarkRound2ImageAccounting(b *testing.B) {
	for _, size := range []int{2048, 256 << 10, 1 << 20} {
		for _, kind := range []string{"text_delta", "text_terminal"} {
			payload := round2CompletedEvent(size)
			if kind == "text_delta" {
				payload = []byte(`{"type":"response.output_text.delta","delta":"` + strings.Repeat("x", size) + `"}`)
			}
			b.Run(fmt.Sprintf("%s/%dKiB", kind, size>>10), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))
				for range b.N {
					counter := newOpenAIImageOutputCounter()
					counter.AddSSEData(payload)
					round2IntSink = counter.BillableCount()
					if round2IntSink != 0 {
						b.Fatal("text fixture counted as image")
					}
				}
			})
		}
	}
}

func BenchmarkRound2UsageExtraction(b *testing.B) {
	for _, size := range []int{1024, 256 << 10, 1 << 20} {
		payload := round2CompletedEvent(size)
		b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			for range b.N {
				usage, ok := extractOpenAIUsageFromJSONBytes(payload)
				if !ok || usage.InputTokens != 100 || usage.OutputTokens != 50 {
					b.Fatal("usage fixture mismatch")
				}
				round2IntSink = usage.CacheReadInputTokens
			}
		})
	}
}

func BenchmarkRound2Tokenizer(b *testing.B) {
	// Warm the lazily initialized vocabulary before measuring steady state.
	codec, err := openAIInputTokensCodecForModel("gpt-5.1")
	if err != nil {
		b.Fatal(err)
	}
	for _, size := range []int{64 << 10, 1 << 20} {
		text := strings.Repeat("The code handles requests, tools, JSON and streaming. ", size/53)
		raw, err := json.Marshal(text)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("fallback_full/%dKiB", size>>10), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(text)))
			for range b.N {
				count, err := estimateOpenAIInputTokens(openAIInputTokensCountRequest{Model: "gpt-5.1", Input: raw})
				if err != nil || count <= 0 {
					b.Fatalf("count=%d err=%v", count, err)
				}
				round2IntSink = count
			}
		})
		b.Run(fmt.Sprintf("codec_count_only/%dKiB", size>>10), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(text)))
			for range b.N {
				count, err := codec.Count(text)
				if err != nil {
					b.Fatal(err)
				}
				round2IntSink = count
			}
		})
	}
}
