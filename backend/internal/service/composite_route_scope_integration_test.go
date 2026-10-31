//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Keep the list projection separate from GetByID so stale membership can be
// exercised without teaching a fake scheduler the implementation under test.
type scopeIsolationAccountRepo struct {
	*groupAwareMockAccountRepo
	listedGroups  []int64
	batchCalls    [][]int64
	batchAccounts []*Account
	batchErr      error
}

func (r *scopeIsolationAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	r.batchCalls = append(r.batchCalls, append([]int64(nil), ids...))
	if r.batchErr != nil || r.batchAccounts != nil {
		return r.batchAccounts, r.batchErr
	}
	return r.groupAwareMockAccountRepo.GetByIDs(ctx, ids)
}

func (r *scopeIsolationAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, id int64, platform string) ([]Account, error) {
	r.listedGroups = append(r.listedGroups, id)
	return r.groupAwareMockAccountRepo.ListSchedulableByGroupIDAndPlatform(ctx, id, platform)
}

func scopeIsolationDecision(groupID, accountID *int64) CompositeRouteDecision {
	return CompositeRouteDecision{
		Matched: true, Source: CompositeRouteSourceExplicit, GroupID: 10,
		PublicModel: "public-model", UpstreamModel: "gpt-5.1", TargetPlatform: PlatformOpenAI,
		TargetGroupID: groupID, TargetAccountID: accountID, Route: &CompositeModelRoute{ID: 77},
	}
}

func scopeIsolationFixtures() (*scopeIsolationAccountRepo, *mockGroupRepoForGateway) {
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: 0,
			GroupIDs: []int64{10}, AccountGroups: []AccountGroup{{GroupID: 10}}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: 20,
			GroupIDs: []int64{20}, AccountGroups: []AccountGroup{{GroupID: 20}}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: 0},
	}
	return &scopeIsolationAccountRepo{groupAwareMockAccountRepo: newGroupAwareMockRepo(accounts)}, &mockGroupRepoForGateway{groups: map[int64]*Group{
		10: {ID: 10, Platform: PlatformComposite, Status: StatusActive, RateMultiplier: 1.5},
		20: {ID: 20, Platform: PlatformOpenAI, Status: StatusActive, RateMultiplier: 9},
	}}
}

type scopeIsolationSelect func(context.Context, string, string, map[int64]struct{}) (*AccountSelectionResult, error)

func scopeIsolationScheduler(t *testing.T, engine string, repo *scopeIsolationAccountRepo, groups *mockGroupRepoForGateway, cache *schedulerTestGatewayCache) scopeIsolationSelect {
	t.Helper()
	sourceID := int64(10)
	cfg := newSchedulerTestOpenAIWSV2Config()
	cfg.Gateway.Scheduling.LoadBatchEnabled = engine != "gateway-priority" && engine != "openai-legacy-priority"
	concurrency := NewConcurrencyService(schedulerTestConcurrencyCache{})
	if engine == "gateway-priority" || engine == "gateway-load" {
		svc := &GatewayService{accountRepo: repo, groupRepo: groups, cache: cache, cfg: cfg, concurrencyService: concurrency}
		return func(ctx context.Context, session, _ string, excluded map[int64]struct{}) (*AccountSelectionResult, error) {
			return svc.SelectAccountWithLoadAwareness(ctx, &sourceID, session, "public-model", excluded, "", 0)
		}
	}
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	svc := &OpenAIGatewayService{
		accountRepo: repo, cache: cache, cfg: cfg, concurrencyService: concurrency,
		schedulerSnapshot: &SchedulerSnapshotService{accountRepo: repo, groupRepo: groups},
	}
	if engine == "openai-advanced" {
		svc.rateLimitService = &RateLimitService{settingService: NewSettingService(&openAIAdvancedSchedulerSettingRepoStub{
			values: map[string]string{openAIAdvancedSchedulerSettingKey: "true"},
		}, cfg)}
		require.NotNil(t, svc.getOpenAIAccountScheduler(context.Background()), "this case must actually exercise the advanced scheduler")
	} else {
		require.Nil(t, svc.getOpenAIAccountScheduler(context.Background()), "legacy cases must not silently use the advanced scheduler")
	}
	return func(ctx context.Context, session, previous string, excluded map[int64]struct{}) (*AccountSelectionResult, error) {
		if previous != "" {
			require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, sourceID, previous, 1, time.Hour))
		}
		result, decision, err := svc.SelectAccountWithSchedulerForCapability(ctx, &sourceID, previous, session, "gpt-5.1", excluded,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true)
		if err == nil && result != nil {
			require.Equal(t, result.Account.ID, decision.SelectedAccountID)
		}
		return result, err
	}
}

