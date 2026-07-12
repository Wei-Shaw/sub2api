package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

type CaptchaRuntime struct {
	Provider  string
	Enabled   bool
	SiteKey   string
	SecretKey string
	Config    map[string]string
}

var captchaSecretFields = map[string][]string{
	CaptchaProviderTurnstile: {"secret_key"},
	CaptchaProviderHcaptcha:  {"secret_key"},
	CaptchaProviderTencent:   {"app_secret_key", "secret_id", "secret_key"},
}

func normalizeCaptchaProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case CaptchaProviderHcaptcha:
		return CaptchaProviderHcaptcha
	case CaptchaProviderTencent:
		return CaptchaProviderTencent
	default:
		return CaptchaProviderTurnstile
	}
}

func NormalizeCaptchaProviderForSettings(provider string) string {
	return normalizeCaptchaProvider(provider)
}

func MaskCaptchaConfigForSettings(provider string, cfg map[string]string) map[string]string {
	return maskCaptchaConfig(provider, cfg)
}

func parseCaptchaConfig(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil || cfg == nil {
		return map[string]string{}
	}
	return cfg
}

func encodeCaptchaConfig(cfg map[string]string) string {
	if cfg == nil {
		cfg = map[string]string{}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func maskCaptchaConfig(provider string, cfg map[string]string) map[string]string {
	masked := cloneStringMap(cfg)
	if fields, ok := captchaSecretFields[provider]; ok {
		for _, field := range fields {
			delete(masked, field)
		}
		return masked
	}
	for _, fields := range captchaSecretFields {
		for _, field := range fields {
			delete(masked, field)
		}
	}
	return masked
}

func captchaConfigPrimarySecret(provider string, cfg map[string]string) string {
	if provider == CaptchaProviderTencent {
		return cfg["app_secret_key"]
	}
	return cfg["secret_key"]
}

func captchaConfigSiteKey(provider string, cfg map[string]string) string {
	if provider == CaptchaProviderTencent {
		return cfg["captcha_app_id"]
	}
	return cfg["site_key"]
}

func captchaRuntimeFromSettings(settings map[string]string) CaptchaRuntime {
	provider := normalizeCaptchaProvider(settings[SettingKeyCaptchaProvider])
	config := parseCaptchaConfig(settings[SettingKeyCaptchaConfig])
	if len(config) > 0 {
		return CaptchaRuntime{
			Provider:  provider,
			Enabled:   config["enabled"] == "true",
			SiteKey:   captchaConfigSiteKey(provider, config),
			SecretKey: captchaConfigPrimarySecret(provider, config),
			Config:    cloneStringMap(config),
		}
	}

	legacyConfig := map[string]string{
		"enabled":    strconv.FormatBool(settings[SettingKeyTurnstileEnabled] == "true"),
		"site_key":   settings[SettingKeyTurnstileSiteKey],
		"secret_key": settings[SettingKeyTurnstileSecretKey],
	}
	return CaptchaRuntime{
		Provider:  CaptchaProviderTurnstile,
		Enabled:   settings[SettingKeyTurnstileEnabled] == "true",
		SiteKey:   settings[SettingKeyTurnstileSiteKey],
		SecretKey: settings[SettingKeyTurnstileSecretKey],
		Config:    legacyConfig,
	}
}

func (s *SettingService) GetCaptchaRuntime(ctx context.Context) CaptchaRuntime {
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyCaptchaProvider,
		SettingKeyTurnstileEnabled,
		SettingKeyTurnstileSiteKey,
		SettingKeyTurnstileSecretKey,
		SettingKeyCaptchaConfig,
	})
	if err != nil {
		return CaptchaRuntime{Provider: CaptchaProviderTurnstile}
	}
	return captchaRuntimeFromSettings(values)
}
