package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClaudeWindowPresenceSurvivesActiveAndPassiveReads(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		visible   bool
	}{
		{"absent", `null`, false},
		{"real_zero", `{"utilization":0,"resets_at":null}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response ClaudeUsageResponse
			require.NoError(t, json.Unmarshal([]byte(`{"five_hour":`+tc.raw+`,"seven_day":{"utilization":5,"resets_at":"2030-01-01T00:00:00Z"}}`), &response))
			repo := &stubQuotaAccountRepo{}
			svc := &AccountUsageService{accountRepo: repo, cache: NewUsageCache(), usageLogRepo: &usageBatchLogRepoStub{}}
			now := time.Now()
			usage := svc.buildUsageInfo(&response, &now)
			require.Equal(t, tc.visible, usage.FiveHour != nil)
			require.NotNil(t, usage.SevenDay)
			svc.syncActiveToPassive(context.Background(), 91, usage)
			extra := repo.extraUpdates[91]
			require.Equal(t, tc.visible, extra["claude_five_hour_available"])
			passive, err := svc.getPassiveUsageForAccount(context.Background(), &Account{ID: 91, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: extra})
			require.NoError(t, err)
			require.Equal(t, tc.visible, passive.FiveHour != nil)
			require.NotNil(t, passive.SevenDay)
		})
	}
	// An omitted field is unknown, not evidence of an unlimited account.
	var response ClaudeUsageResponse
	require.NoError(t, json.Unmarshal([]byte(`{}`), &response))
	require.Nil(t, response.FiveHourAvailable)
}

func TestOpenAIPolicyHidesOnlyPersonalProFiveHourWindow(t *testing.T) {
	for _, plan := range []string{"prolite", "pro", "promax", "plus", "business", "unknown", ""} {
		t.Run(plan, func(t *testing.T) {
			account := &Account{ID: 92, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": plan}, Extra: map[string]any{
				"codex_5h_used_percent": 0.0, "codex_7d_used_percent": 34.0,
			}}
			svc := &AccountUsageService{usageLogRepo: &usageBatchLogRepoStub{}}
			usage, err := svc.getOpenAIUsage(context.Background(), account, false, true)
			require.NoError(t, err)
			require.Equal(t, !openAIPlanHasNoFiveHour(account), usage.FiveHour != nil)
			require.Equal(t, 34.0, usage.SevenDay.Utilization)
		})
	}
}

type reportedWindowReader struct{ openAIUsageReader }

func (reportedWindowReader) QueryUsage(context.Context, int64) (*OpenAIQuotaUsage, error) {
	return &OpenAIQuotaUsage{PlanType: "plus", RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 604800, ResetAfterSeconds: 3600, UsedPercent: 34}}}, nil
}

func TestOpenAIFreshAbsentWindowOverridesStaleSnapshot(t *testing.T) {
	account := &Account{ID: 93, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_5h_used_percent": 55.0}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{93: account}}
	svc := &AccountUsageService{accountRepo: repo, openAIQuotaService: reportedWindowReader{}, cache: NewUsageCache(), usageLogRepo: &usageBatchLogRepoStub{}}
	usage, err := svc.GetActiveUsage(context.Background(), 93, false)
	require.NoError(t, err)
	require.Nil(t, usage.FiveHour)
	require.Equal(t, 34.0, usage.SevenDay.Utilization)
	require.Equal(t, 55.0, account.Extra["codex_5h_used_percent"])
}
