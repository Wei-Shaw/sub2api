package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

var compositeOwnershipTestGroupID int64 = 88001

const (
	compositeOwnershipTestOwnerID  int64 = 88001
	compositeOwnershipTestProxyID  int64 = 88008
	compositeOwnershipTestAlias          = "acme-reasoning-alias"
	compositeOwnershipTestUpstream       = "gpt-5.1"
)

type compositeOwnershipSchedulerMode struct {
	name      string
	advanced  bool
	loadBatch bool
}

var compositeOwnershipSchedulerModes = []compositeOwnershipSchedulerMode{
	{name: "legacy", advanced: false},
	{name: "legacy-batched", advanced: false, loadBatch: true},
	{name: "advanced", advanced: true},
}

func compositeOwnershipTestContext(publicModel, upstreamModel string) context.Context {
	return WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
		Matched:        true,
		Source:         CompositeRouteSourceAccount,
		GroupID:        compositeOwnershipTestGroupID,
		PublicModel:    publicModel,
		TargetPlatform: PlatformOpenAI,
		UpstreamModel:  upstreamModel,
		Endpoint:       CompositeRouteEndpointResponses,
	})
}

func compositeOwnershipTestAccounts() (Account, Account) {
	owner := Account{
		ID:          compositeOwnershipTestOwnerID,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    10,
		GroupIDs:    []int64{compositeOwnershipTestGroupID},
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				compositeOwnershipTestAlias: compositeOwnershipTestUpstream,
			},
		},
	}
	proxy := Account{
		ID:          compositeOwnershipTestProxyID,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    0,
		GroupIDs:    []int64{compositeOwnershipTestGroupID},
		Extra: map[string]any{
			"openai_passthrough": true,
		},
	}
	return owner, proxy
}

func newCompositeOwnershipTestService(accounts []Account, mode compositeOwnershipSchedulerMode) *OpenAIGatewayService {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = mode.loadBatch
	loadMap := make(map[int64]*AccountLoadInfo, len(accounts))
	for i := range accounts {
		loadMap[accounts[i].ID] = &AccountLoadInfo{AccountID: accounts[i].ID}
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{loadMap: loadMap}),
	}
	if mode.advanced {
		svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	}
	return svc
}

func releaseCompositeOwnershipSelection(t *testing.T, selection *AccountSelectionResult) {
	t.Helper()
	if selection != nil && selection.ReleaseFunc != nil {
		t.Cleanup(selection.ReleaseFunc)
	}
}

func TestCompositeAccountModelOwnership_ExactMappedAccountBeatsHigherPriorityPassthrough(t *testing.T) {
	for _, mode := range compositeOwnershipSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			owner, proxy := compositeOwnershipTestAccounts()
			svc := newCompositeOwnershipTestService([]Account{owner, proxy}, mode)
			ctx := compositeOwnershipTestContext(compositeOwnershipTestAlias, compositeOwnershipTestAlias)

			selection, _, err := svc.SelectAccountWithScheduler(
				ctx,
				&compositeOwnershipTestGroupID,
				"",
				"",
				compositeOwnershipTestAlias,
				nil,
				OpenAIUpstreamTransportAny,
				false,
			)
			releaseCompositeOwnershipSelection(t, selection)

			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			require.Equal(t, owner.ID, selection.Account.ID)
		})
	}
}

func TestCompositeAccountModelOwnership_OwnerUnavailableNeverFallsThroughToPassthrough(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*Account)
		excluded bool
	}{
		{
			name: "rate limited",
			mutate: func(account *Account) {
				until := time.Now().Add(time.Hour)
				account.RateLimitResetAt = &until
			},
		},
		{
			name: "unavailable",
			mutate: func(account *Account) {
				account.Schedulable = false
			},
		},
		{
			name:     "excluded",
			excluded: true,
		},
	}

	for _, mode := range compositeOwnershipSchedulerModes {
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				owner, proxy := compositeOwnershipTestAccounts()
				if tc.mutate != nil {
					tc.mutate(&owner)
				}
				var excludedIDs map[int64]struct{}
				if tc.excluded {
					excludedIDs = map[int64]struct{}{owner.ID: {}}
				}
				svc := newCompositeOwnershipTestService([]Account{owner, proxy}, mode)
				ctx := compositeOwnershipTestContext(compositeOwnershipTestAlias, compositeOwnershipTestAlias)

				selection, _, err := svc.SelectAccountWithScheduler(
					ctx,
					&compositeOwnershipTestGroupID,
					"",
					"",
					compositeOwnershipTestAlias,
					excludedIDs,
					OpenAIUpstreamTransportAny,
					false,
				)
				releaseCompositeOwnershipSelection(t, selection)

				require.ErrorIs(t, err, ErrNoAvailableAccounts)
				require.Nil(t, selection)
			})
		}
	}
}

