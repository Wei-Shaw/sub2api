//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func newOpenAIProjectionActiveAccount(id int64, concurrency int, usedPercent float64, models []string) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: concurrency,
		Credentials: map[string]any{"plan_type": "free"},
		Extra: map[string]any{
			openAICapabilityExplicitModelsExtraKey: models,
			"codex_7d_used_percent":                usedPercent,
		},
	}
}

func newOpenAIProjectionExhaustedAccount(id int64, concurrency int, models []string) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: concurrency,
		Extra: map[string]any{
			openAICapabilityExplicitModelsExtraKey: models,
			"quota_limit":                          float64(100),
			"quota_used":                           float64(100),
		},
	}
}

func projectionAccountIDs(accounts []Account) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}

func projectionReserveIDSlice(reserve map[int64]struct{}) []int64 {
	ids := make([]int64, 0, len(reserve))
	for id := range reserve {
		ids = append(ids, id)
	}
	return ids
}

func TestBuildOpenAICanonicalModelCatalog_UsesFiniteSources(t *testing.T) {
	t.Parallel()

	accounts := []Account{
		{
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5.4-Sys":              "gpt-5.4",
					"gpt-*":                    "gpt-5.4",
					"gpt-5.3-codex-spark-high": "gpt-5.3-codex-spark",
				},
			},
			Extra: map[string]any{
				"openai_capability_explicit_models": []string{"gpt-5.6", "gpt-5.4-mini-medium"},
			},
			Groups: []*Group{
				{
					DefaultMappedModel:    "gpt-5.2-high",
					AllowMessagesDispatch: true,
					MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
						OpusMappedModel: "gpt-5.4-high",
						ExactModelMappings: map[string]string{
							"claude-sonnet-4-5-20250929": "gpt-5.1-codex-max-high",
						},
					},
				},
			},
		},
	}

	catalog := BuildOpenAICanonicalModelCatalog(accounts, nil, []string{"gpt-5.4-nano-Sys"})

	require.ElementsMatch(t, []string{
		"gpt-5.1-codex-max",
		"gpt-5.2",
		"gpt-5.3-codex",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.4-nano",
		"gpt-5.6",
	}, catalog)
	require.NotContains(t, catalog, "gpt-*")
	require.NotContains(t, catalog, "gpt-unknown-live")
}

func TestBuildOpenAICanonicalModelCatalog_PreservesExplicitConfiguredUnknownModel(t *testing.T) {
	t.Parallel()

	accounts := []Account{
		{
			Groups: []*Group{
				{
					AllowMessagesDispatch: true,
					MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
						OpusMappedModel: "gpt-5.9-high",
					},
				},
			},
		},
	}

	catalog := BuildOpenAICanonicalModelCatalog(accounts, nil, nil)

	require.Contains(t, catalog, "gpt-5.9-high")
	require.NotContains(t, catalog, "gpt-5.9")
}

func TestProjectionModelReachability_UnknownModelNeedsExplicitCapability(t *testing.T) {
	t.Parallel()

	account := Account{Credentials: map[string]any{}, Extra: map[string]any{}}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.6"))
}

func TestProjectionModelReachability_MissingMappingRejectsUnknownModel(t *testing.T) {
	t.Parallel()

	account := Account{Credentials: map[string]any{}, Extra: map[string]any{}}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.unknown"))
}

func TestProjectionModelReachability_EmptyMappingRejectsUnknownModel(t *testing.T) {
	t.Parallel()

	account := Account{
		Credentials: map[string]any{"model_mapping": map[string]any{}},
		Extra:       map[string]any{},
	}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.unknown"))
}

func TestProjectionModelReachability_UnknownModelRejectsNoMappingAndWildcardByDefault(t *testing.T) {
	t.Parallel()

	account := Account{
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-*": "gpt-5.4"}},
		Extra:       map[string]any{},
	}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.unknown"))
}

func TestProjectionModelReachability_ExplicitCapabilityOverridesConservativeDefault(t *testing.T) {
	t.Parallel()

	account := Account{Extra: map[string]any{"openai_capability_explicit_models": []string{"gpt-5.6"}}}
	require.True(t, accountSupportsProjectionModel(account, "gpt-5.6"))
}

func TestProjectionModelReachability_WildcardRulesAllowCatalogModel(t *testing.T) {
	t.Parallel()

	account := Account{Extra: map[string]any{"openai_capability_wildcard_rules": []string{"gpt-5.7-*"}}}
	snapshot := buildOpenAIModelCapabilitySnapshot(account)
	catalog := map[string]struct{}{"gpt-5.7-preview": {}}

	require.True(t, account.SupportsProjectionModel("gpt-5.7-preview", snapshot, catalog))
}

