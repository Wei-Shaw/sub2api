package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_Hit(t *testing.T) {
	ctx := context.Background()
	groupID := int64(23)
	account := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_1", account.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_1", "gpt-5.1", TargetGroupAny, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestPreviousResponseReserveBindingStillMatchesExhaustedClass(t *testing.T) {
	ctx := context.Background()
	groupID := int64(28)
	responseID := "resp_prev_reserve_exhausted"
	reserveAccount := Account{
		ID:          34,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"plan_type": "free",
		},
		Extra: map[string]any{
			"codex_7d_used_percent":                        20.0,
			"openai_oauth_responses_websockets_v2_enabled": true,
		},
	}
	cache := newOpenAIAffinityGatewayCacheStub()
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()

	writerSvc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{reserveAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}
	readerSvc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{reserveAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: NewOpenAIWSStateStore(cache),
	}
	binding := &openAIAffinityBinding{
		BoundAccountID: reserveAccount.ID,
		AffinityDomain: string(TargetGroupExhausted),
		SelectedGroup:  openAISelectedGroupReserve,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, reserveAccount.ID, time.Hour))
	cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID, binding, time.Hour)

	selection, selectedBinding, err := readerSvc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, reserveAccount.ID, selection.Account.ID)
	require.NotNil(t, selectedBinding)
	require.Equal(t, reserveAccount.ID, selectedBinding.BoundAccountID)
	require.Equal(t, openAISelectedGroupReserve, selectedBinding.AffinityDomain)
	require.Equal(t, openAISelectedGroupReserve, selectedBinding.SelectedGroup)
	storedBinding := cache.mustGetAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID)
	require.Equal(t, openAISelectedGroupReserve, storedBinding.SelectedGroup)
	selection, selectedBinding, err = writerSvc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selectedBinding)
	require.Equal(t, openAISelectedGroupReserve, selectedBinding.SelectedGroup)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	// 被动耗尽后仍应命中，但当前实际命中组应回落为 exhausted，而不是继续保留 reserve 标签。
	reserveAccount.Extra["codex_7d_used_percent"] = 100.0
	selection, selectedBinding, err = readerSvc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, reserveAccount.ID, selection.Account.ID)
	require.NotNil(t, selectedBinding)
	require.Equal(t, string(TargetGroupExhausted), selectedBinding.AffinityDomain)
	require.Equal(t, string(TargetGroupExhausted), selection.SelectedGroup)
}

