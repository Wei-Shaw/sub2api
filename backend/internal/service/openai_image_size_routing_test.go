package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAccountOpenAIImageSizeTiers(t *testing.T) {
	cases := []struct {
		name  string
		extra map[string]any
		want  []string
	}{
		{name: "missing", extra: nil, want: nil},
		{name: "empty", extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []any{}}, want: nil},
		{name: "all tiers", extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []any{"4K", "1K", "2K"}}, want: nil},
		{name: "unknown only", extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []any{"8K", 4}}, want: nil},
		{name: "not a list", extra: map[string]any{AccountExtraOpenAIImageSizeTiers: "4K"}, want: nil},
		{name: "ordered and case-insensitive", extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []any{" 2k", "1K", "2K"}}, want: []string{"1K", "2K"}},
		{name: "string slice", extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []string{"4K"}}, want: []string{"4K"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Platform: PlatformOpenAI, Extra: tc.extra}
			require.Equal(t, tc.want, account.OpenAIImageSizeTiers())
		})
	}

	lowOnly := &Account{Extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []any{"1K", "2K"}}}
	require.True(t, lowOnly.SupportsOpenAIImageSizeTier("2K"))
	require.False(t, lowOnly.SupportsOpenAIImageSizeTier("4K"))
	require.True(t, (&Account{}).SupportsOpenAIImageSizeTier("4K"))
}

func TestAccountOpenAIImageTestSize(t *testing.T) {
	require.Empty(t, (&Account{}).OpenAIImageTestSize())
	require.Equal(t, "1024x1024", (&Account{Extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []any{"2K", "1K"}}}).OpenAIImageTestSize())
	require.Equal(t, "1536x1024", (&Account{Extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []any{"2K", "4K"}}}).OpenAIImageTestSize())
	require.Equal(t, "3840x2160", (&Account{Extra: map[string]any{AccountExtraOpenAIImageSizeTiers: []any{"4K"}}}).OpenAIImageTestSize())
}

func TestOpenAIResponsesImageRequestSize(t *testing.T) {
	require.Equal(t, "3840x2160", OpenAIResponsesImageRequestSize([]byte(`{"model":"gpt-5.4","tools":[{"type":"web_search"},{"type":"image_generation","size":"3840x2160"}]}`)))
	require.Equal(t, "1024x1024", OpenAIResponsesImageRequestSize([]byte(`{"model":"gpt-5.4","size":"1024x1024","tools":[{"type":"image_generation"}]}`)))
	require.Empty(t, OpenAIResponsesImageRequestSize([]byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation"}]}`)))
}

func newOpenAIImageSizeTestAccount(id int64, priority int, tiers ...string) Account {
	extra := map[string]any{}
	if len(tiers) > 0 {
		values := make([]any, 0, len(tiers))
		for _, tier := range tiers {
			values = append(values, tier)
		}
		extra[AccountExtraOpenAIImageSizeTiers] = values
	}
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    priority,
		Extra:       extra,
	}
}

type openAIImageSizeSchedulerMode struct {
	name      string
	advanced  bool
	loadBatch bool
}

var openAIImageSizeSchedulerModes = []openAIImageSizeSchedulerMode{
	{name: "advanced", advanced: true, loadBatch: true},
	{name: "legacy_load_batch", loadBatch: true},
	{name: "legacy_sequential"},
}

func newOpenAIImageSizeSchedulerTestService(mode openAIImageSizeSchedulerMode, accounts []Account, cache *schedulerTestGatewayCache, concurrency schedulerTestConcurrencyCache) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = mode.loadBatch
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 3
	if cache == nil {
		cache = &schedulerTestGatewayCache{}
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrency),
	}
	if mode.advanced {
		svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	} else {
		svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("false")
	}
	return svc
}

func selectOpenAIImageSizeTestAccount(t *testing.T, svc *OpenAIGatewayService, groupID int64, sessionHash string, size string) (*AccountSelectionResult, error) {
	t.Helper()
	ctx := WithOpenAIImageSizeRouting(context.Background(), size)
	selection, _, err := svc.SelectAccountWithSchedulerForImages(ctx, &groupID, sessionHash, "gpt-image-2", nil, OpenAIImagesCapabilityNative)
	return selection, err
}

