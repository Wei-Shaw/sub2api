//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type userRepoStub struct {
	user          *User
	getErr        error
	createErr     error
	deleteErr     error
	exists        bool
	existsErr     error
	nextID        int64
	created       []*User
	updated       []*User
	deletedIDs    []int64
	usersByEmail  map[string]*User
	getByEmailErr error
}

func (s *userRepoStub) Create(ctx context.Context, user *User) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.nextID != 0 && user.ID == 0 {
		user.ID = s.nextID
	}
	s.created = append(s.created, user)
	if s.usersByEmail == nil {
		s.usersByEmail = make(map[string]*User)
	}
	s.usersByEmail[user.Email] = user
	s.user = user
	return nil
}

func (s *userRepoStub) GetByID(ctx context.Context, id int64) (*User, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.user == nil {
		return nil, ErrUserNotFound
	}
	return s.user, nil
}

func (s *userRepoStub) GetByEmail(ctx context.Context, email string) (*User, error) {
	if s.getByEmailErr != nil {
		return nil, s.getByEmailErr
	}
	if s.usersByEmail != nil {
		if user, ok := s.usersByEmail[email]; ok {
			return user, nil
		}
	}
	if s.user != nil && s.user.Email == email {
		return s.user, nil
	}
	return nil, ErrUserNotFound
}

func (s *userRepoStub) GetFirstAdmin(ctx context.Context) (*User, error) {
	panic("unexpected GetFirstAdmin call")
}

func (s *userRepoStub) Update(ctx context.Context, user *User) error {
	s.updated = append(s.updated, user)
	if s.usersByEmail == nil {
		s.usersByEmail = make(map[string]*User)
	}
	s.usersByEmail[user.Email] = user
	s.user = user
	return nil
}

func (s *userRepoStub) Delete(ctx context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return s.deleteErr
}

func (s *userRepoStub) GetUserAvatar(ctx context.Context, userID int64) (*UserAvatar, error) {
	panic("unexpected GetUserAvatar call")
}

func (s *userRepoStub) UpsertUserAvatar(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected UpsertUserAvatar call")
}

func (s *userRepoStub) DeleteUserAvatar(ctx context.Context, userID int64) error {
	panic("unexpected DeleteUserAvatar call")
}

func (s *userRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *userRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *userRepoStub) GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs call")
}

func (s *userRepoStub) GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID call")
}

func (s *userRepoStub) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	panic("unexpected UpdateUserLastActiveAt call")
}

func (s *userRepoStub) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected UpdateBalance call")
}

func (s *userRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected DeductBalance call")
}

func (s *userRepoStub) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	panic("unexpected UpdateConcurrency call")
}

func (s *userRepoStub) BatchSetConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (s *userRepoStub) BatchAddConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }

func (s *userRepoStub) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	return s.exists, nil
}

func (s *userRepoStub) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups call")
}

func (s *userRepoStub) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups call")
}

func (s *userRepoStub) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected AddGroupToAllowedGroups call")
}

func (s *userRepoStub) ListUserAuthIdentities(ctx context.Context, userID int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities call")
}

func (s *userRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider call")
}

func (s *userRepoStub) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	panic("unexpected UpdateTotpSecret call")
}

func (s *userRepoStub) EnableTotp(ctx context.Context, userID int64) error {
	panic("unexpected EnableTotp call")
}

func (s *userRepoStub) DisableTotp(ctx context.Context, userID int64) error {
	panic("unexpected DisableTotp call")
}

type groupRepoStub struct {
	affectedUserIDs []int64
	deleteErr       error
	deleteCalls     []int64
}

func (s *groupRepoStub) Create(ctx context.Context, group *Group) error {
	panic("unexpected Create call")
}

func (s *groupRepoStub) GetByID(ctx context.Context, id int64) (*Group, error) {
	panic("unexpected GetByID call")
}

func (s *groupRepoStub) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	panic("unexpected GetByIDLite call")
}

func (s *groupRepoStub) Update(ctx context.Context, group *Group) error {
	panic("unexpected Update call")
}

