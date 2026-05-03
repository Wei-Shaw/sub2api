package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type openAI403CounterResetStub struct {
	resetCalls []int64
REDACTED

func (s *openAI403CounterResetStub) IncrementOpenAI403Count(context.Context, int64, int) (int64, error) {
	return 0, nil
REDACTED

func (s *openAI403CounterResetStub) ResetOpenAI403Count(_ context.Context, accountID int64) error {
	s.resetCalls = append(s.resetCalls, accountID)
	return nil
REDACTED

func TestOpenAIGatewayServiceRecordUsage_ResetsOpenAI403CounterForZeroUsage(t *testing.T) {
	counter := &openAI403CounterResetStub{REDACTED
	rateLimitSvc := NewRateLimitService(nil, nil, nil, nil, nil)
	rateLimitSvc.SetOpenAI403CounterCache(counter)

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: trueREDACTEDREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)
	svc.rateLimitService = rateLimitSvc

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_zero_usage_reset_403",
			Model:     "gpt-5.1",
	REDACTED,
		APIKey:  &APIKey{ID: 1001, Group: &Group{RateMultiplier: 1REDACTEDREDACTED,
		User:    &User{ID: 2001REDACTED,
		Account: &Account{ID: 777, Platform: PlatformOpenAIREDACTED,
REDACTED)

REDACTED
	require.Equal(t, []int64{777REDACTED, counter.resetCalls)
	require.Equal(t, 1, usageRepo.calls)
REDACTED
