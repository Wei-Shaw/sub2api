// Package service — support_chat_service.go
//
// SupportChatService 提供客服浮窗（add-support-chat-widget）运行时所需的"配置投影"层。
// 它把分散在 SettingService 多个 key 上的 support_chat_* 设置聚合成一个 SupportChatRuntime
// 快照，handler 一次性拿到所有需要的字段，避免在 SSE 处理热路径上反复查 settings。
//
// 之所以单独建 service 而不是直接在 handler 里调 SettingService.GetSystemSettings：
//   - GetSystemSettings 读取的字段集是"管理员后台 GET /admin/settings"全量结构，
//     携带几十个我们用不到的字段；support_chat 跑在用户面 SSE 热路径上，希望尽可能精简。
//   - 模式与 SupportTicketService.GetSupportTicketRuntime 一致，便于读者按图索骥。
package service

import (
	"context"
	"strconv"
	"strings"
)

// SupportChatRuntime 是客服浮窗运行时的配置投影：
// 包含三个公开字段（前端注入需要）+ 11 个仅服务端使用的 LLM/限流/FAQ 字段。
//
// 只读快照：handler 拿到后不应修改其中的 ExcludedRoutes / FAQs 切片。
type SupportChatRuntime struct {
	Enabled            bool
	ExcludedRoutes     []string
	AnonymousLLM       bool
	Title              string
	Welcome            string
	Icon               string
	LLMEnabled         bool
	// 外部 OpenAI-compatible upstream 凭据。LLMAPIKey 是 cleartext（仅服务端运行时使用，
	// 切勿暴露给前端；admin GET 响应里的同名字段是掩码值）。
	// 由 change-support-chat-external-llm 引入，替代旧的 APIKeyID。
	LLMBaseURL         string
	LLMAPIKey          string
	Model              string
	SystemPrompt       string
	MaxTurns           int
	MaxRequestTokens   int
	RLUserPerDay       int
	RLUserPerMin       int
	RLIPPerHour        int
	FAQs               []SupportChatFAQ
}

// GetSupportChatRuntime 一次性读取所有 support_chat_* 设置并应用 Parse*/Clamp* helper，
// 任何持久值损坏（JSON 解析失败、整数越界）都会回退到合法默认；handler 拿到的快照
// 一定可以放心使用。
//
// 注意：本方法走 settingRepo.GetMultiple 单次 round-trip，不进任何 cache。
// SSE 热路径上是否需要缓存 runtime 由 handler 自己决定；当前未引入 cache 的原因：
//
//	1. SSE 一次会话 ~1 分钟，期间 admin 修改 settings 的概率极低；
//	2. settingRepo 内部已经走了 setting cache（参考 settings_cache）；
//	3. 引入额外缓存会让 admin 改完配置仍需等 TTL 才生效，体验降级。
func (s *SettingService) GetSupportChatRuntime(ctx context.Context) SupportChatRuntime {
	keys := []string{
		SettingKeySupportChatEnabled,
		SettingKeySupportChatExcludedRoutes,
		SettingKeySupportChatAnonymousLLM,
		SettingKeySupportChatTitle,
		SettingKeySupportChatWelcome,
		SettingKeySupportChatIcon,
		SettingKeySupportChatLLMEnabled,
		SettingKeySupportChatLLMBaseURL,
		SettingKeySupportChatLLMAPIKey,
		SettingKeySupportChatModel,
		SettingKeySupportChatSystemPrompt,
		SettingKeySupportChatMaxTurns,
		SettingKeySupportChatMaxRequestTokens,
		SettingKeySupportChatRLUserPerDay,
		SettingKeySupportChatRLUserPerMin,
		SettingKeySupportChatRLIPPerHour,
		SettingKeySupportChatFAQs,
	}
	vals, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		// settings store 不可达：回退到全部默认值，handler 据 Enabled=false 直接 404。
		return SupportChatRuntime{
			Enabled:          false,
			ExcludedRoutes:   cloneSupportChatDefaultExcludedRoutes(),
			AnonymousLLM:     false,
			Title:            SupportChatDefaultTitle,
			Welcome:          SupportChatDefaultWelcome,
			Icon:             SupportChatDefaultIcon,
			LLMEnabled:       false,
			LLMBaseURL:       "",
			LLMAPIKey:        "",
			Model:            SupportChatDefaultModel,
			SystemPrompt:     "",
			MaxTurns:         SupportChatMaxTurnsDefault,
			MaxRequestTokens: SupportChatMaxRequestTokensDef,
			RLUserPerDay:     SupportChatRLUserPerDayDefault,
			RLUserPerMin:     SupportChatRLUserPerMinDefault,
			RLIPPerHour:      SupportChatRLIPPerHourDefault,
			FAQs:             []SupportChatFAQ{},
		}
	}

	rt := SupportChatRuntime{
		Enabled:        vals[SettingKeySupportChatEnabled] == "true",
		ExcludedRoutes: ParseSupportChatExcludedRoutes(vals[SettingKeySupportChatExcludedRoutes]),
		AnonymousLLM:   vals[SettingKeySupportChatAnonymousLLM] == "true",
		Title:          strings.TrimSpace(vals[SettingKeySupportChatTitle]),
		Welcome:        strings.TrimSpace(vals[SettingKeySupportChatWelcome]),
		Icon:           strings.TrimSpace(vals[SettingKeySupportChatIcon]),
		LLMEnabled:     vals[SettingKeySupportChatLLMEnabled] == "true",
		LLMBaseURL:     strings.TrimSpace(vals[SettingKeySupportChatLLMBaseURL]),
		LLMAPIKey:      strings.TrimSpace(vals[SettingKeySupportChatLLMAPIKey]),
		Model:          strings.TrimSpace(vals[SettingKeySupportChatModel]),
		SystemPrompt:   vals[SettingKeySupportChatSystemPrompt],
		FAQs:           ParseSupportChatFAQs(vals[SettingKeySupportChatFAQs]),
	}
	if rt.Title == "" {
		rt.Title = SupportChatDefaultTitle
	}
	if rt.Welcome == "" {
		rt.Welcome = SupportChatDefaultWelcome
	}
	if rt.Icon == "" {
		rt.Icon = SupportChatDefaultIcon
	}
	if rt.Model == "" {
		rt.Model = SupportChatDefaultModel
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals[SettingKeySupportChatMaxTurns])); err == nil {
		rt.MaxTurns = ClampSupportChatMaxTurns(v)
	} else {
		rt.MaxTurns = SupportChatMaxTurnsDefault
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals[SettingKeySupportChatMaxRequestTokens])); err == nil {
		rt.MaxRequestTokens = ClampSupportChatMaxRequestTokens(v)
	} else {
		rt.MaxRequestTokens = SupportChatMaxRequestTokensDef
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals[SettingKeySupportChatRLUserPerDay])); err == nil {
		rt.RLUserPerDay = ClampSupportChatRateLimit(v, SupportChatRLUserPerDayDefault)
	} else {
		rt.RLUserPerDay = SupportChatRLUserPerDayDefault
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals[SettingKeySupportChatRLUserPerMin])); err == nil {
		rt.RLUserPerMin = ClampSupportChatRateLimit(v, SupportChatRLUserPerMinDefault)
	} else {
		rt.RLUserPerMin = SupportChatRLUserPerMinDefault
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals[SettingKeySupportChatRLIPPerHour])); err == nil {
		rt.RLIPPerHour = ClampSupportChatRateLimit(v, SupportChatRLIPPerHourDefault)
	} else {
		rt.RLIPPerHour = SupportChatRLIPPerHourDefault
	}
	return rt
}

