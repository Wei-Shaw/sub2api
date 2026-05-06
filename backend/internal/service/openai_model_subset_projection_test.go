//go:build unit || integration

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
		"gpt-5.3-codex-spark",
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

func TestBuildOpenAICanonicalModelCatalog_UsesCatalogModelsFromExtra(t *testing.T) {
	t.Parallel()

	accounts := []Account{{
		Extra: map[string]any{
			openAICapabilityCatalogModelsExtraKey: []string{"gpt-5.unknown-Sys"},
		},
	}}

	catalog := BuildOpenAICanonicalModelCatalog(accounts, nil, nil)

	require.Contains(t, catalog, "gpt-5.unknown")
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

func TestUnknownModel_EmptyMappingAndWildcardRemainConservativelyExcluded(t *testing.T) {
	t.Parallel()

	accounts := []Account{
		{
			ID:          901,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{
				"plan_type":     "free",
				"model_mapping": map[string]any{},
			},
			Extra: map[string]any{
				openAICapabilityWildcardRulesExtraKey: []string{"gpt-5.*"},
			},
		},
	}

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: BuildOpenAICanonicalModelCatalog(accounts, nil, nil),
		AccountsAll:      accounts,
		CapabilityByID: map[int64]OpenAIModelCapabilitySnapshot{
			accounts[0].ID: buildOpenAIModelCapabilitySnapshot(accounts[0]),
		},
	})

	_, ok := projection.ViewForModel("gpt-5.unknown")
	require.False(t, ok)
	require.Empty(t, projection.AccountReserveIDs)
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

func TestNormalizeOpenAIProjectionModelKey_PreservesProjectionIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "sys suffix", input: "gpt-5.4-Sys", want: "gpt-5.4"},
		{name: "compat reasoning and sys", input: " gpt-5.4-xhigh-Sys ", want: "gpt-5.4"},
		{name: "codex spark reasoning and sys", input: "gpt-5.3-codex-spark-high-Sys", want: "gpt-5.3-codex-spark"},
		{name: "removed upstream model preserved for projection", input: "gpt-5.1", want: "gpt-5.1"},
		{name: "legacy codex max reasoning preserved for projection", input: "gpt-5.1-codex-max-high-Sys", want: "gpt-5.1-codex-max"},
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

func TestMergeOpenAIExhaustedAccountsFromBroadSource_PreservesExpectedMergeSemantics(t *testing.T) {
	t.Parallel()

	base := []Account{
		newOpenAIProjectionActiveAccount(201, 1, 10, []string{"gpt-5.4"}),
		newOpenAIProjectionExhaustedAccount(202, 1, []string{"gpt-5.4"}),
	}
	broad := []Account{
		newOpenAIProjectionExhaustedAccount(201, 1, []string{"gpt-5.4"}),
		newOpenAIProjectionActiveAccount(202, 1, 10, []string{"gpt-5.4"}),
		newOpenAIProjectionExhaustedAccount(203, 1, []string{"gpt-5.4"}),
		newOpenAIProjectionActiveAccount(204, 1, 10, []string{"gpt-5.4"}),
		{ID: 205, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
	}

	merged, exhaustedBroadIDs := mergeOpenAIExhaustedAccountsFromBroadSource(base, broad)

	require.ElementsMatch(t, []int64{201, 202, 203}, projectionAccountIDs(merged))
	require.Contains(t, exhaustedBroadIDs, int64(201))
	require.Contains(t, exhaustedBroadIDs, int64(203))
	require.NotContains(t, exhaustedBroadIDs, int64(202))

	byID := make(map[int64]Account, len(merged))
	for _, account := range merged {
		byID[account.ID] = account
	}
	account201 := byID[201]
	account202 := byID[202]
	account203 := byID[203]
	require.True(t, account201.IsExhausted())
	require.False(t, account202.IsExhausted())
	require.True(t, account203.IsExhausted())
	_, exists := byID[204]
	require.False(t, exists)
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
	view, ok := projection.ViewForModel("gpt-5.6")

	require.True(t, ok)
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

	view56, ok56 := projection.ViewForModel("gpt-5.6")
	view54, ok54 := projection.ViewForModel("gpt-5.4")

	require.True(t, ok56)
	require.True(t, ok54)
	require.ElementsMatch(t, []int64{2}, view56.ExhaustedBaseIDs)
	require.ElementsMatch(t, []int64{1}, view56.ReserveOverflowIDs)
	require.ElementsMatch(t, []int64{1, 3}, view54.ReserveOverflowIDs)
	require.NotContains(t, view54.ReserveOverflowIDs, int64(2))
	require.NotContains(t, view54.ExhaustedBaseIDs, int64(1))
	require.ElementsMatch(t, []int64{1, 3}, projectionReserveIDSlice(projection.AccountReserveIDs))
}

func TestBuildOpenAIModelSubsetProjection_ResponsesImageGenerationInheritsReserveIdentity(t *testing.T) {
	t.Parallel()

	previousReserve := newOpenAIProjectionPaidTierAccount(71, 1, "team", []string{"gpt-5.5", "gpt-image-2"})
	otherSubsetReserve := newOpenAIProjectionPaidTierAccount(72, 1, "team", []string{"gpt-5.4", "gpt-5.5", "gpt-image-2"})
	currentReserve := newOpenAIProjectionPaidTierAccount(73, 1, "team", []string{"gpt-5.5", "gpt-image-2"})
	exhausted55 := newOpenAIProjectionExhaustedAccount(74, 2, []string{"gpt-5.5", "gpt-image-2"})
	key := openAIResponsesImageGenerationProjectionKey("gpt-5.5", "gpt-image-2")

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.4", "gpt-5.5"},
		AccountsAll:      []Account{previousReserve, otherSubsetReserve, currentReserve, exhausted55},
		PreviousProjection: &OpenAIModelSubsetProjection{ResponsesImageGenerationModels: map[string]OpenAIModelRoleView{
			key: {CanonicalModel: "gpt-5.5", ReserveOverflowIDs: []int64{previousReserve.ID}},
		}},
	})

	base54, ok := projection.ViewForModel("gpt-5.4")
	require.True(t, ok)
	require.Contains(t, base54.ReserveOverflowIDs, otherSubsetReserve.ID)
	base55, ok := projection.ViewForModel("gpt-5.5")
	require.True(t, ok)
	require.NotContains(t, base55.ReserveOverflowIDs, previousReserve.ID)
	view, ok := projection.ViewForResponsesImageGeneration(&OpenAIResponsesImageGenerationRequirement{Enabled: true, MainModel: "gpt-5.5", ImageModel: "gpt-image-2"})
	require.True(t, ok)
	require.ElementsMatch(t, []int64{previousReserve.ID}, view.ReserveOverflowIDs)
	require.NotContains(t, view.ExhaustedBaseIDs, previousReserve.ID)
}

