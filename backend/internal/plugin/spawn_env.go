package plugin

import (
	"os"
	"strings"
)

// sensitiveEnvPrefixes 列出被视为敏感的环境变量名前缀。
// 匹配时不区分大小写（Windows 环境变量名大小写不敏感）。
// 新增敏感变量类别时只需在此追加前缀，spawnPipe 的调用方无需改动。
var sensitiveEnvPrefixes = []string{
	"POSTGRES_",
	"DATABASE_",
	"REDIS_",
	"JWT_",
	"ADMIN_API_KEY",
	"ENCRYPTION_KEY",
	"SECRET",
	"PASSWORD",
	"TOTP_",
	"PGPASSWORD",
	"SESSION_SECRET",
	"COOKIE_SECRET",
	"STRIPE_",
	"PAYPAL_",
	"WECHAT_PAY_",
	"ALIPAY_",
}

// sanitizedEnvForPlugin 返回 os.Environ() 中过滤掉敏感变量后的副本，
// 供 exec.Cmd.Env 使用。当 cmd.Env 非 nil 时子进程只继承列表中的变量，
// 从而避免数据库密码、JWT 密钥等泄露给插件子进程。
func sanitizedEnvForPlugin() []string {
	return filterSensitiveEnv(os.Environ())
}

// filterSensitiveEnv 是 sanitizedEnvForPlugin 的纯函数内核，便于单元测试。
// 遍历 envs 中每个 "KEY=VALUE" 条目，跳过 KEY 匹配 sensitiveEnvPrefixes
// 中任一前缀的条目（不区分大小写）。
func filterSensitiveEnv(envs []string) []string {
	out := make([]string, 0, len(envs))
	for _, entry := range envs {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			// 格式异常的条目（无 '='），保守丢弃。
			continue
		}
		if isSensitiveEnvKey(key) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// isSensitiveEnvKey 检查 key 是否匹配黑名单中的任一前缀（不区分大小写）。
func isSensitiveEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, prefix := range sensitiveEnvPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}
