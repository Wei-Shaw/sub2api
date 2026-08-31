package service

import "testing"

func TestNormalizeProviderError(t *testing.T) {
	if typ, msg := normalizeProviderError(ProviderErrorModeProxy, "openai", 401, "upstream_error", "Upstream authentication failed"); typ != "upstream_error" || msg != "Upstream authentication failed" {
		t.Fatalf("proxy mode changed error: type=%q message=%q", typ, msg)
	}
	if typ, msg := normalizeProviderError(ProviderErrorModeProvider, "openai", 429, "upstream_error", "Upstream rate limit exceeded"); typ != "rate_limit_error" || msg != "Rate limit reached for requests." {
		t.Fatalf("openai provider error = (%q, %q)", typ, msg)
	}
	if typ, msg := normalizeProviderError(ProviderErrorModeProvider, "anthropic", 529, "upstream_error", "Upstream service overloaded"); typ != "overloaded_error" || msg != "Overloaded" {
		t.Fatalf("anthropic provider error = (%q, %q)", typ, msg)
	}
}
