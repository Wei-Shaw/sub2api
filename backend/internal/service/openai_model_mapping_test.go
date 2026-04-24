package service

import "testing"

func TestResolveOpenAIForwardModel(t *testing.T) {
	tests := []struct {
		name               string
		account            *Account
		requestedModel     string
		defaultMappedModel string
		expectedModel      string
	}{
		{
			name: "falls back to group default when account has no mapping",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-4o-mini",
		},
		{
			name: "preserves exact passthrough mapping instead of group default",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
		{
			name: "preserves wildcard passthrough mapping instead of group default",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-*": "gpt-5.4",
					},
				},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
		{
			name: "uses account remap when explicit target differs",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5": "gpt-5.4",
					},
				},
			},
			requestedModel:     "gpt-5",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAIForwardModel(tt.account, tt.requestedModel, tt.defaultMappedModel); got != tt.expectedModel {
				t.Fatalf("resolveOpenAIForwardModel(...) = %q, want %q", got, tt.expectedModel)
			}
		})
	}
}

func TestResolveOpenAIForwardModel_PreventsClaudeModelFromFallingBackToGpt51(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{},
	}

	withoutDefault := normalizeCodexModel(resolveOpenAIForwardModel(account, "claude-opus-4-6", ""))
	if withoutDefault != "gpt-5.1" {
		t.Fatalf("normalizeCodexModel(...) = %q, want %q", withoutDefault, "gpt-5.1")
	}

	withDefault := normalizeCodexModel(resolveOpenAIForwardModel(account, "claude-opus-4-6", "gpt-5.4"))
	if withDefault != "gpt-5.4" {
		t.Fatalf("normalizeCodexModel(...) = %q, want %q", withDefault, "gpt-5.4")
	}
}

func TestNormalizeCodexModel(t *testing.T) {
	cases := map[string]string{
		"gpt-5.3-codex-spark":       "gpt-5.3-codex",
		"gpt-5.3-codex-spark-high":  "gpt-5.3-codex",
		"gpt-5.3-codex-spark-xhigh": "gpt-5.3-codex",
		"gpt-5.3":                   "gpt-5.3-codex",
	}

	for input, expected := range cases {
		if got := normalizeCodexModel(input); got != expected {
			t.Fatalf("normalizeCodexModel(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestNormalizeCodexModel_PreservesNewGPT55Model(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"gpt-5.5": "gpt-5.5",
		"GPT-5.5": "gpt-5.5",
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := normalizeCodexModel(input); got != want {
				t.Fatalf("normalizeCodexModel(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestNormalizeOpenAIModelForUpstream(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		model   string
		want    string
	}{
		{
			name:    "oauth keeps codex normalization behavior",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gemini-3-flash-preview",
			want:    "gpt-5.1",
		},
		{
			name:    "apikey preserves custom compatible model",
			account: &Account{Type: AccountTypeAPIKey},
			model:   "gemini-3-flash-preview",
			want:    "gemini-3-flash-preview",
		},
		{
			name:    "apikey preserves official non codex model",
			account: &Account{Type: AccountTypeAPIKey},
			model:   "gpt-4.1",
			want:    "gpt-4.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOpenAIModelForUpstream(tt.account, tt.model); got != tt.want {
				t.Fatalf("normalizeOpenAIModelForUpstream(...) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeOpenAIModelForUpstream_OAuthPreservesGPT55(t *testing.T) {
	t.Parallel()

	account := &Account{Type: AccountTypeOAuth}
	tests := map[string]string{
		"gpt-5.5":     "gpt-5.5",
		"GPT-5.5":     "gpt-5.5",
		"GPT-5.5-Sys": "gpt-5.5",
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := normalizeOpenAIModelForUpstream(account, input); got != want {
				t.Fatalf("normalizeOpenAIModelForUpstream(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestIsSysModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "plain", model: "gpt-5", want: false},
		{name: "sys lower", model: "gpt-5-sys", want: true},
		{name: "sys mixed case", model: "gpt-5-SyS", want: true},
		{name: "trimmed", model: "  gpt-5-sYs  ", want: true},
		{name: "empty", model: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSysModel(tt.model); got != tt.want {
				t.Fatalf("IsSysModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestStripSysSuffix(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "plain", model: "gpt-5", want: "gpt-5"},
		{name: "trim non sys", model: "  gpt-5  ", want: "gpt-5"},
		{name: "strip lower", model: "gpt-5-sys", want: "gpt-5"},
		{name: "strip mixed case", model: "gpt-5-SyS", want: "gpt-5"},
		{name: "strip with spaces", model: "  GPT-5-sYs  ", want: "GPT-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripSysSuffix(tt.model); got != tt.want {
				t.Fatalf("StripSysSuffix(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}
