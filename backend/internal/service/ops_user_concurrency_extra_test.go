//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type opsUserConcurrencyRepoStub struct {
	UserRepository
	users []User
}

func (s *opsUserConcurrencyRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return s.users, &pagination.PaginationResult{Total: int64(len(s.users)), Page: 1, PageSize: len(s.users), Pages: 1}, nil
}

type opsUserConcurrencyCacheStub struct {
	ConcurrencyCache
	load  *UserLoadInfo
	input []UserWithConcurrency
}

func (s *opsUserConcurrencyCacheStub) GetUsersLoadBatch(_ context.Context, users []UserWithConcurrency) (map[int64]*UserLoadInfo, error) {
	s.input = append([]UserWithConcurrency(nil), users...)
	return map[int64]*UserLoadInfo{s.load.UserID: s.load}, nil
}

func TestOpsUserConcurrencyStatsSplitStandardAndExtraWhileKeepingAggregate(t *testing.T) {
	user := User{
		ID:               77,
		Email:            "ops-extra@example.com",
		Username:         "ops-extra",
		Status:           StatusActive,
		Concurrency:      2,
		ExtraConcurrency: 3,
	}
	cache := &opsUserConcurrencyCacheStub{load: &UserLoadInfo{
		UserID:              user.ID,
		StandardConcurrency: 2,
		ExtraConcurrency:    1,
		CurrentConcurrency:  3,
		WaitingCount:        4,
	}}
	ops := NewOpsService(
		nil,
		nil,
		nil,
		nil,
		&opsUserConcurrencyRepoStub{users: []User{user}},
		NewConcurrencyService(cache),
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	stats, _, err := ops.GetUserConcurrencyStats(context.Background())

	require.NoError(t, err)
	require.Len(t, cache.input, 1)
	require.Equal(t, user.ExtraConcurrency, cache.input[0].ExtraConcurrency)
	info := stats[user.ID]
	require.NotNil(t, info)
	require.Equal(t, int64(2), info.StandardCurrentInUse)
	require.Equal(t, int64(2), info.StandardMaxCapacity)
	require.Equal(t, int64(1), info.ExtraCurrentInUse)
	require.Equal(t, int64(3), info.ExtraMaxCapacity)
	require.Equal(t, int64(3), info.CurrentInUse)
	require.Equal(t, int64(5), info.MaxCapacity)
	require.Equal(t, 60.0, info.LoadPercentage)
	require.Equal(t, int64(4), info.WaitingInQueue)
}
