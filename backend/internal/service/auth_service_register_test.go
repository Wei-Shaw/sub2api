//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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
	if s.err != nil {
		return nil, s.err
REDACTED
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			result[key] = v
	REDACTED
REDACTED
	return result, nil
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

type refreshTokenCacheStub struct{REDACTED

type userPlatformQuotaRepoStub struct {
	bulkInsertCalls [][]UserPlatformQuotaRecord
	bulkInsertErr   error
REDACTED

func (s *userPlatformQuotaRepoStub) BulkInsertInitial(_ context.Context, records []UserPlatformQuotaRecord) error {
	cloned := make([]UserPlatformQuotaRecord, len(records))
	copy(cloned, records)
	s.bulkInsertCalls = append(s.bulkInsertCalls, cloned)
	return s.bulkInsertErr
REDACTED

func (s *userPlatformQuotaRepoStub) GetByUserPlatform(context.Context, int64, string) (*UserPlatformQuotaRecord, error) {
	panic("unexpected GetByUserPlatform call")
REDACTED

func (s *userPlatformQuotaRepoStub) ListByUser(context.Context, int64) ([]UserPlatformQuotaRecord, error) {
	panic("unexpected ListByUser call")
REDACTED

func (s *userPlatformQuotaRepoStub) IncrementUsageWithReset(context.Context, int64, string, float64, time.Time) error {
	panic("unexpected IncrementUsageWithReset call")
REDACTED

func (s *userPlatformQuotaRepoStub) UpsertForUser(context.Context, int64, []UserPlatformQuotaRecord) error {
	panic("unexpected UpsertForUser call")
REDACTED

func (s *userPlatformQuotaRepoStub) ResetExpiredWindow(context.Context, int64, string, string, time.Time) error {
	panic("unexpected ResetExpiredWindow call")
REDACTED

func (s *userPlatformQuotaRepoStub) BatchSnapshotUsage(_ context.Context, _ []UserPlatformQuotaSnapshot, _ time.Time) error {
	return nil
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

func (s *refreshTokenCacheStub) StoreRefreshToken(context.Context, string, *RefreshTokenData, time.Duration) error {
	return nil
REDACTED

func (s *refreshTokenCacheStub) GetRefreshToken(context.Context, string) (*RefreshTokenData, error) {
	return nil, ErrRefreshTokenNotFound
REDACTED

func (s *refreshTokenCacheStub) DeleteRefreshToken(context.Context, string) error {
	return nil
REDACTED

func (s *refreshTokenCacheStub) DeleteUserRefreshTokens(context.Context, int64) error {
	return nil
REDACTED

func (s *refreshTokenCacheStub) DeleteTokenFamily(context.Context, string) error {
	return nil
REDACTED

func (s *refreshTokenCacheStub) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
REDACTED

func (s *refreshTokenCacheStub) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
REDACTED

func (s *refreshTokenCacheStub) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
REDACTED

func (s *refreshTokenCacheStub) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
REDACTED

func (s *refreshTokenCacheStub) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return false, nil
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

func (s *emailCacheStub) GetNotifyVerifyCode(ctx context.Context, email string) (*VerificationCodeData, error) {
	return nil, nil
REDACTED

func (s *emailCacheStub) SetNotifyVerifyCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error {
	return nil
REDACTED

func (s *emailCacheStub) DeleteNotifyVerifyCode(ctx context.Context, email string) error {
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

func (s *emailCacheStub) GetNotifyCodeUserRate(ctx context.Context, userID int64) (int64, error) {
	return 0, nil
REDACTED

func (s *emailCacheStub) IncrNotifyCodeUserRate(ctx context.Context, userID int64, window time.Duration) (int64, error) {
	return 0, nil
REDACTED

func newAuthService(repo *userRepoStub, settings map[string]string, emailCache EmailCache, quotaRepo UserPlatformQuotaRepository) *AuthService {
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
		nil, // entClient
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
		nil, // affiliateService
		quotaRepo,
	)
REDACTED

func TestAuthService_Register_Disabled(t *testing.T) {
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "false",
REDACTED, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrRegDisabled)
REDACTED

func TestAuthService_Register_DisabledByDefault(t *testing.T) {
	// 当 settings 为 nil（设置项不存在）时，注册应该默认关闭
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, nil, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrRegDisabled)
REDACTED

func TestAuthService_Register_SnapshotsPlatformQuotaDefaults(t *testing.T) {
	repo := &userRepoStub{nextID: 77REDACTED
	quotaRepo := &userPlatformQuotaRepoStub{REDACTED

	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:   "true",
		SettingKeyDefaultPlatformQuotas: `{"openai": {"weekly": 12.34REDACTEDREDACTED`,
REDACTED, nil, quotaRepo)

	_, user, err := service.Register(context.Background(), "newuser@test.com", "password")
REDACTED
	require.NotNil(t, user)

	require.Len(t, quotaRepo.bulkInsertCalls, 1)

	records := quotaRepo.bulkInsertCalls[0]
	var openaiRecord *UserPlatformQuotaRecord
	for i := range records {
		if records[i].Platform == "openai" {
			openaiRecord = &records[i]
			break
	REDACTED
REDACTED
	require.NotNil(t, openaiRecord, "expected openai platform record")
	require.Equal(t, int64(77), openaiRecord.UserID)
	require.NotNil(t, openaiRecord.WeeklyLimitUSD)
	require.InDelta(t, 12.34, *openaiRecord.WeeklyLimitUSD, 0.0001)
REDACTED

func TestAuthService_Register_DoesNotSnapshotOnDisabled(t *testing.T) {
	repo := &userRepoStub{REDACTED
	quotaRepo := &userPlatformQuotaRepoStub{REDACTED

	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "false",
REDACTED, nil, quotaRepo)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrRegDisabled)

	require.Empty(t, quotaRepo.bulkInsertCalls, "registration rejected before user creation must not snapshot")
REDACTED

func TestAuthService_Register_EmailVerifyEnabledButServiceNotConfigured(t *testing.T) {
	repo := &userRepoStub{REDACTED
	// 邮件验证开启但 emailCache 为 nil（emailService 未配置）
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
REDACTED, nil, nil)

	// 应返回服务不可用错误，而不是允许绕过验证
	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "any-code", "", "", "")
	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestAuthService_Register_EmailVerifyRequired(t *testing.T) {
	repo := &userRepoStub{REDACTED
	cache := &emailCacheStub{REDACTED // 配置 emailService
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
REDACTED, cache, nil)

	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "", "", "", "")
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
REDACTED, cache, nil)

	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "wrong", "", "", "")
	require.ErrorIs(t, err, ErrInvalidVerifyCode)
	require.ErrorContains(t, err, "verify code")
REDACTED

func TestAuthService_Register_EmailExists(t *testing.T) {
	repo := &userRepoStub{exists: trueREDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrEmailExists)
REDACTED

func TestAuthService_Register_CheckEmailError(t *testing.T) {
	repo := &userRepoStub{existsErr: errors.New("db down")REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestAuthService_Register_ReservedEmail(t *testing.T) {
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil, nil)

	_, _, err := service.Register(context.Background(), "linuxdo-123@linuxdo-connect.invalid", "password")
	require.ErrorIs(t, err, ErrEmailReserved)
REDACTED

func TestAuthService_Register_EmailSuffixNotAllowed(t *testing.T) {
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:              "true",
		SettingKeyRegistrationEmailSuffixWhitelist: `["@example.com","@company.com"]`,
REDACTED, nil, nil)

	_, _, err := service.Register(context.Background(), "user@other.com", "password")
	require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
	appErr := infraerrors.FromError(err)
	require.Contains(t, appErr.Message, "@example.com")
	require.Contains(t, appErr.Message, "@company.com")
	require.Equal(t, "EMAIL_SUFFIX_NOT_ALLOWED", appErr.Reason)
	require.Equal(t, "2", appErr.Metadata["allowed_suffix_count"])
	require.Equal(t, "@example.com,@company.com", appErr.Metadata["allowed_suffixes"])
REDACTED

func TestAuthService_Register_EmailSuffixAllowed(t *testing.T) {
	repo := &userRepoStub{nextID: 8REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:              "true",
		SettingKeyRegistrationEmailSuffixWhitelist: `["example.com"]`,
REDACTED, nil, nil)

	_, user, err := service.Register(context.Background(), "user@example.com", "password")
REDACTED
	require.NotNil(t, user)
	require.Equal(t, int64(8), user.ID)
REDACTED

func TestAuthService_SendVerifyCode_EmailSuffixNotAllowed(t *testing.T) {
	repo := &userRepoStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:              "true",
		SettingKeyRegistrationEmailSuffixWhitelist: `["@example.com","@company.com"]`,
REDACTED, nil, nil)

	err := service.SendVerifyCode(context.Background(), "user@other.com")
	require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
	appErr := infraerrors.FromError(err)
	require.Contains(t, appErr.Message, "@example.com")
	require.Contains(t, appErr.Message, "@company.com")
	require.Equal(t, "2", appErr.Metadata["allowed_suffix_count"])
REDACTED

func TestAuthService_Register_CreateError(t *testing.T) {
	repo := &userRepoStub{createErr: errors.New("create failed")REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestAuthService_Register_CreateEmailExistsRace(t *testing.T) {
	// 模拟竞态条件：ExistsByEmail 返回 false，但 Create 时因唯一约束失败
	repo := &userRepoStub{createErr: ErrEmailExistsREDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
REDACTED, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrEmailExists)
REDACTED

func TestAuthService_Register_Success(t *testing.T) {
	repo := &userRepoStub{nextID: 5REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "false",
REDACTED, nil, nil)

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
	service := newAuthService(repo, nil, nil, nil)

	// 创建用户并生成 token
	user := &User{
		ID:           1,
		Email:        "test@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
REDACTED
	token, err := service.GenerateToken(context.Background(), user)
REDACTED

	// 验证有效 token
	claims, err := service.ValidateToken(token)
REDACTED
	require.NotNil(t, claims)
	require.Equal(t, int64(1), claims.UserID)

	// 模拟过期 token（通过创建一个过期很久的 token）
	service.cfg.JWT.ExpireHour = -1 // 设置为负数使 token 立即过期
	expiredToken, err := service.GenerateToken(context.Background(), user)
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
	service := newAuthService(repo, nil, nil, nil)

	// 创建过期 token
	service.cfg.JWT.ExpireHour = -1
	expiredToken, err := service.GenerateToken(context.Background(), user)
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
	service := newAuthService(&userRepoStub{REDACTED, nil, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 0

	require.Equal(t, 24*3600, service.GetAccessTokenExpiresIn())
REDACTED

func TestAuthService_GetAccessTokenExpiresIn_MinutesHasPriority(t *testing.T) {
	service := newAuthService(&userRepoStub{REDACTED, nil, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 90

	require.Equal(t, 90*60, service.GetAccessTokenExpiresIn())
REDACTED

func TestAuthService_GenerateToken_UsesExpireHourWhenMinutesZero(t *testing.T) {
	service := newAuthService(&userRepoStub{REDACTED, nil, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 0

	user := &User{
		ID:           1,
		Email:        "test@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
REDACTED

	token, err := service.GenerateToken(context.Background(), user)
REDACTED

	claims, err := service.ValidateToken(token)
REDACTED
	require.NotNil(t, claims)
	require.NotNil(t, claims.IssuedAt)
	require.NotNil(t, claims.ExpiresAt)

	require.WithinDuration(t, claims.IssuedAt.Time.Add(24*time.Hour), claims.ExpiresAt.Time, 2*time.Second)
REDACTED

func TestAuthService_GenerateToken_UsesMinutesWhenConfigured(t *testing.T) {
	service := newAuthService(&userRepoStub{REDACTED, nil, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 90

	user := &User{
		ID:           2,
		Email:        "test2@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
REDACTED

	token, err := service.GenerateToken(context.Background(), user)
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
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyDefaultSubscriptions:                `[{"group_id":11,"validity_days":30REDACTED,{"group_id":12,"validity_days":7REDACTED]`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "false",
REDACTED, nil, nil)
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

func TestAuthService_Register_UsesEmailAuthSourceDefaultsWhenGrantEnabled(t *testing.T) {
	repo := &userRepoStub{nextID: 52REDACTED
	assigner := &defaultSubscriptionAssignerStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyDefaultSubscriptions:                `[{"group_id":91,"validity_days":3REDACTED]`,
		SettingKeyAuthSourceDefaultEmailBalance:       "12.5",
		SettingKeyAuthSourceDefaultEmailConcurrency:   "7",
		SettingKeyAuthSourceDefaultEmailSubscriptions: `[{"group_id":11,"validity_days":30REDACTED]`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "true",
REDACTED, nil, nil)
	service.defaultSubAssigner = assigner

	_, user, err := service.Register(context.Background(), "email-defaults@test.com", "password")
REDACTED
	require.NotNil(t, user)
	require.Equal(t, 12.5, user.Balance)
	require.Equal(t, 7, user.Concurrency)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(11), assigner.calls[0].GroupID)
	require.Equal(t, 30, assigner.calls[0].ValidityDays)
REDACTED

func TestAuthService_Register_GrantOnSignupFalseFallsBackToGlobalDefaults(t *testing.T) {
	repo := &userRepoStub{nextID: 53REDACTED
	assigner := &defaultSubscriptionAssignerStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyDefaultSubscriptions:                `[{"group_id":31,"validity_days":5REDACTED]`,
		SettingKeyAuthSourceDefaultEmailBalance:       "99",
		SettingKeyAuthSourceDefaultEmailConcurrency:   "88",
		SettingKeyAuthSourceDefaultEmailSubscriptions: `[{"group_id":32,"validity_days":9REDACTED]`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "false",
REDACTED, nil, nil)
	service.defaultSubAssigner = assigner

	_, user, err := service.Register(context.Background(), "email-global@test.com", "password")
REDACTED
	require.NotNil(t, user)
	require.Equal(t, 3.5, user.Balance)
	require.Equal(t, 2, user.Concurrency)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(31), assigner.calls[0].GroupID)
	require.Equal(t, 5, assigner.calls[0].ValidityDays)
REDACTED

func TestAuthService_Register_GrantOnSignupMergesSourceOverridesWithGlobalDefaults(t *testing.T) {
	repo := &userRepoStub{nextID: 54REDACTED
	assigner := &defaultSubscriptionAssignerStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyDefaultSubscriptions:                `[{"group_id":31,"validity_days":5REDACTED]`,
		SettingKeyAuthSourceDefaultEmailBalance:       "9.5",
		SettingKeyAuthSourceDefaultEmailConcurrency:   "5",
		SettingKeyAuthSourceDefaultEmailSubscriptions: `[]`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "true",
REDACTED, nil, nil)
	service.defaultSubAssigner = assigner

	_, user, err := service.Register(context.Background(), "email-merged@test.com", "password")
REDACTED
	require.NotNil(t, user)
	require.Equal(t, 9.5, user.Balance)
	require.Equal(t, 5, user.Concurrency)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(31), assigner.calls[0].GroupID)
	require.Equal(t, 5, assigner.calls[0].ValidityDays)
REDACTED

func TestAuthService_LoginOrRegisterOAuthWithTokenPair_UsesLinuxDoAuthSourceDefaultsOnSignup(t *testing.T) {
	repo := &userRepoStub{nextID: 61REDACTED
	assigner := &defaultSubscriptionAssignerStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                   "true",
		SettingKeyDefaultSubscriptions:                  `[{"group_id":81,"validity_days":1REDACTED]`,
		SettingKeyAuthSourceDefaultLinuxDoBalance:       "21.75",
		SettingKeyAuthSourceDefaultLinuxDoConcurrency:   "9",
		SettingKeyAuthSourceDefaultLinuxDoSubscriptions: `[{"group_id":22,"validity_days":14REDACTED]`,
		SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup: "true",
REDACTED, nil, nil)
	service.defaultSubAssigner = assigner
	service.refreshTokenCache = &refreshTokenCacheStub{REDACTED

	tokenPair, user, err := service.LoginOrRegisterOAuthWithTokenPair(context.Background(), "linuxdo-123@linuxdo-connect.invalid", "linuxdo_user", "", "", "linuxdo")
REDACTED
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Equal(t, int64(61), user.ID)
	require.Equal(t, 21.75, user.Balance)
	require.Equal(t, 9, user.Concurrency)
	require.Len(t, repo.created, 1)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(22), assigner.calls[0].GroupID)
	require.Equal(t, 14, assigner.calls[0].ValidityDays)
REDACTED

func TestAuthService_LoginOrRegisterOAuthWithTokenPair_ExistingUserDoesNotGrantAgain(t *testing.T) {
	existing := &User{
		ID:           88,
		Email:        "linuxdo-123@linuxdo-connect.invalid",
		Username:     "existing-linuxdo",
		Role:         RoleUser,
		Status:       StatusActive,
		Balance:      4,
		Concurrency:  1,
		TokenVersion: 2,
REDACTED
	repo := &userRepoStub{user: existingREDACTED
	assigner := &defaultSubscriptionAssignerStub{REDACTED
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                   "true",
		SettingKeyAuthSourceDefaultLinuxDoBalance:       "21.75",
		SettingKeyAuthSourceDefaultLinuxDoConcurrency:   "9",
		SettingKeyAuthSourceDefaultLinuxDoSubscriptions: `[{"group_id":22,"validity_days":14REDACTED]`,
		SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup: "true",
REDACTED, nil, nil)
	service.defaultSubAssigner = assigner
	service.refreshTokenCache = &refreshTokenCacheStub{REDACTED

	tokenPair, user, err := service.LoginOrRegisterOAuthWithTokenPair(context.Background(), existing.Email, "linuxdo_user", "", "", "linuxdo")
REDACTED
	require.NotNil(t, tokenPair)
	require.Equal(t, existing.ID, user.ID)
	require.Equal(t, 4.0, user.Balance)
	require.Equal(t, 1, user.Concurrency)
	require.Empty(t, repo.created)
	require.Empty(t, assigner.calls)
REDACTED

// newAuthServiceWithDingTalkCfg 构建一个含完整 DingTalk config 的 AuthService，
// 用于测试 canBypassRegistrationDisabledForOAuth。
func newAuthServiceWithDingTalkCfg(settings map[string]string, dtCfg config.DingTalkConnectConfig) *AuthService {
	cfg := &config.Config{
		JWT:      config.JWTConfig{Secret: "test-secret", ExpireHour: 1REDACTED,
		Default:  config.DefaultConfig{UserBalance: 3.5, UserConcurrency: 2REDACTED,
		DingTalk: dtCfg,
REDACTED
	settingService := NewSettingService(&settingRepoStub{values: settingsREDACTED, cfg)
	return NewAuthService(nil, nil, nil, nil, cfg, settingService, nil, nil, nil, nil, nil, nil, nil)
REDACTED

// minDingTalkURLs 返回一个包含必填字段的基础 DingTalkConnectConfig（不设 Enabled/BypassRegistration/Policy）。
func minDingTalkURLs() config.DingTalkConnectConfig {
	return config.DingTalkConnectConfig{
		ClientID:            "test-client",
		ClientSecret:        "test-secret",
		AuthorizeURL:        "https://example.com/oauth2/auth",
		TokenURL:            "https://example.com/oauth2/token",
		UserInfoURL:         "https://example.com/oauth2/userinfo",
		RedirectURL:         "https://example.com/callback",
		FrontendRedirectURL: "https://example.com/auth/callback",
		DingTalkAppKind:     "internal_app",
		AppType:             "internal",
REDACTED
REDACTED

func TestCanBypassRegistrationDisabledForOAuth(t *testing.T) {
	cases := []struct {
		name         string
		signupSource string
		settings     map[string]string
		dtCfg        config.DingTalkConnectConfig
		want         bool
REDACTED{
		{
			name:         "non-dingtalk source → false",
			signupSource: "linuxdo",
			settings:     map[string]string{REDACTED,
			dtCfg:        minDingTalkURLs(),
			want:         false,
	REDACTED,
		{
			name:         "dingtalk but cfg.Enabled=false → false",
			signupSource: "dingtalk",
			settings: map[string]string{
				SettingKeyDingTalkConnectEnabled:               "false",
				SettingKeyDingTalkConnectBypassRegistration:    "true",
				SettingKeyDingTalkConnectCorpRestrictionPolicy: "internal_only",
		REDACTED,
			dtCfg: minDingTalkURLs(),
			want:  false,
	REDACTED,
		{
			name:         "dingtalk enabled but BypassRegistration=false → false",
			signupSource: "dingtalk",
			settings: map[string]string{
				SettingKeyDingTalkConnectEnabled:               "true",
				SettingKeyDingTalkConnectBypassRegistration:    "false",
				SettingKeyDingTalkConnectCorpRestrictionPolicy: "internal_only",
		REDACTED,
			dtCfg: minDingTalkURLs(),
			want:  false,
	REDACTED,
		{
			name:         "dingtalk enabled + bypass=true but policy=none → false",
			signupSource: "dingtalk",
			settings: map[string]string{
				SettingKeyDingTalkConnectEnabled:               "true",
				SettingKeyDingTalkConnectBypassRegistration:    "true",
				SettingKeyDingTalkConnectCorpRestrictionPolicy: "none",
		REDACTED,
			dtCfg: minDingTalkURLs(),
			want:  false,
	REDACTED,
		{
			name:         "dingtalk enabled + bypass=true + policy=internal_only → true",
			signupSource: "dingtalk",
			settings: map[string]string{
				SettingKeyDingTalkConnectEnabled:               "true",
				SettingKeyDingTalkConnectBypassRegistration:    "true",
				SettingKeyDingTalkConnectCorpRestrictionPolicy: "internal_only",
		REDACTED,
			dtCfg: minDingTalkURLs(),
			want:  true,
	REDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newAuthServiceWithDingTalkCfg(tc.settings, tc.dtCfg)
			got := svc.canBypassRegistrationDisabledForOAuth(context.Background(), tc.signupSource)
			require.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED
