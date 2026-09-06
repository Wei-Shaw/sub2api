package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// floorClaudeCLIUserAgentVersion 单元测试：版本下限抬升的各种形态。
func TestFloorClaudeCLIUserAgentVersion(t *testing.T) {
	floorUA := "claude-cli/" + claude.CLICurrentVersion
	cases := []struct {
		name        string
		ua          string
		want        string
		wantChanged bool
	}{
		// 线上故障形态：历史版本低于 CLICurrentVersion，就地抬到下限。
		{"old_version_upgraded", "claude-cli/2.1.220 (external, cli)", floorUA + " (external, cli)", true},
		// 只替换版本号段，括号内的真实客户端形态原样保留。
		{"old_version_with_desktop_3p_suffix",
			"claude-cli/2.1.100 (external, claude-desktop-3p, agent-sdk/0.3.100)",
			floorUA + " (external, claude-desktop-3p, agent-sdk/0.3.100)", true},
		// 等于下限：不得改动。
		{"equal_to_floor", floorUA + " (external, cli)", floorUA + " (external, cli)", false},
		// 高于下限：只升不降，不得把客户端上报的更新版本压回去。
		{"newer_than_floor_not_downgraded", "claude-cli/2.9.0 (external, cli)", "claude-cli/2.9.0 (external, cli)", false},
		// 非 claude-cli 产品：一律不动。
		{"other_product_untouched", "opencode/1.2.3 (external, cli)", "opencode/1.2.3 (external, cli)", false},
		// 空串 / 畸形：一律不动。
		{"empty", "", "", false},
		{"no_version", "claude-cli (external, cli)", "claude-cli (external, cli)", false},
		{"unparseable_version", "claude-cli/abc (external, cli)", "claude-cli/abc (external, cli)", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := floorClaudeCLIUserAgentVersion(tc.ua)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantChanged, changed)
		})
	}
}

// GetOrCreateFingerprint 集成行为，直接对应线上故障：缓存指纹停留在历史版本
// 2.1.220（生产 Redis 中账号 147 的实际值），客户端送来更旧的 2.1.75。
// 修复前：isNewerVersion 不触发、UA 形态合法不触发自愈，旧指纹被原样返回并
// 近乎永不过期——上游按指纹 UA 做客户端版本闸门（Fable 5.1 要求 >= 2.1.251），
// 仅升 CLICurrentVersion 对存量账号完全无效。
func TestGetOrCreateFingerprintFloorsStaleCachedUserAgent(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent:               "claude-cli/2.1.220 (external, cli)",
		ClientID:                "cid-1",
		StainlessPackageVersion: "0.91.1",
		UpdatedAt:               time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 147,
		headersWithUA("claude-cli/2.1.75 (external, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/"+claude.CLICurrentVersion+" (external, cli)", fp.UserAgent)
	require.Equal(t, 1, cache.setCalls, "下限抬升必须持久化写回缓存")
	require.Equal(t, "claude-cli/"+claude.CLICurrentVersion+" (external, cli)", cache.lastSet.UserAgent)
	// X-Stainless-* 维持既有 merge 语义：客户端未携带时保留缓存中的真实值，不被下限逻辑覆盖。
	require.Equal(t, "0.91.1", fp.StainlessPackageVersion)
}

// 缓存版本高于下限：不得被降级，也不得触发多余写入。
func TestGetOrCreateFingerprintDoesNotTouchCacheAboveFloor(t *testing.T) {
	ua := "claude-cli/2.9.0 (external, cli)"
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent:               ua,
		ClientID:                "cid-1",
		StainlessPackageVersion: "0.91.1",
		UpdatedAt:               time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithUA("claude-cli/2.1.75 (external, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, ua, fp.UserAgent, "高于下限的缓存版本不得被降级")
	require.Zero(t, cache.setCalls)
}

// 客户端送来高于下限的更新版本：常规升级照常生效，且不被下限压回。
func TestGetOrCreateFingerprintClientNewerThanFloorStillWins(t *testing.T) {
	newUA := "claude-cli/2.9.0 (external, cli)"
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/2.1.220 (external, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, headersWithUA(newUA))

	require.NoError(t, err)
	require.Equal(t, newUA, fp.UserAgent, "客户端更新版本优先于下限，不得被压回")
	require.Equal(t, 1, cache.setCalls)
}

// 客户端版本高于缓存但低于下限：merge 升级后仍被抬到下限（二者取更新者）。
func TestGetOrCreateFingerprintFloorWinsOverStaleClientUpgrade(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/2.1.22 (external, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithUA("claude-cli/2.1.223 (external, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/"+claude.CLICurrentVersion+" (external, cli)", fp.UserAgent)
	require.Equal(t, 1, cache.setCalls)
}

// 首次创建路径同样受下限约束：合法但过旧的 claude-cli UA 落库时必须抬到 CLICurrentVersion。
func TestCreateFingerprintFromHeadersFloorsOldClientUserAgent(t *testing.T) {
	cache := &stubIdentityCache{}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithUA("claude-cli/2.1.75 (external, claude-desktop-3p)"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/"+claude.CLICurrentVersion+" (external, claude-desktop-3p)", fp.UserAgent)
	require.Equal(t, 1, cache.setCalls)
}
