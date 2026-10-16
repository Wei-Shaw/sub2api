//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func projectionCaps(t *testing.T, manifest string) CodexAccountCapabilities {
	t.Helper()
	caps, ok := DeriveCodexAccountCapabilities([]byte(manifest))
	require.True(t, ok)
	return caps
}

func projectionSource(t *testing.T, manifest string, mapping map[string]string) CodexCapabilitySource {
	t.Helper()
	return CodexCapabilitySource{
		Caps: projectionCaps(t, manifest),
		UpstreamSlugOf: func(public string) string {
			if mapped, ok := mapping[public]; ok {
				return mapped
			}
			return public
		},
	}
}

func tierIDs(t *testing.T, body []byte, slug string) []string {
	t.Helper()
	for _, model := range gjson.GetBytes(body, "models").Array() {
		if model.Get("slug").String() != slug {
			continue
		}
		ids := make([]string, 0)
		for _, tier := range model.Get("service_tiers").Array() {
			ids = append(ids, tier.Get("id").String())
		}
		return ids
	}
	t.Fatalf("model %q not found in %s", slug, body)
	return nil
}

func cyberPrograms(t *testing.T, body []byte, slug string) []string {
	t.Helper()
	for _, model := range gjson.GetBytes(body, "models").Array() {
		if model.Get("slug").String() != slug {
			continue
		}
		out := make([]string, 0)
		for _, program := range model.Get("available_access_programs.cyber").Array() {
			out = append(out, program.String())
		}
		return out
	}
	t.Fatalf("model %q not found in %s", slug, body)
	return nil
}

// 由账号 B（无项目）拉取的 manifest：gpt-6-sol 只带 standard / priority。
const projectionStandardBody = `{"models":[
  {"slug":"gpt-6-sol","display_name":"GPT-6 Sol","service_tiers":[{"id":"priority","name":"Fast","description":"Priority processing for lower latency."}],"available_access_programs":{"cyber":["standard"],"other":true},"keep":1},
  {"slug":"gpt-6.1-sol","service_tiers":[{"id":"priority","name":"Fast","description":"Priority processing for lower latency."}],"available_access_programs":{"cyber":["standard"]}},
  {"slug":"gpt-6-luna","service_tiers":[{"id":"priority","name":"Fast"}],"available_access_programs":{"cyber":["standard"]}}
],"metadata":{"version":1}}`

func TestProjectCodexManifestCapabilities_UnionReplacesAndIsolatesBySlug(t *testing.T) {
	blue := projectionSource(t, codexCapabilitiesBlueAccountManifest, nil)
	standard := projectionSource(t, codexCapabilitiesStandardAccountManifest, nil)

	out, changed, err := ProjectCodexManifestCapabilities([]byte(projectionStandardBody), []CodexCapabilitySource{standard, blue}, GroupCapabilityAllowlist{})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"standard", "daybreak_blue"}, cyberPrograms(t, out, "gpt-6-sol"), "混合账号分组：cyber 列表为并集")
	require.Equal(t, []string{"priority"}, tierIDs(t, out, "gpt-6-sol"))
	require.Equal(t, []string{"priority", "ultrafast"}, tierIDs(t, out, "gpt-6.1-sol"), "单一 slug 的 Ultrafast")
	require.Equal(t, []string{"standard"}, cyberPrograms(t, out, "gpt-6.1-sol"), "ultrafast 不扩散到 Daybreak，Blue 不扩散到 gpt-6.1-sol")
	require.Equal(t, []string{"priority"}, tierIDs(t, out, "gpt-6-luna"), "所有记录都不含的 slug 保留原值")
	require.Equal(t, []string{"standard"}, cyberPrograms(t, out, "gpt-6-luna"))
	// 条目名称描述沿用 body 原有条目；body 没有的 ultrafast 用内置描述回填。
	require.Equal(t, "Fast", gjson.GetBytes(out, "models.1.service_tiers.0.name").String())
	require.Equal(t, "Ultrafast", gjson.GetBytes(out, "models.1.service_tiers.1.name").String())
	require.NotEmpty(t, gjson.GetBytes(out, "models.1.service_tiers.1.description").String())
	// 其他字段与 available_access_programs 内其他键保留。
	require.Equal(t, int64(1), gjson.GetBytes(out, "models.0.keep").Int())
	require.True(t, gjson.GetBytes(out, "models.0.available_access_programs.other").Bool())
	require.Equal(t, int64(1), gjson.GetBytes(out, "metadata.version").Int())
}

