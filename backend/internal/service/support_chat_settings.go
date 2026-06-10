// Package service — support_chat_settings.go 提供客服浮窗（add-support-chat-widget）
// 配置项的 parse / normalize / marshal / clamp helper。模式仿照
// support_ticket_settings.go：
//
//   - Parse* 用于 GET 路径，失败/损坏时返回安全默认（前端拿到的永远是合法值）。
//   - Normalize*/Validate* 用于 PUT 严格校验（admin 错误输入返 400）。
//   - Marshal* 写库前的 JSON 编码。
//
// 当前文件只覆盖那些 schema 复杂的字段（excluded_routes 字符串数组、faqs JSON 数组、
// 范围受限的 int 值）。简单 string / bool 字段直接在 setting_service 内联处理。
package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ====================================================================
// Excluded routes (string[])
// ====================================================================

// ParseSupportChatExcludedRoutes 把已持久化的 JSON 字符串解析回 []string。
// 解析失败 / 空值时回退到 SupportChatDefaultExcludedRoutes 的拷贝。
// GET 路径绝不返回 nil。
func ParseSupportChatExcludedRoutes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return cloneSupportChatDefaultExcludedRoutes()
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return cloneSupportChatDefaultExcludedRoutes()
	}
	out, err := normalizeSupportChatExcludedRoutes(items, false)
	if err != nil {
		return cloneSupportChatDefaultExcludedRoutes()
	}
	if out == nil {
		return []string{}
	}
	return out
}

// NormalizeSupportChatExcludedRoutes 严格校验：
//   - 0..SupportChatExcludedRoutesMaxItem 项（允许 admin 清空）
//   - 每项 trim 后 1..SupportChatExcludedRouteMaxLen 字符
//   - 必须以 `/` 开头
//   - 不重复
func NormalizeSupportChatExcludedRoutes(raw []string) ([]string, error) {
	return normalizeSupportChatExcludedRoutes(raw, true)
}

func normalizeSupportChatExcludedRoutes(raw []string, strict bool) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			if strict {
				return nil, infraerrors.BadRequest("INVALID_SUPPORT_CHAT_EXCLUDED_ROUTES", "excluded route must not be blank")
			}
			continue
		}
		if !strings.HasPrefix(trimmed, "/") {
			if strict {
				return nil, infraerrors.BadRequest(
					"INVALID_SUPPORT_CHAT_EXCLUDED_ROUTES",
					fmt.Sprintf("excluded route %q must start with /", trimmed),
				)
			}
			continue
		}
		if utf8.RuneCountInString(trimmed) > SupportChatExcludedRouteMaxLen {
			if strict {
				return nil, infraerrors.BadRequest(
					"INVALID_SUPPORT_CHAT_EXCLUDED_ROUTES",
					fmt.Sprintf("excluded route %q exceeds %d characters", trimmed, SupportChatExcludedRouteMaxLen),
				)
			}
			continue
		}
		if _, dup := seen[trimmed]; dup {
			if strict {
				return nil, infraerrors.BadRequest(
					"INVALID_SUPPORT_CHAT_EXCLUDED_ROUTES",
					fmt.Sprintf("duplicate excluded route %q", trimmed),
				)
			}
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) > SupportChatExcludedRoutesMaxItem {
		if strict {
			return nil, infraerrors.BadRequest(
				"INVALID_SUPPORT_CHAT_EXCLUDED_ROUTES",
				fmt.Sprintf("excluded routes must contain at most %d items", SupportChatExcludedRoutesMaxItem),
			)
		}
		out = out[:SupportChatExcludedRoutesMaxItem]
	}
	return out, nil
}

// MarshalSupportChatExcludedRoutes 编码已校验过的 []string 为可持久化 JSON。
func MarshalSupportChatExcludedRoutes(items []string) (string, error) {
	if items == nil {
		items = []string{}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("marshal support chat excluded routes: %w", err)
	}
	return string(b), nil
}

func cloneSupportChatDefaultExcludedRoutes() []string {
	out := make([]string, len(SupportChatDefaultExcludedRoutes))
	copy(out, SupportChatDefaultExcludedRoutes)
	return out
}

// defaultSupportChatExcludedRoutesJSON 给 EnsureDefaults 写库用。
func defaultSupportChatExcludedRoutesJSON() string {
	out, err := MarshalSupportChatExcludedRoutes(cloneSupportChatDefaultExcludedRoutes())
	if err != nil {
		return "[]"
	}
	return out
}

// ====================================================================
// FAQs (JSON 数组：{question, answer, sort_order, enabled})
// ====================================================================

// SupportChatFAQ 是单条 FAQ 的内部结构（与持久化 JSON 一致）。
type SupportChatFAQ struct {
	Question  string `json:"question"`
	Answer    string `json:"answer"`
	SortOrder int    `json:"sort_order"`
	Enabled   bool   `json:"enabled"`
}

// ParseSupportChatFAQs 把已持久化的 JSON 字符串解析回 []SupportChatFAQ。
// 失败 / 空值时返回空数组（GET 路径承诺非 nil）。
func ParseSupportChatFAQs(raw string) []SupportChatFAQ {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return []SupportChatFAQ{}
	}
	var items []SupportChatFAQ
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []SupportChatFAQ{}
	}
	if items == nil {
		return []SupportChatFAQ{}
	}
	return items
}

