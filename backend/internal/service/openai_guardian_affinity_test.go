package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type guardianAffinityGroupRepo struct {
	GroupRepository
	group *Group
	err   error
}

type guardianAffinityAccountRepo struct {
	schedulerGroupAwareOpenAIAccountRepo
	setErrorCalls int
}

func (r *guardianAffinityAccountRepo) SetError(context.Context, int64, string) error {
	r.setErrorCalls++
	return nil
}

func (r guardianAffinityGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.group, nil
}

func (r guardianAffinityGroupRepo) GetByIDLite(context.Context, int64) (*Group, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.group, nil
}

func guardianAffinityTestContext(t *testing.T, model, subagent, parentHeader, metadata string) context.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set(openAISubagentHeader, subagent)
	if parentHeader != "" {
		c.Request.Header.Set(codexParentThreadIDHeader, parentHeader)
	}
	if metadata != "" {
		c.Request.Header.Set(codexTurnMetadataHeader, metadata)
	}
	return WithOpenAIGuardianParentAffinity(context.Background(), c, nil, model)
}

func TestWithOpenAIGuardianParentAffinity_RequiresUnambiguousReviewLineage(t *testing.T) {
	parentID := "11111111-1111-4111-8111-111111111111"
	wantHash := DeriveSessionHashFromSeed(parentID)

	for _, subagent := range []string{"guardian", "review", "GUARDIAN"} {
		t.Run(subagent, func(t *testing.T) {
			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, subagent, parentID, `{"parent_thread_id":"`+parentID+`"}`)
			affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, wantHash, affinity.currentSessionHash)
		})
	}

	t.Run("metadata only", func(t *testing.T) {
		ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", "", `{"parent_thread_id":"`+parentID+`"}`)
		_, ok := openAIGuardianParentAffinityFromContext(ctx)
		require.True(t, ok)
	})

	t.Run("websocket envelope metadata", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
		body := []byte(`{"type":"response.create","response":{"model":"codex-auto-review","client_metadata":{"x-codex-turn-metadata":"{\"parent_thread_id\":\"` + parentID + `\",\"subagent_kind\":\"guardian\"}"}}}`)
		ctx := WithOpenAIGuardianParentAffinity(context.Background(), c, body, codexAutoReviewModel)
		affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, wantHash, affinity.currentSessionHash)
	})

	for name, ctx := range map[string]context.Context{
		"ordinary model":       guardianAffinityTestContext(t, "gpt-5.6-sol", "guardian", parentID, ""),
		"ordinary subagent":    guardianAffinityTestContext(t, codexAutoReviewModel, "collab_spawn", parentID, ""),
		"missing parent":       guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", "", ""),
		"conflicting lineage":  guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, `{"parent_thread_id":"different-parent"}`),
		"conflicting subagent": guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, `{"parent_thread_id":"`+parentID+`","subagent_kind":"collab_spawn"}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := openAIGuardianParentAffinityFromContext(ctx)
			require.False(t, ok)
		})
	}
}

func TestOpenAIGatewayService_GuardianParentAffinitySelectsParentAccountAcrossSchedulers(t *testing.T) {
	parentID := "22222222-2222-4222-8222-222222222222"
	parentHash := DeriveSessionHashFromSeed(parentID)
	groupID := int64(102001)

	for _, mode := range []struct {
		name           string
		advanced       string
		stickyWeighted string
	}{
		{name: "legacy", advanced: "false"},
		{name: "advanced", advanced: "true"},
		{name: "advanced sticky weighted", advanced: "true", stickyWeighted: "true"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			accounts := []Account{
				{
					ID: 39001, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10,
					GroupIDs: []int64{groupID}, Credentials: map[string]any{"plan_type": "team"},
				},
				{
					ID: 39002, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
					GroupIDs: []int64{groupID}, Credentials: map[string]any{"plan_type": "team"},
				},
			}
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.LBTopK = 2
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + parentHash: 39001}}
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
				cache:              cache,
				cfg:                cfg,
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(mode.advanced, mode.stickyWeighted),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{39001: true, 39002: true}}),
			}

			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, "")
			selection, decision, err := svc.SelectAccountWithScheduler(
				ctx, &groupID, "", "guardian-child-session", codexAutoReviewModel,
				nil, OpenAIUpstreamTransportAny, false,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(39001), selection.Account.ID)
			require.Equal(t, openAIAccountScheduleLayerGuardianParent, decision.Layer)
			require.Zero(t, cache.deletedSessions["openai:"+parentHash])
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestOpenAIGatewayService_GuardianParentAffinityFallsBackWithoutCrossGroupOrFailoverBypass(t *testing.T) {
	parentID := "33333333-3333-4333-8333-333333333333"
	parentHash := DeriveSessionHashFromSeed(parentID)
	groupID := int64(102011)
	otherGroupID := int64(102012)

	for name, excluded := range map[string]map[int64]struct{}{
		"parent moved out of group":              nil,
		"parent excluded after upstream failure": {39011: {}},
	} {
		t.Run(name, func(t *testing.T) {
			parentGroups := []int64{groupID}
			if excluded == nil {
				parentGroups = []int64{otherGroupID}
			}
			accounts := []Account{
				{ID: 39011, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, GroupIDs: parentGroups, Credentials: map[string]any{"plan_type": "team"}},
				{ID: 39012, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, GroupIDs: []int64{groupID}, Credentials: map[string]any{"plan_type": "team"}},
			}
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + parentHash: 39011}}
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
				cache:              cache,
				cfg:                &config.Config{},
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{39011: true, 39012: true}}),
			}

			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, "")
			selection, _, err := svc.SelectAccountWithScheduler(
				ctx, &groupID, "", "guardian-fallback-child", codexAutoReviewModel,
				excluded, OpenAIUpstreamTransportAny, false,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(39012), selection.Account.ID)
			require.Zero(t, cache.deletedSessions["openai:"+parentHash], "a child request must never delete its parent's binding")
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestOpenAIGatewayService_GuardianParentHashCollisionPreservesParentBinding(t *testing.T) {
	parentID := "44444444-4444-4444-8444-444444444444"
	parentHash := DeriveSessionHashFromSeed(parentID)
	groupID := int64(102021)
	otherGroupID := int64(102022)

	for _, advanced := range []string{"false", "true"} {
		t.Run(map[string]string{"false": "legacy", "true": "advanced"}[advanced], func(t *testing.T) {
			accounts := []Account{
				{ID: 39021, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, GroupIDs: []int64{otherGroupID}, Credentials: map[string]any{"plan_type": "team"}},
				{ID: 39022, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, GroupIDs: []int64{groupID}, Credentials: map[string]any{"plan_type": "team"}},
			}
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + parentHash: 39021}}
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
				cache:              cache,
				cfg:                &config.Config{},
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(advanced),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{39021: true, 39022: true}}),
			}

			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, "")
			selection, _, err := svc.SelectAccountWithScheduler(
				ctx, &groupID, "", parentHash, codexAutoReviewModel,
				nil, OpenAIUpstreamTransportAny, false,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(39022), selection.Account.ID)
			require.NoError(t, svc.BindStickySessionAfterProfitAdmission(ctx, &groupID, parentHash, selection.Account.ID))
			require.Equal(t, int64(39021), cache.sessionBindings["openai:"+parentHash])
			require.Zero(t, cache.deletedSessions["openai:"+parentHash])
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestOpenAIGatewayService_GuardianParentAffinityHonorsRequiredPrivacy(t *testing.T) {
	parentID := "55555555-5555-4555-8555-555555555555"
	parentHash := DeriveSessionHashFromSeed(parentID)
	groupID := int64(102031)

	for _, advanced := range []string{"false", "true"} {
		t.Run(map[string]string{"false": "legacy", "true": "advanced"}[advanced], func(t *testing.T) {
			accounts := []Account{
				{ID: 39031, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, GroupIDs: []int64{groupID}, Credentials: map[string]any{"plan_type": "team"}},
				{ID: 39032, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, GroupIDs: []int64{groupID}, Credentials: map[string]any{"plan_type": "team"}, Extra: map[string]any{"privacy_mode": PrivacyModeTrainingOff}},
			}
			repo := &guardianAffinityAccountRepo{schedulerGroupAwareOpenAIAccountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}}
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + parentHash: 39031}}
			svc := &OpenAIGatewayService{
				accountRepo:        repo,
				cache:              cache,
				cfg:                &config.Config{},
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(advanced),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{39031: true, 39032: true}}),
				schedulerSnapshot: &SchedulerSnapshotService{
					accountRepo: repo,
					groupRepo:   guardianAffinityGroupRepo{group: &Group{ID: groupID, Name: "privacy", RequirePrivacySet: true}},
				},
			}

			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, "")
			selection, _, err := svc.SelectAccountWithScheduler(
				ctx, &groupID, "", "guardian-privacy-child", codexAutoReviewModel,
				nil, OpenAIUpstreamTransportAny, false,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(39032), selection.Account.ID)
			require.Zero(t, repo.setErrorCalls, "a group-scoped privacy gate must not globally error a shared account")
			require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(&accounts[0], codexAutoReviewModel))
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestOpenAIGatewayService_PreviousResponseHonorsGroupAndRequiredPrivacy(t *testing.T) {
	groupID := int64(3904)

	tests := []struct {
		name         string
		boundAccount Account
		groupErr     error
	}{
		{
			name: "privacy unset",
			boundAccount: Account{
				ID: 39041, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				GroupIDs: []int64{groupID},
				Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": true},
			},
		},
		{
			name: "privacy policy lookup error fails closed",
			boundAccount: Account{
				ID: 39041, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				GroupIDs: []int64{groupID},
				Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": true},
			},
			groupErr: errors.New("group repository unavailable"),
		},
		{
			name: "different group",
			boundAccount: Account{
				ID: 39041, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				GroupIDs: []int64{groupID + 1},
				Extra: map[string]any{
					"openai_apikey_responses_websockets_v2_enabled": true,
					"privacy_mode": PrivacyModeTrainingOff,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fallback := Account{
				ID: 39042, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5,
				GroupIDs: []int64{groupID},
				Extra: map[string]any{
					"openai_apikey_responses_websockets_v2_enabled": true,
					"privacy_mode": PrivacyModeTrainingOff,
				},
			}
			accounts := []Account{tc.boundAccount, fallback}
			repo := &guardianAffinityAccountRepo{schedulerGroupAwareOpenAIAccountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}}
			cache := &schedulerTestGatewayCache{}
			store := NewOpenAIWSStateStore(cache)
			groupRepo := guardianAffinityGroupRepo{
				group: &Group{
					ID: groupID, Name: "privacy-required", Platform: PlatformOpenAI,
					Status: StatusActive, RequirePrivacySet: true,
				},
				err: tc.groupErr,
			}
			svc := &OpenAIGatewayService{
				accountRepo:        repo,
				cache:              cache,
				cfg:                &config.Config{},
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
				concurrencyService: NewConcurrencyService(&schedulerTestConcurrencyCache{}),
				openaiWSStateStore: store,
				schedulerSnapshot: &SchedulerSnapshotService{
					accountRepo: repo,
					groupRepo:   groupRepo,
				},
			}
			responseID := "resp_privacy_guard"
			require.NoError(t, store.BindResponseAccount(context.Background(), groupID, responseID, tc.boundAccount.ID, time.Hour))

			directSelection, directErr := svc.SelectAccountByPreviousResponseID(
				context.Background(), &groupID, responseID, codexAutoReviewModel, nil, false,
			)
			require.NoError(t, directErr)
			require.Nil(t, directSelection, "the previous-response helper must enforce fresh group/privacy state")

			selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
				context.Background(), &groupID, responseID, "", codexAutoReviewModel,
				nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityResponses,
				false, false, true,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, fallback.ID, selection.Account.ID)
			require.NotEqual(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
			require.Zero(t, repo.setErrorCalls)
			require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(&accounts[0], codexAutoReviewModel))
			boundAccountID, getErr := store.GetResponseAccount(context.Background(), groupID, responseID)
			require.NoError(t, getErr)
			require.Equal(t, tc.boundAccount.ID, boundAccountID, "transient policy misses must preserve the response binding")
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestOpenAIGatewayService_PreviousResponseSimpleModeIgnoresGroupMembership(t *testing.T) {
	groupID := int64(3905)
	bound := Account{
		ID: 39051, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		GroupIDs: []int64{groupID + 1},
		Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": true},
	}
	fallback := Account{
		ID: 39052, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10,
		GroupIDs: []int64{groupID},
		Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": true},
	}
	accounts := []Account{bound, fallback}
	repo := &guardianAffinityAccountRepo{schedulerGroupAwareOpenAIAccountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}}
	cache := &schedulerTestGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(&schedulerTestConcurrencyCache{}),
		openaiWSStateStore: store,
		schedulerSnapshot: &SchedulerSnapshotService{
			accountRepo: repo,
			groupRepo: guardianAffinityGroupRepo{group: &Group{
				ID: groupID, Name: "simple-mode", Platform: PlatformOpenAI, Status: StatusActive,
			}},
		},
	}
	responseID := "resp_simple_mode_cross_group"
	require.NoError(t, store.BindResponseAccount(context.Background(), groupID, responseID, bound.ID, time.Hour))

	directSelection, err := svc.SelectAccountByPreviousResponseID(
		context.Background(), &groupID, responseID, codexAutoReviewModel, nil, false,
	)
	require.NoError(t, err)
	require.NotNil(t, directSelection)
	require.Equal(t, bound.ID, directSelection.Account.ID)
	if directSelection.ReleaseFunc != nil {
		directSelection.ReleaseFunc()
	}

	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		context.Background(), &groupID, responseID, "", codexAutoReviewModel,
		nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityResponses,
		false, false, true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, bound.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
