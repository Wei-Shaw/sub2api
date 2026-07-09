//go:build unit

package service

import (
	"context"
	"testing"

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
