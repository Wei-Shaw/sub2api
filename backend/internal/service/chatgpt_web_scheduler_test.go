package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestWebImageSchedulingKeepsCodexRateLimitIndependent(t *testing.T) {
	reset := time.Now().Add(time.Hour)
	accounts := []Account{
		{ID: 9101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, RateLimitResetAt: &reset},
		{ID: 9102, Platform: PlatformOpenAI, Type: AccountTypeWebImage, Status: StatusActive, Schedulable: true, Concurrency: 1},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	for _, capability := range []OpenAIImagesCapability{OpenAIImagesCapabilityBasic, OpenAIImagesCapabilityNative} {
		selection, _, err := svc.SelectAccountWithSchedulerForImages(context.Background(), nil, "", "gpt-image-2.5-sunburst", nil, capability)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		require.Equal(t, int64(9102), selection.Account.ID)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	}
	_, _, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), nil, "", "", "gpt-5.6-sol", nil, OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityResponses, false, false, false)
	require.Error(t, err, "web-image must not receive ordinary Codex/Responses traffic")
}
