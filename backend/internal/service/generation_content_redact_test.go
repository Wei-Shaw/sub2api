package service

import (
	"strings"
	"testing"
)

func TestRedactGenerationPromptStripsSecretsAndPII(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-8","access_token":"tok_abcdef1234567890","messages":[{"role":"user","content":"mail me a@b.com call +1 415 555 0199 key sk-ant-supersecretvalue1234567890 password: hunter2pass"}]}`)
	out := redactGenerationPrompt(body)

	for _, leak := range []string{"tok_abcdef1234567890", "a@b.com", "sk-ant-supersecretvalue1234567890", "hunter2pass"} {
		if strings.Contains(out, leak) {
			t.Errorf("prompt redaction leaked %q in: %s", leak, out)
		}
	}
	if !strings.Contains(out, "[EMAIL]") {
		t.Errorf("expected [EMAIL] marker, got: %s", out)
	}
	// must NOT over-redact the model name (analytics needs it)
	if !strings.Contains(out, "claude-opus-4-8") {
		t.Errorf("model name was over-redacted, got: %s", out)
	}
}

func TestRedactGenerationResponseStripsSecrets(t *testing.T) {
	out := redactGenerationResponse([]byte("here is Bearer abcdef1234567890XYZ token and contact x@y.com"))
	if strings.Contains(out, "abcdef1234567890XYZ") {
		t.Errorf("response redaction leaked bearer token: %s", out)
	}
	if strings.Contains(out, "x@y.com") {
		t.Errorf("response redaction leaked email: %s", out)
	}
}

func TestRedactGenerationEmptyInputs(t *testing.T) {
	if redactGenerationPrompt(nil) != "" || redactGenerationPrompt([]byte{}) != "" {
		t.Errorf("empty prompt must redact to empty string")
	}
	if redactGenerationResponse(nil) != "" {
		t.Errorf("empty response must redact to empty string")
	}
}
