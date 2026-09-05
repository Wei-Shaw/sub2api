package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrDeviceProfileUnsupported = infraerrors.BadRequest(
	"DEVICE_PROFILE_UNSUPPORTED",
	"no active device profile is available for the detected client OS",
)

type CodexResolvedDeviceSlot struct {
	BindingID                 int64
	AccountID                 int64
	APIKeyID                  int64
	ProfileID                 int64
	SlotID                    int64
	OSClass                   CodexOSClass
	CanonicalSurface          CodexClientSurface
	Architecture              CodexArchitecture
	CatalogVersion            int64
	SlotIndex                 int
	Epoch                     int64
	State                     string
	PolicyVersion             int64
	ProxyID                   *int64
	Proxy                     *Proxy
	ClientVersionMode         CodexClientVersionMode
	ClientVersion             string
	ClientProfileVerification CodexClientProfileVerification
	ClientProfileSource       string
	EffectiveClientVersion    string
	LastSeenAt                time.Time
	AffinityTTLSeconds        int
	BindingCount              int64
}

// CodexConversationBindingRepository adds conversation affinity without changing
// the API-key's preferred slot. Bind is first-writer-wins and returns the winner.
type CodexConversationBindingRepository interface {
	FindCodexConversationBinding(context.Context, int64, int64, CodexOSClass, CodexClientSurface, string) (*CodexResolvedDeviceSlot, error)
	BindCodexConversationSlot(context.Context, int64, int64, CodexOSClass, CodexClientSurface, string, int64) (*CodexResolvedDeviceSlot, error)
	RefreshCodexConversationBinding(context.Context, int64) error
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
	slots, err := repo.ListCodexDeviceSlots(ctx, accountID, "", "", includeDraining)
	if err != nil {
		return nil, err
	}
	for index := range slots {
		if strings.TrimSpace(string(slots[index].ClientVersionMode)) == "" {
			slots[index].ClientVersionMode = CodexClientVersionInherit
		}
		version, err := resolveEffectiveCodexClientVersion(
			ctx,
			s.settingService,
			slots[index].ClientVersionMode,
			slots[index].ClientVersion,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"resolve Codex client version for %s/%s slot %d: %w",
				slots[index].OSClass,
				slots[index].CanonicalSurface,
				slots[index].SlotIndex,
				err,
			)
		}
		slots[index].EffectiveClientVersion = version
		// Production currently uses the built-in catalog. Keep its unverified
		// status explicit until a reviewed provider is wired into both paths.
		slots[index].ClientProfileVerification = CodexClientProfileUnverified
		slots[index].ClientProfileSource = "builtin"
	}
	return slots, nil
}

func (s *adminServiceImpl) FinalizeAccountCodexDeviceSlots(ctx context.Context, accountID int64) (int64, error) {
	repo, ok := s.accountRepo.(CodexDeviceBindingRepository)
	if !ok {
		return 0, infraerrors.New(501, "CODEX_DEVICE_SLOT_ADMIN_UNAVAILABLE", "Codex device slot repository is unavailable")
	}
	return repo.FinalizeDrainedCodexDeviceSlots(ctx, accountID)
}

func resolveEffectiveCodexClientVersion(
	ctx context.Context,
	settings *SettingService,
	mode CodexClientVersionMode,
	pinnedVersion string,
) (string, error) {
	mode = CodexClientVersionMode(strings.TrimSpace(string(mode)))
	switch mode {
	case "", CodexClientVersionInherit:
		// Inherit is resolved for each attempt. A global client upgrade therefore
		// affects the next request without mutating the persisted slot epoch.
		version := codexCLIVersion
		if settings != nil {
			version = settings.GetOpenAICodexClientVersion(ctx)
		}
		if version = NormalizeCodexClientVersion(version); version == "" {
			return "", errors.New("global Codex client version is invalid")
		}
		if CompareVersions(version, codexUpstreamMinVersion) < 0 {
			return "", fmt.Errorf("global Codex client version must be at least %s", codexUpstreamMinVersion)
		}
		return version, nil
	case CodexClientVersionPinned:
		version := NormalizeCodexClientVersion(pinnedVersion)
		if version == "" {
			return "", errors.New("pinned Codex client version is invalid")
		}
		if CompareVersions(version, codexUpstreamMinVersion) < 0 {
			return "", fmt.Errorf("pinned Codex client version must be at least %s", codexUpstreamMinVersion)
		}
		return version, nil
	default:
		return "", fmt.Errorf("unsupported Codex client version mode %q", mode)
	}
}