func TestCompositeAccountModelOwnership_StaleStickyPassthroughCannotBypass(t *testing.T) {
	for _, mode := range compositeOwnershipSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			owner, proxy := compositeOwnershipTestAccounts()
			svc := newCompositeOwnershipTestService([]Account{owner, proxy}, mode)
			ctx := compositeOwnershipTestContext(compositeOwnershipTestAlias, compositeOwnershipTestAlias)
			const sessionHash = "composite-stale-sticky"
			require.NoError(t, svc.setStickySessionAccountID(ctx, &compositeOwnershipTestGroupID, sessionHash, proxy.ID, time.Hour))

			selection, _, err := svc.SelectAccountWithScheduler(
				ctx,
				&compositeOwnershipTestGroupID,
				"",
				sessionHash,
				compositeOwnershipTestAlias,
				nil,
				OpenAIUpstreamTransportAny,
				false,
			)
			releaseCompositeOwnershipSelection(t, selection)

			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			require.Equal(t, owner.ID, selection.Account.ID)
		})
	}
}

func TestCompositeAccountModelOwnership_PreviousResponseSnapshotCannotReusePassthrough(t *testing.T) {
	_, proxy := compositeOwnershipTestAccounts()
	svc := newCompositeOwnershipTestService([]Account{proxy}, compositeOwnershipSchedulerModes[0])
	snapshotProxy := proxy
	svc.schedulerSnapshot = &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{proxy.ID: &snapshotProxy},
	}}
	svc.accountRepo = nil
	ctx := compositeOwnershipTestContext(compositeOwnershipTestAlias, compositeOwnershipTestAlias)
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(
		ctx,
		compositeOwnershipTestGroupID,
		"resp_composite_snapshot",
		proxy.ID,
		time.Hour,
	))

	selection, err := svc.SelectAccountByPreviousResponseID(
		ctx,
		&compositeOwnershipTestGroupID,
		"resp_composite_snapshot",
		compositeOwnershipTestAlias,
		nil,
		false,
	)
	releaseCompositeOwnershipSelection(t, selection)

	require.NoError(t, err)
	require.Nil(t, selection)
}

func TestCompositeAccountModelOwnership_PreviousResponseFreshDBRecheckCannotReusePassthrough(t *testing.T) {
	owner, proxy := compositeOwnershipTestAccounts()
	snapshotOwner := owner
	freshProxy := proxy
	freshProxy.ID = owner.ID
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{freshProxy}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		schedulerSnapshot: &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
			accountsByID: map[int64]*Account{owner.ID: &snapshotOwner},
		}},
	}
	ctx := compositeOwnershipTestContext(compositeOwnershipTestAlias, compositeOwnershipTestAlias)
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(
		ctx,
		compositeOwnershipTestGroupID,
		"resp_composite_db_recheck",
		owner.ID,
		time.Hour,
	))

	selection, err := svc.SelectAccountByPreviousResponseID(
		ctx,
		&compositeOwnershipTestGroupID,
		"resp_composite_db_recheck",
		compositeOwnershipTestAlias,
		nil,
		false,
	)
	releaseCompositeOwnershipSelection(t, selection)

	require.NoError(t, err)
	require.Nil(t, selection)
}

