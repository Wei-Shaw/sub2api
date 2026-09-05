package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 探测资格：/v1/sub2api/billing 是 key 级端点，全部
// 受支持平台（含国产供应商）的 API-key 账号都可开启探测；OAuth/Bedrock 无静态 Key 仍不合格。
func TestUpstreamBillingProbeIdentityCoversAllAPIKeyPlatforms(t *testing.T) {
	for _, platform := range []string{
		PlatformOpenAI, PlatformGrok, PlatformAnthropic, PlatformGemini, PlatformAntigravity,
		PlatformKimi, PlatformZhipu, PlatformDeepseek,
	} {
		require.True(t, IsUpstreamBillingProbeIdentity(platform, AccountTypeAPIKey), platform)
		require.True(t, isUpstreamBillingProbeAccount(&Account{Platform: platform, Type: AccountTypeAPIKey}), platform)
	}
	require.False(t, IsUpstreamBillingProbeIdentity(PlatformOpenAI, AccountTypeOAuth))
	require.False(t, IsUpstreamBillingProbeIdentity(PlatformGrok, AccountTypeOAuth))
	require.False(t, IsUpstreamBillingProbeIdentity(PlatformAnthropic, AccountTypeBedrock))
	require.False(t, IsUpstreamBillingProbeIdentity("", AccountTypeAPIKey))
	require.False(t, IsUpstreamBillingProbeIdentity("future-platform", AccountTypeAPIKey))
	require.False(t, isUpstreamBillingProbeAccount(nil))
}

func TestBuildUpstreamBillingRateSnapshotItemsPreservesAllAPIKeyPlatforms(t *testing.T) {
	makeAccount := func(id int64, platform, accountType string, snapshot any) Account {
		return Account{
			ID:       id,
			Platform: platform,
			Type:     accountType,
			Extra:    map[string]any{UpstreamBillingProbeExtraKey: snapshot},
		}
	}

	accounts := []Account{
		makeAccount(1, PlatformOpenAI, AccountTypeAPIKey, map[string]any{
			"status": "ok",
			"data":   map[string]any{"effective_rate_multiplier": 0.045},
		}),
		makeAccount(2, PlatformAnthropic, AccountTypeAPIKey, map[string]any{
			"status": "ok",
			"data":   map[string]any{"effective_rate_multiplier": 0.047},
		}),
		makeAccount(3, PlatformGemini, AccountTypeAPIKey, map[string]any{
			"status": "failed",
		}),
		makeAccount(4, PlatformOpenAI, AccountTypeOAuth, map[string]any{
			"status": "ok",
		}),
	}

	items := BuildUpstreamBillingRateSnapshotItems(accounts)
	require.Len(t, items, len(accounts))
	require.Equal(t, int64(1), items[0].AccountID)
	require.NotNil(t, items[0].Snapshot)
	require.NotNil(t, items[1].Snapshot)
	require.NotNil(t, items[2].Snapshot)
	require.Equal(t, 0.045, items[0].Snapshot.Data["effective_rate_multiplier"])
	require.Equal(t, 0.047, items[1].Snapshot.Data["effective_rate_multiplier"])
	require.Equal(t, UpstreamBillingProbeStatusFailed, items[2].Snapshot.Status)
	require.Nil(t, items[3].Snapshot)
}

func upstreamBillingProbeValidBody() io.ReadCloser {
	return io.NopCloser(strings.NewReader(`{
		"object":"sub2api.key_billing",
		"schema_version":1,
		"billing_scope":"token",
		"group_rate_multiplier":0.02,
		"resolved_rate_multiplier":0.02,
		"peak_rate_enabled":false,
		"effective_rate_multiplier":0.02,
		"observed_at":"2026-07-26T01:00:00Z"
	}`))
}

