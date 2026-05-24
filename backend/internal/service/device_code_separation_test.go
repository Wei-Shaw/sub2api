//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Tests for Phase 1+2: device_code field on UserDevice + dual-write + dual-read
// =============================================================================

// --- Helper: isDeviceLoginCodePrefix ---

func isDeviceLoginCodePrefix(code string) bool {
	return len(code) >= 4 && strings.ToUpper(code[:4]) == "DLG-"
}

func TestIsDeviceLoginCodePrefix(t *testing.T) {
	tests := []struct {
		code   string
		expect bool
	}{
		{"DLG-FN7Y-NJQJ-XNV6", true},
		{"dlg-fn7y-njqj-xnv6", true},
		{"Dlg-Abcd-1234-5678", true},
		{"DLG-", true},
		{"DCL-ABCD-1234-5678", false},
		{"INV-ABCD-1234-5678", false},
		{"", false},
		{"DL", false},
		{"DLGX-1234", false},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			require.Equal(t, tt.expect, isDeviceLoginCodePrefix(tt.code))
		})
	}
}

// --- Tests for InviteLogin dual-path: device_code lookup ---

func TestInviteLoginUsesDeviceCodeDirectly(t *testing.T) {
	// When device_code is populated on user_devices, the new path should
	// find the device directly without needing redeem_codes lookup.
	const (
		deviceCode = "DLG-TEST-CODE-0001"
		deviceHash = "ac0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"
		installID  = "install-001"
	)

	boundUser := &User{
		ID:       100,
		Email:    "device-code-user@example.com",
		Username: "dc-user",
		Role:     RoleUser,
		Status:   StatusActive,
	}

	dc := deviceCode
	inst := installID
	userDeviceRepo := &inviteLoginUserDeviceRepoStub{
		deviceByLoginCodeID: map[int64]*UserDevice{
			50: {
				ID:                10,
				UserID:            100,
				DeviceCode:        &dc,
				DeviceHash:        deviceHash,
				InstallID:         &inst,
				LoginRedeemCodeID: 50,
				Status:            UserDeviceStatusActive,
			},
		},
	}

	// Redeem repo has the code too (legacy path), but we expect the new path to be used first
	usedAt := time.Now().UTC().Add(-24 * time.Hour)
	loginRedeem := &RedeemCode{
		ID:     50,
		Code:   deviceCode,
		Type:   RedeemTypeDeviceLogin,
		Status: StatusUsed,
		UsedAt: &usedAt,
	}
	redeemRepo := &dcTestRedeemRepo{
		codesByCode: map[string]*RedeemCode{deviceCode: loginRedeem},
		codesByID:   map[int64]*RedeemCode{50: loginRedeem},
	}

	bootstrapSvc := &inviteBootstrapAPIKeyServiceStub{
		groups: []Group{
			{
				ID:                 101,
				Platform:           "openai",
				Status:             StatusActive,
				SubscriptionType:   SubscriptionTypeStandard,
				ActiveAccountCount: 1,
			},
		},
	}

	authService := newAuthServiceForInviteLoginTest(
		&userRepoStub{user: boundUser},
		redeemRepo,
		userDeviceRepo,
		map[string]string{
			SettingKeyRegistrationEnabled: "false",
		},
		bootstrapSvc,
	)

	result, err := authService.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: deviceCode,
		DeviceHash:     deviceHash,
		InstallID:      installID,
		ClientKind:     "desktop",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.TokenPair)
	require.NotNil(t, result.User)
	require.Equal(t, int64(100), result.User.ID)
	require.Equal(t, []int64{10}, userDeviceRepo.updatedLoginIDs)
}

func TestInviteLoginReturnsErrorWhenDeviceCodeNotFound(t *testing.T) {
	// When device_code is NOT found in user_devices, should return error
	// (no fallback to redeem_codes).
	const (
		loginCode  = "DLG-FALL-BACK-0001"
		deviceHash = "bc1addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"
	)

	// Device repo has no matching device_code
	userDeviceRepo := &inviteLoginUserDeviceRepoStub{
		deviceByLoginCodeID: map[int64]*UserDevice{},
	}

	redeemRepo := &dcTestRedeemRepo{codesByCode: map[string]*RedeemCode{}}
	bootstrapSvc := &inviteBootstrapAPIKeyServiceStub{}

	authService := newAuthServiceForInviteLoginTest(
		&userRepoStub{user: &User{ID: 200, Status: StatusActive, Role: RoleUser}},
		redeemRepo,
		userDeviceRepo,
		map[string]string{
			SettingKeyRegistrationEnabled: "false",
		},
		bootstrapSvc,
	)

	result, err := authService.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: loginCode,
		DeviceHash:     deviceHash,
		ClientKind:     "desktop",
	})

	require.Error(t, err)
	require.Nil(t, result)
}

