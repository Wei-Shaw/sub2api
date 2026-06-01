//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateUser_Success(t *testing.T) {
	repo := &userRepoStub{nextID: 10REDACTED
	svc := &adminServiceImpl{userRepo: repoREDACTED
	balance := 12.5

	input := &CreateUserInput{
		Email:         "user@test.com",
		Password:      "strong-pass",
		Username:      "tester",
		Notes:         "note",
		Balance:       &balance,
		Concurrency:   7,
		AllowedGroups: []int64{3, 5REDACTED,
REDACTED

	user, err := svc.CreateUser(context.Background(), input)
REDACTED
	require.NotNil(t, user)
	require.Equal(t, int64(10), user.ID)
	require.Equal(t, input.Email, user.Email)
	require.Equal(t, input.Username, user.Username)
	require.Equal(t, input.Notes, user.Notes)
	require.Equal(t, balance, user.Balance)
	require.Equal(t, input.Concurrency, user.Concurrency)
	require.Equal(t, input.AllowedGroups, user.AllowedGroups)
	require.Equal(t, RoleUser, user.Role)
	require.Equal(t, StatusActive, user.Status)
	require.True(t, user.CheckPassword(input.Password))
	require.Len(t, repo.created, 1)
	require.Equal(t, user, repo.created[0])
REDACTED

func TestAdminService_CreateUser_UsesDefaultBalanceWhenBalanceOmitted(t *testing.T) {
	repo := &userRepoStub{nextID: 11REDACTED
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance: 0,
	REDACTED,
REDACTED
	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultBalance: "0.02",
REDACTEDREDACTED, cfg)
	svc := &adminServiceImpl{userRepo: repo, settingService: settingServiceREDACTED

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "default-balance@test.com",
		Password: "strong-pass",
REDACTED)

REDACTED
	require.NotNil(t, user)
	require.Equal(t, 0.02, user.Balance)
	require.Len(t, repo.created, 1)
	require.Equal(t, 0.02, repo.created[0].Balance)
REDACTED

func TestAdminService_CreateUser_ExplicitZeroBalanceOverridesDefault(t *testing.T) {
	repo := &userRepoStub{nextID: 12REDACTED
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance: 0,
	REDACTED,
REDACTED
	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultBalance: "0.02",
REDACTEDREDACTED, cfg)
	svc := &adminServiceImpl{userRepo: repo, settingService: settingServiceREDACTED
	balance := 0.0

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "zero-balance@test.com",
		Password: "strong-pass",
		Balance:  &balance,
REDACTED)

REDACTED
	require.NotNil(t, user)
	require.Equal(t, 0.0, user.Balance)
	require.Len(t, repo.created, 1)
	require.Equal(t, 0.0, repo.created[0].Balance)
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

func TestAdminService_CreateUser_AssignsDefaultSubscriptions(t *testing.T) {
	repo := &userRepoStub{nextID: 21REDACTED
	assigner := &defaultSubscriptionAssignerStub{REDACTED
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
	REDACTED,
REDACTED
	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultSubscriptions: `[{"group_id":5,"validity_days":30REDACTED]`,
REDACTEDREDACTED, cfg)
	svc := &adminServiceImpl{
		userRepo:           repo,
		settingService:     settingService,
		defaultSubAssigner: assigner,
REDACTED

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "new-user@test.com",
		Password: "password",
REDACTED)
REDACTED
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(21), assigner.calls[0].UserID)
	require.Equal(t, int64(5), assigner.calls[0].GroupID)
	require.Equal(t, 30, assigner.calls[0].ValidityDays)
REDACTED