func TestCompositeScopeIsolation_SchedulingBoundaries(t *testing.T) {
	for _, engine := range []string{"gateway-priority", "gateway-load", "openai-legacy-priority", "openai-legacy-load", "openai-advanced"} {
		for _, destination := range []string{"group", "pin"} {
			t.Run(engine+"/"+destination, func(t *testing.T) {
				repo, groups := scopeIsolationFixtures()
				targetID, accountID := int64(20), int64(2)
				decision := scopeIsolationDecision(&targetID, nil)
				if destination == "pin" {
					decision = scopeIsolationDecision(nil, &accountID)
				}
				ctx := WithCompositeRouteDecision(context.Background(), decision)
				session := "same-session"
				cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{CompositeRouteStickyKey(ctx, session): 1}}
				selectAccount := scopeIsolationScheduler(t, engine, repo, groups, cache)
				previous := ""
				if engine == "openai-advanced" || engine == "openai-legacy-load" || engine == "openai-legacy-priority" {
					previous = "resp_foreign_source_account"
				}
				result, err := selectAccount(ctx, session, previous, nil)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.NotNil(t, result.Account)
				defer releaseLegacySchedulerDecisionSelection(result)
				require.Equal(t, accountID, result.Account.ID, "sticky and previous-response owners cannot escape the explicit grant")
				if destination == "group" {
					require.NotEmpty(t, repo.listedGroups)
					for _, id := range repo.listedGroups {
						require.Equal(t, targetID, id)
					}
				} else {
					require.Empty(t, repo.listedGroups, "a pin must be fetched by ID rather than broadened to any pool")
				}
				result, err = selectAccount(ctx, "", "", map[int64]struct{}{accountID: {}})
				releaseLegacySchedulerDecisionSelection(result)
				require.Error(t, err, "an excluded destination must not fall back to source or ungrouped accounts")
				require.Nil(t, result)
			})
		}
	}
}

func TestCompositeScopeIsolation_MixedProviderGroupDoesNotSelectForeignOAuth(t *testing.T) {
	for _, engine := range []string{"gateway-priority", "gateway-load", "openai-legacy-priority", "openai-legacy-load", "openai-advanced"} {
		t.Run(engine, func(t *testing.T) {
			repo, groups := scopeIsolationFixtures()
			targetID := int64(20)
			decision := scopeIsolationDecision(&targetID, nil)
			foreign := repo.accountsByID[1]
			foreign.Type = AccountTypeOAuth
			foreign.GroupIDs = []int64{10, targetID}
			foreign.AccountGroups = []AccountGroup{{GroupID: 10}, {GroupID: targetID}}
			if engine == "gateway-priority" || engine == "gateway-load" {
				// OpenAI OAuth must not leak into the Anthropic forwarding pipeline,
				// even when it is in the destination group and has higher priority.
				decision.TargetPlatform = PlatformAnthropic
				decision.UpstreamModel = "claude-sonnet-4"
				repo.accountsByID[2].Platform = PlatformAnthropic
				groups.groups[targetID].Platform = PlatformAnthropic
			} else {
				foreign.Platform = PlatformAnthropic
			}
			ctx := WithCompositeRouteDecision(context.Background(), decision)
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{CompositeRouteStickyKey(ctx, "mixed-provider"): foreign.ID}}
			result, err := scopeIsolationScheduler(t, engine, repo, groups, cache)(ctx, "mixed-provider", "", nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			defer releaseLegacySchedulerDecisionSelection(result)
			require.Equal(t, int64(2), result.Account.ID)
			require.Equal(t, decision.TargetPlatform, result.Account.Platform)
			require.Equal(t, AccountTypeAPIKey, result.Account.Type)
		})
	}
}