func TestProjectCodexManifestCapabilities_DoesNotKeepBodyValuesWhenRecordHit(t *testing.T) {
	// 缓存体仍带 ultrafast，记录已撤销 → 不复活。
	stale := `{"models":[{"slug":"gpt-6.1-sol","service_tiers":[{"id":"priority","name":"Fast"},{"id":"ultrafast","name":"Ultrafast"}],"available_access_programs":{"cyber":["standard","daybreak_blue"]}}]}`
	standard := projectionSource(t, codexCapabilitiesStandardAccountManifest, nil)
	out, changed, err := ProjectCodexManifestCapabilities([]byte(stale), []CodexCapabilitySource{standard}, GroupCapabilityAllowlist{})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"priority"}, tierIDs(t, out, "gpt-6.1-sol"))
	require.Equal(t, []string{"standard"}, cyberPrograms(t, out, "gpt-6.1-sol"))
}

func TestProjectCodexManifestCapabilities_LocalTemplateUltrafastOverriddenByRecord(t *testing.T) {
	// 本地模板为 gpt-5.6-sol 硬编码 ultrafast；记录的该 slug 不含 ultrafast。
	template := `{"models":[{"slug":"gpt-5.6-sol","service_tiers":[{"id":"priority","name":"Fast"},{"id":"ultrafast","name":"Ultrafast"}],"available_access_programs":{"cyber":["standard"]}}]}`
	record := projectionSource(t, `{"models":[{"slug":"gpt-5.6-sol","service_tiers":[{"id":"priority"}],"available_access_programs":{"cyber":["standard"]}}]}`, nil)
	out, changed, err := ProjectCodexManifestCapabilities([]byte(template), []CodexCapabilitySource{record}, GroupCapabilityAllowlist{})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"priority"}, tierIDs(t, out, "gpt-5.6-sol"))
}

func TestProjectCodexManifestCapabilities_PublicAliasResolvedPerSource(t *testing.T) {
	body := `{"models":[{"slug":"my-codex","service_tiers":[],"available_access_programs":{"cyber":["standard"]}}]}`
	// A: my-codex → gpt-6-sol（记录带 daybreak_blue）；A 在 gpt-6-luna 上也有记录但不应参与。
	a := projectionSource(t, `{"models":[
	  {"slug":"gpt-6-sol","service_tiers":[{"id":"priority"}],"available_access_programs":{"cyber":["standard","daybreak_blue"]}},
	  {"slug":"gpt-6-luna","service_tiers":[{"id":"ultrafast"}],"available_access_programs":{"cyber":["standard","daybreak_red"]}}
	]}`, map[string]string{"my-codex": "gpt-6-sol"})
	// B: my-codex → gpt-6-luna（无 Daybreak）；B 在 gpt-6-sol 上有 Red 记录但不应参与。
	b := projectionSource(t, `{"models":[
	  {"slug":"gpt-6-luna","service_tiers":[{"id":"priority"}],"available_access_programs":{"cyber":["standard"]}},
	  {"slug":"gpt-6-sol","service_tiers":[{"id":"priority"}],"available_access_programs":{"cyber":["standard","daybreak_red"]}}
	]}`, map[string]string{"my-codex": "gpt-6-luna"})
	out, changed, err := ProjectCodexManifestCapabilities([]byte(body), []CodexCapabilitySource{a, b}, GroupCapabilityAllowlist{})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"standard", "daybreak_blue"}, cyberPrograms(t, out, "my-codex"))
	require.Equal(t, []string{"priority"}, tierIDs(t, out, "my-codex"))
}

func TestProjectCodexManifestCapabilities_AllowlistPrunesWithoutRecords(t *testing.T) {
	body := `{"models":[
	  {"slug":"gpt-6-sol","service_tiers":[{"id":"priority","name":"Fast"},{"id":"ultrafast","name":"Ultrafast"},{"id":"flex","name":"Flex"}],"available_access_programs":{"cyber":["standard","daybreak_blue","daybreak_red"]}}
	]}`
	t.Run("disable ultrafast and all daybreak", func(t *testing.T) {
		out, changed, err := ProjectCodexManifestCapabilities([]byte(body), nil, GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority"}})
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, []string{"priority", "flex"}, tierIDs(t, out, "gpt-6-sol"), "只裁可配置的 tier，flex 保留")
		require.Equal(t, []string{"standard"}, cyberPrograms(t, out, "gpt-6-sol"), "standard 保留")
	})
	t.Run("disabled allowlist changes nothing", func(t *testing.T) {
		out, changed, err := ProjectCodexManifestCapabilities([]byte(body), nil, GroupCapabilityAllowlist{})
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, string(out))
	})
	t.Run("enabled allowlist that permits everything changes nothing", func(t *testing.T) {
		_, changed, err := ProjectCodexManifestCapabilities([]byte(body), nil, GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_blue", "daybreak_red"}, ServiceTiers: []string{"priority", "ultrafast"}})
		require.NoError(t, err)
		require.False(t, changed)
	})
	t.Run("prune after union", func(t *testing.T) {
		blue := projectionSource(t, codexCapabilitiesBlueAccountManifest, nil)
		out, changed, err := ProjectCodexManifestCapabilities([]byte(projectionStandardBody), []CodexCapabilitySource{blue}, GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority"}})
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, []string{"standard"}, cyberPrograms(t, out, "gpt-6-sol"))
		require.Equal(t, []string{"priority"}, tierIDs(t, out, "gpt-6.1-sol"))
	})
}

