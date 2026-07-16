package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ParseSupportTicketCategories 把已持久化的 JSON 字符串解析回 []string。
// 解析失败 / 空值 / 解析后非法时统一回退到 SupportTicketDefaultCategories 的拷贝，
// 调用方拿到的永远是非空、不可变的初始列表（防止 GET /public 返回空数组导致下拉为空）。
func ParseSupportTicketCategories(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return cloneSupportTicketDefaultCategories()
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return cloneSupportTicketDefaultCategories()
	}
	normalized, err := normalizeSupportTicketCategories(items, false)
	if err != nil || len(normalized) == 0 {
		return cloneSupportTicketDefaultCategories()
	}
	return normalized
}

// NormalizeSupportTicketCategories 在写入前严格校验：
//   - 至少 1 项、最多 SupportTicketCategoryMaxCount 项
//   - 单项 trim 后非空，rune 长度 <= SupportTicketCategoryMaxLen
//   - 去重（case-sensitive，与展示层保持一致）
//
// 任意失败返回 BadRequest 风格错误，handler 层包成 400。
func NormalizeSupportTicketCategories(raw []string) ([]string, error) {
	return normalizeSupportTicketCategories(raw, true)
}

func normalizeSupportTicketCategories(raw []string, strict bool) ([]string, error) {
	if len(raw) == 0 {
		if strict {
			return nil, infraerrors.BadRequest("INVALID_SUPPORT_TICKET_CATEGORIES", "categories must not be empty")
		}
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			if strict {
				return nil, infraerrors.BadRequest("INVALID_SUPPORT_TICKET_CATEGORIES", "category must not be blank")
			}
			continue
		}
		if utf8.RuneCountInString(trimmed) > SupportTicketCategoryMaxLen {
			if strict {
				return nil, infraerrors.BadRequest(
					"INVALID_SUPPORT_TICKET_CATEGORIES",
					fmt.Sprintf("category %q exceeds %d characters", trimmed, SupportTicketCategoryMaxLen),
				)
			}
			continue
		}
		if _, dup := seen[trimmed]; dup {
			if strict {
				return nil, infraerrors.BadRequest(
					"INVALID_SUPPORT_TICKET_CATEGORIES",
					fmt.Sprintf("duplicate category %q", trimmed),
				)
			}
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if strict && len(out) == 0 {
		return nil, infraerrors.BadRequest("INVALID_SUPPORT_TICKET_CATEGORIES", "categories must not be empty")
	}
	if len(out) > SupportTicketCategoryMaxCount {
		if strict {
			return nil, infraerrors.BadRequest(
				"INVALID_SUPPORT_TICKET_CATEGORIES",
				fmt.Sprintf("categories must contain at most %d items", SupportTicketCategoryMaxCount),
			)
		}
		out = out[:SupportTicketCategoryMaxCount]
	}
	return out, nil
}

// MarshalSupportTicketCategories 把 []string 编码为可持久化的 JSON 字符串。
// 输入必须先经过 NormalizeSupportTicketCategories。
func MarshalSupportTicketCategories(items []string) (string, error) {
	if items == nil {
		items = []string{}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("marshal support ticket categories: %w", err)
	}
	return string(b), nil
}

// NormalizeSupportTicketPriority 把任意大小写/含空白的字符串规范成 low|normal|high。
// 空 / 未知输入回退到 SupportTicketPriorityNormal，调用方需要严格校验时请用 ValidateSupportTicketPriority。
func NormalizeSupportTicketPriority(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case SupportTicketPriorityLow:
		return SupportTicketPriorityLow
	case SupportTicketPriorityHigh:
		return SupportTicketPriorityHigh
	}
	return SupportTicketPriorityNormal
}

// ValidateSupportTicketPriority 严格校验：仅接受 low / normal / high（已 trim+lower）。
// 设计为 setter 层的硬校验入口；GET 路径调用 NormalizeSupportTicketPriority 即可。
func ValidateSupportTicketPriority(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case SupportTicketPriorityLow, SupportTicketPriorityNormal, SupportTicketPriorityHigh:
		return v, nil
	}
	return "", infraerrors.BadRequest(
		"INVALID_SUPPORT_TICKET_PRIORITY",
		"priority must be one of: low, normal, high",
	)
}

func cloneSupportTicketDefaultCategories() []string {
	out := make([]string, len(SupportTicketDefaultCategories))
	copy(out, SupportTicketDefaultCategories)
	return out
}

// normalizeSupportTicketNotifyEmails 是 setting update 路径的宽松归一化：
//   - 去除 Email 为空 / 超长的项（不报错）
//   - 按小写 Email 做去重（保留第一个出现，保留其 Disabled/Verified 状态）
//   - 超出 SupportTicketNotifyEmailsMaxCount 直接截断
//
// 保留 Disabled/Verified 字段是为了对齐 AccountQuotaNotifyEmails 的 UI 组件
// （前端表单可禁用某项而不删除）。这里 lenient 处理是为了让保存不会因为
// 边界值失败——严格校验（比如 email 语法）在 admin UI 层单项做即可。
func normalizeSupportTicketNotifyEmails(entries []NotifyEmailEntry) []NotifyEmailEntry {
	if len(entries) == 0 {
		return []NotifyEmailEntry{}
	}
	seen := make(map[string]struct{}, len(entries))
	out := make([]NotifyEmailEntry, 0, len(entries))
	for _, e := range entries {
		trimmed := strings.TrimSpace(e.Email)
		if trimmed == "" {
			continue
		}
		if utf8.RuneCountInString(trimmed) > SupportTicketNotifyEmailMaxLen {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, NotifyEmailEntry{
			Email:    trimmed,
			Disabled: e.Disabled,
			Verified: e.Verified,
		})
		if len(out) >= SupportTicketNotifyEmailsMaxCount {
			break
		}
	}
	return out
}
