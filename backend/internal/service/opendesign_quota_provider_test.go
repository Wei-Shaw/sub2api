package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpendesignFundingCheckURL(t *testing.T) {
	t.Parallel()

	url, err := opendesignFundingCheckURL("https://amr-api.open-design.ai", "ws123")
	require.NoError(t, err)
	require.Equal(t, "https://amr-api.open-design.ai/api/v1/workspaces/ws123/billing/funding-check", url)

	_, err = opendesignFundingCheckURL("https://amr-api.open-design.ai", "bad id!")
	require.Error(t, err, "workspace id must be validated")

	_, err = opendesignFundingCheckURL("", "ws123")
	require.Error(t, err)
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
	tiers, plan, err := parseOpenDesignUsageTiers(body)
	require.NoError(t, err)
	require.Len(t, tiers, 2)
	require.Equal(t, "pro", plan)

	require.Equal(t, "5h", tiers[0].Window)
	require.InDelta(t, 10.0, tiers[0].UsedPercent, 1e-9)
	require.Equal(t, "2026-11-18T02:00:00Z", tiers[0].ResetAt)

	require.Equal(t, "weekly", tiers[1].Window)
	require.InDelta(t, 50.0, tiers[1].UsedPercent, 1e-9)
}

func TestParseOpenDesignUsageTiersNoPlan(t *testing.T) {
	t.Parallel()

	// Explicit null codingPlan: the account genuinely has no plan. Empty tiers
	// without error keep the account schedulable (fail-open).
	tiers, _, err := parseOpenDesignUsageTiers([]byte(`{"codingPlan": null}`))
	require.NoError(t, err)
	require.Empty(t, tiers)

	tiers, _, err = parseOpenDesignUsageTiers([]byte(`{"codingPlan": {"eligible": false, "windows": []}}`))
	require.NoError(t, err)
	require.Empty(t, tiers)
}

func TestParseOpenDesignUsageTiersSchemaChange(t *testing.T) {
	t.Parallel()

	// Missing codingPlan field = upstream schema change; must error so the
	// caller keeps the previous snapshot instead of writing an empty one.
	_, _, err := parseOpenDesignUsageTiers([]byte(`{"something": "else"}`))
	require.Error(t, err)

	// Eligible plan but no usable window = malformed; must error too.
	_, _, err = parseOpenDesignUsageTiers([]byte(`{"codingPlan": {"eligible": true, "windows": []}}`))
	require.Error(t, err)
}

func TestParseOpenDesignUsageTiersWindowMismatch(t *testing.T) {
	t.Parallel()

	// A window entry whose duration does not match 5h/7d is skipped; if both
	// entries are junk the call errors (eligible but nothing parsed).
	body := []byte(`{"codingPlan": {"eligible": true, "windows": [{"durationSeconds": 999}]}}`)
	_, _, err := parseOpenDesignUsageTiers(body)
	require.Error(t, err)
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
func TestOpenDesignSchedulingThresholdEvaluation(t *testing.T) {
	t.Parallel()

	extra := map[string]any{
		"open_design_5h_used_percent":     30.0,
		"open_design_weekly_used_percent": 96.0,
		"open_design_weekly_reset_at":     "2026-11-24T00:00:00Z",
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
