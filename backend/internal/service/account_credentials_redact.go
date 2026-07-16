package service

// SensitiveCredentialKeys 列出 Account.Credentials JSON map 中绝不允许返回到前端的子键。
// dto 层做响应脱敏、service 层做更新合并都引用此清单——新增凭证类型时务必同步。
var SensitiveCredentialKeys = []string{
	// OAuth
	"access_token", "refresh_token", "id_token", "agent_private_key",
	// API Key 类
	"api_key", "session_key", "cookie",
	// 云服务凭据
	"aws_secret_access_key", "aws_session_token",
	"service_account_json", "service_account", "private_key",
}

var sensitiveCredentialKeySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(SensitiveCredentialKeys))
	for _, k := range SensitiveCredentialKeys {
		m[k] = struct{}{}
	}
	return m
}()

// IsSensitiveCredentialKey 判断指定键是否为敏感凭证子键。
func IsSensitiveCredentialKey(key string) bool {
	_, ok := sensitiveCredentialKeySet[key]
	return ok
}

// MergePreservingSensitiveCreds 把 incoming 写入 existing 之上，但敏感子键采用"incoming 没提供就保留 existing"
// 的语义。返回新的 map，不修改入参。
//
// 用途：前端编辑账号通常采用"全对象 PUT"模式；脱敏后前端 spread 旧 credentials 时不会带上敏感键，
// 直接覆盖会清空已有 token。此函数保证：
//   - 非敏感键：完全由 incoming 决定（用户可以编辑、删除非敏感字段）。
//   - 敏感键：incoming 显式提供则覆盖（用户主动旋转 token），否则保留 existing。
func MergePreservingSensitiveCreds(existing, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(incoming)+len(SensitiveCredentialKeys))
	for k, v := range incoming {
		out[k] = v
	}
	for _, key := range SensitiveCredentialKeys {
		if _, hasIncoming := incoming[key]; hasIncoming {
			continue
		}
		if existingVal, ok := existing[key]; ok {
			out[key] = existingVal
		}
	}
	return out
}
