package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testIdentityProfileSecret = "00000000000000000000000000000000-test-secret"

func TestIdentityProfileService_StableAcrossCalls(t *testing.T) {
	svc := NewIdentityProfileService(testIdentityProfileSecret, 14)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	a := svc.Profile(42, PlatformAnthropic, now)
	b := svc.Profile(42, PlatformAnthropic, now.Add(6*time.Hour))

	require.Equal(t, a.MachineID, b.MachineID)
	require.Equal(t, a.OS, b.OS)
	require.Equal(t, a.Arch, b.Arch)
	require.Equal(t, a.Locale, b.Locale)
	require.Equal(t, a.Timezone, b.Timezone)
	require.Equal(t, a.RotationSalt, b.RotationSalt)
	require.NotEmpty(t, a.MachineID)
	require.Len(t, a.MachineID, 32, "machine_id should be 32 hex chars")
}

func TestIdentityProfileService_DifferentUsersGetDifferentProfiles(t *testing.T) {
	svc := NewIdentityProfileService(testIdentityProfileSecret, 14)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	uniqueIDs := make(map[string]struct{})
	for userID := int64(1); userID <= 200; userID++ {
		p := svc.Profile(userID, PlatformAnthropic, now)
		uniqueIDs[p.MachineID] = struct{}{}
	}

	require.GreaterOrEqual(t, len(uniqueIDs), 195, "machine_id collisions should be extremely rare across 200 users")
}

func TestIdentityProfileService_DifferentPlatformsGetDifferentMachineIDs(t *testing.T) {
	svc := NewIdentityProfileService(testIdentityProfileSecret, 14)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	anthropic := svc.Profile(42, PlatformAnthropic, now)
	openai := svc.Profile(42, PlatformOpenAI, now)
	gemini := svc.Profile(42, PlatformGemini, now)

	require.NotEqual(t, anthropic.MachineID, openai.MachineID)
	require.NotEqual(t, anthropic.MachineID, gemini.MachineID)
	require.NotEqual(t, openai.MachineID, gemini.MachineID)
}

func TestIdentityProfileService_RotatesAfterTTL(t *testing.T) {
	svc := NewIdentityProfileService(testIdentityProfileSecret, 14)
	t0 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(20 * 24 * time.Hour) // > 14 day window

	a := svc.Profile(42, PlatformAnthropic, t0)
	b := svc.Profile(42, PlatformAnthropic, t1)

	require.NotEqual(t, a.RotationSalt, b.RotationSalt, "rotation salt should change after TTL")
	require.NotEqual(t, a.MachineID, b.MachineID, "machine_id should rotate when salt changes")
}

func TestIdentityProfileService_CandidateValuesAreFromExpectedPools(t *testing.T) {
	svc := NewIdentityProfileService(testIdentityProfileSecret, 14)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	for userID := int64(1); userID <= 50; userID++ {
		p := svc.Profile(userID, PlatformAnthropic, now)
		require.Contains(t, identityProfileOSPool, p.OS)
		require.Contains(t, identityProfileArchPool, p.Arch)
		require.Contains(t, identityProfileLocalePool, p.Locale)
		require.Contains(t, identityProfileTimezonePool, p.Timezone)
		require.NotEmpty(t, p.RotationSalt)
		require.True(t, strings.HasPrefix(p.UserAgentVersion, "2."), "cli version should be a 2.x release, got %q", p.UserAgentVersion)
	}
}

func TestIdentityProfileService_SecretChangeRotatesProfile(t *testing.T) {
	svcA := NewIdentityProfileService(testIdentityProfileSecret, 14)
	svcB := NewIdentityProfileService(testIdentityProfileSecret+"-rotated", 14)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	a := svcA.Profile(42, PlatformAnthropic, now)
	b := svcB.Profile(42, PlatformAnthropic, now)
	require.NotEqual(t, a.MachineID, b.MachineID, "different secrets must produce different fingerprints")
}

func TestIdentityProfileService_EmptyInputsAreSafe(t *testing.T) {
	svc := NewIdentityProfileService("", 0)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	a := svc.Profile(0, "", now)
	require.NotEmpty(t, a.MachineID)
	require.Equal(t, "unknown", a.Platform)

	b := svc.Profile(0, "", now)
	require.Equal(t, a.MachineID, b.MachineID, "deterministic even with zero/empty inputs")
}

// TestIdentityProfileService_DeviceIDHex64 验证 P0-3 §4.4 task 2 引入的 64-hex
// 字段：长度、与 MachineID 互不相同、跨用户低碰撞、跨 rotation 翻新。
// 该字段用于 Anthropic metadata.user_id 中的 device_id 段（必须 64 hex）。
func TestIdentityProfileService_DeviceIDHex64(t *testing.T) {
	svc := NewIdentityProfileService(testIdentityProfileSecret, 14)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	t.Run("长度严格 64 hex", func(t *testing.T) {
		p := svc.Profile(42, PlatformAnthropic, now)
		require.Len(t, p.DeviceIDHex64, 64, "device_id_hex64 must be 64 hex chars")
		// 应该都是 hex
		for _, ch := range p.DeviceIDHex64 {
			require.True(t,
				(ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f'),
				"device_id_hex64 contains non-hex char %q", ch,
			)
		}
	})

	t.Run("与 MachineID 不同 scope（不同派生）", func(t *testing.T) {
		p := svc.Profile(42, PlatformAnthropic, now)
		// MachineID 32 字符；DeviceIDHex64 64 字符；前 32 字符也不应相等（不同 scope）
		require.NotEqual(t, p.MachineID, p.DeviceIDHex64[:32],
			"device_id_hex64 应使用独立 scope 派生，不能是 MachineID 的简单延展")
	})

	t.Run("同 user × 同窗口幂等", func(t *testing.T) {
		a := svc.Profile(42, PlatformAnthropic, now)
		b := svc.Profile(42, PlatformAnthropic, now.Add(2*time.Hour))
		require.Equal(t, a.DeviceIDHex64, b.DeviceIDHex64)
	})

	t.Run("不同 user 低碰撞（200 样本）", func(t *testing.T) {
		unique := make(map[string]struct{})
		for uid := int64(1); uid <= 200; uid++ {
			p := svc.Profile(uid, PlatformAnthropic, now)
			unique[p.DeviceIDHex64] = struct{}{}
		}
		require.GreaterOrEqual(t, len(unique), 195, "device_id_hex64 collision should be extremely rare")
	})

	t.Run("跨平台不同", func(t *testing.T) {
		anthropic := svc.Profile(42, PlatformAnthropic, now)
		openai := svc.Profile(42, PlatformOpenAI, now)
		require.NotEqual(t, anthropic.DeviceIDHex64, openai.DeviceIDHex64)
	})

	t.Run("跨旋转窗口翻新", func(t *testing.T) {
		t0 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		t1 := t0.Add(20 * 24 * time.Hour)
		a := svc.Profile(42, PlatformAnthropic, t0)
		b := svc.Profile(42, PlatformAnthropic, t1)
		require.NotEqual(t, a.DeviceIDHex64, b.DeviceIDHex64)
	})
}
