package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileConstants(t *testing.T) {
	t.Run("Transparent profile is all-skip", func(t *testing.T) {
		p := ProfileToggleValues(ProfileTransparent)
		require.True(t, p.ForwardClientHeaders)
		require.True(t, p.ForwardUserNetworkInfo)
		require.True(t, p.SkipBodyScrub)
		require.True(t, p.SkipSystemPromptInject)
		require.True(t, p.ForwardClientUA)
		require.True(t, p.ForwardBetaFlags)
		require.True(t, p.SkipModelRewrite)
	})

	t.Run("Protected profile skips only system prompt inject", func(t *testing.T) {
		p := ProfileToggleValues(ProfileProtected)
		require.False(t, p.ForwardClientHeaders)
		require.False(t, p.ForwardUserNetworkInfo)
		require.False(t, p.SkipBodyScrub)
		require.True(t, p.SkipSystemPromptInject) // ONLY differs from Strict here
		require.False(t, p.ForwardClientUA)
		require.False(t, p.ForwardBetaFlags)
		require.False(t, p.SkipModelRewrite)
	})

	t.Run("Strict profile is all-protect", func(t *testing.T) {
		p := ProfileToggleValues(ProfileStrict)
		require.False(t, p.ForwardClientHeaders)
		require.False(t, p.ForwardUserNetworkInfo)
		require.False(t, p.SkipBodyScrub)
		require.False(t, p.SkipSystemPromptInject)
		require.False(t, p.ForwardClientUA)
		require.False(t, p.ForwardBetaFlags)
		require.False(t, p.SkipModelRewrite)
	})

	t.Run("Unknown profile returns Protected as safe default", func(t *testing.T) {
		p := ProfileToggleValues(PassthroughProfile("nonexistent"))
		require.Equal(t, ProfileToggleValues(ProfileProtected), p)
	})
}

func TestCategoryDefaultProfile(t *testing.T) {
	require.Equal(t, ProfileTransparent, CategoryDefaultProfile(CategoryRelay))
	require.Equal(t, ProfileProtected, CategoryDefaultProfile(CategoryOfficial))
	require.Equal(t, ProfileStrict, CategoryDefaultProfile(CategoryReverse))
	require.Equal(t, ProfileProtected, CategoryDefaultProfile(UpstreamCategory("garbage"))) // safe fallback
}

func TestUpstreamPassthroughDefaults_JSONRoundTrip(t *testing.T) {
	original := UpstreamPassthroughDefaults{
		Relay: UpstreamPassthroughCategoryDefault{
			Profile: ProfileTransparent,
			Overrides: map[string]bool{
				"skip_body_scrub": true,
			},
		},
		Official: UpstreamPassthroughCategoryDefault{
			Profile:   ProfileProtected,
			Overrides: map[string]bool{},
		},
		Reverse: UpstreamPassthroughCategoryDefault{
			Profile:   ProfileStrict,
			Overrides: map[string]bool{},
		},
	}

	raw, err := json.Marshal(original)
	require.NoError(t, err)

	var restored UpstreamPassthroughDefaults
	require.NoError(t, json.Unmarshal(raw, &restored))
	require.Equal(t, original, restored)
}

func TestUpstreamPassthroughDefaults_ZeroValueIsAllProtected(t *testing.T) {
	// Zero value of defaults should map every category to a default profile,
	// so that "settings absent" mode returns sensible category-aware behavior.
	var d UpstreamPassthroughDefaults
	require.Equal(t, ProfileTransparent, d.For(CategoryRelay).Profile)
	require.Equal(t, ProfileProtected, d.For(CategoryOfficial).Profile)
	require.Equal(t, ProfileStrict, d.For(CategoryReverse).Profile)
}

func TestParseGlobalOverrideMode(t *testing.T) {
	cases := []struct {
		in   string
		want GlobalOverrideMode
	}{
		{"", GlobalOverrideAuto},
		{"auto", GlobalOverrideAuto},
		{"force_transparent", GlobalOverrideForceTransparent},
		{"force_protected", GlobalOverrideForceProtected},
		{"force_strict", GlobalOverrideForceStrict},
		{"garbage", GlobalOverrideAuto}, // invalid -> auto
		{"FORCE_STRICT", GlobalOverrideForceStrict}, // case-insensitive
	}
	for _, c := range cases {
		require.Equal(t, c.want, ParseGlobalOverrideMode(c.in), "input=%q", c.in)
	}
}
