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
REDACTED{
		{name: "ChatCompletions", body: []byte(`{"model":"gpt-5.4","tools":[{"type":"function","function":{"name":"lookup"REDACTEDREDACTED],"messages":[{"role":"system","content":"Be concise."REDACTED,{"role":"user","content":"Hello"REDACTED,{"role":"assistant","content":"` + largeHistory + `"REDACTED,{"role":"user","content":"Follow-up"REDACTED]REDACTED`)REDACTED,
		{name: "Responses", body: []byte(`{"model":"gpt-5.4","instructions":"Be concise.","tools":[{"type":"function","name":"lookup"REDACTED],"input":[{"role":"system","content":"System prompt"REDACTED,{"role":"user","content":"Hello"REDACTED,{"role":"assistant","content":"` + largeHistory + `"REDACTED]REDACTED`)REDACTED,
REDACTED

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(test.body)))
			for range b.N {
				benchmarkOpenAIContentSessionSeed = deriveOpenAIContentSessionSeed(test.body)
		REDACTED
	REDACTED)
REDACTED
REDACTED