func TestProjectionModelReachability_WildcardRulesRejectCatalogOutsideModel(t *testing.T) {
	t.Parallel()

	account := Account{Extra: map[string]any{"openai_capability_wildcard_rules": []string{"gpt-5.7-*"}}}
	snapshot := buildOpenAIModelCapabilitySnapshot(account)
	catalog := map[string]struct{}{"gpt-5.7-preview": {}}

	require.False(t, account.SupportsProjectionModel("gpt-5.8-preview", snapshot, catalog))
}

func TestProjectionModelReachability_DefaultAllowAllowsCatalogModel(t *testing.T) {
	t.Parallel()

	account := Account{Extra: map[string]any{"openai_capability_default_allow": true}}
	snapshot := buildOpenAIModelCapabilitySnapshot(account)
	catalog := map[string]struct{}{"gpt-5.8-preview": {}}

	require.True(t, account.SupportsProjectionModel("gpt-5.8-preview", snapshot, catalog))
}

func TestProjectionModelReachability_DefaultAllowRejectsCatalogOutsideModel(t *testing.T) {
	t.Parallel()

	account := Account{Extra: map[string]any{"openai_capability_default_allow": true}}
	snapshot := buildOpenAIModelCapabilitySnapshot(account)
	catalog := map[string]struct{}{"gpt-5.8-preview": {}}

	require.False(t, account.SupportsProjectionModel("gpt-5.9-preview", snapshot, catalog))
}

func TestProjectionModelReachability_GroupConfiguredModelsProvideSupportProof(t *testing.T) {
	t.Parallel()

	account := Account{
		Groups: []*Group{{DefaultMappedModel: "gpt-5.2-high"}},
	}

	require.True(t, accountSupportsProjectionModel(account, "gpt-5.2"))
}

func TestProjectionModelReachability_MessagesDispatchConfiguredModelsProvideSupportProof(t *testing.T) {
	t.Parallel()

	account := Account{
		Groups: []*Group{
			{
				AllowMessagesDispatch: true,
				MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
					OpusMappedModel: "gpt-5.9-high",
				},
			},
		},
	}

	require.True(t, accountSupportsProjectionModel(account, "gpt-5.9-high"))
}

func TestNormalizeOpenAIProjectionModelKey_ReusesCompatNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "sys suffix", input: "gpt-5.4-Sys", want: "gpt-5.4"},
		{name: "compat reasoning and sys", input: " gpt-5.4-xhigh-Sys ", want: "gpt-5.4"},
		{name: "codex alias and sys", input: "gpt-5.3-codex-spark-high-Sys", want: "gpt-5.3-codex"},
		{name: "unknown model preserved", input: "gpt-5.6-Sys", want: "gpt-5.6"},
		{name: "unknown compat-like suffix preserved conservatively", input: "gpt-5.9-high-Sys", want: "gpt-5.9-high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeOpenAIProjectionModelKey(tt.input))
		})
	}
}

func TestLoadOpenAIProjectionInputs_PreservesBroadSourceExhaustedMembers(t *testing.T) {
	t.Parallel()

	active := newOpenAIProjectionActiveAccount(101, 1, 10, []string{"gpt-5.4"})
	exhausted := newOpenAIProjectionExhaustedAccount(102, 1, []string{"gpt-5.4"})

	svc := &SchedulerSnapshotService{
		accountRepo: splitPoolOpenAIAccountRepo{
			ungrouped: []Account{active},
			broad:     []Account{active, exhausted},
		},
	}

	inputs, err := svc.loadOpenAIProjectionInputs(context.Background(), SchedulerBucket{Platform: PlatformOpenAI, Mode: SchedulerModeSingle})
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{101, 102}, projectionAccountIDs(inputs.AccountsAll))
	require.Contains(t, inputs.ExhaustedBroadIDs, int64(102))
	require.NotContains(t, inputs.ExhaustedBroadIDs, int64(101))
	require.Contains(t, inputs.CanonicalCatalog, "gpt-5.4")
	require.Contains(t, inputs.CapabilityByID, int64(102))
}

