//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayService_ModelSupportUsesChannelMappedModelAcrossPlatforms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		platform string
		target   string
	}{
		{name: "anthropic", platform: PlatformAnthropic, target: "claude-terra"},
		{name: "gemini", platform: PlatformGemini, target: "gemini-terra"},
		{name: "antigravity", platform: PlatformAntigravity, target: "custom-terra"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			channelSvc := newTestChannelService(makeStandardRepo(Channel{
				ID:       1,
				Status:   StatusActive,
				GroupIDs: []int64{10},
				ModelMapping: map[string]map[string]string{
					tt.platform: {"public-luna": tt.target},
				},
			}, map[int64]string{10: tt.platform}))
			svc := &GatewayService{channelService: channelSvc}
			group := &Group{ID: 10, Platform: tt.platform, Status: StatusActive, Hydrated: true}
			ctx := svc.withGroupContext(context.Background(), group)
			account := &Account{
				Platform: tt.platform,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					// 冲突的旧账号映射不能把渠道目标当成“不支持”。
					"model_mapping": map[string]any{"public-luna": "stale-account-target"},
				},
			}

			require.True(t, svc.isModelSupportedByAccountWithContext(ctx, account, "public-luna"))
		})
	}
}

func TestImagePlaygroundModelEligible_RequestedAndChannelMapped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  string
		mapping map[string]map[string]string
		pricing []string
		model   string
		want    bool
	}{
		{
			name:    "requested model allowed",
			source:  BillingModelSourceRequested,
			pricing: []string{"gpt-image-1"},
			model:   "gpt-image-1",
			want:    true,
		},
		{
			name:    "requested model blocked",
			source:  BillingModelSourceRequested,
			pricing: []string{"gpt-image-2"},
			model:   "gpt-image-1",
			want:    false,
		},
		{
			name:    "channel mapped model allowed",
			source:  BillingModelSourceChannelMapped,
			mapping: map[string]map[string]string{PlatformOpenAI: {"gpt-image-1": "gpt-image-2"}},
			pricing: []string{"gpt-image-2"},
			model:   "gpt-image-1",
			want:    true,
		},
		{
			name:    "channel mapped model blocked",
			source:  BillingModelSourceChannelMapped,
			mapping: map[string]map[string]string{PlatformOpenAI: {"gpt-image-1": "gpt-image-1.5"}},
			pricing: []string{"gpt-image-2"},
			model:   "gpt-image-1",
			want:    false,
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
				BillingModelSource: tt.source,
				ModelPricing:       []ChannelModelPricing{{Platform: PlatformOpenAI, Models: tt.pricing}},
				ModelMapping:       tt.mapping,
			}, map[int64]string{10: PlatformOpenAI}))
			repo := &mockAccountRepoForPlatform{accounts: []Account{{
				ID:            1,
				Platform:      PlatformOpenAI,
				Type:          AccountTypeAPIKey,
				Status:        StatusActive,
				Schedulable:   true,
				AccountGroups: []AccountGroup{{GroupID: 10}},
			}}}
			svc := &GatewayService{accountRepo: repo, channelService: channelSvc}

			require.Equal(t, tt.want, svc.IsImagePlaygroundModelEligible(
				context.Background(), 10, PlatformOpenAI, tt.model,
			))
		})
	}
}

func TestImagePlaygroundModelEligible_ChannelMappedRequiresExecutableAccountMapping(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceChannelMapped,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-image-2"}},
		},
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"gpt-image-1": "gpt-image-2"},
		},
	}, map[int64]string{10: PlatformOpenAI}))
	repo := &mockAccountRepoForPlatform{accounts: []Account{{
		ID:            1,
		Platform:      PlatformOpenAI,
		Type:          AccountTypeAPIKey,
		Status:        StatusActive,
		Schedulable:   true,
		AccountGroups: []AccountGroup{{GroupID: 10}},
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-image-1": "gpt-image-1"},
		},
	}}}
	svc := &GatewayService{accountRepo: repo, channelService: channelSvc}

	require.False(t, svc.IsImagePlaygroundModelEligible(
		context.Background(), 10, PlatformOpenAI, "gpt-image-1",
	), "pricing alone is insufficient when no account accepts the channel-mapped model")

	repo.accounts[0].Credentials = map[string]any{
		"model_mapping": map[string]any{
			"gpt-image-1": "gpt-image-1",
			"gpt-image-2": "gpt-image-2",
		},
	}
	require.True(t, svc.IsImagePlaygroundModelEligible(
		context.Background(), 10, PlatformOpenAI, "gpt-image-1",
	))
}

