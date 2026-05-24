package service

import "strings"

// upstreamPassthroughExtraKey is the JSON key in account.Extra holding policy overrides.
const upstreamPassthroughExtraKey = "upstream_passthrough"

// GetUpstreamPassthroughProfile reads the account's per-account profile override.
// Empty string means "no override; inherit category default".
func (a *Account) GetUpstreamPassthroughProfile() PassthroughProfile {
	raw := a.upstreamPassthroughString("profile")
	switch PassthroughProfile(raw) {
	case ProfileTransparent, ProfileProtected, ProfileStrict:
		return PassthroughProfile(raw)
	default:
		return ""
	}
}

// GetUpstreamPassthroughCategoryOverride reads the account's optional category override.
// Empty string means "no override; run DeriveUpstreamCategory".
func (a *Account) GetUpstreamPassthroughCategoryOverride() UpstreamCategory {
	raw := a.upstreamPassthroughString("category_override")
	switch UpstreamCategory(raw) {
	case CategoryRelay, CategoryOfficial, CategoryReverse:
		return UpstreamCategory(raw)
	default:
		return ""
	}
}

// GetUpstreamPassthroughOverrides reads the sparse per-toggle override map.
// Only known toggle keys with bool values are returned; everything else is dropped.
func (a *Account) GetUpstreamPassthroughOverrides() map[string]bool {
	out := map[string]bool{}
	if a == nil || a.Extra == nil {
		return out
	}
	up, ok := a.Extra[upstreamPassthroughExtraKey].(map[string]any)
	if !ok {
		return out
	}
	raw, ok := up["overrides"].(map[string]any)
	if !ok {
		return out
	}
	knownToggles := []string{
		ToggleForwardClientHeaders,
		ToggleForwardUserNetworkInfo,
		ToggleSkipBodyScrub,
		ToggleSkipSystemPromptInject,
		ToggleForwardClientUA,
		ToggleForwardBetaFlags,
		ToggleSkipModelRewrite,
	}
	for _, k := range knownToggles {
		v, exists := raw[k]
		if !exists {
			continue
		}
		b, ok := v.(bool)
		if !ok {
			continue
		}
		out[k] = b
	}
	return out
}

// upstreamPassthroughString returns lowercase trimmed value of a string field
// under extra.upstream_passthrough.<key>. Empty when absent / wrong type.
func (a *Account) upstreamPassthroughString(key string) string {
	if a == nil || a.Extra == nil {
		return ""
	}
	up, ok := a.Extra[upstreamPassthroughExtraKey].(map[string]any)
	if !ok {
		return ""
	}
	raw, _ := up[key].(string)
	return strings.ToLower(strings.TrimSpace(raw))
}
