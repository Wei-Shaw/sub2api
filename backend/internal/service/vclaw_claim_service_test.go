//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type vclawClaimRedeemRepoStub struct {
	codes     map[string]*RedeemCode
	codesByID map[int64]*RedeemCode
	created   []*RedeemCode
}

func (s *vclawClaimRedeemRepoStub) Create(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
	}
	clone := *code
	if clone.ID == 0 {
		clone.ID = int64(len(s.created) + 100)
	}
	code.ID = clone.ID
	if s.codes == nil {
		s.codes = map[string]*RedeemCode{}
	}
	if s.codesByID == nil {
		s.codesByID = map[int64]*RedeemCode{}
	}
	s.codes[clone.Code] = &clone
	s.codesByID[clone.ID] = &clone
	s.created = append(s.created, &clone)
	return nil
}

func (s *vclawClaimRedeemRepoStub) CreateBatch(context.Context, []RedeemCode) error { return nil }
func (s *vclawClaimRedeemRepoStub) Update(context.Context, *RedeemCode) error       { return nil }
func (s *vclawClaimRedeemRepoStub) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	return 0, nil
}
func (s *vclawClaimRedeemRepoStub) Delete(context.Context, int64) error { return nil }
func (s *vclawClaimRedeemRepoStub) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *vclawClaimRedeemRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *int64) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *vclawClaimRedeemRepoStub) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	return nil, nil
}
func (s *vclawClaimRedeemRepoStub) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *vclawClaimRedeemRepoStub) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	return 0, nil
}
func (s *vclawClaimRedeemRepoStub) Use(context.Context, int64, int64) error { return nil }

func (s *vclawClaimRedeemRepoStub) GetByID(ctx context.Context, id int64) (*RedeemCode, error) {
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

func (s *vclawClaimRedeemRepoStub) GetByCode(ctx context.Context, code string) (*RedeemCode, error) {
	if s.codes == nil {
		return nil, ErrRedeemCodeNotFound
	}
	normalized := NormalizeRedeemCode(code)
	stored, ok := s.codes[normalized]
	if !ok {
		return nil, ErrRedeemCodeNotFound
	}
	clone := *stored
	return &clone, nil
}

type vclawClaimUserDeviceRepoStub struct {
	byDeviceHash        map[string]*UserDevice
	byClaimRedeemCodeID map[int64]*UserDevice
	created             []*UserDevice
	updatedClaimedIDs   []int64
}

type vclawClaimAffiliateRepoStub struct {
	profilesByCode map[string]*AffiliateSummary
	codesByCode    map[string]*AffiliateCode
	profilesByUser map[int64]*AffiliateSummary
	bindings       []vclawClaimAffiliateBinding
}

type vclawClaimAffiliateBinding struct {
	UserID    int64
	InviterID int64
}

func (s *vclawClaimAffiliateRepoStub) EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error) {
	if s.profilesByUser == nil {
		s.profilesByUser = map[int64]*AffiliateSummary{}
	}
	if summary, ok := s.profilesByUser[userID]; ok {
		clone := *summary
		return &clone, nil
	}
	summary := &AffiliateSummary{UserID: userID, AffCode: "SELF-CODE"}
	s.profilesByUser[userID] = summary
	clone := *summary
	return &clone, nil
}

func (s *vclawClaimAffiliateRepoStub) GetAffiliateByCode(ctx context.Context, code string) (*AffiliateSummary, error) {
	if s.profilesByCode == nil {
		return nil, ErrAffiliateProfileNotFound
	}
	profile, ok := s.profilesByCode[code]
	if !ok {
		return nil, ErrAffiliateProfileNotFound
	}
	clone := *profile
	return &clone, nil
}

func (s *vclawClaimAffiliateRepoStub) GetAffiliateCode(ctx context.Context, code string) (*AffiliateCode, error) {
	if s.codesByCode == nil {
		return nil, ErrAffiliateProfileNotFound
	}
	entry, ok := s.codesByCode[code]
	if !ok {
		return nil, ErrAffiliateProfileNotFound
	}
	clone := *entry
	return &clone, nil
}

