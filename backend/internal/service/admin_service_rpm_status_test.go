//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type rpmStatusUserRepoStub struct {
	UserRepository
	user *User
}

func (s *rpmStatusUserRepoStub) GetByID(_ context.Context, _ int64) (*User, error) {
	return s.user, nil
}

type rpmStatusAPIKeyRepoStub struct {
	APIKeyRepository
	keys []APIKey
}

func (s *rpmStatusAPIKeyRepoStub) ListByUserID(_ context.Context, _ int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	return s.keys, &pagination.PaginationResult{Total: int64(len(s.keys))}, nil
}

type rpmStatusGroupRepoStub struct {
	GroupRepository
	groups map[int64]*Group
}

func (s *rpmStatusGroupRepoStub) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	return s.groups[id], nil
}

type rpmStatusRateRepoStub struct {
	UserGroupRateRepository
	overrides map[int64]*int
}

func (s *rpmStatusRateRepoStub) GetRPMOverrideByUserAndGroup(_ context.Context, _, groupID int64) (*int, error) {
	return s.overrides[groupID], nil
}

type rpmStatusCacheStub struct {
	UserRPMCache
	userUsed  int
	groupUsed map[int64]int
}

func (s *rpmStatusCacheStub) IncrementUserGroupRPM(context.Context, int64, int64) (int, error) {
	return 0, nil
}

func (s *rpmStatusCacheStub) IncrementUserRPM(context.Context, int64) (int, error) {
	return 0, nil
}

func (s *rpmStatusCacheStub) GetUserGroupRPM(_ context.Context, _, groupID int64) (int, error) {
	return s.groupUsed[groupID], nil
}

func (s *rpmStatusCacheStub) GetUserRPM(context.Context, int64) (int, error) {
	return s.userUsed, nil
}

func TestAdminService_GetUserRPMStatus_AggregatesUserAndGroupLimits(t *testing.T) {
	groupOneID := int64(1)
	groupTwoID := int64(2)
	override := 7
	svc := &adminServiceImpl{
		userRepo: &rpmStatusUserRepoStub{user: &User{
			ID:       42,
			RPMLimit: 20,
		}},
		apiKeyRepo: &rpmStatusAPIKeyRepoStub{keys: []APIKey{
			{ID: 100, UserID: 42, GroupID: &groupTwoID},
			{ID: 101, UserID: 42, GroupID: &groupOneID},
			{ID: 102, UserID: 42, GroupID: &groupTwoID},
			{ID: 103, UserID: 42},
		}},
		groupRepo: &rpmStatusGroupRepoStub{groups: map[int64]*Group{
			groupOneID: {ID: groupOneID, Name: "group-one", RPMLimit: 10},
			groupTwoID: {ID: groupTwoID, Name: "group-two", RPMLimit: 60},
		}},
		userGroupRateRepo: &rpmStatusRateRepoStub{overrides: map[int64]*int{
			groupTwoID: &override,
		}},
		userRPMCache: &rpmStatusCacheStub{
			userUsed: 5,
			groupUsed: map[int64]int{
				groupOneID: 3,
				groupTwoID: 4,
			},
		},
	}

	status, err := svc.GetUserRPMStatus(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, &UserRPMStatus{
		UserRPMUsed:  5,
		UserRPMLimit: 20,
		PerGroup: []UserGroupRPMStatus{
			{GroupID: groupOneID, GroupName: "group-one", Used: 3, Limit: 10, Source: "group"},
			{GroupID: groupTwoID, GroupName: "group-two", Used: 4, Limit: 7, Source: "override"},
		},
	}, status)
}
