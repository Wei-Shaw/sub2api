//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingRepoStub struct {
	values map[string]string
	err    error
REDACTED

func (s *settingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *settingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if s.err != nil {
		return "", s.err
REDACTED
	if v, ok := s.values[key]; ok {
		return v, nil
REDACTED
	return "", ErrSettingNotFound
REDACTED

func (s *settingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *settingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
REDACTED

func (s *settingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (s *settingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *settingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

type emailCacheStub struct {
	data *VerificationCodeData
	err  error
REDACTED

func (s *emailCacheStub) GetVerificationCode(ctx context.Context, email string) (*VerificationCodeData, error) {
	if s.err != nil {
		return nil, s.err
REDACTED
	return s.data, nil
REDACTED

func (s *emailCacheStub) SetVerificationCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error {
	return nil
REDACTED

func (s *emailCacheStub) DeleteVerificationCode(ctx context.Context, email string) error {
	return nil
REDACTED

func newAuthService(repo *userRepoStub, settings map[string]string, emailCache EmailCache) *AuthService {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			ExpireHour: 1,
	REDACTED,
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
	REDACTED,
REDACTED

	var settingService *SettingService
	if settings != nil {
		settingService = NewSettingService(&settingRepoStub{values: settingsREDACTED, cfg)
REDACTED

	var emailService *EmailService
	if emailCache != nil {
		emailService = NewEmailService(&settingRepoStub{values: settingsREDACTED, emailCache)
REDACTED

	return NewAuthService(
		repo,
		cfg,
		settingService,
		emailService,
		nil,
		nil,
	)
REDACTED

func TestAuthService_Register_Disabled(t *testing.T) {
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "false",
REDACTED, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrRegDisabled)
REDACTED

func TestAuthService_Register_EmailVerifyRequired(t *testing.T) {
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
REDACTED, nil)

	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "")
	require.ErrorIs(t, err, ErrEmailVerifyRequired)
REDACTED

func TestAuthService_Register_EmailVerifyInvalid(t *testing.T) {
	repo := &userRepoStub{REDACTED
	cache := &emailCacheStub{
		data: &VerificationCodeData{Code: "expected", Attempts: 0REDACTED,
REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
REDACTED, cache)

	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "wrong")
	require.ErrorIs(t, err, ErrInvalidVerifyCode)
	require.ErrorContains(t, err, "verify code")
REDACTED

func TestAuthService_Register_EmailExists(t *testing.T) {
	repo := &userRepoStub{exists: trueREDACTED
	service := newAuthService(repo, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrEmailExists)
REDACTED

func TestAuthService_Register_CheckEmailError(t *testing.T) {
	repo := &userRepoStub{existsErr: errors.New("db down")REDACTED
	service := newAuthService(repo, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestAuthService_Register_CreateError(t *testing.T) {
	repo := &userRepoStub{createErr: errors.New("create failed")REDACTED
	service := newAuthService(repo, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestAuthService_Register_Success(t *testing.T) {
	repo := &userRepoStub{nextID: 5REDACTED
	service := newAuthService(repo, nil, nil)

	token, user, err := service.Register(context.Background(), "user@test.com", "password")
REDACTED
	require.NotEmpty(t, token)
	require.NotNil(t, user)
	require.Equal(t, int64(5), user.ID)
	require.Equal(t, "user@test.com", user.Email)
	require.Equal(t, RoleUser, user.Role)
	require.Equal(t, StatusActive, user.Status)
	require.Equal(t, 3.5, user.Balance)
	require.Equal(t, 2, user.Concurrency)
	require.Len(t, repo.created, 1)
	require.True(t, user.CheckPassword("password"))
REDACTED
