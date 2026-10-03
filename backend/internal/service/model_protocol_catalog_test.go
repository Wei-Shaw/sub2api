//go:build unit

package service

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 取自 https://api.commandcode.ai/provider/v1/models 的片段（2026-09-26）。
const commandCodeModelsSample = `{"object":"list","data":[
	{"id":"claude-sonnet-4-6","supported_endpoints":["/messages"]},
	{"id":"gpt-5.5","supported_endpoints":["/chat/completions","/responses"]},
	{"id":"deepseek/deepseek-v4-flash","supported_endpoints":["/chat/completions","/responses"]},
	{"id":"deepseek/deepseek-v4-flash-fast","supported_endpoints":["/chat/completions"]},
	{"id":"zai-org/GLM-5.3","supported_endpoints":["/chat/completions","/responses"]},
	{"id":"typesafe/jev","supported_endpoints":["/systemone"]},
	{"id":"no-endpoints"}
]}`

func TestParseModelProtocolCatalog(t *testing.T) {
	models, err := parseModelProtocolCatalog([]byte(commandCodeModelsSample))
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"claude-sonnet-4-6":               {APIProtocolAnthropic},
		"gpt-5.5":                         {APIProtocolChatCompletions, APIProtocolResponses},
		"deepseek/deepseek-v4-flash":      {APIProtocolChatCompletions, APIProtocolResponses},
		"deepseek/deepseek-v4-flash-fast": {APIProtocolChatCompletions},
		"zai-org/glm-5.3":                 {APIProtocolChatCompletions, APIProtocolResponses},
	}, models)

	_, err = parseModelProtocolCatalog([]byte(`{"data":[{"id":"gpt-5"},{"id":"glm-5"}]}`))
	require.Error(t, err, "a plain /models without supported_endpoints is not a protocol catalog")
}

func TestModelProtocolCatalogLookupRefreshesInBackground(t *testing.T) {
	catalog := modelProtocolCatalog{firstLoadWait: 20 * time.Millisecond}
	const url = "https://catalog.example/v1/models"
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	var refreshes atomic.Int32
	started := make(chan struct{}, 4)
	refresh := func() { refreshes.Add(1); started <- struct{}{} }

	// 未就绪且刷新迟迟不返回：等待上限后返回 nil；刷新期间不重复触发。
	require.Nil(t, catalog.lookup(url, "gpt-5.5", now, refresh))
	<-started
	require.Nil(t, catalog.lookup(url, "gpt-5.5", now, refresh))
	require.EqualValues(t, 1, refreshes.Load())

	models, err := parseModelProtocolCatalog([]byte(commandCodeModelsSample))
	require.NoError(t, err)
	catalog.store(url, models, nil, now)
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, catalog.lookup(url, "ZAI-ORG/GLM-5.3", now, refresh))
	require.Nil(t, catalog.lookup(url, "unknown-model", now, refresh))
	require.EqualValues(t, 1, refreshes.Load())

	// 过期后继续返回旧目录，同时后台刷新；失败时保留旧目录并退避。
	later := now.Add(modelProtocolCatalogTTL)
	require.NotNil(t, catalog.lookup(url, "gpt-5.5", later, refresh))
	<-started
	require.EqualValues(t, 2, refreshes.Load())
	catalog.store(url, nil, errors.New("boom"), later)
	require.NotNil(t, catalog.lookup(url, "gpt-5.5", later.Add(time.Minute), refresh))
	require.EqualValues(t, 2, refreshes.Load(), "failure backoff suppresses refresh")
	require.NotNil(t, catalog.lookup(url, "gpt-5.5", later.Add(modelProtocolCatalogRetryBackoff), refresh))
	<-started
	require.EqualValues(t, 3, refreshes.Load())
}

// 冷启动：目录从未加载时首批请求等待进行中的刷新，按目录分流；刷新失败则立即回落，
// 退避期内不再等待。
func TestModelProtocolCatalogFirstLoadWaitsForRefresh(t *testing.T) {
	const key = "https://catalog.example/v1/models"
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	models, err := parseModelProtocolCatalog([]byte(commandCodeModelsSample))
	require.NoError(t, err)

	loaded := modelProtocolCatalog{firstLoadWait: 5 * time.Second}
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, loaded.lookup(key, "gpt-5.5", now, func() {
		time.Sleep(10 * time.Millisecond)
		loaded.store(key, models, nil, now)
	}))

	failing := modelProtocolCatalog{firstLoadWait: 5 * time.Second}
	var refreshes atomic.Int32
	refresh := func() {
		refreshes.Add(1)
		failing.store(key, nil, errors.New("boom"), now)
	}
	start := time.Now()
	require.Nil(t, failing.lookup(key, "gpt-5.5", now, refresh))
	require.Nil(t, failing.lookup(key, "gpt-5.5", now.Add(time.Minute), refresh))
	require.Less(t, time.Since(start), time.Second, "a failed first load must not hold requests")
	require.EqualValues(t, 1, refreshes.Load())
}

