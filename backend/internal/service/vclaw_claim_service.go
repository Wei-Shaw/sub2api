package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type VClawClaimService struct {
	entClient      *dbent.Client
	userRepo       UserRepository
	redeemRepo     RedeemCodeRepository
	userDeviceRepo UserDeviceRepository
	cfg            *config.Config
	settingService *SettingService
}

func NewVClawClaimService(
	entClient *dbent.Client,
	userRepo UserRepository,
	redeemRepo RedeemCodeRepository,
	userDeviceRepo UserDeviceRepository,
	cfg *config.Config,
	settingService *SettingService,
) *VClawClaimService {
	return &VClawClaimService{
		entClient:      entClient,
		userRepo:       userRepo,
		redeemRepo:     redeemRepo,
		userDeviceRepo: userDeviceRepo,
		cfg:            cfg,
		settingService: settingService,
	}
}

type VClawDeviceInput struct {
	DeviceHash         string
	FingerprintVersion int
	InstallID          string
	Platform           string
	Arch               string
	AppVersion         string
}

type VClawClaimRequest struct {
	ClaimCode string
	Device    VClawDeviceInput
}

type VClawClaimResult struct {
	Status          string    `json:"status"`
	DeviceStatus    string    `json:"device_status,omitempty"`
	Mode            string    `json:"mode"`
	UserID          int64     `json:"user_id,omitempty"`
	DeviceLoginCode string    `json:"device_login_code,omitempty"`
	DeviceBindingID int64     `json:"device_binding_id,omitempty"`
	ClaimedAt       time.Time `json:"claimed_at,omitempty"`
}

func (s *VClawClaimService) Claim(ctx context.Context, req VClawClaimRequest) (*VClawClaimResult, error) {
	if err := validateVClawClaimRequest(req); err != nil {
		return nil, err
	}
	if s == nil || s.userRepo == nil || s.userDeviceRepo == nil {
		return nil, ErrServiceUnavailable
	}

	deviceHash := normalizeDeviceHash(req.Device.DeviceHash)
	now := time.Now().UTC()
	binding, err := s.userDeviceRepo.GetByDeviceHash(ctx, deviceHash)
	if err == nil {
		return s.resumeExistingClaim(ctx, binding, now)
	}
	if !errors.Is(err, ErrUserDeviceNotFound) {
		return nil, ErrServiceUnavailable
	}

	claimCode := strings.ToUpper(strings.TrimSpace(req.ClaimCode))
	if claimCode == "" {
		return s.createFirstClaim(ctx, req, deviceHash, nil, now)
	}
	if s.redeemRepo == nil {
		return nil, ErrServiceUnavailable
	}
	claimRedeemCode, err := s.redeemRepo.GetByCode(ctx, claimCode)
	if err != nil || claimRedeemCode == nil || claimRedeemCode.Type != RedeemTypeDeviceClaim {
		return nil, ErrClaimCodeInvalid
	}
	if claimRedeemCode.Status == StatusUsed {
		binding, bindingErr := s.userDeviceRepo.GetByClaimRedeemCodeID(ctx, claimRedeemCode.ID)
		if bindingErr != nil {
			if errors.Is(bindingErr, ErrUserDeviceNotFound) {
				return nil, ErrClaimCodeInvalid
			}
			return nil, ErrServiceUnavailable
		}
		return s.resumeExistingClaim(ctx, binding, now)
	}
	if claimRedeemCode.Status != StatusUnused || claimRedeemCode.IsExpiredAt(now) {
		return nil, ErrClaimCodeInvalid
	}
	return s.createFirstClaim(ctx, req, deviceHash, claimRedeemCode, now)
}

func validateVClawClaimRequest(req VClawClaimRequest) error {
	deviceHash := normalizeDeviceHash(req.Device.DeviceHash)
	if deviceHash == "" {
		return ErrDeviceHashRequired
	}
	if len(deviceHash) != 64 {
		return ErrDeviceHashInvalid
	}
	for _, ch := range deviceHash {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return ErrDeviceHashInvalid
		}
	}
	if strings.TrimSpace(req.Device.Platform) == "" {
		return infraerrors.BadRequest("DEVICE_PLATFORM_REQUIRED", "platform is required")
	}
	if strings.TrimSpace(req.Device.Arch) == "" {
		return infraerrors.BadRequest("DEVICE_ARCH_REQUIRED", "arch is required")
	}
	if req.Device.FingerprintVersion <= 0 {
		return infraerrors.BadRequest("FINGERPRINT_VERSION_INVALID", "fingerprint_version must be greater than zero")
	}
	return nil
}

