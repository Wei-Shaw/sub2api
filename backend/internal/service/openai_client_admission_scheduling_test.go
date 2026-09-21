package service

import (
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCodexClientAdmissionRestrictedAndBusyCandidateKeepsWaitPlan(t *testing.T) {
	groupID := int64(8801)
	newAccounts := func() []Account {
		restricted := codexAdmissionAccount(88011, true)
		restricted.GroupIDs = []int64{groupID}
		restricted.Priority = 0
		regular := codexAdmissionAccount(88012, false)
		regular.GroupIDs = []int64{groupID}
		regular.Priority = 1
		return []Account{restricted, regular}
	}
	newConcurrency := func() *ConcurrencyService {
		return NewConcurrencyService(schedulerTestConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				88011: {AccountID: 88011, CurrentConcurrency: 1, LoadRate: 100},
				88012: {AccountID: 88012, CurrentConcurrency: 1, LoadRate: 100},
			},
			acquireResults: map[int64]bool{88012: false},
		})
	}

	t.Run("legacy load aware", func(t *testing.T) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		accounts := newAccounts()
		repo := &codexAdmissionAccountRepo{
			accounts: accounts,
			byID: map[int64]*Account{
				accounts[0].ID: &accounts[0],
				accounts[1].ID: &accounts[1],
			},
		}
		svc := &OpenAIGatewayService{
			accountRepo:        repo,
			cfg:                &config.Config{},
			codexDetector:      &accountAwareCodexAdmissionDetector{},
			concurrencyService: newConcurrency(),
		}
		ctx := newCodexAdmissionContext(t, svc)

		selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.Equal(t, int64(88012), selection.Account.ID)
		require.NotNil(t, selection.WaitPlan)
		require.False(t, errors.Is(err, ErrCodexClientRestricted))
	})

	t.Run("advanced scheduler", func(t *testing.T) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
		accounts := newAccounts()
		repo := &codexAdmissionAccountRepo{
			accounts: accounts,
			byID: map[int64]*Account{
				accounts[0].ID: &accounts[0],
				accounts[1].ID: &accounts[1],
			},
		}
		cfg := &config.Config{}
		svc := &OpenAIGatewayService{
			accountRepo:        repo,
			cfg:                cfg,
			codexDetector:      &accountAwareCodexAdmissionDetector{},
			rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
			concurrencyService: newConcurrency(),
		}
		ctx := newCodexAdmissionContext(t, svc)

		selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.Equal(t, int64(88012), selection.Account.ID)
		require.NotNil(t, selection.WaitPlan)
		require.False(t, errors.Is(err, ErrCodexClientRestricted))
	})
}

func TestCodexClientAdmissionTokenCountWithoutGenerationSlot(t *testing.T) {
	for _, tc := range []struct {
		name                string
		candidateRestricted bool
		latestRestricted    bool
		withFallback        bool
		repositoryError     error
		wantAccountID       int64
		wantError           error
	}{
		{name: "skip restricted candidate", candidateRestricted: true, latestRestricted: true, withFallback: true, wantAccountID: 88102},
		{name: "retry after terminal veto", latestRestricted: true, withFallback: true, wantAccountID: 88102},
		{name: "terminal veto exhausts pool", latestRestricted: true, wantError: ErrCodexClientRestricted},
		{name: "repository unavailable", withFallback: true, repositoryError: errors.New("repository unavailable"), wantError: ErrCodexClientAdmissionUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := codexAdmissionAccount(88101, tc.candidateRestricted)
			latest := codexAdmissionAccount(candidate.ID, tc.latestRestricted)
			repo := &codexAdmissionAccountRepo{
				accounts: []Account{candidate},
				byID:     map[int64]*Account{candidate.ID: &latest},
				getErr:   tc.repositoryError,
			}
			if tc.withFallback {
				fallback := codexAdmissionAccount(88102, false)
				fallback.Priority = 1
				repo.accounts = append(repo.accounts, fallback)
				repo.byID[fallback.ID] = &fallback
			}
			var acquiredIDs []int64
			svc := &OpenAIGatewayService{
				accountRepo:   repo,
				cfg:           &config.Config{},
				codexDetector: &accountAwareCodexAdmissionDetector{},
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
					acquiredIDs: &acquiredIDs,
				}),
			}
			ctx := newCodexAdmissionContext(t, svc)
			account, err := svc.SelectAccountForTokenCount(ctx, nil, "", "gpt-5.1", OpenAIEndpointCapabilityChatCompletions, PlatformOpenAI)
			require.Empty(t, acquiredIDs, "token counting must not acquire a generation slot")
			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
				require.Nil(t, account)
				if tc.repositoryError != nil {
					require.Equal(t, int64(1), repo.getCalls.Load(), "repository failures must not retry another account")
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantAccountID, account.ID)
			_, err = svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, account)
			require.NoError(t, err, "token counting must forward the terminally admitted object")
		})
	}
}

func TestCodexClientAdmissionLegacyPreviousResponseKeepsPrioritySelection(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		t.Run(accountType, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
			groupID := int64(8811)
			bound := codexAdmissionAccount(88111, false)
			bound.Type = accountType
			bound.GroupIDs = []int64{groupID}
			bound.Priority = 5
			bound.Extra["openai_oauth_responses_websockets_v2_enabled"] = true
			preferred := codexAdmissionAccount(88112, false)
			preferred.Type = accountType
			preferred.GroupIDs = []int64{groupID}
			cache := &countingCodexStickyCache{}
			svc := &OpenAIGatewayService{
				accountRepo: &codexAdmissionAccountRepo{
					accounts: []Account{bound, preferred},
					byID:     map[int64]*Account{bound.ID: &bound, preferred.ID: &preferred},
				},
				cache:              cache,
				cfg:                newOpenAIWSV2TestConfig(),
				codexDetector:      &accountAwareCodexAdmissionDetector{},
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
			}
			ctx := newCodexAdmissionContext(t, svc)
			require.True(t, codexClientAdmissionActive(ctx))
			store := svc.getOpenAIWSStateStore()
			require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_legacy_admission", bound.ID, time.Hour))

			selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "resp_legacy_admission", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.Equal(t, preferred.ID, selection.Account.ID)
			require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
			require.False(t, decision.StickyPreviousHit)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			boundID, err := store.GetResponseAccount(ctx, groupID, "resp_legacy_admission")
			require.NoError(t, err)
			require.Equal(t, bound.ID, boundID)
		})
	}
}
