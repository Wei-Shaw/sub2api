//go:build unit

package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSimpleModeRecordUsageWindowOptIn(t *testing.T) {
	for _, openAI := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("openai=%v/enabled=%v/stream=%v", openAI, enabled, stream), func(t *testing.T) {
					logs := &openAIRecordUsageLogRepoStub{inserted: true}
					billing := &openAIRecordUsageBillingRepoStub{}
					users := &openAIRecordUsageUserRepoStub{}
					subs := &openAIRecordUsageSubRepoStub{}
					key := &APIKey{ID: 1, Quota: 100, RateLimit5h: 30, Group: &Group{RateMultiplier: 1}}
					user := &User{ID: 2, Balance: 0}
					account := &Account{ID: 3, Type: AccountTypeAPIKey}
					if openAI {
						svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, billing, users, subs, nil)
						svc.cfg.RunMode = config.RunModeSimple
						svc.cfg.SimpleModeKeyRateLimitEnabled = enabled
						err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
							Result: &OpenAIForwardResult{RequestID: "simple-request", Model: "gpt-5.1", Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 20}, Stream: stream},
							APIKey: key, User: user, Account: account,
						})
						require.NoError(t, err)
					} else {
						svc := newGatewayRecordUsageServiceWithBillingRepoForTest(logs, billing, users, subs)
						svc.cfg.RunMode = config.RunModeSimple
						svc.cfg.SimpleModeKeyRateLimitEnabled = enabled
						err := svc.RecordUsage(context.Background(), &RecordUsageInput{
							Result: &ForwardResult{RequestID: "simple-request", Model: "claude-sonnet-4", Usage: ClaudeUsage{InputTokens: 100, OutputTokens: 20}, Stream: stream},
							APIKey: key, User: user, Account: account,
						})
						require.NoError(t, err)
					}
					require.Equal(t, 1, logs.calls)
					require.Positive(t, logs.lastLog.ActualCost)
					require.Zero(t, users.deductCalls)
					require.Zero(t, subs.incrementCalls)
					if !enabled {
						require.Zero(t, billing.calls)
						return
					}
					require.Equal(t, 1, billing.calls)
					require.InDelta(t, logs.lastLog.ActualCost, billing.lastCmd.APIKeyRateLimitCost, 1e-12)
					require.Zero(t, billing.lastCmd.BalanceCost)
					require.Zero(t, billing.lastCmd.SubscriptionCost)
					require.Zero(t, billing.lastCmd.APIKeyQuotaCost)
					require.Zero(t, billing.lastCmd.AccountQuotaCost)
				})
			}
		}
	}
}
