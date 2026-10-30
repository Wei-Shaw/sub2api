package service

import (
	"context"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ContentModerationEngineOpenAI   = "openai"
	ContentModerationEngineTypeSafe = "typesafe"
)

type ContentModerationEngineConfig struct {
	BaseURL    string             `json:"base_url"`
	Model      string             `json:"model"`
	ProxyID    *int64             `json:"proxy_id,omitempty"`
	APIKeys    []string           `json:"api_keys,omitempty"`
	TimeoutMS  int                `json:"timeout_ms"`
	RetryCount int                `json:"retry_count"`
	Thresholds map[string]float64 `json:"thresholds"`
}

type UpdateContentModerationEngineInput struct {
	BaseURL            *string             `json:"base_url"`
	Model              *string             `json:"model"`
	ProxyID            *int64              `json:"proxy_id"`
	APIKey             *string             `json:"api_key"`
	APIKeys            *[]string           `json:"api_keys"`
	APIKeysMode        string              `json:"api_keys_mode"`
	DeleteAPIKeyHashes *[]string           `json:"delete_api_key_hashes"`
	ClearAPIKey        bool                `json:"clear_api_key"`
	TimeoutMS          *int                `json:"timeout_ms"`
	RetryCount         *int                `json:"retry_count"`
	Thresholds         *map[string]float64 `json:"thresholds"`
}

type ContentModerationEngineMeta struct {
	Engine        string `json:"engine"`
	Model         string `json:"model"`
	RulesVersion  string `json:"rules_version"`
	SkippedImages int    `json:"skipped_images"`
}

func moderationEngine(engine string) string {
	if engine == "" {
		return ContentModerationEngineOpenAI
	}
	return engine
}

func validModerationEngine(engine string) bool {
	return engine == ContentModerationEngineOpenAI || engine == ContentModerationEngineTypeSafe
}

func moderationEngineDefaults(engine string) *ContentModerationEngineConfig {
	p := &ContentModerationEngineConfig{BaseURL: defaultContentModerationBaseURL, Model: defaultContentModerationModel, TimeoutMS: 3000, RetryCount: 2, Thresholds: ContentModerationDefaultThresholds()}
	if engine == ContentModerationEngineTypeSafe {
		p.BaseURL, p.Model = "https://api.typesafe.ai", "jev-latest"
	}
	return p
}

func (cfg *ContentModerationConfig) engineProfile(engine string) *ContentModerationEngineConfig {
	var p ContentModerationEngineConfig
	if engine == ContentModerationEngineTypeSafe {
		if cfg.TypeSafe == nil {
			return moderationEngineDefaults(engine)
		}
		p = *cfg.TypeSafe
	} else {
		p = ContentModerationEngineConfig{BaseURL: cfg.BaseURL, Model: cfg.Model, ProxyID: cfg.ProxyID, APIKeys: cfg.apiKeys(), TimeoutMS: cfg.TimeoutMS, RetryCount: cfg.RetryCount, Thresholds: cfg.Thresholds}
	}
	p.ProxyID = cloneInt64Ptr(p.ProxyID)
	p.APIKeys = append([]string(nil), p.APIKeys...)
	p.Thresholds = cloneFloatMap(p.Thresholds)
	return &p
}

func (cfg *ContentModerationConfig) applyEngineProfile(p *ContentModerationEngineConfig) {
	cfg.BaseURL, cfg.Model, cfg.ProxyID = p.BaseURL, p.Model, cloneInt64Ptr(p.ProxyID)
	cfg.APIKey, cfg.APIKeys = "", append([]string(nil), p.APIKeys...)
	cfg.TimeoutMS, cfg.RetryCount = p.TimeoutMS, p.RetryCount
	cfg.Thresholds = cloneFloatMap(p.Thresholds)
}

func (cfg *ContentModerationConfig) effectiveEngine(engine string) *ContentModerationConfig {
	out := cloneContentModerationConfig(cfg)
	out.Engine = moderationEngine(engine)
	out.applyEngineProfile(cfg.engineProfile(out.Engine))
	return out
}

func (s *ContentModerationService) updateEngineProfile(ctx context.Context, cfg *ContentModerationConfig, engine string, input UpdateContentModerationEngineInput) error {
	if !validModerationEngine(engine) {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_ENGINE", "内容审计引擎无效")
	}
	p := cfg.engineProfile(engine)
	defaults := moderationEngineDefaults(engine)
	if input.BaseURL != nil {
		p.BaseURL = strings.TrimSpace(*input.BaseURL)
		if p.BaseURL == "" {
			p.BaseURL = defaults.BaseURL
		}
	}
	if input.Model != nil {
		p.Model = strings.TrimSpace(*input.Model)
		if p.Model == "" {
			p.Model = defaults.Model
		}
	}
	if input.ProxyID != nil {
		p.ProxyID = cloneInt64Ptr(input.ProxyID)
		if *input.ProxyID <= 0 {
			p.ProxyID = nil
		}
	}
	if input.TimeoutMS != nil {
		p.TimeoutMS = *input.TimeoutMS
	}
	if input.RetryCount != nil {
		p.RetryCount = *input.RetryCount
	}
	if input.Thresholds != nil {
		p.Thresholds = mergeContentModerationThresholds(defaults.Thresholds, *input.Thresholds)
	}
	if input.ClearAPIKey {
		p.APIKeys = []string{}
	} else {
		mode := normalizeContentModerationAPIKeysMode(input.APIKeysMode)
		if input.DeleteAPIKeyHashes != nil && mode != contentModerationAPIKeysModeReplace {
			p.APIKeys = deleteModerationAPIKeysByHash(p.APIKeys, *input.DeleteAPIKeyHashes, engine)
		}
		if input.APIKeys != nil {
			if mode == contentModerationAPIKeysModeReplace {
				p.APIKeys = normalizeModerationAPIKeys(*input.APIKeys)
			} else {
				p.APIKeys = normalizeModerationAPIKeys(append(p.APIKeys, *input.APIKeys...))
			}
		}
		if input.APIKey != nil {
			p.APIKeys = normalizeModerationAPIKeys(append(p.APIKeys, *input.APIKey))
		}
	}
	check := cloneContentModerationConfig(cfg)
	check.Engine = engine
	check.applyEngineProfile(p)
	if err := s.validateConfig(ctx, check); err != nil {
		return err
	}
	p = &ContentModerationEngineConfig{BaseURL: check.BaseURL, Model: check.Model, ProxyID: check.ProxyID, APIKeys: check.APIKeys, TimeoutMS: check.TimeoutMS, RetryCount: check.RetryCount, Thresholds: check.Thresholds}
	if engine == ContentModerationEngineTypeSafe {
		cfg.TypeSafe = p
	} else {
		cfg.applyEngineProfile(p)
	}
	return nil
}

func (s *ContentModerationService) engineConfigView(cfg *ContentModerationConfig) *ContentModerationConfigView {
	view := s.configView(cfg.effectiveEngine(cfg.Engine))
	view.EngineConfigs = map[string]*ContentModerationConfigView{}
	for _, engine := range []string{ContentModerationEngineOpenAI, ContentModerationEngineTypeSafe} {
		view.EngineConfigs[engine] = s.configView(cfg.effectiveEngine(engine))
	}
	return view
}

func scopedModerationKeyHash(key string, engine ...string) string {
	if len(engine) > 0 && engine[0] == ContentModerationEngineTypeSafe {
		return moderationAPIKeyHash("typesafe\x00" + key)
	}
	return moderationAPIKeyHash(key)
}