func (s *vclawClaimAffiliateRepoStub) BindInviter(ctx context.Context, userID, inviterID int64) (bool, error) {
	s.bindings = append(s.bindings, vclawClaimAffiliateBinding{UserID: userID, InviterID: inviterID})
	if s.profilesByUser == nil {
		s.profilesByUser = map[int64]*AffiliateSummary{}
	}
	profile := s.profilesByUser[userID]
	if profile == nil {
		profile = &AffiliateSummary{UserID: userID, AffCode: "SELF-CODE"}
		s.profilesByUser[userID] = profile
	}
	profile.InviterID = &inviterID
	return true, nil
}

func (s *vclawClaimAffiliateRepoStub) AccrueQuota(context.Context, int64, int64, float64, int, *int64) (bool, error) {
	panic("unexpected AccrueQuota call")
}
func (s *vclawClaimAffiliateRepoStub) GetAccruedRebateFromInvitee(context.Context, int64, int64) (float64, error) {
	panic("unexpected GetAccruedRebateFromInvitee call")
}
func (s *vclawClaimAffiliateRepoStub) ThawFrozenQuota(context.Context, int64) (float64, error) {
	return 0, nil
}
func (s *vclawClaimAffiliateRepoStub) TransferQuotaToBalance(context.Context, int64) (float64, float64, error) {
	panic("unexpected TransferQuotaToBalance call")
}
func (s *vclawClaimAffiliateRepoStub) ListInvitees(context.Context, int64, int) ([]AffiliateInvitee, error) {
	return nil, nil
}
func (s *vclawClaimAffiliateRepoStub) EnsureUserAutoActiveAffCode(context.Context, int64) (*AffiliateCode, error) {
	panic("unexpected EnsureUserAutoActiveAffCode call")
}
func (s *vclawClaimAffiliateRepoStub) DeleteUserAutoActiveAffCode(context.Context, int64) error {
	panic("unexpected DeleteUserAutoActiveAffCode call")
}
func (s *vclawClaimAffiliateRepoStub) UpdateUserAffCode(context.Context, int64, string) error {
	panic("unexpected UpdateUserAffCode call")
}
func (s *vclawClaimAffiliateRepoStub) ResetUserAffCode(context.Context, int64) (string, error) {
	panic("unexpected ResetUserAffCode call")
}
func (s *vclawClaimAffiliateRepoStub) SetUserRebateRate(context.Context, int64, *float64) error {
	panic("unexpected SetUserRebateRate call")
}
func (s *vclawClaimAffiliateRepoStub) BatchSetUserRebateRate(context.Context, []int64, *float64) error {
	panic("unexpected BatchSetUserRebateRate call")
}
func (s *vclawClaimAffiliateRepoStub) ListUsersWithCustomSettings(context.Context, AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	panic("unexpected ListUsersWithCustomSettings call")
}
func (s *vclawClaimAffiliateRepoStub) ListAffiliateInviteRecords(context.Context, AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error) {
	panic("unexpected ListAffiliateInviteRecords call")
}
func (s *vclawClaimAffiliateRepoStub) ListAffiliateRebateRecords(context.Context, AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error) {
	panic("unexpected ListAffiliateRebateRecords call")
}
func (s *vclawClaimAffiliateRepoStub) ListAffiliateTransferRecords(context.Context, AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error) {
	panic("unexpected ListAffiliateTransferRecords call")
}
func (s *vclawClaimAffiliateRepoStub) GetAffiliateUserOverview(context.Context, int64) (*AffiliateUserOverview, error) {
	panic("unexpected GetAffiliateUserOverview call")
}