func TestImagePlaygroundModelEligible_UpstreamRequiresAllowedAccountMapping(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-image-2"}},
		},
	}, map[int64]string{10: PlatformOpenAI}))
	repo := &mockAccountRepoForPlatform{accounts: []Account{
		{
			ID:            1,
			Platform:      PlatformOpenAI,
			Type:          AccountTypeAPIKey,
			Status:        StatusActive,
			Schedulable:   true,
			AccountGroups: []AccountGroup{{GroupID: 10}},
			Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-image-1": "gpt-image-1.5"},
			},
		},
		{
			ID:            2,
			Platform:      PlatformOpenAI,
			Type:          AccountTypeAPIKey,
			Status:        StatusActive,
			Schedulable:   true,
			AccountGroups: []AccountGroup{{GroupID: 10}},
			Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-image-1": "gpt-image-2"},
			},
		},
	}}
	svc := &GatewayService{accountRepo: repo, channelService: channelSvc}

	require.True(t, svc.IsImagePlaygroundModelEligible(
		context.Background(), 10, PlatformOpenAI, "gpt-image-1",
	))
	require.False(t, svc.IsImagePlaygroundModelEligible(
		context.Background(), 10, PlatformOpenAI, "gpt-image-1.5",
	))
	require.False(t, svc.IsImagePlaygroundModelEligible(
		context.Background(), 10, PlatformGrok, "gpt-image-1",
	), "accounts from another platform must not make the model eligible")
}

func TestImagePlaygroundModelEligible_UpstreamUsesImageForwardMappingOrder(t *testing.T) {
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
			repo := &mockAccountRepoForPlatform{accounts: []Account{{
				ID:            1,
				Platform:      PlatformOpenAI,
				Type:          tt.accountType,
				Status:        StatusActive,
				Schedulable:   true,
				AccountGroups: []AccountGroup{{GroupID: 10}},
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-image-1":   tt.originalMapped,
						"gpt-image-1.5": tt.channelMapped,
					},
				},
			}}}
			svc := &GatewayService{accountRepo: repo, channelService: channelSvc}

			require.Equal(t, tt.want, svc.IsImagePlaygroundModelEligible(
				context.Background(), 10, PlatformOpenAI, "gpt-image-1",
			))
		})
	}
}

func TestImagePlaygroundModelEligible_OpenAIUsesForwardedModelForAccountSupport(t *testing.T) {
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
	repo := &mockAccountRepoForPlatform{accounts: []Account{
		{
			ID:            1,
			Platform:      PlatformOpenAI,
			Type:          AccountTypeAPIKey,
			Status:        StatusActive,
			Schedulable:   true,
			AccountGroups: []AccountGroup{{GroupID: 10}},
			Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-image-1.5": "gpt-image-2"},
			},
		},
	}}
	svc := &GatewayService{accountRepo: repo, channelService: channelSvc}

	require.True(t, svc.IsImagePlaygroundModelEligible(
		context.Background(), 10, PlatformOpenAI, "gpt-image-1",
	), "API key support must be checked after the channel mapping")

	repo.accounts[0].Type = AccountTypeOAuth
	repo.accounts[0].Credentials = map[string]any{
		"model_mapping": map[string]any{"unrelated-text-model": "gpt-5"},
	}
	require.True(t, svc.IsImagePlaygroundModelEligible(
		context.Background(), 10, PlatformOpenAI, "gpt-image-1",
	), "OAuth image forwarding does not use the account model mapping")
}

func TestImagePlaygroundModelEligible_GrokUpstreamUsesAccountMappingForAllAccountTypes(t *testing.T) {
	t.Parallel()

	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		accountType := accountType
		t.Run(accountType, func(t *testing.T) {
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
			repo := &mockAccountRepoForPlatform{accounts: []Account{
				{
					ID:            1,
					Platform:      PlatformGrok,
					Type:          accountType,
					Status:        StatusActive,
					Schedulable:   true,
					AccountGroups: []AccountGroup{{GroupID: 10}},
					Credentials: map[string]any{
						"model_mapping": map[string]any{"grok-imagine-image": "grok-imagine-image-quality"},
					},
				},
			}}
			svc := &GatewayService{accountRepo: repo, channelService: channelSvc}

			require.True(t, svc.IsImagePlaygroundModelEligible(
				context.Background(), 10, PlatformGrok, "grok-imagine-image",
			))

			repo.accounts[0].Credentials = map[string]any{
				"model_mapping": map[string]any{"grok-imagine-image": "grok-unpriced"},
			}
			require.False(t, svc.IsImagePlaygroundModelEligible(
				context.Background(), 10, PlatformGrok, "grok-imagine-image",
			))
		})
	}
}

