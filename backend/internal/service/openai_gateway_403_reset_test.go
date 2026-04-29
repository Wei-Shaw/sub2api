package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type openAI403CounterResetStub struct {
	resetCalls []int64
}

func (s *openAI403CounterResetStub) IncrementOpenAI403Count(context.Context, int64, int) (int64, error) {
	return 0, nil
}

func (s *openAI403CounterResetStub) ResetOpenAI403Count(_ context.Context, accountID int64) error {
	s.resetCalls = append(s.resetCalls, accountID)
	return nil
}

// TestOpenAIGatewayServiceRecordUsage_ResetsOpenAI403CounterOnZeroUsage 验证：
// 即便上游返回零 token 用量（如 gpt-5.4-pro 不带 usage 字段），403 计数器仍必须被重置；
// 同时 RecordUsage 不再因零 usage 提前 return，而是继续走完 service_quota.Record 链路。
func TestOpenAIGatewayServiceRecordUsage_ResetsOpenAI403CounterOnZeroUsage(t *testing.T) {
	counter := &openAI403CounterResetStub{}
	rateLimitSvc := NewRateLimitService(nil, nil, nil, nil, nil)
	rateLimitSvc.SetOpenAI403CounterCache(counter)

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)
	svc.rateLimitService = rateLimitSvc

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result:  &OpenAIForwardResult{RequestID: "resp_zero_usage_403"},
		APIKey:  &APIKey{ID: 1},
		User:    &User{ID: 2},
		Account: &Account{ID: 777, Platform: PlatformOpenAI},
	})

	require.NoError(t, err)
	require.Equal(t, []int64{777}, counter.resetCalls)
	// 零 token 用量场景下 DB usage_log 仍跳过写入
	require.Equal(t, 0, usageRepo.calls)
}
