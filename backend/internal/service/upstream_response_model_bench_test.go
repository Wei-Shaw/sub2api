package service

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

var (
	upstreamResponseModelBenchmarkSink string

	upstreamResponseModelBenchmarkTerminal = []byte(`{"type":"response.completed","response":{"id":"resp_123","model":"gpt-5.5-2026-04-23"},"usage":{"input_tokens":128,"output_tokens":64}}`)
	upstreamResponseModelBenchmarkDelta    = []byte(`{"type":"response.output_text.delta","delta":"hello"}`)
	upstreamResponseModelBenchmarkWrapper  = []byte(`{"response":{"response":{"modelVersion":"gemini-3-pro","candidates":[]}}}`)
)

func BenchmarkUpstreamResponseModelOpenAI(b *testing.B) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "terminal_with_model", payload: upstreamResponseModelBenchmarkTerminal},
		{name: "delta_without_model", payload: upstreamResponseModelBenchmarkDelta},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.Run("legacy", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					upstreamResponseModelBenchmarkSink = benchmarkLegacyOpenAIModel(tt.payload)
				}
			})
			b.Run("optimized", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					upstreamResponseModelBenchmarkSink = firstValidTrimmedGJSONModel(tt.payload, "response.model", "model")
				}
			})
		})
	}
}

func BenchmarkUpstreamResponseModelAntigravityWrapper(b *testing.B) {
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			upstreamResponseModelBenchmarkSink = benchmarkLegacyWrappedGeminiModel(upstreamResponseModelBenchmarkWrapper)
		}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			upstreamResponseModelBenchmarkSink = firstValidTrimmedGJSONModel(
				upstreamResponseModelBenchmarkWrapper,
				"modelVersion",
				"response.modelVersion",
				"response.response.modelVersion",
			)
		}
	})
}

func benchmarkLegacyOpenAIModel(payload []byte) string {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return ""
	}
	return benchmarkFirstTrimmedGJSONModel(
		gjson.GetBytes(payload, "response.model"),
		gjson.GetBytes(payload, "model"),
	)
}

func benchmarkLegacyWrappedGeminiModel(payload []byte) string {
	if inner := gjson.GetBytes(payload, "response"); inner.Exists() {
		payload = []byte(inner.Raw)
	}
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return ""
	}
	return benchmarkFirstTrimmedGJSONModel(
		gjson.GetBytes(payload, "modelVersion"),
		gjson.GetBytes(payload, "response.modelVersion"),
	)
}

func benchmarkFirstTrimmedGJSONModel(values ...gjson.Result) string {
	for _, value := range values {
		if !value.Exists() || value.Type != gjson.String {
			continue
		}
		if model := strings.TrimSpace(value.String()); model != "" {
			return model
		}
	}
	return ""
}