func TestSelectAccountWithSchedulerForImages_ExplicitSizeFiltersByTier(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(93001)
			accounts := []Account{
				newOpenAIImageSizeTestAccount(81001, 0, "1K", "2K"),
				newOpenAIImageSizeTestAccount(81002, 5, "4K"),
			}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, nil, schedulerTestConcurrencyCache{})

			selection, err := selectOpenAIImageSizeTestAccount(t, svc, groupID, "", "3840x2160")
			require.NoError(t, err)
			require.Equal(t, int64(81002), selection.Account.ID)

			selection, err = selectOpenAIImageSizeTestAccount(t, svc, groupID, "", "1536x1024")
			require.NoError(t, err)
			require.Equal(t, int64(81001), selection.Account.ID)

			selection, err = selectOpenAIImageSizeTestAccount(t, svc, groupID, "", "4K")
			require.NoError(t, err)
			require.Equal(t, int64(81002), selection.Account.ID)
		})
	}
}

func TestSelectAccountWithSchedulerForImages_ExplicitSizeWithoutTierAccountFails(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(93002)
			accounts := []Account{
				newOpenAIImageSizeTestAccount(81011, 0, "1K", "2K"),
				newOpenAIImageSizeTestAccount(81012, 0, "2K"),
			}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, nil, schedulerTestConcurrencyCache{})

			selection, err := selectOpenAIImageSizeTestAccount(t, svc, groupID, "", "2880x2880")
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrNoAvailableAccounts))
			require.Nil(t, selection)
		})
	}
}

func TestSelectAccountWithSchedulerForImages_UnspecifiedSizePreferenceOrder(t *testing.T) {
	cases := []struct {
		name     string
		accounts []Account
		want     int64
	}{
		{
			name: "unrestricted first",
			accounts: []Account{
				newOpenAIImageSizeTestAccount(81021, 0, "4K"),
				newOpenAIImageSizeTestAccount(81022, 0, "1K", "2K"),
				newOpenAIImageSizeTestAccount(81023, 9),
			},
			want: 81023,
		},
		{
			name: "then 4K",
			accounts: []Account{
				newOpenAIImageSizeTestAccount(81031, 0, "1K", "2K"),
				newOpenAIImageSizeTestAccount(81032, 9, "2K", "4K"),
			},
			want: 81032,
		},
		{
			name: "then 2K",
			accounts: []Account{
				newOpenAIImageSizeTestAccount(81041, 0, "1K"),
				newOpenAIImageSizeTestAccount(81042, 9, "2K"),
			},
			want: 81042,
		},
		{
			name: "then 1K",
			accounts: []Account{
				newOpenAIImageSizeTestAccount(81051, 0, "1K"),
			},
			want: 81051,
		},
	}
	for _, mode := range openAIImageSizeSchedulerModes {
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				svc := newOpenAIImageSizeSchedulerTestService(mode, tc.accounts, nil, schedulerTestConcurrencyCache{})
				for _, size := range []string{"", "auto", "3:2"} {
					selection, err := selectOpenAIImageSizeTestAccount(t, svc, 93003, "", size)
					require.NoError(t, err, size)
					require.Equal(t, tc.want, selection.Account.ID, size)
				}
			})
		}
	}
}

func TestSelectAccountWithSchedulerForImages_UnspecifiedSizeSkipsBusyPreferredAccount(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			accounts := []Account{
				newOpenAIImageSizeTestAccount(81061, 0),
				newOpenAIImageSizeTestAccount(81062, 9, "4K"),
			}
			concurrency := schedulerTestConcurrencyCache{acquireResults: map[int64]bool{81061: false}}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, nil, concurrency)

			selection, err := selectOpenAIImageSizeTestAccount(t, svc, 93004, "", "")
			require.NoError(t, err)
			require.Equal(t, int64(81062), selection.Account.ID)
			require.True(t, selection.Acquired)
		})
	}
}

