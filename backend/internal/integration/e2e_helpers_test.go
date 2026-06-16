//go:build e2e

package integration

import (
	"os"
	"strings"
	"testing"
)

// =============================================================================
// E2E Mock 模式支持
// =============================================================================
// 当 E2E_MOCK=true 时，使用本地 Mock 响应替代真实 API 调用。
// 这允许在没有真实 API Key 的环境（如 CI）中验证基本的请求/响应流程。

// isMockMode 检查是否启用 Mock 模式
func isMockMode() bool {
	return strings.EqualFold(os.Getenv("E2E_MOCK"), "true")
}

// skipIfNoRealAPI 如果未配置真实 API Key 且不在 Mock 模式，则跳过测试
func skipIfNoRealAPI(t *testing.T) {
	t.Helper()
	if isMockMode() {
		return // Mock 模式下不跳过
	}
	claudeKey := strings.TrimSpace(os.Getenv(claudeAPIKeyEnv))
	geminiKey := strings.TrimSpace(os.Getenv(geminiAPIKeyEnv))
	if claudeKey == "" && geminiKey == "" {
		t.Skip("未设置 API Key 且未启用 Mock 模式，跳过测试")
	}
}

// =============================================================================
// API Key 脱敏（Task 6.10）
// =============================================================================

// safeLogKey 安全地记录 API Key（仅显示前 8 位）
func safeLogKey(t *testing.T, prefix string, key string) {
	t.Helper()
	key = strings.TrimSpace(key)
	if len(key) <= 8 {
		t.Logf("%s: ***（长度: %d）", prefix, len(key))
		return
	}
	t.Logf("%s: %s...（长度: %d）", prefix, key[:8], len(key))
}
