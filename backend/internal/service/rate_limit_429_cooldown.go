package service

import (
	"context"
	"encoding/json"
	"fmt"
)

type RateLimit429CooldownSettings struct {
	Enabled         bool `json:"enabled"`
	CooldownSeconds int  `json:"cooldown_seconds"`
}

func defaultRateLimit429CooldownSettings() *RateLimit429CooldownSettings {
	return &RateLimit429CooldownSettings{
		Enabled:         true,
		CooldownSeconds: defaultRateLimit429CooldownSeconds,
	}
}

func (s *SettingService) GetRateLimit429CooldownSettings(ctx context.Context) (*RateLimit429CooldownSettings, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyRateLimit429CooldownSettings)
	if err != nil {
		if err == ErrSettingNotFound {
			return defaultRateLimit429CooldownSettings(), nil
		}
		return nil, fmt.Errorf("get rate limit 429 cooldown settings: %w", err)
	}
	if raw == "" {
		return defaultRateLimit429CooldownSettings(), nil
	}

	settings := defaultRateLimit429CooldownSettings()
	if err := json.Unmarshal([]byte(raw), settings); err != nil {
		return nil, fmt.Errorf("unmarshal rate limit 429 cooldown settings: %w", err)
	}
	if settings.Enabled {
		settings.CooldownSeconds = clampRateLimit429CooldownSeconds(settings.CooldownSeconds)
	}
	return settings, nil
}

func (s *SettingService) SetRateLimit429CooldownSettings(ctx context.Context, settings *RateLimit429CooldownSettings) error {
	if settings == nil {
		return fmt.Errorf("rate limit 429 cooldown settings is required")
	}
	if settings.Enabled {
		if settings.CooldownSeconds < 1 || settings.CooldownSeconds > maxRateLimit429CooldownSeconds {
			return fmt.Errorf("cooldown_seconds must be between 1-7200")
		}
	} else if settings.CooldownSeconds <= 0 {
		settings.CooldownSeconds = defaultRateLimit429CooldownSeconds
	}

	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal rate limit 429 cooldown settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyRateLimit429CooldownSettings, string(raw)); err != nil {
		return fmt.Errorf("set rate limit 429 cooldown settings: %w", err)
	}
	return nil
}
