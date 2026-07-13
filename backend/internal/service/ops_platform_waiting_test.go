//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type opsPlatformWaitingAccountRepoStub struct {
	AccountRepository
	accounts []Account
}

func (s *opsPlatformWaitingAccountRepoStub) ListOpsAccountsForStats(context.Context, string, *int64) ([]Account, error) {
	return s.accounts, nil
}

type opsPlatformWaitingCacheStub struct {
	ConcurrencyCache
	accountLoads    map[int64]*AccountLoadInfo
	platformWaiting map[string]int
}

func (s *opsPlatformWaitingCacheStub) GetAccountsLoadBatch(context.Context, []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	return s.accountLoads, nil
}

func (s *opsPlatformWaitingCacheStub) GetPlatformsWaitingBatch(context.Context, []string) (map[string]int, error) {
	return s.platformWaiting, nil
}

func TestOpsConcurrencyStatsAddsPlatformWaitingWithoutAttributingItToAccounts(t *testing.T) {
	const platformName = PlatformAnthropic
	accounts := []Account{
		{ID: 101, Name: "first", Platform: platformName, Concurrency: 2},
		{ID: 102, Name: "second", Platform: platformName, Concurrency: 3},
	}
	cache := &opsPlatformWaitingCacheStub{
		accountLoads: map[int64]*AccountLoadInfo{
			101: {AccountID: 101, WaitingCount: 1},
			102: {AccountID: 102, WaitingCount: 2},
		},
		platformWaiting: map[string]int{platformName: 4},
	}
	ops := NewOpsService(
		nil,
		nil,
		nil,
		&opsPlatformWaitingAccountRepoStub{accounts: accounts},
		nil,
		NewConcurrencyService(cache),
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	platforms, _, accountStats, _, err := ops.GetConcurrencyStats(context.Background(), "", nil)

	require.NoError(t, err)
	require.Equal(t, int64(1), accountStats[101].WaitingInQueue)
	require.Equal(t, int64(2), accountStats[102].WaitingInQueue)
	require.Equal(t, int64(7), platforms[platformName].WaitingInQueue)
}

func TestOpsConcurrencyStatsGroupFilterExcludesUnattributedPlatformWaiting(t *testing.T) {
	const (
		platformName = PlatformAnthropic
		groupID      = int64(901)
	)
	accounts := []Account{{
		ID:          101,
		Name:        "filtered",
		Platform:    platformName,
		Concurrency: 2,
		Groups:      []*Group{{ID: groupID, Name: "target-group"}},
	}}
	cache := &opsPlatformWaitingCacheStub{
		accountLoads: map[int64]*AccountLoadInfo{
			101: {AccountID: 101, WaitingCount: 1},
		},
		platformWaiting: map[string]int{platformName: 4},
	}
	ops := NewOpsService(
		nil,
		nil,
		nil,
		&opsPlatformWaitingAccountRepoStub{accounts: accounts},
		nil,
		NewConcurrencyService(cache),
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	platforms, groups, accountStats, _, err := ops.GetConcurrencyStats(context.Background(), "", ptr(groupID))

	require.NoError(t, err)
	require.Equal(t, int64(1), accountStats[101].WaitingInQueue)
	require.Equal(t, int64(1), groups[groupID].WaitingInQueue)
	require.Equal(t, int64(1), platforms[platformName].WaitingInQueue)
}
