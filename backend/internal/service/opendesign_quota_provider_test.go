package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenDesignFundingCheckURL(t *testing.T) {
	t.Parallel()

	url, err := opendesignFundingCheckURL("https://amr-api.open-design.ai", "ws123")
	require.NoError(t, err)
	require.Equal(t, "https://amr-api.open-design.ai/api/v1/workspaces/ws123/billing/funding-check", url)

	_, err = opendesignFundingCheckURL("https://amr-api.open-design.ai", "bad id!")
	require.Error(t, err, "workspace id must be validated")

	_, err = opendesignFundingCheckURL("", "ws123")
	require.Error(t, err, "base url is required")
}

func TestParseOpenDesignUsageTiers(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"balanceUsd": "12.34",
		"codingPlan": {
			"eligible": true,
			"tier": "pro",
			"windows": [
				{"durationSeconds": 18000, "usedCredits": "10", "limitCredits": "100", "remainingCredits": "90", "resetsAt": "2026-11-18T02:00:00Z"},
				{"durationSeconds": 604800, "usedCredits": "500", "limitCredits": "1000", "remainingCredits": "500", "resetsAt": "2026-11-24T00:00:00Z"}
			]
		}
	}`)
	tiers, plan, noPlan, err := parseOpenDesignUsageTiers(body)
	require.NoError(t, err)
	require.False(t, noPlan)
	require.Len(t, tiers, 2)
	require.Equal(t, "pro", plan)

	require.Equal(t, "5h", tiers[0].Window)
	require.InDelta(t, 10.0, tiers[0].UsedPercent, 1e-9)
	require.Equal(t, "2026-11-18T02:00:00Z", tiers[0].ResetAt)

	require.Equal(t, "weekly", tiers[1].Window)
	require.InDelta(t, 50.0, tiers[1].UsedPercent, 1e-9)
}

// A response that explicitly reports no coding plan must be a success, not the
// shared "no recognized window" failure: the caller clears the windows so the
// account neither stays parked nor loses its schedulability.
func TestParseOpenDesignUsageTiersNoPlan(t *testing.T) {
	t.Parallel()

	tiers, planLevel, noPlan, err := parseOpenDesignUsageTiers([]byte(`{"balanceUsd":"3.5","codingPlan": null}`))
	require.NoError(t, err)
	require.True(t, noPlan)
	require.Empty(t, tiers)
	// 套餐名保持为空，不能把余额当成套餐名展示。
	require.Empty(t, planLevel)

	tiers, _, noPlan, err = parseOpenDesignUsageTiers([]byte(`{"codingPlan": {"eligible": false, "windows": []}}`))
	require.NoError(t, err)
	require.True(t, noPlan)
	require.Empty(t, tiers)
}

// Schema breaks must error so the caller keeps the previous snapshot instead of
// writing a bogus empty one.
func TestParseOpenDesignUsageTiersSchemaChange(t *testing.T) {
	t.Parallel()

	_, _, _, err := parseOpenDesignUsageTiers([]byte(`{"something": "else"}`))
	require.Error(t, err, "missing codingPlan field")

	_, _, _, err = parseOpenDesignUsageTiers([]byte(`{"codingPlan": {"windows": []}}`))
	require.Error(t, err, "missing eligible field must not be read as 'no plan'")

	_, _, _, err = parseOpenDesignUsageTiers([]byte(`{"codingPlan": {"eligible": true, "windows": []}}`))
	require.Error(t, err, "eligible plan without a usable window")

	_, _, _, err = parseOpenDesignUsageTiers([]byte(`{"codingPlan": {"eligible": true, "windows": [{"durationSeconds": 999}]}}`))
	require.Error(t, err, "only unknown-duration windows")
}

// A window whose duration matches but whose numbers or reset time are unusable
// must fail the whole parse: a partial write would leave "new usage + old reset".
func TestParseOpenDesignUsageTiersRejectsBrokenWindow(t *testing.T) {
	t.Parallel()

	good := `{"durationSeconds": 18000, "usedCredits": "10", "limitCredits": "100", "remainingCredits": "90", "resetsAt": "2026-11-18T02:00:00Z"}`

	for name, broken := range map[string]string{
		"missing reset":    `{"durationSeconds": 18000, "usedCredits": "10", "limitCredits": "100", "remainingCredits": "90"}`,
		"invalid reset":    `{"durationSeconds": 18000, "usedCredits": "10", "limitCredits": "100", "remainingCredits": "90", "resetsAt": "not-a-time"}`,
		"negative used":    `{"durationSeconds": 18000, "usedCredits": "-5", "limitCredits": "100", "remainingCredits": "90", "resetsAt": "2026-11-18T02:00:00Z"}`,
		"non-numeric used": `{"durationSeconds": 18000, "usedCredits": "ten", "limitCredits": "100", "remainingCredits": "90", "resetsAt": "2026-11-18T02:00:00Z"}`,
	} {
		body := []byte(`{"codingPlan": {"eligible": true, "windows": [` + good + `,` + broken + `]}}`)
		_, _, _, err := parseOpenDesignUsageTiers(body)
		require.Error(t, err, name)
	}
}

// Unknown window durations are ignored (forward compatible) as long as one known
// window still parses.
func TestParseOpenDesignUsageTiersIgnoresUnknownWindow(t *testing.T) {
	t.Parallel()

	body := []byte(`{"codingPlan": {"eligible": true, "windows": [
		{"durationSeconds": 18000, "usedCredits": "10", "limitCredits": "100", "remainingCredits": "90", "resetsAt": "2026-11-18T02:00:00Z"},
		{"durationSeconds": 2592000, "usedCredits": "1", "limitCredits": "2", "remainingCredits": "1", "resetsAt": "2026-12-18T02:00:00Z"}
	]}}`)
	tiers, _, noPlan, err := parseOpenDesignUsageTiers(body)
	require.NoError(t, err)
	require.False(t, noPlan)
	require.Len(t, tiers, 1)
	require.Equal(t, "5h", tiers[0].Window)
}

func TestGetCodingPlanProviderOpenDesign(t *testing.T) {
	t.Parallel()

	withKey := &Account{Platform: PlatformOpenDesign, Type: AccountTypeAPIKey, Credentials: map[string]any{OpenDesignUsageCredentialsKey: "ck"}}
	require.Equal(t, PlatformOpenDesign, withKey.GetCodingPlanProvider())

	withoutKey := &Account{Platform: PlatformOpenDesign, Type: AccountTypeAPIKey}
	require.Equal(t, "", withoutKey.GetCodingPlanProvider())

	oauth := &Account{Platform: PlatformOpenDesign, Type: AccountTypeOAuth, Credentials: map[string]any{OpenDesignUsageCredentialsKey: "ck"}}
	require.Equal(t, "", oauth.GetCodingPlanProvider(), "account type must stay apikey for probing")
}

func TestOpenDesignActivationURLValidation(t *testing.T) {
	t.Parallel()

	valid, err := validateOpenDesignActivationURL("https://open-design.ai/device?code=abc")
	require.NoError(t, err)
	require.Equal(t, "https://open-design.ai/device?code=abc", valid)

	_, err = validateOpenDesignActivationURL("https://amr.open-design.ai/activate")
	require.NoError(t, err, "vendor subdomains are allowed")

	for name, raw := range map[string]string{
		"empty":         "",
		"http":          "http://open-design.ai/device",
		"foreign host":  "https://evil.example.com/device",
		"suffix spoof":  "https://open-design.ai.evil.com/device",
		"with userinfo": "https://user@open-design.ai/device",
	} {
		_, err := validateOpenDesignActivationURL(raw)
		require.Error(t, err, name)
	}
}

func TestOpenDesignSchedulingThresholdEvaluation(t *testing.T) {
	t.Parallel()

	// Relative reset time keeps the fixture from expiring over time.
	resetAt := time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	extra := map[string]any{
		"open_design_5h_used_percent":     30.0,
		"open_design_weekly_used_percent": 96.0,
		"open_design_weekly_reset_at":     resetAt,
	}
	account := &Account{
		ID:          1,
		Platform:    PlatformOpenDesign,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Extra:       extra,
	}

	// open_design must be an allowed platform and evaluate to a pause at 95.
	require.Contains(t, AllowedSchedulingThresholdPlatforms, PlatformOpenDesign)
	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenDesign: 95}, time.Now())
	require.True(t, decision.ShouldPause, "weekly 96% must pause at threshold 95")
	require.Equal(t, "weekly", decision.Window)
	require.NotNil(t, decision.Until)

	// Below threshold: no pause.
	decision = EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenDesign: 99}, time.Now())
	require.False(t, decision.ShouldPause)

	// Threshold disabled (100): no pause.
	decision = EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenDesign: 100}, time.Now())
	require.False(t, decision.ShouldPause)
}

// fakeQuotaExtraRepo 只关心 UpdateExtra：用来断言探测分支真的落库了什么。
type fakeQuotaExtraRepo struct {
	AccountRepository
	updates map[string]any
	err     error
}

func (r *fakeQuotaExtraRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return r.err
}

// 走真实调用方方法（queryUsageForAccount 的 noPlan 分支），而不是只测 helper：
// 删掉分支、漏写 "No coding plan"、或没清窗口，这里都会红。
func TestCompleteOpenDesignNoPlanClearsWindowsAndPersists(t *testing.T) {
	t.Parallel()

	repo := &fakeQuotaExtraRepo{}
	svc := &CNProviderQuotaService{accountRepo: repo}
	account := &Account{ID: 7, Platform: PlatformOpenDesign, Type: AccountTypeAPIKey, Status: StatusActive}

	out := svc.completeOpenDesignNoPlan(context.Background(), account, &CNProviderQuotaProbeResult{}, time.Now())

	require.True(t, out.Success)
	require.True(t, out.Persisted)
	require.Nil(t, out.Tiers)
	require.Equal(t, "No coding plan", out.PlanLevel, "余额不得被当成套餐名")

	for _, suffix := range []string{
		cnExtraSuffix5hUsed, cnExtraSuffix5hReset,
		cnExtraSuffixWeeklyUsed, cnExtraSuffixWeeklyReset,
		cnExtraSuffixMonthlyUsed, cnExtraSuffixMonthlyReset,
	} {
		value, ok := repo.updates[cnExtraKey(PlatformOpenDesign, suffix)]
		require.True(t, ok, "%s must be written", suffix)
		require.Nil(t, value, "%s must be cleared", suffix)
	}
	require.NotNil(t, repo.updates[cnExtraKey(PlatformOpenDesign, cnExtraSuffixUsageUpdated)])

	// 已有套餐名（例如厂商返回的 tier）不应被覆盖成占位文案。
	kept := svc.completeOpenDesignNoPlan(context.Background(), account,
		&CNProviderQuotaProbeResult{PlanLevel: "pro"}, time.Now())
	require.Equal(t, "pro", kept.PlanLevel)
}

// 落库失败时探测本身仍算成功，但必须标成未持久化（方便上层观察/重试）。
func TestCompleteOpenDesignNoPlanReportsPersistFailure(t *testing.T) {
	t.Parallel()

	repo := &fakeQuotaExtraRepo{err: errors.New("db down")}
	svc := &CNProviderQuotaService{accountRepo: repo}

	out := svc.completeOpenDesignNoPlan(context.Background(),
		&Account{ID: 8, Platform: PlatformOpenDesign, Type: AccountTypeAPIKey},
		&CNProviderQuotaProbeResult{}, time.Now())

	require.True(t, out.Success)
	require.False(t, out.Persisted)
	require.Nil(t, out.Tiers)
}

// 激活链接只接受默认 HTTPS 端口（8443 之类的显式端口一律拒绝）。
func TestOpenDesignActivationURLRejectsNonDefaultPort(t *testing.T) {
	t.Parallel()

	_, err := validateOpenDesignActivationURL("https://open-design.ai:8443/device")
	require.Error(t, err)

	ok, err := validateOpenDesignActivationURL("https://open-design.ai:443/device")
	require.NoError(t, err)
	require.Equal(t, "https://open-design.ai:443/device", ok)
}