func TestUpstreamBillingProbeGrokAccountPersistsSnapshot(t *testing.T) {
	account := &Account{
		ID:          151,
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 2,
		Credentials: map[string]any{
			"api_key":  "sk-grok-relay",
			"base_url": "https://relay.example/v1",
		},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       upstreamBillingProbeValidBody(),
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	fixedNow := time.Date(2026, time.July, 26, 2, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.Equal(t, 0.02, snapshot.Data["resolved_rate_multiplier"])
	require.Equal(t, "https://relay.example/v1/sub2api/billing", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-grok-relay", upstream.lastReq.Header.Get("Authorization"))
	// 非 OpenAI 平台探测使用默认传输画像。
	require.Equal(t, HTTPUpstreamProfileDefault, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))

	persisted := decodeUpstreamBillingProbeSnapshot(account.Extra)
	require.NotNil(t, persisted)
	require.Equal(t, UpstreamBillingProbeStatusOK, persisted.Status)
}

// 非 OpenAI 平台没有自定义 base_url 时，上游是各自官方 API，必无
// /v1/sub2api/billing：不发请求，直接落 unsupported。
func TestUpstreamBillingProbeNonOpenAIWithoutBaseURLIsUnsupportedWithoutRequest(t *testing.T) {
	account := &Account{
		ID:          152,
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Credentials: map[string]any{"api_key": "sk-grok-official"},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusUnsupported, snapshot.Status)
	require.Nil(t, upstream.lastReq)
}

// 前端创建 API-key 账号时会把空 base_url 填成平台官方默认域；官方 API 同样
// 必无 /v1/sub2api/billing，不发请求直接 unsupported。
// 显式端口、DNS 尾点、官方区域子域（前端 Grok 预设含 us-east-1.api.x.ai 等）
// 与跨平台官方域都不能绕过。
func TestUpstreamBillingProbeOfficialAPIBaseURLIsUnsupportedWithoutRequest(t *testing.T) {
	cases := []struct {
		platform string
		baseURL  string
	}{
		{PlatformAnthropic, "https://api.anthropic.com"},
		{PlatformAnthropic, "https://api.anthropic.com:443"},
		{PlatformAnthropic, "https://api.anthropic.com./"},
		{PlatformAnthropic, "HTTPS://API.ANTHROPIC.COM/"},
		{PlatformGemini, "https://generativelanguage.googleapis.com"},
		{PlatformAntigravity, "https://cloudcode-pa.googleapis.com"},
		{PlatformGrok, "https://api.x.ai/v1"},
		{PlatformGrok, "https://us-east-1.api.x.ai/v1"},
		{PlatformGrok, "https://eu-west-1.api.x.ai/v1"},
		{PlatformGrok, "https://cli-chat-proxy.grok.com/v1"},
		// 跨平台官方域同样必无该端点，一并拦截。
		{PlatformAnthropic, "https://api.x.ai/v1"},
		{PlatformGrok, "https://api.openai.com"},
		// Ollama Cloud 是本仓一等支持配置（platform openai/anthropic +
		// base_url https://ollama.com/v1），同为官方 API，不能拿 Key 去空探。
		{PlatformAnthropic, "https://ollama.com/v1"},
		{PlatformAnthropic, "https://ollama.com"},
		{PlatformAnthropic, "https://www.ollama.com/v1"},
		// 国产供应商官方域（含各协议端点）同样是官方 API，创建即开探测也不发请求。
		{PlatformKimi, "https://api.moonshot.cn/v1"},
		{PlatformKimi, "https://api.moonshot.cn/anthropic"},
		{PlatformKimi, "https://api.kimi.com/coding"},
		{PlatformZhipu, "https://open.bigmodel.cn/api/paas/v4"},
		{PlatformZhipu, "https://open.bigmodel.cn/api/anthropic"},
		{PlatformDeepseek, "https://api.deepseek.com"},
		{PlatformDeepseek, "https://api.deepseek.com/anthropic"},
	}
	for i, tc := range cases {
		account := &Account{
			ID:          int64(200 + i),
			Platform:    tc.platform,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Credentials: map[string]any{"api_key": "sk-official", "base_url": tc.baseURL},
		}
		repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
		upstream := &httpUpstreamRecorder{}
		svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

		snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
		require.NoError(t, err, "%s %s", tc.platform, tc.baseURL)
		require.Equal(t, UpstreamBillingProbeStatusUnsupported, snapshot.Status, "%s %s", tc.platform, tc.baseURL)
		require.Nil(t, upstream.lastReq, "%s %s", tc.platform, tc.baseURL)
	}
}

// 官方域判定按规范化 host 的根域后缀匹配：端口、尾点、区域子域都拦；
// 自定义中转与相似但不同的域照常探测。
func TestUpstreamBillingProbeOfficialAPIHostMatchingIsNormalized(t *testing.T) {
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI(""))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://api.x.ai"))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://api.anthropic.com:8443"))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://us-west-2.api.x.ai/v1"))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://generativelanguage.googleapis.com."))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://x.ai"))
	// openai 官方域在全集内；openai 平台账号不经过本判定（行为级测试钉死照探）。
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://api.openai.com"))
	// Ollama Cloud 官方域及其子域（Ollama Cloud 账号的 base_url 允许带 www.）。
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://ollama.com/v1"))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://ollama.com:443/v1"))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://www.ollama.com/v1"))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("HTTPS://OLLAMA.COM./v1"))
	// 国产供应商官方域及子域。
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://api.moonshot.cn/v1"))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://api.kimi.com/coding/v1"))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://open.bigmodel.cn/api/anthropic"))
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://api.deepseek.com/anthropic"))
	// 相似但不同的注册域不拦：中转完全可能叫 *-x.ai 之外的任何名字。
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://relay.example/v1"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://notx.ai"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://anthropic.com.evil.example"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://api.relay-station.example"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://notollama.com/v1"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://ollama.com.evil.example/v1"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://ollama.example/v1"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://notmoonshot.cn/v1"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://moonshot.cn.evil.example/v1"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://kimi.example/v1"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://notbigmodel.cn"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://deepseek.example.com"))
}

// OpenAI 语义保持不变：无自定义 base 时仍探官方域，且沿用 openai 传输画像。
func TestUpstreamBillingProbeOpenAIDefaultBaseURLPreserved(t *testing.T) {
	account := &Account{
		ID:          17,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Credentials: map[string]any{"api_key": "sk-openai"},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       upstreamBillingProbeValidBody(),
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	_, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/sub2api/billing", upstream.lastReq.URL.String())
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
}

func TestUpstreamBillingProbeSetAccountEnabledAcceptsGrokAPIKey(t *testing.T) {
	grokAPIKey := &Account{
		ID:          151,
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Credentials: map[string]any{"api_key": "sk", "base_url": "https://relay.example"},
	}
	grokOAuth := &Account{ID: 152, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		grokAPIKey.ID: grokAPIKey,
		grokOAuth.ID:  grokOAuth,
	}}
	svc := newUpstreamBillingProbeTestService(repo, &upstreamBillingProbeHTTPStub{}, &upstreamBillingProbeSettingRepo{})

	require.NoError(t, svc.SetAccountEnabled(context.Background(), grokAPIKey.ID, true))
	require.Equal(t, true, repo.accounts[grokAPIKey.ID].Extra[UpstreamBillingProbeEnabledExtraKey])

	err := svc.SetAccountEnabled(context.Background(), grokOAuth.ID, true)
	require.ErrorIs(t, err, ErrUpstreamBillingProbeAccountInvalid)
}
