package antigravity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewPromptRule(t *testing.T) {
	if _, err := NewPromptRule("ok", `RFC\s+2119`, "RFC-2119"); err != nil {
		t.Fatalf("expected valid rule, got %v", err)
	}
	if _, err := NewPromptRule("empty", "   ", "x"); err == nil {
		t.Fatal("expected error for empty pattern")
	}
	if _, err := NewPromptRule("bad", "(", "x"); err == nil {
		t.Fatal("expected error for invalid pattern")
	}
}

func TestPromptSanitizerRuleCount(t *testing.T) {
	if NewPromptSanitizer(nil) != nil {
		t.Fatal("expected nil sanitizer for empty rules")
	}
	var zero PromptRule
	if s := NewPromptSanitizer([]PromptRule{zero}); s != nil {
		t.Fatal("expected nil sanitizer when all rules are uncompiled")
	}

	s := NewPromptSanitizer([]PromptRule{
		mustRule(t, "r1", `RFC\s+2119`, "RFC-2119"),
		mustRule(t, "r2", `MUST`, "must"),
	})
	if !s.Enabled() {
		t.Fatal("expected enabled sanitizer")
	}
	if got := s.RuleCount(); got != 2 {
		t.Fatalf("RuleCount() = %d, want 2", got)
	}
}

func TestPromptSanitizerSanitizeText(t *testing.T) {
	sanitizer := NewPromptSanitizer([]PromptRule{
		mustRule(t, "rfc", `(?i)RFC\s+2119\s+keywords\s*:`, "RFC-2119 keywords:"),
		mustRule(t, "must", `MUST`, "must"),
	})

	out, count := sanitizer.SanitizeText("RFC 2119 keywords: MUST, REQUIRED")
	if count != 2 {
		t.Fatalf("replacement count = %d, want 2", count)
	}
	want := "RFC-2119 keywords: must, REQUIRED"
	if out != want {
		t.Fatalf("SanitizeText() = %q, want %q", out, want)
	}

	// 没有命中时原样返回，计数为 0。
	untouched, count := sanitizer.SanitizeText("nothing to redact here")
	if count != 0 || untouched != "nothing to redact here" {
		t.Fatalf("unexpected no-op result %q/%d", untouched, count)
	}
}

func TestPromptSanitizerCaptureGroup(t *testing.T) {
	sanitizer := NewPromptSanitizer([]PromptRule{
		mustRule(t, "capture", `(RFC)\s+(2119)`, `$1-$2`),
	})
	out, count := sanitizer.SanitizeText("per RFC 2119 conventions")
	if count != 1 {
		t.Fatalf("replacement count = %d, want 1", count)
	}
	if out != "per RFC-2119 conventions" {
		t.Fatalf("capture-group replacement = %q", out)
	}
}

func TestPromptSanitizerSanitizeGeminiContent(t *testing.T) {
	sanitizer := NewPromptSanitizer([]PromptRule{
		mustRule(t, "rfc", `RFC\s+2119`, "RFC-2119"),
	})
	content := &GeminiContent{
		Role: "user",
		Parts: []GeminiPart{
			{Text: "RFC 2119 keywords: MUST"},
			{Text: ""},
			{InlineData: &GeminiInlineData{MimeType: "image/png", Data: "AAAA"}},
		},
	}
	if got := sanitizer.SanitizeGeminiContent(content); got != 1 {
		t.Fatalf("replacement count = %d, want 1", got)
	}
	if content.Parts[0].Text != "RFC-2119 keywords: MUST" {
		t.Fatalf("part text not rewritten: %q", content.Parts[0].Text)
	}
	if content.Parts[1].Text != "" {
		t.Fatalf("empty part should stay empty")
	}

	var nilContent *GeminiContent
	if got := sanitizer.SanitizeGeminiContent(nilContent); got != 0 {
		t.Fatalf("nil content replaced = %d, want 0", got)
	}
	var nilSanitizer *PromptSanitizer
	if nilSanitizer.Enabled() {
		t.Fatal("nil sanitizer should be disabled")
	}
	if got := nilSanitizer.SanitizeGeminiContent(content); got != 0 {
		t.Fatalf("nil sanitizer replaced = %d, want 0", got)
	}
}