func TestCompositeScopeIsolation_LegacyWithoutScopeKeepsSourcePool(t *testing.T) {
	for _, engine := range []string{"gateway-priority", "gateway-load", "openai-legacy-priority", "openai-legacy-load", "openai-advanced"} {
		t.Run(engine, func(t *testing.T) {
			repo, groups := scopeIsolationFixtures()
			groups.groups[10].Platform = PlatformOpenAI
			result, err := scopeIsolationScheduler(t, engine, repo, groups, &schedulerTestGatewayCache{})(context.Background(), "", "", nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			defer releaseLegacySchedulerDecisionSelection(result)
			require.Equal(t, int64(1), result.Account.ID)
			for _, groupID := range repo.listedGroups {
				require.Equal(t, int64(10), groupID, "legacy scheduling must still query the authenticated source pool")
			}
		})
	}
}

func TestCompositeScopeIsolation_UnavailableDestinationsFailClosed(t *testing.T) {
	for _, engine := range []string{"gateway-priority", "gateway-load", "openai-legacy-priority", "openai-legacy-load", "openai-advanced"} {
		for _, unavailable := range []string{"disabled-group", "deleted-group", "changed-group-platform", "disabled-pin", "deleted-pin", "cooldown-pin", "changed-pin-platform"} {
			t.Run(engine+"/"+unavailable, func(t *testing.T) {
				repo, groups := scopeIsolationFixtures()
				targetID, accountID := int64(20), int64(2)
				decision := scopeIsolationDecision(&targetID, nil)
				switch unavailable {
				case "disabled-group":
					groups.groups[targetID].Status = StatusDisabled
				case "deleted-group":
					delete(groups.groups, targetID)
				case "changed-group-platform":
					groups.groups[targetID].Platform = PlatformGemini
				default:
					decision = scopeIsolationDecision(nil, &accountID)
					switch unavailable {
					case "disabled-pin":
						repo.accountsByID[accountID].Schedulable = false
					case "deleted-pin":
						delete(repo.accountsByID, accountID)
					case "cooldown-pin":
						until := time.Now().Add(time.Hour)
						repo.accountsByID[accountID].RateLimitResetAt = &until
					case "changed-pin-platform":
						repo.accountsByID[accountID].Platform = PlatformGemini
					}
				}
				ctx := WithCompositeRouteDecision(context.Background(), decision)
				cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{CompositeRouteStickyKey(ctx, "sticky"): accountID}}
				result, err := scopeIsolationScheduler(t, engine, repo, groups, cache)(ctx, "sticky", "", nil)
				releaseLegacySchedulerDecisionSelection(result)
				require.Error(t, err)
				require.Nil(t, result)
			})
		}
	}
}

func TestCompositeScopeIsolation_FreshMembershipOverridesListProjection(t *testing.T) {
	repo, groups := scopeIsolationFixtures()
	targetID := int64(20)
	ctx := WithCompositeRouteDecision(context.Background(), scopeIsolationDecision(&targetID, nil))
	// The list still contains account 2 in group 20, but its authoritative row no longer does.
	fresh := *repo.accountsByID[2]
	fresh.AccountGroups = []AccountGroup{{GroupID: 10}}
	fresh.GroupIDs = []int64{10}
	repo.accountsByID[2] = &fresh
	accounts, handled, err := ListCompositeRouteScopedAccounts(ctx, groups, repo, PlatformOpenAI)
	require.NoError(t, err)
	require.True(t, handled)
	require.Empty(t, accounts)
	require.False(t, CompositeRouteAccountAllowed(ctx, &fresh))
	accounts, handled, err = ListCompositeRouteScopedAccounts(ctx, groups, repo, PlatformGemini)
	require.Error(t, err)
	require.True(t, handled)
	require.Empty(t, accounts)

	for _, engine := range []string{"gateway-priority", "gateway-load", "openai-legacy-priority", "openai-legacy-load", "openai-advanced"} {
		t.Run(engine, func(t *testing.T) {
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{CompositeRouteStickyKey(ctx, "stale-membership"): 2}}
			result, err := scopeIsolationScheduler(t, engine, repo, groups, cache)(ctx, "stale-membership", "", nil)
			releaseLegacySchedulerDecisionSelection(result)
			require.Error(t, err, "a stale list or sticky cache cannot resurrect revoked destination membership")
			require.Nil(t, result)
		})
	}
}

