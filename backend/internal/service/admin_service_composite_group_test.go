//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type accountRepoStubForCompositeModelsList struct {
	accountRepoStub
	accounts []Account
}

func (s *accountRepoStubForCompositeModelsList) ListSchedulableByGroupID(_ context.Context, _ int64) ([]Account, error) {
	return s.accounts, nil
}

func TestAdminService_CreateCompositeGroupCopiesAccountsFromConcreteGroups(t *testing.T) {
	var copiedFrom []int64
	var boundGroupID int64
	var boundAccountIDs []int64
	groupRepo := &groupRepoStubForAdmin{
		createID: 99,
		getByIDByID: map[int64]*Group{
			10: {ID: 10, Platform: PlatformOpenAI},
			20: {ID: 20, Platform: PlatformGemini},
		},
		getAccountIDsByGroupIDsFn: func(groupIDs []int64) ([]int64, error) {
			copiedFrom = append([]int64{}, groupIDs...)
			return []int64{101, 202}, nil
		},
		bindAccountsToGroupFn: func(groupID int64, accountIDs []int64) error {
			boundGroupID = groupID
			boundAccountIDs = append([]int64{}, accountIDs...)
			return nil
		},
	}
	svc := &adminServiceImpl{groupRepo: groupRepo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                     "Composite",
		Platform:                 PlatformComposite,
		RateMultiplier:           1,
		CopyAccountsFromGroupIDs: []int64{10, 20, 10},
	})

	require.NoError(t, err)
	require.Equal(t, PlatformComposite, groupRepo.created.Platform)
	require.Equal(t, int64(99), group.ID)
	require.Equal(t, int64(2), group.AccountCount)
	require.ElementsMatch(t, []int64{10, 20}, copiedFrom)
	require.Equal(t, int64(99), boundGroupID)
	require.ElementsMatch(t, []int64{101, 202}, boundAccountIDs)
}

func TestAdminService_UpdateCompositeGroupCopiesAccountsFromConcreteGroups(t *testing.T) {
	var clearedGroupID int64
	var copiedFrom []int64
	var boundGroupID int64
	var boundAccountIDs []int64
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			10: {ID: 10, Platform: PlatformOpenAI},
			20: {ID: 20, Platform: PlatformGrok},
			99: {ID: 99, Platform: PlatformComposite, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard},
		},
		deleteAccountGroupsByGroupIDFn: func(groupID int64) (int64, error) {
			clearedGroupID = groupID
			return 2, nil
		},
		getAccountIDsByGroupIDsFn: func(groupIDs []int64) ([]int64, error) {
			copiedFrom = append([]int64{}, groupIDs...)
			return []int64{301, 302}, nil
		},
		bindAccountsToGroupFn: func(groupID int64, accountIDs []int64) error {
			boundGroupID = groupID
			boundAccountIDs = append([]int64{}, accountIDs...)
			return nil
		},
	}
	svc := &adminServiceImpl{groupRepo: groupRepo}

	group, err := svc.UpdateGroup(context.Background(), 99, &UpdateGroupInput{
		CopyAccountsFromGroupIDs: []int64{10, 20},
	})

	require.NoError(t, err)
	require.Equal(t, PlatformComposite, group.Platform)
	require.Equal(t, int64(99), clearedGroupID)
	require.ElementsMatch(t, []int64{10, 20}, copiedFrom)
	require.Equal(t, int64(99), boundGroupID)
	require.ElementsMatch(t, []int64{301, 302}, boundAccountIDs)
}

func TestAdminService_CreateAccountAllowsCompositeGroupAssignment(t *testing.T) {
	accountRepo := &accountRepoStubForBulkUpdate{createID: 7}
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			99: {ID: 99, Platform: PlatformComposite},
		},
	}
	svc := &adminServiceImpl{accountRepo: accountRepo, groupRepo: groupRepo}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                  "OpenAI account",
		Platform:              PlatformOpenAI,
		Type:                  AccountTypeAPIKey,
		Concurrency:           1,
		GroupIDs:              []int64{99},
		SkipDefaultGroupBind:  true,
		SkipMixedChannelCheck: true,
	})

	require.NoError(t, err)
	require.Equal(t, int64(7), account.ID)
	require.Equal(t, PlatformOpenAI, accountRepo.createAccount.Platform)
	require.ElementsMatch(t, []int64{99}, accountRepo.bindGroupsByAccount[7])
}

func TestAdminService_UpdateAccountAllowsCompositeGroupAssignment(t *testing.T) {
	accountRepo := &accountRepoStubForBulkUpdate{
		getByIDAccounts: map[int64]*Account{
			7: {ID: 7, Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Extra: map[string]any{}},
		},
	}
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			99: {ID: 99, Platform: PlatformComposite},
		},
	}
	svc := &adminServiceImpl{accountRepo: accountRepo, groupRepo: groupRepo}
	groupIDs := []int64{99}

	account, err := svc.UpdateAccount(context.Background(), 7, &UpdateAccountInput{
		GroupIDs:              &groupIDs,
		SkipMixedChannelCheck: true,
	})

	require.NoError(t, err)
	require.Equal(t, int64(7), account.ID)
	require.Len(t, accountRepo.updatedAccounts, 1)
	require.ElementsMatch(t, []int64{99}, accountRepo.bindGroupsByAccount[7])
}

