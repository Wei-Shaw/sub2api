package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrDeviceProfileUnsupported = infraerrors.BadRequest(
	"DEVICE_PROFILE_UNSUPPORTED",
	"no active device profile is available for the detected client OS",
)

type CodexResolvedDeviceSlot struct {
	BindingID          int64
	AccountID          int64
	APIKeyID           int64
	ProfileID          int64
	SlotID             int64
	OSClass            CodexOSClass
	CanonicalSurface   CodexClientSurface
	Architecture       CodexArchitecture
	CatalogVersion     int64
	SlotIndex          int
	Epoch              int64
	State              string
	PolicyVersion      int64
	ProxyID            *int64
	Proxy              *Proxy
	LastSeenAt         time.Time
	AffinityTTLSeconds int
	BindingCount       int64
}

type CodexDeviceSlotAdminService interface {
	ListAccountCodexDeviceSlots(ctx context.Context, accountID int64, includeDraining bool) ([]CodexResolvedDeviceSlot, error)
	FinalizeAccountCodexDeviceSlots(ctx context.Context, accountID int64) (int64, error)
}

type CodexDeviceBindingRepository interface {
	ResolveCodexDeviceBinding(ctx context.Context, accountID, apiKeyID int64, osClass CodexOSClass, surface CodexClientSurface) (*CodexResolvedDeviceSlot, error)
	RebindCodexDeviceBinding(ctx context.Context, oldAccountID, newAccountID, apiKeyID int64, osClass CodexOSClass, surface CodexClientSurface) (*CodexResolvedDeviceSlot, error)
	DeleteCodexDeviceBinding(ctx context.Context, accountID, apiKeyID int64, osClass CodexOSClass, surface CodexClientSurface) error
	ListCodexDeviceSlots(ctx context.Context, accountID int64, osClass CodexOSClass, surface CodexClientSurface, includeDraining bool) ([]CodexResolvedDeviceSlot, error)
	FinalizeDrainedCodexDeviceSlots(ctx context.Context, accountID int64) (int64, error)
}

func (s *adminServiceImpl) ListAccountCodexDeviceSlots(
	ctx context.Context,
	accountID int64,
	includeDraining bool,
) ([]CodexResolvedDeviceSlot, error) {
	repo, ok := s.accountRepo.(CodexDeviceBindingRepository)
	if !ok {
		return nil, infraerrors.New(501, "CODEX_DEVICE_SLOT_ADMIN_UNAVAILABLE", "Codex device slot repository is unavailable")
	}
	return repo.ListCodexDeviceSlots(ctx, accountID, "", "", includeDraining)
}

func (s *adminServiceImpl) FinalizeAccountCodexDeviceSlots(ctx context.Context, accountID int64) (int64, error) {
	repo, ok := s.accountRepo.(CodexDeviceBindingRepository)
	if !ok {
		return 0, infraerrors.New(501, "CODEX_DEVICE_SLOT_ADMIN_UNAVAILABLE", "Codex device slot repository is unavailable")
	}
	return repo.FinalizeDrainedCodexDeviceSlots(ctx, accountID)
}
