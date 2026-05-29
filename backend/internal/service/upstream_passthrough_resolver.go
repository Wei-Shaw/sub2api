package service

// ResolveUpstreamPassthroughPolicy is a pure function that computes the
// effective 7-toggle upstream policy for a request, given the account and
// system-level settings. Inputs may be nil for defensive fallback paths.
//
// Resolution order (high → low precedence):
//  1. Global kill switch (defaults to auto = pass through)
//  2. Per-account sparse toggle overrides (account.Extra.upstream_passthrough.overrides)
//  3. Per-account profile override (account.Extra.upstream_passthrough.profile)
//  4. Per-group default profile (Group.DefaultPassthroughProfile) — M2 三级覆盖中间层
//  5. Per-account category override → chooses which slot of system defaults to apply
//  6. System defaults for the (derived or overridden) category
//  7. Legacy fields (anthropic_passthrough / openai_passthrough) — only when nothing above set
//  8. Code constants (DefaultUpstreamPassthroughDefaults)
//
// The function never returns an error. Unknown / malformed inputs degrade
// gracefully to the safest non-Strict default (Protected).
func ResolveUpstreamPassthroughPolicy(
	account *Account,
	defaults *UpstreamPassthroughDefaults,
	globalOverride GlobalOverrideMode,
) EffectiveUpstreamPolicy {
	return ResolveUpstreamPassthroughPolicyWithGroup(account, nil, defaults, globalOverride)
}

// ResolveUpstreamPassthroughPolicyWithGroup is the three-level variant that
// accepts an optional *Group to provide the group-level profile default.
// group may be nil (equivalent to calling ResolveUpstreamPassthroughPolicy).
func ResolveUpstreamPassthroughPolicyWithGroup(
	account *Account,
	group *Group,
	defaults *UpstreamPassthroughDefaults,
	globalOverride GlobalOverrideMode,
) EffectiveUpstreamPolicy {
	policy := EffectiveUpstreamPolicy{}

	// Step 1: kill switch short-circuits everything
	if forced := globalOverride.ToProfile(); forced != "" {
		toggles := ProfileToggleValues(forced)
		fillFromToggles(&policy, toggles)
		policy.ProfileApplied = forced
		policy.Category = DeriveUpstreamCategory(account) // still record for logs
		policy.GlobalOverrideActive = true
		fillSignals(&policy, account)
		return policy
	}

	// Step 2: derive (or read override) category
	policy.Category = DeriveUpstreamCategory(account)

	// Step 3: pick the category default profile (from system settings, or code constants)
	effDefaults := defaults
	if effDefaults == nil {
		d := DefaultUpstreamPassthroughDefaults()
		effDefaults = &d
	}
	categorySlot := effDefaults.For(policy.Category)

	// Step 4: account profile override beats group profile, which beats category slot.
	acctProfile := getAccountProfile(account)
	groupProfile := getGroupProfile(group)
	effectiveProfile := acctProfile
	if effectiveProfile == "" {
		effectiveProfile = groupProfile
	}
	if effectiveProfile != "" {
		categorySlot = UpstreamPassthroughCategoryDefault{
			Profile:   effectiveProfile,
			Overrides: nil,
		}
	}

	// Step 5: start from profile toggles
	toggles := ProfileToggleValues(categorySlot.Profile)

	// Step 6: layer the system-default per-toggle overrides (these are sparse)
	toggles.Apply(categorySlot.Overrides)

	// Step 7: layer the account's sparse per-toggle overrides (highest non-kill-switch precedence)
	accountOverrides := getAccountOverrides(account)
	toggles.Apply(accountOverrides)

	// Step 8: legacy fallback ONLY if no new fields contributed
	applyLegacyFallback(&toggles, account, acctProfile, accountOverrides, categorySlot.Overrides)

	fillFromToggles(&policy, toggles)
	policy.ProfileApplied = categorySlot.Profile
	fillSignals(&policy, account)
	return policy
}

func getAccountProfile(a *Account) PassthroughProfile {
	if a == nil {
		return ""
	}
	return a.GetUpstreamPassthroughProfile()
}

func getAccountOverrides(a *Account) map[string]bool {
	if a == nil {
		return nil
	}
	return a.GetUpstreamPassthroughOverrides()
}

// getGroupProfile returns the validated profile from Group.DefaultPassthroughProfile.
// Returns "" if group is nil or the profile string is not a known value.
func getGroupProfile(g *Group) PassthroughProfile {
	if g == nil || g.DefaultPassthroughProfile == "" {
		return ""
	}
	switch PassthroughProfile(g.DefaultPassthroughProfile) {
	case ProfileTransparent, ProfileProtected, ProfileStrict:
		return PassthroughProfile(g.DefaultPassthroughProfile)
	default:
		return ""
	}
}

// applyLegacyFallback fills toggles from legacy fields IFF the account has no
// new-field signals at all (no profile override, no toggle overrides, AND the
// system defaults for this category did not set any per-toggle overrides).
func applyLegacyFallback(
	t *PassthroughToggles,
	a *Account,
	accountProf PassthroughProfile,
	accountOverrides map[string]bool,
	systemDefaultOverrides map[string]bool,
) {
	if a == nil {
		return
	}
	if accountProf != "" {
		return
	}
	if len(accountOverrides) > 0 {
		return
	}
	if len(systemDefaultOverrides) > 0 {
		return
	}
	if a.Extra == nil {
		return
	}
	legacy := false
	if v, ok := a.Extra["anthropic_passthrough"].(bool); ok && v {
		legacy = true
	}
	if v, ok := a.Extra["openai_passthrough"].(bool); ok && v {
		legacy = true
	}
	if !legacy {
		return
	}
	// Legacy "passthrough=true" historically meant: forward client headers, forward beta flags,
	// skip body scrub. It did NOT forward user network info or skip system inject — keep those
	// conservative defaults from the profile.
	t.ForwardClientHeaders = true
	t.ForwardBetaFlags = true
	t.SkipBodyScrub = true
}

// fillFromToggles copies a PassthroughToggles into the corresponding fields of
// EffectiveUpstreamPolicy without touching the signal/read-through fields.
func fillFromToggles(p *EffectiveUpstreamPolicy, t PassthroughToggles) {
	p.ForwardClientHeaders = t.ForwardClientHeaders
	p.ForwardUserNetworkInfo = t.ForwardUserNetworkInfo
	p.SkipBodyScrub = t.SkipBodyScrub
	p.SkipSystemPromptInject = t.SkipSystemPromptInject
	p.ForwardClientUA = t.ForwardClientUA
	p.ForwardBetaFlags = t.ForwardBetaFlags
	p.SkipModelRewrite = t.SkipModelRewrite
}

// fillSignals populates non-toggle output fields from the account.
func fillSignals(p *EffectiveUpstreamPolicy, a *Account) {
	if a == nil {
		return
	}
	// Privacy mode is a string; pass through directly
	if a.Extra != nil {
		if v, ok := a.Extra["privacy_mode"].(string); ok {
			p.PrivacyMode = v
		}
	}
	// Account-level concurrency (already a direct field)
	p.AccountConcurrency = a.Concurrency
	// Account-level base_rpm (lives in Extra)
	if a.Extra != nil {
		if v, ok := extractInt(a.Extra["base_rpm"]); ok {
			p.AccountBaseRPM = v
		}
	}
}

// extractInt accepts int, int64, float64, json.Number-shaped values.
func extractInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}
