package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIReasoningEffortForRouting(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		model string
		want  string
	}{
		{name: "responses nested effort", body: `{"reasoning":{"effort":" high "}}`, model: "gpt-5.4", want: "high"},
		{name: "chat completions flat effort", body: `{"reasoning_effort":"x-high"}`, model: "gpt-5.4", want: "xhigh"},
		{name: "model suffix without field stays unchanged", body: `{}`, model: "gpt-5.4-low", want: ""},
		{name: "omitted effort", body: `{}`, model: "gpt-5.4", want: ""},
		{name: "unknown explicit effort does not infer model", body: `{"reasoning":{"effort":"turbo"}}`, model: "gpt-5.4-high", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ExtractOpenAIReasoningEffortForRouting([]byte(tt.body), tt.model))
		})
	}
}

func TestNormalizeOpenAIReasoningEffortPreferences(t *testing.T) {
	require.Equal(
		t,
		[]string{"low", "xhigh", "max"},
		normalizeOpenAIReasoningEffortPreferences([]any{" low ", "x-high", "LOW", "unknown", 42, "max"}),
	)
	require.Equal(
		t,
		[]string{"minimal", "high"},
		normalizeOpenAIReasoningEffortPreferences("minimal, HIGH"),
	)
}

func TestOpenAIGatewayService_ReasoningEffortAccountPreference(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		name := "legacy"
		if advanced {
			name = "advanced"
		}
		t.Run(name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

			accounts := []Account{
				{
					ID:          51001,
					Name:        "default-priority",
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Priority:    0,
				},
				{
					ID:          51002,
					Name:        "high-effort",
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Priority:    10,
					Extra: map[string]any{
						OpenAIReasoningEffortPreferencesExtraKey: []any{"high", "xhigh"},
					},
				},
			}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = false
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
				cache: &schedulerTestGatewayCache{
					sessionBindings: map[string]int64{
						"reasoning-preference": 51001,
						"reasoning-baseline":   51001,
					},
				},
				cfg:                cfg,
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
			}
			if advanced {
				svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
			}

			selection, _, err := svc.SelectAccountWithSchedulerForCapabilityAndReasoningEffort(
				context.Background(),
				nil,
				"",
				"reasoning-preference",
				"gpt-5.4",
				nil,
				OpenAIUpstreamTransportAny,
				OpenAIEndpointCapabilityChatCompletions,
				false,
				false,
				true,
				"high",
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			require.Equal(t, int64(51002), selection.Account.ID)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}

			baselineSelection, _, err := svc.SelectAccountWithSchedulerForCapabilityAndReasoningEffort(
				context.Background(),
				nil,
				"",
				"reasoning-baseline",
				"gpt-5.4",
				nil,
				OpenAIUpstreamTransportAny,
				OpenAIEndpointCapabilityChatCompletions,
				false,
				false,
				true,
				"",
			)
			require.NoError(t, err)
			require.NotNil(t, baselineSelection)
			require.NotNil(t, baselineSelection.Account)
			require.Equal(t, int64(51001), baselineSelection.Account.ID)
			if baselineSelection.ReleaseFunc != nil {
				baselineSelection.ReleaseFunc()
			}
		})
	}
}

func TestOpenAIGatewayService_ReasoningEffortPreferenceFallsBack(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		name := "legacy"
		if advanced {
			name = "advanced"
		}
		t.Run(name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

			accounts := []Account{
				{
					ID:          52001,
					Name:        "unconfigured",
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Priority:    0,
				},
				{
					ID:          52002,
					Name:        "low-only",
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Priority:    10,
					Extra: map[string]any{
						OpenAIReasoningEffortPreferencesExtraKey: []string{"low"},
					},
				},
			}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = false
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
				cache: &schedulerTestGatewayCache{
					sessionBindings: map[string]int64{"reasoning-fallback": 52001},
				},
				cfg:                cfg,
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
			}
			if advanced {
				svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
			}

			selection, _, err := svc.SelectAccountWithSchedulerForCapabilityAndReasoningEffort(
				context.Background(),
				nil,
				"",
				"reasoning-fallback",
				"gpt-5.4",
				nil,
				OpenAIUpstreamTransportAny,
				OpenAIEndpointCapabilityChatCompletions,
				false,
				false,
				true,
				"xhigh",
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			require.Equal(t, int64(52001), selection.Account.ID)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}