func TestSelectAccountWithSchedulerForImages_UnspecifiedSizeSkipsBusyPreferredAccountWithSession(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(93009)
			accounts := []Account{
				newOpenAIImageSizeTestAccount(81111, 0),
				newOpenAIImageSizeTestAccount(81112, 9, "4K"),
			}
			for i := range accounts {
				accounts[i].GroupIDs = []int64{groupID}
			}
			cache := &schedulerTestGatewayCache{}
			concurrency := schedulerTestConcurrencyCache{acquireResults: map[int64]bool{81111: false}}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, cache, concurrency)

			selection, err := selectOpenAIImageSizeTestAccount(t, svc, groupID, "image-size-busy-session", "")
			require.NoError(t, err)
			require.Equal(t, int64(81112), selection.Account.ID)
			require.True(t, selection.Acquired)
		})
	}
}

func TestSelectAccountWithSchedulerForImages_SpilloverToRestrictedAccountKeepsStickyBinding(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(93011)
			sessionHash := "image-size-spillover"
			accounts := []Account{
				newOpenAIImageSizeTestAccount(81131, 0),
				newOpenAIImageSizeTestAccount(81132, 9, "4K"),
			}
			for i := range accounts {
				accounts[i].GroupIDs = []int64{groupID}
			}
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + sessionHash: 81131}}
			concurrency := schedulerTestConcurrencyCache{
				acquireResults: map[int64]bool{81131: false},
				waitCounts:     map[int64]int{81131: 3},
				loadMap:        map[int64]*AccountLoadInfo{81131: {AccountID: 81131, LoadRate: 100, WaitingCount: 3}},
			}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, cache, concurrency)

			selection, err := selectOpenAIImageSizeTestAccount(t, svc, groupID, sessionHash, "")
			require.NoError(t, err)
			require.NotNil(t, selection.Account)
			require.Equal(t, int64(81131), cache.sessionBindings["openai:"+sessionHash], "a spillover pick must not rebind the session")
			if mode.loadBatch {
				require.Equal(t, int64(81132), selection.Account.ID)
				require.True(t, selection.Acquired)
			}
		})
	}
}

func TestSelectAccountWithSchedulerForImages_SpilloverWithoutBindingBindsServedAccount(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		if !mode.loadBatch {
			continue
		}
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(93012)
			sessionHash := "image-size-spillover-unbound"
			accounts := []Account{
				newOpenAIImageSizeTestAccount(81141, 0),
				newOpenAIImageSizeTestAccount(81142, 9, "4K"),
			}
			for i := range accounts {
				accounts[i].GroupIDs = []int64{groupID}
			}
			cache := &schedulerTestGatewayCache{}
			concurrency := schedulerTestConcurrencyCache{
				acquireResults: map[int64]bool{81141: false},
				waitCounts:     map[int64]int{81141: 3},
				loadMap:        map[int64]*AccountLoadInfo{81141: {AccountID: 81141, LoadRate: 100, WaitingCount: 3}},
			}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, cache, concurrency)

			selection, err := selectOpenAIImageSizeTestAccount(t, svc, groupID, sessionHash, "")
			require.NoError(t, err)
			require.Equal(t, int64(81142), selection.Account.ID)
			require.Equal(t, int64(81142), cache.sessionBindings["openai:"+sessionHash])
		})
	}
}