func TestImagePlaygroundModelEligible_UpstreamLookupFailureClosesPicker(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-image-2"}},
		},
	}, map[int64]string{10: PlatformOpenAI}))
	repo := &mockAccountRepoForPlatform{
		listModelAvailabilityCandidatesFunc: func(context.Context, *int64, []string, bool) ([]Account, error) {
			return nil, errors.New("database unavailable")
		},
	}
	svc := &GatewayService{accountRepo: repo, channelService: channelSvc}

	require.False(t, svc.IsImagePlaygroundModelEligible(
		context.Background(), 10, PlatformOpenAI, "gpt-image-2",
	))
}

// --- billingModelForRestriction ---

func TestBillingModelForRestriction_Requested(t *testing.T) {
	t.Parallel()
	got := billingModelForRestriction(BillingModelSourceRequested, "claude-sonnet-4-5", "claude-sonnet-4-6")
	require.Equal(t, "claude-sonnet-4-5", got)
}

func TestBillingModelForRestriction_ChannelMapped(t *testing.T) {
	t.Parallel()
	got := billingModelForRestriction(BillingModelSourceChannelMapped, "claude-sonnet-4-5", "claude-sonnet-4-6")
	require.Equal(t, "claude-sonnet-4-6", got)
}

func TestBillingModelForRestriction_Upstream(t *testing.T) {
	t.Parallel()
	got := billingModelForRestriction(BillingModelSourceUpstream, "claude-sonnet-4-5", "claude-sonnet-4-6")
	require.Equal(t, "", got, "upstream should return empty (per-account check needed)")
}

func TestBillingModelForRestriction_ResponseModelUsesMappedPrecheck(t *testing.T) {
	t.Parallel()
	got := billingModelForRestriction(BillingModelSourceResponse, "claude-fable-5", "claude-fable-5")
	require.Equal(t, "claude-fable-5", got)
}

func TestBillingModelForRestriction_Empty(t *testing.T) {
	t.Parallel()
	got := billingModelForRestriction("", "claude-sonnet-4-5", "claude-sonnet-4-6")
	require.Equal(t, "claude-sonnet-4-6", got, "empty source defaults to channel_mapped")
}

// --- resolveAccountUpstreamModel ---

func TestResolveAccountUpstreamModel_Antigravity(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformAntigravity,
	}
	// Antigravity 平台使用 DefaultAntigravityModelMapping
	got := resolveAccountUpstreamModel(account, "claude-sonnet-4-6")
	require.Equal(t, "claude-sonnet-4-6", got)
}

func TestResolveAccountUpstreamModel_Antigravity_Unsupported(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformAntigravity,
	}
	got := resolveAccountUpstreamModel(account, "totally-unknown-model")
	require.Equal(t, "", got, "unsupported model should return empty")
}

func TestResolveAccountUpstreamModel_NonAntigravity(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformAnthropic,
	}
	got := resolveAccountUpstreamModel(account, "claude-sonnet-4-6")
	require.Equal(t, "claude-sonnet-4-6", got, "no mapping = passthrough")
}

// --- checkChannelPricingRestriction ---

func TestCheckChannelPricingRestriction_NilGroupID(t *testing.T) {
	t.Parallel()
	svc := &GatewayService{channelService: &ChannelService{}}
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), nil, "claude-sonnet-4"))
}

func TestCheckChannelPricingRestriction_NilChannelService(t *testing.T) {
	t.Parallel()
	svc := &GatewayService{}
	gid := int64(10)
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &gid, "claude-sonnet-4"))
}

func TestCheckChannelPricingRestriction_EmptyModel(t *testing.T) {
	t.Parallel()
	svc := &GatewayService{channelService: &ChannelService{}}
	gid := int64(10)
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &gid, ""))
}

func TestCheckChannelPricingRestriction_ChannelMapped_Restricted(t *testing.T) {
	t.Parallel()
	// 渠道映射 claude-sonnet-4-5 → claude-sonnet-4-6，但定价列表只有 claude-opus-4-6
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceChannelMapped,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4-6"}},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet-4-5": "claude-sonnet-4-6"},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"}))
	svc := &GatewayService{channelService: channelSvc}

	gid := int64(10)
	require.True(t, svc.checkChannelPricingRestriction(context.Background(), &gid, "claude-sonnet-4-5"),
		"mapped model claude-sonnet-4-6 is NOT in pricing → restricted")
}

