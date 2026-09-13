package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/keyprotection"
)

const SettingKeyKeyProtection = "automatic_key_protection"

// GetKeyProtectionConfig takes a fresh policy snapshot for the request. Unlike
// optional display settings, unavailable or corrupt protection settings must
// never silently fall back to a disabled policy.
func (s *SettingService) GetKeyProtectionConfig(ctx context.Context) (keyprotection.Config, error) {
	if s == nil || s.settingRepo == nil {
		return keyprotection.Config{}, errors.New("key protection settings unavailable")
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyKeyProtection)
	if errors.Is(err, ErrSettingNotFound) {
		return keyprotection.DefaultConfig(), nil
	}
	if err != nil {
		return keyprotection.Config{}, errors.New("read key protection settings failed")
	}
	// Start with defaults so additive settings remain compatible with older
	// policy documents, but reject null/empty documents rather than disabling.
	cfg := keyprotection.DefaultConfig()
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &object); err != nil || object == nil {
		return keyprotection.Config{}, errors.New("invalid key protection settings document")
	}
	if err := json.Unmarshal([]byte(value), &cfg); err != nil {
		return keyprotection.Config{}, errors.New("invalid key protection settings document")
	}
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return keyprotection.Config{}, errors.New("invalid key protection settings policy")
	}
	return cfg, nil
}

func (s *SettingService) SetKeyProtectionConfig(ctx context.Context, cfg keyprotection.Config) error {
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid key protection settings: %w", err)
	}
	if s == nil || s.settingRepo == nil {
		return errors.New("key protection settings unavailable")
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return errors.New("encode key protection settings failed")
	}
	if err := s.settingRepo.Set(ctx, SettingKeyKeyProtection, string(data)); err != nil {
		return errors.New("save key protection settings failed")
	}
	return nil
}