// NormalizeSupportChatFAQs 严格校验：
//   - 0..SupportChatFAQMaxItems 条
//   - question trim 后 1..SupportChatFAQQuestionMaxLen 字符（rune）
//   - answer  trim 后 1..SupportChatFAQAnswerMaxLen 字符（rune）
//   - sort_order 任意（仅排序用，不校验范围）
//   - 不去重（admin 可能故意配同 question 不同 answer 的多个变体）
func NormalizeSupportChatFAQs(items []SupportChatFAQ) ([]SupportChatFAQ, error) {
	if len(items) == 0 {
		return []SupportChatFAQ{}, nil
	}
	if len(items) > SupportChatFAQMaxItems {
		return nil, infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_FAQS",
			fmt.Sprintf("faqs must contain at most %d items", SupportChatFAQMaxItems),
		)
	}
	out := make([]SupportChatFAQ, 0, len(items))
	for i, raw := range items {
		q := strings.TrimSpace(raw.Question)
		a := strings.TrimSpace(raw.Answer)
		qLen := utf8.RuneCountInString(q)
		aLen := utf8.RuneCountInString(a)
		if qLen < 1 || qLen > SupportChatFAQQuestionMaxLen {
			return nil, infraerrors.BadRequest(
				"INVALID_SUPPORT_CHAT_FAQS",
				fmt.Sprintf("faqs[%d].question must be 1..%d characters", i, SupportChatFAQQuestionMaxLen),
			)
		}
		if aLen < 1 || aLen > SupportChatFAQAnswerMaxLen {
			return nil, infraerrors.BadRequest(
				"INVALID_SUPPORT_CHAT_FAQS",
				fmt.Sprintf("faqs[%d].answer must be 1..%d characters", i, SupportChatFAQAnswerMaxLen),
			)
		}
		out = append(out, SupportChatFAQ{
			Question:  q,
			Answer:    a,
			SortOrder: raw.SortOrder,
			Enabled:   raw.Enabled,
		})
	}
	return out, nil
}

// MarshalSupportChatFAQs 编码已校验过的 []SupportChatFAQ 为持久化 JSON。
func MarshalSupportChatFAQs(items []SupportChatFAQ) (string, error) {
	if items == nil {
		items = []SupportChatFAQ{}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("marshal support chat faqs: %w", err)
	}
	return string(b), nil
}

// ====================================================================
// 范围裁剪 helper（int 类）
// ====================================================================

// ClampSupportChatMaxTurns 把任意整数裁剪到 [SupportChatMaxTurnsMin, SupportChatMaxTurnsMax]，
// 0 / 负数回退到默认值；GET 路径用，永远返回合法值。
func ClampSupportChatMaxTurns(v int) int {
	if v <= 0 {
		return SupportChatMaxTurnsDefault
	}
	if v < SupportChatMaxTurnsMin {
		return SupportChatMaxTurnsMin
	}
	if v > SupportChatMaxTurnsMax {
		return SupportChatMaxTurnsMax
	}
	return v
}

// ValidateSupportChatMaxTurns 严格校验区间，PUT 路径用。
func ValidateSupportChatMaxTurns(v int) (int, error) {
	if v < SupportChatMaxTurnsMin || v > SupportChatMaxTurnsMax {
		return 0, infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_MAX_TURNS",
			fmt.Sprintf("max_turns must be in [%d, %d]", SupportChatMaxTurnsMin, SupportChatMaxTurnsMax),
		)
	}
	return v, nil
}

// ClampSupportChatMaxRequestTokens 同语义，max_request_tokens 区间裁剪。
func ClampSupportChatMaxRequestTokens(v int) int {
	if v <= 0 {
		return SupportChatMaxRequestTokensDef
	}
	if v < SupportChatMaxRequestTokensMin {
		return SupportChatMaxRequestTokensMin
	}
	if v > SupportChatMaxRequestTokensMax {
		return SupportChatMaxRequestTokensMax
	}
	return v
}

// ValidateSupportChatMaxRequestTokens 严格校验。
func ValidateSupportChatMaxRequestTokens(v int) (int, error) {
	if v < SupportChatMaxRequestTokensMin || v > SupportChatMaxRequestTokensMax {
		return 0, infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_MAX_REQUEST_TOKENS",
			fmt.Sprintf("max_request_tokens must be in [%d, %d]", SupportChatMaxRequestTokensMin, SupportChatMaxRequestTokensMax),
		)
	}
	return v, nil
}

// ClampSupportChatRateLimit 用于 rl_user_per_day / rl_user_per_min / rl_ip_per_hour。
// 0 / 负数 / NaN 这些异常情况回退到 fallback（caller 提供该 setting 的默认值）。
func ClampSupportChatRateLimit(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	if v < SupportChatRateLimitMin {
		return SupportChatRateLimitMin
	}
	if v > SupportChatRateLimitMax {
		return SupportChatRateLimitMax
	}
	return v
}

// ValidateSupportChatRateLimit 严格校验，PUT 路径用。fieldName 进入错误描述便于定位。
func ValidateSupportChatRateLimit(v int, fieldName string) (int, error) {
	if v < SupportChatRateLimitMin || v > SupportChatRateLimitMax {
		return 0, infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RATE_LIMIT",
			fmt.Sprintf("%s must be in [%d, %d]", fieldName, SupportChatRateLimitMin, SupportChatRateLimitMax),
		)
	}
	return v, nil
}
