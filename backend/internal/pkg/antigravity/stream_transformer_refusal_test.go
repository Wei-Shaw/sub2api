package antigravity

import "testing"

func feedLines(p *StreamingProcessor, lines []string) {
	for _, l := range lines {
		_ = p.ProcessLine(l)
	}
	_, _ = p.Finish()
}

func TestStreamingProcessor_RefusalSignals_SafetyNoContent(t *testing.T) {
	p := NewStreamingProcessor("claude-opus-4")
	feedLines(p, []string{
		`data: {"response":{"candidates":[{"finishReason":"SAFETY","content":{"parts":[]}}]}}`,
	})
	if got := p.LastFinishReason(); got != "SAFETY" {
		t.Fatalf("LastFinishReason = %q, want SAFETY", got)
	}
	if p.SawMeaningfulContent() {
		t.Fatal("SafetyNoContent: SawMeaningfulContent should be false")
	}
}

func TestStreamingProcessor_RefusalSignals_TextContent(t *testing.T) {
	p := NewStreamingProcessor("claude-opus-4")
	feedLines(p, []string{
		`data: {"response":{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}}`,
		`data: {"response":{"candidates":[{"finishReason":"STOP"}]}}`,
	})
	if !p.SawMeaningfulContent() {
		t.Fatal("TextContent: SawMeaningfulContent should be true")
	}
	if got := p.LastFinishReason(); got != "STOP" {
		t.Fatalf("LastFinishReason = %q, want STOP", got)
	}
}

func TestStreamingProcessor_RefusalSignals_ThinkingCounts(t *testing.T) {
	p := NewStreamingProcessor("claude-opus-4")
	feedLines(p, []string{
		`data: {"response":{"candidates":[{"content":{"parts":[{"text":"reasoning","thought":true}]}}]}}`,
		`data: {"response":{"candidates":[{"finishReason":"SAFETY"}]}}`,
	})
	if !p.SawMeaningfulContent() {
		t.Fatal("ThinkingCounts: thinking should count as meaningful content")
	}
}

func TestStreamingProcessor_RefusalSignals_ToolUseCounts(t *testing.T) {
	p := NewStreamingProcessor("claude-opus-4")
	feedLines(p, []string{
		`data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read","args":{}}}]}}]}}`,
		`data: {"response":{"candidates":[{"finishReason":"STOP"}]}}`,
	})
	if !p.SawMeaningfulContent() {
		t.Fatal("ToolUseCounts: tool use should count as meaningful content")
	}
	if !p.SawToolUse() {
		t.Fatal("ToolUseCounts: SawToolUse should be true")
	}
}
