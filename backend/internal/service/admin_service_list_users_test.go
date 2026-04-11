//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type userRepoStubForListUsers struct {
	userRepoStub
	users                 []User
	err                   error
	listWithFiltersParams pagination.PaginationParams
}

func (s *userRepoStubForListUsers) ListWithFilters(_ context.Context, params pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	s.listWithFiltersParams = params
	if s.err != nil {
		return nil, nil, s.err
	}
	out := make([]User, len(s.users))
	copy(out, s.users)
	return out, &pagination.PaginationResult{
		Total:    int64(len(out)),
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

type userGroupRateRepoStubForListUsers struct {
	batchCalls int
	singleCall []int64

	batchErr  error
	batchData map[int64]map[int64]float64

	singleErr  map[int64]error
	singleData map[int64]map[int64]float64
}

func (s *userGroupRateRepoStubForListUsers) GetByUserIDs(_ context.Context, _ []int64) (map[int64]map[int64]float64, error) {
	s.batchCalls++
	if s.batchErr != nil {
		return nil, s.batchErr
	}
	return s.batchData, nil
}

func (s *userGroupRateRepoStubForListUsers) GetByUserID(_ context.Context, userID int64) (map[int64]float64, error) {
	s.singleCall = append(s.singleCall, userID)
	if err, ok := s.singleErr[userID]; ok {
		return nil, err
	}
	if rates, ok := s.singleData[userID]; ok {
		return rates, nil
	}
	return map[int64]float64{}, nil
}

func (s *userGroupRateRepoStubForListUsers) GetByUserAndGroup(_ context.Context, userID, groupID int64) (*float64, error) {
	panic("unexpected GetByUserAndGroup call")
}

func (s *userGroupRateRepoStubForListUsers) SyncUserGroupRates(_ context.Context, userID int64, rates map[int64]*float64) error {
	panic("unexpected SyncUserGroupRates call")
}

func (s *userGroupRateRepoStubForListUsers) GetByGroupID(_ context.Context, _ int64) ([]UserGroupRateEntry, error) {
	panic("unexpected GetByGroupID call")
}

func (s *userGroupRateRepoStubForListUsers) SyncGroupRateMultipliers(_ context.Context, _ int64, _ []GroupRateMultiplierInput) error {
	panic("unexpected SyncGroupRateMultipliers call")
}

func (s *userGroupRateRepoStubForListUsers) DeleteByGroupID(_ context.Context, _ int64) error {
	panic("unexpected DeleteByGroupID call")
}

func (s *userGroupRateRepoStubForListUsers) DeleteByUserID(_ context.Context, userID int64) error {
	panic("unexpected DeleteByUserID call")
}

func TestAdminService_ListUsers_BatchRateFallbackToSingle(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{
			{ID: 101, Username: "u1"},
			{ID: 202, Username: "u2"},
		},
	}
	rateRepo := &userGroupRateRepoStubForListUsers{
		batchErr: errors.New("batch unavailable"),
		singleData: map[int64]map[int64]float64{
			101: {11: 1.1},
			202: {22: 2.2},
		},
	}
	svc := &adminServiceImpl{
		userRepo:          userRepo,
		userGroupRateRepo: rateRepo,
	}

	users, total, err := svc.ListUsers(context.Background(), 1, 20, UserListFilters{}, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, users, 2)
	require.Equal(t, 1, rateRepo.batchCalls)
	require.ElementsMatch(t, []int64{101, 202}, rateRepo.singleCall)
	require.Equal(t, 1.1, users[0].GroupRates[11])
	require.Equal(t, 2.2, users[1].GroupRates[22])
}

func TestAdminService_ListUsers_PassesSortParams(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{{ID: 1, Email: "a@example.com"}},
	}
	svc := &adminServiceImpl{userRepo: userRepo}

	_, _, err := svc.ListUsers(context.Background(), 2, 50, UserListFilters{}, "email", "ASC")
	require.NoError(t, err)
	require.Equal(t, pagination.PaginationParams{
		Page:      2,
		PageSize:  50,
		SortBy:    "email",
		SortOrder: "ASC",
	}, userRepo.listWithFiltersParams)
}