// 缓存范围与请求头：官方默认地址按地址共享，自定义上游按账号隔离；请求头与转发一致。
func TestModelProtocolCatalogKeyAndHeaders(t *testing.T) {
	official := commandCodeTestAccount(21)
	base := official.GetCNProtocolBaseURL(APIProtocolChatCompletions)
	url := buildOpenAIModelsURL(base)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/models", url)
	require.Equal(t, url, modelProtocolCatalogKey(official, base, url))
	require.Equal(t, url, modelProtocolCatalogKey(commandCodeTestAccount(22), base+"/", url))

	headers := modelProtocolCatalogHeaders(official, url)
	require.Equal(t, "Bearer user_test_key", headers.Get("Authorization"))
	require.Equal(t, CodexCanonicalUserAgent(), headers.Get("User-Agent"), "official host gets the canonical UA like forwarding")

	custom := commandCodeTestAccount(23)
	custom.Credentials["api_base_urls"] = map[string]any{APIProtocolChatCompletions: "https://relay.example/v1"}
	custom.Credentials["header_override_enabled"] = true
	custom.Credentials["header_overrides"] = map[string]any{"X-Tenant": "t-1"}
	customBase := custom.GetCNProtocolBaseURL(APIProtocolChatCompletions)
	customURL := buildOpenAIModelsURL(customBase)
	require.Equal(t, customURL+"#account=23", modelProtocolCatalogKey(custom, customBase, customURL))
	headers = modelProtocolCatalogHeaders(custom, customURL)
	require.Equal(t, "t-1", getHeaderRaw(headers, "x-tenant"))
	require.Empty(t, headers.Get("User-Agent"))
}

func TestCommandCodeModelProtocolSetUsesCatalog(t *testing.T) {
	account := commandCodeTestAccount(11)
	both := []string{APIProtocolChatCompletions, APIProtocolResponses}

	// 目录声明两种协议：入站同协议直通；Anthropic 入站走首选（内置规则没有命中时为 Chat Completions）。
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolResponses, "deepseek/deepseek-v4-flash", both))
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, APIProtocolChatCompletions, "deepseek/deepseek-v4-flash", both))
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, APIProtocolAnthropic, "deepseek/deepseek-v4-flash", both))
	// GPT 的首选沿用内置规则（Responses）。
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolAnthropic, "gpt-5.5", both))
	// 目录只声明 Chat Completions：Responses 入站也转换到 Chat Completions。
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, APIProtocolResponses, "deepseek/deepseek-v4-flash-fast", []string{APIProtocolChatCompletions}))
	require.Equal(t, APIProtocolAnthropic, resolveUpstreamProtocol(account, APIProtocolResponses, "claude-sonnet-4-6", []string{APIProtocolAnthropic}))

	// 账号规则命中时优先于目录；已配置但未命中时仍使用目录。
	account.Credentials["protocol_rules"] = []any{map[string]any{"pattern": "deepseek/*", "protocol": APIProtocolChatCompletions}}
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, APIProtocolResponses, "deepseek/deepseek-v4-flash", both))
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolResponses, "zai-org/glm-5.3", both))

	// 显式协议优先于一切。
	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, APIProtocolResponses, "zai-org/glm-5.3", both))
}

func TestProtocolRulesWithProtocolSets(t *testing.T) {
	credentials := map[string]any{"protocol_rules": []any{
		map[string]any{"pattern": " GPT-* ", "protocol": APIProtocolResponses, "protocols": []any{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolChatCompletions}},
		map[string]any{"pattern": "qwen*", "protocols": []any{APIProtocolAnthropic}},
		map[string]any{"pattern": "glm-*", "protocol": APIProtocolChatCompletions},
	}}
	require.NoError(t, NormalizeProtocolRulesCredentials(credentials))
	require.Equal(t, []any{
		map[string]any{"pattern": "gpt-*", "protocol": APIProtocolResponses, "protocols": []any{APIProtocolResponses, APIProtocolChatCompletions}},
		map[string]any{"pattern": "qwen*", "protocol": APIProtocolAnthropic},
		map[string]any{"pattern": "glm-*", "protocol": APIProtocolChatCompletions},
	}, credentials["protocol_rules"])

	bad := map[string]any{"protocol_rules": []any{map[string]any{"pattern": "gpt-*", "protocols": []any{"responses", "bogus"}}}}
	require.Error(t, NormalizeProtocolRulesCredentials(bad))

	// 单一协议规则（OpenCode）不受影响：与入站无关。
	openCode := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k"}}
	for _, inbound := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
		require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(openCode, inbound, "gpt-5.5", nil), inbound)
		require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(openCode, inbound, "glm-5.3", nil), inbound)
	}
}

