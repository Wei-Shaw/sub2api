package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayServiceRecordUsage_WebImagePreservesCodexHealth(t *testing.T) {
	for _, model := range []string{"gpt-image-1", "gpt-5.1"} {
		t.Run(model, func(t *testing.T) {
			counter := &openAI403CounterResetStub{}
			rateLimits := NewRateLimitService(nil, nil, nil, nil, nil)
			rateLimits.SetOpenAI403CounterCache(counter)
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			svc.rateLimitService = rateLimits

			store := jobTestStore(t)
			ref := completedJobBillingRef(t, store, jobTestKey(model))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(webImageJobBillingContextKey, ref)
			result := &OpenAIForwardResult{
				Model:      model,
				ImageCount: 1,
				Usage:      OpenAIUsage{InputTokens: 10, OutputTokens: 5},
			}
			// Images and Responses attach this same server-owned reference before
			// their handlers submit normal usage billing.
			applyChatGPTWebImageJobBilling(c, result)
			resetAt := time.Now().Add(time.Hour)
			account := &Account{
				ID: 777, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Status: StatusError, ErrorMessage: "Codex quota exhausted",
				Schedulable: false, RateLimitResetAt: &resetAt,
			}
			before := *account
			input := &OpenAIRecordUsageInput{
				Result: result,
				APIKey: &APIKey{ID: 1001, Group: &Group{RateMultiplier: 1}},
				User:   &User{ID: 2001}, Account: account,
			}

			require.NoError(t, svc.RecordUsage(context.Background(), input))
			require.Equal(t, 1, billingRepo.calls, "first delivery must reach normal billing")
			require.Equal(t, 1, usageRepo.calls)
			require.Empty(t, counter.resetCalls, "web success cannot clear Codex 403 history")
			require.Equal(t, before, *account, "web billing cannot restore error, scheduling or 429 state")

			// Rehydrate the durable job as a replay after restart, exercising the
			// same RecordUsage entry point without charging or changing health.
			restarted := &webImageJobStore{dir: store.dir, active: make(map[string]*webImageRunningJob)}
			replayedRef, err := restarted.billingRef(ref.key)
			require.NoError(t, err)
			c.Set(webImageJobBillingContextKey, replayedRef)
			applyChatGPTWebImageJobBilling(c, result)
			require.NoError(t, svc.RecordUsage(context.Background(), input))
			require.Equal(t, 1, billingRepo.calls)
			require.Equal(t, 1, usageRepo.calls)
			require.Empty(t, counter.resetCalls)
			require.Equal(t, before, *account)
		})
	}
}

func TestOpenAIGatewayServiceRecordUsage_CodexSuccessStillResets403(t *testing.T) {
	for _, tc := range []struct {
		name       string
		model      string
		requestID  string
		imageCount int
	}{
		{name: "text", model: "gpt-5.1", requestID: "resp_text"},
		{name: "hosted image", model: "gpt-5.1", requestID: "resp_hosted_image", imageCount: 1},
		{name: "image endpoint", model: "gpt-image-1", requestID: "resp_image", imageCount: 1},
		{name: "untrusted web image request id", model: "gpt-image-1", requestID: "web-image:00000000-0000-4000-8000-000000000000", imageCount: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counter := &openAI403CounterResetStub{}
			rateLimits := NewRateLimitService(nil, nil, nil, nil, nil)
			rateLimits.SetOpenAI403CounterCache(counter)
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			svc.rateLimitService = rateLimits

			require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID: tc.requestID, Model: tc.model, ImageCount: tc.imageCount,
					Usage: OpenAIUsage{InputTokens: 10, OutputTokens: 5},
				},
				APIKey:  &APIKey{ID: 1001, Group: &Group{RateMultiplier: 1}},
				User:    &User{ID: 2001},
				Account: &Account{ID: 777, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			}))
			require.Equal(t, []int64{777}, counter.resetCalls)
			require.Equal(t, 1, billingRepo.calls)
			require.Equal(t, 1, usageRepo.calls)
		})
	}
}