func TestCompositeScopeIsolation_BatchRefreshPreservesOrderAndChecksFreshRows(t *testing.T) {
	repo, groups := scopeIsolationFixtures()
	targetID := int64(20)
	ctx := WithCompositeRouteDecision(context.Background(), scopeIsolationDecision(&targetID, nil))
	// List order deliberately differs from both IDs and the batch response. Stale
	// candidates include a revoked membership, changed platform and deleted row.
	repo.allAccounts = nil
	for _, id := range []int64{5, 2, 3, 4, 6} {
		repo.allAccounts = append(repo.allAccounts, Account{ID: id, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, AccountGroups: []AccountGroup{{GroupID: targetID}}})
	}
	repo.batchAccounts = []*Account{
		{ID: 2, Platform: PlatformOpenAI, AccountGroups: []AccountGroup{{GroupID: targetID}}},
		nil,
		{ID: 3, Platform: PlatformOpenAI, AccountGroups: []AccountGroup{{GroupID: 10}}},
		{ID: 4, Platform: PlatformGemini, AccountGroups: []AccountGroup{{GroupID: targetID}}},
		{ID: 5, Name: "fresh row", Platform: PlatformOpenAI, AccountGroups: []AccountGroup{{GroupID: targetID}}},
		{ID: 99, Platform: PlatformOpenAI, AccountGroups: []AccountGroup{{GroupID: targetID}}},
	}
	accounts, handled, err := ListCompositeRouteScopedAccounts(ctx, groups, repo, PlatformOpenAI)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, [][]int64{{5, 2, 3, 4, 6}}, repo.batchCalls)
	require.Zero(t, repo.getByIDCalls, "group enumeration must not perform per-account lookups")
	require.Len(t, accounts, 2)
	require.Equal(t, int64(5), accounts[0].ID)
	require.Equal(t, "fresh row", accounts[0].Name)
	require.Equal(t, int64(2), accounts[1].ID)
}

func TestCompositeScopeIsolation_BatchRefreshErrorFailsClosed(t *testing.T) {
	repo, groups := scopeIsolationFixtures()
	targetID := int64(20)
	ctx := WithCompositeRouteDecision(context.Background(), scopeIsolationDecision(&targetID, nil))
	repo.batchErr = errors.New("batch refresh unavailable")
	// A partial batch result must not be used when the repository reports failure.
	repo.batchAccounts = []*Account{repo.accountsByID[2]}
	accounts, handled, err := ListCompositeRouteScopedAccounts(ctx, groups, repo, PlatformOpenAI)
	require.ErrorIs(t, err, repo.batchErr)
	require.True(t, handled, "an error must not reopen the broad pool")
	require.Nil(t, accounts)
	require.Equal(t, [][]int64{{2}}, repo.batchCalls)
	require.Zero(t, repo.getByIDCalls)

	for _, engine := range []string{"gateway-priority", "gateway-load", "openai-legacy-priority", "openai-legacy-load", "openai-advanced"} {
		t.Run(engine, func(t *testing.T) {
			result, err := scopeIsolationScheduler(t, engine, repo, groups, &schedulerTestGatewayCache{})(ctx, "", "", nil)
			releaseLegacySchedulerDecisionSelection(result)
			require.Error(t, err)
			require.Nil(t, result, "batch failure must not select source or ungrouped accounts")
		})
	}
}