func TestPreviousResponseReserveBindingStillMatchesExhaustedClass_OnProjectionMiss(t *testing.T) {
	ctx := context.Background()
	groupID := int64(2801)
	responseID := "resp_prev_reserve_exhausted_projection_miss"
	reserveAccount := Account{
		ID:          341,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"plan_type": "free"},
		Extra: map[string]any{
			"codex_7d_used_percent":                        20.0,
			"openai_oauth_responses_websockets_v2_enabled": true,
		},
	}
	snapshotCache := &openAISnapshotCacheStub{accountsByID: map[int64]*Account{reserveAccount.ID: &reserveAccount}, openAIStateMiss: true}
	cache := newOpenAIAffinityGatewayCacheStub()
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{reserveAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}
	binding := &openAIAffinityBinding{BoundAccountID: reserveAccount.ID, AffinityDomain: string(TargetGroupExhausted), SelectedGroup: openAISelectedGroupReserve}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, reserveAccount.ID, time.Hour))
	cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID, binding, time.Hour)

	selection, selectedBinding, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, reserveAccount.ID, selection.Account.ID)
	require.NotNil(t, selectedBinding)
	require.Equal(t, openAISelectedGroupReserve, selectedBinding.SelectedGroup)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestPreviousResponseReserveBindingReadsSameProjectionKeyAsSysModel(t *testing.T) {
	ctx := context.Background()
	groupID := int64(280)
	responseID := "resp_prev_reserve_projection_sys"
	reserveAccount := Account{
		ID:          340,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"plan_type":     "free",
			"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"},
		},
		Extra: map[string]any{
			"codex_7d_used_percent":                        20.0,
			"openai_oauth_responses_websockets_v2_enabled": true,
		},
	}
	snapshotCache := &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{reserveAccount.ID: &reserveAccount},
		openAIState: newOpenAIBucketStateForTest([]Account{reserveAccount}, 7, map[string]OpenAIModelRoleView{
			"gpt-5.4": {
				CanonicalModel:     "gpt-5.4",
				ReserveOverflowIDs: []int64{reserveAccount.ID},
			},
		}),
	}
	cache := newOpenAIAffinityGatewayCacheStub()
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{reserveAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}
	binding := &openAIAffinityBinding{
		BoundAccountID:     reserveAccount.ID,
		AffinityDomain:     string(TargetGroupExhausted),
		SelectedGroup:      openAISelectedGroupReserve,
		ProjectionVersion:  7,
		ProjectionModelKey: "gpt-5.4",
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, reserveAccount.ID, time.Hour))
	cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID, binding, time.Hour)

	selection, selectedBinding, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.4-Sys", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, reserveAccount.ID, selection.Account.ID)
	require.NotNil(t, selectedBinding)
	require.Equal(t, openAISelectedGroupReserve, selectedBinding.SelectedGroup)
	require.Equal(t, int64(7), selectedBinding.ProjectionVersion)
	require.Equal(t, "gpt-5.4", selectedBinding.ProjectionModelKey)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_ResponsesImageGenerationUsesImageProjection(t *testing.T) {
	ctx := context.Background()
	groupID := int64(2802)
	responseID := "resp_prev_image_projection_reserve"
	exhaustedAccount := newOpenAIProjectionExhaustedAccount(3421, 1, []string{"gpt-5.5"})
	freeReserve := newOpenAIProjectionActiveAccount(3422, 1, 20, []string{"gpt-5.5"})
	paidImageReserve := newOpenAIProjectionPaidTierAccount(3423, 1, "team", []string{"gpt-5.5", "gpt-image-2"})
	paidImageReserve.Extra["openai_oauth_responses_websockets_v2_enabled"] = true
	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: groupID, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.5"},
		AccountsAll:      []Account{exhaustedAccount, freeReserve, paidImageReserve},
	})
	cache := newOpenAIAffinityGatewayCacheStub()
	store := NewOpenAIWSStateStore(cache)
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedAccount, freeReserve, paidImageReserve}},
		cache:              cache,
		cfg:                newOpenAIWSV2TestConfig(),
		schedulerSnapshot:  &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{openAIState: newOpenAIBucketStateForTest([]Account{exhaustedAccount, freeReserve, paidImageReserve}, 71, projection.Models, projection)}},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}
	projectionBuiltAt := time.Unix(1_716_000_000, 0).UTC()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, paidImageReserve.ID, time.Hour))
	cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID, &openAIAffinityBinding{
		BoundAccountID:     paidImageReserve.ID,
		AffinityDomain:     openAISelectedGroupReserve,
		SelectedGroup:      openAISelectedGroupReserve,
		ProjectionVersion:  71,
		ProjectionModelKey: "gpt-5.5",
		ProjectionBuiltAt:  &projectionBuiltAt,
	}, time.Hour)

	selection, binding, err := svc.SelectAccountByPreviousResponseIDWithResponsesImageGeneration(ctx, &groupID, responseID, "gpt-5.5", TargetGroupExhausted, nil, false, &OpenAIResponsesImageGenerationRequirement{Enabled: true, MainModel: "gpt-5.5", ImageModel: "gpt-image-2"})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, paidImageReserve.ID, selection.Account.ID)
	require.Equal(t, openAISelectedGroupReserve, selection.SelectedGroup)
	require.NotNil(t, binding)
	require.Equal(t, openAISelectedGroupReserve, binding.SelectedGroup)
	require.Equal(t, openAISelectedGroupReserve, binding.AffinityDomain)
	require.Equal(t, int64(71), binding.ProjectionVersion)
	require.Equal(t, "gpt-5.5", binding.ProjectionModelKey)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestWSContinuationReserveBindingAcceptedForActiveAny(t *testing.T) {
	ctx := context.Background()
	exhaustedBase := newOpenAIExhaustedAccountForTest(43001, 4)
	reserveAccount := newOpenAIReserveCandidateAccountForTest(43002, 4, 20)
	activeAccount := Account{ID: 43003, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 4}
	accounts := []Account{exhaustedBase, reserveAccount, activeAccount}

	for _, tc := range []struct {
		name        string
		groupID     int64
		targetGroup AccountTargetGroup
	}{
		{name: "any", groupID: 43010, targetGroup: TargetGroupAny},
		{name: "active", groupID: 43011, targetGroup: TargetGroupActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responseID := "resp_ws_continuation_reserve_" + tc.name
			cache := newOpenAIAffinityGatewayCacheStub()
			svc := newOpenAIProjectedReserveBindingServiceForTest(cache, accounts, 25, map[int64]*AccountLoadInfo{
				exhaustedBase.ID:  {AccountID: exhaustedBase.ID, CurrentConcurrency: 3, LoadRate: 90},
				reserveAccount.ID: {AccountID: reserveAccount.ID, CurrentConcurrency: 0, LoadRate: 0},
				activeAccount.ID:  {AccountID: activeAccount.ID, CurrentConcurrency: 0, LoadRate: 80},
			})
			store := svc.getOpenAIWSStateStore()
			require.NoError(t, store.BindResponseAccount(ctx, tc.groupID, responseID, reserveAccount.ID, time.Hour))

			selection, binding, err := svc.SelectAccountByPreviousResponseID(ctx, &tc.groupID, responseID, "gpt-5.1", tc.targetGroup, nil, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			require.Equal(t, reserveAccount.ID, selection.Account.ID)
			require.Equal(t, openAISelectedGroupReserve, selection.SelectedGroup)
			require.NotNil(t, binding)
			require.Equal(t, reserveAccount.ID, binding.BoundAccountID)
			require.Equal(t, openAISelectedGroupReserve, binding.SelectedGroup)
			require.Equal(t, openAISelectedGroupReserve, binding.AffinityDomain)
			require.Equal(t, int64(25), binding.ProjectionVersion)
			require.Equal(t, "gpt-5.1", binding.ProjectionModelKey)
			storedBinding := cache.mustGetAffinityBinding(t, openAIResponseAffinityBindingNamespace, tc.groupID, responseID)
			require.Equal(t, openAISelectedGroupReserve, storedBinding.AffinityDomain)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_ReserveRecheckRejectsStaleExhaustedForActiveAny(t *testing.T) {
	ctx := context.Background()
	exhaustedBase := newOpenAIExhaustedAccountForTest(43013, 4)
	projectedReserve := newOpenAIReserveCandidateAccountForTest(43014, 4, 20)
	currentReserve := projectedReserve
	currentReserve.Extra = cloneJSONObject(projectedReserve.Extra)
	currentReserve.Extra["codex_7d_used_percent"] = 100.0

	for _, tc := range []struct {
		name        string
		groupID     int64
		targetGroup AccountTargetGroup
	}{
		{name: "any", groupID: 43015, targetGroup: TargetGroupAny},
		{name: "active", groupID: 43016, targetGroup: TargetGroupActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responseID := "resp_ws_continuation_stale_reserve_" + tc.name
			cache := newOpenAIAffinityGatewayCacheStub()
			svc := newOpenAIProjectedReserveRecheckServiceForTest(cache, []Account{exhaustedBase, projectedReserve}, []Account{exhaustedBase, currentReserve}, 33, map[int64]*AccountLoadInfo{
				exhaustedBase.ID:    {AccountID: exhaustedBase.ID, CurrentConcurrency: 3, LoadRate: 90},
				projectedReserve.ID: {AccountID: projectedReserve.ID, CurrentConcurrency: 0, LoadRate: 0},
			})
			store := svc.getOpenAIWSStateStore()
			require.NoError(t, store.BindResponseAccount(ctx, tc.groupID, responseID, projectedReserve.ID, time.Hour))
			cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, tc.groupID, responseID, &openAIAffinityBinding{
				BoundAccountID:     projectedReserve.ID,
				AffinityDomain:     openAISelectedGroupReserve,
				SelectedGroup:      openAISelectedGroupReserve,
				ProjectionVersion:  33,
				ProjectionModelKey: "gpt-5.1",
			}, time.Hour)

			selection, binding, err := svc.SelectAccountByPreviousResponseID(ctx, &tc.groupID, responseID, "gpt-5.1", tc.targetGroup, nil, false)
			require.NoError(t, err)
			require.Nil(t, selection)
			require.Nil(t, binding)
			boundAccountID, getErr := store.GetResponseAccount(ctx, tc.groupID, responseID)
			require.NoError(t, getErr)
			require.Zero(t, boundAccountID)
		})
	}
}

func TestWSContinuationReserveBindingAcceptedForExhaustedWritesReserveDomain(t *testing.T) {
	ctx := context.Background()
	groupID := int64(43012)
	responseID := "resp_ws_continuation_reserve_exhausted"
	exhaustedBase := newOpenAIExhaustedAccountForTest(43101, 4)
	reserveAccount := newOpenAIReserveCandidateAccountForTest(43102, 4, 20)
	activeAccount := Account{ID: 43103, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 4}
	accounts := []Account{exhaustedBase, reserveAccount, activeAccount}
	cache := newOpenAIAffinityGatewayCacheStub()
	svc := newOpenAIProjectedReserveBindingServiceForTest(cache, accounts, 26, map[int64]*AccountLoadInfo{
		exhaustedBase.ID:  {AccountID: exhaustedBase.ID, CurrentConcurrency: 3, LoadRate: 90},
		reserveAccount.ID: {AccountID: reserveAccount.ID, CurrentConcurrency: 0, LoadRate: 0},
		activeAccount.ID:  {AccountID: activeAccount.ID, CurrentConcurrency: 0, LoadRate: 80},
	})
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, reserveAccount.ID, time.Hour))

	selection, binding, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, reserveAccount.ID, selection.Account.ID)
	require.Equal(t, openAISelectedGroupReserve, selection.SelectedGroup)
	require.NotNil(t, binding)
	require.Equal(t, openAISelectedGroupReserve, binding.SelectedGroup)
	require.Equal(t, openAISelectedGroupReserve, binding.AffinityDomain)
	require.Equal(t, int64(26), binding.ProjectionVersion)
	storedBinding := cache.mustGetAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID)
	require.Equal(t, openAISelectedGroupReserve, storedBinding.AffinityDomain)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestStickyReserveBinding_RebindsOnProjectionVersionChange(t *testing.T) {
	ctx := context.Background()
	groupID := int64(281)
	sessionHash := "session_hash_projection_rebind"
	cache := newOpenAIAffinityGatewayCacheStub()
	svc := &OpenAIGatewayService{cache: cache}
	cache.setAffinityBinding(t, openAIStickyAffinityBindingNamespace, groupID, "openai:"+sessionHash, &openAIAffinityBinding{
		BoundAccountID:     340,
		AffinityDomain:     string(TargetGroupExhausted),
		SelectedGroup:      openAISelectedGroupReserve,
		ProjectionVersion:  7,
		ProjectionModelKey: "gpt-5.4",
		ProjectionBuiltAt:  ptrTime(time.Unix(1_716_000_000, 0).UTC()),
	}, time.Hour)

	require.NoError(t, svc.BindOpenAIStickySession(ctx, &groupID, sessionHash, 340, openAISelectedGroupReserve))

	binding := cache.mustGetAffinityBinding(t, openAIStickyAffinityBindingNamespace, groupID, "openai:"+sessionHash)
	require.Equal(t, int64(340), binding.BoundAccountID)
	require.Equal(t, openAISelectedGroupReserve, binding.AffinityDomain)
	require.Equal(t, openAISelectedGroupReserve, binding.SelectedGroup)
	require.Zero(t, binding.ProjectionVersion)
	require.Empty(t, binding.ProjectionModelKey)
	require.Nil(t, binding.ProjectionBuiltAt)
}

