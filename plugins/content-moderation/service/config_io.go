package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	apperrors "github.com/Wei-Shaw/sub2api/plugins/content-moderation/internal/errors"
)

// loadConfig reads the moderation config blob from the plugin's settings
// namespace, falling back to defaults when unset. Replaces the core's
// SettingRepository-backed loader.
func (s *ModerationService) loadConfig(ctx context.Context) (*ContentModerationConfig, error) {
	cfg := defaultContentModerationConfig()
	raw, err := s.settings.Get(ctx, SettingKeyModerationConfig)
	if err != nil {
		if errors.Is(err, pluginsdk.ErrSettingNotFound) {
			cfg.normalize()
			return cfg, nil
		}
		return nil, fmt.Errorf("get content moderation config: %w", err)
	}
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		cfg.normalize()
		return cfg, nil
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, apperrors.BadRequest("INVALID_CONTENT_MODERATION_CONFIG", "内容审计配置不是有效 JSON")
	}
	cfg.normalize()
	return cfg, nil
}

func (s *ModerationService) saveConfig(ctx context.Context, cfg *ContentModerationConfig) error {
	if err := s.settings.Set(ctx, SettingKeyModerationConfig, cfg); err != nil {
		return fmt.Errorf("save content moderation config: %w", err)
	}
	return nil
}

func (s *ModerationService) UpdateConfig(ctx context.Context, input UpdateContentModerationConfigInput) (*ContentModerationConfigView, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	applyContentModerationConfigUpdate(cfg, input)
	applyContentModerationAPIKeyUpdate(cfg, input)
	if err := s.validateConfig(ctx, cfg); err != nil {
		return nil, err
	}
	cfg.normalize()
	if err := s.saveConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return s.configView(cfg), nil
}

func applyContentModerationConfigUpdate(cfg *ContentModerationConfig, input UpdateContentModerationConfigInput) {
	if input.Enabled != nil {
		cfg.Enabled = *input.Enabled
	}
	if input.Mode != nil {
		cfg.Mode = strings.TrimSpace(*input.Mode)
	}
	if input.BaseURL != nil {
		cfg.BaseURL = strings.TrimSpace(*input.BaseURL)
	}
	if input.Model != nil {
		cfg.Model = strings.TrimSpace(*input.Model)
	}
	if input.TimeoutMS != nil {
		cfg.TimeoutMS = *input.TimeoutMS
	}
	if input.SampleRate != nil {
		cfg.SampleRate = *input.SampleRate
	}
	if input.WorkerCount != nil {
		cfg.WorkerCount = *input.WorkerCount
	}
	if input.QueueSize != nil {
		cfg.QueueSize = *input.QueueSize
	}
	if input.BlockStatus != nil {
		cfg.BlockStatus = *input.BlockStatus
	}
	if input.BlockMessage != nil {
		cfg.BlockMessage = strings.TrimSpace(*input.BlockMessage)
	}
	if input.EmailOnHit != nil {
		cfg.EmailOnHit = *input.EmailOnHit
	}
	if input.AutoBanEnabled != nil {
		cfg.AutoBanEnabled = *input.AutoBanEnabled
	}
	if input.BanThreshold != nil {
		cfg.BanThreshold = *input.BanThreshold
	}
	if input.ViolationWindowHours != nil {
		cfg.ViolationWindowHours = *input.ViolationWindowHours
	}
	if input.RetryCount != nil {
		cfg.RetryCount = *input.RetryCount
	}
	if input.HitRetentionDays != nil {
		cfg.HitRetentionDays = *input.HitRetentionDays
	}
	if input.NonHitRetentionDays != nil {
		cfg.NonHitRetentionDays = *input.NonHitRetentionDays
	}
	if input.PreHashCheckEnabled != nil {
		cfg.PreHashCheckEnabled = *input.PreHashCheckEnabled
	}
	if input.BlockedKeywords != nil {
		cfg.BlockedKeywords = normalizeBlockedKeywords(*input.BlockedKeywords)
	}
	if input.KeywordBlockingMode != nil {
		cfg.KeywordBlockingMode = strings.TrimSpace(*input.KeywordBlockingMode)
	}
	if input.ModelFilter != nil {
		cfg.ModelFilter = *input.ModelFilter
	}
	if input.AllGroups != nil {
		cfg.AllGroups = *input.AllGroups
	}
	if input.GroupIDs != nil {
		cfg.GroupIDs = normalizeInt64IDs(*input.GroupIDs)
	}
	if input.RecordNonHits != nil {
		cfg.RecordNonHits = *input.RecordNonHits
	}
	if input.Thresholds != nil {
		cfg.Thresholds = mergeContentModerationThresholds(ContentModerationDefaultThresholds(), *input.Thresholds)
	}
}

