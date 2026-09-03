//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// custom_page_iframe_hosts 是自定义页面 iframe 策略的唯一事实来源：
// 既下发给前端净化器，又注入 CSP frame-src。这组用例锁定它的三态语义
// （未配置 / 显式为空 / 已配置）以及两条下游必须同源。

func TestParseCustomPageIframeHosts_NotConfigured(t *testing.T) {
	for _, raw := range []string{"", "   ", "\n\t "} {
		hosts, configured := ParseCustomPageIframeHosts(raw)
		require.False(t, configured, "raw=%q 应视为从未配置", raw)
		require.Empty(t, hosts)
	}
}

func TestParseCustomPageIframeHosts_ExplicitEmptyMeansLockdown(t *testing.T) {
	// "[]" 是运维明确要求「一个 iframe 都不许嵌」，绝不能回落到默认列表——
	// 否则后台看起来是锁死的，实际仍放行 youtube 等默认站点。
	hosts, configured := ParseCustomPageIframeHosts("[]")
	require.True(t, configured)
	require.Empty(t, hosts)

	require.Empty(t, EffectiveCustomPageIframeHosts("[]"))
}

func TestParseCustomPageIframeHosts_NormalizesAndDeduplicates(t *testing.T) {
	hosts, configured := ParseCustomPageIframeHosts(
		`[" .Embed.Example.COM. ", "embed.example.com", "a.test", ""]`,
	)
	require.True(t, configured)
	require.Equal(t, []string{"embed.example.com", "a.test"}, hosts)
}

func TestParseCustomPageIframeHosts_RejectsNonHostEntries(t *testing.T) {
	// 只接受主机名：协议、路径、端口、通配符、裸主机、非法字符一律丢弃。
	hosts, configured := ParseCustomPageIframeHosts(
		`["https://a.com","a.com/embed","a.com:8443","*.a.com","localhost","user@a.com","a_b.com","a.com"]`,
	)
	require.True(t, configured)
	require.Equal(t, []string{"a.com"}, hosts)
}

func TestParseCustomPageIframeHosts_AllEntriesInvalidFailsClosed(t *testing.T) {
	// 配错了应当 fail closed（禁止全部），而不是悄悄回到默认白名单。
	hosts, configured := ParseCustomPageIframeHosts(`["https://a.com","*.b.com"]`)
	require.True(t, configured)
	require.Empty(t, hosts)

	broken, configured := ParseCustomPageIframeHosts(`{"not":"an array"`)
	require.True(t, configured)
	require.Empty(t, broken)
}

func TestParseCustomPageIframeHosts_AcceptsHandWrittenLists(t *testing.T) {
	// 运维直接在 psql 里写非 JSON 的值也要能用。
	hosts, configured := ParseCustomPageIframeHosts("youtube.com, player.vimeo.com\nbilibili.com")
	require.True(t, configured)
	require.Equal(t, []string{"youtube.com", "player.vimeo.com", "bilibili.com"}, hosts)
}

func TestEffectiveCustomPageIframeHosts_FallsBackToDefaults(t *testing.T) {
	require.Equal(t, DefaultCustomPageIframeHosts, EffectiveCustomPageIframeHosts(""))
	// 返回值必须是副本，调用方改动不能污染全局默认值
	got := EffectiveCustomPageIframeHosts("")
	got[0] = "evil.com"
	require.Equal(t, "youtube.com", DefaultCustomPageIframeHosts[0])
}

func TestSettingService_CustomPageIframeHosts(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{}, &config.Config{})
	require.Equal(t, DefaultCustomPageIframeHosts, svc.CustomPageIframeHosts(context.Background()))

	svc = NewSettingService(&settingPublicRepoStub{
		values: map[string]string{SettingKeyCustomPageIframeHosts: `["embed.example.com"]`},
	}, &config.Config{})
	require.Equal(t, []string{"embed.example.com"}, svc.CustomPageIframeHosts(context.Background()))

	svc = NewSettingService(&settingPublicRepoStub{
		values: map[string]string{SettingKeyCustomPageIframeHosts: `[]`},
	}, &config.Config{})
	require.Empty(t, svc.CustomPageIframeHosts(context.Background()))
}

func TestSettingService_CustomPageIframeHosts_ReadErrorKeepsDefaults(t *testing.T) {
	// 读设置失败是基础设施故障，不是「运维要求锁死」——静默掐掉全部已有嵌入更糟。
	svc := NewSettingService(&settingPublicRepoStub{err: errors.New("boom")}, &config.Config{})
	require.Equal(t, DefaultCustomPageIframeHosts, svc.CustomPageIframeHosts(context.Background()))
}

func TestGetFrameSrcOrigins_IncludesCustomPageIframeHosts(t *testing.T) {
	// 关键回归：前端净化器放行的主机，CSP frame-src 也必须放行，否则嵌入永远加载不出来。
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{
			SettingKeyCustomPageIframeHosts: `["embed.example.com"]`,
		},
	}, &config.Config{})

	origins, err := svc.GetFrameSrcOrigins(context.Background())
	require.NoError(t, err)
	// 精确匹配 + 点边界子域，与 isAllowedIframeSrc 的匹配规则一一对应
	require.Contains(t, origins, "https://embed.example.com")
	require.Contains(t, origins, "https://*.embed.example.com")
	require.NotContains(t, origins, "https://youtube.com")
}

func TestGetFrameSrcOrigins_DefaultsWhenUnconfigured(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{}, &config.Config{})

	origins, err := svc.GetFrameSrcOrigins(context.Background())
	require.NoError(t, err)
	for _, host := range DefaultCustomPageIframeHosts {
		require.Contains(t, origins, "https://"+host)
		require.Contains(t, origins, "https://*."+host)
	}
}

func TestGetFrameSrcOrigins_LockdownEmitsNoIframeOrigins(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{SettingKeyCustomPageIframeHosts: `[]`},
	}, &config.Config{})

	origins, err := svc.GetFrameSrcOrigins(context.Background())
	require.NoError(t, err)
	require.NotContains(t, origins, "https://youtube.com")
	require.NotContains(t, origins, "https://*.youtube.com")
}

func TestGetFrameSrcOrigins_DeduplicatesAgainstMenuItemOrigins(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{
			SettingKeyCustomMenuItems:       `[{"url":"https://embed.example.com/a"}]`,
			SettingKeyCustomPageIframeHosts: `["embed.example.com"]`,
		},
	}, &config.Config{})

	origins, err := svc.GetFrameSrcOrigins(context.Background())
	require.NoError(t, err)
	count := 0
	for _, origin := range origins {
		if origin == "https://embed.example.com" {
			count++
		}
	}
	require.Equal(t, 1, count, "同一个 origin 不应重复注入 frame-src")
}

func TestGetPublicSettingsForInjection_CarriesIframeHosts(t *testing.T) {
	// __APP_CONFIG__ 注入路径要带上白名单，否则首屏渲染的自定义页面会退回默认列表。
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{SettingKeyCustomPageIframeHosts: `["embed.example.com"]`},
	}, &config.Config{})

	payload, err := svc.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	injected, ok := payload.(*PublicSettingsInjectionPayload)
	require.True(t, ok)
	require.Equal(t, []string{"embed.example.com"}, injected.CustomPageIframeHosts)
}