func (s *groupRepoStub) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (s *groupRepoStub) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	s.deleteCalls = append(s.deleteCalls, id)
	return s.affectedUserIDs, s.deleteErr
}

func (s *groupRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *groupRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *groupRepoStub) ListActive(ctx context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}

func (s *groupRepoStub) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}

func (s *groupRepoStub) ExistsByName(ctx context.Context, name string) (bool, error) {
	panic("unexpected ExistsByName call")
}

func (s *groupRepoStub) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}

func (s *groupRepoStub) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}

func (s *groupRepoStub) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	panic("unexpected BindAccountsToGroup call")
}

func (s *groupRepoStub) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}

func (s *groupRepoStub) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return nil
}

type proxyRepoStub struct {
	deleteErr    error
	countErr     error
	accountCount int64
	deletedIDs   []int64
}

func (s *proxyRepoStub) Create(ctx context.Context, proxy *Proxy) error {
	panic("unexpected Create call")
}

func (s *proxyRepoStub) GetByID(ctx context.Context, id int64) (*Proxy, error) {
	panic("unexpected GetByID call")
}

func (s *proxyRepoStub) ListByIDs(ctx context.Context, ids []int64) ([]Proxy, error) {
	panic("unexpected ListByIDs call")
}

func (s *proxyRepoStub) Update(ctx context.Context, proxy *Proxy) error {
	panic("unexpected Update call")
}

func (s *proxyRepoStub) Delete(ctx context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return s.deleteErr
}

func (s *proxyRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *proxyRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *proxyRepoStub) ListActive(ctx context.Context) ([]Proxy, error) {
	panic("unexpected ListActive call")
}

func (s *proxyRepoStub) ListActiveWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	panic("unexpected ListActiveWithAccountCount call")
}

func (s *proxyRepoStub) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFiltersAndAccountCount call")
}

func (s *proxyRepoStub) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	panic("unexpected ExistsByHostPortAuth call")
}

func (s *proxyRepoStub) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	return s.accountCount, nil
}

func (s *proxyRepoStub) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error) {
	panic("unexpected ListAccountSummariesByProxyID call")
}

type redeemRepoStub struct {
	deleteErrByID map[int64]error
	deletedIDs    []int64
}

func (s *redeemRepoStub) Create(ctx context.Context, code *RedeemCode) error {
	panic("unexpected Create call")
}

func (s *redeemRepoStub) CreateBatch(ctx context.Context, codes []RedeemCode) error {
	panic("unexpected CreateBatch call")
}

func (s *redeemRepoStub) GetByID(ctx context.Context, id int64) (*RedeemCode, error) {
	panic("unexpected GetByID call")
}

func (s *redeemRepoStub) GetByCode(ctx context.Context, code string) (*RedeemCode, error) {
	panic("unexpected GetByCode call")
}

func (s *redeemRepoStub) Update(ctx context.Context, code *RedeemCode) error {
	panic("unexpected Update call")
}

func (s *redeemRepoStub) Delete(ctx context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	if s.deleteErrByID != nil {
		if err, ok := s.deleteErrByID[id]; ok {
			return err
		}
	}
	return nil
}

func (s *redeemRepoStub) Use(ctx context.Context, id, userID int64) error {
	panic("unexpected Use call")
}

func (s *redeemRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *redeemRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *redeemRepoStub) ListByUser(ctx context.Context, userID int64, limit int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
}

func (s *redeemRepoStub) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}

func (s *redeemRepoStub) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

type subscriptionInvalidateCall struct {
	userID  int64
	groupID int64
}

type billingCacheStub struct {
	invalidations chan subscriptionInvalidateCall
}

func newBillingCacheStub(buffer int) *billingCacheStub {
	return &billingCacheStub{invalidations: make(chan subscriptionInvalidateCall, buffer)}
}

func (s *billingCacheStub) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	panic("unexpected GetUserBalance call")
}

func (s *billingCacheStub) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	panic("unexpected SetUserBalance call")
}

