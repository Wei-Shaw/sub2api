package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/userdevice"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userDeviceRepository struct {
	client *dbent.Client
}

func NewUserDeviceRepository(client *dbent.Client) service.UserDeviceRepository {
	return &userDeviceRepository{client: client}
}

func (r *userDeviceRepository) GetByDeviceHash(ctx context.Context, deviceHash string) (*service.UserDevice, error) {
	client := clientFromContext(ctx, r.client)
	device, err := client.UserDevice.Query().
		Where(userdevice.DeviceHashEQ(deviceHash)).
		WithUser().
		WithClaimRedeemCode().
		WithLoginRedeemCode().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserDeviceNotFound, nil)
	}
	return userDeviceEntityToService(device), nil
}

func (r *userDeviceRepository) GetByLoginRedeemCodeID(ctx context.Context, codeID int64) (*service.UserDevice, error) {
	client := clientFromContext(ctx, r.client)
	device, err := client.UserDevice.Query().
		Where(userdevice.LoginRedeemCodeIDEQ(codeID)).
		WithUser().
		WithClaimRedeemCode().
		WithLoginRedeemCode().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserDeviceNotFound, nil)
	}
	return userDeviceEntityToService(device), nil
}

func (r *userDeviceRepository) Create(ctx context.Context, device *service.UserDevice) error {
	if device == nil {
		return nil
	}

	client := clientFromContext(ctx, r.client)
	create := client.UserDevice.Create().
		SetUserID(device.UserID).
		SetDeviceHash(device.DeviceHash).
		SetFingerprintVersion(device.FingerprintVersion).
		SetPlatform(device.Platform).
		SetArch(device.Arch).
		SetLoginRedeemCodeID(device.LoginRedeemCodeID).
		SetStatus(device.Status).
		SetFirstClaimedAt(device.FirstClaimedAt)
	if device.InstallID != nil {
		create.SetInstallID(*device.InstallID)
	}
	if device.AppVersion != nil {
		create.SetAppVersion(*device.AppVersion)
	}
	if device.ClaimRedeemCodeID != nil {
		create.SetClaimRedeemCodeID(*device.ClaimRedeemCodeID)
	}
	if device.LastClaimedAt != nil {
		create.SetLastClaimedAt(*device.LastClaimedAt)
	}
	if device.LastLoginAt != nil {
		create.SetLastLoginAt(*device.LastLoginAt)
	}

	created, err := create.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserDeviceNotFound, nil)
	}
	applyUserDeviceEntityToService(device, created)
	return nil
}

func (r *userDeviceRepository) UpdateLastClaimedAt(ctx context.Context, id int64, at time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserDevice.UpdateOneID(id).SetLastClaimedAt(at).Save(ctx)
	return translatePersistenceError(err, service.ErrUserDeviceNotFound, nil)
}

func (r *userDeviceRepository) UpdateLastLoginAt(ctx context.Context, id int64, at time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserDevice.UpdateOneID(id).SetLastLoginAt(at).Save(ctx)
	return translatePersistenceError(err, service.ErrUserDeviceNotFound, nil)
}

func userDeviceEntityToService(m *dbent.UserDevice) *service.UserDevice {
	if m == nil {
		return nil
	}
	out := &service.UserDevice{
		ID:                 m.ID,
		UserID:             m.UserID,
		DeviceHash:         m.DeviceHash,
		FingerprintVersion: m.FingerprintVersion,
		InstallID:          m.InstallID,
		Platform:           m.Platform,
		Arch:               m.Arch,
		AppVersion:         m.AppVersion,
		ClaimRedeemCodeID:  m.ClaimRedeemCodeID,
		LoginRedeemCodeID:  m.LoginRedeemCodeID,
		Status:             m.Status,
		FirstClaimedAt:     m.FirstClaimedAt,
		LastClaimedAt:      m.LastClaimedAt,
		LastLoginAt:        m.LastLoginAt,
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	}
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
	}
	if m.Edges.ClaimRedeemCode != nil {
		out.ClaimRedeemCode = redeemCodeEntityToService(m.Edges.ClaimRedeemCode)
	}
	if m.Edges.LoginRedeemCode != nil {
		out.LoginRedeemCode = redeemCodeEntityToService(m.Edges.LoginRedeemCode)
	}
	return out
}

func applyUserDeviceEntityToService(dst *service.UserDevice, src *dbent.UserDevice) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
	dst.FirstClaimedAt = src.FirstClaimedAt
	dst.LastClaimedAt = src.LastClaimedAt
	dst.LastLoginAt = src.LastLoginAt
}
