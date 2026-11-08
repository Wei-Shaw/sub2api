package service

import (
	"strings"
	"testing"
)

var benchmarkOpenAIContentSessionSeed string

func BenchmarkDeriveOpenAIContentSessionSeedLargeBody(b *testing.B) {
	largeHistory := strings.Repeat("payload", 1<<17)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "ChatCompletions", body: []byte(`{"model":"gpt-5.4","tools":[{"type":"function","function":{"name":"lookup"}}],"messages":[{"role":"system","content":"Be concise."},{"role":"user","content":"Hello"},{"role":"assistant","content":"` + largeHistory + `"},{"role":"user","content":"Follow-up"}]}`)},
		{name: "Responses", body: []byte(`{"model":"gpt-5.4","instructions":"Be concise.","tools":[{"type":"function","name":"lookup"}],"input":[{"role":"system","content":"System prompt"},{"role":"user","content":"Hello"},{"role":"assistant","content":"` + largeHistory + `"}]}`)},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(test.body)))
			for range b.N {
				benchmarkOpenAIContentSessionSeed = deriveOpenAIContentSessionSeed(test.body)
			}
		})
	}
}
