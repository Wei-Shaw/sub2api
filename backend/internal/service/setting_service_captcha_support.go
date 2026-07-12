package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
)

// ── Fork-only settings runtime helpers ──────────────────────────────────────
// 以下代码在上游将 setting_service.go 拆分重构（677 commits）时保留于此，
// 因为上游没有这些 fork 专属特性：Tencent/hCaptcha 验证码 provider 与工单系统。

type CaptchaRuntime struct {
	Provider  string
	Enabled   bool
	SiteKey   string
	SecretKey string
	Config    map[string]string
}

// captchaSecretFields 列出每个 provider 的所有敏感字段名（在 captcha_config JSON map 中）。
// maskCaptchaConfig 用此表把这些字段从对外响应中移除，并在 SystemSettings DTO 中改用 *_configured: bool 暴露。
//
// 注：在统一 captcha_config 抽象之前，Turnstile/hCaptcha 都把私钥放在 "secret_key" 这一个键中；
// 腾讯天御因为腾讯云 API 强制依赖 4 个独立字段（CaptchaAppId 用作 site_key 显式公开；
// AppSecretKey 是天御本身验票的密钥；SecretId/SecretKey 是腾讯云 IAM 接入凭证），因此天御
// 同时有 3 个敏感字段需要 mask。
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

// MaskCaptchaConfigForSettings 是 maskCaptchaConfig 的对外导出包装，供 handler 包在
// GET / PUT admin settings 时复用同一份脱敏规则（D7）。provider 必须是 normalize 后的值。
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
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// maskCaptchaConfig 按 provider 移除 captcha_config 中的所有敏感字段（D7）。
// provider 取 normalizeCaptchaProvider 之后的稳定值；未知 provider 走保守降级：
// 把所有已知 provider 的敏感字段都剥掉，避免遗留。
func maskCaptchaConfig(provider string, cfg map[string]string) map[string]string {
	masked := cloneStringMap(cfg)
	if fields, ok := captchaSecretFields[provider]; ok {
		for _, f := range fields {
			delete(masked, f)
		}
		return masked
	}
	// 降级：未知 provider，剥掉所有 provider 已知的敏感字段。
	for _, fields := range captchaSecretFields {
		for _, f := range fields {
			delete(masked, f)
		}
	}
	return masked
}

// captchaConfigPrimarySecret 抽出指定 provider 的"主"密钥（用于 SystemSettings.CaptchaSecretKey 内部传递与
// admin GET/PUT 阶段的 unchanged-secret 留存逻辑）。
//   - Turnstile / hCaptcha: secret_key
//   - Tencent: app_secret_key（业务验票密钥）
func captchaConfigPrimarySecret(provider string, cfg map[string]string) string {
	switch provider {
	case CaptchaProviderTencent:
		return cfg["app_secret_key"]
	default:
		return cfg["secret_key"]
	}
}

// captchaConfigSiteKey 抽出指定 provider 用于前端公开页面的 site key
// （weak public credential：Turnstile/hCaptcha 是 site_key；Tencent 是 captcha_app_id）。
func captchaConfigSiteKey(provider string, cfg map[string]string) string {
	switch provider {
	case CaptchaProviderTencent:
		return cfg["captcha_app_id"]
	default:
		return cfg["site_key"]
	}
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

	// Legacy Turnstile settings remain a fallback for existing deployments.
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


// SupportTicketRuntime 是工单系统的运行时配置投影。
//
// 只读快照：调用方拿到的 Categories 切片应被视为不可变（共享自 ParseSupportTicketCategories
// 的回退默认值时切片底层可能与 SupportTicketDefaultCategories 不同——已 clone 过——
// 但语义上仍按只读对待）。
type SupportTicketRuntime struct {
	Enabled         bool
	Categories      []string
	DefaultPriority string
}

// GetSupportTicketRuntime 直接从 settings store 读取工单系统的三项配置：
//   - support_ticket_enabled (bool, default false)
//   - support_ticket_categories (JSON string[], default SupportTicketDefaultCategories)
//   - support_ticket_default_priority (low|normal|high, default normal)
//
// 失败时 fail-closed：返回 Enabled=false + 默认 categories + normal 优先级。
// 与 GetCaptchaRuntime / IsUserErrorViewAllowed 等相同的轻量 runtime 风格。
func (s *SettingService) GetSupportTicketRuntime(ctx context.Context) SupportTicketRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeySupportTicketEnabled,
		SettingKeySupportTicketCategories,
		SettingKeySupportTicketDefaultPriority,
	})
	if err != nil {
		slog.Warn("failed to get support ticket runtime settings, defaulting to disabled", "error", err)
		return SupportTicketRuntime{
			Enabled:         false,
			Categories:      cloneSupportTicketDefaultCategories(),
			DefaultPriority: SupportTicketPriorityNormal,
		}
	}
	return SupportTicketRuntime{
		Enabled:         vals[SettingKeySupportTicketEnabled] == "true",
		Categories:      ParseSupportTicketCategories(vals[SettingKeySupportTicketCategories]),
		DefaultPriority: NormalizeSupportTicketPriority(vals[SettingKeySupportTicketDefaultPriority]),
	}
}
