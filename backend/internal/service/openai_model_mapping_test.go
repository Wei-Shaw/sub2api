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
			name: "uses messages dispatch default for claude model",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "claude-opus-4-6",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-4o-mini",
		},
		{
			name: "does not fall back to group default for invalid gpt model",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt6",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "gpt6",
		},
		{
			name: "preserves explicit gpt-5.4 instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
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
		{
			name: "preserves codex spark instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.3-codex-spark",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "gpt-5.3-codex-spark",
		},
		{
			name: "preserves gpt-5.5 instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.5",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "gpt-5.5",
		},
		{
			name: "preserves compact-spelled gpt5.5 instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt5.5",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "gpt5.5",
		},
		{
			name: "preserves openai namespaced gpt-5.5 instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "openai/gpt-5.5",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "openai/gpt-5.5",
		},
		{
			name: "preserves compact gpt-5.5 instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.5-openai-compact",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "gpt-5.5-openai-compact",
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

func TestResolveOpenAIForwardModel_PreventsClaudeModelFromFallingBackToGpt54(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{},
	}

	withoutDefault := resolveOpenAIForwardModel(account, "claude-opus-4-6", "")
	if withoutDefault != "claude-opus-4-6" {
		t.Fatalf("resolveOpenAIForwardModel(...) = %q, want %q", withoutDefault, "claude-opus-4-6")
	}

	withDefault := resolveOpenAIForwardModel(account, "claude-opus-4-6", "gpt-5.4")
	if withDefault != "gpt-5.4" {
		t.Fatalf("resolveOpenAIForwardModel(...) = %q, want %q", withDefault, "gpt-5.4")
	}
}

func TestResolveOpenAICompactForwardModel(t *testing.T) {
	tests := []struct {
		name          string
		account       *Account
		model         string
		expectedModel string
	}{
		{
			name:          "nil account keeps original model",
			account:       nil,
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
		},
		{
			name: "missing compact mapping keeps original model",
			account: &Account{
				Credentials: map[string]any{},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
		},
		{
			name: "exact compact mapping overrides model",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4-openai-compact",
					},
				},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4-openai-compact",
		},
		{
			name: "wildcard compact mapping overrides model",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.*": "gpt-5-openai-compact",
					},
				},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5-openai-compact",
		},
		{
			name: "passthrough compact mapping remains unchanged",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAICompactForwardModel(tt.account, tt.model); got != tt.expectedModel {
				t.Fatalf("resolveOpenAICompactForwardModel(...) = %q, want %q", got, tt.expectedModel)
			}
		})
	}
}

func TestNormalizeCodexModel(t *testing.T) {
	cases := map[string]string{
		"gpt-5.3-codex-spark":       "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-high":  "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-xhigh": "gpt-5.3-codex-spark",
		"gpt-5.3":                   "gpt-5.3-codex",
		"gpt-image-2":               "gpt-image-2",
		"gpt-5.4-nano":              "gpt-5.4-nano",
		"gpt-5.4-nano-high":         "gpt-5.4-nano",
		"gpt6":                      "gpt6",
		"claude-opus-4-6":           "claude-opus-4-6",
	}

	for input, expected := range cases {
		if got := normalizeCodexModel(input); got != expected {
			t.Fatalf("normalizeCodexModel(%q) = %q, want %q", input, got, expected)
		}
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
			name:    "oauth preserves unknown non codex model",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gemini-3-flash-preview",
			want:    "gemini-3-flash-preview",
		},
		{
			name:    "oauth preserves invalid gpt model",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gpt6",
			want:    "gpt6",
		},
		{
			name:    "oauth normalizes known codex alias",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gpt-5.4-high",
			want:    "gpt-5.4",
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
