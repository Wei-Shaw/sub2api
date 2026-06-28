package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchClientEntries_WhitelistAND(t *testing.T) {
	wl := []AllowedClientEntry{{Originator: "opencode", UAContains: []string{"opencode/"}}}
	require.True(t, MatchClientEntries("opencode/1.2 (x)", "opencode", wl))
	require.False(t, MatchClientEntries("opencode/1.2 (x)", "other", wl), "originator 不符不放")
	require.False(t, MatchClientEntries("curl/8", "opencode", wl), "UA marker 缺失不放")
	require.False(t, MatchClientEntries("opencode/1.2", "opencode", []AllowedClientEntry{{Originator: "opencode"}}), "空 UAContains 安全失败")
}

func TestDenyEntries_BlacklistOR(t *testing.T) {
	bl := []AllowedClientEntry{
		{Originator: "evilbot"},
		{UAContains: []string{"badscan/"}},
	}
	require.True(t, MatchDenyEntries("anything/1", "evilbot", bl), "originator 命中即拒")
	require.True(t, MatchDenyEntries("badscan/9 (x)", "whatever", bl), "UA marker 命中即拒")
	require.False(t, MatchDenyEntries("codex_cli_rs/0.141.0", "codex_cli_rs", bl), "都不命中不拒")
	require.False(t, MatchDenyEntries("x", "y", []AllowedClientEntry{{}}), "全空条目安全忽略")
}