// GetSupportChatLLMCredentials 是 GetSupportChatRuntime 的轻量便捷方法：
// 仅读取 base_url + api_key 两条 setting，供 embedding service / RAG retrieval helper
// 在不需要其它运行时字段时调用，省去整组 support_chat_* 的 GetMultiple 与默认值套用。
//
// 返回值与 SupportChatRuntime 中同名字段语义一致：cleartext，仅服务端运行时使用。
// 任何一边为空字符串都视为"未配置"，调用方应据此走兜底分支（embedding=NULL / 空检索结果）。
func (s *SettingService) GetSupportChatLLMCredentials(ctx context.Context) (baseURL, apiKey string) {
	keys := []string{SettingKeySupportChatLLMBaseURL, SettingKeySupportChatLLMAPIKey}
	vals, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(vals[SettingKeySupportChatLLMBaseURL]), strings.TrimSpace(vals[SettingKeySupportChatLLMAPIKey])
}

// MaskSupportChatLLMAPIKey 把外部 LLM api_key 转成"展示给 admin GET 响应"的掩码。
// 由 change-support-chat-external-llm 引入，用于：
//   - parseSettings/GetSystemSettings 在 GET 响应里把 api_key 掩码后再返回；
//   - buildSystemSettingsUpdates 在 PUT 时识别"请求里的 api_key 等于当前存储值的掩码"
//     这种"未改动"语义，跳过该字段写入；
//   - admin TestLLMConnection handler 在前端只发回掩码时，识别"沿用已存 api_key"。
//
// 导出供 handler 包复用（保持单一规则源，避免两边各写一份易漂移）。
//
// 规则与前端约定：
//   - 空字符串：返回 ""（前端据此判断"还没配置"）；
//   - len < 4：返回 "***"（极短值不暴露任何前缀/尾部，避免泄漏）；
//   - len >= 4：返回 "sk-***" + 最后 4 位（与 OpenAI 风格 sk- 前缀对齐，
//     即便存储值不是真的以 sk- 开头也使用同一展示前缀以避免泄漏真实前缀）。
func MaskSupportChatLLMAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) < 4 {
		return "***"
	}
	return "sk-***" + value[len(value)-4:]
}
