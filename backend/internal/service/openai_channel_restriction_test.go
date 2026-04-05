//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAISelectAccountForModelWithExclusions_ChannelMappedRestrictionRejectsEarly(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceChannelMapped,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-4o"}},
		},
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"gpt-4.1": "o3-mini"},
		},
	}, map[int64]string{10: PlatformOpenAI}))

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true},
		}},
		channelService: channelSvc,
	}

	groupID := int64(10)
	_, err := svc.SelectAccountForModelWithExclusions(context.Background(), &groupID, "", "gpt-4.1", nil)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Contains(t, err.Error(), "channel pricing restriction")
}

func TestOpenAISelectAccountForModelWithExclusions_UpstreamRestrictionSkipsDisallowedAccount(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"o3-mini"}},
		},
	}, map[int64]string{10: PlatformOpenAI}))

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Priority:    10,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-4.1": "gpt-4o"},
				},
			},
			{
				ID:          2,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Priority:    20,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-4.1": "o3-mini"},
				},
			},
		}},
		channelService: channelSvc,
	}

	groupID := int64(10)
	account, err := svc.SelectAccountForModelWithExclusions(context.Background(), &groupID, "", "gpt-4.1", nil)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, int64(2), account.ID)
}

func TestOpenAISelectAccountForModelWithExclusions_StickyRestrictedUpstreamFallsBack(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"o3-mini"}},
		},
	}, map[int64]string{10: PlatformOpenAI}))

	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:sticky-session": 1},
	}
	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Priority:    10,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-4.1": "gpt-4o"},
				},
			},
			{
				ID:          2,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Priority:    20,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-4.1": "o3-mini"},
				},
			},
		}},
		channelService: channelSvc,
		cache:          cache,
	}

	groupID := int64(10)
	account, err := svc.SelectAccountForModelWithExclusions(context.Background(), &groupID, "sticky-session", "gpt-4.1", nil)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, int64(2), account.ID)
	require.Equal(t, 1, cache.deletedSessions["openai:sticky-session"])
	require.Equal(t, int64(2), cache.sessionBindings["openai:sticky-session"])
}