func TestPreviousResponseReserveBinding_InvalidatesWhenProjectionVersionChanges(t *testing.T) {
	ctx := context.Background()
	groupID := int64(282)
	responseID := "resp_prev_projection_version_invalidated"
	account := Account{
		ID:          341,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"plan_type":     "free",
			"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"},
		},
		Extra: map[string]any{
			"codex_7d_used_percent":                        20.0,
			"openai_oauth_responses_websockets_v2_enabled": true,
		},
	}
	snapshotCache := &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{account.ID: &account},
		openAIState: newOpenAIBucketStateForTest([]Account{account}, 8, map[string]OpenAIModelRoleView{
			"gpt-5.4": {
				CanonicalModel:     "gpt-5.4",
				ReserveOverflowIDs: []int64{account.ID},
			},
		}),
	}
	cache := newOpenAIAffinityGatewayCacheStub()
	store := NewOpenAIWSStateStore(cache)
	svc := &OpenAIGatewayService{
		cache:              cache,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		openaiWSStateStore: store,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, account.ID, time.Hour))
	cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID, &openAIAffinityBinding{
		BoundAccountID:     account.ID,
		AffinityDomain:     string(TargetGroupExhausted),
		SelectedGroup:      openAISelectedGroupReserve,
		ProjectionVersion:  7,
		ProjectionModelKey: "gpt-5.4",
	}, time.Hour)

	require.False(t, svc.isOpenAIReservePreviousResponseAnchor(ctx, &groupID, "gpt-5.4", responseID))
}

