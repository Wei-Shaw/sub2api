//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateUser_WithAdminRole(t *testing.T) {
	repo := &userRepoStub{nextID: 30REDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "admin@test.com",
		Password: "strong-pass",
		Role:     RoleAdmin,
REDACTED)
REDACTED
	require.Equal(t, RoleAdmin, user.Role)
REDACTED

func TestAdminService_CreateUser_DefaultsToUserRole(t *testing.T) {
	repo := &userRepoStub{nextID: 31REDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "plain@test.com",
		Password: "strong-pass",
REDACTED)
REDACTED
	require.Equal(t, RoleUser, user.Role)
REDACTED

func TestAdminService_CreateUser_InvalidRoleRejected(t *testing.T) {
	repo := &userRepoStub{nextID: 32REDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "bad@test.com",
		Password: "strong-pass",
		Role:     "superuser",
REDACTED)
REDACTED
	require.Empty(t, repo.created, "非法角色不应写入用户")
REDACTED

func TestAdminService_UpdateUser_PromoteToAdmin(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleUserREDACTEDREDACTED
	repo := &rpmUserRepoStub{userRepoStub: baseREDACTED
	invalidator := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{REDACTED,
		authCacheInvalidator: invalidator,
REDACTED

	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Role: RoleAdminREDACTED)
REDACTED
	require.Equal(t, RoleAdmin, updated.Role)
	require.Equal(t, []int64{42REDACTED, invalidator.userIDs, "角色变更应失效认证缓存")
REDACTED

func TestAdminService_UpdateUser_RoleOmittedKeepsExisting(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleAdminREDACTEDREDACTED
	repo := &rpmUserRepoStub{userRepoStub: baseREDACTED
	svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{REDACTEDREDACTED

	newName := "renamed"
	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Username: &newNameREDACTED)
REDACTED
	require.Equal(t, RoleAdmin, updated.Role, "未提供 role 时不应改变现有角色")
REDACTED

func TestAdminService_UpdateUser_InvalidRoleRejected(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleUserREDACTEDREDACTED
	repo := &rpmUserRepoStub{userRepoStub: baseREDACTED
	svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{REDACTEDREDACTED

	_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Role: "root"REDACTED)
REDACTED
	require.Nil(t, repo.lastUpdated, "非法角色不应触发持久化")
REDACTED

// roleGuardUserRepoStub 在 rpmUserRepoStub 之上提供可控的管理员计数，
// 用于测试"最后一个管理员不可降级"守卫。
type roleGuardUserRepoStub struct {
	*rpmUserRepoStub
	adminTotal int64
	listCalls  int
REDACTED

func (s *roleGuardUserRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	s.listCalls++
	return nil, &pagination.PaginationResult{Total: s.adminTotalREDACTED, nil
REDACTED

func TestAdminService_UpdateUser_DemoteLastAdminRejected(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "a@example.com", Role: RoleAdminREDACTEDREDACTED
	repo := &roleGuardUserRepoStub{rpmUserRepoStub: &rpmUserRepoStub{userRepoStub: baseREDACTED, adminTotal: 1REDACTED
	svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{REDACTEDREDACTED

	_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Role: RoleUserREDACTED)
REDACTED
	require.Contains(t, err.Error(), "last admin")
	require.Nil(t, repo.lastUpdated, "最后一个管理员不应被降级持久化")
	require.Equal(t, 1, repo.listCalls, "降级路径应触发管理员计数")
REDACTED

func TestAdminService_UpdateUser_DemoteAdminAllowedWhenOthersExist(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "a@example.com", Role: RoleAdminREDACTEDREDACTED
	repo := &roleGuardUserRepoStub{rpmUserRepoStub: &rpmUserRepoStub{userRepoStub: baseREDACTED, adminTotal: 2REDACTED
	invalidator := &authCacheInvalidatorStub{REDACTED
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{REDACTED,
		authCacheInvalidator: invalidator,
REDACTED

	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Role: RoleUserREDACTED)
REDACTED
	require.Equal(t, RoleUser, updated.Role)
	require.NotNil(t, repo.lastUpdated)
	require.Equal(t, RoleUser, repo.lastUpdated.Role, "存在其他管理员时允许降级")
REDACTED

func TestAdminService_UpdateUser_PromoteDoesNotCountAdmins(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleUserREDACTEDREDACTED
	repo := &roleGuardUserRepoStub{rpmUserRepoStub: &rpmUserRepoStub{userRepoStub: baseREDACTED, adminTotal: 1REDACTED
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{REDACTED,
		authCacheInvalidator: &authCacheInvalidatorStub{REDACTED,
REDACTED

	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Role: RoleAdminREDACTED)
REDACTED
	require.Equal(t, RoleAdmin, updated.Role)
	require.Equal(t, 0, repo.listCalls, "升级路径不应触发管理员计数")
REDACTED
