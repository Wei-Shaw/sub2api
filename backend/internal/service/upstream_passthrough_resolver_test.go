package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveUpstreamPassthroughPolicy(t *testing.T) {
	defaults := DefaultUpstreamPassthroughDefaults() // code constants

	t.Run("nil account returns Protected", func(t *testing.T) {
		got := ResolveUpstreamPassthroughPolicy(nil, &defaults, GlobalOverrideAuto)
		require.Equal(t, ProfileProtected, got.ProfileApplied)
		require.Equal(t, CategoryOfficial, got.Category)
		require.False(t, got.GlobalOverrideActive)
		expected := ProfileToggleValues(ProfileProtected)
		require.Equal(t, expected.SkipBodyScrub, got.SkipBodyScrub)
	})

	t.Run("relay account with no overrides → Transparent", func(t *testing.T) {
		a := &Account{Type: AccountTypeUpstream, Platform: PlatformAnthropic}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, CategoryRelay, got.Category)
		require.Equal(t, ProfileTransparent, got.ProfileApplied)
		require.True(t, got.ForwardClientHeaders)
		require.True(t, got.ForwardUserNetworkInfo)
		require.True(t, got.SkipBodyScrub)
	})

	t.Run("official account (APIKey) with no overrides → Protected", func(t *testing.T) {
		a := &Account{Type: AccountTypeAPIKey, Platform: PlatformAnthropic}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, CategoryOfficial, got.Category)
		require.Equal(t, ProfileProtected, got.ProfileApplied)
		require.False(t, got.ForwardClientHeaders)
		require.False(t, got.ForwardUserNetworkInfo)
		require.True(t, got.SkipSystemPromptInject) // protected skips inject
	})

	t.Run("reverse account (Kiro) with no overrides → Strict", func(t *testing.T) {
		a := &Account{Type: AccountTypeOAuth, Platform: PlatformKiro}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, CategoryReverse, got.Category)
		require.Equal(t, ProfileStrict, got.ProfileApplied)
		require.False(t, got.SkipSystemPromptInject) // strict injects
	})

	t.Run("global kill switch force_protected overrides everything", func(t *testing.T) {
		a := &Account{Type: AccountTypeUpstream, Platform: PlatformAnthropic} // would be relay
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideForceProtected)
		require.True(t, got.GlobalOverrideActive)
		require.Equal(t, ProfileProtected, got.ProfileApplied)
		require.False(t, got.ForwardClientHeaders) // forced protected behavior
	})

	t.Run("global kill switch force_strict beats account profile=transparent", func(t *testing.T) {
		a := &Account{
			Type:     AccountTypeUpstream,
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"upstream_passthrough": map[string]any{"profile": "transparent"},
			},
		}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideForceStrict)
		require.True(t, got.GlobalOverrideActive)
		require.Equal(t, ProfileStrict, got.ProfileApplied)
		require.False(t, got.ForwardClientHeaders)
		require.False(t, got.SkipSystemPromptInject)
	})

	t.Run("account profile override beats category default", func(t *testing.T) {
		a := &Account{
			Type:     AccountTypeAPIKey, // official by default
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"upstream_passthrough": map[string]any{"profile": "transparent"},
			},
		}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, CategoryOfficial, got.Category)         // category unchanged
		require.Equal(t, ProfileTransparent, got.ProfileApplied) // profile overridden
		require.True(t, got.ForwardClientHeaders)
	})

	t.Run("account category_override sends through different category default", func(t *testing.T) {
		// APIKey/Anthropic is official by derivation; override to relay
		a := &Account{
			Type:     AccountTypeAPIKey,
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"upstream_passthrough": map[string]any{"category_override": "relay"},
			},
		}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, CategoryRelay, got.Category)
		require.Equal(t, ProfileTransparent, got.ProfileApplied) // relay default
		require.True(t, got.ForwardClientHeaders)
	})

	t.Run("sparse toggle override layered on profile", func(t *testing.T) {
		a := &Account{
			Type:     AccountTypeOAuth,
			Platform: PlatformKiro, // reverse → Strict
			Extra: map[string]any{
				"upstream_passthrough": map[string]any{
					"overrides": map[string]any{
						"forward_client_ua": true, // override only this toggle
					},
				},
			},
		}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, ProfileStrict, got.ProfileApplied)
		// All toggles default to Strict values, except forward_client_ua overridden ON
		require.False(t, got.ForwardClientHeaders)   // strict default
		require.False(t, got.SkipBodyScrub)          // strict default
		require.True(t, got.ForwardClientUA)         // overridden
		require.False(t, got.SkipSystemPromptInject) // strict default
	})

	t.Run("profile + sparse override combined", func(t *testing.T) {
		a := &Account{
			Type:     AccountTypeUpstream,
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"upstream_passthrough": map[string]any{
					"profile": "protected",
					"overrides": map[string]any{
						"skip_body_scrub": true, // override on top of protected
					},
				},
			},
		}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, CategoryRelay, got.Category)
		require.Equal(t, ProfileProtected, got.ProfileApplied)
		require.False(t, got.ForwardClientHeaders) // protected default
		require.True(t, got.SkipBodyScrub)         // overridden
	})

	t.Run("system defaults override codeConstants for category", func(t *testing.T) {
		customDefaults := UpstreamPassthroughDefaults{
			Relay:    UpstreamPassthroughCategoryDefault{Profile: ProfileStrict}, // admin made relay strict
			Official: UpstreamPassthroughCategoryDefault{Profile: ProfileProtected},
			Reverse:  UpstreamPassthroughCategoryDefault{Profile: ProfileStrict},
		}
		a := &Account{Type: AccountTypeUpstream, Platform: PlatformAnthropic} // relay
		got := ResolveUpstreamPassthroughPolicy(a, &customDefaults, GlobalOverrideAuto)
		require.Equal(t, CategoryRelay, got.Category)
		require.Equal(t, ProfileStrict, got.ProfileApplied) // not Transparent because admin overrode
		require.False(t, got.ForwardClientHeaders)
	})

	t.Run("system defaults with per-toggle override", func(t *testing.T) {
		customDefaults := UpstreamPassthroughDefaults{
			Relay: UpstreamPassthroughCategoryDefault{
				Profile: ProfileTransparent,
				Overrides: map[string]bool{
					"forward_user_network_info": false, // admin: even relay shouldn't forward IPs
				},
			},
			Official: UpstreamPassthroughCategoryDefault{Profile: ProfileProtected},
			Reverse:  UpstreamPassthroughCategoryDefault{Profile: ProfileStrict},
		}
		a := &Account{Type: AccountTypeUpstream, Platform: PlatformAnthropic}
		got := ResolveUpstreamPassthroughPolicy(a, &customDefaults, GlobalOverrideAuto)
		require.True(t, got.ForwardClientHeaders)    // transparent default
		require.False(t, got.ForwardUserNetworkInfo) // system override
		require.True(t, got.SkipBodyScrub)           // transparent default
	})

	t.Run("legacy anthropic_passthrough=true fills toggles when no new fields set", func(t *testing.T) {
		a := &Account{
			Type:     AccountTypeAPIKey,
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"anthropic_passthrough": true,
			},
		}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, CategoryOfficial, got.Category)
		require.True(t, got.ForwardClientHeaders)
		require.True(t, got.SkipBodyScrub)
		require.True(t, got.ForwardBetaFlags)
		require.False(t, got.ForwardUserNetworkInfo) // legacy doesn't forward IPs
	})

	t.Run("new fields beat legacy fields", func(t *testing.T) {
		a := &Account{
			Type:     AccountTypeAPIKey,
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"anthropic_passthrough": true,
				"upstream_passthrough": map[string]any{
					"profile": "protected",
				},
			},
		}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, ProfileProtected, got.ProfileApplied)
		require.False(t, got.ForwardClientHeaders) // protected default wins, not legacy true
	})

	t.Run("privacy_mode passes through as signal", func(t *testing.T) {
		a := &Account{
			Type:     AccountTypeOAuth,
			Platform: PlatformOpenAI,
			Extra:    map[string]any{"privacy_mode": "training_off"},
		}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, "training_off", got.PrivacyMode)
	})

	t.Run("AccountConcurrency and AccountBaseRPM pass through as read-through", func(t *testing.T) {
		baseRPM := 50
		a := &Account{
			Type:        AccountTypeAPIKey,
			Platform:    PlatformAnthropic,
			Concurrency: 7,
			Extra:       map[string]any{"base_rpm": baseRPM},
		}
		got := ResolveUpstreamPassthroughPolicy(a, &defaults, GlobalOverrideAuto)
		require.Equal(t, 7, got.AccountConcurrency)
		require.Equal(t, baseRPM, got.AccountBaseRPM)
	})

	t.Run("nil defaults uses code defaults", func(t *testing.T) {
		a := &Account{Type: AccountTypeAPIKey, Platform: PlatformAnthropic}
		got := ResolveUpstreamPassthroughPolicy(a, nil, GlobalOverrideAuto)
		require.Equal(t, CategoryOfficial, got.Category)
		require.Equal(t, ProfileProtected, got.ProfileApplied)
	})
}