func (s *vclawClaimUserDeviceRepoStub) GetByDeviceHash(ctx context.Context, deviceHash string) (*UserDevice, error) {
	if s.byDeviceHash == nil {
		return nil, ErrUserDeviceNotFound
	}
	device, ok := s.byDeviceHash[deviceHash]
	if !ok {
		return nil, ErrUserDeviceNotFound
	}
	clone := *device
	return &clone, nil
}

func (s *vclawClaimUserDeviceRepoStub) GetByLoginRedeemCodeID(context.Context, int64) (*UserDevice, error) {
	return nil, ErrUserDeviceNotFound
}

func (s *vclawClaimUserDeviceRepoStub) GetByDeviceCode(context.Context, string) (*UserDevice, error) {
	return nil, ErrUserDeviceNotFound
}

func (s *vclawClaimUserDeviceRepoStub) GetByClaimRedeemCodeID(ctx context.Context, codeID int64) (*UserDevice, error) {
	if s.byClaimRedeemCodeID == nil {
		return nil, ErrUserDeviceNotFound
	}
	device, ok := s.byClaimRedeemCodeID[codeID]
	if !ok {
		return nil, ErrUserDeviceNotFound
	}
	clone := *device
	return &clone, nil
}

func (s *vclawClaimUserDeviceRepoStub) Create(_ context.Context, device *UserDevice) error {
	if device == nil {
		return nil
	}
	clone := *device
	if clone.ID == 0 {
		clone.ID = int64(len(s.created) + 1000)
	}
	device.ID = clone.ID
	if s.byDeviceHash == nil {
		s.byDeviceHash = map[string]*UserDevice{}
	}
	s.byDeviceHash[normalizeDeviceHash(clone.DeviceHash)] = &clone
	if clone.ClaimRedeemCodeID != nil {
		if s.byClaimRedeemCodeID == nil {
			s.byClaimRedeemCodeID = map[int64]*UserDevice{}
		}
		s.byClaimRedeemCodeID[*clone.ClaimRedeemCodeID] = &clone
	}
	s.created = append(s.created, &clone)
	return nil
}
func (s *vclawClaimUserDeviceRepoStub) UpdateLastClaimedAt(_ context.Context, id int64, _ time.Time) error {
	s.updatedClaimedIDs = append(s.updatedClaimedIDs, id)
	return nil
}
func (s *vclawClaimUserDeviceRepoStub) UpdateLastLoginAt(context.Context, int64, time.Time) error {
	return nil
}

