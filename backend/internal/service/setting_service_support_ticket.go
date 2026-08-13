package service

import (
	"context"
	"log/slog"
	"strings"
)

// SupportTicketRuntime is the runtime projection of the support-ticket feature
// config. Support is a fork-only feature (feat/support); captcha runtime lives
// in setting_captcha.go.
//
// TicketNotifyEmails 是"已解禁"的白名单邮箱清单（去除 disabled=true 项，字符串
// 已 trim + 小写），供 SupportTicketNotificationService 直接投递管理员方向邮件。
// 空切片表示未配置白名单，通知服务会退化为向所有 role=admin 用户发送。
type SupportTicketRuntime struct {
	Enabled            bool
	Categories         []string
	DefaultPriority    string
	TicketNotifyEmails []string
}

// GetSupportTicketRuntime 直接从 settings store 读取工单系统的四项配置：
//   - support_ticket_enabled (bool, default false)
//   - support_ticket_categories (JSON string[], default SupportTicketDefaultCategories)
//   - support_ticket_default_priority (low|normal|high, default normal)
//   - support_ticket_notify_emails (JSON []NotifyEmailEntry, disabled=true 的自动过滤)
//
// 失败时 fail-closed：返回 Enabled=false + 默认 categories + normal 优先级 + 空白名单。
func (s *SettingService) GetSupportTicketRuntime(ctx context.Context) SupportTicketRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeySupportTicketEnabled,
		SettingKeySupportTicketCategories,
		SettingKeySupportTicketDefaultPriority,
		SettingKeySupportTicketNotifyEmails,
	})
	if err != nil {
		slog.Warn("failed to get support ticket runtime settings, defaulting to disabled", "error", err)
		return SupportTicketRuntime{
			Enabled:            false,
			Categories:         cloneSupportTicketDefaultCategories(),
			DefaultPriority:    SupportTicketPriorityNormal,
			TicketNotifyEmails: nil,
		}
	}
	return SupportTicketRuntime{
		Enabled:            vals[SettingKeySupportTicketEnabled] == "true",
		Categories:         ParseSupportTicketCategories(vals[SettingKeySupportTicketCategories]),
		DefaultPriority:    NormalizeSupportTicketPriority(vals[SettingKeySupportTicketDefaultPriority]),
		TicketNotifyEmails: enabledNotifyEmails(ParseNotifyEmails(vals[SettingKeySupportTicketNotifyEmails])),
	}
}

// enabledNotifyEmails 把 []NotifyEmailEntry 投影成"已启用（disabled=false）的
// trim + 小写 email 字符串切片"，去重后返回。空输入 → nil。
//
// runtime 层只关心"要不要发到这个邮箱"，因此不透传 Disabled/Verified 字段；
// admin GET 路径（SystemSettings）仍保留原对象以便 UI 完整回显。
func enabledNotifyEmails(entries []NotifyEmailEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(entries))
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Disabled {
			continue
		}
		v := strings.ToLower(strings.TrimSpace(e.Email))
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
