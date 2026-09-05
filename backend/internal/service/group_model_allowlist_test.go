package service

import (
	"strings"
	"testing"
)

func TestNormalizeGroupModelAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		in      GroupModelAllowlist
		want    GroupModelAllowlist
		wantErr string
	}{
		{
			name: "trims and dedupes case-insensitively preserving order",
			in: GroupModelAllowlist{
				Enabled: true,
				Models:  []string{" gpt-5.4 ", "GPT-5.4", "claude-sonnet-4.5", ""},
			},
			want: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4", "claude-sonnet-4.5"}},
		},
		{
			name: "disabled with empty list is fine",
			in:   GroupModelAllowlist{Enabled: false},
			want: GroupModelAllowlist{Enabled: false},
		},
		{
			name:    "enabled with empty list is rejected",
			in:      GroupModelAllowlist{Enabled: true},
			wantErr: "INVALID_MODEL_ALLOWLIST",
		},
		{
			name:    "enabled with only blank entries is rejected",
			in:      GroupModelAllowlist{Enabled: true, Models: []string{" ", ""}},
			wantErr: "INVALID_MODEL_ALLOWLIST",
		},
		{
			name: "trailing wildcard is accepted",
			in:   GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.5-*"}},
			want: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.5-*"}},
		},
		{
			name:    "wildcard in the middle is rejected",
			in:      GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*-5.4"}},
			wantErr: "INVALID_MODEL_ALLOWLIST",
		},
		{
			name:    "bare wildcard is accepted as allow-all",
			in:      GroupModelAllowlist{Enabled: true, Models: []string{"*"}},
			want:    GroupModelAllowlist{Enabled: true, Models: []string{"*"}},
			wantErr: "",
		},
		{
			name:    "disabled config with invalid wildcard still rejected",
			in:      GroupModelAllowlist{Enabled: false, Models: []string{"foo-*bar"}},
			wantErr: "INVALID_MODEL_ALLOWLIST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeGroupModelAllowlist(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Enabled != tt.want.Enabled {
				t.Fatalf("enabled mismatch: got %v want %v", got.Enabled, tt.want.Enabled)
			}
			if len(got.Models) != len(tt.want.Models) {
				t.Fatalf("models mismatch: got %#v want %#v", got.Models, tt.want.Models)
			}
			for i := range tt.want.Models {
				if got.Models[i] != tt.want.Models[i] {
					t.Fatalf("models[%d] mismatch: got %q want %q", i, got.Models[i], tt.want.Models[i])
				}
			}
		})
	}
}

