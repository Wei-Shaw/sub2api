//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveShareRevenueMode(t *testing.T) {
	owner := int64(10)
	caller := int64(10)
	other := int64(20)

	priv := &Group{Name: PrivateGroupName(caller, PlatformOpenAI), IsSharePool: false}
	share := &Group{Name: "pool", IsSharePool: true}
	ops := &Group{Name: "ops", IsSharePool: false}

	accOwn := &Account{OwnerUserID: &owner}
	accOther := &Account{OwnerUserID: &other}
	accSys := &Account{OwnerUserID: nil}

	require.Equal(t, RevenueModeLegacy, ResolveShareRevenueMode(false, share, accOther, caller))
	require.Equal(t, RevenueModeSelfPrivateEnv, ResolveShareRevenueMode(true, priv, accOwn, caller))
	require.Equal(t, RevenueModeShareSplit, ResolveShareRevenueMode(true, share, accOther, caller))
	require.Equal(t, RevenueModeLegacy, ResolveShareRevenueMode(true, share, accOwn, caller), "own public via share pool → legacy")
	require.Equal(t, RevenueModeLegacy, ResolveShareRevenueMode(true, share, accSys, caller))
	require.Equal(t, RevenueModeLegacy, ResolveShareRevenueMode(true, ops, accOther, caller))
}

func TestComputeShareRevenuePlan_ShareSplit(t *testing.T) {
	owner := int64(2)
	inviter := int64(3)
	cfg := ShareRevenueSettings{
		Enabled: true, InvitePct: 10, UserPct: 40, PlatformPct: 50, AffiliateEnabled: true,
	}
	plan := ComputeShareRevenuePlan(RevenueModeShareSplit, 100, cfg, &owner, &inviter)
	require.Equal(t, RevenueModeShareSplit, plan.Mode)
	require.InDelta(t, 100, plan.BilledAmount, 1e-9)
	require.InDelta(t, 10, plan.InviteAmount, 1e-9)
	require.InDelta(t, 40, plan.UserAmount, 1e-9)
	require.InDelta(t, 50, plan.PlatformAmount, 1e-9)

	// no inviter → invite 并入平台
	plan2 := ComputeShareRevenuePlan(RevenueModeShareSplit, 100, cfg, &owner, nil)
	require.InDelta(t, 0, plan2.InviteAmount, 1e-9)
	require.InDelta(t, 40, plan2.UserAmount, 1e-9)
	require.InDelta(t, 60, plan2.PlatformAmount, 1e-9)
}

func TestComputeShareRevenuePlan_SelfPrivateEnv(t *testing.T) {
	owner := int64(1)
	cfg := ShareRevenueSettings{PrivateSelfEnvPct: 1}
	plan := ComputeShareRevenuePlan(RevenueModeSelfPrivateEnv, 100, cfg, &owner, nil)
	require.InDelta(t, 1, plan.BilledAmount, 1e-9)
	require.InDelta(t, 1, plan.PlatformAmount, 1e-9)
	require.Zero(t, plan.UserAmount)
}

func TestNormalizeShareSplitPercents(t *testing.T) {
	i, u, p := normalizeShareSplitPercents(10, 40, 50)
	require.InDelta(t, 10, i, 1e-9)
	require.InDelta(t, 40, u, 1e-9)
	require.InDelta(t, 50, p, 1e-9)
}

// TestOpenAIGatewayBillingDeps_ShareRevenue 回归：Grok/OpenAI 计费必须带上分账依赖，
// 否则 prepareShareRevenuePlan 因 settingService==nil 永远返回 nil，贡献者永不入账。
func TestOpenAIGatewayBillingDeps_ShareRevenue(t *testing.T) {
	svc := &OpenAIGatewayService{}
	deps := svc.billingDeps()
	require.NotNil(t, deps)
	require.Nil(t, deps.settingService, "unset service should leave setting nil")
	require.Nil(t, deps.shareRevenueLedger)

	fakeLedger := &stubShareRevenueLedger{}
	svc.SetShareRevenueDeps(nil, fakeLedger)
	// settingService still nil until assigned — simulate production fields
	svc.settingService = &SettingService{}
	deps2 := svc.billingDeps()
	require.Same(t, svc.settingService, deps2.settingService)
	require.Same(t, fakeLedger, deps2.shareRevenueLedger)
}

type stubShareRevenueLedger struct{}

func (s *stubShareRevenueLedger) InsertShareRevenueLedger(ctx context.Context, row *ShareRevenueLedgerRow) error {
	return nil
}
