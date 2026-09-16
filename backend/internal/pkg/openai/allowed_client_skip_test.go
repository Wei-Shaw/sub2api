package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchClientEntry_ReturnsHitEntry(t *testing.T) {
	entries := []AllowedClientEntry{
		{Originator: "opencode", UAContains: []string{"opencode/"}, SkipEngineFingerprint: true},
		{Originator: "Claude Code", UAContains: []string{"Claude Code/"}},
	}

	e, ok := MatchClientEntry("opencode/1.0", "opencode", entries)
	require.True(t, ok)
	require.True(t, e.SkipEngineFingerprint)

	e2, ok2 := MatchClientEntry("Claude Code/1.0 (x) (Claude Code; 1)", "Claude Code", entries)
	require.True(t, ok2)
	require.False(t, e2.SkipEngineFingerprint)

	_, ok3 := MatchClientEntry("curl/8", "evil", entries)
	require.False(t, ok3)

	// 薄封装保持兼容
	require.True(t, MatchClientEntries("opencode/1.0", "opencode", entries))
	require.False(t, MatchClientEntries("curl/8", "evil", entries))
}