func TestAdminService_CompositeModelsListCandidatesRequirePublishedCatalog(t *testing.T) {
	accountRepo := &accountRepoStubForCompositeModelsList{
		accounts: []Account{
			{
				ID:       1,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-custom": "gpt-5"},
				},
			},
			{
				ID:       2,
				Platform: PlatformGemini,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gemini-custom": "gemini-2.5-flash"},
				},
			},
		},
	}
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			99: {ID: 99, Platform: PlatformComposite},
		},
	}
	svc := &adminServiceImpl{accountRepo: accountRepo, groupRepo: groupRepo}

	candidates, err := svc.GetGroupModelsListCandidates(context.Background(), 99, PlatformComposite)

	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestAdminService_GroupModelsListCandidatesUseStrictPublishedCatalogOnly(t *testing.T) {
	now := time.Now().UTC()
	spec := CatalogSnapshotSpec{
		Scope:      CatalogScopeGlobal,
		Epoch:      13,
		RevisionID: 1300,
		Revision:   13,
		Checksum:   "admin-group-candidates",
		VerifiedAt: now,
		Models: []CatalogSnapshotModelSpec{
			{
				ID: 1, RevisionID: 101, CanonicalKey: "claude-opus-5",
				OperatorState: CatalogOperatorStateEnabled, SourceState: CatalogSourceStatePresent,
				Provider: "anthropic", Platform: "anthropic", PricingValid: true, PricingSource: "official:anthropic",
				Pricing: &LiteLLMModelPricing{InputCostPerToken: 1e-6},
			},
			{
				ID: 2, RevisionID: 102, CanonicalKey: "disabled-catalog-model",
				OperatorState: CatalogOperatorStateDisabled, SourceState: CatalogSourceStatePresent,
				Provider: "anthropic", Platform: "anthropic", PricingValid: true, PricingSource: "official:anthropic",
				Pricing: &LiteLLMModelPricing{InputCostPerToken: 1e-6},
			},
			{
				ID: 3, RevisionID: 103, CanonicalKey: "unpriced-catalog-model",
				OperatorState: CatalogOperatorStateEnabled, SourceState: CatalogSourceStatePresent,
				Provider: "anthropic", Platform: "anthropic", PricingValid: false, PricingSource: "official:anthropic",
			},
			{
				ID: 4, RevisionID: 104, CanonicalKey: "missing-source-catalog-model",
				OperatorState: CatalogOperatorStateEnabled, SourceState: CatalogSourceStateMissing,
				Provider: "anthropic", Platform: "anthropic", PricingValid: true, PricingSource: "official:anthropic",
				Pricing: &LiteLLMModelPricing{InputCostPerToken: 1e-6},
			},
			{
				ID: 5, RevisionID: 105, CanonicalKey: "manual-catalog-model",
				OperatorState: CatalogOperatorStateEnabled, SourceState: CatalogSourceStatePresent,
				Provider: "anthropic", Platform: "anthropic", PricingValid: true,
				Pricing: &LiteLLMModelPricing{InputCostPerToken: 1e-6},
			},
		},
	}
	accountRepo := &accountRepoStubForCompositeModelsList{
		accounts: []Account{{
			ID:       1,
			Platform: PlatformAnthropic,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"customer-custom-model": "claude-opus-5"},
			},
		}},
	}
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			99: {ID: 99, Platform: PlatformAnthropic},
		},
	}
	reader := NewAtomicModelCatalogReader(mustCatalogSnapshot(t, spec), time.Hour)
	svc := &adminServiceImpl{
		accountRepo:        accountRepo,
		groupRepo:          groupRepo,
		modelCatalogReader: reader,
	}

	candidates, err := svc.GetGroupModelsListCandidates(context.Background(), 99, PlatformAnthropic)

	require.NoError(t, err)
	require.Equal(t, []string{"claude-opus-5"}, candidates)
	require.NotContains(t, candidates, "disabled-catalog-model")
	require.NotContains(t, candidates, "unpriced-catalog-model")
	require.NotContains(t, candidates, "missing-source-catalog-model")
	require.NotContains(t, candidates, "manual-catalog-model")
	require.NotContains(t, candidates, "customer-custom-model")
	require.NotContains(t, candidates, "claude-fable-5")
}

func TestCatalogModelMatchesGroupPlatformAcceptsNativePlatformsAndAliases(t *testing.T) {
	tests := []struct {
		name     string
		model    CatalogModelDecision
		platform string
		want     bool
	}{
		{
			name:     "kiro native platform",
			model:    CatalogModelDecision{Provider: PlatformKiro, Platform: PlatformKiro},
			platform: PlatformKiro,
			want:     true,
		},
		{
			name:     "antigravity native platform",
			model:    CatalogModelDecision{Provider: PlatformAntigravity, Platform: PlatformAntigravity},
			platform: PlatformAntigravity,
			want:     true,
		},
		{
			name:     "antigravity anthropic provider alias",
			model:    CatalogModelDecision{Provider: PlatformAnthropic},
			platform: PlatformAntigravity,
			want:     true,
		},
		{
			name:     "provider mismatch",
			model:    CatalogModelDecision{Provider: PlatformGemini, Platform: PlatformGemini},
			platform: PlatformOpenAI,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, catalogModelMatchesGroupPlatform(tt.model, tt.platform))
		})
	}
}