func TestVClawClaimServiceResumesUsedClaimCodeByBinding(t *testing.T) {
	now := time.Now().UTC()
	claimCode := &RedeemCode{
		ID:     11,
		Code:   "DLG-FN7Y-NJQJ-XNV6",
		Type:   RedeemTypeDeviceClaim,
		Status: StatusUsed,
	}
	loginCode := &RedeemCode{
		ID:     22,
		Code:   "DLL-LOGIN-CODE-1234",
		Type:   RedeemTypeDeviceLogin,
		Status: StatusUsed,
		UsedAt: &now,
	}
	binding := &UserDevice{
		ID:                77,
		UserID:            88,
		DeviceCode:        ptrStringVClaw("DLL-LOGIN-CODE-1234"),
		DeviceHash:        "existing-device-hash",
		ClaimRedeemCodeID: ptrInt64VClaw(claimCode.ID),
		LoginRedeemCodeID: loginCode.ID,
		Status:            UserDeviceStatusActive,
	}

	redeemRepo := &vclawClaimRedeemRepoStub{
		codes: map[string]*RedeemCode{claimCode.Code: claimCode},
		codesByID: map[int64]*RedeemCode{
			claimCode.ID: claimCode,
			loginCode.ID: loginCode,
		},
	}
	deviceRepo := &vclawClaimUserDeviceRepoStub{
		byClaimRedeemCodeID: map[int64]*UserDevice{claimCode.ID: binding},
	}

	svc := NewVClawClaimService(nil, &mockUserRepo{getByIDUser: &User{ID: binding.UserID, Status: StatusActive}}, redeemRepo, deviceRepo, nil, nil, nil, nil)
	result, err := svc.Claim(context.Background(), VClawClaimRequest{
		ClaimCode: claimCode.Code,
		Device: VClawDeviceInput{
			DeviceHash:         validDeviceHash(1),
			FingerprintVersion: 1,
			Platform:           "windows",
			Arch:               "amd64",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resume", result.Mode)
	require.Equal(t, StatusActive, result.Status)
	require.Equal(t, UserDeviceStatusActive, result.DeviceStatus)
	require.Equal(t, binding.UserID, result.UserID)
	require.Equal(t, loginCode.Code, result.DeviceLoginCode)
	require.Equal(t, binding.ID, result.DeviceBindingID)
	require.Equal(t, []int64{binding.ID}, deviceRepo.updatedClaimedIDs)
}

func TestVClawClaimServiceRejectsUsedClaimCodeWithoutBinding(t *testing.T) {
	claimCode := &RedeemCode{
		ID:     11,
		Code:   "DLG-FN7Y-NJQJ-XNV6",
		Type:   RedeemTypeDeviceClaim,
		Status: StatusUsed,
	}

	redeemRepo := &vclawClaimRedeemRepoStub{
		codes: map[string]*RedeemCode{claimCode.Code: claimCode},
		codesByID: map[int64]*RedeemCode{
			claimCode.ID: claimCode,
		},
	}
	deviceRepo := &vclawClaimUserDeviceRepoStub{}

	svc := NewVClawClaimService(nil, &mockUserRepo{}, redeemRepo, deviceRepo, nil, nil, nil, nil)
	result, err := svc.Claim(context.Background(), VClawClaimRequest{
		ClaimCode: claimCode.Code,
		Device: VClawDeviceInput{
			DeviceHash:         validDeviceHash(2),
			FingerprintVersion: 1,
			Platform:           "windows",
			Arch:               "amd64",
		},
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrClaimCodeInvalid)
}

func TestVClawClaimServiceFirstClaimAssignsDefaultSubscriptionOnce(t *testing.T) {
	const deviceHash = "ac0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"

	userRepo := &userRepoStub{nextID: 102}
	redeemRepo := &vclawClaimRedeemRepoStub{}
	deviceRepo := &vclawClaimUserDeviceRepoStub{}
	assigner := &defaultSubscriptionAssignerStub{}
	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultSubscriptions: `[{"group_id":8,"validity_days":3}]`,
	}}, &config.Config{})
	svc := NewVClawClaimService(nil, userRepo, redeemRepo, deviceRepo, &config.Config{}, settingService, assigner, nil)
	req := VClawClaimRequest{
		Device: VClawDeviceInput{
			DeviceHash:         deviceHash,
			FingerprintVersion: 1,
			Platform:           "windows",
			Arch:               "amd64",
		},
	}

	first, err := svc.Claim(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, "first_claim", first.Mode)
	require.Equal(t, StatusActive, first.Status)
	require.Equal(t, UserDeviceStatusActive, first.DeviceStatus)
	require.Equal(t, int64(102), first.UserID)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(8), assigner.calls[0].GroupID)
	require.Equal(t, 3, assigner.calls[0].ValidityDays)
	require.Equal(t, "auto assigned by first device claim", assigner.calls[0].Notes)

	resume, err := svc.Claim(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resume)
	require.Equal(t, "resume", resume.Mode)
	require.Equal(t, StatusActive, resume.Status)
	require.Equal(t, UserDeviceStatusActive, resume.DeviceStatus)
	require.Equal(t, first.DeviceLoginCode, resume.DeviceLoginCode)
	require.Len(t, assigner.calls, 1)
}

