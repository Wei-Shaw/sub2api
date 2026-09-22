package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAICodexTicketTargetLength(t *testing.T) {
	rules := []OpenAICodexTicketPlanLengthRule{
		{Plan: "business", Length: 332},
		{Plan: "team", Length: 332},
	}
	require.Equal(t, 292, resolveOpenAICodexTicketTargetLength("", rules, 292), "空 plan_type 回退默认")
	require.Equal(t, 332, resolveOpenAICodexTicketTargetLength("team", rules, 292))
	require.Equal(t, 332, resolveOpenAICodexTicketTargetLength("TEAM", rules, 292), "大小写不敏感")
	require.Equal(t, 332, resolveOpenAICodexTicketTargetLength("self_serve_business_usage_based", rules, 292), "子串兼容 workspace 计费名")
	require.Equal(t, 292, resolveOpenAICodexTicketTargetLength("plus", rules, 292), "未命中回退默认")
	require.Equal(t, 300, resolveOpenAICodexTicketTargetLength("pro", rules, 300), "回退到自定义默认长度")
	require.Equal(t, 292, resolveOpenAICodexTicketTargetLength("team", nil, 292), "无规则回退默认")
	// 越界长度的规则视为无效，跳过后继续匹配。
	invalid := append(rules, OpenAICodexTicketPlanLengthRule{Plan: "team", Length: 9999})
	require.Equal(t, 332, resolveOpenAICodexTicketTargetLength("team", invalid, 292))
	require.Equal(t, 292, resolveOpenAICodexTicketTargetLength("whatever", rules, 0), "默认长度非正数时兜底 292")
	// 先命中先生效：business 规则排在 team 之前。
	ordered := []OpenAICodexTicketPlanLengthRule{
		{Plan: "team", Length: 316},
		{Plan: "business", Length: 332},
	}
	require.Equal(t, 316, resolveOpenAICodexTicketTargetLength("team", ordered, 292))
}

func TestParseOpenAICodexTicketPlanLengthRules(t *testing.T) {
	require.Nil(t, parseOpenAICodexTicketPlanLengthRules(""))
	require.Nil(t, parseOpenAICodexTicketPlanLengthRules("null"))
	require.Nil(t, parseOpenAICodexTicketPlanLengthRules("{bad json"))
	rules := parseOpenAICodexTicketPlanLengthRules(`[{"plan":" Team ","length":332},{"plan":"","length":292},{"plan":"pro","length":99},{"plan":"plus","length":292}]`)
	require.Equal(t, []OpenAICodexTicketPlanLengthRule{{Plan: "team", Length: 332}, {Plan: "plus", Length: 292}}, rules)
}

func TestValidateOpenAICodexTicketTargetLengthSettings(t *testing.T) {
	require.NoError(t, validateOpenAICodexTicketTargetLengthSettings(0, nil), "0 表示未提交，不校验")
	require.NoError(t, validateOpenAICodexTicketTargetLengthSettings(292, []OpenAICodexTicketPlanLengthRule{{Plan: "team", Length: 332}}))
	require.Error(t, validateOpenAICodexTicketTargetLengthSettings(99, nil))
	require.Error(t, validateOpenAICodexTicketTargetLengthSettings(513, nil))
	require.Error(t, validateOpenAICodexTicketTargetLengthSettings(292, []OpenAICodexTicketPlanLengthRule{{Plan: "team", Length: 99}}))
	require.Error(t, validateOpenAICodexTicketTargetLengthSettings(292, []OpenAICodexTicketPlanLengthRule{{Plan: " ", Length: 332}}))
	require.Error(t, validateOpenAICodexTicketTargetLengthSettings(292, []OpenAICodexTicketPlanLengthRule{
		{Plan: "team", Length: 332}, {Plan: "TEAM", Length: 316},
	}), "小写去重")
	many := make([]OpenAICodexTicketPlanLengthRule, 0, openAICodexTicketMaxPlanRules+1)
	for i := 0; i <= openAICodexTicketMaxPlanRules; i++ {
		many = append(many, OpenAICodexTicketPlanLengthRule{Plan: strings.Repeat("p", i+1), Length: 332})
	}
	require.Error(t, validateOpenAICodexTicketTargetLengthSettings(292, many))
}

func TestOpenAICodexTicketTargetLengthRuntimeSettingOverridesYaml(t *testing.T) {
	repo := &codexTicketSettingRepo{codexPolicyMigrationRepoStub: &codexPolicyMigrationRepoStub{values: map[string]string{
		SettingKeyOpenAICodexTicketDefaultLength: "300",
		SettingKeyOpenAICodexTicketPlanLengths:   `[{"plan":"business","length":332},{"plan":"team","length":332}]`,
	}}}
	settings := NewSettingService(repo, &config.Config{})
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{TargetLength: 292}, nil)
	svc.settingService = settings

	team := ticketTestAccount(41)
	team.Credentials["plan_type"] = "team"
	plus := ticketTestAccount(42)
	plus.Credentials["plan_type"] = "plus"
	noPlan := ticketTestAccount(43)

	require.Equal(t, 332, svc.openAICodexTicketTargetLength(team))
	require.Equal(t, 300, svc.openAICodexTicketTargetLength(plus), "未命中规则用后台默认长度")
	require.Equal(t, 300, svc.openAICodexTicketTargetLength(noPlan), "无 plan_type 用后台默认长度")

	// 后台键缺失 → 回退 yaml target_length。
	emptyRepo := &codexTicketSettingRepo{codexPolicyMigrationRepoStub: &codexPolicyMigrationRepoStub{values: map[string]string{}}}
	svc.settingService = NewSettingService(emptyRepo, &config.Config{})
	require.Equal(t, 292, svc.openAICodexTicketTargetLength(team))

	// 无 settingService → 直接 yaml 兜底。
	svc.settingService = nil
	require.Equal(t, 292, svc.openAICodexTicketTargetLength(team))

	// 热更新：改库 + 失效后立即生效。
	svc.settingService = settings
	repo.values[SettingKeyOpenAICodexTicketDefaultLength] = "316"
	settings.InvalidateOpenAICodexTicketTargetLengthCache()
	require.Equal(t, 316, svc.openAICodexTicketTargetLength(plus))
}