func (s *VClawClaimService) resumeExistingClaim(ctx context.Context, binding *UserDevice, now time.Time) (*VClawClaimResult, error) {
	if binding == nil {
		return nil, ErrUserDeviceNotFound
	}
	if !binding.IsActive() {
		return nil, ErrDeviceRevoked
	}
	user, err := s.userRepo.GetByID(ctx, binding.UserID)
	if err != nil || user == nil {
		return nil, ErrServiceUnavailable
	}
	if binding.DeviceCode == nil || strings.TrimSpace(*binding.DeviceCode) == "" {
		return nil, ErrServiceUnavailable
	}
	if err := s.userDeviceRepo.UpdateLastClaimedAt(ctx, binding.ID, now); err != nil {
		return nil, ErrServiceUnavailable
	}
	return &VClawClaimResult{
		Status:          user.Status,
		DeviceStatus:    binding.Status,
		Mode:            "resume",
		UserID:          binding.UserID,
		DeviceLoginCode: *binding.DeviceCode,
		DeviceBindingID: binding.ID,
		ClaimedAt:       now,
	}, nil
}

func (s *VClawClaimService) createFirstClaim(ctx context.Context, req VClawClaimRequest, deviceHash string, claimRedeemCode *RedeemCode, now time.Time) (*VClawClaimResult, error) {
	create := func(runCtx context.Context) (*VClawClaimResult, error) {
		var (
			user *User
			err  error
		)
		if claimRedeemCode != nil {
			user, err = createInviteBootstrapUserWithRedeem(runCtx, s.entClient, s.userRepo, s.redeemRepo, s.cfg, s.settingService, claimRedeemCode)
		} else {
			user, err = createInviteBootstrapUserWithoutRedeem(runCtx, s.userRepo, s.cfg, s.settingService)
		}
		if err != nil {
			return nil, err
		}

		loginCode, err := generateDeviceLoginCode()
		if err != nil {
			return nil, fmt.Errorf("generate device login code: %w", err)
		}
		installID := optionalTrimmedString(req.Device.InstallID)
		appVersion := optionalTrimmedString(req.Device.AppVersion)
		var claimRedeemCodeID *int64
		if claimRedeemCode != nil {
			id := claimRedeemCode.ID
			claimRedeemCodeID = &id
		}
		binding := &UserDevice{
			UserID:             user.ID,
			DeviceCode:         &loginCode,
			DeviceHash:         deviceHash,
			FingerprintVersion: req.Device.FingerprintVersion,
			InstallID:          installID,
			Platform:           strings.TrimSpace(req.Device.Platform),
			Arch:               strings.TrimSpace(req.Device.Arch),
			AppVersion:         appVersion,
			ClaimRedeemCodeID:  claimRedeemCodeID,
			Status:             UserDeviceStatusActive,
			FirstClaimedAt:     now,
			LastClaimedAt:      &now,
		}
		if err := s.userDeviceRepo.Create(runCtx, binding); err != nil {
			return nil, ErrServiceUnavailable
		}
		return &VClawClaimResult{
			Status:          user.Status,
			DeviceStatus:    UserDeviceStatusActive,
			Mode:            "first_claim",
			UserID:          user.ID,
			DeviceLoginCode: loginCode,
			DeviceBindingID: binding.ID,
			ClaimedAt:       now,
		}, nil
	}

	if s.entClient == nil || dbent.TxFromContext(ctx) != nil {
		return create(ctx)
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	result, err := create(dbent.NewTxContext(ctx, tx))
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrServiceUnavailable
	}
	return result, nil
}

func generateDeviceLoginCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	const blockSize = 4
	const blockCount = 3
	buf := make([]byte, blockSize*blockCount)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return fmt.Sprintf("DLG-%s-%s-%s", buf[0:4], buf[4:8], buf[8:12]), nil
}

func normalizeDeviceHash(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func optionalTrimmedString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
