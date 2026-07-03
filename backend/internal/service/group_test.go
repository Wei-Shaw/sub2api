//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type ttlCaptureGatewayCache struct {
	GatewayCache

	groupID     int64
	sessionHash string
	accountID   int64
	ttl         time.Duration
}

func (c *ttlCaptureGatewayCache) SetSessionAccountID(_ context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	c.groupID = groupID
	c.sessionHash = sessionHash
	c.accountID = accountID
	c.ttl = ttl
	return nil
}

// TestGroup_GetImagePrice_1K 测试 1K 尺寸返回正确价格
func TestGroup_GetImagePrice_1K(t *testing.T) {
	price := 0.10
	group := &Group{
		ImagePrice1K: &price,
	}

	result := group.GetImagePrice("1K")
	require.NotNil(t, result)
	require.InDelta(t, 0.10, *result, 0.0001)
}

// TestGroup_GetImagePrice_2K 测试 2K 尺寸返回正确价格
func TestGroup_GetImagePrice_2K(t *testing.T) {
	price := 0.15
	group := &Group{
		ImagePrice2K: &price,
	}

	result := group.GetImagePrice("2K")
	require.NotNil(t, result)
	require.InDelta(t, 0.15, *result, 0.0001)
}

// TestGroup_GetImagePrice_4K 测试 4K 尺寸返回正确价格
func TestGroup_GetImagePrice_4K(t *testing.T) {
	price := 0.30
	group := &Group{
		ImagePrice4K: &price,
	}

	result := group.GetImagePrice("4K")
	require.NotNil(t, result)
	require.InDelta(t, 0.30, *result, 0.0001)
}

// TestGroup_GetImagePrice_UnknownSize 测试未知尺寸回退 2K
func TestGroup_GetImagePrice_UnknownSize(t *testing.T) {
	price2K := 0.15
	group := &Group{
		ImagePrice2K: &price2K,
	}

	// 未知尺寸 "3K" 应该回退到 2K
	result := group.GetImagePrice("3K")
	require.NotNil(t, result)
	require.InDelta(t, 0.15, *result, 0.0001)

	// 空字符串也回退到 2K
	result = group.GetImagePrice("")
	require.NotNil(t, result)
	require.InDelta(t, 0.15, *result, 0.0001)
}

// TestGroup_GetImagePrice_NilValues 测试未配置时返回 nil
func TestGroup_GetImagePrice_NilValues(t *testing.T) {
	group := &Group{
		// 所有 ImagePrice 字段都是 nil
	}

	require.Nil(t, group.GetImagePrice("1K"))
	require.Nil(t, group.GetImagePrice("2K"))
	require.Nil(t, group.GetImagePrice("4K"))
	require.Nil(t, group.GetImagePrice("unknown"))
}

// TestGroup_GetImagePrice_PartialConfig 测试部分配置
func TestGroup_GetImagePrice_PartialConfig(t *testing.T) {
	price1K := 0.10
	group := &Group{
		ImagePrice1K: &price1K,
		// ImagePrice2K 和 ImagePrice4K 未配置
	}

	result := group.GetImagePrice("1K")
	require.NotNil(t, result)
	require.InDelta(t, 0.10, *result, 0.0001)

	// 2K 和 4K 返回 nil
	require.Nil(t, group.GetImagePrice("2K"))
	require.Nil(t, group.GetImagePrice("4K"))
}

func TestGroup_EffectiveKiroStickySessionTTL(t *testing.T) {
	cases := []struct {
		name    string
		group   *Group
		wantSec int
	}{
		{
			name:    "nil group uses gateway default duration",
			group:   nil,
			wantSec: int(stickySessionTTL.Seconds()),
		},
		{
			name:    "non kiro uses gateway default duration",
			group:   &Group{Platform: PlatformAnthropic, KiroStickySessionTTLSeconds: 120},
			wantSec: int(stickySessionTTL.Seconds()),
		},
		{
			name:    "kiro zero uses default",
			group:   &Group{Platform: PlatformKiro},
			wantSec: DefaultKiroStickySessionTTLSeconds,
		},
		{
			name:    "kiro clamps low value",
			group:   &Group{Platform: PlatformKiro, KiroStickySessionTTLSeconds: 1},
			wantSec: MinKiroStickySessionTTLSeconds,
		},
		{
			name:    "kiro preserves configured value",
			group:   &Group{Platform: PlatformKiro, KiroStickySessionTTLSeconds: 7200},
			wantSec: 7200,
		},
		{
			name:    "kiro clamps high value",
			group:   &Group{Platform: PlatformKiro, KiroStickySessionTTLSeconds: 999999},
			wantSec: MaxKiroStickySessionTTLSeconds,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, time.Duration(tc.wantSec)*time.Second, stickySessionTTLForGroup(tc.group))
		})
	}
}

func TestGatewayService_BindStickySessionForGroupUsesConfiguredTTL(t *testing.T) {
	cache := &ttlCaptureGatewayCache{}
	service := &GatewayService{cache: cache}
	groupID := int64(7)

	err := service.BindStickySessionForGroup(context.Background(), &groupID, "session-hash", 42, &Group{
		ID:                          groupID,
		Platform:                    PlatformKiro,
		KiroStickySessionTTLSeconds: 7200,
	})

	require.NoError(t, err)
	require.Equal(t, groupID, cache.groupID)
	require.Equal(t, "session-hash", cache.sessionHash)
	require.Equal(t, int64(42), cache.accountID)
	require.Equal(t, 2*time.Hour, cache.ttl)
}

func TestNormalizeGroupRuntimeFields_KiroStickySessionTTL(t *testing.T) {
	kiro := &Group{Platform: PlatformKiro, KiroStickySessionTTLSeconds: 10}
	NormalizeGroupRuntimeFields(kiro)
	require.Equal(t, MinKiroStickySessionTTLSeconds, kiro.KiroStickySessionTTLSeconds)

	nonKiro := &Group{
		Platform:                    PlatformAnthropic,
		KiroAutoStickyEnabled:       true,
		KiroStickySessionTTLSeconds: 7200,
		KiroCacheEmulationEnabled:   true,
		KiroCacheEmulationRatio:     0.5,
	}
	NormalizeGroupRuntimeFields(nonKiro)
	require.False(t, nonKiro.KiroAutoStickyEnabled)
	require.Zero(t, nonKiro.KiroStickySessionTTLSeconds)
	require.False(t, nonKiro.KiroCacheEmulationEnabled)
	require.Zero(t, nonKiro.KiroCacheEmulationRatio)
}
