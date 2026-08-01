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

func TestOpenAIImageSelection_UpstreamRestrictionUsesImageForwardMappingOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		accountType    string
		originalMapped string
		channelMapped  string
		allowedModel   string
		want           bool
	}{
		{
			name:           "API key allows its mapped channel result",
			accountType:    AccountTypeAPIKey,
			originalMapped: "gpt-image-wrong-order",
			channelMapped:  "gpt-image-2",
			allowedModel:   "gpt-image-2",
			want:           true,
		},
		{
			name:           "API key blocks its unpriced mapped channel result",
			accountType:    AccountTypeAPIKey,
			originalMapped: "gpt-image-2",
			channelMapped:  "gpt-image-disallowed",
			allowedModel:   "gpt-image-2",
			want:           false,
		},
		{
			name:           "OAuth allows the priced channel result",
			accountType:    AccountTypeOAuth,
			originalMapped: "gpt-image-wrong-order",
			channelMapped:  "gpt-image-disallowed",
			allowedModel:   "gpt-image-1.5",
			want:           true,
		},
		{
			name:           "OAuth blocks the unpriced channel result",
			accountType:    AccountTypeOAuth,
			originalMapped: "gpt-image-2",
			channelMapped:  "gpt-image-2",
			allowedModel:   "gpt-image-2",
			want:           false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channelSvc := newTestChannelService(makeStandardRepo(Channel{
				ID:                 1,
				Status:             StatusActive,
				GroupIDs:           []int64{10},
				RestrictModels:     true,
				BillingModelSource: BillingModelSourceUpstream,
				ModelPricing: []ChannelModelPricing{
					{Platform: PlatformOpenAI, Models: []string{tt.allowedModel}},
				},
				ModelMapping: map[string]map[string]string{
					PlatformOpenAI: {"gpt-image-1": "gpt-image-1.5"},
				},
			}, map[int64]string{10: PlatformOpenAI}))
			svc := &OpenAIGatewayService{
				accountRepo: stubOpenAIAccountRepo{accounts: []Account{{
					ID:          1,
					Platform:    PlatformOpenAI,
					Type:        tt.accountType,
					Status:      StatusActive,
					Schedulable: true,
					Credentials: map[string]any{
						"model_mapping": map[string]any{
							"gpt-image-1":   tt.originalMapped,
							"gpt-image-1.5": tt.channelMapped,
						},
					},
				}}},
				channelService: channelSvc,
			}

			groupID := int64(10)
			account, err := svc.SelectAccountForModelWithExclusions(
				withOpenAIImagesRequest(context.Background()),
				&groupID,
				"",
				"gpt-image-1",
				nil,
			)
			if tt.want {
				require.NoError(t, err)
				require.NotNil(t, account)
				require.Equal(t, int64(1), account.ID)
			} else {
				require.ErrorIs(t, err, ErrNoAvailableAccounts)
				require.Nil(t, account)
			}
		})
	}
}

func TestOpenAIImageSelection_UsesForwardedModelForAccountSupport(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-image-2", "gpt-image-1.5"}},
		},
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"gpt-image-1": "gpt-image-1.5"},
		},
	}, map[int64]string{10: PlatformOpenAI}))

	for _, account := range []Account{
		{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-image-1.5": "gpt-image-2"},
			},
		},
		{
			ID:          2,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"unrelated-text-model": "gpt-5"},
			},
		},
	} {
		account := account
		t.Run(account.Type, func(t *testing.T) {
			t.Parallel()
			svc := &OpenAIGatewayService{
				accountRepo:    stubOpenAIAccountRepo{accounts: []Account{account}},
				channelService: channelSvc,
			}
			groupID := int64(10)
			selected, err := svc.SelectAccountForModelWithExclusions(
				withOpenAIImagesRequest(context.Background()),
				&groupID,
				"",
				"gpt-image-1",
				nil,
			)
			require.NoError(t, err)
			require.NotNil(t, selected)
			require.Equal(t, account.ID, selected.ID)
		})
	}
}

func TestGrokImageSelection_UpstreamRestrictionUsesAccountMappingForAllAccountTypes(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformGrok, Models: []string{"grok-imagine-image-quality"}},
		},
		ModelMapping: map[string]map[string]string{
			PlatformGrok: {"grok-imagine-image": "grok-channel-mapped"},
		},
	}, map[int64]string{10: PlatformGrok}))
	svc := &OpenAIGatewayService{channelService: channelSvc}

	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		account := &Account{
			Platform: PlatformGrok,
			Type:     accountType,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"grok-imagine-image": "grok-imagine-image-quality"},
			},
		}
		require.False(t, svc.isUpstreamModelRestrictedByChannel(
			context.Background(), 10, account, "grok-imagine-image", false,
		))

		account.Credentials = map[string]any{
			"model_mapping": map[string]any{"grok-imagine-image": "grok-unpriced"},
		}
		require.True(t, svc.isUpstreamModelRestrictedByChannel(
			context.Background(), 10, account, "grok-imagine-image", false,
		))
	}
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
