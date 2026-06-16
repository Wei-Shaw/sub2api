//go:build unit

package platforms

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"openai":     "openai",
		"OpenAI":     "openai",
		" openai ":   "openai",
		"":           "",
		" ":          "",
		"\tGEMINI\n": "gemini",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatch(t *testing.T) {
	if !Match("openai", "OpenAI") {
		t.Error("expected openai == OpenAI after normalize")
	}
	if !Match(" openai ", "openai") {
		t.Error("expected trim normalize to work")
	}
	if Match("openai", "anthropic") {
		t.Error("openai should not match anthropic")
	}
	// 不再有"空串通配"语义: 空只匹配空
	if !Match("", "") {
		t.Error("empty should match empty")
	}
	if Match("", "openai") {
		t.Error("empty must NOT match openai (this would be the old buggy behaviour)")
	}
	if Match("openai", "") {
		t.Error("openai must NOT match empty")
	}
}

func TestMatchAny(t *testing.T) {
	if !MatchAny([]string{"OpenAI", "Anthropic"}, "openai") {
		t.Error("MatchAny should hit normalized openai")
	}
	if MatchAny([]string{"OpenAI"}, "gemini") {
		t.Error("MatchAny should miss")
	}
	if MatchAny(nil, "openai") {
		t.Error("MatchAny on nil should be false")
	}
}
