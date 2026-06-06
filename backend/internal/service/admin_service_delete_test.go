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
REDACTED

func (s *userRepoStub) Create(ctx context.Context, user *User) error {
	if s.createErr != nil {
		return s.createErr
REDACTED
	if s.nextID != 0 && user.ID == 0 {
		user.ID = s.nextID
REDACTED
	s.created = append(s.created, user)
	if s.usersByEmail == nil {
		s.usersByEmail = make(map[string]*User)
REDACTED
	s.usersByEmail[user.Email] = user
	s.user = user
	return nil
REDACTED

func (s *userRepoStub) GetByID(ctx context.Context, id int64) (*User, error) {
	if s.getErr != nil {
		return nil, s.getErr
REDACTED
	if s.user == nil {
		return nil, ErrUserNotFound
REDACTED
	return s.user, nil
REDACTED

func (s *userRepoStub) GetByEmail(ctx context.Context, email string) (*User, error) {
	if s.getByEmailErr != nil {
		return nil, s.getByEmailErr
REDACTED
	if s.usersByEmail != nil {
		if user, ok := s.usersByEmail[email]; ok {
			return user, nil
	REDACTED
REDACTED
	if s.user != nil && s.user.Email == email {
		return s.user, nil
REDACTED
	return nil, ErrUserNotFound
REDACTED

func (s *userRepoStub) GetFirstAdmin(ctx context.Context) (*User, error) {
	panic("unexpected GetFirstAdmin call")
REDACTED

func (s *userRepoStub) Update(ctx context.Context, user *User) error {
	s.updated = append(s.updated, user)
	if s.usersByEmail == nil {
		s.usersByEmail = make(map[string]*User)
REDACTED
	s.usersByEmail[user.Email] = user
	s.user = user
	return nil
REDACTED

func (s *userRepoStub) Delete(ctx context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return s.deleteErr
REDACTED

func (s *userRepoStub) GetUserAvatar(ctx context.Context, userID int64) (*UserAvatar, error) {
	panic("unexpected GetUserAvatar call")
REDACTED

func (s *userRepoStub) UpsertUserAvatar(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected UpsertUserAvatar call")
REDACTED

func (s *userRepoStub) DeleteUserAvatar(ctx context.Context, userID int64) error {
	panic("unexpected DeleteUserAvatar call")
REDACTED

func (s *userRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (s *userRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
REDACTED

func (s *userRepoStub) GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs call")
REDACTED

func (s *userRepoStub) GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID call")
REDACTED

func (s *userRepoStub) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	panic("unexpected UpdateUserLastActiveAt call")
REDACTED

func (s *userRepoStub) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected UpdateBalance call")
REDACTED

func (s *userRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected DeductBalance call")
REDACTED

func (s *userRepoStub) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	panic("unexpected UpdateConcurrency call")
REDACTED

func (s *userRepoStub) BatchSetConcurrency(context.Context, []int64, int) (int, error) { return 0, nil REDACTED
func (s *userRepoStub) BatchAddConcurrency(context.Context, []int64, int) (int, error) { return 0, nil REDACTED

func (s *userRepoStub) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
REDACTED
	return s.exists, nil
REDACTED

func (s *userRepoStub) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups call")
REDACTED

func (s *userRepoStub) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups call")
REDACTED

func (s *userRepoStub) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected AddGroupToAllowedGroups call")
REDACTED

func (s *userRepoStub) ListUserAuthIdentities(ctx context.Context, userID int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities call")
REDACTED

func (s *userRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider call")
REDACTED

func (s *userRepoStub) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	panic("unexpected UpdateTotpSecret call")
REDACTED

func (s *userRepoStub) EnableTotp(ctx context.Context, userID int64) error {
	panic("unexpected EnableTotp call")
REDACTED

func (s *userRepoStub) DisableTotp(ctx context.Context, userID int64) error {
	panic("unexpected DisableTotp call")
REDACTED

func (s *userRepoStub) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	return s.GetByID(ctx, id)
REDACTED

type groupRepoStub struct {
	affectedUserIDs []int64
	deleteErr       error
	deleteCalls     []int64
REDACTED

func (s *groupRepoStub) Create(ctx context.Context, group *Group) error {
	panic("unexpected Create call")
REDACTED

func (s *groupRepoStub) GetByID(ctx context.Context, id int64) (*Group, error) {
	panic("unexpected GetByID call")
REDACTED

func (s *groupRepoStub) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	panic("unexpected GetByIDLite call")
REDACTED

func (s *groupRepoStub) Update(ctx context.Context, group *Group) error {
	panic("unexpected Update call")
REDACTED

func (s *groupRepoStub) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
REDACTED

func (s *groupRepoStub) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	s.deleteCalls = append(s.deleteCalls, id)
	return s.affectedUserIDs, s.deleteErr
REDACTED

func (s *groupRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (s *groupRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
REDACTED

func (s *groupRepoStub) ListActive(ctx context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
REDACTED

func (s *groupRepoStub) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
REDACTED

func (s *groupRepoStub) ExistsByName(ctx context.Context, name string) (bool, error) {
	panic("unexpected ExistsByName call")
REDACTED

func (s *groupRepoStub) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
REDACTED

func (s *groupRepoStub) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
REDACTED

func (s *groupRepoStub) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	panic("unexpected BindAccountsToGroup call")
REDACTED

func (s *groupRepoStub) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
REDACTED

func (s *groupRepoStub) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return nil
REDACTED

type deleteGroupAPIKeyRepoStub struct {
	apiKeyRepoStubForGroupUpdate
	keys         []string
	listErr      error
	listGroupIDs []int64
REDACTED

func (s *deleteGroupAPIKeyRepoStub) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	s.listGroupIDs = append(s.listGroupIDs, groupID)
	if s.listErr != nil {
		return nil, s.listErr
REDACTED
	return s.keys, nil
REDACTED

type proxyRepoStub struct {
	deleteErr    error
	countErr     error
	accountCount int64
	deletedIDs   []int64
REDACTED

func (s *proxyRepoStub) Create(ctx context.Context, proxy *Proxy) error {
	panic("unexpected Create call")
REDACTED

func (s *proxyRepoStub) GetByID(ctx context.Context, id int64) (*Proxy, error) {
	panic("unexpected GetByID call")
REDACTED

func (s *proxyRepoStub) ListByIDs(ctx context.Context, ids []int64) ([]Proxy, error) {
	panic("unexpected ListByIDs call")
REDACTED

func (s *proxyRepoStub) Update(ctx context.Context, proxy *Proxy) error {
	panic("unexpected Update call")
REDACTED

func (s *proxyRepoStub) Delete(ctx context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return s.deleteErr
REDACTED

func (s *proxyRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (s *proxyRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
REDACTED

func (s *proxyRepoStub) ListActive(ctx context.Context) ([]Proxy, error) {
	panic("unexpected ListActive call")
REDACTED

func (s *proxyRepoStub) ListActiveWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	panic("unexpected ListActiveWithAccountCount call")
REDACTED

func (s *proxyRepoStub) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFiltersAndAccountCount call")
REDACTED

func (s *proxyRepoStub) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	panic("unexpected ExistsByHostPortAuth call")
REDACTED

func (s *proxyRepoStub) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	if s.countErr != nil {
		return 0, s.countErr
REDACTED
	return s.accountCount, nil
REDACTED

func (s *proxyRepoStub) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error) {
	panic("unexpected ListAccountSummariesByProxyID call")
REDACTED

type redeemRepoStub struct {
	deleteErrByID map[int64]error
	deletedIDs    []int64

	batchUpdateIDs    []int64
	batchUpdateFields RedeemCodeBatchUpdateFields
	batchUpdateResult int64
	batchUpdateErr    error
	batchUpdateCalled bool
REDACTED

func (s *redeemRepoStub) Create(ctx context.Context, code *RedeemCode) error {
	panic("unexpected Create call")
REDACTED

func (s *redeemRepoStub) CreateBatch(ctx context.Context, codes []RedeemCode) error {
	panic("unexpected CreateBatch call")
REDACTED

func (s *redeemRepoStub) GetByID(ctx context.Context, id int64) (*RedeemCode, error) {
	panic("unexpected GetByID call")
REDACTED

func (s *redeemRepoStub) GetByCode(ctx context.Context, code string) (*RedeemCode, error) {
	panic("unexpected GetByCode call")
REDACTED

func (s *redeemRepoStub) Update(ctx context.Context, code *RedeemCode) error {
	panic("unexpected Update call")
REDACTED

func (s *redeemRepoStub) BatchUpdate(ctx context.Context, ids []int64, fields RedeemCodeBatchUpdateFields) (int64, error) {
	s.batchUpdateCalled = true
	s.batchUpdateIDs = append([]int64(nil), ids...)
	s.batchUpdateFields = fields
	if s.batchUpdateErr != nil {
		return 0, s.batchUpdateErr
REDACTED
	if s.batchUpdateResult != 0 {
		return s.batchUpdateResult, nil
REDACTED
	return int64(len(ids)), nil
REDACTED

func (s *redeemRepoStub) Delete(ctx context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	if s.deleteErrByID != nil {
		if err, ok := s.deleteErrByID[id]; ok {
			return err
	REDACTED
REDACTED
	return nil
REDACTED

func (s *redeemRepoStub) Use(ctx context.Context, id, userID int64) error {
	panic("unexpected Use call")
REDACTED

func (s *redeemRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (s *redeemRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
REDACTED

func (s *redeemRepoStub) ListByUser(ctx context.Context, userID int64, limit int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
REDACTED

func (s *redeemRepoStub) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
REDACTED

func (s *redeemRepoStub) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
REDACTED

type subscriptionInvalidateCall struct {
	userID  int64
	groupID int64
REDACTED

type billingCacheStub struct {
	invalidations chan subscriptionInvalidateCall
REDACTED

func newBillingCacheStub(buffer int) *billingCacheStub {
	return &billingCacheStub{invalidations: make(chan subscriptionInvalidateCall, buffer)REDACTED
REDACTED

func (s *billingCacheStub) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	panic("unexpected GetUserBalance call")
REDACTED

func (s *billingCacheStub) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	panic("unexpected SetUserBalance call")
REDACTED

func (s *billingCacheStub) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	panic("unexpected DeductUserBalance call")
REDACTED

func (s *billingCacheStub) InvalidateUserBalance(ctx context.Context, userID int64) error {
	panic("unexpected InvalidateUserBalance call")
REDACTED

func (s *billingCacheStub) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	panic("unexpected GetSubscriptionCache call")
REDACTED

func (s *billingCacheStub) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	panic("unexpected SetSubscriptionCache call")
REDACTED

func (s *billingCacheStub) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	panic("unexpected UpdateSubscriptionUsage call")
REDACTED

func (s *billingCacheStub) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	s.invalidations <- subscriptionInvalidateCall{userID: userID, groupID: groupIDREDACTED
	return nil
REDACTED

func (s *billingCacheStub) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error) {
	panic("unexpected GetAPIKeyRateLimit call")
REDACTED
func (s *billingCacheStub) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error {
	panic("unexpected SetAPIKeyRateLimit call")
REDACTED
func (s *billingCacheStub) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	panic("unexpected UpdateAPIKeyRateLimitUsage call")
REDACTED
func (s *billingCacheStub) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	panic("unexpected InvalidateAPIKeyRateLimit call")
REDACTED

func (s *billingCacheStub) GetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) (*UserPlatformQuotaCacheEntry, bool, error) {
	panic("unexpected GetUserPlatformQuotaCache call")
REDACTED

func (s *billingCacheStub) SetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string, entry *UserPlatformQuotaCacheEntry, ttl time.Duration) error {
	panic("unexpected SetUserPlatformQuotaCache call")
REDACTED

func (s *billingCacheStub) DeleteUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) error {
	panic("unexpected DeleteUserPlatformQuotaCache call")
REDACTED

func (s *billingCacheStub) IncrUserPlatformQuotaUsageCache(ctx context.Context, userID int64, platform string, cost float64, ttl time.Duration, markDirty bool) error {
	panic("unexpected IncrUserPlatformQuotaUsageCache call")
REDACTED

func (s *billingCacheStub) PopDirtyUserPlatformQuotaKeys(ctx context.Context, n int) ([]UserPlatformQuotaKey, error) {
	panic("unexpected PopDirtyUserPlatformQuotaKeys call")
REDACTED

func (s *billingCacheStub) ReaddDirtyUserPlatformQuotaKeys(ctx context.Context, keys []UserPlatformQuotaKey) error {
	panic("unexpected ReaddDirtyUserPlatformQuotaKeys call")
REDACTED

func (s *billingCacheStub) BatchGetUserPlatformQuotaCache(ctx context.Context, keys []UserPlatformQuotaKey) ([]*UserPlatformQuotaCacheEntry, error) {
	panic("unexpected BatchGetUserPlatformQuotaCache call")
REDACTED

func waitForInvalidations(t *testing.T, ch <-chan subscriptionInvalidateCall, expected int) []subscriptionInvalidateCall {
REDACTED
	calls := make([]subscriptionInvalidateCall, 0, expected)
	timeout := time.After(2 * time.Second)
	for len(calls) < expected {
		select {
		case call := <-ch:
			calls = append(calls, call)
		case <-timeout:
			t.Fatalf("timeout waiting for %d invalidations, got %d", expected, len(calls))
	REDACTED
REDACTED
	return calls
REDACTED

func TestAdminService_DeleteUser_Success(t *testing.T) {
	repo := &userRepoStub{user: &User{ID: 7, Role: RoleUserREDACTEDREDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	err := svc.DeleteUser(context.Background(), 7)
REDACTED
	require.Equal(t, []int64{7REDACTED, repo.deletedIDs)
REDACTED

func TestAdminService_DeleteUser_DeletesOwnedAPIKeys(t *testing.T) {
	repo := &userRepoStub{user: &User{ID: 7, Role: RoleUserREDACTEDREDACTED
	apiKeyRepo := &apiKeyRepoStub{
		allowListByUserID: true,
		listByUserIDKeys: []APIKey{
			{ID: 11, UserID: 7, Key: "sk-user-1"REDACTED,
			{ID: 12, UserID: 7, Key: "sk-user-2"REDACTED,
	REDACTED,
REDACTED
	invalidator := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{
		userRepo:             repo,
		apiKeyRepo:           apiKeyRepo,
		authCacheInvalidator: invalidator,
REDACTED

	err := svc.DeleteUser(context.Background(), 7)
REDACTED
	require.Equal(t, []int64{7REDACTED, repo.deletedIDs)
	require.Equal(t, []int64{7REDACTED, apiKeyRepo.listByUserIDCalls)
	require.Equal(t, []int64{11, 12REDACTED, apiKeyRepo.deletedIDs)
	require.ElementsMatch(t, []string{"sk-user-1", "sk-user-2"REDACTED, invalidator.keys)
	require.Equal(t, []int64{7REDACTED, invalidator.userIDs)
REDACTED

func TestAdminService_DeleteUser_NotFound(t *testing.T) {
	repo := &userRepoStub{getErr: ErrUserNotFoundREDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	err := svc.DeleteUser(context.Background(), 404)
	require.ErrorIs(t, err, ErrUserNotFound)
	require.Empty(t, repo.deletedIDs)
REDACTED

func TestAdminService_DeleteUser_AdminGuard(t *testing.T) {
	repo := &userRepoStub{user: &User{ID: 1, Role: RoleAdminREDACTEDREDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	err := svc.DeleteUser(context.Background(), 1)
REDACTED
	require.ErrorContains(t, err, "cannot delete admin user")
	require.Empty(t, repo.deletedIDs)
REDACTED

func TestAdminService_DeleteUser_DeleteError(t *testing.T) {
	deleteErr := errors.New("delete failed")
	repo := &userRepoStub{
		user:      &User{ID: 9, Role: RoleUserREDACTED,
		deleteErr: deleteErr,
REDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	err := svc.DeleteUser(context.Background(), 9)
	require.ErrorIs(t, err, deleteErr)
	require.Equal(t, []int64{9REDACTED, repo.deletedIDs)
REDACTED

func TestAdminService_DeleteGroup_Success_WithCacheInvalidation(t *testing.T) {
	cache := newBillingCacheStub(2)
	repo := &groupRepoStub{affectedUserIDs: []int64{11, 12REDACTEDREDACTED
	svc := &adminServiceImpl{
		groupRepo:           repo,
		billingCacheService: &BillingCacheService{cache: cacheREDACTED,
REDACTED

	err := svc.DeleteGroup(context.Background(), 5)
REDACTED
	require.Equal(t, []int64{5REDACTED, repo.deleteCalls)

	calls := waitForInvalidations(t, cache.invalidations, 2)
	require.ElementsMatch(t, []subscriptionInvalidateCall{
		{userID: 11, groupID: 5REDACTED,
		{userID: 12, groupID: 5REDACTED,
REDACTED, calls)
REDACTED

func TestAdminService_DeleteGroup_InvalidatesAuthCacheForBoundKeys(t *testing.T) {
	repo := &groupRepoStub{REDACTED
	apiKeyRepo := &deleteGroupAPIKeyRepoStub{keys: []string{"k1", "k2"REDACTEDREDACTED
	invalidator := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{
		groupRepo:            repo,
		apiKeyRepo:           apiKeyRepo,
		authCacheInvalidator: invalidator,
REDACTED

	err := svc.DeleteGroup(context.Background(), 5)
REDACTED
	require.Equal(t, []int64{5REDACTED, repo.deleteCalls)
	require.Equal(t, []int64{5REDACTED, apiKeyRepo.listGroupIDs)
	require.Equal(t, []string{"k1", "k2"REDACTED, invalidator.keys)
REDACTED

func TestAdminService_DeleteGroup_NotFound(t *testing.T) {
	repo := &groupRepoStub{deleteErr: ErrGroupNotFoundREDACTED
	svc := &adminServiceImpl{groupRepo: repoREDACTED

	err := svc.DeleteGroup(context.Background(), 99)
	require.ErrorIs(t, err, ErrGroupNotFound)
REDACTED

func TestAdminService_DeleteGroup_Error(t *testing.T) {
	deleteErr := errors.New("delete failed")
	repo := &groupRepoStub{deleteErr: deleteErrREDACTED
	svc := &adminServiceImpl{groupRepo: repoREDACTED

	err := svc.DeleteGroup(context.Background(), 42)
	require.ErrorIs(t, err, deleteErr)
REDACTED

func TestAdminService_DeleteProxy_Success(t *testing.T) {
	repo := &proxyRepoStub{REDACTED
	svc := &adminServiceImpl{proxyRepo: repoREDACTED

	err := svc.DeleteProxy(context.Background(), 7)
REDACTED
	require.Equal(t, []int64{7REDACTED, repo.deletedIDs)
REDACTED

func TestAdminService_DeleteProxy_Idempotent(t *testing.T) {
	repo := &proxyRepoStub{REDACTED
	svc := &adminServiceImpl{proxyRepo: repoREDACTED

	err := svc.DeleteProxy(context.Background(), 404)
REDACTED
	require.Equal(t, []int64{404REDACTED, repo.deletedIDs)
REDACTED

func TestAdminService_DeleteProxy_InUse(t *testing.T) {
	repo := &proxyRepoStub{accountCount: 2REDACTED
	svc := &adminServiceImpl{proxyRepo: repoREDACTED

	err := svc.DeleteProxy(context.Background(), 77)
	require.ErrorIs(t, err, ErrProxyInUse)
	require.Empty(t, repo.deletedIDs)
REDACTED

func TestAdminService_DeleteProxy_Error(t *testing.T) {
	deleteErr := errors.New("delete failed")
	repo := &proxyRepoStub{deleteErr: deleteErrREDACTED
	svc := &adminServiceImpl{proxyRepo: repoREDACTED

	err := svc.DeleteProxy(context.Background(), 33)
	require.ErrorIs(t, err, deleteErr)
REDACTED

func TestAdminService_DeleteRedeemCode_Success(t *testing.T) {
	repo := &redeemRepoStub{REDACTED
	svc := &adminServiceImpl{redeemCodeRepo: repoREDACTED

	err := svc.DeleteRedeemCode(context.Background(), 10)
REDACTED
	require.Equal(t, []int64{10REDACTED, repo.deletedIDs)
REDACTED

func TestAdminService_DeleteRedeemCode_Idempotent(t *testing.T) {
	repo := &redeemRepoStub{REDACTED
	svc := &adminServiceImpl{redeemCodeRepo: repoREDACTED

	err := svc.DeleteRedeemCode(context.Background(), 999)
REDACTED
	require.Equal(t, []int64{999REDACTED, repo.deletedIDs)
REDACTED

func TestAdminService_DeleteRedeemCode_Error(t *testing.T) {
	deleteErr := errors.New("delete failed")
	repo := &redeemRepoStub{deleteErrByID: map[int64]error{1: deleteErrREDACTEDREDACTED
	svc := &adminServiceImpl{redeemCodeRepo: repoREDACTED

	err := svc.DeleteRedeemCode(context.Background(), 1)
	require.ErrorIs(t, err, deleteErr)
	require.Equal(t, []int64{1REDACTED, repo.deletedIDs)
REDACTED

func TestAdminService_BatchDeleteRedeemCodes_Success(t *testing.T) {
	repo := &redeemRepoStub{REDACTED
	svc := &adminServiceImpl{redeemCodeRepo: repoREDACTED

	deleted, err := svc.BatchDeleteRedeemCodes(context.Background(), []int64{1, 2, 3REDACTED)
REDACTED
	require.Equal(t, int64(3), deleted)
	require.Equal(t, []int64{1, 2, 3REDACTED, repo.deletedIDs)
REDACTED

func TestAdminService_BatchDeleteRedeemCodes_PartialFailures(t *testing.T) {
	repo := &redeemRepoStub{
		deleteErrByID: map[int64]error{
			2: errors.New("db error"),
	REDACTED,
REDACTED
	svc := &adminServiceImpl{redeemCodeRepo: repoREDACTED

	deleted, err := svc.BatchDeleteRedeemCodes(context.Background(), []int64{1, 2, 3REDACTED)
REDACTED
	require.Equal(t, int64(2), deleted)
	require.Equal(t, []int64{1, 2, 3REDACTED, repo.deletedIDs)
REDACTED
