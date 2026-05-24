package service

import "strings"

// UpstreamCategory categorizes an upstream account by its relationship to the
// real upstream endpoint. Determined at runtime by DeriveUpstreamCategory.
//
// - relay:    points at another aggregator/relay; minimal protection needed
// - official: vendor-issued credentials on vendor's official API; standard hygiene
// - reverse:  impersonates a known client (Kiro/Antigravity/Claude Code/Codex);
//             must match the client's request fingerprint or risk ban
type UpstreamCategory string

const (
	CategoryRelay    UpstreamCategory = "relay"
	CategoryOfficial UpstreamCategory = "official"
	CategoryReverse  UpstreamCategory = "reverse"
)

// PassthroughProfile selects a preset of 7-toggle values.
type PassthroughProfile string

const (
	ProfileTransparent PassthroughProfile = "transparent"
	ProfileProtected   PassthroughProfile = "protected"
	ProfileStrict      PassthroughProfile = "strict"
)

// GlobalOverrideMode is the emergency kill switch. When non-auto, all
// per-account/per-profile resolution is bypassed.
type GlobalOverrideMode string

const (
	GlobalOverrideAuto             GlobalOverrideMode = "auto"
	GlobalOverrideForceTransparent GlobalOverrideMode = "force_transparent"
	GlobalOverrideForceProtected   GlobalOverrideMode = "force_protected"
	GlobalOverrideForceStrict      GlobalOverrideMode = "force_strict"
)

// ParseGlobalOverrideMode normalizes a raw string from settings.
// Invalid or empty returns GlobalOverrideAuto (fail-open to per-account resolution).
func ParseGlobalOverrideMode(s string) GlobalOverrideMode {
	switch GlobalOverrideMode(strings.ToLower(strings.TrimSpace(s))) {
	case GlobalOverrideForceTransparent:
		return GlobalOverrideForceTransparent
	case GlobalOverrideForceProtected:
		return GlobalOverrideForceProtected
	case GlobalOverrideForceStrict:
		return GlobalOverrideForceStrict
	default:
		return GlobalOverrideAuto
	}
}

// ToProfile converts a global override to the equivalent profile (only for
// the force_* values). Auto returns empty string to signal "fall through".
func (g GlobalOverrideMode) ToProfile() PassthroughProfile {
	switch g {
	case GlobalOverrideForceTransparent:
		return ProfileTransparent
	case GlobalOverrideForceProtected:
		return ProfileProtected
	case GlobalOverrideForceStrict:
		return ProfileStrict
	default:
		return ""
	}
}

// PassthroughToggles is the 7-toggle representation used everywhere downstream.
// A "true" value means "skip protection / forward as-is".
type PassthroughToggles struct {
	ForwardClientHeaders   bool
	ForwardUserNetworkInfo bool
	SkipBodyScrub          bool
	SkipSystemPromptInject bool
	ForwardClientUA        bool
	ForwardBetaFlags       bool
	SkipModelRewrite       bool
}

// Toggle names used in serialized overrides (account.Extra and system defaults).
// Names match the JSON field names produced by the frontend three-state controls.
const (
	ToggleForwardClientHeaders   = "forward_client_headers"
	ToggleForwardUserNetworkInfo = "forward_user_network_info"
	ToggleSkipBodyScrub          = "skip_body_scrub"
	ToggleSkipSystemPromptInject = "skip_system_prompt_inject"
	ToggleForwardClientUA        = "forward_client_ua"
	ToggleForwardBetaFlags       = "forward_beta_flags"
	ToggleSkipModelRewrite       = "skip_model_rewrite"
)

// Apply merges a sparse overrides map into the toggle struct. Unknown keys are silently ignored.
func (t *PassthroughToggles) Apply(overrides map[string]bool) {
	if t == nil || len(overrides) == 0 {
		return
	}
	if v, ok := overrides[ToggleForwardClientHeaders]; ok {
		t.ForwardClientHeaders = v
	}
	if v, ok := overrides[ToggleForwardUserNetworkInfo]; ok {
		t.ForwardUserNetworkInfo = v
	}
	if v, ok := overrides[ToggleSkipBodyScrub]; ok {
		t.SkipBodyScrub = v
	}
	if v, ok := overrides[ToggleSkipSystemPromptInject]; ok {
		t.SkipSystemPromptInject = v
	}
	if v, ok := overrides[ToggleForwardClientUA]; ok {
		t.ForwardClientUA = v
	}
	if v, ok := overrides[ToggleForwardBetaFlags]; ok {
		t.ForwardBetaFlags = v
	}
	if v, ok := overrides[ToggleSkipModelRewrite]; ok {
		t.SkipModelRewrite = v
	}
}

