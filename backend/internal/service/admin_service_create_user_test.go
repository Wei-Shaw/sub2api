//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateUser_Success(t *testing.T) {
	repo := &userRepoStub{nextID: 10REDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	input := &CreateUserInput{
		Email:         "user@test.com",
		Password:      "strong-pass",
		Username:      "tester",
		Wechat:        "wx",
		Notes:         "note",
		Balance:       12.5,
		Concurrency:   7,
		AllowedGroups: []int64{3, 5REDACTED,
REDACTED

	user, err := svc.CreateUser(context.Background(), input)
REDACTED
	require.NotNil(t, user)
	require.Equal(t, int64(10), user.ID)
	require.Equal(t, input.Email, user.Email)
	require.Equal(t, input.Username, user.Username)
	require.Equal(t, input.Wechat, user.Wechat)
	require.Equal(t, input.Notes, user.Notes)
	require.Equal(t, input.Balance, user.Balance)
	require.Equal(t, input.Concurrency, user.Concurrency)
	require.Equal(t, input.AllowedGroups, user.AllowedGroups)
	require.Equal(t, RoleUser, user.Role)
	require.Equal(t, StatusActive, user.Status)
	require.True(t, user.CheckPassword(input.Password))
	require.Len(t, repo.created, 1)
	require.Equal(t, user, repo.created[0])
REDACTED

func TestAdminService_CreateUser_EmailExists(t *testing.T) {
	repo := &userRepoStub{createErr: ErrEmailExistsREDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "dup@test.com",
		Password: "password",
REDACTED)
	require.ErrorIs(t, err, ErrEmailExists)
	require.Empty(t, repo.created)
REDACTED

func TestAdminService_CreateUser_CreateError(t *testing.T) {
	createErr := errors.New("db down")
	repo := &userRepoStub{createErr: createErrREDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "user@test.com",
		Password: "password",
REDACTED)
	require.ErrorIs(t, err, createErr)
	require.Empty(t, repo.created)
REDACTED
