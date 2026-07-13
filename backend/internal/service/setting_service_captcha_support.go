package service

import (
	"context"
	"log/slog"
)

// SupportTicketRuntime is the runtime projection of the support-ticket feature
// config. Support is a fork-only feature (feat/support); captcha runtime lives
// in setting_captcha.go.
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
