//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type inviteLoginUserDeviceRepoStub struct {
	deviceByLoginCodeID map[int64]*UserDevice
	updatedLoginIDs     []int64
}

type inviteBootstrapAPIKeyServiceStub struct {
	groups       []Group
	createdKeys  []*APIKey
	createErrors map[int64]error
}

func (s *inviteBootstrapAPIKeyServiceStub) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return append([]Group(nil), s.groups...), nil
}

func (s *inviteBootstrapAPIKeyServiceStub) Create(_ context.Context, _ int64, req CreateAPIKeyRequest) (*APIKey, error) {
	groupID := int64(0)
	if req.GroupID != nil {
		groupID = *req.GroupID
	}
	if err, ok := s.createErrors[groupID]; ok {
		return nil, err
	}
	created := &APIKey{
		ID:      int64(len(s.createdKeys) + 1),
		Name:    req.Name,
		Key:     "sk-bootstrap-" + req.Name,
		GroupID: &groupID,
	}
	s.createdKeys = append(s.createdKeys, created)
	return created, nil
}

func (s *inviteLoginUserDeviceRepoStub) GetByDeviceHash(context.Context, string) (*UserDevice, error) {
	panic("unexpected GetByDeviceHash call")
}

func (s *inviteLoginUserDeviceRepoStub) GetByLoginRedeemCodeID(_ context.Context, codeID int64) (*UserDevice, error) {
	if s.deviceByLoginCodeID == nil {
		return nil, ErrUserDeviceNotFound
	}
	device, ok := s.deviceByLoginCodeID[codeID]
	if !ok {
		return nil, ErrUserDeviceNotFound
	}
	clone := *device
	return &clone, nil
}

func (s *inviteLoginUserDeviceRepoStub) GetByClaimRedeemCodeID(context.Context, int64) (*UserDevice, error) {
	panic("unexpected GetByClaimRedeemCodeID call")
}

func (s *inviteLoginUserDeviceRepoStub) Create(context.Context, *UserDevice) error {
	panic("unexpected Create call")
}

func (s *inviteLoginUserDeviceRepoStub) UpdateLastClaimedAt(context.Context, int64, time.Time) error {
	panic("unexpected UpdateLastClaimedAt call")
}

func (s *inviteLoginUserDeviceRepoStub) UpdateLastLoginAt(_ context.Context, id int64, _ time.Time) error {
	s.updatedLoginIDs = append(s.updatedLoginIDs, id)
	return nil
}

func newAuthServiceForInviteLoginTest(
	userRepo UserRepository,
	redeemRepo RedeemCodeRepository,
	userDeviceRepo UserDeviceRepository,
	settings map[string]string,
	bootstrapSvc InviteBootstrapAPIKeyService,
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
	authService := NewAuthService(
		nil,
		userRepo,
		redeemRepo,
		&refreshTokenCacheStub{},
		cfg,
		settingService,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	authService.SetInviteLoginDeviceResolver(userDeviceRepo)
	if bootstrapSvc != nil {
		authService.SetInviteBootstrapAPIKeyService(bootstrapSvc)
	}
	return authService
}

func TestAuthServiceInviteLoginAcceptsDeviceLoginCode(t *testing.T) {
	const (
		loginCode  = "DLG-FN7Y-NJQJ-XNV6"
		deviceHash = "ac0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"
		installID  = "000f0c66-0a84-4a72-a7bb-a82249dbc3c7"
	)

	usedAt := time.Now().UTC().Add(-24 * time.Hour)
	loginRedeem := &RedeemCode{
		ID:     50,
		Code:   loginCode,
		Type:   RedeemTypeDeviceLogin,
		Status: StatusUsed,
		UsedAt: &usedAt,
	}
	boundUser := &User{
		ID:       51,
		Email:    "bound@example.com",
		Username: "bound-user",
		Role:     RoleUser,
		Status:   StatusActive,
	}
	userRepo := &userRepoStub{user: boundUser}
	userDeviceRepo := &inviteLoginUserDeviceRepoStub{
		deviceByLoginCodeID: map[int64]*UserDevice{
			50: {
				ID:                2,
				UserID:            51,
				DeviceHash:        deviceHash,
				InstallID:         stringPtr(installID),
				LoginRedeemCodeID: 50,
				Status:            UserDeviceStatusActive,
			},
		},
	}
	bootstrapSvc := &inviteBootstrapAPIKeyServiceStub{
		groups: []Group{{
			ID:                 101,
			Platform:           "openai",
			Status:             StatusActive,
			ActiveAccountCount: 1,
		}},
	}
	authService := newAuthServiceForInviteLoginTest(
		userRepo,
		&redeemCodeRepoStub{codesByCode: map[string]*RedeemCode{loginCode: loginRedeem}},
		userDeviceRepo,
		map[string]string{SettingKeyRegistrationEnabled: "false"},
		bootstrapSvc,
	)

	result, err := authService.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: loginCode,
		DeviceHash:     deviceHash,
		InstallID:      installID,
		ClientKind:     "desktop",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.TokenPair)
	require.NotNil(t, result.User)
	require.Equal(t, int64(51), result.User.ID)
	require.Equal(t, []int64{2}, userDeviceRepo.updatedLoginIDs)
	require.Len(t, result.BootstrapAPIKeys, 1)
	require.Equal(t, "openai", result.BootstrapAPIKeys[0].Platform)
	require.Equal(t, "sk-bootstrap-bootstrap-openai", result.BootstrapAPIKeys[0].Key)
	require.Len(t, bootstrapSvc.createdKeys, 1)
}

func TestAuthServiceInviteLoginRejectsDeviceMismatch(t *testing.T) {
	const loginCode = "DLG-FN7Y-NJQJ-XNV6"
	usedAt := time.Now().UTC().Add(-24 * time.Hour)
	loginRedeem := &RedeemCode{
		ID:     50,
		Code:   loginCode,
		Type:   RedeemTypeDeviceLogin,
		Status: StatusUsed,
		UsedAt: &usedAt,
	}
	userRepo := &userRepoStub{user: &User{ID: 51, Email: "bound@example.com", Username: "bound-user", Role: RoleUser, Status: StatusActive}}
	userDeviceRepo := &inviteLoginUserDeviceRepoStub{
		deviceByLoginCodeID: map[int64]*UserDevice{
			50: {
				ID:                2,
				UserID:            51,
				DeviceHash:        "ac0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668",
				InstallID:         stringPtr("000f0c66-0a84-4a72-a7bb-a82249dbc3c7"),
				LoginRedeemCodeID: 50,
				Status:            UserDeviceStatusActive,
			},
		},
	}
	authService := newAuthServiceForInviteLoginTest(
		userRepo,
		&redeemCodeRepoStub{codesByCode: map[string]*RedeemCode{loginCode: loginRedeem}},
		userDeviceRepo,
		map[string]string{SettingKeyRegistrationEnabled: "false"},
		nil,
	)

	result, err := authService.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: loginCode,
		DeviceHash:     "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		InstallID:      "000f0c66-0a84-4a72-a7bb-a82249dbc3c7",
		ClientKind:     "desktop",
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrDeviceMismatch)
	require.Empty(t, userDeviceRepo.updatedLoginIDs)
}
