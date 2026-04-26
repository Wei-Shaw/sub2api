package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrVClawClaimNotImplemented = infraerrors.ServiceUnavailable(
	"VCLAW_CLAIM_NOT_IMPLEMENTED",
	"V-Claw claim flow is not implemented yet",
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
	if s.userRepo == nil || s.redeemRepo == nil || s.userDeviceRepo == nil {
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

	claimCode := NormalizeRedeemCode(req.ClaimCode)
	if claimCode == "" {
		return s.createFirstClaim(ctx, req, deviceHash, nil, now)
	}
	claimRedeemCode, err := s.redeemRepo.GetByCode(ctx, claimCode)
	if err != nil || claimRedeemCode == nil {
		return nil, ErrClaimCodeInvalid
	}
	if claimRedeemCode.Type != RedeemTypeDeviceClaim || claimRedeemCode.Status != StatusUnused {
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
	if binding.Status != UserDeviceStatusActive {
		return nil, ErrDeviceRevoked
	}
	loginCode, err := s.redeemRepo.GetByID(ctx, binding.LoginRedeemCodeID)
	if err != nil || loginCode == nil {
		return nil, ErrServiceUnavailable
	}
	if err := s.userDeviceRepo.UpdateLastClaimedAt(ctx, binding.ID, now); err != nil {
		return nil, ErrServiceUnavailable
	}
	return &VClawClaimResult{
		Status:          "ok",
		Mode:            "resume",
		UserID:          binding.UserID,
		DeviceLoginCode: loginCode.Code,
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
		if err := s.applyDeviceClaimBonus(runCtx, user, deviceHash, now); err != nil {
			return nil, err
		}
		loginCodeText, err := GenerateRedeemCodeForType(RedeemTypeDeviceLogin)
		if err != nil {
			return nil, fmt.Errorf("generate device login code: %w", err)
		}
		usedAt := now
		loginRedeemCode := &RedeemCode{
			Code:   loginCodeText,
			Type:   RedeemTypeDeviceLogin,
			Status: StatusUsed,
			UsedBy: &user.ID,
			UsedAt: &usedAt,
			Notes:  fmt.Sprintf("vclaw device login for device_hash=%s", deviceHash),
		}
		if err := s.redeemRepo.Create(runCtx, loginRedeemCode); err != nil {
			return nil, ErrServiceUnavailable
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
			DeviceHash:         deviceHash,
			FingerprintVersion: req.Device.FingerprintVersion,
			InstallID:          installID,
			Platform:           strings.TrimSpace(req.Device.Platform),
			Arch:               strings.TrimSpace(req.Device.Arch),
			AppVersion:         appVersion,
			ClaimRedeemCodeID:  claimRedeemCodeID,
			LoginRedeemCodeID:  loginRedeemCode.ID,
			Status:             UserDeviceStatusActive,
			FirstClaimedAt:     now,
			LastClaimedAt:      &now,
		}
		if err := s.userDeviceRepo.Create(runCtx, binding); err != nil {
			return nil, ErrServiceUnavailable
		}

		return &VClawClaimResult{
			Status:          "ok",
			Mode:            "first_claim",
			UserID:          user.ID,
			DeviceLoginCode: loginRedeemCode.Code,
			DeviceBindingID: binding.ID,
			ClaimedAt:       now,
		}, nil
	}

	if s.entClient == nil {
		return create(ctx)
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	result, err := create(txCtx)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrServiceUnavailable
	}
	return result, nil
}

func normalizeDeviceHash(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func optionalTrimmedString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (s *VClawClaimService) applyDeviceClaimBonus(ctx context.Context, user *User, deviceHash string, now time.Time) error {
	if user == nil || s.userRepo == nil || s.redeemRepo == nil {
		return ErrServiceUnavailable
	}
	bonus := 0.0
	if s.settingService != nil && s.settingService.settingRepo != nil {
		rawBonus, err := s.settingService.settingRepo.GetValue(ctx, SettingKeyDeviceClaimBonusBalance)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			return ErrServiceUnavailable
		}
		if err == nil {
			parsedBonus, parseErr := strconv.ParseFloat(strings.TrimSpace(rawBonus), 64)
			if parseErr != nil {
				return ErrServiceUnavailable
			}
			if parsedBonus > 0 {
				bonus = parsedBonus
			}
		}
	}
	if bonus <= 0 {
		return nil
	}
	if err := s.userRepo.UpdateBalance(ctx, user.ID, bonus); err != nil {
		return ErrServiceUnavailable
	}
	user.Balance += bonus
	bonusCode, err := GenerateRedeemCodeForType(AdjustmentTypeAdminBalance)
	if err != nil {
		return fmt.Errorf("generate device claim bonus record: %w", err)
	}
	usedAt := now
	adjustment := &RedeemCode{
		Code:   bonusCode,
		Type:   AdjustmentTypeAdminBalance,
		Value:  bonus,
		Status: StatusUsed,
		UsedBy: &user.ID,
		UsedAt: &usedAt,
		Notes:  fmt.Sprintf("vclaw device claim bonus for device_hash=%s", deviceHash),
	}
	if err := s.redeemRepo.Create(ctx, adjustment); err != nil {
		return ErrServiceUnavailable
	}
	return nil
}
