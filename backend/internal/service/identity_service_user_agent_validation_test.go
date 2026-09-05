package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

type stubIdentityCache struct {
	fingerprint *Fingerprint
	setCalls    int
	lastSet     *Fingerprint
}

func (s *stubIdentityCache) GetFingerprint(_ context.Context, _ int64) (*Fingerprint, error) {
	if s.fingerprint == nil {
		return nil, nil
	}
	clone := *s.fingerprint
	return &clone, nil
}

func (s *stubIdentityCache) SetFingerprint(_ context.Context, _ int64, fp *Fingerprint) error {
	s.setCalls++
	clone := *fp
	s.lastSet = &clone
	s.fingerprint = &clone
	return nil
}

func (s *stubIdentityCache) GetMaskedSessionID(_ context.Context, _ int64) (string, error) {
	return "", nil
}

func (s *stubIdentityCache) SetMaskedSessionID(_ context.Context, _ int64, _ string) error {
	return nil
}

func headersWithUA(ua string) http.Header {
	h := http.Header{}
	if ua != "" {
		h.Set("User-Agent", ua)
	}
	return h
}

func TestIsAcceptableFingerprintUserAgent(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want bool
	}{
		{"official_cli", "claude-cli/2.1.220 (external, cli)", true},
		{"official_cli_no_meta", "claude-cli/2.1.220", true},
		{"next_major_still_allowed", "claude-cli/4.0.0 (external, cli)", true},
		{"other_product_valid_form", "some-sdk/1.2.3 (node)", true},

		// 本地/开发构建：版本号后带后缀，正是 #5254 的毒化 UA 形态。
		{"local_build_suffix", "claude-cli/999.0.0-local (undefined, cli)", false},
		{"dev_build_suffix", "claude-cli/2.1.220-dev (external, cli)", false},
		{"build_metadata_suffix", "claude-cli/2.1.220+build1 (external, cli)", false},

		// 哨兵版本号：形态合法但主版本号远超 sub2api 自身伪装版本。
		{"sentinel_major", "claude-cli/999.0.0 (external, cli)", false},

		{"empty", "", false},
		{"no_version", "claude-cli (external, cli)", false},
		{"two_segment_version", "claude-cli/2.1 (external, cli)", false},
		// 浏览器 UA 是两段版本号，不符合三段 semver 形态：不适合作为账号身份。
		{"browser_ua_two_segment", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", false},
		{"leading_junk", "x claude-cli/2.1.220 (external, cli)", false},
		{"too_long", "claude-cli/2.1.220 (" + strings.Repeat("a", 300) + ")", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isAcceptableFingerprintUserAgent(tc.ua))
		})
	}
}

// 首次创建路径：畸形 UA 不得被原样持久化，回退默认指纹。
// 只在 isNewerVersion 处加校验是不够的——删键恢复后账号会被同一客户端立即再次毒化。
func TestGetOrCreateFingerprintRejectsMalformedUserAgentOnCreate(t *testing.T) {
	cache := &stubIdentityCache{}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithUA("claude-cli/999.0.0-local (undefined, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, defaultFingerprint.UserAgent, fp.UserAgent,
		"畸形 UA 必须回退默认指纹，而不是被写成账号级持久身份")
	require.NotContains(t, cache.lastSet.UserAgent, "999.0.0")
}

// 升级路径：哨兵版本号不得覆盖已缓存的真实指纹。
// isNewerVersion 是纯数值比较，999.0.0 恒大于任何真实版本，一旦写入永远无法夺回。
func TestGetOrCreateFingerprintRejectsSentinelVersionOnUpgrade(t *testing.T) {
	cached := &Fingerprint{
		UserAgent: "claude-cli/2.1.22 (external, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}
	cache := &stubIdentityCache{fingerprint: cached}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithUA("claude-cli/999.0.0-local (undefined, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/2.1.22 (external, cli)", fp.UserAgent,
		"真实指纹不得被哨兵版本覆盖")
	require.Zero(t, cache.setCalls, "被拒的 UA 不应触发任何写入")
}

// 合法的真实版本升级必须照常生效，校验不能把正常升级一起挡掉。
func TestGetOrCreateFingerprintStillUpgradesOnValidNewerVersion(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/2.1.22 (external, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	newUA := "claude-cli/2.1.223 (external, cli)"
	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, headersWithUA(newUA))

	require.NoError(t, err)
	require.Equal(t, newUA, fp.UserAgent)
	require.Equal(t, 1, cache.setCalls)
}

// 合法 UA 的首次创建路径不受影响。
func TestGetOrCreateFingerprintAcceptsValidUserAgentOnCreate(t *testing.T) {
	cache := &stubIdentityCache{}
	svc := NewIdentityService(cache)

	ua := "claude-cli/" + claude.CLICurrentVersion + " (external, cli)"
	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, headersWithUA(ua))

	require.NoError(t, err)
	require.Equal(t, ua, fp.UserAgent)
	require.NotEmpty(t, fp.ClientID)
	require.Equal(t, 1, cache.setCalls)
}

// 不变式：默认指纹自身必须能通过校验，否则自愈路径会在每次读取时反复重写。
func TestDefaultFingerprintUserAgentIsAcceptable(t *testing.T) {
	require.True(t, isAcceptableFingerprintUserAgent(defaultFingerprint.UserAgent),
		"defaultFingerprint.UserAgent 必须自洽，否则自愈会陷入反复重写")
}

// 存量自愈：本次加固之前写入的畸形指纹在读取时被纠正。
// 指纹在活跃账号上懒续期后近乎永不过期，且系统内没有重置入口——
// 不在读取时纠正，已中招的账号只能靠手工删 Redis 键恢复。
func TestGetOrCreateFingerprintHealsPoisonedCacheUsingValidClientUA(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/999.0.0-local (undefined, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	realUA := "claude-cli/2.1.22 (external, cli)"
	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, headersWithUA(realUA))

	require.NoError(t, err)
	require.Equal(t, realUA, fp.UserAgent,
		"真实客户端必须能从被毒化的指纹手中夺回账号身份")
	require.Equal(t, 1, cache.setCalls)
	require.NotContains(t, cache.lastSet.UserAgent, "999.0.0")
	require.Equal(t, "cid-1", fp.ClientID, "自愈不应重置 ClientID")
}

// 毒化指纹 + 同样畸形的客户端 UA：回退默认指纹，不保留任何一方的畸形值。
func TestGetOrCreateFingerprintHealsPoisonedCacheWithoutValidClientUA(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/999.0.0-local (undefined, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithUA("claude-cli/999.0.0-local (undefined, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, defaultFingerprint.UserAgent, fp.UserAgent)
	require.Equal(t, 1, cache.setCalls)
}

// 自愈只针对畸形缓存：合法缓存 + 非更新版本的合法 UA 不得触发额外写入。
func TestGetOrCreateFingerprintDoesNotRewriteHealthyCache(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/2.1.220 (external, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithUA("claude-cli/2.1.22 (external, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/2.1.220 (external, cli)", fp.UserAgent)
	require.Zero(t, cache.setCalls)
}

// 无 UA 时的既有行为（回退默认指纹）保持不变。
func TestGetOrCreateFingerprintMissingUserAgentKeepsDefault(t *testing.T) {
	cache := &stubIdentityCache{}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, http.Header{})

	require.NoError(t, err)
	require.Equal(t, defaultFingerprint.UserAgent, fp.UserAgent)
}
