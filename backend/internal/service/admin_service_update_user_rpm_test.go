//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// rpmUserRepoStub 复用 admin_service_update_balance_test.go 的基础 stub 结构，
// 只在 Update 时把入参克隆一份，便于断言修改后的 RPMLimit。
type rpmUserRepoStub struct {
	*userRepoStub
	lastUpdated *User
}

func (s *rpmUserRepoStub) Update(_ context.Context, user *User, _ UserUpdateFields) error {
	if user == nil {
		return nil
	}
	clone := *user
	s.lastUpdated = &clone
	if s.userRepoStub != nil {
		s.userRepoStub.user = &clone
	}
	return nil
}

func TestAdminService_UpdateUser_InvalidatesAuthCacheOnRPMLimitChange(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", RPMLimit: 10}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{},
		authCacheInvalidator: invalidator,
	}

	newRPM := 60
	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{
		RPMLimit: &newRPM,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 60, updated.RPMLimit)
	require.Equal(t, []int64{42}, invalidator.userIDs, "仅修改 RPMLimit 也应失效 API Key 认证缓存")
}

func TestAdminService_UpdateUser_NoInvalidateWhenRPMLimitUnchanged(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", RPMLimit: 10, Username: "old"}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{},
		authCacheInvalidator: invalidator,
	}

	newName := "new"
	sameRPM := 10
	_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{
		Username: &newName,
		RPMLimit: &sameRPM,
	})
	require.NoError(t, err)
	require.Empty(t, invalidator.userIDs, "只改 username 不应触发认证缓存失效")
}

func TestAdminService_UpdateUser_ChangesRoleAndInvalidatesAuthCache(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleUser}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{},
		authCacheInvalidator: invalidator,
	}

	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{
		Role: RoleMarketing,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, RoleMarketing, updated.Role)
	require.Equal(t, []int64{42}, invalidator.userIDs, "修改 role 后应失效 API Key 认证缓存")
}

func TestAdminService_UpdateUser_InvalidRole(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleUser}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	svc := &adminServiceImpl{
		userRepo:       repo,
		redeemCodeRepo: &redeemRepoStub{},
	}

	_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{
		Role: "sales",
	})
	require.Error(t, err)
	require.Nil(t, repo.lastUpdated)
}

type userStatusUpdateUserRepoStub struct {
	*userRepoStubForListUsers
	lastUpdated    *User
	lastUpdateMask UserUpdateFields
}

func (s *userStatusUpdateUserRepoStub) Update(_ context.Context, user *User, fields UserUpdateFields) error {
	if user == nil {
		return nil
	}
	clone := *user
	s.lastUpdated = &clone
	s.lastUpdateMask = fields
	if s.userRepoStubForListUsers != nil {
		s.userRepoStubForListUsers.user = &clone
		for i := range s.userRepoStubForListUsers.users {
			if s.userRepoStubForListUsers.users[i].ID == clone.ID {
				s.userRepoStubForListUsers.users[i] = clone
				break
			}
		}
	}
	return nil
}

func TestAdminService_UpdateUserStatusStoresUserStatus(t *testing.T) {
	repo := &userStatusUpdateUserRepoStub{userRepoStubForListUsers: &userRepoStubForListUsers{
		userRepoStub: userRepoStub{user: &User{ID: 42, Email: "pending@example.com", Status: StatusActive, Role: RoleUser}},
		users:        []User{{ID: 42, Email: "pending@example.com", Status: StatusActive, Role: RoleUser}},
	}}
	svc := &adminServiceImpl{userRepo: repo, groupRepo: &groupRepoStubForAdmin{listWithFiltersGroups: []Group{{ID: 1, Name: "default"}}}}

	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{
		Status: StatusPendingActivation,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, StatusPendingActivation, updated.Status)
	require.NotNil(t, repo.lastUpdated)
	require.Equal(t, StatusPendingActivation, repo.lastUpdated.Status)
	require.Equal(t, UserUpdateFields{Status: true}, repo.lastUpdateMask)
}

func TestAdminService_UpdateUserRejectsAdminPromotionWithDisabledUserStatus(t *testing.T) {
	repo := &userStatusUpdateUserRepoStub{
		userRepoStubForListUsers: &userRepoStubForListUsers{
			userRepoStub: userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleUser, Status: StatusActive}},
			users:        []User{{ID: 42, Email: "u@example.com", Role: RoleUser, Status: StatusActive}},
		},
	}
	svc := &adminServiceImpl{userRepo: repo}

	_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{
		Role:   RoleAdmin,
		Status: StatusDisabled,
	})
	require.Error(t, err)
	require.Nil(t, repo.lastUpdated)
}
