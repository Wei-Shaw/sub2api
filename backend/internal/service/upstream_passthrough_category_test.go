package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeriveUpstreamCategory(t *testing.T) {
	cases := []struct {
		name string
		acct *Account
		want UpstreamCategory
	}{
		// Rule 1: Upstream type → relay (regardless of platform)
		{
			name: "Upstream type with anthropic platform → relay",
			acct: &Account{Type: AccountTypeUpstream, Platform: PlatformAnthropic},
			want: CategoryRelay,
		},
		{
			name: "Upstream type with openai platform → relay",
			acct: &Account{Type: AccountTypeUpstream, Platform: PlatformOpenAI},
			want: CategoryRelay,
		},

		// Rule 2: Reverse platforms → reverse (regardless of type)
		{
			name: "Antigravity platform OAuth → reverse",
			acct: &Account{Type: AccountTypeOAuth, Platform: PlatformAntigravity},
			want: CategoryReverse,
		},
		{
			name: "Kiro platform OAuth → reverse",
			acct: &Account{Type: AccountTypeOAuth, Platform: PlatformKiro},
			want: CategoryReverse,
		},
		{
			name: "Kiro platform APIKey → reverse",
			acct: &Account{Type: AccountTypeAPIKey, Platform: PlatformKiro},
			want: CategoryReverse,
		},

		// Rule 3: SetupToken → reverse (Claude Code inference-only)
		{
			name: "SetupToken type → reverse",
			acct: &Account{Type: AccountTypeSetupToken, Platform: PlatformAnthropic},
			want: CategoryReverse,
		},

		// Rule 4: OAuth + recognized client → reverse
		{
			name: "OAuth Anthropic with credentials.client=claude-cli → reverse",
			acct: &Account{Type: AccountTypeOAuth, Platform: PlatformAnthropic, Credentials: map[string]any{"client": "claude-cli"}},
			want: CategoryReverse,
		},
		{
			name: "OAuth Anthropic with credentials.client=claude-code → reverse",
			acct: &Account{Type: AccountTypeOAuth, Platform: PlatformAnthropic, Credentials: map[string]any{"client": "claude-code"}},
			want: CategoryReverse,
		},
		{
			name: "OAuth OpenAI with credentials.client=codex → reverse",
			acct: &Account{Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{"client": "codex"}},
			want: CategoryReverse,
		},
		{
			name: "OAuth Anthropic with no client marker → reverse (conservative)",
			acct: &Account{Type: AccountTypeOAuth, Platform: PlatformAnthropic},
			want: CategoryReverse,
		},
		{
			name: "OAuth OpenAI with no client marker → reverse (conservative)",
			acct: &Account{Type: AccountTypeOAuth, Platform: PlatformOpenAI},
			want: CategoryReverse,
		},
		{
			name: "OAuth Gemini with no client marker → official (not a reverse-client platform)",
			acct: &Account{Type: AccountTypeOAuth, Platform: PlatformGemini},
			want: CategoryOfficial,
		},

		// Rule 5: Fallback → official
		{
			name: "APIKey Anthropic → official",
			acct: &Account{Type: AccountTypeAPIKey, Platform: PlatformAnthropic},
			want: CategoryOfficial,
		},
		{
			name: "APIKey OpenAI → official",
			acct: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI},
			want: CategoryOfficial,
		},
		{
			name: "Bedrock → official",
			acct: &Account{Type: AccountTypeBedrock, Platform: PlatformAnthropic},
			want: CategoryOfficial,
		},
		{
			name: "ServiceAccount/Vertex → official",
			acct: &Account{Type: AccountTypeServiceAccount, Platform: PlatformGemini},
			want: CategoryOfficial,
		},

		// category_override forces specific value
		{
			name: "extra.upstream_passthrough.category_override=relay overrides derivation",
			acct: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformAnthropic,
				Extra: map[string]any{
					"upstream_passthrough": map[string]any{
						"category_override": "relay",
					},
				},
			},
			want: CategoryRelay,
		},
		{
			name: "extra.upstream_passthrough.category_override=reverse overrides Upstream type",
			acct: &Account{
				Type:     AccountTypeUpstream,
				Platform: PlatformAnthropic,
				Extra: map[string]any{
					"upstream_passthrough": map[string]any{
						"category_override": "reverse",
					},
				},
			},
			want: CategoryReverse,
		},
		{
			name: "category_override with invalid value is ignored, falls back to derivation",
			acct: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformAnthropic,
				Extra: map[string]any{
					"upstream_passthrough": map[string]any{
						"category_override": "garbage",
					},
				},
			},
			want: CategoryOfficial,
		},

		// Nil safety
		{
			name: "nil account → official (safe default)",
			acct: nil,
			want: CategoryOfficial,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DeriveUpstreamCategory(c.acct)
			require.Equal(t, c.want, got)
		})
	}
}