func TestSelectAccountWithSchedulerForImages_UnspecifiedSizePrefersHealthyProxyOverQuarantinedUnrestricted(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			quarantinedProxy := int64(71)
			healthyProxy := int64(72)
			unrestricted := newOpenAIImageSizeTestAccount(81121, 0)
			unrestricted.ProxyID = &quarantinedProxy
			highOnly := newOpenAIImageSizeTestAccount(81122, 9, "4K")
			highOnly.ProxyID = &healthyProxy
			svc := newOpenAIImageSizeSchedulerTestService(mode, []Account{unrestricted, highOnly}, nil, schedulerTestConcurrencyCache{})
			svc.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
				failureThreshold: 1,
				failureWindow:    time.Minute,
				quarantineTTL:    10 * time.Minute,
				maxEntries:       16,
			})
			svc.openaiProxyStreamCircuit.recordFailure(quarantinedProxy, time.Now())

			selection, err := selectOpenAIImageSizeTestAccount(t, svc, 93010, "", "")
			require.NoError(t, err)
			require.Equal(t, int64(81122), selection.Account.ID, "a restricted account on a healthy proxy beats the quarantine fail-open")

			svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{unrestricted}}
			selection, err = selectOpenAIImageSizeTestAccount(t, svc, 93010, "", "")
			require.NoError(t, err)
			require.Equal(t, int64(81121), selection.Account.ID, "fail-open still serves when nothing else can")
		})
	}
}

func TestSelectAccountWithSchedulerForImages_StickyAccountUnderSizeRouting(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(93005)
			sessionHash := "image-size-sticky"
			accounts := []Account{
				newOpenAIImageSizeTestAccount(81071, 0),
				newOpenAIImageSizeTestAccount(81072, 9, "1K", "2K"),
			}
			for i := range accounts {
				accounts[i].GroupIDs = []int64{groupID}
			}
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
				"openai:" + sessionHash: 81072,
			}}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, cache, schedulerTestConcurrencyCache{})

			selection, err := selectOpenAIImageSizeTestAccount(t, svc, groupID, sessionHash, "")
			require.NoError(t, err)
			require.Equal(t, int64(81072), selection.Account.ID, "unspecified size keeps the sticky account")

			cache.sessionBindings["openai:"+sessionHash] = 81072
			selection, err = selectOpenAIImageSizeTestAccount(t, svc, groupID, sessionHash, "3840x2160")
			require.NoError(t, err)
			require.Equal(t, int64(81071), selection.Account.ID, "explicit 4K leaves a 1K/2K-only sticky account")
		})
	}
}

func TestSelectAccountWithScheduler_PreviousResponseAccountUnderSizeRouting(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(93008)
			accounts := []Account{
				newOpenAIImageSizeTestAccount(81101, 0),
				newOpenAIImageSizeTestAccount(81102, 9, "1K", "2K"),
			}
			for i := range accounts {
				accounts[i].GroupIDs = []int64{groupID}
			}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, nil, schedulerTestConcurrencyCache{})
			require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(context.Background(), groupID, "resp_image_size", 81102, time.Hour))

			selectWithSize := func(size string) int64 {
				ctx := WithOpenAIImageSizeRouting(context.Background(), size)
				selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "resp_image_size", "", "gpt-5.4", nil, OpenAIUpstreamTransportAny, false)
				require.NoError(t, err)
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				return selection.Account.ID
			}
			require.Equal(t, int64(81102), selectWithSize(""), "unspecified size keeps the previous_response account")
			require.Equal(t, int64(81101), selectWithSize("3840x2160"), "explicit 4K leaves a 1K/2K-only previous_response account")
		})
	}
}

func TestSelectAccountWithScheduler_TextRequestIgnoresImageSizeTiers(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(93006)
			accounts := []Account{newOpenAIImageSizeTestAccount(81081, 0, "4K")}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, nil, schedulerTestConcurrencyCache{})

			selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-5.4", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.Equal(t, int64(81081), selection.Account.ID)
		})
	}
}

func TestSelectOpenAIAccountByImageSizePasses_StopsWhenNoAccountVetoed(t *testing.T) {
	restricted := newOpenAIImageSizeTestAccount(1, 0, "4K")
	unrestricted := newOpenAIImageSizeTestAccount(2, 0)

	calls := 0
	_, _, err := selectOpenAIAccountByImageSizePasses(WithOpenAIImageSizeRouting(context.Background(), ""), func(ctx context.Context) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
		calls++
		vetoed, _ := openAIImageSizeVetoReason(ctx, &unrestricted)
		require.False(t, vetoed)
		return nil, OpenAIAccountScheduleDecision{}, ErrNoAvailableAccounts
	})
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Equal(t, 1, calls)

	calls = 0
	_, _, err = selectOpenAIAccountByImageSizePasses(WithOpenAIImageSizeRouting(context.Background(), ""), func(ctx context.Context) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
		calls++
		_, _ = openAIImageSizeVetoReason(ctx, &restricted)
		return nil, OpenAIAccountScheduleDecision{}, ErrNoAvailableAccounts
	})
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Equal(t, 2, calls, "the 4K pass admits the 4K-only account, so the 2K and 1K passes are skipped")
}

