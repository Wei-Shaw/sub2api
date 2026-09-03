//go:build unit

package routes

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// settingsStepUpExemptions 列出 /admin/settings 下允许不挂 step-up 门控的写入路由，
// 每条必须写明理由。新增写入路由时要么挂 stepUpAuth，要么在此登记，
// 避免再次出现「整组设置路由静默绕过 step-up」（见 registerSettingsRoutes 注释）。
var settingsStepUpExemptions = map[string]string{
	// 连通性自测，不落库
	"POST /test-smtp":                 "SMTP 连通性自测，不写入任何设置",
	"POST /send-test-email":           "发送测试邮件，不写入任何设置",
	"POST /email-template-preview":    "模板渲染预览，不写入任何设置",
	"POST /web-search-emulation/test": "Web Search 模拟自测，不写入配置",
	// 运营参数：影响调度/限流手感，不构成安全边界，且前端对应入口未接入 step-up 重试
	"PUT /email-templates/:event/:locale":                   "邮件模板正文，非安全边界；前端保存入口尚未接入 step-up 重试",
	"POST /email-templates/:event/:locale/restore-official": "恢复官方邮件模板，仅回退到内置内容",
	"PUT /overload-cooldown":                                "529 过载冷却参数，运营调参",
	"PUT /rate-limit-429-cooldown":                          "429 回避参数，运营调参",
	"PUT /openai-images-oauth-unavailable-cooldown":         "OAuth 生图不可用冷却参数，运营调参",
	"PUT /panel-rate-limit":                                 "面板限流参数，运营调参",
	"PUT /stream-timeout":                                   "流超时参数，运营调参",
	"PUT /rectifier":                                        "请求整流器参数，运营调参",
	"PUT /beta-policy":                                      "Beta 头策略参数，运营调参",
	"PUT /web-search-emulation":                             "Web Search 模拟配置，运营调参",
	"POST /web-search-emulation/reset-usage":                "重置模拟用量计数",
	// 注：POST /admin-api-key/regenerate 与 DELETE /admin-api-key 曾登记在此，
	// 原因是前端尚未接入 step-up 重试。SettingsView.vue 的 createAdminApiKey /
	// deleteAdminApiKey 现已包进 settingsStepUp.run()，两条路由已改为门控，
	// 因此从例外清单移除——不要再加回来。
}

func TestSettingsMutatingRoutesAreStepUpGatedOrExempt(t *testing.T) {
	source, err := os.ReadFile("admin.go")
	require.NoError(t, err)

	body := extractFuncBody(t, string(source), "func registerSettingsRoutes(")

	routePattern := regexp.MustCompile(`adminSettings\.(GET|PUT|POST|DELETE|PATCH)\("([^"]*)",\s*([^\n]*)`)
	matches := routePattern.FindAllStringSubmatch(body, -1)
	require.NotEmpty(t, matches, "registerSettingsRoutes 中没有匹配到任何路由，正则需要同步更新")

	ungated := make([]string, 0)
	seen := map[string]struct{}{}
	for _, m := range matches {
		method, path, rest := m[1], m[2], m[3]
		key := method + " " + path
		seen[key] = struct{}{}
		if method == "GET" {
			// 读取路由刻意不门控：设置页加载时的第一个请求，挂门控等于打开页面就弹 TOTP。
			require.NotContains(t, rest, "stepUpAuth", "%s 不应挂 step-up：读取路由门控会让设置页无法打开", key)
			continue
		}
		if strings.Contains(rest, "gin.HandlerFunc(stepUpAuth)") {
			continue
		}
		if _, ok := settingsStepUpExemptions[key]; ok {
			continue
		}
		ungated = append(ungated, key)
	}
	sort.Strings(ungated)
	require.Empty(t, ungated,
		"新增的 /admin/settings 写入路由必须挂 gin.HandlerFunc(stepUpAuth) 或在 settingsStepUpExemptions 中登记理由")

	// PUT /settings 是全站设置总写入口，门控不得被摘掉。
	require.Contains(t, body, `adminSettings.PUT("", gin.HandlerFunc(stepUpAuth), h.Admin.Setting.UpdateSettings)`,
		"PUT /admin/settings 必须保持 step-up 门控")

	stale := make([]string, 0)
	for key := range settingsStepUpExemptions {
		if _, ok := seen[key]; !ok {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	require.Empty(t, stale, "settingsStepUpExemptions 中存在已删除的路由条目")
}

// extractFuncBody 返回 admin.go 中指定函数签名到下一个顶层 `\n}` 之间的源码。
func extractFuncBody(t *testing.T, source, signature string) string {
	t.Helper()
	start := strings.Index(source, signature)
	require.GreaterOrEqual(t, start, 0, "未找到函数 %s", signature)
	rest := source[start:]
	end := strings.Index(rest, "\n}\n")
	require.GreaterOrEqual(t, end, 0, "未找到函数 %s 的结尾", signature)
	return rest[:end]
}