func TestInviteLoginByDeviceCodeRejectsRevokedDevice(t *testing.T) {
	const (
		deviceCode = "DLG-REVO-KEDC-0001"
		deviceHash = "dd0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"
	)

	dc := deviceCode
	userDeviceRepo := &inviteLoginUserDeviceRepoStub{
		deviceByLoginCodeID: map[int64]*UserDevice{
			70: {
				ID:                30,
				UserID:            300,
				DeviceCode:        &dc,
				DeviceHash:        deviceHash,
				LoginRedeemCodeID: 70,
				Status:            UserDeviceStatusRevoked, // REVOKED
			},
		},
	}

	redeemRepo := &dcTestRedeemRepo{codesByCode: map[string]*RedeemCode{}}
	bootstrapSvc := &inviteBootstrapAPIKeyServiceStub{}

	authService := newAuthServiceForInviteLoginTest(
		&userRepoStub{user: &User{ID: 300, Status: StatusActive, Role: RoleUser}},
		redeemRepo,
		userDeviceRepo,
		map[string]string{},
		bootstrapSvc,
	)

	_, err := authService.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: deviceCode,
		DeviceHash:     deviceHash,
		ClientKind:     "desktop",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeviceRevoked)
}

func TestInviteLoginByDeviceCodeRejectsDeviceHashMismatch(t *testing.T) {
	const (
		deviceCode    = "DLG-MISM-ATCH-0001"
		boundHash     = "ee0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"
		differentHash = "ff0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"
	)

	dc := deviceCode
	userDeviceRepo := &inviteLoginUserDeviceRepoStub{
		deviceByLoginCodeID: map[int64]*UserDevice{
			80: {
				ID:                40,
				UserID:            400,
				DeviceCode:        &dc,
				DeviceHash:        boundHash,
				LoginRedeemCodeID: 80,
				Status:            UserDeviceStatusActive,
			},
		},
	}

	redeemRepo := &dcTestRedeemRepo{codesByCode: map[string]*RedeemCode{}}
	bootstrapSvc := &inviteBootstrapAPIKeyServiceStub{}

	authService := newAuthServiceForInviteLoginTest(
		&userRepoStub{user: &User{ID: 400, Status: StatusActive, Role: RoleUser}},
		redeemRepo,
		userDeviceRepo,
		map[string]string{},
		bootstrapSvc,
	)

	_, err := authService.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: deviceCode,
		DeviceHash:     differentHash, // MISMATCH
		ClientKind:     "desktop",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeviceMismatch)
}

func TestInviteLoginByDeviceCodeWebLoginWithoutHash(t *testing.T) {
	// Web login should work without device_hash when client_kind=web
	const deviceCode = "DLG-WEBL-OGIN-0001"

	dc := deviceCode
	userDeviceRepo := &inviteLoginUserDeviceRepoStub{
		deviceByLoginCodeID: map[int64]*UserDevice{
			90: {
				ID:                50,
				UserID:            500,
				DeviceCode:        &dc,
				DeviceHash:        "aa0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668",
				LoginRedeemCodeID: 90,
				Status:            UserDeviceStatusActive,
			},
		},
	}

	usedAt := time.Now().UTC().Add(-24 * time.Hour)
	loginRedeem := &RedeemCode{
		ID:     90,
		Code:   deviceCode,
		Type:   RedeemTypeDeviceLogin,
		Status: StatusUsed,
		UsedAt: &usedAt,
	}
	redeemRepo := &dcTestRedeemRepo{
		codesByCode: map[string]*RedeemCode{},
		codesByID:   map[int64]*RedeemCode{90: loginRedeem},
	}

	bootstrapSvc := &inviteBootstrapAPIKeyServiceStub{}

	authService := newAuthServiceForInviteLoginTest(
		&userRepoStub{user: &User{ID: 500, Email: "web@example.com", Username: "web", Role: RoleUser, Status: StatusActive}},
		redeemRepo,
		userDeviceRepo,
		map[string]string{},
		bootstrapSvc,
	)

	result, err := authService.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: deviceCode,
		DeviceHash:     "", // No hash for web
		ClientKind:     "web",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(500), result.User.ID)
	// Web login should NOT provision bootstrap keys
	require.Empty(t, result.BootstrapAPIKeys)
}