func (s *billingCacheStub) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	panic("unexpected DeductUserBalance call")
}

func (s *billingCacheStub) InvalidateUserBalance(ctx context.Context, userID int64) error {
	panic("unexpected InvalidateUserBalance call")
}

func (s *billingCacheStub) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	panic("unexpected GetSubscriptionCache call")
}

func (s *billingCacheStub) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	panic("unexpected SetSubscriptionCache call")
}

func (s *billingCacheStub) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	panic("unexpected UpdateSubscriptionUsage call")
}

func (s *billingCacheStub) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	s.invalidations <- subscriptionInvalidateCall{userID: userID, groupID: groupID}
	return nil
}

func (s *billingCacheStub) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error) {
	panic("unexpected GetAPIKeyRateLimit call")
}
func (s *billingCacheStub) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error {
	panic("unexpected SetAPIKeyRateLimit call")
}
func (s *billingCacheStub) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	panic("unexpected UpdateAPIKeyRateLimitUsage call")
}
func (s *billingCacheStub) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	panic("unexpected InvalidateAPIKeyRateLimit call")
}

func waitForInvalidations(t *testing.T, ch <-chan subscriptionInvalidateCall, expected int) []subscriptionInvalidateCall {
	t.Helper()
	calls := make([]subscriptionInvalidateCall, 0, expected)
	timeout := time.After(2 * time.Second)
	for len(calls) < expected {
		select {
		case call := <-ch:
			calls = append(calls, call)
		case <-timeout:
			t.Fatalf("timeout waiting for %d invalidations, got %d", expected, len(calls))
		}
	}
	return calls
}

func TestAdminService_DeleteUser_Success(t *testing.T) {
	repo := &userRepoStub{user: &User{ID: 7, Role: RoleUser}}
	svc := &adminServiceImpl{userRepo: repo}

	err := svc.DeleteUser(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, []int64{7}, repo.deletedIDs)
}

func TestAdminService_DeleteUser_NotFound(t *testing.T) {
	repo := &userRepoStub{getErr: ErrUserNotFound}
	svc := &adminServiceImpl{userRepo: repo}

	err := svc.DeleteUser(context.Background(), 404)
	require.ErrorIs(t, err, ErrUserNotFound)
	require.Empty(t, repo.deletedIDs)
}

func TestAdminService_DeleteUser_AdminGuard(t *testing.T) {
	repo := &userRepoStub{user: &User{ID: 1, Role: RoleAdmin}}
	svc := &adminServiceImpl{userRepo: repo}

	err := svc.DeleteUser(context.Background(), 1)
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot delete admin user")
	require.Empty(t, repo.deletedIDs)
}

func TestAdminService_DeleteUser_DeleteError(t *testing.T) {
	deleteErr := errors.New("delete failed")
	repo := &userRepoStub{
		user:      &User{ID: 9, Role: RoleUser},
		deleteErr: deleteErr,
	}
	svc := &adminServiceImpl{userRepo: repo}

	err := svc.DeleteUser(context.Background(), 9)
	require.ErrorIs(t, err, deleteErr)
	require.Equal(t, []int64{9}, repo.deletedIDs)
}

func TestAdminService_DeleteGroup_Success_WithCacheInvalidation(t *testing.T) {
	cache := newBillingCacheStub(2)
	repo := &groupRepoStub{affectedUserIDs: []int64{11, 12}}
	svc := &adminServiceImpl{
		groupRepo:           repo,
		billingCacheService: &BillingCacheService{cache: cache},
	}

	err := svc.DeleteGroup(context.Background(), 5)
	require.NoError(t, err)
	require.Equal(t, []int64{5}, repo.deleteCalls)

	calls := waitForInvalidations(t, cache.invalidations, 2)
	require.ElementsMatch(t, []subscriptionInvalidateCall{
		{userID: 11, groupID: 5},
		{userID: 12, groupID: 5},
	}, calls)
}

