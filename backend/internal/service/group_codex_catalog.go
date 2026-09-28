package service

import (
	"context"
	"errors"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GetGroupCodexModelsManifest 供网关与管理预览共用同一目录获取流程。
func (s *OpenAIGatewayService) GetGroupCodexModelsManifest(ctx context.Context, group *Group, version, ifNoneMatch string, maxSwitches int) (*OpenAIModelsResponse, *Account, error) {
	if group.CodexModelsManifestConfig.Enabled {
		manifest, account, err := s.FetchPinnedCodexModelsManifest(ctx, group, version)
		if err == nil {
			err = s.MergeGroupConfiguredCodexModels(ctx, group, manifest, ifNoneMatch)
			return manifest, account, err
		}
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if !group.CodexModelsManifestConfig.FallbackToScheduler {
			if errors.Is(err, ErrNoPinnedCodexModelsAccounts) {
				err = infraerrors.New(http.StatusServiceUnavailable, "NO_CODEX_MODELS_ACCOUNTS", "No available pinned OpenAI accounts")
			}
			return nil, nil, err
		}
	} else {
		manifest, configured, err := s.BuildGroupConfiguredCodexModelsManifest(ctx, group, ifNoneMatch)
		if err != nil || configured {
			return manifest, nil, err
		}
	}
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	excluded := make(map[int64]struct{})
	var lastErr error
	for attempt := 0; attempt <= maxSwitches; attempt++ {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		account, err := s.SelectAccountForModelWithExclusions(ctx, &group.ID, "", "", excluded)
		if err != nil {
			if lastErr != nil {
				return nil, nil, lastErr
			}
			return nil, nil, infraerrors.New(http.StatusServiceUnavailable, "NO_CODEX_MODELS_ACCOUNTS", "No available OpenAI accounts")
		}
		manifest, err := s.FetchCodexModelsManifest(ctx, account, version, "")
		if err != nil {
			if IsRetryableCodexModelsManifestError(err) && attempt < maxSwitches {
				excluded[account.ID] = struct{}{}
				lastErr = err
				continue
			}
			return nil, account, err
		}
		if err := s.CompleteAPIKeyCodexModelsManifestForClient(manifest, account); err != nil {
			return nil, account, err
		}
		if err := ApplyPinnedCodexModelsMapping(manifest, account, group); err != nil {
			return nil, account, err
		}
		err = s.MergeGroupConfiguredCodexModels(ctx, group, manifest, ifNoneMatch)
		return manifest, account, err
	}
	return nil, nil, lastErr
}
