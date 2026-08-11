//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type redeemCodeRepoStub struct {
	codesByCode map[string]*RedeemCode
	useCalls    []struct {
		id     int64
		userID int64
REDACTED
	updateCalls []*RedeemCode
REDACTED

func (s *redeemCodeRepoStub) Create(context.Context, *RedeemCode) error {
	panic("unexpected Create call")
REDACTED

func (s *redeemCodeRepoStub) CreateBatch(context.Context, []RedeemCode) error {
	panic("unexpected CreateBatch call")
REDACTED

func (s *redeemCodeRepoStub) GetByID(context.Context, int64) (*RedeemCode, error) {
	panic("unexpected GetByID call")
REDACTED

func (s *redeemCodeRepoStub) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	if s.codesByCode == nil {
		return nil, ErrRedeemCodeNotFound
REDACTED
	redeemCode, ok := s.codesByCode[code]
	if !ok {
		return nil, ErrRedeemCodeNotFound
REDACTED
	cloned := *redeemCode
	return &cloned, nil
REDACTED

func (s *redeemCodeRepoStub) Update(_ context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
REDACTED
	cloned := *code
	s.updateCalls = append(s.updateCalls, &cloned)
	if s.codesByCode == nil {
		s.codesByCode = make(map[string]*RedeemCode)
REDACTED
	s.codesByCode[cloned.Code] = &cloned
	return nil
REDACTED

func (s *redeemCodeRepoStub) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected BatchUpdate call")
REDACTED

func (s *redeemCodeRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
REDACTED

func (s *redeemCodeRepoStub) Use(_ context.Context, id, userID int64) error {
	for code, redeemCode := range s.codesByCode {
		if redeemCode.ID != id {
			continue
	REDACTED
		now := time.Now().UTC()
		redeemCode.Status = StatusUsed
		redeemCode.UsedBy = &userID
		redeemCode.UsedAt = &now
		s.codesByCode[code] = redeemCode
		s.useCalls = append(s.useCalls, struct {
			id     int64
			userID int64
	REDACTED{id: id, userID: userIDREDACTED)
		return nil
REDACTED
	return ErrRedeemCodeNotFound
REDACTED

func (s *redeemCodeRepoStub) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (s *redeemCodeRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
REDACTED

func (s *redeemCodeRepoStub) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
REDACTED

func (s *redeemCodeRepoStub) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
REDACTED

func (s *redeemCodeRepoStub) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
REDACTED

func newOAuthEmailFlowAuthService(
	userRepo UserRepository,
	redeemRepo RedeemCodeRepository,
	refreshTokenCache RefreshTokenCache,
	settings map[string]string,
	emailCache EmailCache,
	quotaRepo UserPlatformQuotaRepository, // 新增
) *AuthService {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
	REDACTED,
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
	REDACTED,
REDACTED

	settingService := NewSettingService(&settingRepoStub{values: settingsREDACTED, cfg)
	emailService := NewEmailService(&settingRepoStub{values: settingsREDACTED, emailCache)

	return NewAuthService(
		nil,
		userRepo,
		redeemRepo,
		refreshTokenCache,
		cfg,
		settingService,
		emailService,
		nil,
		nil,
		nil,
		nil,
		nil,
		quotaRepo, // 替换原来的 nil
	)
REDACTED

func TestRegisterOAuthEmailAccountRollsBackCreatedUserWhenTokenPairGenerationFails(t *testing.T) {
	userRepo := &userRepoStub{nextID: 42REDACTED
	redeemRepo := &redeemCodeRepoStub{
		codesByCode: map[string]*RedeemCode{
			"INVITE123": {
				ID:     7,
				Code:   "INVITE123",
				Type:   RedeemTypeInvitation,
				Status: StatusUnused,
		REDACTED,
	REDACTED,
REDACTED
	emailCache := &emailCacheStub{
		data: &VerificationCodeData{
			Code:      "246810",
			Attempts:  0,
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
	REDACTED,
REDACTED
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		redeemRepo,
		nil,
		map[string]string{
			SettingKeyRegistrationEnabled:   "true",
			SettingKeyInvitationCodeEnabled: "true",
			SettingKeyEmailVerifyEnabled:    "true",
	REDACTED,
		emailCache,
		nil,
	)

	tokenPair, user, err := authService.RegisterOAuthEmailAccount(
		context.Background(),
		"fresh@example.com",
		"secret-123",
		"246810",
		"INVITE123",
		"oidc",
	)

	require.Nil(t, tokenPair)
	require.Nil(t, user)
REDACTED
	require.Contains(t, err.Error(), "generate token pair")
	require.Equal(t, []int64{42REDACTED, userRepo.deletedIDs)
	require.Len(t, userRepo.created, 1)
	require.Empty(t, redeemRepo.useCalls)
	require.Empty(t, redeemRepo.updateCalls)
REDACTED

func TestRegisterOAuthEmailAccount_NonWhitelistDomainLimit(t *testing.T) {
	userRepo := &userRepoStub{domainCounts: map[string]int{"custom.example": 1REDACTEDREDACTED
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		&redeemCodeRepoStub{REDACTED,
		&refreshTokenCacheStub{REDACTED,
		map[string]string{
			SettingKeyRegistrationEnabled:                 "true",
			SettingKeyRegistrationEmailSuffixWhitelist:    `["@example.com"]`,
			SettingKeyRegistrationEmailDomainQuotaEnabled: "true",
	REDACTED,
		&emailCacheStub{data: &VerificationCodeData{
			Code:      "246810",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
REDACTED
		nil,
	)

	_, _, err := authService.RegisterOAuthEmailAccount(
		context.Background(),
		"second@custom.example",
		"secret-123",
		"246810",
		"",
		"oidc",
	)

	require.ErrorIs(t, err, ErrEmailDomainRegistrationLimit)
REDACTED

func TestRegisterVerifiedOAuthEmailAccount_NonWhitelistDomainLimit(t *testing.T) {
	userRepo := &userRepoStub{domainCounts: map[string]int{"custom.example": 1REDACTEDREDACTED
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		nil,
		&refreshTokenCacheStub{REDACTED,
		map[string]string{
			SettingKeyRegistrationEnabled:                 "true",
			SettingKeyRegistrationEmailSuffixWhitelist:    `["@example.com"]`,
			SettingKeyRegistrationEmailDomainQuotaEnabled: "true",
	REDACTED,
		&emailCacheStub{REDACTED,
		nil,
	)

	_, _, err := authService.RegisterVerifiedOAuthEmailAccount(
		context.Background(),
		"second@custom.example",
		"secret-123",
		"",
		"oidc",
	)

	require.ErrorIs(t, err, ErrEmailDomainRegistrationLimit)
REDACTED

func TestSendPendingOAuthVerifyCode_NonWhitelistDomainLimit(t *testing.T) {
	userRepo := &userRepoStub{domainCounts: map[string]int{"custom.example": 1REDACTEDREDACTED
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		nil,
		nil,
		map[string]string{
			SettingKeyRegistrationEnabled:                 "true",
			SettingKeyRegistrationEmailSuffixWhitelist:    `["@example.com"]`,
			SettingKeyRegistrationEmailDomainQuotaEnabled: "true",
	REDACTED,
		&emailCacheStub{REDACTED,
		nil,
	)

	_, err := authService.SendPendingOAuthVerifyCode(context.Background(), "second@custom.example")
	require.ErrorIs(t, err, ErrEmailDomainRegistrationLimit)
REDACTED

// 域名限量注册开关默认关闭：白名单外域名在 pending OAuth 发码阶段即被严格拒绝。
func TestSendPendingOAuthVerifyCode_NonWhitelistDomainRejectedWhenQuotaDisabled(t *testing.T) {
	userRepo := &userRepoStub{domainCounts: map[string]int{"custom.example": 0REDACTEDREDACTED
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		nil,
		nil,
		map[string]string{
			SettingKeyRegistrationEnabled:              "true",
			SettingKeyRegistrationEmailSuffixWhitelist: `["@example.com"]`,
	REDACTED,
		&emailCacheStub{REDACTED,
		nil,
	)

	_, err := authService.SendPendingOAuthVerifyCode(context.Background(), "first@custom.example")
	require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
REDACTED

func TestSendPendingOAuthVerifyCode_NilServiceReturnsUnavailable(t *testing.T) {
	var authService *AuthService

	_, err := authService.SendPendingOAuthVerifyCode(context.Background(), "fresh@example.com")

	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestRegisterOAuthEmailAccountSetsNormalizedSignupSourceOnCreatedUser(t *testing.T) {
	userRepo := &userRepoStub{nextID: 42REDACTED
	emailCache := &emailCacheStub{
		data: &VerificationCodeData{
			Code:      "246810",
			Attempts:  0,
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
	REDACTED,
REDACTED
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		&redeemCodeRepoStub{REDACTED,
		&refreshTokenCacheStub{REDACTED,
		map[string]string{
			SettingKeyRegistrationEnabled: "true",
			SettingKeyEmailVerifyEnabled:  "true",
	REDACTED,
		emailCache,
		nil,
	)

	tokenPair, user, err := authService.RegisterOAuthEmailAccount(
		context.Background(),
		"fresh@example.com",
		"secret-123",
		"246810",
		"",
		" OIDC ",
	)

REDACTED
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Len(t, userRepo.created, 1)
	require.Equal(t, "oidc", userRepo.created[0].SignupSource)
REDACTED

func TestRegisterOAuthEmailAccountKeepsGitHubAndGoogleSignupSource(t *testing.T) {
	tests := []struct {
		name         string
		email        string
		signupSource string
		want         string
REDACTED{
		{
			name:         "github",
			email:        "github@example.com",
			signupSource: " GitHub ",
			want:         "github",
	REDACTED,
		{
			name:         "google",
			email:        "google@example.com",
			signupSource: " Google ",
			want:         "google",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRepo := &userRepoStub{nextID: 43REDACTED
			emailCache := &emailCacheStub{
				data: &VerificationCodeData{
					Code:      "246810",
					Attempts:  0,
					CreatedAt: time.Now().UTC(),
					ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
			REDACTED,
		REDACTED
			authService := newOAuthEmailFlowAuthService(
				userRepo,
				&redeemCodeRepoStub{REDACTED,
				&refreshTokenCacheStub{REDACTED,
				map[string]string{
					SettingKeyRegistrationEnabled: "true",
					SettingKeyEmailVerifyEnabled:  "true",
			REDACTED,
				emailCache,
				nil,
			)

			tokenPair, user, err := authService.RegisterOAuthEmailAccount(
				context.Background(),
				tt.email,
				"secret-123",
				"246810",
				"",
				tt.signupSource,
			)

		REDACTED
			require.NotNil(t, tokenPair)
			require.NotNil(t, user)
			require.Len(t, userRepo.created, 1)
			require.Equal(t, tt.want, userRepo.created[0].SignupSource)
	REDACTED)
REDACTED
REDACTED

func TestRegisterOAuthEmailAccountFallsBackUnknownSignupSourceToEmail(t *testing.T) {
	userRepo := &userRepoStub{nextID: 43REDACTED
	emailCache := &emailCacheStub{
		data: &VerificationCodeData{
			Code:      "246810",
			Attempts:  0,
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
	REDACTED,
REDACTED
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		&redeemCodeRepoStub{REDACTED,
		&refreshTokenCacheStub{REDACTED,
		map[string]string{
			SettingKeyRegistrationEnabled: "true",
			SettingKeyEmailVerifyEnabled:  "true",
	REDACTED,
		emailCache,
		nil,
	)

	tokenPair, user, err := authService.RegisterOAuthEmailAccount(
		context.Background(),
		"fallback@example.com",
		"secret-123",
		"246810",
		"",
		"unknown-provider",
	)

REDACTED
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Len(t, userRepo.created, 1)
	require.Equal(t, "email", userRepo.created[0].SignupSource)
REDACTED

func TestRollbackOAuthEmailAccountCreationRestoresInvitationUsage(t *testing.T) {
	userRepo := &userRepoStub{REDACTED
	redeemRepo := &redeemCodeRepoStub{
		codesByCode: map[string]*RedeemCode{
			"INVITE123": {
				ID:     7,
				Code:   "INVITE123",
				Type:   RedeemTypeInvitation,
				Status: StatusUsed,
				UsedBy: func() *int64 {
					v := int64(42)
					return &v
			REDACTED(),
				UsedAt: func() *time.Time {
					v := time.Now().UTC()
					return &v
			REDACTED(),
		REDACTED,
	REDACTED,
REDACTED
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		redeemRepo,
		&refreshTokenCacheStub{REDACTED,
		map[string]string{
			SettingKeyRegistrationEnabled:   "true",
			SettingKeyInvitationCodeEnabled: "true",
	REDACTED,
		&emailCacheStub{REDACTED,
		nil,
	)

	err := authService.RollbackOAuthEmailAccountCreation(context.Background(), 42, "INVITE123")

REDACTED
	require.Equal(t, []int64{42REDACTED, userRepo.deletedIDs)
	require.Len(t, redeemRepo.updateCalls, 1)
	require.Equal(t, StatusUnused, redeemRepo.updateCalls[0].Status)
	require.Nil(t, redeemRepo.updateCalls[0].UsedBy)
	require.Nil(t, redeemRepo.updateCalls[0].UsedAt)
REDACTED

func TestRollbackOAuthEmailAccountCreationPropagatesDeleteError(t *testing.T) {
	userRepo := &userRepoStub{deleteErr: errors.New("delete failed")REDACTED
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		&redeemCodeRepoStub{REDACTED,
		&refreshTokenCacheStub{REDACTED,
		map[string]string{
			SettingKeyRegistrationEnabled: "true",
	REDACTED,
		&emailCacheStub{REDACTED,
		nil,
	)

	err := authService.RollbackOAuthEmailAccountCreation(context.Background(), 42, "")

REDACTED
	require.Contains(t, err.Error(), "delete created oauth user")
REDACTED

func TestFinalizeOAuthEmailAccount_SnapshotsPlatformQuotaDefaults(t *testing.T) {
	userRepo := &userRepoStub{nextID: 99REDACTED
	quotaRepo := &userPlatformQuotaRepoStub{REDACTED

	authService := newOAuthEmailFlowAuthService(
		userRepo,
		nil,
		&refreshTokenCacheStub{REDACTED,
		map[string]string{
			SettingKeyRegistrationEnabled:   "true",
			SettingKeyEmailVerifyEnabled:    "true",
			SettingKeyDefaultPlatformQuotas: `{"anthropic": {"daily": 5.5REDACTEDREDACTED`,
	REDACTED,
		&emailCacheStub{REDACTED,
		quotaRepo,
	)

	user := &User{
		ID:           99,
		Email:        "newuser@example.com",
		Role:         RoleUser,
		Status:       StatusActive,
		SignupSource: "oidc",
REDACTED

	err := authService.FinalizeOAuthEmailAccount(
		context.Background(),
		user,
		"",
		"oidc",
		"",
	)

REDACTED

	require.Len(t, quotaRepo.bulkInsertCalls, 1, "snapshotPlatformQuotaDefaults must call BulkInsertInitial once on successful OAuth signup")

	records := quotaRepo.bulkInsertCalls[0]
	var anthropicRecord *UserPlatformQuotaRecord
	for i := range records {
		if records[i].Platform == "anthropic" {
			anthropicRecord = &records[i]
			break
	REDACTED
REDACTED
	require.NotNil(t, anthropicRecord, "expected anthropic platform record")
	require.Equal(t, int64(99), anthropicRecord.UserID)
	require.NotNil(t, anthropicRecord.DailyLimitUSD)
	require.InDelta(t, 5.5, *anthropicRecord.DailyLimitUSD, 0.0001)
REDACTED