// 经真实入口验证：目录声明模型支持入站协议时原样直通，不做协议转换。
func TestCommandCodeGatewayPassesThroughCatalogProtocols(t *testing.T) {
	type observation struct {
		url  string
		body []byte
	}
	forward := func(t *testing.T, ingress routingMatrixIngress, model string, catalog map[string][]string) observation {
		t.Helper()
		base := fmt.Sprintf("http://cc-%s-%d.example", strings.ReplaceAll(t.Name(), "/", "-"), time.Now().UnixNano())
		account := commandCodeTestAccount(12)
		account.Credentials["api_base_urls"] = map[string]any{
			APIProtocolChatCompletions: base + "/provider/v1",
			APIProtocolResponses:       base + "/provider/v1",
			APIProtocolAnthropic:       base + "/provider",
		}
		url := buildOpenAIModelsURL(base + "/provider/v1")
		key := modelProtocolCatalogKey(account, base+"/provider/v1", url)
		if catalog != nil {
			upstreamModelProtocols.store(key, catalog, nil, time.Now())
		} else {
			// 目录不可用（退避中）：不触发刷新，回落内置规则。
			upstreamModelProtocols.store(key, nil, errors.New("unavailable"), time.Now())
		}
		upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		body := routingMatrixCase{ingress: ingress, model: model}.body()
		_ = ingress.forward(svc, adaptiveProtocolTestContext(ingress.path, body), account, body)
		require.NotEmpty(t, upstream.requests)
		return observation{url: upstream.requests[len(upstream.requests)-1].URL.String(), body: upstream.lastBody}
	}
	ingresses := map[string]routingMatrixIngress{}
	for _, ingress := range routingMatrixIngresses() {
		ingresses[ingress.name] = ingress
	}
	catalog := map[string][]string{
		"deepseek/deepseek-v4-flash":      {APIProtocolChatCompletions, APIProtocolResponses},
		"deepseek/deepseek-v4-flash-fast": {APIProtocolChatCompletions},
		"gpt-5.5":                         {APIProtocolChatCompletions, APIProtocolResponses},
	}

	obs := forward(t, ingresses["responses"], "deepseek/deepseek-v4-flash", catalog)
	require.True(t, strings.HasSuffix(obs.url, "/provider/v1/responses"), obs.url)
	require.True(t, gjson.GetBytes(obs.body, "input").Exists(), "Responses body passes through unconverted")

	obs = forward(t, ingresses["responses"], "deepseek/deepseek-v4-flash", nil)
	require.True(t, strings.HasSuffix(obs.url, "/provider/v1/chat/completions"), obs.url)
	require.True(t, gjson.GetBytes(obs.body, "messages").Exists())

	obs = forward(t, ingresses["responses"], "deepseek/deepseek-v4-flash-fast", catalog)
	require.True(t, strings.HasSuffix(obs.url, "/provider/v1/chat/completions"), obs.url)

	obs = forward(t, ingresses["chat"], "gpt-5.5", catalog)
	require.True(t, strings.HasSuffix(obs.url, "/provider/v1/chat/completions"), obs.url)
	require.True(t, gjson.GetBytes(obs.body, "messages").Exists(), "Chat body passes through unconverted")
}

// 目录缺失时网关拉取目录，首个请求即按目录分流；自定义上游按账号各自拉取，并带上
// 账号的请求头覆写。
func TestCommandCodeGatewayFetchesModelCatalog(t *testing.T) {
	base := fmt.Sprintf("http://cc-fetch-%d.example/provider/v1", time.Now().UnixNano())
	newAccount := func(id int64) *Account {
		account := commandCodeTestAccount(id)
		account.Credentials["api_base_urls"] = map[string]any{
			APIProtocolChatCompletions: base,
			APIProtocolResponses:       base,
			APIProtocolAnthropic:       strings.TrimSuffix(base, "/v1"),
		}
		account.Credentials["header_override_enabled"] = true
		account.Credentials["header_overrides"] = map[string]any{"X-Tenant": fmt.Sprintf("t-%d", id)}
		return account
	}
	upstream := &commandCodeAlphaUpstream{responses: map[string]commandCodeAlphaResponse{
		"/provider/v1/models": {status: 200, body: commandCodeModelsSample},
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	both := []string{APIProtocolChatCompletions, APIProtocolResponses}

	first, second := newAccount(13), newAccount(14)
	require.Equal(t, both, svc.modelCatalogProtocols(first, "deepseek/deepseek-v4-flash"))
	require.Equal(t, both, svc.modelCatalogProtocols(first, "deepseek/deepseek-v4-flash"))
	require.Equal(t, both, svc.modelCatalogProtocols(second, "deepseek/deepseek-v4-flash"))

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 2, "one fetch per account on a custom upstream")
	for i, account := range []*Account{first, second} {
		req := upstream.requests[i]
		require.Equal(t, "/provider/v1/models", req.URL.Path)
		require.Equal(t, "Bearer user_test_key", req.Header.Get("Authorization"))
		require.Equal(t, fmt.Sprintf("t-%d", account.ID), getHeaderRaw(req.Header, "x-tenant"))
	}
}
