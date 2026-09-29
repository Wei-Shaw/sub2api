//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordUsage_ReasoningPricingUsesForwardedEffort(t *testing.T) {
	for _, platform := range []string{PlatformAnthropic, PlatformOpenAI} {
		t.Run(platform, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			subRepo := &openAIRecordUsageSubRepoStub{}
			groupID := int64(77)
			inputPrice, outputPrice := 0.001, 0.002
			requested, forwarded := "max", "high"
			model := "gpt-5.4"
			if platform == PlatformAnthropic {
				model = "claude-fable-5-1"
			}
			apiKey := &APIKey{
				ID: 10, GroupID: &groupID,
				Group: &Group{
					ID: groupID, Platform: platform, Status: StatusActive, Hydrated: true, RateMultiplier: 0.5,
					ModelPricing: []ChannelModelPricing{{
						Models: []string{model}, BillingMode: BillingModeToken,
						InputPrice: &inputPrice, OutputPrice: &outputPrice,
						ReasoningEffortMultipliers: map[string]float64{"high": 1.5, "max": 3},
					}},
				},
			}
			account := &Account{ID: 30, Platform: platform, Type: AccountTypeAPIKey}
			user := &User{ID: 20}
			if platform == PlatformAnthropic {
				svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, subRepo)
				svc.resolver = NewModelPricingResolver(nil, svc.billingService)
				require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{
					Result: &ForwardResult{
						RequestID: "reasoning_pricing", Model: model, Duration: time.Second,
						Usage:           ClaudeUsage{InputTokens: 100, OutputTokens: 50},
						ReasoningEffort: &forwarded, RequestedReasoningEffort: &requested,
					},
					APIKey: apiKey, User: user, Account: account,
				}))
			} else {
				svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
				svc.resolver = NewModelPricingResolver(nil, svc.billingService)
				require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
					Result: &OpenAIForwardResult{
						RequestID: "reasoning_pricing", Model: model, Duration: time.Second,
						Usage:           OpenAIUsage{InputTokens: 100, OutputTokens: 50},
						ReasoningEffort: &forwarded, RequestedReasoningEffort: &requested,
					},
					APIKey: apiKey, User: user, Account: account,
				}))
			}
			require.NotNil(t, usageRepo.lastLog)
			require.InDelta(t, 0.3, usageRepo.lastLog.TotalCost, 1e-12)
			require.InDelta(t, 0.15, usageRepo.lastLog.ActualCost, 1e-12)
			require.Equal(t, forwarded, *usageRepo.lastLog.ReasoningEffort)
			require.Equal(t, requested, *usageRepo.lastLog.RequestedReasoningEffort)
			require.InDelta(t, 0.15, userRepo.lastAmount, 1e-12)
		})
	}
}
