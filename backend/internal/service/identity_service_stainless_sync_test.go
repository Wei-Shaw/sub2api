package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

func headersWithStainless(ua, packageVersion, runtimeVersion string) http.Header {
	h := headersWithUA(ua)
	if packageVersion != "" {
		h.Set("X-Stainless-Package-Version", packageVersion)
	}
	if runtimeVersion != "" {
		h.Set("X-Stainless-Runtime-Version", runtimeVersion)
	}
	h.Set("X-Stainless-Lang", "js")
	h.Set("X-Stainless-OS", "Darwin")
	h.Set("X-Stainless-Arch", "x64")
	h.Set("X-Stainless-Runtime", "node")
	return h
}

// 首次创建：UA 被 floor 抬升时，客户端带来的旧 Package/Runtime 必须被
// 抬升后版本的同源组合覆盖；OS/Arch 等设备字段保持客户端原值。
func TestCreateFingerprintFromHeadersSyncsStainlessAfterFloor(t *testing.T) {
	cache := &stubIdentityCache{}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithStainless("claude-cli/2.1.200 (external, cli)", "0.80.0", "v24.3.0"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/"+claude.EffectiveCLIVersion()+" (external, cli)", fp.UserAgent)
	want, _ := claude.StainlessVersionsForCLIVersion(claude.EffectiveCLIVersion())
	require.Equal(t, want.PackageVersion, fp.StainlessPackageVersion)
	require.Equal(t, want.RuntimeVersion, fp.StainlessRuntimeVersion)
	require.Equal(t, "Darwin", fp.StainlessOS)
	require.Equal(t, "x64", fp.StainlessArch)
	require.Equal(t, "node", fp.StainlessRuntime)
	require.Equal(t, 1, cache.setCalls)
}

// 首次创建：UA 未被 floor 时，三个值本来就来自同一个真实客户端，原样保留。
func TestCreateFingerprintFromHeadersKeepsStainlessWhenNotFloored(t *testing.T) {
	svc := NewIdentityService(&stubIdentityCache{})

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithStainless("claude-cli/2.9.0 (external, cli)", "0.130.0", "v27.0.0"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/2.9.0 (external, cli)", fp.UserAgent)
	require.Equal(t, "0.130.0", fp.StainlessPackageVersion)
	require.Equal(t, "v27.0.0", fp.StainlessRuntimeVersion)
}

// 客户端版本高于缓存但低于下限：merge 先更新，floor 后仍须把 Package/Runtime
// 拉回抬升后版本的同源组合。
func TestGetOrCreateFingerprintFloorWinsOverStaleClientUpgradeSyncsStainless(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent:               "claude-cli/2.1.22 (external, cli)",
		ClientID:                "cid-1",
		StainlessPackageVersion: "0.80.0",
		StainlessRuntimeVersion: "v24.3.0",
		UpdatedAt:               time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(), 1,
		headersWithStainless("claude-cli/2.1.223 (external, cli)", "0.90.0", "v24.3.0"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/"+claude.EffectiveCLIVersion()+" (external, cli)", fp.UserAgent)
	want, _ := claude.StainlessVersionsForCLIVersion(claude.EffectiveCLIVersion())
	require.Equal(t, want.PackageVersion, fp.StainlessPackageVersion)
	require.Equal(t, want.RuntimeVersion, fp.StainlessRuntimeVersion)
	require.Equal(t, want.PackageVersion, cache.lastSet.StainlessPackageVersion)
	require.Equal(t, want.RuntimeVersion, cache.lastSet.StainlessRuntimeVersion)
}

// floor 目标版本进入最新区间（2.1.311+）：Runtime 也要跟到 v26.12.1。
func TestGetOrCreateFingerprintFloorSyncFollowsRuntimeVersion(t *testing.T) {
	const upgraded = "2.1.330"
	withCLIVersionResolverForTest(t, func() string { return upgraded })
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent:               "claude-cli/2.1.220 (external, cli)",
		ClientID:                "cid-1",
		StainlessPackageVersion: "0.94.0",
		StainlessRuntimeVersion: "v26.3.0",
		UpdatedAt:               time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, headersWithUA("claude-cli/2.1.75 (external, cli)"))

	require.NoError(t, err)
	require.Equal(t, "claude-cli/"+upgraded+" (external, cli)", fp.UserAgent)
	require.Equal(t, "0.128.0", fp.StainlessPackageVersion)
	require.Equal(t, "v26.12.1", fp.StainlessRuntimeVersion)
}

// 无客户端头时默认指纹同样按运行期版本取同源组合，不能再退回旧构建常量。
func TestDefaultFingerprintStainlessVersionsFollowRuntimeVersion(t *testing.T) {
	const upgraded = "2.1.330"
	withCLIVersionResolverForTest(t, func() string { return upgraded })

	fp := defaultFingerprint()
	require.Equal(t, "claude-cli/"+upgraded+" (external, cli)", fp.UserAgent)
	require.Equal(t, "0.128.0", fp.StainlessPackageVersion)
	require.Equal(t, "v26.12.1", fp.StainlessRuntimeVersion)
	require.Equal(t, "Linux", fp.StainlessOS)
	require.Equal(t, "arm64", fp.StainlessArch)
}
