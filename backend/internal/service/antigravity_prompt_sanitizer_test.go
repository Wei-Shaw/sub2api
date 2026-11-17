//go:build unit

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resetAntigravityPromptSanitizerCache(t *testing.T) {
	t.Helper()
	cache := &antigravityPromptSanitizerCache
	cache.mu.Lock()
	cache.key = ""
	cache.sanitizer = nil
	cache.mu.Unlock()
}

func configureAntigravityPromptSanitize(t *testing.T, enabled, inlineRules, filePath string) {
	t.Helper()
	t.Setenv(antigravityPromptSanitizeEnabledEnv, enabled)
	t.Setenv(antigravityPromptSanitizeRulesEnv, inlineRules)
	t.Setenv(antigravityPromptSanitizeFileEnv, filePath)
	resetAntigravityPromptSanitizerCache(t)
}

const testAntigravitySanitizeRule = `[{"name":"rfc2119-keywords","pattern":"(?i)RFC\\s+2119\\s+keywords\\s*:","replacement":"RFC-2119 keywords:"}]`

func testAntigravitySanitizeBody() []byte {
	return []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"systemInstruction":{"parts":[{"text":"RFC 2119 keywords: MUST, REQUIRED"}]},"generationConfig":{"maxOutputTokens":1024}}`)
}

func TestAntigravityPromptSanitizerInlineRules(t *testing.T) {
	configureAntigravityPromptSanitize(t, "", testAntigravitySanitizeRule, "")

	sanitizer := antigravityPromptSanitizer()
	if sanitizer == nil || sanitizer.RuleCount() != 1 {
		t.Fatalf("expected 1 configured rule, got %+v", sanitizer)
	}

	gateway := &AntigravityGatewayService{}
	wrapped, err := gateway.wrapV1InternalRequest("project-1", "gemini-4-argon-high", testAntigravitySanitizeBody())
	if err != nil {
		t.Fatalf("wrapV1InternalRequest error: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(wrapped, &body); err != nil {
		t.Fatalf("unmarshal wrapped body: %v", err)
	}
	request := body["request"].(map[string]any)
	parts := request["systemInstruction"].(map[string]any)["parts"].([]any)
	if text := parts[0].(map[string]any)["text"].(string); text != "RFC-2119 keywords: MUST, REQUIRED" {
		t.Fatalf("systemInstruction text = %q", text)
	}

	// 非 systemInstruction 字段保持不变。
	contents := request["contents"].([]any)
	if text := contents[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string); text != "hi" {
		t.Fatalf("contents text changed: %q", text)
	}
	config := request["generationConfig"].(map[string]any)
	if tokens, ok := config["maxOutputTokens"].(float64); !ok || tokens != 1024 {
		t.Fatalf("generationConfig changed: %v", config["maxOutputTokens"])
	}
}

func TestAntigravityPromptSanitizerDisabled(t *testing.T) {
	configureAntigravityPromptSanitize(t, "false", testAntigravitySanitizeRule, "")

	if sanitizer := antigravityPromptSanitizer(); sanitizer != nil {
		t.Fatalf("expected nil sanitizer when disabled, got %+v", sanitizer)
	}

	wrapped, err := (&AntigravityGatewayService{}).wrapV1InternalRequest("project-1", "gemini-4-argon-high", testAntigravitySanitizeBody())
	if err != nil {
		t.Fatalf("wrapV1InternalRequest error: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(wrapped, &body); err != nil {
		t.Fatalf("unmarshal wrapped body: %v", err)
	}
	request := body["request"].(map[string]any)
	parts := request["systemInstruction"].(map[string]any)["parts"].([]any)
	if text := parts[0].(map[string]any)["text"].(string); text != "RFC 2119 keywords: MUST, REQUIRED" {
		t.Fatalf("disabled sanitizer rewrote text: %q", text)
	}
}

func TestAntigravityPromptSanitizerSkipsInvalidRules(t *testing.T) {
	configureAntigravityPromptSanitize(t, "",
		`[{"name":"broken","pattern":"("},{"name":"empty","pattern":"  "},{"name":"ok","pattern":"RFC 2119","replacement":"RFC-2119"}]`, "")

	sanitizer := antigravityPromptSanitizer()
	if sanitizer == nil || sanitizer.RuleCount() != 1 {
		t.Fatalf("expected only the valid rule to survive, got %+v", sanitizer)
	}
	out, count := sanitizer.SanitizeText("RFC 2119 keywords")
	if count != 1 || out != "RFC-2119 keywords" {
		t.Fatalf("valid rule not applied: %q/%d", out, count)
	}
}

func TestAntigravityPromptSanitizerInvalidJSON(t *testing.T) {
	configureAntigravityPromptSanitize(t, "", "{not-json", "")
	if sanitizer := antigravityPromptSanitizer(); sanitizer != nil {
		t.Fatalf("expected nil sanitizer for invalid JSON, got %+v", sanitizer)
	}
}

func TestAntigravityPromptSanitizerFileHotReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "antigravity_prompt_sanitize.json")
	if err := os.WriteFile(path, []byte(`[{"name":"v1","pattern":"RFC 2119","replacement":"RFC-2119"}]`), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}
	first := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, first, first); err != nil {
		t.Fatalf("chtimes rules file: %v", err)
	}
	configureAntigravityPromptSanitize(t, "", "", path)

	sanitizer := antigravityPromptSanitizer()
	if sanitizer == nil || sanitizer.RuleCount() != 1 {
		t.Fatalf("expected file rules to load, got %+v", sanitizer)
	}
	if out, count := sanitizer.SanitizeText("RFC 2119"); count != 1 || out != "RFC-2119" {
		t.Fatalf("v1 rule not applied: %q/%d", out, count)
	}

	// 覆盖规则文件并刷新 mtime，下一次请求应使用新规则（无需重启）。
	if err := os.WriteFile(path, []byte(`[{"name":"v2","pattern":"MUST","replacement":"must"}]`), 0o600); err != nil {
		t.Fatalf("rewrite rules file: %v", err)
	}
	second := time.Now()
	if err := os.Chtimes(path, second, second); err != nil {
		t.Fatalf("chtimes rules file again: %v", err)
	}
	reloaded := antigravityPromptSanitizer()
	if reloaded == nil || reloaded.RuleCount() != 1 {
		t.Fatalf("expected reloaded sanitizer, got %+v", reloaded)
	}
	if out, count := reloaded.SanitizeText("MUST"); count != 1 || out != "must" {
		t.Fatalf("v2 rule not applied after reload: %q/%d", out, count)
	}
	if sanitizer == reloaded {
		t.Fatal("expected a fresh sanitizer instance after rules file change")
	}
}

func TestAntigravityPromptSanitizerMissingFile(t *testing.T) {
	configureAntigravityPromptSanitize(t, "", "", filepath.Join(t.TempDir(), "missing.json"))
	if sanitizer := antigravityPromptSanitizer(); sanitizer != nil {
		t.Fatalf("expected nil sanitizer for missing file, got %+v", sanitizer)
	}
}

func TestAntigravityPromptSanitizerNoSystemInstruction(t *testing.T) {
	configureAntigravityPromptSanitize(t, "", testAntigravitySanitizeRule, "")
	wrapped, err := (&AntigravityGatewayService{}).wrapV1InternalRequest("project-1", "gemini-4-argon-high",
		[]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	if err != nil {
		t.Fatalf("wrapV1InternalRequest error: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(wrapped, &body); err != nil {
		t.Fatalf("unmarshal wrapped body: %v", err)
	}
	request := body["request"].(map[string]any)
	if _, ok := request["systemInstruction"]; ok {
		t.Fatal("systemInstruction should not be created when absent from the request")
	}
}

func TestAntigravityClaudeTransformOptionsCarriesSanitizer(t *testing.T) {
	configureAntigravityPromptSanitize(t, "", testAntigravitySanitizeRule, "")
	opts := (&AntigravityGatewayService{}).getClaudeTransformOptions(t.Context())
	if opts.PromptSanitizer == nil || !opts.PromptSanitizer.Enabled() {
		t.Fatalf("expected sanitizer in claude transform options, got %+v", opts.PromptSanitizer)
	}
}