func TestCompositeScopeIsolation_ContextSnapshotAndStickyNamespace(t *testing.T) {
	targetID := int64(20)
	decision := scopeIsolationDecision(&targetID, nil)
	ctx := WithCompositeRouteDecision(context.Background(), decision)
	targetID = 999
	decision.Route.ID = 999
	scope, ok := CompositeRouteScopeFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, int64(20), scope.TargetGroupID)
	require.Equal(t, int64(77), scope.RouteID)
	key := CompositeRouteStickyKey(ctx, "shared-session")
	require.Equal(t, key, CompositeRouteStickyKey(ctx, key), "nested entrypoints must not double-prefix a route binding")
	require.Empty(t, CompositeRouteStickyKey(ctx, ""))
	for _, changed := range []string{"model", "route", "source", "group", "pin", "platform"} {
		t.Run(changed, func(t *testing.T) {
			groupID := int64(20)
			other := scopeIsolationDecision(&groupID, nil)
			switch changed {
			case "model":
				other.PublicModel = "subagent-model"
			case "route":
				other.Route.ID++
			case "source":
				other.GroupID++
			case "group":
				groupID++
			case "pin":
				other.TargetGroupID = nil
				other.TargetAccountID = &groupID
			case "platform":
				other.TargetPlatform = PlatformGemini
			}
			otherKey := CompositeRouteStickyKey(WithCompositeRouteDecision(context.Background(), other), "shared-session")
			require.NotEqual(t, key, otherKey)
			bindings := map[string]int64{key: 1, otherKey: 2}
			require.Len(t, bindings, 2)
		})
	}
	require.False(t, HasCompositeRouteDestination(nil))
	require.NoError(t, ValidateCompositeRouteScope(context.Background(), nil, nil))
	accounts, handled, err := ListCompositeRouteScopedAccounts(context.Background(), nil, nil, PlatformOpenAI)
	require.NoError(t, err)
	require.False(t, handled)
	require.Nil(t, accounts)
	require.True(t, CompositeRouteAccountAllowed(context.Background(), nil), "legacy callers retain their existing gates")
	require.Equal(t, "legacy-key", CompositeRouteStickyKey(context.Background(), "legacy-key"))
}

func TestCompositeScopeIsolation_SelectedDestinationKeepsSourceBillingIdentity(t *testing.T) {
	repo, groups := scopeIsolationFixtures()
	targetID := int64(20)
	sourceID := int64(10)
	sourceGroup := groups.groups[sourceID]
	apiKey := &APIKey{ID: 501, GroupID: &sourceID, Group: sourceGroup, Quota: 100}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	c.Set("api_key", apiKey)
	SetCompositeRouteDecision(c, scopeIsolationDecision(&targetID, nil))
	require.Same(t, apiKey, c.MustGet("api_key"))
	require.Same(t, sourceGroup, apiKey.Group)
	require.Equal(t, sourceID, *apiKey.GroupID)

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	svc.accountRepo, svc.groupRepo = repo, groups
	selected, err := svc.SelectAccountForModel(c.Request.Context(), apiKey.GroupID, "", "public-model")
	require.NoError(t, err)
	require.Equal(t, int64(2), selected.ID)
	require.NoError(t, svc.RecordUsage(c.Request.Context(), &RecordUsageInput{
		Result: &ForwardResult{RequestID: "composite_source_billing_identity", Model: "claude-sonnet-4", Usage: ClaudeUsage{InputTokens: 10, OutputTokens: 6}, Duration: time.Second},
		APIKey: apiKey, User: &User{ID: 601}, Account: selected,
	}))
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.GroupID)
	require.Equal(t, sourceID, *usageRepo.lastLog.GroupID)
	require.Equal(t, selected.ID, usageRepo.lastLog.AccountID)
	require.Equal(t, sourceGroup.RateMultiplier, usageRepo.lastLog.RateMultiplier)
	require.Same(t, sourceGroup, apiKey.Group)
	require.Equal(t, sourceID, *apiKey.GroupID, fmt.Sprintf("target group %d must never become the billing identity", targetID))

	// The OpenAI usage entrypoint has its own log/billing implementation; do not
	// infer its identity behavior from Gateway.RecordUsage passing above.
	openAIUsageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	openAISvc := newOpenAIRecordUsageServiceForTest(openAIUsageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	require.NoError(t, openAISvc.RecordUsage(c.Request.Context(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "openai_composite_source_billing_identity", Model: "gpt-5.1", Usage: OpenAIUsage{InputTokens: 10, OutputTokens: 6}, Duration: time.Second},
		APIKey: apiKey, User: &User{ID: 601}, Account: selected,
	}))
	require.NotNil(t, openAIUsageRepo.lastLog)
	require.NotNil(t, openAIUsageRepo.lastLog.GroupID)
	require.Equal(t, sourceID, *openAIUsageRepo.lastLog.GroupID)
	require.Equal(t, selected.ID, openAIUsageRepo.lastLog.AccountID)
	require.Equal(t, sourceGroup.RateMultiplier, openAIUsageRepo.lastLog.RateMultiplier)
	require.Same(t, sourceGroup, apiKey.Group)
}