func TestResolveUpstreamPassthroughPolicyWithGroup(t *testing.T) {
	defaults := DefaultUpstreamPassthroughDefaults()

	t.Run("group profile inherited when account has no override", func(t *testing.T) {
		// reverse account would default to Strict; group forces transparent.
		a := &Account{Type: AccountTypeOAuth, Platform: PlatformKiro}
		g := &Group{ID: 1, Hydrated: true, Platform: PlatformAnthropic, Status: "active",
			DefaultPassthroughProfile: string(ProfileTransparent)}
		got := ResolveUpstreamPassthroughPolicyWithGroup(a, g, &defaults, GlobalOverrideAuto)
		require.Equal(t, CategoryReverse, got.Category)
		require.Equal(t, ProfileTransparent, got.ProfileApplied)
		require.True(t, got.ForwardClientHeaders)
	})

	t.Run("account profile beats group profile", func(t *testing.T) {
		a := &Account{
			Type:     AccountTypeOAuth,
			Platform: PlatformKiro,
			Extra: map[string]any{
				"upstream_passthrough": map[string]any{"profile": "protected"},
			},
		}
		g := &Group{ID: 1, Hydrated: true, Platform: PlatformAnthropic, Status: "active",
			DefaultPassthroughProfile: string(ProfileTransparent)}
		got := ResolveUpstreamPassthroughPolicyWithGroup(a, g, &defaults, GlobalOverrideAuto)
		require.Equal(t, ProfileProtected, got.ProfileApplied) // account wins
	})

	t.Run("account sparse toggle still layered on top of group profile", func(t *testing.T) {
		a := &Account{
			Type:     AccountTypeOAuth,
			Platform: PlatformKiro,
			Extra: map[string]any{
				"upstream_passthrough": map[string]any{
					"overrides": map[string]any{"forward_client_ua": true},
				},
			},
		}
		g := &Group{ID: 1, Hydrated: true, Platform: PlatformAnthropic, Status: "active",
			DefaultPassthroughProfile: string(ProfileProtected)}
		got := ResolveUpstreamPassthroughPolicyWithGroup(a, g, &defaults, GlobalOverrideAuto)
		require.Equal(t, ProfileProtected, got.ProfileApplied) // group profile
		require.True(t, got.ForwardClientUA)                   // account toggle override
		require.False(t, got.ForwardClientHeaders)             // protected default
	})

	t.Run("invalid/empty group profile falls through to category default", func(t *testing.T) {
		a := &Account{Type: AccountTypeOAuth, Platform: PlatformKiro}
		g := &Group{ID: 1, Hydrated: true, Platform: PlatformAnthropic, Status: "active",
			DefaultPassthroughProfile: "bogus"}
		got := ResolveUpstreamPassthroughPolicyWithGroup(a, g, &defaults, GlobalOverrideAuto)
		require.Equal(t, ProfileStrict, got.ProfileApplied) // reverse default
	})

	t.Run("nil group behaves like base resolver", func(t *testing.T) {
		a := &Account{Type: AccountTypeOAuth, Platform: PlatformKiro}
		got := ResolveUpstreamPassthroughPolicyWithGroup(a, nil, &defaults, GlobalOverrideAuto)
		require.Equal(t, ProfileStrict, got.ProfileApplied)
	})

	t.Run("kill switch beats group profile", func(t *testing.T) {
		a := &Account{Type: AccountTypeOAuth, Platform: PlatformKiro}
		g := &Group{ID: 1, Hydrated: true, Platform: PlatformAnthropic, Status: "active",
			DefaultPassthroughProfile: string(ProfileTransparent)}
		got := ResolveUpstreamPassthroughPolicyWithGroup(a, g, &defaults, GlobalOverrideForceStrict)
		require.True(t, got.GlobalOverrideActive)
		require.Equal(t, ProfileStrict, got.ProfileApplied)
	})
}
