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

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_1", "gpt-5.1", TargetGroupAny, nil)
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

	selection, selectedBinding, err := readerSvc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, reserveAccount.ID, selection.Account.ID)
	require.NotNil(t, selectedBinding)
	require.Equal(t, reserveAccount.ID, selectedBinding.BoundAccountID)
	require.Equal(t, string(TargetGroupExhausted), selectedBinding.AffinityDomain)
	require.Equal(t, openAISelectedGroupReserve, selectedBinding.SelectedGroup)
	storedBinding := cache.mustGetAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID)
	require.Equal(t, openAISelectedGroupReserve, storedBinding.SelectedGroup)
	selection, selectedBinding, err = writerSvc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selectedBinding)
	require.Equal(t, openAISelectedGroupReserve, selectedBinding.SelectedGroup)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	// 被动耗尽后仍应命中，但当前实际命中组应回落为 exhausted，而不是继续保留 reserve 标签。
	reserveAccount.Extra["codex_7d_used_percent"] = 100.0
	selection, selectedBinding, err = readerSvc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, reserveAccount.ID, selection.Account.ID)
	require.NotNil(t, selectedBinding)
	require.Equal(t, string(TargetGroupExhausted), selectedBinding.AffinityDomain)
	require.Equal(t, string(TargetGroupExhausted), selection.SelectedGroup)
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
			"plan_type":      "free",
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
		BoundAccountID:    reserveAccount.ID,
		AffinityDomain:    string(TargetGroupExhausted),
		SelectedGroup:     openAISelectedGroupReserve,
		ProjectionVersion: 7,
		ProjectionModelKey: "gpt-5.4",
	}

	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, reserveAccount.ID, time.Hour))
	cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, responseID, binding, time.Hour)

	selection, selectedBinding, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.4-Sys", TargetGroupExhausted, nil)
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

	selection, selectedBinding, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil)
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
	selection, _, err = disabledSvc.SelectAccountByPreviousResponseID(ctx, &groupID, responseID, "gpt-5.1", TargetGroupExhausted, nil)
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

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_rl", "gpt-5.1", TargetGroupAny, nil)
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

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_db_rl", "gpt-5.1", TargetGroupAny, nil)
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

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_2", "gpt-5.1", TargetGroupAny, map[int64]struct{}{account.ID: {}})
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

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_force_http", "gpt-5.1", TargetGroupAny, nil)
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

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_busy", "gpt-5.1", TargetGroupAny, nil)
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

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_target_group_mismatch", "gpt-5.1", TargetGroupExhausted, nil)
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

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_unsched", "gpt-5.1", TargetGroupAny, nil)
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

	selection, _, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_exhausted_rl", "gpt-5.1", TargetGroupExhausted, nil)
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

func newOpenAIWSV2TestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 3600
	return cfg
}