func TestGroupModelAllowlistAllows(t *testing.T) {
	allowlist := GroupModelAllowlist{
		Enabled: true,
		Models:  []string{"claude-sonnet-4.5", "gemini-2.5-pro", "gpt-5.5", "grok-*"},
	}

	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "exact match", model: "claude-sonnet-4.5", want: true},
		{name: "case-insensitive entry match", model: "Claude-Sonnet-4.5", want: true},
		{name: "not listed", model: "claude-opus-4.6", want: false},
		{name: "thinking suffix tolerated via claude normalization", model: "claude-sonnet-4.5-thinking", want: true},
		{name: "gemini models/ prefix stripped", model: "models/gemini-2.5-pro", want: true},
		{name: "gemini models/ prefix not in list", model: "models/gemini-2.5-flash", want: false},
		{name: "openai reasoning suffix normalizes to base model", model: "gpt-5.5-codex-high", want: true},
		{name: "openai reasoning suffix on unlisted model", model: "gpt-4.1-low", want: false},
		{name: "trailing wildcard prefix match", model: "grok-4.6", want: true},
		{name: "trailing wildcard requires prefix", model: "grok", want: false},
		{name: "empty model passes (handler decides required-ness)", model: "", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowlist.Allows(tt.model); got != tt.want {
				t.Fatalf("Allows(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}

	t.Run("disabled allowlist allows everything", func(t *testing.T) {
		disabled := GroupModelAllowlist{Enabled: false, Models: []string{"only-model"}}
		if !disabled.Allows("anything") {
			t.Fatal("disabled allowlist must allow all models")
		}
	})

	t.Run("enabled empty allowlist denies everything", func(t *testing.T) {
		empty := GroupModelAllowlist{Enabled: true}
		if empty.Allows("anything") {
			t.Fatal("enabled empty allowlist must deny all models")
		}
	})

	t.Run("bare wildcard allows everything", func(t *testing.T) {
		all := GroupModelAllowlist{Enabled: true, Models: []string{"*"}}
		if !all.Allows("claude-opus-4.6") || !all.Allows("models/gemini-2.5-pro") {
			t.Fatal("bare wildcard must allow all models")
		}
	})
}

func TestGroupModelAllowlistEnabled(t *testing.T) {
	var nilGroup *Group
	if nilGroup.ModelAllowlistEnabled() {
		t.Fatal("nil group must not report enabled")
	}
	group := &Group{}
	if group.ModelAllowlistEnabled() {
		t.Fatal("default group must not report enabled")
	}
	group.ModelAllowlist = GroupModelAllowlist{Enabled: true, Models: []string{"m"}}
	if !group.ModelAllowlistEnabled() {
		t.Fatal("enabled group must report enabled")
	}
}

func TestGroupModelAllowlistFilterForListing(t *testing.T) {
	source := []string{"claude-opus-4.6", "claude-sonnet-4.5", "gpt-5.4", "gpt-5.5-codex", "gpt-5.5-mini", "grok-4.6"}

	t.Run("passthrough when disabled", func(t *testing.T) {
		disabled := GroupModelAllowlist{Enabled: false, Models: []string{"gpt-5.4"}}
		got := disabled.FilterForListing(source)
		if len(got) != len(source) {
			t.Fatalf("disabled allowlist must not filter, got %#v", got)
		}
	})

	t.Run("exact entries keep entry order and require source membership", func(t *testing.T) {
		cfg := GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.5-codex", "claude-sonnet-4.5", "claude-haiku-4.5"}}
		got := cfg.FilterForListing(source)
		want := []string{"gpt-5.5-codex", "claude-sonnet-4.5"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})

	t.Run("bare wildcard expands to the full source list", func(t *testing.T) {
		cfg := GroupModelAllowlist{Enabled: true, Models: []string{"*"}}
		got := cfg.FilterForListing([]string{"gpt-5.4"})
		if strings.Join(got, ",") != "gpt-5.4" {
			t.Fatalf("bare wildcard must expose the full source, got %#v", got)
		}
	})

	t.Run("wildcard entries expand to source order", func(t *testing.T) {
		cfg := GroupModelAllowlist{Enabled: true, Models: []string{"claude-*", "gpt-5.5-*"}}
		got := cfg.FilterForListing(source)
		want := []string{"claude-opus-4.6", "claude-sonnet-4.5", "gpt-5.5-codex", "gpt-5.5-mini"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})

	t.Run("mixed exact and wildcard dedupes globally", func(t *testing.T) {
		cfg := GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*", "gpt-5.4"}}
		got := cfg.FilterForListing(source)
		want := []string{"gpt-5.4", "gpt-5.5-codex", "gpt-5.5-mini"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})

	t.Run("thinking-tolerant exact entries resolve against source", func(t *testing.T) {
		cfg := GroupModelAllowlist{Enabled: true, Models: []string{"claude-sonnet-4.5-thinking"}}
		got := cfg.FilterForListing(source)
		want := []string{"claude-sonnet-4.5-thinking"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})

	t.Run("exact entries match case-insensitively against source", func(t *testing.T) {
		// 归一化保留首次出现的拼写：条目大写、来源小写也必须命中，
		// 与 Allows 的大小写不敏感语义保持「准入允许 ⇒ 列表可见」。
		cfg := GroupModelAllowlist{Enabled: true, Models: []string{"GPT-5.4"}}
		got := cfg.FilterForListing([]string{"gpt-5.4"})
		want := []string{"GPT-5.4"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})

	t.Run("wildcard source patterns match case-insensitively", func(t *testing.T) {
		cfg := GroupModelAllowlist{Enabled: true, Models: []string{"claude-sonnet-4.20250514"}}
		got := cfg.FilterForListing([]string{"Claude-Sonnet-4*"})
		want := []string{"claude-sonnet-4.20250514"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})

	t.Run("wildcard source patterns whitelist exact entries", func(t *testing.T) {
		cfg := GroupModelAllowlist{Enabled: true, Models: []string{"claude-sonnet-4.20250514"}}
		got := cfg.FilterForListing([]string{"claude-sonnet-4*"})
		want := []string{"claude-sonnet-4.20250514"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})

	t.Run("empty source yields empty output", func(t *testing.T) {
		cfg := GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4"}}
		if got := cfg.FilterForListing(nil); len(got) != 0 {
			t.Fatalf("expected empty output, got %#v", got)
		}
	})

	t.Run("enabled empty config yields empty output", func(t *testing.T) {
		cfg := GroupModelAllowlist{Enabled: true}
		if got := cfg.FilterForListing(source); len(got) != 0 {
			t.Fatalf("expected empty output for enabled empty config, got %#v", got)
		}
	})
}