func TestSelectOpenAIAccountByImageSizePasses_ReturnsEarliestWaitPlan(t *testing.T) {
	restricted := newOpenAIImageSizeTestAccount(1, 0, "1K")
	calls := 0
	selection, _, err := selectOpenAIAccountByImageSizePasses(WithOpenAIImageSizeRouting(context.Background(), "auto"), func(ctx context.Context) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
		calls++
		_, _ = openAIImageSizeVetoReason(ctx, &restricted)
		if calls == 1 {
			return &AccountSelectionResult{Account: &Account{ID: 10}, WaitPlan: &AccountWaitPlan{AccountID: 10}}, OpenAIAccountScheduleDecision{}, nil
		}
		if calls == 2 {
			return &AccountSelectionResult{Account: &Account{ID: 11}, WaitPlan: &AccountWaitPlan{AccountID: 11}}, OpenAIAccountScheduleDecision{}, nil
		}
		return nil, OpenAIAccountScheduleDecision{}, ErrNoAvailableAccounts
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), selection.Account.ID)
	require.Equal(t, 2, calls, "the unrestricted pass jumps straight to the 1K-only rank")
}

func TestOpenAIImageSizeTierUnsupported(t *testing.T) {
	groupID := int64(93007)
	lowOnly := newOpenAIImageSizeTestAccount(81091, 0, "1K", "2K")
	highOnly := newOpenAIImageSizeTestAccount(81092, 0, "4K")
	textOnly := newOpenAIImageSizeTestAccount(81093, 0)
	textOnly.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}}

	svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{lowOnly, textOnly}}}
	tier, unsupported := svc.OpenAIImageSizeTierUnsupported(context.Background(), &groupID, "gpt-image-2", "3840x2160", "", OpenAIImagesCapabilityNative)
	require.True(t, unsupported)
	require.Equal(t, "4K", tier)

	_, unsupported = svc.OpenAIImageSizeTierUnsupported(context.Background(), &groupID, "gpt-image-2", "1024x1024", "", OpenAIImagesCapabilityNative)
	require.False(t, unsupported)
	_, unsupported = svc.OpenAIImageSizeTierUnsupported(context.Background(), &groupID, "gpt-image-2", "auto", "", OpenAIImagesCapabilityNative)
	require.False(t, unsupported)

	svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{lowOnly, highOnly}}
	_, unsupported = svc.OpenAIImageSizeTierUnsupported(context.Background(), &groupID, "gpt-image-2", "3840x2160", "", OpenAIImagesCapabilityNative)
	require.False(t, unsupported)

	svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{textOnly}}
	_, unsupported = svc.OpenAIImageSizeTierUnsupported(context.Background(), &groupID, "gpt-image-2", "3840x2160", "", OpenAIImagesCapabilityNative)
	require.False(t, unsupported, "no account serves the model: leave classification to model_not_found")

	chatOnly := newOpenAIImageSizeTestAccount(81094, 0)
	chatOnly.Extra["openai_responses_supported"] = false
	svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{chatOnly, lowOnly}}
	_, unsupported = svc.OpenAIImageSizeTierUnsupported(context.Background(), &groupID, "gpt-5.4", "3840x2160", OpenAIEndpointCapabilityResponses, "")
	require.True(t, unsupported, "an unrestricted account without the Responses capability cannot serve the request")
	_, unsupported = svc.OpenAIImageSizeTierUnsupported(context.Background(), &groupID, "gpt-5.4", "3840x2160", OpenAIEndpointCapabilityChatCompletions, "")
	require.False(t, unsupported)
}
