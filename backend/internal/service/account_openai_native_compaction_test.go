package service

import "testing"

func TestAccountOpenAINativeCompactionV2SupportKnown(t *testing.T) {
	tests := []struct {
		name          string
		account       *Account
		wantSupported bool
		wantKnown     bool
	}{
		{name: "nil account"},
		{
			name:    "unknown",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		},
		{
			name:          "supported",
			account:       &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openAINativeCompactionV2SupportedKey: true}},
			wantSupported: true,
			wantKnown:     true,
		},
		{
			name:      "unsupported",
			account:   &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openAINativeCompactionV2SupportedKey: false}},
			wantKnown: true,
		},
		{
			name:    "legacy field does not leak",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_compact_supported": false}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported, known := tt.account.OpenAINativeCompactionV2SupportKnown()
			if supported != tt.wantSupported || known != tt.wantKnown {
				t.Fatalf("OpenAINativeCompactionV2SupportKnown() = (%v, %v), want (%v, %v)", supported, known, tt.wantSupported, tt.wantKnown)
			}
		})
	}
}

func TestAccountSupportsOpenAIResponsesCompactionV2Capability(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  bool
	}{
		{
			name:  "unknown native capability remains eligible",
			extra: map[string]any{"openai_responses_supported": true},
			want:  true,
		},
		{
			name: "native capability supported",
			extra: map[string]any{
				"openai_responses_supported":         true,
				openAINativeCompactionV2SupportedKey: true,
			},
			want: true,
		},
		{
			name: "native capability unsupported",
			extra: map[string]any{
				"openai_responses_supported":         true,
				openAINativeCompactionV2SupportedKey: false,
			},
		},
		{
			name: "responses capability unsupported",
			extra: map[string]any{
				"openai_responses_supported":         false,
				openAINativeCompactionV2SupportedKey: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Extra:    tt.extra,
			}
			if got := account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponsesCompactionV2); got != tt.want {
				t.Fatalf("SupportsOpenAIEndpointCapability(responses_compaction_v2) = %v, want %v", got, tt.want)
			}
		})
	}
}