func TestResponseAffinityBindingSyncsTTLAndDelete(t *testing.T) {
	ctx := context.Background()
	groupID := int64(29)
	responseID := "resp_prev_affinity_ttl_delete"
	account := Account{
		ID:          35,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"plan_type": "free"},
		Extra: map[string]any{
			"codex_7d_used_percent":                        20.0,
			"openai_oauth_responses_websockets_v2_enabled": true,
		},
	}
	cache := newOpenAIAffinityGatewayCacheStub()
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}
	binding := &openAIAffinityBinding{
		BoundAccountID: account.ID,
		AffinityDomain: string(TargetGroupExhausted),
		SelectedGroup:  openAISelectedGroupReserve,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, account.ID, time.Minute))
	cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID, binding, time.Minute)
	oldResponseKey := openAIWSResponseAccountCacheKey(responseID)
	oldSessionExpiry := cache.sessionExpiry(oldResponseKey)
	oldAffinityExpiry := cache.affinityExpiry(openAIResponseAffinityBindingNamespace, groupID, responseID)

	selection, selectedBinding, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selectedBinding)
	require.True(t, cache.sessionExpiry(oldResponseKey).After(oldSessionExpiry))
	require.True(t, cache.affinityExpiry(openAIResponseAffinityBindingNamespace, groupID, responseID).After(oldAffinityExpiry))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	disabledAccount := account
	disabledAccount.Status = StatusDisabled
	disabledSvc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{disabledAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: NewOpenAIWSStateStore(cache),
	}
	selection, _, err = disabledSvc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.Nil(t, selection)
	freshStore := NewOpenAIWSStateStore(cache)
	boundAccountID, getErr := freshStore.GetResponseAccount(ctx, groupID, responseID)
	require.NoError(t, getErr)
	require.Zero(t, boundAccountID)
	require.False(t, cache.hasAffinityBinding(openAIResponseAffinityBindingNamespace, groupID, responseID))
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_RateLimitedMiss(t *testing.T) {
	ctx := context.Background()
	groupID := int64(23)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	account := Account{
		ID:               12,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		RateLimitResetAt: &rateLimitedUntil,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_rl", account.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_rl", "gpt-5.1", TargetGroupAny, nil, false)
	require.NoError(t, err)
	require.Nil(t, selection, "限额中的账号不应继续命中 previous_response_id 粘连")
	boundAccountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_rl")
	require.NoError(t, getErr)
	require.Zero(t, boundAccountID)
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_DBRuntimeRecheckRateLimitedMiss(t *testing.T) {
	ctx := context.Background()
	groupID := int64(24)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	staleAccount := &Account{
		ID:          13,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	dbAccount := Account{
		ID:               13,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		RateLimitResetAt: &rateLimitedUntil,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	snapshotCache := &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{dbAccount.ID: staleAccount},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{dbAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_db_rl", dbAccount.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_db_rl", "gpt-5.1", TargetGroupAny, nil, false)
	require.NoError(t, err)
	require.Nil(t, selection, "DB 中已限流的账号不应继续命中 previous_response_id 粘连")
	boundAccountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_db_rl")
	require.NoError(t, getErr)
	require.Zero(t, boundAccountID)
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_Excluded(t *testing.T) {
	ctx := context.Background()
	groupID := int64(23)
	account := Account{
		ID:          8,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_2", account.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_2", "gpt-5.1", TargetGroupAny, map[int64]struct{}{account.ID: {}}, false)
	require.NoError(t, err)
	require.Nil(t, selection)
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_ForceHTTPIgnored(t *testing.T) {
	ctx := context.Background()
	groupID := int64(23)
	account := Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_ws_force_http":            true,
			"responses_websockets_v2_enabled": true,
		},
	}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_force_http", account.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_force_http", "gpt-5.1", TargetGroupAny, nil, false)
	require.NoError(t, err)
	require.Nil(t, selection, "force_http 场景应忽略 previous_response_id 粘连")
	boundAccountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_force_http")
	require.NoError(t, getErr)
	require.Equal(t, account.ID, boundAccountID, "非 WSV2 兼容传输时应保留 previous_response_id 绑定")
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_BusyKeepsSticky(t *testing.T) {
	ctx := context.Background()
	groupID := int64(23)
	accounts := []Account{
		{
			ID:          21,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
			},
		},
		{
			ID:          22,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    9,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
			},
		},
	}

	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 2
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = 30 * time.Second

	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{
			21: false, // previous_response 命中的账号繁忙
			22: true,  // 次优账号可用（若回退会命中）
		},
		waitCounts: map[int64]int{
			21: 999,
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		openaiWSStateStore: store,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_busy", 21, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_busy", "gpt-5.1", TargetGroupAny, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(21), selection.Account.ID, "busy previous_response sticky account should remain selected")
	require.False(t, selection.Acquired)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, int64(21), selection.WaitPlan.AccountID)
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_TargetGroupMismatchKeepsBinding(t *testing.T) {
	ctx := context.Background()
	groupID := int64(25)
	account := Account{
		ID:          31,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"quota_limit": float64(100),
			"quota_used":  float64(10),
		},
	}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                newOpenAIWSV2TestConfig(),
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_target_group_mismatch", account.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_target_group_mismatch", "gpt-5.1", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.Nil(t, selection)
	boundAccountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_target_group_mismatch")
	require.NoError(t, getErr)
	require.Equal(t, account.ID, boundAccountID)
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_UnschedulableClearsBinding(t *testing.T) {
	ctx := context.Background()
	groupID := int64(26)
	account := Account{
		ID:          32,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusDisabled,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                newOpenAIWSV2TestConfig(),
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_unsched", account.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_unsched", "gpt-5.1", TargetGroupAny, nil, false)
	require.NoError(t, err)
	require.Nil(t, selection)
	boundAccountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_unsched")
	require.NoError(t, getErr)
	require.Zero(t, boundAccountID)
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_ExhaustedRateLimitedHitKeepsBinding(t *testing.T) {
	ctx := context.Background()
	groupID := int64(27)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	account := Account{
		ID:               33,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		RateLimitResetAt: &rateLimitedUntil,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"quota_limit": float64(100),
			"quota_used":  float64(100),
		},
	}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                newOpenAIWSV2TestConfig(),
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_exhausted_rl", account.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_exhausted_rl", "gpt-5.1", TargetGroupExhausted, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	boundAccountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_exhausted_rl")
	require.NoError(t, getErr)
	require.Equal(t, account.ID, boundAccountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_ReserveProjectionMissFailsClosedForAny(t *testing.T) {
	ctx := context.Background()
	groupID := int64(28)
	exhaustedBase := Account{ID: 34, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Extra: map[string]any{"quota_limit": float64(100), "quota_used": float64(100), "openai_apikey_responses_websockets_v2_enabled": true}}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(35, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(36, 1, 20)
	activeAccount := Account{ID: 37, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&exhaustedBase, &overlayReserve, &activeReserve, &activeAccount}, accountsByID: map[int64]*Account{34: &exhaustedBase, 35: &overlayReserve, 36: &activeReserve, 37: &activeAccount}, openAIStateMiss: true}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}}, cache: cache, cfg: cfg, concurrencyService: NewConcurrencyService(stubConcurrencyCache{}), openaiWSStateStore: store, schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}}

	overlayFromSnapshot, getAccountErr := svc.getSchedulableAccount(ctx, overlayReserve.ID)
	require.NoError(t, getAccountErr)
	require.NotNil(t, overlayFromSnapshot)
	require.False(t, svc.isCurrentOpenAIReserveOverlayAccount(ctx, &groupID, "gpt-5.1", overlayFromSnapshot))
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_projection_miss_any_reserve", overlayReserve.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_projection_miss_any_reserve", "gpt-5.1", TargetGroupAny, nil, false)
	require.NoError(t, err)
	require.Nil(t, selection)
	boundAccountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_projection_miss_any_reserve")
	require.NoError(t, getErr)
	require.Zero(t, boundAccountID)
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_NonOverlayReserveAcceptedForAnyOnProjectionMiss(t *testing.T) {
	ctx := context.Background()
	groupID := int64(29)
	exhaustedBase := Account{ID: 38, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Extra: map[string]any{"quota_limit": float64(100), "quota_used": float64(100), "openai_apikey_responses_websockets_v2_enabled": true}}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(39, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(40, 1, 20)
	activeAccount := Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	cfg := newOpenAIWSV2TestConfig()
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&exhaustedBase, &overlayReserve, &activeReserve, &activeAccount}, accountsByID: map[int64]*Account{38: &exhaustedBase, 39: &overlayReserve, 40: &activeReserve, 41: &activeAccount}, openAIStateMiss: true}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}}, cache: cache, cfg: cfg, concurrencyService: NewConcurrencyService(stubConcurrencyCache{}), openaiWSStateStore: store, schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_projection_miss_any_non_overlay", activeReserve.ID, time.Hour))

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_projection_miss_any_non_overlay", "gpt-5.1", TargetGroupAny, nil, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeReserve.ID, selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_ReserveAffinityProjectionMissFailsClosedForActiveAny(t *testing.T) {
	ctx := context.Background()
	exhaustedBase := Account{ID: 43030, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Extra: map[string]any{"quota_limit": float64(100), "quota_used": float64(100), "openai_apikey_responses_websockets_v2_enabled": true}}
	preferredLiveReserve := newOpenAIReserveCandidateAccountForTest(43031, 1, 80)
	boundReserve := newOpenAIReserveCandidateAccountForTest(43032, 1, 20)
	activeAccount := Account{ID: 43033, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}

	for _, tc := range []struct {
		name        string
		groupID     int64
		targetGroup AccountTargetGroup
	}{
		{name: "any", groupID: 43034, targetGroup: TargetGroupAny},
		{name: "active", groupID: 43035, targetGroup: TargetGroupActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responseID := "resp_prev_projection_miss_reserve_affinity_" + tc.name
			cache := newOpenAIAffinityGatewayCacheStub()
			store := NewOpenAIWSStateStore(cache)
			cfg := newOpenAIWSV2TestConfig()
			snapshotCache := &openAISnapshotCacheStub{
				snapshotAccounts: []*Account{&exhaustedBase, &preferredLiveReserve, &boundReserve, &activeAccount},
				accountsByID: map[int64]*Account{
					exhaustedBase.ID:        &exhaustedBase,
					preferredLiveReserve.ID: &preferredLiveReserve,
					boundReserve.ID:         &boundReserve,
					activeAccount.ID:        &activeAccount,
				},
				openAIStateMiss: true,
			}
			svc := &OpenAIGatewayService{
				accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, preferredLiveReserve, boundReserve, activeAccount}},
				cache:              cache,
				cfg:                cfg,
				schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
				concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
				openaiWSStateStore: store,
			}
			require.NoError(t, store.BindResponseAccount(ctx, tc.groupID, responseID, boundReserve.ID, time.Hour))
			cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, tc.groupID, responseID, &openAIAffinityBinding{
				BoundAccountID: boundReserve.ID,
				AffinityDomain: openAISelectedGroupReserve,
				SelectedGroup:  openAISelectedGroupReserve,
			}, time.Hour)

			selection, binding, err := svc.SelectAccountByPreviousResponseID(ctx, &tc.groupID, responseID, "gpt-5.1", tc.targetGroup, nil, false)
			require.NoError(t, err)
			require.Nil(t, selection)
			require.Nil(t, binding)
			boundAccountID, getErr := store.GetResponseAccount(ctx, tc.groupID, responseID)
			require.NoError(t, getErr)
			require.Zero(t, boundAccountID)
		})
	}
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_UnknownModelProjectionMissFailsClosed(t *testing.T) {
	ctx := context.Background()
	reserveAccount := newOpenAIReserveCandidateAccountForTest(42, 1, 20)

	for _, tc := range []struct {
		name        string
		groupID     int64
		targetGroup AccountTargetGroup
	}{
		{name: "any", groupID: 30, targetGroup: TargetGroupAny},
		{name: "active", groupID: 31, targetGroup: TargetGroupActive},
		{name: "exhausted", groupID: 32, targetGroup: TargetGroupExhausted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responseID := "resp_prev_projection_miss_unknown_reserve_" + tc.name
			cache := newOpenAIAffinityGatewayCacheStub()
			store := NewOpenAIWSStateStore(cache)
			cfg := newOpenAIWSV2TestConfig()
			snapshotCache := &openAISnapshotCacheStub{accountsByID: map[int64]*Account{reserveAccount.ID: &reserveAccount}, openAIStateMiss: true}
			svc := &OpenAIGatewayService{
				accountRepo:        stubOpenAIAccountRepo{accounts: []Account{reserveAccount}},
				cache:              cache,
				cfg:                cfg,
				schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
				concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
				openaiWSStateStore: store,
			}
			binding := &openAIAffinityBinding{BoundAccountID: reserveAccount.ID, AffinityDomain: string(TargetGroupExhausted), SelectedGroup: openAISelectedGroupReserve}

			require.NoError(t, store.BindResponseAccount(ctx, tc.groupID, responseID, reserveAccount.ID, time.Hour))
			cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, tc.groupID, responseID, binding, time.Hour)

			selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &tc.groupID, responseID, "gpt-5.unknown", tc.targetGroup, nil, false)
			require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
			require.Nil(t, selection)
		})
	}
}

func newOpenAIWSV2TestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 3600
	return cfg
}