func TestBuildOpenAIModelSubsetProjection_NewModelTwoAccountsPromotesReserve(t *testing.T) {
	t.Parallel()

	inputs := &OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.6"},
		AccountsAll: []Account{
			newOpenAIProjectionActiveAccount(1, 1, 10, []string{"gpt-5.6"}),
			newOpenAIProjectionActiveAccount(2, 1, 20, []string{"gpt-5.6"}),
		},
	}

	projection := BuildOpenAIModelSubsetProjection(inputs)
	view := projection.ViewForModel("gpt-5.6")

	require.Empty(t, view.ExhaustedBaseIDs)
	require.NotEmpty(t, view.ReserveOverflowIDs)
	require.NotEmpty(t, projection.AccountReserveIDs)
	for _, id := range view.ReserveOverflowIDs {
		_, ok := projection.AccountReserveIDs[id]
		require.True(t, ok)
	}
}

func TestBuildOpenAIModelSubsetProjection_AsymmetricMatrixLiftsReserveAcrossSupportedModels(t *testing.T) {
	t.Parallel()

	accountA := newOpenAIProjectionActiveAccount(1, 1, 10, []string{"gpt-5.6", "gpt-5.4"})
	accountB := newOpenAIProjectionExhaustedAccount(2, 1, []string{"gpt-5.6"})
	accountC := newOpenAIProjectionActiveAccount(3, 2, 90, []string{"gpt-5.4"})

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.4", "gpt-5.6"},
		AccountsAll:      []Account{accountA, accountB, accountC},
	})

	view56 := projection.ViewForModel("gpt-5.6")
	view54 := projection.ViewForModel("gpt-5.4")

	require.ElementsMatch(t, []int64{2}, view56.ExhaustedBaseIDs)
	require.ElementsMatch(t, []int64{1}, view56.ReserveOverflowIDs)
	require.ElementsMatch(t, []int64{1, 3}, view54.ReserveOverflowIDs)
	require.NotContains(t, view54.ReserveOverflowIDs, int64(2))
	require.NotContains(t, view54.ExhaustedBaseIDs, int64(1))
	require.ElementsMatch(t, []int64{1, 3}, projectionReserveIDSlice(projection.AccountReserveIDs))
}

func TestBuildOpenAIModelSubsetProjection_ExhaustedEmptyMeansOneHundredPercent(t *testing.T) {
	t.Parallel()

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.6"},
		AccountsAll: []Account{
			newOpenAIProjectionActiveAccount(11, 1, 15, []string{"gpt-5.6"}),
		},
	})

	view := projection.ViewForModel("gpt-5.6")
	require.Empty(t, view.ExhaustedBaseIDs)
	require.ElementsMatch(t, []int64{11}, view.ReserveOverflowIDs)
	_, ok := projection.AccountReserveIDs[11]
	require.True(t, ok)
}

func TestBuildOpenAIModelSubsetProjection_ReserveThresholdMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		accounts      []Account
		wantExhausted []int64
		wantReserve   []int64
	}{
		{
			name: "exhausted_capacity_already_covers_sixty_percent",
			accounts: []Account{
				newOpenAIProjectionExhaustedAccount(21, 2, []string{"gpt-5.6"}),
				newOpenAIProjectionActiveAccount(22, 1, 10, []string{"gpt-5.6"}),
			},
			wantExhausted: []int64{21},
			wantReserve:   nil,
		},
		{
			name: "reserve_fills_remaining_capacity_gap",
			accounts: []Account{
				newOpenAIProjectionExhaustedAccount(31, 1, []string{"gpt-5.6"}),
				newOpenAIProjectionActiveAccount(32, 1, 10, []string{"gpt-5.6"}),
				newOpenAIProjectionActiveAccount(33, 2, 90, []string{"gpt-5.6"}),
			},
			wantExhausted: []int64{31},
			wantReserve:   []int64{33},
		},
		{
			name: "exhausted_empty_is_treated_as_one_hundred_percent",
			accounts: []Account{
				newOpenAIProjectionActiveAccount(41, 1, 15, []string{"gpt-5.6"}),
			},
			wantExhausted: nil,
			wantReserve:   []int64{41},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
				Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
				CanonicalCatalog: []string{"gpt-5.6"},
				AccountsAll:      tt.accounts,
			})

			view := projection.ViewForModel("gpt-5.6")
			require.ElementsMatch(t, tt.wantExhausted, view.ExhaustedBaseIDs)
			require.ElementsMatch(t, tt.wantReserve, view.ReserveOverflowIDs)
			for _, exhaustedID := range view.ExhaustedBaseIDs {
				require.NotContains(t, view.ReserveOverflowIDs, exhaustedID)
			}
		})
	}
}
