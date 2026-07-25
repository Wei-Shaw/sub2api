//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type ensureEmailCall struct {
	userID int64
	email  string
REDACTED

type replaceEmailCall struct {
	userID   int64
	oldEmail string
	newEmail string
REDACTED

type emailSyncRepoStub struct {
	user         *User
	nextID       int64
	updateCalls  int
	created      []*User
	updated      []*User
	ensureCalls  []ensureEmailCall
	replaceCalls []replaceEmailCall
	ensureErr    error
	replaceErr   error
REDACTED

func (s *emailSyncRepoStub) CreateWithEmailAliasGuard(ctx context.Context, user *User) error {
	return s.Create(ctx, user)
REDACTED

func (s *emailSyncRepoStub) Create(_ context.Context, user *User) error {
	if s.nextID != 0 && user.ID == 0 {
		user.ID = s.nextID
REDACTED
	s.created = append(s.created, user)
	s.user = user
	return nil
REDACTED

func (s *emailSyncRepoStub) GetByID(_ context.Context, _ int64) (*User, error) {
	if s.user == nil {
		return nil, ErrUserNotFound
REDACTED
	cloned := *s.user
	return &cloned, nil
REDACTED

func (s *emailSyncRepoStub) GetByEmail(_ context.Context, _ string) (*User, error) {
	return nil, ErrUserNotFound
REDACTED

func (s *emailSyncRepoStub) GetFirstAdmin(context.Context) (*User, error) {
	return nil, fmt.Errorf("unexpected GetFirstAdmin call")
REDACTED

func (s *emailSyncRepoStub) Update(_ context.Context, user *User) error {
	s.updateCalls++
	s.updated = append(s.updated, user)
	s.user = user
	return nil
REDACTED

func (s *emailSyncRepoStub) Delete(context.Context, int64) error { return nil REDACTED

func (s *emailSyncRepoStub) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	return nil, fmt.Errorf("unexpected GetUserAvatar call")
REDACTED

func (s *emailSyncRepoStub) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	return nil, fmt.Errorf("unexpected UpsertUserAvatar call")
REDACTED

func (s *emailSyncRepoStub) DeleteUserAvatar(context.Context, int64) error {
	return fmt.Errorf("unexpected DeleteUserAvatar call")
REDACTED

func (s *emailSyncRepoStub) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, fmt.Errorf("unexpected List call")
REDACTED

func (s *emailSyncRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, fmt.Errorf("unexpected ListWithFilters call")
REDACTED

func (s *emailSyncRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{REDACTED, nil
REDACTED

func (s *emailSyncRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
REDACTED

func (s *emailSyncRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	return nil
REDACTED

func (s *emailSyncRepoStub) UpdateBalance(context.Context, int64, float64) error { return nil REDACTED

func (s *emailSyncRepoStub) DeductBalance(context.Context, int64, float64) error { return nil REDACTED

func (s *emailSyncRepoStub) UpdateConcurrency(context.Context, int64, int) error { return nil REDACTED

func (s *emailSyncRepoStub) ExistsByEmail(context.Context, string) (bool, error) { return false, nil REDACTED

func (s *emailSyncRepoStub) ExistsByEmailAlias(context.Context, string) (bool, error) {
	return false, nil
REDACTED

func (s *emailSyncRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
REDACTED

func (s *emailSyncRepoStub) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
REDACTED
func (s *emailSyncRepoStub) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
REDACTED
func (s *emailSyncRepoStub) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	return 0, nil
REDACTED

func (s *emailSyncRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error { return nil REDACTED

func (s *emailSyncRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
REDACTED

func (s *emailSyncRepoStub) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	return nil, nil
REDACTED

func (s *emailSyncRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error { return nil REDACTED

func (s *emailSyncRepoStub) UpdateTotpSecret(context.Context, int64, *string) error { return nil REDACTED

func (s *emailSyncRepoStub) EnableTotp(context.Context, int64) error { return nil REDACTED

func (s *emailSyncRepoStub) DisableTotp(context.Context, int64) error { return nil REDACTED
func (s *emailSyncRepoStub) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	return s.GetByID(ctx, id)
REDACTED

func (s *emailSyncRepoStub) EnsureEmailAuthIdentity(_ context.Context, userID int64, email string) error {
	s.ensureCalls = append(s.ensureCalls, ensureEmailCall{userID: userID, email: emailREDACTED)
	return s.ensureErr
REDACTED

func (s *emailSyncRepoStub) ReplaceEmailAuthIdentity(_ context.Context, userID int64, oldEmail, newEmail string) error {
	s.replaceCalls = append(s.replaceCalls, replaceEmailCall{
		userID:   userID,
		oldEmail: oldEmail,
		newEmail: newEmail,
REDACTED)
	return s.replaceErr
REDACTED

func TestAdminService_CreateUser_DoesNotReturnPartialSuccessFromEmailIdentityResync(t *testing.T) {
	repo := &emailSyncRepoStub{
		nextID:    55,
		ensureErr: fmt.Errorf("unexpected email resync"),
REDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "admin-created@example.com",
		Password: "strong-pass",
REDACTED)
REDACTED
	require.NotNil(t, user)
	require.Equal(t, int64(55), user.ID)
	require.Empty(t, repo.ensureCalls)
	require.Empty(t, repo.replaceCalls)
REDACTED

func TestAdminService_UpdateUser_DoesNotReturnPartialSuccessFromEmailIdentityResync(t *testing.T) {
	repo := &emailSyncRepoStub{
		user: &User{
			ID:          91,
			Email:       "before@example.com",
			Role:        RoleUser,
			Status:      StatusActive,
			Concurrency: 3,
	REDACTED,
		replaceErr: fmt.Errorf("unexpected email resync"),
REDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	updated, err := svc.UpdateUser(context.Background(), 91, &UpdateUserInput{
		Email: "after@example.com",
REDACTED)
REDACTED
	require.NotNil(t, updated)
	require.Equal(t, "after@example.com", updated.Email)
	require.Empty(t, repo.replaceCalls)
	require.Empty(t, repo.ensureCalls)
REDACTED
