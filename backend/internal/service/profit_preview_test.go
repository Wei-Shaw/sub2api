package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPreviewProfitAdmissionUsesAccountRatesAndPreinitializesModels(t *testing.T) {
	now := time.Now()
	group := profitControlTestGroup(50, 0.2, 0)
	group.Name = "VIP-preview"

	cheap := profitControlTestAccountWithRate(
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 1.0, now.Add(-3*time.Hour), 30*time.Minute),
		0.5,
	)
	cheap.Name = "cheap"
	cheap.Extra[UpstreamBillingRateSyncEnabledExtraKey] = true
	cheap.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-sol": "gpt-sol"}}

	boundary := profitControlTestAccountWithRate(
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute),
		0.8,
	)
	boundary.Name = "boundary"
	boundary.Credentials = map[string]any{"model_mapping": map[string]any{
		"gpt-sol":  "gpt-sol",
		"gpt-luna": "gpt-luna",
	}}

	expensive := profitControlTestAccountWithRate(
		upstreamCostTestAccount(3, UpstreamBillingProbeStatusOK, 0.2, now.Add(-time.Minute), 30*time.Minute),
		1.0,
	)
	expensive.Name = "expensive"

	invalid := upstreamCostTestOAuthAccount(4)
	invalid.Name = "invalid"

	reports := PreviewProfitAdmission([]ProfitPreviewGroupInput{{
		Group:         group,
		Accounts:      []*Account{cheap, boundary, expensive, invalid},
		UserOverrides: map[int64]float64{40: 0.5},
		Models:        []string{"gpt-sol", "gpt-luna", "gpt-no-account"},
	}}, now)
	require.Len(t, reports, 1)
	report := reports[0]

	require.True(t, report.EffectiveGate)
	require.InDelta(t, 1.0, report.DefaultD, 1e-12)
	require.InDelta(t, 0.8, report.ThresholdDefault, 1e-12)
	require.InDelta(t, 0.5, report.MinEffectiveD, 1e-12)
	require.InDelta(t, 0.4, report.ThresholdMinD, 1e-12)

	byID := map[int64]ProfitPreviewAccountVerdict{}
	for _, verdict := range report.Verdicts {
		byID[verdict.AccountID] = verdict
	}
	require.Equal(t, ProfitPreviewClassAdmitted, byID[cheap.ID].Class)
	require.Equal(t, ProfitPreviewRateSourceUpstreamProbe, byID[cheap.ID].RateSource)
	require.Contains(t, byID[cheap.ID].Warnings, ProfitPreviewWarningProbeStale)
	require.True(t, byID[cheap.ID].RejectedUnderMinD)

	require.Equal(t, ProfitPreviewClassAdmitted, byID[boundary.ID].Class, "U == 阈值按 epsilon 语义准入")
	require.Equal(t, ProfitPreviewClassRejectedThreshold, byID[expensive.ID].Class)
	require.Contains(t, byID[expensive.ID].Warnings, ProfitPreviewWarningManualRateOne)
	require.Equal(t, ProfitPreviewClassRejectedInvalidRate, byID[invalid.ID].Class)

	require.Equal(t, 2, report.RemainingByModel["gpt-sol"])
	require.Equal(t, 1, report.RemainingByModel["gpt-luna"])
	require.Equal(t, 0, report.RemainingByModel["gpt-no-account"])
	_, present := report.RemainingByModel["gpt-no-account"]
	require.True(t, present, "全部账号均不支持时也必须显式保留 0，供 CLI 发出警告")
}

func TestPreviewProfitAdmissionAssumeEnabled(t *testing.T) {
	now := time.Now()
	group := profitControlTestGroup(51, 0, 0)
	group.Platform = PlatformOpenAI
	group.RateMultiplier = 0.5
	group.ProfitControlEnabled = false

	cheapAccount := profitControlTestAccountWithRate(
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.9, now.Add(-time.Minute), 30*time.Minute),
		0.2,
	)
	expensiveAccount := profitControlTestAccountWithRate(
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.2, now.Add(-time.Minute), 30*time.Minute),
		0.9,
	)

	withoutAssume := PreviewProfitAdmission([]ProfitPreviewGroupInput{{
		Group:    group,
		Accounts: []*Account{cheapAccount, expensiveAccount},
		Models:   []string{"gpt-test"},
	}}, now)[0]
	require.False(t, withoutAssume.EffectiveGate)
	require.Equal(t, ProfitPreviewClassAdmitted, withoutAssume.Verdicts[0].Class)
	require.Equal(t, ProfitPreviewClassAdmitted, withoutAssume.Verdicts[1].Class)

	withAssume := PreviewProfitAdmission([]ProfitPreviewGroupInput{{
		Group:         group,
		Accounts:      []*Account{cheapAccount, expensiveAccount},
		Models:        []string{"gpt-test"},
		AssumeEnabled: true,
	}}, now)[0]
	require.True(t, withAssume.EffectiveGate)
	require.True(t, withAssume.AssumedEnabled)

	byID := map[int64]ProfitPreviewAccountVerdict{}
	for _, verdict := range withAssume.Verdicts {
		byID[verdict.AccountID] = verdict
	}
	require.Equal(t, ProfitPreviewClassAdmitted, byID[cheapAccount.ID].Class,
		"账号倍率 0.2 <= 阈值 0.5，探测快照的高倍率不参与准入判断")
	require.Equal(t, ProfitPreviewClassRejectedThreshold, byID[expensiveAccount.ID].Class,
		"账号倍率 0.9 > 阈值 0.5，探测快照的低倍率不能替代账号倍率")
}

func TestPreviewProfitAdmissionSupportsFivePlatforms(t *testing.T) {
	for i, platform := range []string{
		PlatformOpenAI,
		PlatformAnthropic,
		PlatformGemini,
		PlatformGrok,
		PlatformAntigravity,
	} {
		group := profitControlTestGroup(int64(100+i), 0, 0)
		group.Platform = platform
		rate := 0.2
		account := &Account{
			ID:             int64(200 + i),
			Platform:       platform,
			RateMultiplier: &rate,
		}
		report := PreviewProfitAdmission([]ProfitPreviewGroupInput{{
			Group:    group,
			Accounts: []*Account{account},
			Models:   []string{"model"},
		}}, time.Now())[0]
		require.True(t, report.EffectiveGate, platform)
		require.Equal(t, ProfitPreviewClassAdmitted, report.Verdicts[0].Class, platform)
	}
}