func TestParseSettingsOpenAICodexTicketTargetLengthFallback(t *testing.T) {
	// parseSettings 会发布进程级 Grok 映射选项（xai 全局状态），测完恢复，
	// 避免影响后续依赖默认映射状态的测试。
	origXAIOptions := xai.RuntimeModelMappingOptions()
	t.Cleanup(func() { xai.SetRuntimeModelMappingOptions(origXAIOptions) })

	cfg := &config.Config{}
	cfg.Gateway.OpenAICodexTicket.TargetLength = 296
	svc := NewSettingService(&codexTicketSettingRepo{codexPolicyMigrationRepoStub: &codexPolicyMigrationRepoStub{values: map[string]string{}}}, cfg)
	view := svc.parseSettings(map[string]string{})
	require.Equal(t, 296, view.OpenAICodexTicketDefaultLength, "DB 缺失回退 yaml target_length")
	require.Nil(t, view.OpenAICodexTicketPlanLengthRules)

	repo := &codexTicketSettingRepo{codexPolicyMigrationRepoStub: &codexPolicyMigrationRepoStub{values: map[string]string{
		SettingKeyOpenAICodexTicketDefaultLength: "332",
		SettingKeyOpenAICodexTicketPlanLengths:   `[{"plan":"team","length":316}]`,
	}}}
	view = NewSettingService(repo, cfg).parseSettings(map[string]string{
		SettingKeyOpenAICodexTicketDefaultLength: "332",
		SettingKeyOpenAICodexTicketPlanLengths:   `[{"plan":"team","length":316}]`,
	})
	require.Equal(t, 332, view.OpenAICodexTicketDefaultLength)
	require.Equal(t, []OpenAICodexTicketPlanLengthRule{{Plan: "team", Length: 316}}, view.OpenAICodexTicketPlanLengthRules)
}

func TestProbeOpenAICodexTicketHarvestsPerAccountPlanLength(t *testing.T) {
	upstream := &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		h := http.Header{}
		h.Set(openAICodexTurnStateHeader, fakeCodexTicketState(332))
		return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader("data: {}\n\n"))}, nil
	}}
	settings := NewSettingService(&codexTicketSettingRepo{codexPolicyMigrationRepoStub: &codexPolicyMigrationRepoStub{values: map[string]string{
		SettingKeyOpenAICodexTicketEnabled:     "true",
		SettingKeyOpenAICodexTicketPlanLengths: `[{"plan":"team","length":332}]`,
	}}}, &config.Config{})
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{
		Enabled:         true,
		TargetLength:    292,
		FailClosed:      true,
		HarvestProxyURL: "socks5h://proxy.example.com:1080",
		Models:          []string{"gpt-6-astra"},
	}, upstream)
	svc.settingService = settings

	team := ticketTestAccount(41)
	team.Credentials["plan_type"] = "team"
	svc.probeOnceOpenAICodexTicket(context.Background(), team, "gpt-6-astra")

	ticket := svc.lookupOpenAICodexTicket(team, "gpt-6-astra")
	require.NotNil(t, ticket, "332 门票按账号档位命中入库")
	require.Equal(t, 332, ticket.Length)
	require.Len(t, ticket.State, 332)

	h := http.Header{}
	require.NoError(t, svc.applyOpenAICodexTicket(context.Background(), team, "gpt-6-astra", h))
	require.Equal(t, ticket.State, h.Get(openAICodexTurnStateHeader))

	// 状态摘要读的是持久化形态（account.Extra），模拟打票落库后的回读。
	team.Extra = map[string]any{openAICodexTicketExtraKey("gpt-6-astra"): ticket}
	statuses := OpenAICodexTicketStatuses(team, config.OpenAICodexTicketConfig{Enabled: true, Models: []string{"gpt-6-astra"}, FailClosed: true}, svc.openAICodexTicketTargetLength(team), time.Now())
	require.Len(t, statuses, 1)
	require.True(t, statuses[0].Ready)
	require.Equal(t, 332, statuses[0].Length)

	// 同一张 332 票对无档位账号（目标 292）无效：fail-closed 下拦截。
	plus := ticketTestAccount(42)
	plus.Credentials["plan_type"] = "plus"
	require.True(t, svc.openAICodexTicketBlocksAccount(plus, "gpt-6-astra"), "plus 账号目标长度 292，332 票不通用")
}
