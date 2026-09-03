//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// custom_menu_items[].pass_token 决定是否把用户真实的面板 JWT 以 ?token= 透传给内嵌页面。
// 开启时链路必须是 https，否则令牌会以明文出现在网络上；关闭时保持既有行为，
// 允许 http（含内网/回环地址）的非令牌内嵌页继续工作。

func customMenuItem(url string, passToken bool) map[string]any {
	return map[string]any{
		"id":         "help",
		"label":      "Help Center",
		"url":        url,
		"visibility": "user",
		"sort_order": 0,
		"pass_token": passToken,
	}
}

func storedMenuItems(t *testing.T, repo *settingHandlerRepoStub) []dto.CustomMenuItem {
	t.Helper()
	return dto.ParseCustomMenuItems(repo.values[service.SettingKeyCustomMenuItems])
}

func TestUpdateSettingsCustomMenuPassTokenRequiresHTTPS(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{
		"custom_menu_items": []any{customMenuItem("http://vendor.example.com/page", true)},
	}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "pass_token")
	require.Empty(t, repo.values[service.SettingKeyCustomMenuItems],
		"a rejected payload must not be persisted")
}

// md:<slug> 页面根本不走 iframe，没有可透传的对象，同样必须被拒绝而不是静默忽略。
func TestUpdateSettingsCustomMenuPassTokenRejectsMarkdownSlug(t *testing.T) {
	h, _ := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{
		"custom_menu_items": []any{customMenuItem("md:faq", true)},
	}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "pass_token")
}

func TestUpdateSettingsCustomMenuPassTokenAllowsHTTPS(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{
		"custom_menu_items": []any{customMenuItem("https://vendor.example.com/page", true)},
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	items := storedMenuItems(t, repo)
	require.Len(t, items, 1)
	require.True(t, items[0].PassToken)
}

// 关闭透传时 http:// 仍然合法：现存的非令牌内嵌页不能因为这次收紧而失效。
func TestUpdateSettingsCustomMenuPlainHTTPAllowedWithoutPassToken(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{
		"custom_menu_items": []any{customMenuItem("http://vendor.example.com/page", false)},
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	items := storedMenuItems(t, repo)
	require.Len(t, items, 1)
	require.False(t, items[0].PassToken)
}

// 历史数据里没有 pass_token 字段，反序列化后必须落到 false（默认不透传）。
func TestParseCustomMenuItemsLegacyEntryDefaultsToNoTokenPassthrough(t *testing.T) {
	raw := `[{"id":"help","label":"Help","url":"https://vendor.example.com","visibility":"user","sort_order":0}]`
	items := dto.ParseCustomMenuItems(raw)

	require.Len(t, items, 1)
	require.False(t, items[0].PassToken)

	encoded, err := json.Marshal(items)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"pass_token":false`)
}