func TestCompositeAccountModelOwnership_PreviousResponseFallsBackToMappedOwner(t *testing.T) {
	for _, mode := range compositeOwnershipSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			owner, proxy := compositeOwnershipTestAccounts()
			svc := newCompositeOwnershipTestService([]Account{owner, proxy}, mode)
			ctx := compositeOwnershipTestContext(compositeOwnershipTestAlias, compositeOwnershipTestAlias)
			require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(
				ctx,
				compositeOwnershipTestGroupID,
				"resp_composite_fallback",
				proxy.ID,
				time.Hour,
			))

			selection, _, err := svc.SelectAccountWithScheduler(
				ctx,
				&compositeOwnershipTestGroupID,
				"resp_composite_fallback",
				"",
				compositeOwnershipTestAlias,
				nil,
				OpenAIUpstreamTransportAny,
				false,
			)
			releaseCompositeOwnershipSelection(t, selection)

			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			require.Equal(t, owner.ID, selection.Account.ID)
		})
	}
}

func TestCompositeAccountModelOwnership_MissingPublicAliasFailsClosed(t *testing.T) {
	for _, mode := range compositeOwnershipSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			owner, proxy := compositeOwnershipTestAccounts()
			svc := newCompositeOwnershipTestService([]Account{owner, proxy}, mode)
			ctx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
				Matched:        true,
				Source:         CompositeRouteSourceAccount,
				GroupID:        compositeOwnershipTestGroupID,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  compositeOwnershipTestAlias,
				Endpoint:       CompositeRouteEndpointResponses,
			})

			selection, _, err := svc.SelectAccountWithScheduler(
				ctx,
				&compositeOwnershipTestGroupID,
				"",
				"",
				compositeOwnershipTestAlias,
				nil,
				OpenAIUpstreamTransportAny,
				false,
			)
			releaseCompositeOwnershipSelection(t, selection)

			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			require.Nil(t, selection)
		})
	}
}

func TestCompositeAccountModelOwnership_UsesPublicAliasAfterModelRewrite(t *testing.T) {
	for _, mode := range compositeOwnershipSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			owner, proxy := compositeOwnershipTestAccounts()
			owner.Extra = map[string]any{"openai_passthrough": true}
			svc := newCompositeOwnershipTestService([]Account{owner, proxy}, mode)
			ctx := compositeOwnershipTestContext(compositeOwnershipTestAlias, compositeOwnershipTestUpstream)

			selection, _, err := svc.SelectAccountWithScheduler(
				ctx,
				&compositeOwnershipTestGroupID,
				"",
				"",
				compositeOwnershipTestUpstream,
				nil,
				OpenAIUpstreamTransportAny,
				false,
			)
			releaseCompositeOwnershipSelection(t, selection)

			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			require.Equal(t, owner.ID, selection.Account.ID)
		})
	}
}

func TestCompositeAccountModelOwnership_NonAccountModelPassthroughRemainsEligible(t *testing.T) {
	contexts := []struct {
		name string
		ctx  context.Context
	}{
		{name: "noncomposite", ctx: context.Background()},
		{
			name: "detector",
			ctx: WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
				Matched:        true,
				Source:         CompositeRouteSourceDetector,
				GroupID:        compositeOwnershipTestGroupID,
				PublicModel:    compositeOwnershipTestAlias,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  compositeOwnershipTestAlias,
				Endpoint:       CompositeRouteEndpointResponses,
			}),
		},
		{
			name: "explicit",
			ctx: WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
				Matched:        true,
				Source:         CompositeRouteSourceExplicit,
				GroupID:        compositeOwnershipTestGroupID,
				PublicModel:    compositeOwnershipTestAlias,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  compositeOwnershipTestUpstream,
				Endpoint:       CompositeRouteEndpointResponses,
			}),
		},
	}

	for _, mode := range compositeOwnershipSchedulerModes {
		for _, tc := range contexts {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				_, proxy := compositeOwnershipTestAccounts()
				svc := newCompositeOwnershipTestService([]Account{proxy}, mode)
				requestedModel := compositeOwnershipTestAlias
				if tc.name == "explicit" {
					requestedModel = compositeOwnershipTestUpstream
				}

				selection, _, err := svc.SelectAccountWithScheduler(
					tc.ctx,
					&compositeOwnershipTestGroupID,
					"",
					"",
					requestedModel,
					nil,
					OpenAIUpstreamTransportAny,
					false,
				)
				releaseCompositeOwnershipSelection(t, selection)

				require.NoError(t, err)
				require.NotNil(t, selection)
				require.NotNil(t, selection.Account)
				require.Equal(t, proxy.ID, selection.Account.ID)
			})
		}
	}
}
