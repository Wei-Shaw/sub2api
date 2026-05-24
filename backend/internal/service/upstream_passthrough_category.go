package service

// DeriveUpstreamCategory classifies an upstream account into one of three
// categories. The decision is a pure function of the account's Type + Platform
// fields, with an optional admin override in extra.upstream_passthrough.category_override.
//
// Resolution rules (first match wins):
//  0. extra.upstream_passthrough.category_override (if a valid category string)
//  1. Type == AccountTypeUpstream → relay
//  2. Platform ∈ {Antigravity, Kiro} → reverse
//  3. Type == AccountTypeSetupToken → reverse (Claude Code inference-only mode)
//  4. Type == AccountTypeOAuth on Anthropic/OpenAI → reverse
//     (conservative: with or without credentials.client marker, since sub2api's
//      OAuth flows for these platforms predominantly serve client impersonation)
//  5. Otherwise → official (safest fallback for unknown combinations)
//
// nil account → CategoryOfficial (defensive).
func DeriveUpstreamCategory(a *Account) UpstreamCategory {
	if a == nil {
		return CategoryOfficial
	}

	// Rule 0: explicit override wins (only if valid value)
	if override := a.GetUpstreamPassthroughCategoryOverride(); override != "" {
		return override
	}

	// Rule 1: Upstream type → relay
	if a.Type == AccountTypeUpstream {
		return CategoryRelay
	}

	// Rule 2: known reverse-client platforms → reverse
	if a.Platform == PlatformAntigravity || a.Platform == PlatformKiro {
		return CategoryReverse
	}

	// Rule 3: SetupToken (Claude Code inference-only) → reverse
	if a.Type == AccountTypeSetupToken {
		return CategoryReverse
	}

	// Rule 4: OAuth on Anthropic/OpenAI → reverse (conservative client-impersonation default)
	if a.Type == AccountTypeOAuth {
		if a.Platform == PlatformAnthropic || a.Platform == PlatformOpenAI {
			return CategoryReverse
		}
	}

	// Rule 5: fallback
	return CategoryOfficial
}
