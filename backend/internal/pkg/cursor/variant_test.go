package cursor

import "testing"

func TestParseRunSlug(t *testing.T) {
	tests := []struct {
		in       string
		family   string
		effort   string
		fast     bool
		thinking bool
	}{
		{in: "grok-4.6", family: "grok-4.6"},
		{in: "cursor-grok-4.6-medium", family: "grok-4.6", effort: "medium"},
		{in: "cursor-grok-4.6-xhigh-fast", family: "grok-4.6", effort: "xhigh", fast: true},
		{in: "grok-4.5-fast-medium", family: "grok-4.5", effort: "medium", fast: true},
		{in: "claude-opus-5-thinking-max-fast", family: "claude-opus-5", effort: "max", fast: true, thinking: true},
		{in: "claude-4.6-sonnet-medium-thinking", family: "claude-4.6-sonnet", effort: "medium", thinking: true},
		{in: "claude-4.5-haiku", family: "claude-4.5-haiku"},
		{in: "claude-4.5-haiku-thinking", family: "claude-4.5-haiku", thinking: true},
		{in: "composer-2.5-fast", family: "composer-2.5", fast: true},
		{in: "gpt-5.5-extra-high-fast", family: "gpt-5.5", effort: "extra-high", fast: true},
		{in: "gpt-5.3-codex-fast", family: "gpt-5.3-codex", fast: true},
		{in: "gemini-3.6-flash-minimal", family: "gemini-3.6-flash", effort: "minimal"},
	}
	for _, tt := range tests {
		got := parseRunSlug(tt.in)
		if got.FamilyHint != tt.family || got.Effort != tt.effort || got.Fast != tt.fast || got.Thinking != tt.thinking {
			t.Fatalf("parseRunSlug(%q) = family=%q effort=%q fast=%v thinking=%v; want family=%q effort=%q fast=%v thinking=%v",
				tt.in, got.FamilyHint, got.Effort, got.Fast, got.Thinking, tt.family, tt.effort, tt.fast, tt.thinking)
		}
	}
}

func TestResolveRunModelDefaults(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		picker  string
		alias   bool
		variant bool
	}{
		{in: "default", want: "default", picker: "default"},
		{in: "composer-2.5", want: "composer-2.5", picker: "composer-2.5"},
		{in: "gemini-3.1-pro", want: "gemini-3.1-pro", picker: "gemini-3.1-pro"},
		{in: "grok-4.6", want: "cursor-grok-4.6-medium", picker: "grok-4.6", variant: true},
		{in: "claude-opus-5", want: "claude-opus-5-medium", picker: "claude-opus-5", variant: true},
		{in: "gpt-5.6-sol", want: "gpt-5.6-sol-medium", picker: "gpt-5.6-sol", variant: true},
		{in: "opus", want: "claude-opus-5-medium", picker: "claude-opus-5", alias: true, variant: true},
		{in: "cursor-grok-4.6-medium", want: "cursor-grok-4.6-medium", picker: "grok-4.6"},
		{in: "claude-opus-5-thinking-high-fast", want: "claude-opus-5-thinking-high-fast", picker: "claude-opus-5"},
		{in: "gpt-5.3-codex", want: "gpt-5.3-codex-fast", picker: "gpt-5.3-codex", variant: true},
		{in: "kimi-k3", want: "kimi-k3-high", picker: "kimi-k3", variant: true},
		{in: "glm-5.2", want: "glm-5.2-high", picker: "glm-5.2", variant: true},
		{in: "claude-haiku-4-5", want: "claude-4.5-haiku", picker: "claude-haiku-4-5", variant: true},
		{in: "claude-sonnet-4-6", want: "claude-4.6-sonnet-medium", picker: "claude-sonnet-4-6", variant: true},
		{in: "grok-4.6-high", want: "cursor-grok-4.6-high", picker: "grok-4.6", variant: true},
		{in: "composer", want: "composer-2.5", picker: "composer-2.5", alias: true},
		{in: "gpt-5.1", want: "gpt-5.1-high", picker: "gpt-5.1", variant: true},
	}
	for _, tt := range tests {
		got := ResolveRunModel(tt.in, RunOpts{}, nil)
		if got.RunSlug != tt.want || got.PickerID != tt.picker || got.AliasFallback != tt.alias || got.VariantApplied != tt.variant {
			t.Fatalf("ResolveRunModel(%q) = %+v; want slug=%q picker=%q alias=%v variant=%v",
				tt.in, got, tt.want, tt.picker, tt.alias, tt.variant)
		}
	}
}

func TestResolveRunModelRespectsEffortAndThinking(t *testing.T) {
	thinking := true
	got := ResolveRunModel("claude-opus-5", RunOpts{Effort: "max"}, nil)
	if got.RunSlug != "claude-opus-5-thinking-max" {
		t.Fatalf("max without thinking flag: %s", got.RunSlug)
	}
	got = ResolveRunModel("claude-opus-5", RunOpts{Effort: "high", Fast: true, Thinking: &thinking}, nil)
	if got.RunSlug != "claude-opus-5-thinking-high-fast" {
		t.Fatalf("thinking high fast: %s", got.RunSlug)
	}
	got = ResolveRunModel("gpt-5.5", RunOpts{Effort: "xhigh"}, nil)
	if got.RunSlug != "gpt-5.5-extra-high" {
		t.Fatalf("gpt-5.5 xhigh maps to extra-high: %s", got.RunSlug)
	}
	got = ResolveRunModel("composer-2.5", RunOpts{Fast: true}, nil)
	if got.RunSlug != "composer-2.5-fast" {
		t.Fatalf("composer fast: %s", got.RunSlug)
	}
}

func TestResolveRunModelUsesLiveCatalogLegacySlugs(t *testing.T) {
	catalog := []AvailableModel{{
		Name:        "claude-opus-5",
		DisplayName: "Claude Opus 5",
		Aliases:     []string{"opus"},
		LegacySlugs: []string{"claude-opus-5-high", "claude-opus-5-low"},
	}}
	got := ResolveRunModel("opus", RunOpts{}, catalog)
	if got.RunSlug != "claude-opus-5-high" {
		t.Fatalf("live catalog should pick high when medium is absent: %+v", got)
	}
}

func TestNormalizeEffort(t *testing.T) {
	tests := map[string]string{
		"x-high":     "xhigh",
		"extra_high": "xhigh",
		"MAX":        "max",
		"minimal":    "minimal",
		"nope":       "",
	}
	for in, want := range tests {
		if got := NormalizeEffort(in); got != want {
			t.Fatalf("NormalizeEffort(%q)=%q want %q", in, got, want)
		}
	}
}
