//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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

// ─── groupUpdateNeedsStrictInvalidation tests ─────────────────────────────────

// TestGroupUpdateNeedsStrictInvalidation_StatusDisabled verifies that disabling
// an active group triggers strict cache invalidation.
func TestGroupUpdateNeedsStrictInvalidation_StatusDisabled(t *testing.T) {
	require.True(t,
		groupUpdateNeedsStrictInvalidation(
			StatusActive, StatusDisabled,
			false, false,
			SubscriptionTypeStandard, SubscriptionTypeStandard,
			1.0, 1.0,
			0, 0,
		),
		"disabling a group must trigger strict invalidation")
}

// TestGroupUpdateNeedsStrictInvalidation_StatusActiveNoChange verifies that
// keeping status=active does NOT trigger strict invalidation on its own.
func TestGroupUpdateNeedsStrictInvalidation_StatusActiveNoChange(t *testing.T) {
	require.False(t,
		groupUpdateNeedsStrictInvalidation(
			StatusActive, StatusActive,
			false, false,
			SubscriptionTypeStandard, SubscriptionTypeStandard,
			1.0, 1.0,
			0, 0,
		),
		"no effective change must not trigger strict invalidation")
}

// TestGroupUpdateNeedsStrictInvalidation_IsExclusiveChanged verifies that
// toggling is_exclusive triggers strict invalidation.
func TestGroupUpdateNeedsStrictInvalidation_IsExclusiveChanged(t *testing.T) {
	require.True(t,
		groupUpdateNeedsStrictInvalidation(
			StatusActive, StatusActive,
			false, true,
			SubscriptionTypeStandard, SubscriptionTypeStandard,
			1.0, 1.0,
			0, 0,
		),
		"changing is_exclusive must trigger strict invalidation")
}

// TestGroupUpdateNeedsStrictInvalidation_SubscriptionTypeChanged verifies that
// changing subscription_type triggers strict invalidation.
func TestGroupUpdateNeedsStrictInvalidation_SubscriptionTypeChanged(t *testing.T) {
	require.True(t,
		groupUpdateNeedsStrictInvalidation(
			StatusActive, StatusActive,
			false, false,
			SubscriptionTypeStandard, SubscriptionTypeSubscription,
			1.0, 1.0,
			0, 0,
		),
		"changing subscription_type must trigger strict invalidation")
}

// TestGroupUpdateNeedsStrictInvalidation_RateMultiplierChanged verifies that
// changing rate_multiplier triggers strict invalidation.
func TestGroupUpdateNeedsStrictInvalidation_RateMultiplierChanged(t *testing.T) {
	require.True(t,
		groupUpdateNeedsStrictInvalidation(
			StatusActive, StatusActive,
			false, false,
			SubscriptionTypeStandard, SubscriptionTypeStandard,
			1.0, 0.8,
			0, 0,
		),
		"changing rate_multiplier must trigger strict invalidation")
}

// TestGroupUpdateNeedsStrictInvalidation_RPMTightened verifies that reducing
// rpm_limit (from unlimited to limited) triggers strict invalidation.
func TestGroupUpdateNeedsStrictInvalidation_RPMTightened(t *testing.T) {
	require.True(t,
		groupUpdateNeedsStrictInvalidation(
			StatusActive, StatusActive,
			false, false,
			SubscriptionTypeStandard, SubscriptionTypeStandard,
			1.0, 1.0,
			0, 100, // unlimited → limited
		),
		"tightening RPM from unlimited to 100 must trigger strict invalidation")
}

// TestGroupUpdateNeedsStrictInvalidation_RPMRelaxed verifies that increasing
// rpm_limit does NOT trigger strict invalidation (relaxation, not tightening).
func TestGroupUpdateNeedsStrictInvalidation_RPMRelaxed(t *testing.T) {
	require.False(t,
		groupUpdateNeedsStrictInvalidation(
			StatusActive, StatusActive,
			false, false,
			SubscriptionTypeStandard, SubscriptionTypeStandard,
			1.0, 1.0,
			100, 200, // 100 → 200 (relaxation)
		),
		"relaxing RPM must not trigger strict invalidation")
}

// TestGroupUpdateNeedsStrictInvalidation_ReenablingGroupNoOtherChanges verifies
// that re-enabling a group (disabled → active) does not trigger strict
// invalidation (it's a permission grant, not a revocation).
func TestGroupUpdateNeedsStrictInvalidation_ReenablingGroupNoOtherChanges(t *testing.T) {
	require.False(t,
		groupUpdateNeedsStrictInvalidation(
			StatusDisabled, StatusActive,
			false, false,
			SubscriptionTypeStandard, SubscriptionTypeStandard,
			1.0, 1.0,
			0, 0,
		),
		"re-enabling group (no other change) must not trigger strict invalidation")
}