var profileTogglesByName = map[PassthroughProfile]PassthroughToggles{
	ProfileTransparent: {
		ForwardClientHeaders:   true,
		ForwardUserNetworkInfo: true,
		SkipBodyScrub:          true,
		SkipSystemPromptInject: true,
		ForwardClientUA:        true,
		ForwardBetaFlags:       true,
		SkipModelRewrite:       true,
	},
	ProfileProtected: {
		// All protections ON except: system prompt inject skipped (user owns their system prompt)
		ForwardClientHeaders:   false,
		ForwardUserNetworkInfo: false,
		SkipBodyScrub:          false,
		SkipSystemPromptInject: true,
		ForwardClientUA:        false,
		ForwardBetaFlags:       false,
		SkipModelRewrite:       false,
	},
	ProfileStrict: {
		// All protections ON (including system prompt inject — required for reverse clients)
		ForwardClientHeaders:   false,
		ForwardUserNetworkInfo: false,
		SkipBodyScrub:          false,
		SkipSystemPromptInject: false,
		ForwardClientUA:        false,
		ForwardBetaFlags:       false,
		SkipModelRewrite:       false,
	},
}

// ProfileToggleValues returns a copy of the 7-toggle preset for the given profile.
// Unknown profile returns Protected as a safe default.
func ProfileToggleValues(p PassthroughProfile) PassthroughToggles {
	if v, ok := profileTogglesByName[p]; ok {
		return v
	}
	return profileTogglesByName[ProfileProtected]
}

// CategoryDefaultProfile returns the recommended default profile for a category.
// Used as the fallback when system defaults are absent.
// Unknown category returns Protected (safe fallback).
func CategoryDefaultProfile(c UpstreamCategory) PassthroughProfile {
	switch c {
	case CategoryRelay:
		return ProfileTransparent
	case CategoryOfficial:
		return ProfileProtected
	case CategoryReverse:
		return ProfileStrict
	default:
		return ProfileProtected
	}
}

// UpstreamPassthroughCategoryDefault is one category's slice of the system default.
type UpstreamPassthroughCategoryDefault struct {
	Profile   PassthroughProfile `json:"profile"`
	Overrides map[string]bool    `json:"overrides"`
}

// UpstreamPassthroughDefaults is the JSON shape stored in settings under
// SettingKeyUpstreamPassthroughDefaults. Each category has a profile + sparse
// per-toggle overrides applied on top of the profile.
type UpstreamPassthroughDefaults struct {
	Relay    UpstreamPassthroughCategoryDefault `json:"relay"`
	Official UpstreamPassthroughCategoryDefault `json:"official"`
	Reverse  UpstreamPassthroughCategoryDefault `json:"reverse"`
}

// For returns the category-specific defaults. Empty profile in a category
// slot is filled with that category's recommended default profile.
func (d UpstreamPassthroughDefaults) For(c UpstreamCategory) UpstreamPassthroughCategoryDefault {
	var slot UpstreamPassthroughCategoryDefault
	switch c {
	case CategoryRelay:
		slot = d.Relay
	case CategoryOfficial:
		slot = d.Official
	case CategoryReverse:
		slot = d.Reverse
	default:
		slot = d.Official
	}
	if slot.Profile == "" {
		slot.Profile = CategoryDefaultProfile(c)
	}
	return slot
}

// DefaultUpstreamPassthroughDefaults returns the code-baked defaults used when
// the setting row is absent or fails to parse. Matches CategoryDefaultProfile
// for each category, with empty per-toggle overrides.
func DefaultUpstreamPassthroughDefaults() UpstreamPassthroughDefaults {
	return UpstreamPassthroughDefaults{
		Relay:    UpstreamPassthroughCategoryDefault{Profile: ProfileTransparent},
		Official: UpstreamPassthroughCategoryDefault{Profile: ProfileProtected},
		Reverse:  UpstreamPassthroughCategoryDefault{Profile: ProfileStrict},
	}
}

// EffectiveUpstreamPolicy is the output of ResolveUpstreamPassthroughPolicy.
// All downstream gateway code (Phase B) reads from this struct.
type EffectiveUpstreamPolicy struct {
	// 7 toggles
	ForwardClientHeaders   bool
	ForwardUserNetworkInfo bool
	SkipBodyScrub          bool
	SkipSystemPromptInject bool
	ForwardClientUA        bool
	ForwardBetaFlags       bool
	SkipModelRewrite       bool

	// Signal fields (for logs / passthrough decisions outside the 7 toggles)
	PrivacyMode          string // training_off / privacy_set / ""
	Category             UpstreamCategory
	ProfileApplied       PassthroughProfile
	GlobalOverrideActive bool

	// Read-through rate fields (Phase 2 will make these category-aware)
	AccountConcurrency int
	AccountBaseRPM     int
}