func TestCheckChannelPricingRestriction_ChannelMapped_Allowed(t *testing.T) {
	t.Parallel()
	// 渠道映射 claude-sonnet-4-5 → claude-sonnet-4-6，定价列表包含 claude-sonnet-4-6
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceChannelMapped,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet-4-6"}},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet-4-5": "claude-sonnet-4-6"},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"}))
	svc := &GatewayService{channelService: channelSvc}

	gid := int64(10)
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &gid, "claude-sonnet-4-5"),
		"mapped model claude-sonnet-4-6 IS in pricing → allowed")
}

func TestCheckChannelPricingRestriction_Requested_Restricted(t *testing.T) {
	t.Parallel()
	// billing_model_source=requested，定价列表有 claude-sonnet-4-6 但请求的是 claude-sonnet-4-5
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceRequested,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet-4-6"}},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"}))
	svc := &GatewayService{channelService: channelSvc}

	gid := int64(10)
	require.True(t, svc.checkChannelPricingRestriction(context.Background(), &gid, "claude-sonnet-4-5"),
		"requested model claude-sonnet-4-5 is NOT in pricing → restricted")
}

func TestCheckChannelPricingRestriction_Requested_Allowed(t *testing.T) {
	t.Parallel()
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceRequested,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet-4-5"}},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"}))
	svc := &GatewayService{channelService: channelSvc}

	gid := int64(10)
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &gid, "claude-sonnet-4-5"),
		"requested model IS in pricing → allowed")
}

func TestCheckChannelPricingRestriction_Upstream_SkipsPreCheck(t *testing.T) {
	t.Parallel()
	// upstream 模式：预检查始终跳过（返回 false），需逐账号检查
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4-6"}},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"}))
	svc := &GatewayService{channelService: channelSvc}

	gid := int64(10)
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &gid, "unknown-model"),
		"upstream mode should skip pre-check (per-account check needed)")
}

func TestCheckChannelPricingRestriction_RestrictModelsDisabled(t *testing.T) {
	t.Parallel()
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10},
		RestrictModels: false, // 未开启模型限制
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4-6"}},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"}))
	svc := &GatewayService{channelService: channelSvc}

	gid := int64(10)
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &gid, "any-model"),
		"RestrictModels=false → always allowed")
}

func TestCheckChannelPricingRestriction_NoChannel(t *testing.T) {
	t.Parallel()
	// 分组没有关联渠道
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) { return nil, nil },
	}
	channelSvc := newTestChannelService(repo)
	svc := &GatewayService{channelService: channelSvc}

	gid := int64(999)
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &gid, "any-model"),
		"no channel for group → allowed")
}

// --- isUpstreamModelRestrictedByChannel ---

func TestIsUpstreamModelRestrictedByChannel_Restricted(t *testing.T) {
	t.Parallel()
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10},
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4-6"}},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"}))
	svc := &GatewayService{channelService: channelSvc}

	account := &Account{Platform: PlatformAntigravity}
	// claude-sonnet-4-6 在 DefaultAntigravityModelMapping 中，映射后仍为 claude-sonnet-4-6
	// 但定价列表只有 claude-opus-4-6
	require.True(t, svc.isUpstreamModelRestrictedByChannel(context.Background(), 10, account, "claude-sonnet-4-6"),
		"upstream model claude-sonnet-4-6 NOT in pricing → restricted")
}

func TestIsUpstreamModelRestrictedByChannel_Allowed(t *testing.T) {
	t.Parallel()
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10},
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet-4-6"}},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"}))
	svc := &GatewayService{channelService: channelSvc}

	account := &Account{Platform: PlatformAntigravity}
	require.False(t, svc.isUpstreamModelRestrictedByChannel(context.Background(), 10, account, "claude-sonnet-4-6"),
		"upstream model claude-sonnet-4-6 IS in pricing → allowed")
}

func TestIsUpstreamModelRestrictedByChannel_UnsupportedModel(t *testing.T) {
	t.Parallel()
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10},
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-opus-4-6"}},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"}))
	svc := &GatewayService{channelService: channelSvc}

	account := &Account{Platform: PlatformAntigravity}
	// totally-unknown-model 不在 DefaultAntigravityModelMapping 中 → 映射结果为空
	require.False(t, svc.isUpstreamModelRestrictedByChannel(context.Background(), 10, account, "totally-unknown-model"),
		"unmappable model → upstream model empty → not restricted (account filter handles this)")
}
