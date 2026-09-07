package claude

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fable51MinCLIVersion 是上游对 claude-fable-5-1 的客户端版本闸门下限。
// 低于该版本时上游直接 400：
//
//	claude_code_version_too_old
//	"Claude Code <ver> does not support this model; version 2.1.251 or newer is required."
const fable51MinCLIVersion = "2.1.251"

// TestCLICurrentVersionMatchesDefaultUserAgent 锁定"常量与 UA 同源"这条约束。
// 两者不一致会被 Anthropic 判为第三方客户端，且只改一处是历史上的常见疏漏，
// 所以这里从 UA 里反解版本号来比对，而不是并列两个字面量。
func TestCLICurrentVersionMatchesDefaultUserAgent(t *testing.T) {
	t.Parallel()

	ua := DefaultHeaders["User-Agent"]
	const prefix = "claude-cli/"
	require.True(t, strings.HasPrefix(ua, prefix), "unexpected User-Agent: %q", ua)

	rest := strings.TrimPrefix(ua, prefix)
	version := rest
	if idx := strings.IndexByte(rest, ' '); idx >= 0 {
		version = rest[:idx]
	}
	require.Equal(t, CLICurrentVersion, version,
		"DefaultHeaders User-Agent version must match CLICurrentVersion")
}

// TestCLICurrentVersionSatisfiesFable51Gate 锁定伪装版本不低于 claude-fable-5-1
// 的闸门下限。DefaultModels 里登记了该模型却发着更旧的版本号，等于模型清单可见
// 但一调就 400。
func TestCLICurrentVersionSatisfiesFable51Gate(t *testing.T) {
	t.Parallel()

	require.GreaterOrEqual(t, compareSemver(t, CLICurrentVersion, fable51MinCLIVersion), 0,
		"CLICurrentVersion %s is below the claude-fable-5-1 gate %s",
		CLICurrentVersion, fable51MinCLIVersion)
}

// compareSemver 比较两个三段 semver，返回 -1 / 0 / 1。
func compareSemver(t *testing.T, a, b string) int {
	t.Helper()

	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	require.Len(t, as, 3, "not a three-part semver: %q", a)
	require.Len(t, bs, 3, "not a three-part semver: %q", b)

	for i := range as {
		ai, err := strconv.Atoi(as[i])
		require.NoError(t, err, "non-numeric segment in %q", a)
		bi, err := strconv.Atoi(bs[i])
		require.NoError(t, err, "non-numeric segment in %q", b)

		switch {
		case ai > bi:
			return 1
		case ai < bi:
			return -1
		}
	}
	return 0
}
