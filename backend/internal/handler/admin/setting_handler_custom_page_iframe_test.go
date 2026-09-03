//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// custom_page_iframe_hosts 是自定义页面 iframe 策略的唯一事实来源（前端 DOMPurify 白名单
// + CSP frame-src）。它有三态，写入/回显链路上任何一跳把三态压成两态都会让「显式锁死」
// 悄悄退化成「内置默认白名单」——运维以为关掉了嵌入，youtube 照旧可用。
//
// 下面的用例把三态钉死在 PUT /api/v1/admin/settings 这一层：
//   字段缺席   → 库里的键连写都不写（也就不会把已有配置冲掉）；
//   显式 []    → 落库成 "[]"，回显是 []，绝不是默认列表；
//   显式 [...] → 归一化后落库；
//   显式 null  → 落库成 ""，回到「从未配置」，默认列表重新生效；
//   非法条目   → 整体 400 并点名该条目，一个字节都不落库。

// updateSettingsResponseHosts 取出响应体里的 custom_page_iframe_hosts。
// 第二个返回值区分 JSON null（nil）与 []（非 nil 空切片）——这正是三态的关键信号。
func updateSettingsResponseHosts(t *testing.T, body []byte) ([]string, bool) {
	t.Helper()
	var resp struct {
		Data struct {
			CustomPageIframeHosts []string `json:"custom_page_iframe_hosts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	var raw struct {
		Data struct {
			CustomPageIframeHosts json.RawMessage `json:"custom_page_iframe_hosts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &raw))
	present := string(raw.Data.CustomPageIframeHosts) != "null"
	return resp.Data.CustomPageIframeHosts, present
}

// 字段缺席（老客户端全量 PUT）：这个键不能出现在本次写入里，库里也不能凭空长出来。
func TestUpdateSettingsCustomPageIframeHostsOmittedStaysAbsent(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{"site_name": "Sub2API"}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, written := repo.lastUpdates[service.SettingKeyCustomPageIframeHosts]
	require.False(t, written, "omitted field must not be written at all")
	_, stored := repo.values[service.SettingKeyCustomPageIframeHosts]
	require.False(t, stored, "omitted field must not create the setting key")

	// 回显必须是 JSON null：管理端据此显示「当前生效的是内置默认列表」。
	hosts, present := updateSettingsResponseHosts(t, rec.Body.Bytes())
	require.False(t, present, "never-configured must serialize as null, not []")
	require.Nil(t, hosts)

	// 实际生效值仍然是内置默认列表。
	require.Equal(t, service.DefaultCustomPageIframeHosts,
		service.EffectiveCustomPageIframeHosts(repo.values[service.SettingKeyCustomPageIframeHosts]))
}

// 字段缺席且库里已有「显式锁死」：不得被这次保存冲回默认值。
func TestUpdateSettingsCustomPageIframeHostsOmittedKeepsStoredLockdown(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyCustomPageIframeHosts: "[]",
	})

	rec := doUpdateSettings(t, h, map[string]any{"site_name": "Sub2API"}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "[]", repo.values[service.SettingKeyCustomPageIframeHosts])

	hosts, present := updateSettingsResponseHosts(t, rec.Body.Bytes())
	require.True(t, present, "explicit lockdown must serialize as [], not null")
	require.Empty(t, hosts)
}

// 显式空数组 = 显式锁死：落库为 "[]"，读取侧必须解析成「已配置且为空」，不得回落默认值。
func TestUpdateSettingsCustomPageIframeHostsExplicitEmptyIsLockdown(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyCustomPageIframeHosts: `["youtube.com"]`,
	})

	rec := doUpdateSettings(t, h, map[string]any{
		"custom_page_iframe_hosts": []string{},
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "[]", repo.values[service.SettingKeyCustomPageIframeHosts])

	// 服务层解析：configured=true 且 hosts 为空 —— 这是「一个 iframe 都不许嵌」。
	parsed, configured := service.ParseCustomPageIframeHosts(
		repo.values[service.SettingKeyCustomPageIframeHosts],
	)
	require.True(t, configured)
	require.Empty(t, parsed)
	require.Empty(t, service.EffectiveCustomPageIframeHosts(
		repo.values[service.SettingKeyCustomPageIframeHosts],
	), "explicit lockdown must never fall back to the built-in defaults")

	hosts, present := updateSettingsResponseHosts(t, rec.Body.Bytes())
	require.True(t, present)
	require.Empty(t, hosts)
}

// 显式主机列表：大小写/首尾点/重复项被归一化后落库。
func TestUpdateSettingsCustomPageIframeHostsExplicitListIsNormalized(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{
		"custom_page_iframe_hosts": []string{"  Embed.Example.COM ", ".player.vimeo.com.", "embed.example.com", ""},
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.JSONEq(t, `["embed.example.com","player.vimeo.com"]`,
		repo.values[service.SettingKeyCustomPageIframeHosts])

	hosts, present := updateSettingsResponseHosts(t, rec.Body.Bytes())
	require.True(t, present)
	require.Equal(t, []string{"embed.example.com", "player.vimeo.com"}, hosts)
}

// 显式 null = 重置为「从未配置」，内置默认列表重新生效。
func TestUpdateSettingsCustomPageIframeHostsExplicitNullResetsToDefaults(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyCustomPageIframeHosts: "[]",
	})

	rec := doUpdateSettings(t, h, map[string]any{
		"custom_page_iframe_hosts": nil,
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "", repo.values[service.SettingKeyCustomPageIframeHosts])
	require.Equal(t, service.DefaultCustomPageIframeHosts,
		service.EffectiveCustomPageIframeHosts(repo.values[service.SettingKeyCustomPageIframeHosts]))

	hosts, present := updateSettingsResponseHosts(t, rec.Body.Bytes())
	require.False(t, present, "reset to never-configured must serialize as null")
	require.Nil(t, hosts)
}

// 非法条目：写入侧必须整体拒绝并点名，绝不能像读取侧那样静默丢弃——
// 静默丢弃会让运维以为白名单已生效，实际上一条都没进去。
func TestUpdateSettingsCustomPageIframeHostsRejectsInvalidEntry(t *testing.T) {
	for _, entry := range []string{
		"https://youtube.com",
		"youtube.com/embed",
		"*.youtube.com",
		"youtube.com:8443",
		"user@youtube.com",
		"localhost",
	} {
		t.Run(entry, func(t *testing.T) {
			h, repo := newStepUpSwitchTestHandler(t, map[string]string{
				service.SettingKeyCustomPageIframeHosts: `["youtube.com"]`,
			})

			rec := doUpdateSettings(t, h, map[string]any{
				"custom_page_iframe_hosts": []string{"embed.example.com", entry},
			}, nil)

			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), entry,
				"the error must name the offending entry")
			require.Equal(t, `["youtube.com"]`, repo.values[service.SettingKeyCustomPageIframeHosts],
				"a rejected payload must not be persisted")
		})
	}
}

// 条目数量上限：避免 CSP 头被撑爆（每条主机展开成 2 个 frame-src 值）。
func TestUpdateSettingsCustomPageIframeHostsRejectsTooManyEntries(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	entries := make([]string, service.MaxCustomPageIframeHosts+1)
	for i := range entries {
		entries[i] = "host.example.com"
	}

	rec := doUpdateSettings(t, h, map[string]any{"custom_page_iframe_hosts": entries}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, repo.values[service.SettingKeyCustomPageIframeHosts])
}
