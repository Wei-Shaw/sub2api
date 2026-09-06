package admin

import "encoding/json"

// PUT /settings 的响应瘦身：保存只需要回传「这次发过来的字段 + 其只读伴生键」，
// 而不是整份 800KB 的设置文档（GET 才负责整份）。客户端（SettingsView）的合并
// 逻辑已按「键存在才写」设计，因此少回传键是安全的。
//
// 契约：
//   - 发送过的键 → 回传持久化后的终值（含后端 clamp/归一化的结果）；
//   - 未知/多余的发送键 → 自然落空（响应 map 里没有就不回传）；
//   - always 键 → 无论是否发送都回传（保存路径会 fail-closed 重推导它们，
//     客户端需要拿到重推导后的值）；
//   - 伴生键 → 只读派生键（*_configured / *_effective_* 等），跟随触发它的
//     可写字段一起回传。
//
// 维护提醒：如果给响应新增了派生自请求字段的只读键（例如新的 *_configured），
// 必须同步登记到 putResponseCompanions，否则部分载荷保存后客户端拿不到它。

// putResponseAlwaysKeys 每次保存 handler 都会重推导的安全键，即使未发送也回传。
var putResponseAlwaysKeys = []string{
	"api_key_acl_trust_forwarded_ip",
	"forwarded_client_ip_headers",
}

// putResponseCompanions 发送键 → 需要一并回传的只读伴生响应键。
var putResponseCompanions = map[string][]string{
	// 密钥字段：GET 永不回传明文，客户端靠 *_configured 判断已配置状态。
	// 发送密钥（含发送空串清除）后必须回传新的 *_configured。
	"smtp_password":                    {"smtp_password_configured"},
	"turnstile_secret_key":             {"turnstile_secret_key_configured"},
	"tencent_captcha_app_secret_key":   {"tencent_captcha_app_secret_key_configured"},
	"tencent_captcha_cloud_secret_id":  {"tencent_captcha_cloud_secret_id_configured"},
	"tencent_captcha_cloud_secret_key": {"tencent_captcha_cloud_secret_key_configured"},
	"aliyun_captcha_access_key_secret": {"aliyun_captcha_access_key_secret_configured"},
	"linuxdo_connect_client_secret":    {"linuxdo_connect_client_secret_configured"},
	"dingtalk_connect_client_secret":   {"dingtalk_connect_client_secret_configured"},
	"wechat_connect_app_secret":        {"wechat_connect_app_secret_configured"},
	"wechat_connect_open_app_secret":   {"wechat_connect_open_app_secret_configured"},
	"wechat_connect_mp_app_secret":     {"wechat_connect_mp_app_secret_configured"},
	"wechat_connect_mobile_app_secret": {"wechat_connect_mobile_app_secret_configured"},
	"oidc_connect_client_secret":       {"oidc_connect_client_secret_configured"},
	"github_oauth_client_secret":       {"github_oauth_client_secret_configured"},
	"google_oauth_client_secret":       {"google_oauth_client_secret_configured"},

	// passkey_configured 由部署配置派生，开启 passkey_enabled 时有 gate 校验，
	// 客户端需要看到校验后的状态。
	"passkey_enabled": {"passkey_configured"},

	// codex 版本自动同步会改写 openai_codex_client_version，终值经 *_synced 体现。
	"openai_codex_client_version":            {"openai_codex_client_version_synced"},
	"openai_codex_version_auto_sync_enabled": {"openai_codex_client_version_synced"},
}

// 调度器 14 个可写请求键共享同一组 12 个 effective 只读派生键，
// 单列出来避免上面 map 里重复 14 遍。
var openAIAdvancedSchedulerEffectiveKeys = []string{
	"openai_advanced_scheduler_effective_lb_top_k",
	"openai_advanced_scheduler_effective_weight_priority",
	"openai_advanced_scheduler_effective_weight_load",
	"openai_advanced_scheduler_effective_weight_queue",
	"openai_advanced_scheduler_effective_weight_error_rate",
	"openai_advanced_scheduler_effective_weight_ttft",
	"openai_advanced_scheduler_effective_weight_reset",
	"openai_advanced_scheduler_effective_weight_quota_headroom",
	"openai_advanced_scheduler_effective_weight_upstream_cost",
	"openai_advanced_scheduler_effective_weight_previous_response",
	"openai_advanced_scheduler_effective_weight_session_sticky",
}

func init() {
	for _, key := range []string{
		"openai_advanced_scheduler_enabled",
		"openai_advanced_scheduler_sticky_weighted_enabled",
		"openai_advanced_scheduler_subscription_priority_enabled",
		"openai_advanced_scheduler_lb_top_k",
		"openai_advanced_scheduler_weight_priority",
		"openai_advanced_scheduler_weight_load",
		"openai_advanced_scheduler_weight_queue",
		"openai_advanced_scheduler_weight_error_rate",
		"openai_advanced_scheduler_weight_ttft",
		"openai_advanced_scheduler_weight_reset",
		"openai_advanced_scheduler_weight_quota_headroom",
		"openai_advanced_scheduler_weight_upstream_cost",
		"openai_advanced_scheduler_weight_previous_response",
		"openai_advanced_scheduler_weight_session_sticky",
	} {
		putResponseCompanions[key] = append(
			putResponseCompanions[key], openAIAdvancedSchedulerEffectiveKeys...)
	}
}

// filterSystemSettingsResponseForPut 把完整的设置响应裁剪成部分载荷保存所需的最小子集：
// always 键 ∪ 发送键 ∪ 发送键的伴生键。data 里不存在但客户端发送的键自然被忽略。
func filterSystemSettingsResponseForPut(data map[string]any, sentFields map[string]json.RawMessage) map[string]any {
	filtered := make(map[string]any, len(sentFields)+len(putResponseAlwaysKeys)+4)
	copyIfPresent := func(key string) {
		if v, ok := data[key]; ok {
			filtered[key] = v
		}
	}
	for _, key := range putResponseAlwaysKeys {
		copyIfPresent(key)
	}
	for key := range sentFields {
		copyIfPresent(key)
		for _, companion := range putResponseCompanions[key] {
			copyIfPresent(companion)
		}
	}
	return filtered
}