func TestInviteLoginByDeviceCodeRejectsPendingUser(t *testing.T) {
	const (
		deviceCode = "DLG-PEND-USER-0001"
		deviceHash = "bb0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"
	)

	dc := deviceCode
	userDeviceRepo := &inviteLoginUserDeviceRepoStub{
		deviceByLoginCodeID: map[int64]*UserDevice{
			95: {
				ID:                55,
				UserID:            550,
				DeviceCode:        &dc,
				DeviceHash:        deviceHash,
				LoginRedeemCodeID: 95,
				Status:            UserDeviceStatusActive,
			},
		},
	}

	redeemRepo := &dcTestRedeemRepo{codesByCode: map[string]*RedeemCode{}}
	bootstrapSvc := &inviteBootstrapAPIKeyServiceStub{}

	authService := newAuthServiceForInviteLoginTest(
		&userRepoStub{user: &User{ID: 550, Status: StatusPendingActivation, Role: RoleUser}},
		redeemRepo,
		userDeviceRepo,
		map[string]string{},
		bootstrapSvc,
	)

	_, err := authService.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: deviceCode,
		DeviceHash:     deviceHash,
		ClientKind:     "desktop",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeviceActivationPending)
}

// --- Tests for VClawClaimService dual-write ---

func TestVClawClaimFirstClaimWritesDeviceCode(t *testing.T) {
	// When a new device is claimed, device_code should be written to UserDevice
	const deviceHash = "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	claimCode := &RedeemCode{
		ID:     1,
		Code:   "DCL-TEST-CLAM-0001",
		Type:   RedeemTypeDeviceClaim,
		Status: StatusUnused,
	}

	redeemRepo := &vclawClaimRedeemRepoStub{
		codes:     map[string]*RedeemCode{claimCode.Code: claimCode},
		codesByID: map[int64]*RedeemCode{claimCode.ID: claimCode},
	}
	userRepo := &userRepoStub{nextID: 500}
	deviceRepo := &vclawClaimUserDeviceRepoStub{}

	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}}, &config.Config{})

	svc := NewVClawClaimService(
		nil, // entClient
		userRepo,
		redeemRepo,
		deviceRepo,
		&config.Config{},
		settingService,
		nil, // subscriptionAssigner
		nil, // affiliateService
	)

	result, err := svc.Claim(context.Background(), VClawClaimRequest{
		ClaimCode: claimCode.Code,
		Device: VClawDeviceInput{
			DeviceHash:         deviceHash,
			FingerprintVersion: 1,
			Platform:           "windows",
			Arch:               "x64",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "first_claim", result.Mode)
	require.NotEmpty(t, result.DeviceLoginCode)
	// Verify the device_code was written to the created device
	require.NotEmpty(t, deviceRepo.created)
	lastCreated := deviceRepo.created[len(deviceRepo.created)-1]
	require.NotNil(t, lastCreated.DeviceCode)
	require.Equal(t, result.DeviceLoginCode, *lastCreated.DeviceCode)
	// Verify it starts with DLG-
	require.True(t, isDeviceLoginCodePrefix(*lastCreated.DeviceCode))
}

func TestVClawClaimResumeUsesDeviceCodeDirectly(t *testing.T) {
	// When resuming an existing claim, if device_code is populated,
	// it should be returned directly without redeem_codes lookup.
	deviceCode := "DLG-RESU-MEDC-0001"
	deviceHash := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	existingDevice := &UserDevice{
		ID:                10,
		UserID:            100,
		DeviceCode:        &deviceCode,
		DeviceHash:        deviceHash,
		LoginRedeemCodeID: 999, // Should NOT be looked up since device_code is set
		Status:            UserDeviceStatusActive,
	}

	// redeemRepo intentionally does NOT have code 999 — proves we don't look it up
	redeemRepo := &vclawClaimRedeemRepoStub{
		codes:     map[string]*RedeemCode{},
		codesByID: map[int64]*RedeemCode{},
	}
	deviceRepo := &vclawClaimUserDeviceRepoStub{
		byDeviceHash: map[string]*UserDevice{
			deviceHash: existingDevice,
		},
	}

	svc := NewVClawClaimService(
		nil,
		&mockUserRepo{getByIDUser: &User{ID: 100, Email: "resume@example.com", Username: "resume-user", Status: StatusActive, Role: RoleUser}},
		redeemRepo,
		deviceRepo,
		nil,
		nil,
		nil,
		nil,
	)

	result, err := svc.Claim(context.Background(), VClawClaimRequest{
		ClaimCode: "DCL-DOES-NTMA-TTER",
		Device: VClawDeviceInput{
			DeviceHash:         deviceHash,
			FingerprintVersion: 1,
			Platform:           "windows",
			Arch:               "x64",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resume", result.Mode)
	require.Equal(t, deviceCode, result.DeviceLoginCode)
}

func TestVClawClaimResumeReturnsErrorWhenNoDeviceCode(t *testing.T) {
	// When resuming and device_code is nil (legacy device without migration),
	// should return error since we no longer fall back to redeem_codes.
	deviceHash := "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"

	existingDevice := &UserDevice{
		ID:                20,
		UserID:            200,
		DeviceCode:        nil, // Legacy — no device_code
		DeviceHash:        deviceHash,
		LoginRedeemCodeID: 888,
		Status:            UserDeviceStatusActive,
	}

	redeemRepo := &vclawClaimRedeemRepoStub{}
	deviceRepo := &vclawClaimUserDeviceRepoStub{
		byDeviceHash: map[string]*UserDevice{
			deviceHash: existingDevice,
		},
	}

	svc := NewVClawClaimService(
		nil,
		&mockUserRepo{getByIDUser: &User{ID: 200, Email: "legacy@example.com", Username: "legacy-user", Status: StatusActive, Role: RoleUser}},
		redeemRepo,
		deviceRepo,
		nil,
		nil,
		nil,
		nil,
	)

	result, err := svc.Claim(context.Background(), VClawClaimRequest{
		ClaimCode: "DCL-IGNO-RETH-IS01",
		Device: VClawDeviceInput{
			DeviceHash:         deviceHash,
			FingerprintVersion: 1,
			Platform:           "macos",
			Arch:               "arm64",
		},
	})

	require.Error(t, err)
	require.Nil(t, result)
}

// =============================================================================
// Test-specific stubs
// =============================================================================

// dcTestRedeemRepo is a RedeemCodeRepository stub that supports both GetByCode and GetByID.
type dcTestRedeemRepo struct {
	codesByCode map[string]*RedeemCode
	codesByID   map[int64]*RedeemCode
	useCalls    []struct {
		id     int64
		userID int64
	}
	updateCalls []*RedeemCode
	created     []*RedeemCode
}

func (s *dcTestRedeemRepo) Create(_ context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
	}
	clone := *code
	if clone.ID == 0 {
		clone.ID = int64(len(s.created) + 9000)
	}
	code.ID = clone.ID
	if s.codesByCode == nil {
		s.codesByCode = map[string]*RedeemCode{}
	}
	if s.codesByID == nil {
		s.codesByID = map[int64]*RedeemCode{}
	}
	s.codesByCode[clone.Code] = &clone
	s.codesByID[clone.ID] = &clone
	s.created = append(s.created, &clone)
	return nil
}
func (s *dcTestRedeemRepo) CreateBatch(context.Context, []RedeemCode) error { return nil }
func (s *dcTestRedeemRepo) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	return 0, nil
}
func (s *dcTestRedeemRepo) Update(_ context.Context, code *RedeemCode) error {
	if code != nil {
		clone := *code
		s.updateCalls = append(s.updateCalls, &clone)
	}
	return nil
}
func (s *dcTestRedeemRepo) Delete(context.Context, int64) error { return nil }
func (s *dcTestRedeemRepo) GetByID(_ context.Context, id int64) (*RedeemCode, error) {
	if s.codesByID == nil {
		return nil, ErrRedeemCodeNotFound
	}
	code, ok := s.codesByID[id]
	if !ok {
		return nil, ErrRedeemCodeNotFound
	}
	clone := *code
	return &clone, nil
}
func (s *dcTestRedeemRepo) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	if s.codesByCode == nil {
		return nil, ErrRedeemCodeNotFound
	}
	normalized := NormalizeRedeemCode(code)
	stored, ok := s.codesByCode[normalized]
	if !ok {
		return nil, ErrRedeemCodeNotFound
	}
	clone := *stored
	return &clone, nil
}
func (s *dcTestRedeemRepo) Use(_ context.Context, id int64, userID int64) error {
	s.useCalls = append(s.useCalls, struct {
		id     int64
		userID int64
	}{id, userID})
	return nil
}
func (s *dcTestRedeemRepo) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *dcTestRedeemRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *int64) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *dcTestRedeemRepo) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	return nil, nil
}
func (s *dcTestRedeemRepo) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *dcTestRedeemRepo) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	return 0, nil
}