func TestBuildOpenAIModelSubsetProjection_ResponsesImageGenerationPrefersOtherSubsetReserveWithinCapacity(t *testing.T) {
	t.Parallel()

	otherSubsetReserve := newOpenAIProjectionPaidTierAccount(82, 1, "team", []string{"gpt-5.4", "gpt-5.5", "gpt-image-2"})
	currentReserve := newOpenAIProjectionPaidTierAccount(83, 1, "team", []string{"gpt-5.5", "gpt-image-2"})
	exhausted55 := newOpenAIProjectionExhaustedAccount(84, 2, []string{"gpt-5.5", "gpt-image-2"})

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.4", "gpt-5.5"},
		AccountsAll:      []Account{otherSubsetReserve, currentReserve, exhausted55},
	})

	base54, ok := projection.ViewForModel("gpt-5.4")
	require.True(t, ok)
	require.Contains(t, base54.ReserveOverflowIDs, otherSubsetReserve.ID)
	base55, ok := projection.ViewForModel("gpt-5.5")
	require.True(t, ok)
	require.Contains(t, base55.ReserveOverflowIDs, currentReserve.ID)
	view, ok := projection.ViewForResponsesImageGeneration(&OpenAIResponsesImageGenerationRequirement{Enabled: true, MainModel: "gpt-5.5", ImageModel: "gpt-image-2"})
	require.True(t, ok)
	require.ElementsMatch(t, []int64{otherSubsetReserve.ID}, view.ReserveOverflowIDs)
}

func TestBuildOpenAIModelSubsetProjection_ResponsesImageGenerationFillsRemainingCapacityAfterPreviousReserve(t *testing.T) {
	t.Parallel()

	previousReserve := newOpenAIProjectionPaidTierAccount(91, 1, "team", []string{"gpt-5.5", "gpt-image-2"})
	otherSubsetReserve := newOpenAIProjectionPaidTierAccount(92, 1, "team", []string{"gpt-5.4", "gpt-5.5", "gpt-image-2"})
	currentReserve := newOpenAIProjectionPaidTierAccount(93, 1, "team", []string{"gpt-5.5", "gpt-image-2"})
	exhausted55 := newOpenAIProjectionExhaustedAccount(94, 1, []string{"gpt-5.5", "gpt-image-2"})
	key := openAIResponsesImageGenerationProjectionKey("gpt-5.5", "gpt-image-2")

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.4", "gpt-5.5"},
		AccountsAll:      []Account{previousReserve, otherSubsetReserve, currentReserve, exhausted55},
		PreviousProjection: &OpenAIModelSubsetProjection{ResponsesImageGenerationModels: map[string]OpenAIModelRoleView{
			key: {CanonicalModel: "gpt-5.5", ReserveOverflowIDs: []int64{previousReserve.ID}},
		}},
	})

	view, ok := projection.ViewForResponsesImageGeneration(&OpenAIResponsesImageGenerationRequirement{Enabled: true, MainModel: "gpt-5.5", ImageModel: "gpt-image-2"})
	require.True(t, ok)
	require.ElementsMatch(t, []int64{previousReserve.ID, otherSubsetReserve.ID}, view.ReserveOverflowIDs)
	require.NotContains(t, view.ReserveOverflowIDs, currentReserve.ID)
}