func TestProjectCodexManifestCapabilities_NoSourcesNoRecordsKeepsBody(t *testing.T) {
	out, changed, err := ProjectCodexManifestCapabilities([]byte(projectionStandardBody), []CodexCapabilitySource{{Caps: CodexAccountCapabilities{}}}, GroupCapabilityAllowlist{})
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, projectionStandardBody, string(out))
	_, _, err = ProjectCodexManifestCapabilities([]byte(`{"models":{}}`), nil, GroupCapabilityAllowlist{Enabled: true})
	require.Error(t, err)
}

// FinalizeCodexModelsManifest：来源来自分组内可调度 OAuth-like 账号的记录、ETag 随投影后的 body、
// Composite 分组只裁剪。
func TestFinalizeCodexModelsManifest(t *testing.T) {
	blueCaps := projectionCaps(t, codexCapabilitiesBlueAccountManifest)
	blueOnlyCaps := projectionCaps(t, capabilityRoutingBlueOnlyManifest)
	standardCaps := projectionCaps(t, codexCapabilitiesStandardAccountManifest)
	group := &Group{ID: 91, Platform: PlatformOpenAI}
	svc := &OpenAIGatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{
		91: {
			{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{CodexCapabilitiesExtraKey: standardCaps}},
			{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{CodexCapabilitiesExtraKey: blueOnlyCaps}},
			{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{CodexCapabilitiesExtraKey: blueCaps}},
		},
	}}}

	manifest := &OpenAIModelsResponse{Body: []byte(projectionStandardBody), ETag: `W/"upstream"`}
	require.NoError(t, svc.FinalizeCodexModelsManifest(context.Background(), group, manifest, ""))
	require.False(t, manifest.NotModified)
	require.Equal(t, []string{"standard", "daybreak_blue"}, cyberPrograms(t, manifest.Body, "gpt-6-sol"))
	require.Equal(t, []string{"priority"}, tierIDs(t, manifest.Body, "gpt-6.1-sol"), "API Key 账号的记录不参与并集")
	require.Equal(t, codexModelsManifestBodyETag(manifest.Body), manifest.ETag, "ETag 基于投影后的 body")
	projectedETag := manifest.ETag

	// 携带投影后 ETag → 304；携带旧上游 ETag → 200。
	second := &OpenAIModelsResponse{Body: []byte(projectionStandardBody), ETag: `W/"upstream"`}
	require.NoError(t, svc.FinalizeCodexModelsManifest(context.Background(), group, second, projectedETag))
	require.True(t, second.NotModified)
	require.Empty(t, second.Body)
	third := &OpenAIModelsResponse{Body: []byte(projectionStandardBody), ETag: `W/"upstream"`}
	require.NoError(t, svc.FinalizeCodexModelsManifest(context.Background(), group, third, `W/"upstream"`))
	require.False(t, third.NotModified)

	// 能力变化导致 ETag 变化：账号 2 新获得 Ultrafast（gpt-6.1-sol）。
	svc.accountRepo = codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{91: {
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{CodexCapabilitiesExtraKey: standardCaps}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{CodexCapabilitiesExtraKey: blueCaps}},
	}}}
	fourth := &OpenAIModelsResponse{Body: []byte(projectionStandardBody), ETag: `W/"upstream"`}
	require.NoError(t, svc.FinalizeCodexModelsManifest(context.Background(), group, fourth, projectedETag))
	require.False(t, fourth.NotModified, "能力变化后旧 ETag 不得命中 304")
	require.Equal(t, []string{"priority", "ultrafast"}, tierIDs(t, fourth.Body, "gpt-6.1-sol"))
	require.NotEqual(t, projectedETag, fourth.ETag)

	// Composite 分组：只裁剪不并集。
	composite := &Group{ID: 92, Platform: PlatformComposite, CapabilityAllowlist: GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"ultrafast"}}}
	fifth := &OpenAIModelsResponse{Body: []byte(projectionStandardBody)}
	require.NoError(t, svc.FinalizeCodexModelsManifest(context.Background(), composite, fifth, ""))
	require.Equal(t, []string{}, tierIDs(t, fifth.Body, "gpt-6-sol"))
	require.Equal(t, []string{"standard"}, cyberPrograms(t, fifth.Body, "gpt-6-sol"))
	require.NotEmpty(t, fifth.ETag)

	// NotModified / 空 body 直接返回。
	require.NoError(t, svc.FinalizeCodexModelsManifest(context.Background(), group, &OpenAIModelsResponse{NotModified: true}, ""))
	require.NoError(t, svc.FinalizeCodexModelsManifest(context.Background(), group, nil, ""))
}
