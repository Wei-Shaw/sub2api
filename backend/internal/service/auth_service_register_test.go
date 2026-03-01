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

type defaultSubscriptionAssignerStub struct {
	calls []AssignSubscriptionInput
	err   error
REDACTED

func (s *defaultSubscriptionAssignerStub) AssignOrExtendSubscription(_ context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	if input != nil {
		s.calls = append(s.calls, *input)
REDACTED
	if s.err != nil {
		return nil, false, s.err
REDACTED
	return &UserSubscription{UserID: input.UserID, GroupID: input.GroupIDREDACTED, false, nil
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

func (s *emailCacheStub) GetPasswordResetToken(ctx context.Context, email string) (*PasswordResetTokenData, error) {
	return nil, nil
REDACTED

func (s *emailCacheStub) SetPasswordResetToken(ctx context.Context, email string, data *PasswordResetTokenData, ttl time.Duration) error {
	return nil
REDACTED

func (s *emailCacheStub) DeletePasswordResetToken(ctx context.Context, email string) error {
	return nil
REDACTED

func (s *emailCacheStub) IsPasswordResetEmailInCooldown(ctx context.Context, email string) bool {
	return false
REDACTED

func (s *emailCacheStub) SetPasswordResetEmailCooldown(ctx context.Context, email string, ttl time.Duration) error {
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
		nil, // redeemRepo
		nil, // refreshTokenCache
		cfg,
		settingService,
		emailService,
		nil,
		nil,
		nil, // promoService
		nil, // defaultSubAssigner
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

func TestAuthService_Register_DisabledByDefault(t *testing.T) {
	// 当 settings 为 nil（设置项不存在）时，注册应该默认关闭
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrRegDisabled)
REDACTED

func TestAuthService_Register_EmailVerifyEnabledButServiceNotConfigured(t *testing.T) {
	repo := &userRepoStub{REDACTED
	// 邮件验证开启但 emailCache 为 nil（emailService 未配置）
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
REDACTED, nil)

	// 应返回服务不可用错误，而不是允许绕过验证
	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "any-code", "", "")
	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestAuthService_Register_EmailVerifyRequired(t *testing.T) {
	repo := &userRepoStub{REDACTED
	cache := &emailCacheStub{REDACTED // 配置 emailService
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
REDACTED, cache)

	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "", "", "")
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

	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "wrong", "", "")
	require.ErrorIs(t, err, ErrInvalidVerifyCode)
	require.ErrorContains(t, err, "verify code")
REDACTED

func TestAuthService_Register_EmailExists(t *testing.T) {
	repo := &userRepoStub{exists: trueREDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrEmailExists)
REDACTED

func TestAuthService_Register_CheckEmailError(t *testing.T) {
	repo := &userRepoStub{existsErr: errors.New("db down")REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestAuthService_Register_ReservedEmail(t *testing.T) {
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil)

	_, _, err := service.Register(context.Background(), "linuxdo-123@linuxdo-connect.invalid", "password")
	require.ErrorIs(t, err, ErrEmailReserved)
REDACTED

func TestAuthService_Register_CreateError(t *testing.T) {
	repo := &userRepoStub{createErr: errors.New("create failed")REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestAuthService_Register_CreateEmailExistsRace(t *testing.T) {
	// 模拟竞态条件：ExistsByEmail 返回 false，但 Create 时因唯一约束失败
	repo := &userRepoStub{createErr: ErrEmailExistsREDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrEmailExists)
REDACTED

func TestAuthService_Register_Success(t *testing.T) {
	repo := &userRepoStub{nextID: 5REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil)

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

func TestAuthService_ValidateToken_ExpiredReturnsClaimsWithError(t *testing.T) {
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, nil, nil)

	// 创建用户并生成 token
	user := &User{
		ID:           1,
		Email:        "test@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
REDACTED
	token, err := service.GenerateToken(user)
REDACTED

	// 验证有效 token
	claims, err := service.ValidateToken(token)
REDACTED
	require.NotNil(t, claims)
	require.Equal(t, int64(1), claims.UserID)

	// 模拟过期 token（通过创建一个过期很久的 token）
	service.cfg.JWT.ExpireHour = -1 // 设置为负数使 token 立即过期
	expiredToken, err := service.GenerateToken(user)
REDACTED
	service.cfg.JWT.ExpireHour = 1 // 恢复

	// 验证过期 token 应返回 claims 和 ErrTokenExpired
	claims, err = service.ValidateToken(expiredToken)
	require.ErrorIs(t, err, ErrTokenExpired)
	require.NotNil(t, claims, "claims should not be nil when token is expired")
	require.Equal(t, int64(1), claims.UserID)
	require.Equal(t, "test@test.com", claims.Email)
REDACTED

func TestAuthService_RefreshToken_ExpiredTokenNoPanic(t *testing.T) {
	user := &User{
		ID:           1,
		Email:        "test@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
REDACTED
	repo := &userRepoStub{user: userREDACTED
	service := newAuthService(repo, nil, nil)

	// 创建过期 token
	service.cfg.JWT.ExpireHour = -1
	expiredToken, err := service.GenerateToken(user)
REDACTED
	service.cfg.JWT.ExpireHour = 1

	// RefreshToken 使用过期 token 不应 panic
	require.NotPanics(t, func() {
		newToken, err := service.RefreshToken(context.Background(), expiredToken)
	REDACTED
		require.NotEmpty(t, newToken)
REDACTED)
REDACTED

func TestAuthService_GetAccessTokenExpiresIn_FallbackToExpireHour(t *testing.T) {
	service := newAuthService(&userRepoStub{REDACTED, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 0

	require.Equal(t, 24*3600, service.GetAccessTokenExpiresIn())
REDACTED

func TestAuthService_GetAccessTokenExpiresIn_MinutesHasPriority(t *testing.T) {
	service := newAuthService(&userRepoStub{REDACTED, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 90

	require.Equal(t, 90*60, service.GetAccessTokenExpiresIn())
REDACTED

func TestAuthService_GenerateToken_UsesExpireHourWhenMinutesZero(t *testing.T) {
	service := newAuthService(&userRepoStub{REDACTED, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 0

	user := &User{
		ID:           1,
		Email:        "test@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
REDACTED

	token, err := service.GenerateToken(user)
REDACTED

	claims, err := service.ValidateToken(token)
REDACTED
	require.NotNil(t, claims)
	require.NotNil(t, claims.IssuedAt)
	require.NotNil(t, claims.ExpiresAt)

	require.WithinDuration(t, claims.IssuedAt.Time.Add(24*time.Hour), claims.ExpiresAt.Time, 2*time.Second)
REDACTED

func TestAuthService_GenerateToken_UsesMinutesWhenConfigured(t *testing.T) {
	service := newAuthService(&userRepoStub{REDACTED, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 90

	user := &User{
		ID:           2,
		Email:        "test2@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
REDACTED

	token, err := service.GenerateToken(user)
REDACTED

	claims, err := service.ValidateToken(token)
REDACTED
	require.NotNil(t, claims)
	require.NotNil(t, claims.IssuedAt)
	require.NotNil(t, claims.ExpiresAt)

	require.WithinDuration(t, claims.IssuedAt.Time.Add(90*time.Minute), claims.ExpiresAt.Time, 2*time.Second)
REDACTED

func TestAuthService_Register_AssignsDefaultSubscriptions(t *testing.T) {
	repo := &userRepoStub{nextID: 42REDACTED
	assigner := &defaultSubscriptionAssignerStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyDefaultSubscriptions: `[{"group_id":11,"validity_days":30REDACTED,{"group_id":12,"validity_days":7REDACTED]`,
REDACTED, nil)
	service.defaultSubAssigner = assigner

	_, user, err := service.Register(context.Background(), "default-sub@test.com", "password")
REDACTED
	require.NotNil(t, user)
	require.Len(t, assigner.calls, 2)
	require.Equal(t, int64(42), assigner.calls[0].UserID)
	require.Equal(t, int64(11), assigner.calls[0].GroupID)
	require.Equal(t, 30, assigner.calls[0].ValidityDays)
	require.Equal(t, int64(12), assigner.calls[1].GroupID)
	require.Equal(t, 7, assigner.calls[1].ValidityDays)
REDACTED