func TestBuildOpenAIModelSubsetProjection_ResponsesImageGenerationRequiresProjectionMainModelSupport(t *testing.T) {
	t.Parallel()

	imageOnly := newOpenAIProjectionPaidTierAccount(101, 1, "team", []string{"gpt-image-2"})
	exhausted55 := newOpenAIProjectionExhaustedAccount(102, 1, []string{"gpt-5.5", "gpt-image-2"})

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.5"},
		AccountsAll:      []Account{imageOnly, exhausted55},
	})

	view, ok := projection.ViewForResponsesImageGeneration(&OpenAIResponsesImageGenerationRequirement{Enabled: true, MainModel: "gpt-5.5", ImageModel: "gpt-image-2"})
	require.True(t, ok)
	require.ElementsMatch(t, []int64{exhausted55.ID}, view.ExhaustedBaseIDs)
	require.NotContains(t, view.ReserveOverflowIDs, imageOnly.ID)
	require.Empty(t, view.ReserveOverflowIDs)
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

	view, ok := projection.ViewForModel("gpt-5.6")
	require.True(t, ok)
	require.Empty(t, view.ExhaustedBaseIDs)
	require.ElementsMatch(t, []int64{11}, view.ReserveOverflowIDs)
	_, reserveOK := projection.AccountReserveIDs[11]
	require.True(t, reserveOK)
}

func TestBuildOpenAIModelSubsetProjection_PaidTierOnlyGPT55PromotesReserve(t *testing.T) {
	t.Parallel()

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.5"},
		AccountsAll: []Account{
			newOpenAIProjectionPaidTierAccount(51, 1, "team", []string{"gpt-5.5"}),
		},
	})

	view, ok := projection.ViewForModel("gpt-5.5")
	require.True(t, ok)
	require.Empty(t, view.ExhaustedBaseIDs)
	require.ElementsMatch(t, []int64{51}, view.ReserveOverflowIDs)
	_, reserveOK := projection.AccountReserveIDs[51]
	require.False(t, reserveOK)
}

func TestBuildOpenAIModelSubsetProjection_PaidTierOnlyGPT55ReserveDoesNotLiftAcrossModels(t *testing.T) {
	t.Parallel()

	paidTier := newOpenAIProjectionPaidTierAccount(61, 1, "team", []string{"gpt-5.5", "gpt-5.4"})
	exhausted54 := newOpenAIProjectionExhaustedAccount(62, 2, []string{"gpt-5.4"})

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.4", "gpt-5.5"},
		AccountsAll:      []Account{paidTier, exhausted54},
	})

	view55, ok55 := projection.ViewForModel("gpt-5.5")
	view54, ok54 := projection.ViewForModel("gpt-5.4")

	require.True(t, ok55)
	require.True(t, ok54)
	require.Empty(t, view55.ExhaustedBaseIDs)
	require.ElementsMatch(t, []int64{61}, view55.ReserveOverflowIDs)
	require.ElementsMatch(t, []int64{62}, view54.ExhaustedBaseIDs)
	require.Empty(t, view54.ReserveOverflowIDs)
	_, reserveOK := projection.AccountReserveIDs[61]
	require.False(t, reserveOK)
}

func TestBuildOpenAIModelSubsetProjection_ViewForModelDistinguishesMissingViewFromEmptyView(t *testing.T) {
	t.Parallel()

	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.6"},
	})

	view, ok := projection.ViewForModel("gpt-5.6")
	require.True(t, ok)
	require.Equal(t, "gpt-5.6", view.CanonicalModel)
	require.Empty(t, view.ExhaustedBaseIDs)
	require.Empty(t, view.ReserveOverflowIDs)

	_, ok = projection.ViewForModel("gpt-5.4")
	require.False(t, ok)

	var nilProjection *OpenAIModelSubsetProjection
	_, ok = nilProjection.ViewForModel("gpt-5.6")
	require.False(t, ok)
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

			view, ok := projection.ViewForModel("gpt-5.6")
			require.True(t, ok)
			require.ElementsMatch(t, tt.wantExhausted, view.ExhaustedBaseIDs)
			require.ElementsMatch(t, tt.wantReserve, view.ReserveOverflowIDs)
			for _, exhaustedID := range view.ExhaustedBaseIDs {
				require.NotContains(t, view.ReserveOverflowIDs, exhaustedID)
			}
		})
	}
}
