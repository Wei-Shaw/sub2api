package antigravity

import (
	"strings"
	"testing"
)

func TestStreamingProcessorResponseShapesPreserveContent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		parts   string
		markers []string
	}{
		{
			name:    "text",
			parts:   `[{"text":"你好，请把研究方案发给我。"}]`,
			markers: []string{"text_delta", "你好，请把研究方案发给我。"},
		},
		{
			name:    "thinking",
			parts:   `[{"thought":true,"text":"先整理研究问题。"}]`,
			markers: []string{"thinking_delta", "先整理研究问题。"},
		},
		{
			name:    "tool_call",
			parts:   `[{"functionCall":{"name":"read_file","args":{"path":"notes.md"}}}]`,
			markers: []string{"tool_use", "read_file", "notes.md"},
		},
	} {
		for _, shape := range []string{"direct", "wrapped"} {
			t.Run(tc.name+"/"+shape, func(t *testing.T) {
				body := `{"candidates":[{"content":{"role":"model","parts":` + tc.parts + `},"finishReason":"STOP"}],` +
					`"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":30,"candidatesTokenCount":20,"thoughtsTokenCount":5},"responseId":"response-shape"}`
				if shape == "wrapped" {
					body = `{"response":` + body + `}`
				}
				p := NewStreamingProcessor("gemini-model")
				events := p.ProcessLine("data: " + body)
				finalEvents, usage := p.Finish()
				events = append(events, finalEvents...)
				if !p.HasContent() {
					t.Fatalf("valid %s response lost its content: %s", shape, events)
				}
				for _, marker := range append(tc.markers, "response-shape", "message_stop") {
					if !strings.Contains(string(events), marker) {
						t.Errorf("missing %q in events: %s", marker, events)
					}
				}
				if usage.InputTokens != 70 || usage.CacheReadInputTokens != 30 || usage.OutputTokens != 25 {
					t.Errorf("usage lost: %+v", usage)
				}
			})
		}
	}
}

func TestStreamingProcessorResponseShapesPreserveUsageWithoutContent(t *testing.T) {
	for _, shape := range []string{"direct", "wrapped"} {
		t.Run(shape, func(t *testing.T) {
			body := `{"usageMetadata":{"promptTokenCount":123,"candidatesTokenCount":0}}`
			if shape == "wrapped" {
				body = `{"response":` + body + `}`
			}
			p := NewStreamingProcessor("gemini-model")
			p.ProcessLine("data: " + body)
			_, usage := p.Finish()
			if usage.InputTokens != 123 || usage.OutputTokens != 0 {
				t.Fatalf("usage lost: %+v", usage)
			}
			if p.HasContent() {
				t.Fatal("usage-only response must not become content")
			}
		})
	}
}
