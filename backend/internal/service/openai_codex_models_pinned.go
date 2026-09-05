package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ErrNoPinnedCodexModelsAccounts signals that an enabled pinned-accounts
// manifest config has no usable account right now (inactive, unschedulable,
// or expired members are skipped, and remaining IDs are no longer bound).
var ErrNoPinnedCodexModelsAccounts = errors.New("no usable pinned codex models manifest accounts")

// isPinnedCodexModelsAccountUsable decides whether a pinned account may fetch
// the manifest. Deliberately narrower than Account.IsSchedulable(): priority,
// load factor, rate-limit windows, overload windows and temporary
// unschedulable cooldowns are ignored, because the pinned mode exists to keep
// the manifest deterministic regardless of scheduler state.
func isPinnedCodexModelsAccountUsable(account *Account) bool {
	if account == nil || !account.IsActive() || !account.Schedulable {
		return false
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !account.ExpiresAt.After(time.Now()) {
		return false
	}
	return true
}

// mergeCodexModelsManifestBodies merges manifest bodies from the pinned
// accounts: the first body's top-level envelope is the base, and the models
// arrays are unioned by slug with first-seen (config-order) entries winning.
// Entries without a usable slug are kept once in order of appearance.
func mergeCodexModelsManifestBodies(bodies [][]byte) ([]byte, error) {
	if len(bodies) == 0 {
		return nil, errors.New("no codex models manifest bodies to merge")
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(bodies[0], &base); err != nil {
		return nil, fmt.Errorf("decode base codex models manifest: %w", err)
	}
	if base == nil {
		return nil, errors.New("base codex models manifest is not a JSON object")
	}

	mergedModels := make([]json.RawMessage, 0, len(bodies)*8)
	seenSlug := make(map[string]struct{})
	seenSlugLess := make(map[string]struct{})
	for _, body := range bodies {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("decode codex models manifest body: %w", err)
		}
		modelsRaw, ok := envelope["models"]
		if !ok {
			continue
		}
		var models []json.RawMessage
		if err := json.Unmarshal(modelsRaw, &models); err != nil {
			return nil, fmt.Errorf("decode codex models manifest models array: %w", err)
		}
		for _, raw := range models {
			var model struct {
				Slug string `json:"slug"`
			}
			slug := ""
			if err := json.Unmarshal(raw, &model); err == nil {
				slug = strings.TrimSpace(model.Slug)
			}
			if slug != "" {
				if _, exists := seenSlug[slug]; exists {
					continue
				}
				seenSlug[slug] = struct{}{}
			} else {
				fingerprint := string(bytes.TrimSpace(raw))
				if _, exists := seenSlugLess[fingerprint]; exists {
					continue
				}
				seenSlugLess[fingerprint] = struct{}{}
			}
			mergedModels = append(mergedModels, raw)
		}
	}

	encodedModels, err := json.Marshal(mergedModels)
	if err != nil {
		return nil, fmt.Errorf("encode merged codex models: %w", err)
	}
	base["models"] = encodedModels
	merged, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("encode merged codex models manifest: %w", err)
	}
	return merged, nil
}

// FetchPinnedCodexModelsManifest fetches the Codex models manifest using only
// the accounts pinned on the group's codex_models_manifest_config, ignoring
// scheduler priorities, load factors and rate-limit/overload windows. All
// usable accounts are fetched concurrently; successful bodies are merged by
// slug with config-order precedence. It returns the merged manifest and the
// first successful account (in config order) for ops attribution.
//
//   - no usable account → ErrNoPinnedCodexModelsAccounts
//   - every fetch failed → the last upstream error
//   - partial failure → successful accounts are still merged and a warning
//     naming the failed account IDs is logged.
func (s *OpenAIGatewayService) FetchPinnedCodexModelsManifest(ctx context.Context, group *Group, clientVersion string) (*CodexModelsManifest, *Account, error) {
	if s == nil || s.accountRepo == nil || group == nil {
		return nil, nil, ErrNoPinnedCodexModelsAccounts
	}
	cfg := group.CodexModelsManifestConfig
	if group.Platform != PlatformOpenAI || !cfg.Enabled || len(cfg.AccountIDs) == 0 {
		return nil, nil, ErrNoPinnedCodexModelsAccounts
	}

	members, err := s.accountRepo.ListByGroup(ctx, group.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("load pinned codex models manifest accounts: %w", err)
	}
	memberByID := make(map[int64]Account, len(members))
	for _, member := range members {
		memberByID[member.ID] = member
	}

	// 按配置顺序筛选可用账号；已解绑/已删除的 ID 直接跳过。
	usable := make([]Account, 0, len(cfg.AccountIDs))
	for _, id := range cfg.AccountIDs {
		member, ok := memberByID[id]
		if !ok || member.Platform != PlatformOpenAI {
			continue
		}
		if !isPinnedCodexModelsAccountUsable(&member) {
			continue
		}
		usable = append(usable, member)
	}
	if len(usable) == 0 {
		return nil, nil, ErrNoPinnedCodexModelsAccounts
	}

	bodies := make([][]byte, len(usable))
	fetchErrs := make([]error, len(usable))
	// 各自独立完成、不取消兄弟请求，错误按下标收集，普通 WaitGroup 足够。
	var fetchGroup sync.WaitGroup
	for i := range usable {
		account := usable[i]
		index := i
		fetchGroup.Add(1)
		go func() {
			defer fetchGroup.Done()
			manifest, fetchErr := s.FetchCodexModelsManifest(ctx, &account, clientVersion, "")
			if fetchErr != nil {
				fetchErrs[index] = fetchErr
				return
			}
			if completeErr := s.CompleteAPIKeyCodexModelsManifestForClient(manifest, &account); completeErr != nil {
				fetchErrs[index] = completeErr
				return
			}
			bodies[index] = manifest.Body
		}()
	}
	fetchGroup.Wait()

	successBodies := make([][]byte, 0, len(usable))
	successAccounts := make([]*Account, 0, len(usable))
	failedIDs := make([]int64, 0)
	for i := range usable {
		if fetchErrs[i] != nil || bodies[i] == nil {
			failedIDs = append(failedIDs, usable[i].ID)
			continue
		}
		successBodies = append(successBodies, bodies[i])
		successAccounts = append(successAccounts, &usable[i])
	}
	if len(successBodies) == 0 {
		var lastErr error
		for i := range fetchErrs {
			if fetchErrs[i] != nil {
				lastErr = fetchErrs[i]
			}
		}
		if lastErr == nil {
			lastErr = infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "pinned codex models manifest accounts all failed")
		}
		return nil, nil, lastErr
	}
	if len(failedIDs) > 0 {
		slog.Warn("codex_models_manifest_pinned_partial_failure",
			"group_id", group.ID,
			"failed_account_ids", failedIDs,
		)
	}

	merged, err := mergeCodexModelsManifestBodies(successBodies)
	if err != nil {
		return nil, nil, fmt.Errorf("merge pinned codex models manifests: %w", err)
	}
	firstAccount := *successAccounts[0]
	return &CodexModelsManifest{
		Body: merged,
		ETag: codexModelsManifestBodyETag(merged),
	}, &firstAccount, nil
}