func TestPromptSanitizerSanitizeRequestJSON(t *testing.T) {
	sanitizer := NewPromptSanitizer([]PromptRule{
		mustRule(t, "rfc", `RFC\s+2119`, "RFC-2119"),
	})

	bare := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []any{
				map[string]any{"text": "RFC 2119 keywords"},
			},
		},
		"contents": []any{map[string]any{"parts": []any{map[string]any{"text": "RFC 2119 in user content"}}}},
	}
	if got := sanitizer.SanitizeRequestJSON(bare); got != 1 {
		t.Fatalf("bare request replacement count = %d, want 1", got)
	}
	parts := bare["systemInstruction"].(map[string]any)["parts"].([]any)
	if text := parts[0].(map[string]any)["text"].(string); text != "RFC-2119 keywords" {
		t.Fatalf("systemInstruction text = %q", text)
	}
	// contents 不属于 systemInstruction，必须保持原样。
	contents := bare["contents"].([]any)
	userText := contents[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string)
	if userText != "RFC 2119 in user content" {
		t.Fatalf("contents should not be rewritten, got %q", userText)
	}

	wrapped := map[string]any{
		"project": "p",
		"request": map[string]any{
			"systemInstruction": map[string]any{
				"parts": []any{map[string]any{"text": "RFC 2119 wrapped"}},
			},
		},
	}
	if got := sanitizer.SanitizeRequestJSON(wrapped); got != 1 {
		t.Fatalf("wrapped request replacement count = %d, want 1", got)
	}

	// 结构不符合预期时安全返回 0。
	for _, value := range []any{nil, "string", 42, map[string]any{"systemInstruction": "oops"}} {
		if got := sanitizer.SanitizeRequestJSON(value); got != 0 {
			t.Fatalf("unexpected replacement %d for %v", got, value)
		}
	}
}

func mustRule(t *testing.T, name, pattern, replacement string) PromptRule {
	t.Helper()
	rule, err := NewPromptRule(name, pattern, replacement)
	if err != nil {
		t.Fatalf("NewPromptRule(%q) error: %v", name, err)
	}
	return rule
}

func TestPromptSanitizerDSHFingerprintRules(t *testing.T) {
	sanitizer := NewPromptSanitizer([]PromptRule{
		mustRule(t, "dsh-harness-identity", `(?i)You are an AI agent powered by DeepSeek Harness\.`, "You are an AI agent powered by an agent runtime."),
		mustRule(t, "dsh-harness-token", `(?i)\bDeepSeek Harness\b`, "agent runtime"),
		mustRule(t, "browser-dom-context", `(?i)The browser provides no implicit DOM, route, or screenshot context\.`, "The browser provides no implicit DOM context."),
	})

	input := strings.Join([]string{
		"You are an AI agent powered by DeepSeek Harness.",
		"",
		"The runtime context names the DeepSeek Harness implementation checkout.",
		"",
		"You are interacting with the user through the DeepSeek Harness Web GUI whose URL the runtime context names.",
		"",
		"The browser provides no implicit DOM, route, or screenshot context.",
	}, "\n")

	out, count := sanitizer.SanitizeText(input)
	if count != 4 {
		t.Fatalf("replacement count = %d, want 4 (identity + 2 tokens + browser sentence)", count)
	}
	if strings.Contains(out, "DeepSeek Harness") {
		t.Fatalf("harness token left in output: %q", out)
	}
	if strings.Contains(out, "implicit DOM, route, or screenshot context") {
		t.Fatalf("browser fingerprint sentence left in output: %q", out)
	}
	if !strings.Contains(out, "You are an AI agent powered by an agent runtime.") {
		t.Fatalf("identity sentence not rewritten: %q", out)
	}
	if !strings.Contains(out, "The browser provides no implicit DOM context.") {
		t.Fatalf("browser sentence not rewritten: %q", out)
	}
}

func TestBuildSystemInstructionAppliesPromptSanitizer(t *testing.T) {
	sanitizer := NewPromptSanitizer([]PromptRule{
		mustRule(t, "rfc", `(?i)RFC\s+2119\s+keywords\s*:`, "RFC-2119 keywords:"),
	})
	system := json.RawMessage(`"You are a helpful assistant. RFC 2119 keywords: MUST, REQUIRED."`)
	content := buildSystemInstruction(system, "gemini-4-argon-high", TransformOptions{
		EnableIdentityPatch: false,
		PromptSanitizer:     sanitizer,
	}, nil)
	if content == nil {
		t.Fatal("expected non-nil systemInstruction content")
	}
	var joined string
	for _, part := range content.Parts {
		joined += part.Text
		if strings.Contains(part.Text, "RFC 2119 keywords") {
			t.Fatalf("fingerprint phrase left in systemInstruction part: %q", part.Text)
		}
	}
	if !strings.Contains(joined, "RFC-2119 keywords: MUST, REQUIRED.") {
		t.Fatalf("sanitized phrase missing from systemInstruction: %q", joined)
	}

	// 未配置 sanitizer 时保持原样。
	plain := buildSystemInstruction(system, "gemini-4-argon-high", TransformOptions{EnableIdentityPatch: false}, nil)
	var plainJoined string
	for _, part := range plain.Parts {
		plainJoined += part.Text
	}
	if !strings.Contains(plainJoined, "RFC 2119 keywords: MUST, REQUIRED.") {
		t.Fatalf("unexpected rewrite without sanitizer: %q", plainJoined)
	}
}