func TestAdminService_DeleteGroup_NotFound(t *testing.T) {
	repo := &groupRepoStub{deleteErr: ErrGroupNotFound}
	svc := &adminServiceImpl{groupRepo: repo}

	err := svc.DeleteGroup(context.Background(), 99)
	require.ErrorIs(t, err, ErrGroupNotFound)
}

func TestAdminService_DeleteGroup_Error(t *testing.T) {
	deleteErr := errors.New("delete failed")
	repo := &groupRepoStub{deleteErr: deleteErr}
	svc := &adminServiceImpl{groupRepo: repo}

	err := svc.DeleteGroup(context.Background(), 42)
	require.ErrorIs(t, err, deleteErr)
}

func TestAdminService_DeleteProxy_Success(t *testing.T) {
	repo := &proxyRepoStub{}
	svc := &adminServiceImpl{proxyRepo: repo}

	err := svc.DeleteProxy(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, []int64{7}, repo.deletedIDs)
}

func TestAdminService_DeleteProxy_Idempotent(t *testing.T) {
	repo := &proxyRepoStub{}
	svc := &adminServiceImpl{proxyRepo: repo}

	err := svc.DeleteProxy(context.Background(), 404)
	require.NoError(t, err)
	require.Equal(t, []int64{404}, repo.deletedIDs)
}

func TestAdminService_DeleteProxy_InUse(t *testing.T) {
	repo := &proxyRepoStub{accountCount: 2}
	svc := &adminServiceImpl{proxyRepo: repo}

	err := svc.DeleteProxy(context.Background(), 77)
	require.ErrorIs(t, err, ErrProxyInUse)
	require.Empty(t, repo.deletedIDs)
}

func TestAdminService_DeleteProxy_Error(t *testing.T) {
	deleteErr := errors.New("delete failed")
	repo := &proxyRepoStub{deleteErr: deleteErr}
	svc := &adminServiceImpl{proxyRepo: repo}

	err := svc.DeleteProxy(context.Background(), 33)
	require.ErrorIs(t, err, deleteErr)
}

func TestAdminService_DeleteRedeemCode_Success(t *testing.T) {
	repo := &redeemRepoStub{}
	svc := &adminServiceImpl{redeemCodeRepo: repo}

	err := svc.DeleteRedeemCode(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, []int64{10}, repo.deletedIDs)
}

func TestAdminService_DeleteRedeemCode_Idempotent(t *testing.T) {
	repo := &redeemRepoStub{}
	svc := &adminServiceImpl{redeemCodeRepo: repo}

	err := svc.DeleteRedeemCode(context.Background(), 999)
	require.NoError(t, err)
	require.Equal(t, []int64{999}, repo.deletedIDs)
}

func TestAdminService_DeleteRedeemCode_Error(t *testing.T) {
	deleteErr := errors.New("delete failed")
	repo := &redeemRepoStub{deleteErrByID: map[int64]error{1: deleteErr}}
	svc := &adminServiceImpl{redeemCodeRepo: repo}

	err := svc.DeleteRedeemCode(context.Background(), 1)
	require.ErrorIs(t, err, deleteErr)
	require.Equal(t, []int64{1}, repo.deletedIDs)
}

func TestAdminService_BatchDeleteRedeemCodes_Success(t *testing.T) {
	repo := &redeemRepoStub{}
	svc := &adminServiceImpl{redeemCodeRepo: repo}

	deleted, err := svc.BatchDeleteRedeemCodes(context.Background(), []int64{1, 2, 3})
	require.NoError(t, err)
	require.Equal(t, int64(3), deleted)
	require.Equal(t, []int64{1, 2, 3}, repo.deletedIDs)
}

func TestAdminService_BatchDeleteRedeemCodes_PartialFailures(t *testing.T) {
	repo := &redeemRepoStub{
		deleteErrByID: map[int64]error{
			2: errors.New("db error"),
		},
	}
	svc := &adminServiceImpl{redeemCodeRepo: repo}

	deleted, err := svc.BatchDeleteRedeemCodes(context.Background(), []int64{1, 2, 3})
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	require.Equal(t, []int64{1, 2, 3}, repo.deletedIDs)
}
