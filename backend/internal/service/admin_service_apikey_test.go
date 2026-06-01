//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// userRepoStubForGroupUpdate implements UserRepository for AdminUpdateAPIKeyGroupID tests.
type userRepoStubForGroupUpdate struct {
	addGroupErr    error
	addGroupCalled bool
	addedUserID    int64
	addedGroupID   int64
REDACTED

func (s *userRepoStubForGroupUpdate) AddGroupToAllowedGroups(_ context.Context, userID int64, groupID int64) error {
	s.addGroupCalled = true
	s.addedUserID = userID
	s.addedGroupID = groupID
	return s.addGroupErr
REDACTED

func (s *userRepoStubForGroupUpdate) Create(context.Context, *User) error { panic("unexpected") REDACTED
func (s *userRepoStubForGroupUpdate) GetByID(context.Context, int64) (*User, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) GetByEmail(context.Context, string) (*User, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) GetFirstAdmin(context.Context) (*User, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) Update(context.Context, *User) error { panic("unexpected") REDACTED
func (s *userRepoStubForGroupUpdate) Delete(context.Context, int64) error { panic("unexpected") REDACTED
func (s *userRepoStubForGroupUpdate) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) UpdateBalance(context.Context, int64, float64) error {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) DeductBalance(context.Context, int64, float64) error {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) UpdateConcurrency(context.Context, int64, int) error {
	panic("unexpected")
REDACTED

func (s *userRepoStubForGroupUpdate) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
REDACTED
func (s *userRepoStubForGroupUpdate) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
REDACTED
func (s *userRepoStubForGroupUpdate) ExistsByEmail(context.Context, string) (bool, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) UpdateTotpSecret(context.Context, int64, *string) error {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) EnableTotp(context.Context, int64) error  { panic("unexpected") REDACTED
func (s *userRepoStubForGroupUpdate) DisableTotp(context.Context, int64) error { panic("unexpected") REDACTED
func (s *userRepoStubForGroupUpdate) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	panic("unexpected GetByIDIncludeDeleted call")
REDACTED
func (s *userRepoStubForGroupUpdate) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected")
REDACTED

func (s *userRepoStubForGroupUpdate) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected")
REDACTED

func (s *userRepoStubForGroupUpdate) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	panic("unexpected")
REDACTED
func (s *userRepoStubForGroupUpdate) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected")
REDACTED

// apiKeyRepoStubForGroupUpdate implements APIKeyRepository for AdminUpdateAPIKeyGroupID tests.
type apiKeyRepoStubForGroupUpdate struct {
	key       *APIKey
	getErr    error
	updateErr error
	updated   *APIKey // captures what was passed to Update
REDACTED

func (s *apiKeyRepoStubForGroupUpdate) GetByID(_ context.Context, _ int64) (*APIKey, error) {
	if s.getErr != nil {
		return nil, s.getErr
REDACTED
	clone := *s.key
	return &clone, nil
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) Update(_ context.Context, key *APIKey) error {
	if s.updateErr != nil {
		return s.updateErr
REDACTED
	clone := *key
	s.updated = &clone
	return nil
REDACTED

// Unused methods – panic on unexpected call.
func (s *apiKeyRepoStubForGroupUpdate) Create(context.Context, *APIKey) error { panic("unexpected") REDACTED
func (s *apiKeyRepoStubForGroupUpdate) GetKeyAndOwnerID(context.Context, int64) (string, int64, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) GetByKey(context.Context, string) (*APIKey, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) GetByKeyForAuth(context.Context, string) (*APIKey, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) Delete(context.Context, int64) error { panic("unexpected") REDACTED
func (s *apiKeyRepoStubForGroupUpdate) DeleteWithAudit(context.Context, int64) error {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) VerifyOwnership(context.Context, int64, []int64) ([]int64, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) CountByUserID(context.Context, int64) (int64, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) ExistsByKey(context.Context, string) (bool, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) SearchAPIKeys(context.Context, int64, string, int) ([]APIKey, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) ClearGroupIDByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) CountByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) ListKeysByUserID(context.Context, int64) ([]string, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) ListKeysByGroupID(context.Context, int64) ([]string, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) IncrementQuotaUsed(context.Context, int64, float64) (float64, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) UpdateLastUsed(context.Context, int64, time.Time) error {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) IncrementRateLimitUsage(context.Context, int64, float64) error {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) ResetRateLimitWindows(context.Context, int64) error {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	panic("unexpected")
REDACTED
func (s *apiKeyRepoStubForGroupUpdate) UpdateGroupIDByUserAndGroup(context.Context, int64, int64, int64) (int64, error) {
	panic("unexpected")
REDACTED

// groupRepoStubForGroupUpdate implements GroupRepository for AdminUpdateAPIKeyGroupID tests.
type groupRepoStubForGroupUpdate struct {
	group          *Group
	getErr         error
	lastGetByIDArg int64
REDACTED

func (s *groupRepoStubForGroupUpdate) GetByID(_ context.Context, id int64) (*Group, error) {
	s.lastGetByIDArg = id
	if s.getErr != nil {
		return nil, s.getErr
REDACTED
	clone := *s.group
	return &clone, nil
REDACTED

// Unused methods – panic on unexpected call.
func (s *groupRepoStubForGroupUpdate) Create(context.Context, *Group) error { panic("unexpected") REDACTED
func (s *groupRepoStubForGroupUpdate) GetByIDLite(context.Context, int64) (*Group, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) Update(context.Context, *Group) error { panic("unexpected") REDACTED
func (s *groupRepoStubForGroupUpdate) Delete(context.Context, int64) error  { panic("unexpected") REDACTED
func (s *groupRepoStubForGroupUpdate) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) ListActive(context.Context) ([]Group, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected")
REDACTED
func (s *groupRepoStubForGroupUpdate) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("unexpected")
REDACTED

type userSubRepoStubForGroupUpdate struct {
	userSubRepoNoop
	getActiveSub  *UserSubscription
	getActiveErr  error
	called        bool
	calledUserID  int64
	calledGroupID int64
REDACTED

func (s *userSubRepoStubForGroupUpdate) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	s.called = true
	s.calledUserID = userID
	s.calledGroupID = groupID
	if s.getActiveErr != nil {
		return nil, s.getActiveErr
REDACTED
	if s.getActiveSub == nil {
		return nil, ErrSubscriptionNotFound
REDACTED
	clone := *s.getActiveSub
	return &clone, nil
REDACTED

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestAdminService_AdminUpdateAPIKeyGroupID_KeyNotFound(t *testing.T) {
	repo := &apiKeyRepoStubForGroupUpdate{getErr: ErrAPIKeyNotFoundREDACTED
	svc := &adminServiceImpl{apiKeyRepo: repoREDACTED

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 999, int64Ptr(1))
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_NilGroupID_NoOp(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: int64Ptr(5)REDACTED
	repo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	svc := &adminServiceImpl{apiKeyRepo: repoREDACTED

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, nil)
REDACTED
	require.Equal(t, int64(1), got.APIKey.ID)
	// Update should NOT have been called (updated stays nil)
	require.Nil(t, repo.updated)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_Unbind(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: int64Ptr(5), Group: &Group{ID: 5, Name: "Old"REDACTEDREDACTED
	repo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	cache := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cacheREDACTED

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(0))
REDACTED
	require.Nil(t, got.APIKey.GroupID, "group_id should be nil after unbind")
	require.Nil(t, got.APIKey.Group, "group object should be nil after unbind")
	require.NotNil(t, repo.updated, "Update should have been called")
	require.Nil(t, repo.updated.GroupID)
	require.Equal(t, []string{"sk-test"REDACTED, cache.keys, "cache should be invalidated")
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_BindActiveGroup(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: nilREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Pro", Status: StatusActiveREDACTEDREDACTED
	cache := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, authCacheInvalidator: cacheREDACTED

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
REDACTED
	require.NotNil(t, got.APIKey.GroupID)
	require.Equal(t, int64(10), *got.APIKey.GroupID)
	require.Equal(t, int64(10), *apiKeyRepo.updated.GroupID)
	require.Equal(t, []string{"sk-test"REDACTED, cache.keys)
	// M3: verify correct group ID was passed to repo
	require.Equal(t, int64(10), groupRepo.lastGetByIDArg)
	// C1 fix: verify Group object is populated
	require.NotNil(t, got.APIKey.Group)
	require.Equal(t, "Pro", got.APIKey.Group.Name)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_SameGroup_Idempotent(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: int64Ptr(10), Group: &Group{ID: 10, Name: "Pro"REDACTEDREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Pro", Status: StatusActiveREDACTEDREDACTED
	cache := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, authCacheInvalidator: cacheREDACTED

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
REDACTED
	require.NotNil(t, got.APIKey.GroupID)
	require.Equal(t, int64(10), *got.APIKey.GroupID)
	// Update is still called (current impl doesn't short-circuit on same group)
	require.NotNil(t, apiKeyRepo.updated)
	require.Equal(t, []string{"sk-test"REDACTED, cache.keys)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_GroupNotFound(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test"REDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{getErr: ErrGroupNotFoundREDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepoREDACTED

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(99))
	require.ErrorIs(t, err, ErrGroupNotFound)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_GroupNotActive(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test"REDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 5, Status: StatusDisabledREDACTEDREDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepoREDACTED

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(5))
REDACTED
	require.Equal(t, "GROUP_NOT_ACTIVE", infraerrors.Reason(err))
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_UpdateFails(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: int64Ptr(3)REDACTED
	repo := &apiKeyRepoStubForGroupUpdate{key: existing, updateErr: errors.New("db write error")REDACTED
	svc := &adminServiceImpl{apiKeyRepo: repoREDACTED

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(0))
REDACTED
	require.Contains(t, err.Error(), "update api key")
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_NegativeGroupID(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test"REDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepoREDACTED

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(-5))
REDACTED
	require.Equal(t, "INVALID_GROUP_ID", infraerrors.Reason(err))
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_PointerIsolation(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: nilREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Pro", Status: StatusActiveREDACTEDREDACTED
	cache := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, authCacheInvalidator: cacheREDACTED

	inputGID := int64(10)
	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, &inputGID)
REDACTED
	require.NotNil(t, got.APIKey.GroupID)
	// Mutating the input pointer must NOT affect the stored value
	inputGID = 999
	require.Equal(t, int64(10), *got.APIKey.GroupID)
	require.Equal(t, int64(10), *apiKeyRepo.updated.GroupID)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_NilCacheInvalidator(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test"REDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 7, Status: StatusActiveREDACTEDREDACTED
	// authCacheInvalidator is nil – should not panic
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepoREDACTED

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(7))
REDACTED
	require.NotNil(t, got.APIKey.GroupID)
	require.Equal(t, int64(7), *got.APIKey.GroupID)
REDACTED

// ---------------------------------------------------------------------------
// Tests: AllowedGroup auto-sync
// ---------------------------------------------------------------------------

func TestAdminService_AdminUpdateAPIKeyGroupID_ExclusiveGroup_AddsAllowedGroup(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nilREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Exclusive", Status: StatusActive, IsExclusive: true, SubscriptionType: SubscriptionTypeStandardREDACTEDREDACTED
	userRepo := &userRepoStubForGroupUpdate{REDACTED
	cache := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, userRepo: userRepo, authCacheInvalidator: cacheREDACTED

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
REDACTED
	require.NotNil(t, got.APIKey.GroupID)
	require.Equal(t, int64(10), *got.APIKey.GroupID)
	// 验证 AddGroupToAllowedGroups 被调用，且参数正确
	require.True(t, userRepo.addGroupCalled)
	require.Equal(t, int64(42), userRepo.addedUserID)
	require.Equal(t, int64(10), userRepo.addedGroupID)
	// 验证 result 标记了自动授权
	require.True(t, got.AutoGrantedGroupAccess)
	require.NotNil(t, got.GrantedGroupID)
	require.Equal(t, int64(10), *got.GrantedGroupID)
	require.Equal(t, "Exclusive", got.GrantedGroupName)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_NonExclusiveGroup_NoAllowedGroupUpdate(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nilREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Public", Status: StatusActive, IsExclusive: false, SubscriptionType: SubscriptionTypeStandardREDACTEDREDACTED
	userRepo := &userRepoStubForGroupUpdate{REDACTED
	cache := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, userRepo: userRepo, authCacheInvalidator: cacheREDACTED

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
REDACTED
	require.NotNil(t, got.APIKey.GroupID)
	// 非专属分组不触发 AddGroupToAllowedGroups
	require.False(t, userRepo.addGroupCalled)
	require.False(t, got.AutoGrantedGroupAccess)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_SubscriptionGroup_Blocked(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nilREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Sub", Status: StatusActive, IsExclusive: false, SubscriptionType: SubscriptionTypeSubscriptionREDACTEDREDACTED
	userRepo := &userRepoStubForGroupUpdate{REDACTED
	userSubRepo := &userSubRepoStubForGroupUpdate{getActiveErr: ErrSubscriptionNotFoundREDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, userRepo: userRepo, userSubRepo: userSubRepoREDACTED

	// 无有效订阅时应拒绝绑定
	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
REDACTED
	require.Equal(t, "SUBSCRIPTION_REQUIRED", infraerrors.Reason(err))
	require.True(t, userSubRepo.called)
	require.Equal(t, int64(42), userSubRepo.calledUserID)
	require.Equal(t, int64(10), userSubRepo.calledGroupID)
	require.False(t, userRepo.addGroupCalled)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_SubscriptionGroup_RequiresRepo(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nilREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Sub", Status: StatusActive, IsExclusive: false, SubscriptionType: SubscriptionTypeSubscriptionREDACTEDREDACTED
	userRepo := &userRepoStubForGroupUpdate{REDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, userRepo: userRepoREDACTED

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
REDACTED
	require.Equal(t, "SUBSCRIPTION_REPOSITORY_UNAVAILABLE", infraerrors.Reason(err))
	require.False(t, userRepo.addGroupCalled)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_SubscriptionGroup_AllowsActiveSubscription(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nilREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Sub", Status: StatusActive, IsExclusive: true, SubscriptionType: SubscriptionTypeSubscriptionREDACTEDREDACTED
	userRepo := &userRepoStubForGroupUpdate{REDACTED
	userSubRepo := &userSubRepoStubForGroupUpdate{
		getActiveSub: &UserSubscription{ID: 99, UserID: 42, GroupID: 10REDACTED,
REDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, userRepo: userRepo, userSubRepo: userSubRepoREDACTED

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
REDACTED
	require.True(t, userSubRepo.called)
	require.NotNil(t, got.APIKey.GroupID)
	require.Equal(t, int64(10), *got.APIKey.GroupID)
	require.False(t, userRepo.addGroupCalled)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_ExclusiveGroup_AllowedGroupAddFails_ReturnsError(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nilREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Exclusive", Status: StatusActive, IsExclusive: true, SubscriptionType: SubscriptionTypeStandardREDACTEDREDACTED
	userRepo := &userRepoStubForGroupUpdate{addGroupErr: errors.New("db error")REDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, userRepo: userRepoREDACTED

	// 严格模式：AddGroupToAllowedGroups 失败时，整体操作报错
	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
REDACTED
	require.Contains(t, err.Error(), "add group to user allowed groups")
	require.True(t, userRepo.addGroupCalled)
	// apiKey 不应被更新
	require.Nil(t, apiKeyRepo.updated)
REDACTED

func TestAdminService_AdminUpdateAPIKeyGroupID_Unbind_NoAllowedGroupUpdate(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: int64Ptr(10), Group: &Group{ID: 10, Name: "Exclusive"REDACTEDREDACTED
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existingREDACTED
	userRepo := &userRepoStubForGroupUpdate{REDACTED
	cache := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, userRepo: userRepo, authCacheInvalidator: cacheREDACTED

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(0))
REDACTED
	require.Nil(t, got.APIKey.GroupID)
	// 解绑时不修改 allowed_groups
	require.False(t, userRepo.addGroupCalled)
	require.False(t, got.AutoGrantedGroupAccess)
REDACTED
