package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const openAIAPIKeyHealthBreakerSettingsCacheTTL = 30 * time.Second

type cachedOpenAIAPIKeyHealthBreakerSettings struct {
	settings  OpenAIAPIKeyHealthBreakerSettings
	expiresAt time.Time
}

func normalizeOpenAIAPIKeyHealthBreakerSettings(settings *OpenAIAPIKeyHealthBreakerSettings) *OpenAIAPIKeyHealthBreakerSettings {
	if settings == nil {
		return DefaultOpenAIAPIKeyHealthBreakerSettings()
	}
	result := *settings
	if result.WindowMinutes < 1 {
		result.WindowMinutes = 1
	} else if result.WindowMinutes > 60 {
		result.WindowMinutes = 60
	}
	if result.FailureThreshold < 1 {
		result.FailureThreshold = 1
	} else if result.FailureThreshold > 10000 {
		result.FailureThreshold = 10000
	}
	if result.CooldownMinutes < 1 {
		result.CooldownMinutes = 1
	} else if result.CooldownMinutes > 60 {
		result.CooldownMinutes = 60
	}
	return &result
}

func (s *SettingService) GetOpenAIAPIKeyHealthBreakerSettings(ctx context.Context) (*OpenAIAPIKeyHealthBreakerSettings, error) {
	if s == nil || s.settingRepo == nil {
		return DefaultOpenAIAPIKeyHealthBreakerSettings(), nil
	}
	if cached, ok := s.openAIAPIKeyHealthBreakerCache.Load().(*cachedOpenAIAPIKeyHealthBreakerSettings); ok && cached != nil && time.Now().Before(cached.expiresAt) {
		result := cached.settings
		return &result, nil
	}

	settings := DefaultOpenAIAPIKeyHealthBreakerSettings()
	value, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAIAPIKeyHealthBreakerSettings)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, fmt.Errorf("get OpenAI API key health breaker settings: %w", err)
	}
	if err == nil && strings.TrimSpace(value) != "" {
		var stored OpenAIAPIKeyHealthBreakerSettings
		if json.Unmarshal([]byte(value), &stored) == nil {
			settings = normalizeOpenAIAPIKeyHealthBreakerSettings(&stored)
		}
	}
	s.openAIAPIKeyHealthBreakerCache.Store(&cachedOpenAIAPIKeyHealthBreakerSettings{
		settings:  *settings,
		expiresAt: time.Now().Add(openAIAPIKeyHealthBreakerSettingsCacheTTL),
	})
	result := *settings
	return &result, nil
}

func (s *SettingService) SetOpenAIAPIKeyHealthBreakerSettings(ctx context.Context, settings *OpenAIAPIKeyHealthBreakerSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	normalized := normalizeOpenAIAPIKeyHealthBreakerSettings(settings)
	if settings.Enabled && *normalized != *settings {
		return fmt.Errorf("window_minutes and cooldown_minutes must be between 1-60; failure_threshold must be between 1-10000")
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpenAIAPIKeyHealthBreakerSettings, string(data)); err != nil {
		return fmt.Errorf("set OpenAI API key health breaker settings: %w", err)
	}
	s.openAIAPIKeyHealthBreakerCache.Store(&cachedOpenAIAPIKeyHealthBreakerSettings{})
	return nil
}
