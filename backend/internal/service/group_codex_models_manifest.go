package service

import (
	"context"
	"fmt"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// maxCodexModelsManifestAccounts is the upper bound of pinned accounts per group.
const maxCodexModelsManifestAccounts = 10

// normalizeCodexModelsManifestConfig normalizes a group's pinned-accounts Codex
// manifest config before persistence:
//   - Non-OpenAI platforms always persist a disabled zero config (spec: silently
//     normalize instead of rejecting).
//   - Account IDs are de-duplicated preserving first-seen order; invalid IDs (<= 0)
//     are dropped. The list is kept even when disabled so toggling enabled back
//     on does not lose the previous selection.
func normalizeCodexModelsManifestConfig(platform string, cfg GroupCodexModelsManifestConfig) GroupCodexModelsManifestConfig {
	if platform != PlatformOpenAI {
		return GroupCodexModelsManifestConfig{}
	}
	out := GroupCodexModelsManifestConfig{
		Enabled:             cfg.Enabled,
		FallbackToScheduler: cfg.FallbackToScheduler,
	}
	if len(cfg.AccountIDs) == 0 {
		return out
	}
	seen := make(map[int64]struct{}, len(cfg.AccountIDs))
	out.AccountIDs = make([]int64, 0, len(cfg.AccountIDs))
	for _, id := range cfg.AccountIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out.AccountIDs = append(out.AccountIDs, id)
	}
	if len(out.AccountIDs) == 0 {
		out.AccountIDs = nil
	}
	return out
}

// validateCodexModelsManifestConfig enforces the admin contract for an enabled
// pinned-accounts config: at least one account, at most
// maxCodexModelsManifestAccounts, and every account must be an active OpenAI
// member of the group. Validation failures return 400 with reason
// INVALID_CODEX_MODELS_MANIFEST_CONFIG and never reach persistence.
func (s *adminServiceImpl) validateCodexModelsManifestConfig(ctx context.Context, groupID int64, cfg GroupCodexModelsManifestConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if len(cfg.AccountIDs) == 0 {
		return infraerrors.New(http.StatusBadRequest, "INVALID_CODEX_MODELS_MANIFEST_CONFIG", "codex models manifest config requires at least one account")
	}
	if len(cfg.AccountIDs) > maxCodexModelsManifestAccounts {
		return infraerrors.Newf(http.StatusBadRequest, "INVALID_CODEX_MODELS_MANIFEST_CONFIG", "codex models manifest config allows at most %d accounts, got %d", maxCodexModelsManifestAccounts, len(cfg.AccountIDs))
	}
	accounts, err := s.accountRepo.ListByGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("load group accounts for codex models manifest config: %w", err)
	}
	members := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc.IsActive() && acc.Platform == PlatformOpenAI {
			members[acc.ID] = struct{}{}
		}
	}
	var invalid []int64
	for _, id := range cfg.AccountIDs {
		if _, ok := members[id]; !ok {
			invalid = append(invalid, id)
		}
	}
	if len(invalid) > 0 {
		return infraerrors.Newf(http.StatusBadRequest, "INVALID_CODEX_MODELS_MANIFEST_CONFIG", "codex models manifest config contains accounts not in this group or not active openai accounts: %v", invalid)
	}
	return nil
}
