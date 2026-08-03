package service

import (
	"errors"
	"testing"

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
