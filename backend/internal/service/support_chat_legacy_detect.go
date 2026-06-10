// Package service — support_chat_legacy_detect.go
//
// 启动期一次性检测：是否存在 add-support-chat-widget 时代写入的 `support_chat_api_key_id`
// （legacy setting，由 change-support-chat-external-llm 替换为 base_url + api_key 双键）。
//
// 用途：admin 升级到新版本后，旧的 `support_chat_api_key_id` 行可能仍残留在 settings 表里
// （为 support 回滚我们故意不删 DB 中的旧值），但运行时已经不再读取这个键。
// 这里**只**打一条 warn log 提示重新配置，不修改任何 setting，便于：
//
//  1. admin 升级版本后第一次重启就能在日志里看到 "需要重新配置" 的提示；
//  2. 即便 admin 一时没注意，也不会因为残留 row 把新版本的行为搞坏（chat 主流程已经在
//     SupportChatRuntime / handler pre-flight 用 base_url+api_key 兜底）；
//  3. 万一发现严重问题需要回滚到旧版本，旧版本读到原始 `support_chat_api_key_id` 仍能工作。
//
// 由 wire.go 通过 ProvideSupportChatLegacyDetector 注册为启动副作用。
package service

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
)

// SupportChatLegacyDetector 封装一次性 legacy 检测。
//
// 故意定义成 struct 而不是裸函数：
//
//	a) 与 SupportFaqMigrationService 风格一致，便于将来加状态字段（如 didRun bool）；
//	b) wire 图里有名字的依赖比 init() 副作用更易追踪。
type SupportChatLegacyDetector struct {
	settings *SettingService
}

// NewSupportChatLegacyDetector 构造检测器（不立即触发，由 caller 主动 Run）。
func NewSupportChatLegacyDetector(settings *SettingService) *SupportChatLegacyDetector {
	return &SupportChatLegacyDetector{settings: settings}
}

// legacySupportChatAPIKeyIDKey 是 add-support-chat-widget 时代的 setting key。
// 不引入 domain_constants 中的常量（已在 §1.1 删除）。
const legacySupportChatAPIKeyIDKey = "support_chat_api_key_id"

// Run 执行一次性检测：如果 legacy `support_chat_api_key_id > 0` AND 新 `support_chat_llm_base_url`
// 仍是空，就 warn 一条结构化日志，提示 admin 去后台 Settings 重新配置。
//
// 该方法**幂等且无副作用**：
//   - 不写任何 setting；
//   - 不删除 legacy 行（保留以支持版本回滚）；
//   - 多次调用结果一致（仅日志一致）。
//
// 任意错误（DB 读失败、字段非法）都 silent skip，启动流程不应该被告警逻辑阻塞。
func (d *SupportChatLegacyDetector) Run(ctx context.Context) {
	if d == nil || d.settings == nil || d.settings.settingRepo == nil {
		return
	}

	keys := []string{legacySupportChatAPIKeyIDKey, SettingKeySupportChatLLMBaseURL}
	values, err := d.settings.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		slog.WarnContext(ctx, "support_chat_legacy_detect: read settings failed, skipping",
			slog.Any("err", err))
		return
	}

	rawLegacy := strings.TrimSpace(values[legacySupportChatAPIKeyIDKey])
	if rawLegacy == "" {
		return // 没有 legacy 行（fresh install / 已被人手清理），无需提醒。
	}
	legacyID, perr := strconv.Atoi(rawLegacy)
	if perr != nil || legacyID <= 0 {
		// "0" / 非数字都视为"未配置"，不提示。
		return
	}

	newBaseURL := strings.TrimSpace(values[SettingKeySupportChatLLMBaseURL])
	if newBaseURL != "" {
		// admin 已经按新方案配过 base_url，legacy 行只是"历史遗留"，不需提醒。
		return
	}

	slog.Warn(
		"legacy support_chat_api_key_id detected; please reconfigure support_chat_llm_base_url + support_chat_llm_api_key in admin Settings; LLM chat will be disabled until reconfigured",
		slog.Int("legacy_api_key_id", legacyID),
	)
}

// ProvideSupportChatLegacyDetector wire helper：构造 + 立即在后台触发一次 Run。
//
// 与 ProvideSupportFaqMigrationService 风格一致：把启动副作用封进 wire 图，
// 避免 main.go 显式调用而漏调；用 goroutine 包裹是因为：
//
//	a) 启动期 root context 还未建立，避免阻塞 wire 构造链；
//	b) 即便 DB 暂时慢/不可达，也不该让整个进程卡在启动检测上。
func ProvideSupportChatLegacyDetector(settings *SettingService) *SupportChatLegacyDetector {
	d := NewSupportChatLegacyDetector(settings)
	go func() {
		d.Run(context.Background())
	}()
	return d
}
