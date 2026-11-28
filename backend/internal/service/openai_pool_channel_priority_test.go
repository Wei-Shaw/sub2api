package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func openAIPoolPriorityTestAccount(id int64, priority int, poolMode bool) *Account {
	credentials := map[string]any{}
	if poolMode {
		credentials["pool_mode"] = true
	}
	return &Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    priority,
		Credentials: credentials,
	}
}

func TestOpenAIPoolChannelPriorityTierUsesMinimumPriority(t *testing.T) {
	tier, handled := openAIPoolChannelPriorityTier([]*Account{
		openAIPoolPriorityTestAccount(1, 2, true),
		openAIPoolPriorityTestAccount(2, 1, true),
		openAIPoolPriorityTestAccount(3, 1, false),
	})

	require.True(t, handled)
	require.Len(t, tier, 2)
	require.ElementsMatch(t, []int64{2, 3}, []int64{tier[0].ID, tier[1].ID})
}

func TestOpenAIPoolChannelPriorityTierLeavesNonPoolCandidatesUnchanged(t *testing.T) {
	candidates := []*Account{
		openAIPoolPriorityTestAccount(1, 2, false),
		openAIPoolPriorityTestAccount(2, 1, false),
	}

	tier, handled := openAIPoolChannelPriorityTier(candidates)

	require.False(t, handled)
	require.Nil(t, tier)
}

func TestOpenAIPoolModeSelectionHonorsStrictPriority(t *testing.T) {
	ctx := context.Background()
	groupID := int64(91)
	highPriority := openAIPoolPriorityTestAccount(401, 0, true)
	lowPriority := openAIPoolPriorityTestAccount(402, 1, true)
	accounts := []Account{*highPriority, *lowPriority}
	var acquiredIDs []int64
	concurrencyCache := schedulerTestConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			highPriority.ID: {AccountID: highPriority.ID, LoadRate: 95},
			lowPriority.ID:  {AccountID: lowPriority.ID, LoadRate: 0},
		},
		acquireResults: map[int64]bool{highPriority.ID: true, lowPriority.ID: true},
		acquiredIDs:    &acquiredIDs,
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 0
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.1",
		nil,
		OpenAIUpstreamTransportAny,
		false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, highPriority.ID, selection.Account.ID)
	require.Equal(t, []int64{highPriority.ID}, acquiredIDs)
	require.Equal(t, 1, decision.CandidateCount)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