func applyContentModerationAPIKeyUpdate(cfg *ContentModerationConfig, input UpdateContentModerationConfigInput) {
	if input.ClearAPIKey {
		cfg.APIKey = ""
		cfg.APIKeys = []string{}
		return
	}
	apiKeysMode := normalizeContentModerationAPIKeysMode(input.APIKeysMode)
	if input.DeleteAPIKeyHashes != nil && apiKeysMode != contentModerationAPIKeysModeReplace {
		cfg.APIKeys = deleteModerationAPIKeysByHash(cfg.apiKeys(), *input.DeleteAPIKeyHashes)
		cfg.APIKey = ""
	}
	if input.APIKeys != nil {
		if apiKeysMode == contentModerationAPIKeysModeReplace {
			cfg.APIKeys = normalizeModerationAPIKeys(*input.APIKeys)
		} else {
			cfg.APIKeys = normalizeModerationAPIKeys(append(cfg.apiKeys(), *input.APIKeys...))
		}
		cfg.APIKey = ""
	}
	if input.APIKey != nil && strings.TrimSpace(*input.APIKey) != "" {
		cfg.APIKeys = normalizeModerationAPIKeys(append(cfg.APIKeys, *input.APIKey))
		cfg.APIKey = ""
	}
}

func (s *ModerationService) validateConfig(ctx context.Context, cfg *ContentModerationConfig) error {
	if cfg == nil {
		return apperrors.BadRequest("INVALID_CONTENT_MODERATION_CONFIG", "内容审计配置不能为空")
	}
	cfg.normalize()
	switch cfg.Mode {
	case ContentModerationModeOff, ContentModerationModeObserve, ContentModerationModePreBlock:
	default:
		return apperrors.BadRequest("INVALID_CONTENT_MODERATION_MODE", "内容审计模式无效")
	}
	if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		return apperrors.BadRequest("INVALID_CONTENT_MODERATION_BASE_URL", "OpenAI Base URL 无效")
	}
	if cfg.BlockStatus < 400 || cfg.BlockStatus > 599 {
		return apperrors.BadRequest("INVALID_CONTENT_MODERATION_BLOCK_STATUS", "拦截 HTTP 状态码必须在 400-599 之间")
	}
	if cfg.ModelFilter.Type != ContentModerationModelFilterAll && len(cfg.ModelFilter.Models) == 0 {
		return apperrors.BadRequest("INVALID_CONTENT_MODERATION_MODEL_FILTER", "指定或排除模型时至少需要配置 1 个模型")
	}
	if !cfg.AllGroups && len(cfg.GroupIDs) > 0 && s.repo != nil {
		for _, groupID := range cfg.GroupIDs {
			exists, err := s.repo.GroupExists(ctx, groupID)
			if err != nil {
				return fmt.Errorf("validate content moderation group: %w", err)
			}
			if !exists {
				return apperrors.BadRequest("INVALID_CONTENT_MODERATION_GROUP", fmt.Sprintf("审计分组不存在: %d", groupID))
			}
		}
	}
	return nil
}

func (s *ModerationService) configView(cfg *ContentModerationConfig) *ContentModerationConfigView {
	keys := cfg.apiKeys()
	masks := make([]string, 0, len(keys))
	for _, key := range keys {
		masks = append(masks, maskSecretTail(key))
	}
	apiKeyMasked := ""
	if len(masks) > 0 {
		apiKeyMasked = masks[0]
	}
	return &ContentModerationConfigView{
		Enabled:              cfg.Enabled,
		Mode:                 cfg.Mode,
		BaseURL:              cfg.BaseURL,
		Model:                cfg.Model,
		APIKeyConfigured:     len(keys) > 0,
		APIKeyMasked:         apiKeyMasked,
		APIKeyCount:          len(keys),
		APIKeyMasks:          masks,
		APIKeyStatuses:       s.apiKeyStatuses(keys),
		TimeoutMS:            cfg.TimeoutMS,
		SampleRate:           cfg.SampleRate,
		AllGroups:            cfg.AllGroups,
		GroupIDs:             append([]int64(nil), cfg.GroupIDs...),
		RecordNonHits:        cfg.RecordNonHits,
		Thresholds:           cloneFloatMap(cfg.Thresholds),
		WorkerCount:          cfg.WorkerCount,
		QueueSize:            cfg.QueueSize,
		BlockStatus:          cfg.BlockStatus,
		BlockMessage:         cfg.BlockMessage,
		EmailOnHit:           cfg.EmailOnHit,
		AutoBanEnabled:       cfg.AutoBanEnabled,
		BanThreshold:         cfg.BanThreshold,
		ViolationWindowHours: cfg.ViolationWindowHours,
		RetryCount:           cfg.RetryCount,
		HitRetentionDays:     cfg.HitRetentionDays,
		NonHitRetentionDays:  cfg.NonHitRetentionDays,
		PreHashCheckEnabled:  cfg.PreHashCheckEnabled,
		BlockedKeywords:      append([]string(nil), cfg.BlockedKeywords...),
		KeywordBlockingMode:  cfg.KeywordBlockingMode,
		ModelFilter:          cloneContentModerationModelFilter(cfg.ModelFilter),
	}
}
