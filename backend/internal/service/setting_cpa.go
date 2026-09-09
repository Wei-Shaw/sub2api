package service

import (
	"context"
	"strings"
	"time"
)

const (
	// DefaultCpaManagementBaseURL points at the CPA instance as seen from the
	// Sub2API container. Docker Desktop / compose exposes the host as
	// host.docker.internal (compose already sets extra_hosts for it).
	DefaultCpaManagementBaseURL = "http://host.docker.internal:8317"

	cpaSettingsDBTimeout = 5 * time.Second
)

// CpaConfig is the internal view of the CPA read-only panel configuration.
// ManagementKey is the plaintext management key and must never leave the
// backend: API responses use CpaConfigView instead.
type CpaConfig struct {
	Enabled       bool
	BaseURL       string
	ManagementKey string
}

// CpaConfigView is the API-safe projection of CpaConfig.
type CpaConfigView struct {
	Enabled                 bool   `json:"enabled"`
	BaseURL                 string `json:"base_url"`
	ManagementKeyConfigured bool   `json:"management_key_configured"`
	ManagementKeyHint       string `json:"management_key_hint,omitempty"`
}

// View projects the config without exposing the management key.
func (c *CpaConfig) View() CpaConfigView {
	if c == nil {
		return CpaConfigView{BaseURL: DefaultCpaManagementBaseURL}
	}
	view := CpaConfigView{
		Enabled:                 c.Enabled,
		BaseURL:                 c.BaseURL,
		ManagementKeyConfigured: strings.TrimSpace(c.ManagementKey) != "",
	}
	if view.ManagementKeyConfigured {
		view.ManagementKeyHint = maskCpaManagementKey(c.ManagementKey)
	}
	return view
}

// Configured reports whether the panel has everything it needs to talk to CPA.
func (c *CpaConfig) Configured() bool {
	return c != nil && strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.ManagementKey) != ""
}

// maskCpaManagementKey keeps a short prefix/suffix so operators can tell which
// key is stored without learning it from the panel.
func maskCpaManagementKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= 8 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:2]) + strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-2:])
}

// GetCpaConfig reads the CPA panel configuration. Missing keys fall back to
// the defaults (disabled, Docker host URL, empty key).
func (s *SettingService) GetCpaConfig(ctx context.Context) *CpaConfig {
	cfg := &CpaConfig{
		Enabled: false,
		BaseURL: DefaultCpaManagementBaseURL,
	}
	if s == nil || s.settingRepo == nil {
		return cfg
	}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cpaSettingsDBTimeout)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(dbCtx, []string{
		SettingKeyCpaManagementEnabled,
		SettingKeyCpaManagementBaseURL,
		SettingKeyCpaManagementKey,
	})
	if err != nil {
		return cfg
	}
	if raw, ok := values[SettingKeyCpaManagementEnabled]; ok {
		cfg.Enabled = strings.EqualFold(strings.TrimSpace(raw), "true")
	}
	if raw, ok := values[SettingKeyCpaManagementBaseURL]; ok {
		if normalized := NormalizeCpaManagementBaseURL(raw); normalized != "" {
			cfg.BaseURL = normalized
		}
	}
	if raw, ok := values[SettingKeyCpaManagementKey]; ok {
		cfg.ManagementKey = strings.TrimSpace(raw)
	}
	return cfg
}

// UpdateCpaConfig persists the panel configuration. managementKey is optional:
// nil keeps the stored key, an empty string clears it.
func (s *SettingService) UpdateCpaConfig(ctx context.Context, enabled bool, baseURL string, managementKey *string) error {
	if s == nil || s.settingRepo == nil {
		return ErrServiceUnavailable
	}
	normalizedURL := NormalizeCpaManagementBaseURL(baseURL)
	if normalizedURL == "" {
		normalizedURL = DefaultCpaManagementBaseURL
	}
	values := map[string]string{
		SettingKeyCpaManagementEnabled: boolToString(enabled),
		SettingKeyCpaManagementBaseURL: normalizedURL,
	}
	if managementKey != nil {
		values[SettingKeyCpaManagementKey] = strings.TrimSpace(*managementKey)
	}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cpaSettingsDBTimeout)
	defer cancel()
	if err := s.settingRepo.SetMultiple(dbCtx, values); err != nil {
		return err
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return nil
}

// NormalizeCpaManagementBaseURL trims whitespace, drops trailing slashes and
// accepts host:port without a scheme by assuming http://.
func NormalizeCpaManagementBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	return trimmed
}

func boolToString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
