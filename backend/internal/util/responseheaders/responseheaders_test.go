package responseheaders

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestFilterHeadersDisabledUsesDefaultAllowlist(t *testing.T) {
	src := http.Header{}
	src.Add("Content-Type", "application/json")
	src.Add("X-Request-Id", "req-123")
	src.Add("X-Test", "ok")
	src.Add("Connection", "keep-alive")
	src.Add("Content-Length", "123")

	cfg := config.ResponseHeaderConfig{
		Enabled:     false,
		ForceRemove: []string{"x-request-id"},
	}

	filtered := FilterHeaders(src, CompileHeaderFilter(cfg))
	if filtered.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type passthrough, got %q", filtered.Get("Content-Type"))
	}
	if filtered.Get("X-Request-Id") != "req-123" {
		t.Fatalf("expected X-Request-Id allowed, got %q", filtered.Get("X-Request-Id"))
	}
	if filtered.Get("X-Test") != "" {
		t.Fatalf("expected X-Test removed, got %q", filtered.Get("X-Test"))
	}
	if filtered.Get("Connection") != "" {
		t.Fatalf("expected Connection to be removed, got %q", filtered.Get("Connection"))
	}
	if filtered.Get("Content-Length") != "" {
		t.Fatalf("expected Content-Length to be removed, got %q", filtered.Get("Content-Length"))
	}
}

func TestFilterHeadersAllowsReasoningIncludedByDefault(t *testing.T) {
	src := http.Header{}
	src.Set("X-Reasoning-Included", "1")

	filtered := FilterHeaders(src, CompileHeaderFilter(config.ResponseHeaderConfig{}))
	if got := filtered.Get("X-Reasoning-Included"); got != "1" {
		t.Fatalf("expected X-Reasoning-Included passthrough, got %q", got)
	}
}

func TestFilterHeadersForceRemoveOverridesReasoningIncluded(t *testing.T) {
	src := http.Header{}
	src.Set("X-Reasoning-Included", "1")

	filtered := FilterHeaders(src, CompileHeaderFilter(config.ResponseHeaderConfig{
		Enabled:     true,
		ForceRemove: []string{"x-reasoning-included"},
	}))
	if got := filtered.Get("X-Reasoning-Included"); got != "" {
		t.Fatalf("expected X-Reasoning-Included removal, got %q", got)
	}
}

func TestWriteClaudeCodeResponseHeaders(t *testing.T) {
	src := http.Header{}
	src.Set("X-Should-Retry", "true")
	src.Set("Anthropic-Ratelimit-Unified-Status", "allowed")
	src.Set("Anthropic-Ratelimit-Unified-Reset", "1760000000")
	src.Set("Anthropic-Organization-Id", "org-upstream")
	src.Set("Anthropic-Ratelimit-Tokens-Remaining", "100")
	src.Set("Set-Cookie", "secret=1")
	filter := CompileHeaderFilter(config.ResponseHeaderConfig{
		Enabled: true, AdditionalAllowed: []string{"x-should-retry"},
		ForceRemove: []string{"anthropic-ratelimit-unified-reset"},
	})
	dst := FilterHeaders(src, filter)
	WriteClaudeCodeResponseHeaders(dst, src, filter)

	if got := dst.Values("X-Should-Retry"); len(got) != 1 || got[0] != "true" {
		t.Fatalf("retry header copied twice or lost: %v", got)
	}
	if got := dst.Get("Anthropic-Ratelimit-Unified-Status"); got != "allowed" {
		t.Fatalf("unified rate limit header lost: %q", got)
	}
	for _, key := range []string{"Anthropic-Ratelimit-Unified-Reset", "Anthropic-Organization-Id", "Anthropic-Ratelimit-Tokens-Remaining", "Set-Cookie"} {
		if dst.Get(key) != "" {
			t.Fatalf("%s must not be forwarded", key)
		}
	}

	nilDst := http.Header{}
	WriteClaudeCodeResponseHeaders(nilDst, src, nil)
	if nilDst.Get("X-Should-Retry") != "true" || nilDst.Get("Anthropic-Organization-Id") != "" {
		t.Fatalf("nil filter must use the default rules: %v", nilDst)
	}
}

func TestFilterHeadersEnabledUsesAllowlist(t *testing.T) {
	src := http.Header{}
	src.Add("Content-Type", "application/json")
	src.Add("X-Extra", "ok")
	src.Add("X-Remove", "nope")
	src.Add("X-Blocked", "nope")

	cfg := config.ResponseHeaderConfig{
		Enabled:           true,
		AdditionalAllowed: []string{"x-extra"},
		ForceRemove:       []string{"x-remove"},
	}

	filtered := FilterHeaders(src, CompileHeaderFilter(cfg))
	if filtered.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type allowed, got %q", filtered.Get("Content-Type"))
	}
	if filtered.Get("X-Extra") != "ok" {
		t.Fatalf("expected X-Extra allowed, got %q", filtered.Get("X-Extra"))
	}
	if filtered.Get("X-Remove") != "" {
		t.Fatalf("expected X-Remove removed, got %q", filtered.Get("X-Remove"))
	}
	if filtered.Get("X-Blocked") != "" {
		t.Fatalf("expected X-Blocked removed, got %q", filtered.Get("X-Blocked"))
	}
}
