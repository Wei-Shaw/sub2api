//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type vclawClaimRedeemRepoStub struct {
	codes    map[string]*RedeemCode
	codesByID map[int64]*RedeemCode
	created  []*RedeemCode
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
func (s *vclawClaimRedeemRepoStub) Update(context.Context, *RedeemCode) error { return nil }
func (s *vclawClaimRedeemRepoStub) Delete(context.Context, int64) error { return nil }
func (s *vclawClaimRedeemRepoStub) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *vclawClaimRedeemRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
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
	byDeviceHash       map[string]*UserDevice
	byClaimRedeemCodeID map[int64]*UserDevice
	updatedClaimedIDs  []int64
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

func (s *vclawClaimUserDeviceRepoStub) Create(context.Context, *UserDevice) error { return nil }
func (s *vclawClaimUserDeviceRepoStub) UpdateLastClaimedAt(_ context.Context, id int64, _ time.Time) error {
	s.updatedClaimedIDs = append(s.updatedClaimedIDs, id)
	return nil
}
func (s *vclawClaimUserDeviceRepoStub) UpdateLastLoginAt(context.Context, int64, time.Time) error { return nil }

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

	svc := NewVClawClaimService(nil, &mockUserRepo{}, redeemRepo, deviceRepo, nil, nil)
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

	svc := NewVClawClaimService(nil, &mockUserRepo{}, redeemRepo, deviceRepo, nil, nil)
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

func validDeviceHash(seed byte) string {
	buf := make([]byte, 64)
	for i := range buf {
		buf[i] = 'a' + seed%6
	}
	return string(buf)
}

func ptrInt64VClaw(v int64) *int64 { return &v }