func TestVClawClaimServiceFirstClaimBindsAffiliateCodeOnce(t *testing.T) {
	const deviceHash = "bc0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"

	userRepo := &userRepoStub{nextID: 202}
	redeemRepo := &vclawClaimRedeemRepoStub{}
	deviceRepo := &vclawClaimUserDeviceRepoStub{}
	affiliateRepo := &vclawClaimAffiliateRepoStub{
		profilesByCode: map[string]*AffiliateSummary{
			"AFF-INVITER": {UserID: 901, AffCode: "AFF-INVITER"},
		},
	}
	affiliateService := NewAffiliateService(affiliateRepo, NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyAffiliateEnabled: "true",
	}}, &config.Config{}), nil, nil)
	svc := NewVClawClaimService(nil, userRepo, redeemRepo, deviceRepo, &config.Config{}, nil, nil, affiliateService)
	req := VClawClaimRequest{
		AffCode: " aff-inviter ",
		Device: VClawDeviceInput{
			DeviceHash:         deviceHash,
			FingerprintVersion: 1,
			Platform:           "windows",
			Arch:               "amd64",
		},
	}

	first, err := svc.Claim(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, "first_claim", first.Mode)
	require.Equal(t, StatusPendingActivation, first.Status)
	require.Equal(t, UserDeviceStatusActive, first.DeviceStatus)
	require.Equal(t, int64(202), first.UserID)
	require.Equal(t, []vclawClaimAffiliateBinding{{UserID: 202, InviterID: 901}}, affiliateRepo.bindings)

	resume, err := svc.Claim(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resume)
	require.Equal(t, "resume", resume.Mode)
	require.Equal(t, StatusPendingActivation, resume.Status)
	require.Equal(t, UserDeviceStatusActive, resume.DeviceStatus)
	require.Equal(t, first.DeviceLoginCode, resume.DeviceLoginCode)
	require.Equal(t, []vclawClaimAffiliateBinding{{UserID: 202, InviterID: 901}}, affiliateRepo.bindings)
}

func TestVClawClaimServiceFirstClaimAutoActivatesAffiliateCodeFlag(t *testing.T) {
	const deviceHash = "cc0addf134d4ac9d6ac98ffdb1f4796dd2b27d6ab2b66ec0bab9e181a007b668"

	userRepo := &userRepoStub{nextID: 203}
	redeemRepo := &vclawClaimRedeemRepoStub{}
	deviceRepo := &vclawClaimUserDeviceRepoStub{}
	affiliateRepo := &vclawClaimAffiliateRepoStub{
		profilesByCode: map[string]*AffiliateSummary{
			"AFF-AUTO": {UserID: 902, AffCode: "AFF-MANUAL"},
		},
		codesByCode: map[string]*AffiliateCode{
			"AFF-AUTO": {UserID: 902, AffCode: "AFF-AUTO", IsAutoActive: true},
		},
	}
	affiliateService := NewAffiliateService(affiliateRepo, NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyAffiliateEnabled: "true",
	}}, &config.Config{}), nil, nil)
	svc := NewVClawClaimService(nil, userRepo, redeemRepo, deviceRepo, &config.Config{}, nil, nil, affiliateService)
	req := VClawClaimRequest{
		AffCode: " aff-auto ",
		Device: VClawDeviceInput{
			DeviceHash:         deviceHash,
			FingerprintVersion: 1,
			Platform:           "windows",
			Arch:               "amd64",
		},
	}

	first, err := svc.Claim(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, "first_claim", first.Mode)
	require.Equal(t, StatusActive, first.Status)
	require.Equal(t, UserDeviceStatusActive, first.DeviceStatus)
	require.Equal(t, int64(203), first.UserID)
	require.Equal(t, []vclawClaimAffiliateBinding{{UserID: 203, InviterID: 902}}, affiliateRepo.bindings)
}

func validDeviceHash(seed byte) string {
	buf := make([]byte, 64)
	for i := range buf {
		buf[i] = 'a' + seed%6
	}
	return string(buf)
}

func ptrInt64VClaw(v int64) *int64 { return &v }

func ptrStringVClaw(v string) *string { return &v }
