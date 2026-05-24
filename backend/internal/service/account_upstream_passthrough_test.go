package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_GetUpstreamPassthroughProfile(t *testing.T) {
	t.Run("empty when extra absent", func(t *testing.T) {
		a := &Account{}
		require.Equal(t, PassthroughProfile(""), a.GetUpstreamPassthroughProfile())
	})

	t.Run("empty when profile field missing", func(t *testing.T) {
		a := &Account{Extra: map[string]any{"upstream_passthrough": map[string]any{}}}
		require.Equal(t, PassthroughProfile(""), a.GetUpstreamPassthroughProfile())
	})

	t.Run("returns valid profile string", func(t *testing.T) {
		a := &Account{Extra: map[string]any{
			"upstream_passthrough": map[string]any{"profile": "transparent"},
		}}
		require.Equal(t, ProfileTransparent, a.GetUpstreamPassthroughProfile())
	})

	t.Run("normalizes case and whitespace", func(t *testing.T) {
		a := &Account{Extra: map[string]any{
			"upstream_passthrough": map[string]any{"profile": "  STRICT  "},
		}}
		require.Equal(t, ProfileStrict, a.GetUpstreamPassthroughProfile())
	})

	t.Run("invalid profile returns empty", func(t *testing.T) {
		a := &Account{Extra: map[string]any{
			"upstream_passthrough": map[string]any{"profile": "garbage"},
		}}
		require.Equal(t, PassthroughProfile(""), a.GetUpstreamPassthroughProfile())
	})

	t.Run("nil account safe", func(t *testing.T) {
		var a *Account
		require.Equal(t, PassthroughProfile(""), a.GetUpstreamPassthroughProfile())
	})
}

func TestAccount_GetUpstreamPassthroughOverrides(t *testing.T) {
	t.Run("empty map when absent", func(t *testing.T) {
		a := &Account{}
		require.Empty(t, a.GetUpstreamPassthroughOverrides())
	})

	t.Run("returns only known toggle keys", func(t *testing.T) {
		a := &Account{Extra: map[string]any{
			"upstream_passthrough": map[string]any{
				"overrides": map[string]any{
					"forward_client_headers":    true,
					"skip_body_scrub":           false,
					"forward_user_network_info": true,
					"unknown_toggle":            true, // ignored
				},
			},
		}}
		got := a.GetUpstreamPassthroughOverrides()
		require.Len(t, got, 3)
		require.True(t, got[ToggleForwardClientHeaders])
		require.False(t, got[ToggleSkipBodyScrub])
		require.True(t, got[ToggleForwardUserNetworkInfo])
		_, hasUnknown := got["unknown_toggle"]
		require.False(t, hasUnknown)
	})

	t.Run("ignores non-bool override values", func(t *testing.T) {
		a := &Account{Extra: map[string]any{
			"upstream_passthrough": map[string]any{
				"overrides": map[string]any{
					"forward_client_headers": "yes", // not a bool
				},
			},
		}}
		got := a.GetUpstreamPassthroughOverrides()
		require.Empty(t, got)
	})

	t.Run("nil account safe", func(t *testing.T) {
		var a *Account
		require.Empty(t, a.GetUpstreamPassthroughOverrides())
	})
}

func TestAccount_GetUpstreamPassthroughCategoryOverride(t *testing.T) {
	t.Run("empty when absent", func(t *testing.T) {
		a := &Account{}
		require.Equal(t, UpstreamCategory(""), a.GetUpstreamPassthroughCategoryOverride())
	})

	t.Run("returns valid override", func(t *testing.T) {
		a := &Account{Extra: map[string]any{
			"upstream_passthrough": map[string]any{"category_override": "relay"},
		}}
		require.Equal(t, CategoryRelay, a.GetUpstreamPassthroughCategoryOverride())
	})

	t.Run("invalid returns empty", func(t *testing.T) {
		a := &Account{Extra: map[string]any{
			"upstream_passthrough": map[string]any{"category_override": "nope"},
		}}
		require.Equal(t, UpstreamCategory(""), a.GetUpstreamPassthroughCategoryOverride())
	})
}
