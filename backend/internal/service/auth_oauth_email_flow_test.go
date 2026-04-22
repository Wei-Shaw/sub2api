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
	}
	updateCalls []*RedeemCode
}

func (s *redeemCodeRepoStub) Create(context.Context, *RedeemCode) error {
	panic("unexpected Create call")
}

func (s *redeemCodeRepoStub) CreateBatch(context.Context, []RedeemCode) error {
	panic("unexpected CreateBatch call")
}

func (s *redeemCodeRepoStub) GetByID(context.Context, int64) (*RedeemCode, error) {
	panic("unexpected GetByID call")
}

func (s *redeemCodeRepoStub) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	if s.codesByCode == nil {
		return nil, ErrRedeemCodeNotFound
	}
	redeemCode, ok := s.codesByCode[code]
	if !ok {
		return nil, ErrRedeemCodeNotFound
	}
	cloned := *redeemCode
	return &cloned, nil
}

func (s *redeemCodeRepoStub) Update(_ context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
	}
	cloned := *code
	s.updateCalls = append(s.updateCalls, &cloned)
	if s.codesByCode == nil {
		s.codesByCode = make(map[string]*RedeemCode)
	}
	s.codesByCode[cloned.Code] = &cloned
	return nil
}

func (s *redeemCodeRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (s *redeemCodeRepoStub) Use(_ context.Context, id, userID int64) error {
	for code, redeemCode := range s.codesByCode {
		if redeemCode.ID != id {
			continue
		}
		now := time.Now().UTC()
		redeemCode.Status = StatusUsed
		redeemCode.UsedBy = &userID
		redeemCode.UsedAt = &now
		s.codesByCode[code] = redeemCode
		s.useCalls = append(s.useCalls, struct {
			id     int64
			userID int64
		}{id: id, userID: userID})
		return nil
	}
	return ErrRedeemCodeNotFound
}

func (s *redeemCodeRepoStub) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *redeemCodeRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *redeemCodeRepoStub) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
}

func (s *redeemCodeRepoStub) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}

func (s *redeemCodeRepoStub) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

func newOAuthEmailFlowAuthService(
	userRepo UserRepository,
	redeemRepo RedeemCodeRepository,
	refreshTokenCache RefreshTokenCache,
	settings map[string]string,
	emailCache EmailCache,
) *AuthService {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
		},
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
		},
	}

	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)
	emailService := NewEmailService(&settingRepoStub{values: settings}, emailCache)

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
	)
}

func TestRegisterOAuthEmailAccountRollsBackCreatedUserWhenTokenPairGenerationFails(t *testing.T) {
	userRepo := &userRepoStub{nextID: 42}
	redeemRepo := &redeemCodeRepoStub{
		codesByCode: map[string]*RedeemCode{
			"INVITE123": {
				ID:     7,
				Code:   "INVITE123",
				Type:   RedeemTypeInvitation,
				Status: StatusUnused,
			},
		},
	}
	emailCache := &emailCacheStub{
		data: &VerificationCodeData{
			Code:      "246810",
			Attempts:  0,
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
		},
	}
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		redeemRepo,
		nil,
		map[string]string{
			SettingKeyRegistrationEnabled:   "true",
			SettingKeyInvitationCodeEnabled: "true",
			SettingKeyEmailVerifyEnabled:    "true",
		},
		emailCache,
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
	require.Error(t, err)
	require.Contains(t, err.Error(), "generate token pair")
	require.Equal(t, []int64{42}, userRepo.deletedIDs)
	require.Len(t, userRepo.created, 1)
	require.Empty(t, redeemRepo.useCalls)
	require.Empty(t, redeemRepo.updateCalls)
}

func TestRegisterOAuthEmailAccountSetsNormalizedSignupSourceOnCreatedUser(t *testing.T) {
	userRepo := &userRepoStub{nextID: 42}
	emailCache := &emailCacheStub{
		data: &VerificationCodeData{
			Code:      "246810",
			Attempts:  0,
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
		},
	}
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		&redeemCodeRepoStub{},
		&refreshTokenCacheStub{},
		map[string]string{
			SettingKeyRegistrationEnabled: "true",
			SettingKeyEmailVerifyEnabled:  "true",
		},
		emailCache,
	)

	tokenPair, user, err := authService.RegisterOAuthEmailAccount(
		context.Background(),
		"fresh@example.com",
		"secret-123",
		"246810",
		"",
		" OIDC ",
	)

	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Len(t, userRepo.created, 1)
	require.Equal(t, "oidc", userRepo.created[0].SignupSource)
}

func TestRegisterOAuthEmailAccountFallsBackUnknownSignupSourceToEmail(t *testing.T) {
	userRepo := &userRepoStub{nextID: 43}
	emailCache := &emailCacheStub{
		data: &VerificationCodeData{
			Code:      "246810",
			Attempts:  0,
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
		},
	}
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		&redeemCodeRepoStub{},
		&refreshTokenCacheStub{},
		map[string]string{
			SettingKeyRegistrationEnabled: "true",
			SettingKeyEmailVerifyEnabled:  "true",
		},
		emailCache,
	)

	tokenPair, user, err := authService.RegisterOAuthEmailAccount(
		context.Background(),
		"fallback@example.com",
		"secret-123",
		"246810",
		"",
		"github",
	)

	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Len(t, userRepo.created, 1)
	require.Equal(t, "email", userRepo.created[0].SignupSource)
}

func TestRollbackOAuthEmailAccountCreationRestoresInvitationUsage(t *testing.T) {
	userRepo := &userRepoStub{}
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
				}(),
				UsedAt: func() *time.Time {
					v := time.Now().UTC()
					return &v
				}(),
			},
		},
	}
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		redeemRepo,
		&refreshTokenCacheStub{},
		map[string]string{
			SettingKeyRegistrationEnabled:   "true",
			SettingKeyInvitationCodeEnabled: "true",
		},
		&emailCacheStub{},
	)

	err := authService.RollbackOAuthEmailAccountCreation(context.Background(), 42, "INVITE123")

	require.NoError(t, err)
	require.Equal(t, []int64{42}, userRepo.deletedIDs)
	require.Len(t, redeemRepo.updateCalls, 1)
	require.Equal(t, StatusUnused, redeemRepo.updateCalls[0].Status)
	require.Nil(t, redeemRepo.updateCalls[0].UsedBy)
	require.Nil(t, redeemRepo.updateCalls[0].UsedAt)
}

func TestRollbackOAuthEmailAccountCreationPropagatesDeleteError(t *testing.T) {
	userRepo := &userRepoStub{deleteErr: errors.New("delete failed")}
	authService := newOAuthEmailFlowAuthService(
		userRepo,
		&redeemCodeRepoStub{},
		&refreshTokenCacheStub{},
		map[string]string{
			SettingKeyRegistrationEnabled: "true",
		},
		&emailCacheStub{},
	)

	err := authService.RollbackOAuthEmailAccountCreation(context.Background(), 42, "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "delete created oauth user")
}
