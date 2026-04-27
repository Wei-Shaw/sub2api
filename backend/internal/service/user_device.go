package service

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	RedeemTypeDeviceClaim = "device_claim"
	RedeemTypeDeviceLogin = "device_login"

	UserDeviceStatusActive  = "active"
	UserDeviceStatusRevoked = "revoked"
	UserDeviceStatusBlocked = "blocked"
)

var (
	ErrUserDeviceNotFound = infraerrors.NotFound("USER_DEVICE_NOT_FOUND", "user device not found")
	ErrClaimCodeRequired  = infraerrors.BadRequest("CLAIM_CODE_REQUIRED", "claim_code is required for first claim")
	ErrClaimCodeInvalid   = infraerrors.BadRequest("CLAIM_CODE_INVALID", "invalid or used claim code")
	ErrDeviceHashRequired = infraerrors.BadRequest("DEVICE_HASH_REQUIRED", "device_hash is required")
	ErrDeviceHashInvalid  = infraerrors.BadRequest("DEVICE_HASH_INVALID", "device_hash must be a 64-character hex string")
	ErrDeviceRevoked      = infraerrors.Forbidden("DEVICE_REVOKED", "device binding has been revoked")
	ErrDeviceMismatch     = infraerrors.Forbidden("DEVICE_MISMATCH", "device does not match bound login code")
)

type UserDevice struct {
	ID                 int64
	UserID             int64
	DeviceHash         string
	FingerprintVersion int
	InstallID          *string
	Platform           string
	Arch               string
	AppVersion         *string
	ClaimRedeemCodeID  *int64
	LoginRedeemCodeID  int64
	Status             string
	FirstClaimedAt     time.Time
	LastClaimedAt      *time.Time
	LastLoginAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time

	User            *User
	ClaimRedeemCode *RedeemCode
	LoginRedeemCode *RedeemCode
}

func (d *UserDevice) IsActive() bool {
	return d != nil && strings.TrimSpace(d.Status) == UserDeviceStatusActive
}

type UserDeviceRepository interface {
	GetByDeviceHash(ctx context.Context, deviceHash string) (*UserDevice, error)
	GetByLoginRedeemCodeID(ctx context.Context, codeID int64) (*UserDevice, error)
	GetByClaimRedeemCodeID(ctx context.Context, codeID int64) (*UserDevice, error)
	Create(ctx context.Context, device *UserDevice) error
	UpdateLastClaimedAt(ctx context.Context, id int64, at time.Time) error
	UpdateLastLoginAt(ctx context.Context, id int64, at time.Time) error
}
