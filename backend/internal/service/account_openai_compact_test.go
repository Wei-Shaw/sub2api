package service

import "testing"

func TestAccountGetOpenAICompactMode(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    string
	}{
		{
			name: "nil account defaults to auto",
			want: OpenAICompactModeAuto,
		},
		{
			name: "non openai account defaults to auto",
			account: &Account{
				Platform: PlatformAnthropic,
				Extra:    map[string]any{"openai_compact_mode": OpenAICompactModeForceOn},
			},
			want: OpenAICompactModeAuto,
		},
		{
			name: "missing extra defaults to auto",
			account: &Account{
				Platform: PlatformOpenAI,
			},
			want: OpenAICompactModeAuto,
		},
		{
			name: "invalid mode falls back to auto",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": "  invalid  "},
			},
			want: OpenAICompactModeAuto,
		},
		{
			name: "force on is normalized",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": " FORCE_ON "},
			},
			want: OpenAICompactModeForceOn,
		},
		{
			name: "force off is normalized",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": "force_off"},
			},
			want: OpenAICompactModeForceOff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.account.GetOpenAICompactMode(); got != tt.want {
				t.Fatalf("GetOpenAICompactMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAccountOpenAICompactSupportKnown(t *testing.T) {
	tests := []struct {
		name          string
		account       *Account
		wantSupported bool
		wantKnown     bool
	}{
		{
			name:          "nil account is unknown",
			wantSupported: false,
			wantKnown:     false,
		},
		{
			name: "non openai account is unknown",
			account: &Account{
				Platform: PlatformAnthropic,
				Extra:    map[string]any{"openai_compact_supported": true},
			},
			wantSupported: false,
			wantKnown:     false,
		},
		{
			name: "force on overrides probe state",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					"openai_compact_mode":      OpenAICompactModeForceOn,
					"openai_compact_supported": false,
				},
			},
			wantSupported: true,
			wantKnown:     true,
		},
		{
			name: "force off overrides probe state",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					"openai_compact_mode":      OpenAICompactModeForceOff,
					"openai_compact_supported": true,
				},
			},
			wantSupported: false,
			wantKnown:     true,
		},
		{
			name: "auto true is known supported",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": true},
			},
			wantSupported: true,
			wantKnown:     true,
		},
		{
			name: "auto false is known unsupported",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": false},
			},
			wantSupported: false,
			wantKnown:     true,
		},
		{
			name: "auto without probe state remains unknown",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{},
			},
			wantSupported: false,
			wantKnown:     false,
		},
		{
			name: "invalid probe field remains unknown",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": "true"},
			},
			wantSupported: false,
			wantKnown:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSupported, gotKnown := tt.account.OpenAICompactSupportKnown()
			if gotSupported != tt.wantSupported || gotKnown != tt.wantKnown {
				t.Fatalf("OpenAICompactSupportKnown() = (%v, %v), want (%v, %v)", gotSupported, gotKnown, tt.wantSupported, tt.wantKnown)
			}
		})
	}
}

func TestAccountAllowsOpenAICompact(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name: "nil account does not allow compact",
			want: false,
		},
		{
			name: "non openai account does not allow compact",
			account: &Account{
				Platform: PlatformAnthropic,
			},
			want: false,
		},
		{
			name: "unknown openai account remains allowed",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{},
			},
			want: true,
		},
		{
			name: "supported openai account is allowed",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": true},
			},
			want: true,
		},
		{
			name: "unsupported openai account is rejected",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": false},
			},
			want: false,
		},
		{
			name: "force on is allowed",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": OpenAICompactModeForceOn},
			},
			want: true,
		},
		{
			name: "force off is rejected",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": OpenAICompactModeForceOff},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.account.AllowsOpenAICompact(); got != tt.want {
				t.Fatalf("AllowsOpenAICompact() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAccountGetCompactModelMapping(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    map[string]string
	}{
		{
			name: "nil account returns nil",
			want: nil,
		},
		{
			name: "missing credentials returns nil",
			account: &Account{
				Platform: PlatformOpenAI,
			},
			want: nil,
		},
		{
			name: "map any is converted",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4-openai-compact",
						"invalid": 1,
					},
				},
			},
			want: map[string]string{
				"gpt-5.4": "gpt-5.4-openai-compact",
			},
		},
		{
			name: "map string string is copied",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]string{
						"gpt-*": "compact-*",
					},
				},
			},
			want: map[string]string{
				"gpt-*": "compact-*",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.GetCompactModelMapping()
			if !equalStringMap(got, tt.want) {
				t.Fatalf("GetCompactModelMapping() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestAccountResolveCompactMappedModel(t *testing.T) {
	tests := []struct {
		name           string
		credentials    map[string]any
		requestedModel string
		expectedModel  string
		expectedMatch  bool
	}{
		{
			name:           "no compact mapping reports unmatched",
			credentials:    nil,
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  false,
		},
		{
			name: "exact compact mapping matches",
			credentials: map[string]any{
				"compact_model_mapping": map[string]any{
					"gpt-5.4": "gpt-5.4-openai-compact",
				},
			},
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4-openai-compact",
			expectedMatch:  true,
		},
		{
			name: "exact passthrough counts as match",
			credentials: map[string]any{
				"compact_model_mapping": map[string]any{
					"gpt-5.4": "gpt-5.4",
				},
			},
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  true,
		},
		{
			name: "longest wildcard wins",
			credentials: map[string]any{
				"compact_model_mapping": map[string]any{
					"gpt-*":         "fallback-compact",
					"gpt-5.4*":      "gpt-5.4-openai-compact",
					"gpt-5.4-mini*": "gpt-5.4-mini-openai-compact",
				},
			},
			requestedModel: "gpt-5.4-mini",
			expectedModel:  "gpt-5.4-mini-openai-compact",
			expectedMatch:  true,
		},
		{
			name: "missing compact mapping reports unmatched",
			credentials: map[string]any{
				"compact_model_mapping": map[string]any{
					"gpt-5.3": "gpt-5.3-openai-compact",
				},
			},
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformOpenAI,
				Credentials: tt.credentials,
			}
			gotModel, gotMatch := account.ResolveCompactMappedModel(tt.requestedModel)
			if gotModel != tt.expectedModel || gotMatch != tt.expectedMatch {
				t.Fatalf("ResolveCompactMappedModel(%q) = (%q, %v), want (%q, %v)", tt.requestedModel, gotModel, gotMatch, tt.expectedModel, tt.expectedMatch)
			}
		})
	}
}

func equalStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, want := range right {
		if got, ok := left[key]; !ok || got != want {
			return false
		}
	}
	return true
}
