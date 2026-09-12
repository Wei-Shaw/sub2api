//go:build unit

package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// zhipu 账号出站 ZCode Desktop 指纹注入（applyZCodeIdentityHeaders）的单元回归。

func zcodeIdentityTestAccount(platform string) *Account {
	return &Account{
		ID:          4105,
		Name:        "cn-identity",
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
	}
}

// claudeCLISampleHeaders 模拟 claude-cli 入站残留的典型身份/协议头组合。
func claudeCLISampleHeaders() http.Header {
	h := http.Header{}
	h.Set("User-Agent", "claude-cli/2.1.81 (external, cli)")
	h.Set("x-app", "cli")
	h.Set("X-Stainless-Lang", "js")
	h.Set("X-Stainless-Runtime", "node")
	h.Set("anthropic-version", "2023-06-01")
	h.Set("Authorization", "Bearer sk-test")
	return h
}

func TestApplyZCodeIdentityHeaders_NonZhipuNoop(t *testing.T) {
	// 非 zhipu（kimi）账号：header 必须完全不变。
	h := claudeCLISampleHeaders()
	before := h.Clone()
	applyZCodeIdentityHeaders(h, zcodeIdentityTestAccount(PlatformKimi), zcodeIdentityAnthropic)
	require.Equal(t, before, h)
}

func TestApplyZCodeIdentityHeaders_ZhipuOverwritesClientFingerprint(t *testing.T) {
	h := claudeCLISampleHeaders()
	applyZCodeIdentityHeaders(h, zcodeIdentityTestAccount(PlatformZhipu), zcodeIdentityAnthropic)

	// claude-cli 身份被剥离 / 覆盖，UA 以 ZCode 版本开头。
	require.True(t, strings.HasPrefix(getHeaderRaw(h, "user-agent"), "ZCode/"+zcodeAppVersion))
	require.Empty(t, getHeaderRaw(h, "x-app"))
	require.Empty(t, getHeaderRaw(h, "x-stainless-lang"))
	require.Empty(t, getHeaderRaw(h, "x-stainless-runtime"))

	// 固定指纹头。
	require.Equal(t, zcodeAppVersion, getHeaderRaw(h, "X-Zcode-App-Version"))
	require.Equal(t, "glm", getHeaderRaw(h, "X-Zcode-Agent"))
	require.Equal(t, "Z Code@electron", getHeaderRaw(h, "X-Title"))

	// Referer 必须落在字面 key HTTP-Referer 上（Header.Set 会写成 Http-Referer）。
	require.Equal(t, []string{"https://zcode.z.ai"}, h["HTTP-Referer"])
	require.NotContains(t, h, "Http-Referer")

	// 不动鉴权与协议头。
	require.Equal(t, "Bearer sk-test", h.Get("Authorization"))
	require.Equal(t, "2023-06-01", getHeaderRaw(h, "anthropic-version"))

	// 随机 / 稳定 UUID 均为合法 UUID 形态。
	for _, key := range []string{"X-Zcode-Trace-Id", "X-Request-Id", "X-Query-Id", "X-Session-Id"} {
		_, err := uuid.Parse(getHeaderRaw(h, key))
		require.NoError(t, err, key)
	}
}

func TestApplyZCodeIdentityHeaders_SessionIDStableTraceIDVarying(t *testing.T) {
	account := zcodeIdentityTestAccount(PlatformZhipu)
	h1, h2 := http.Header{}, http.Header{}
	applyZCodeIdentityHeaders(h1, account, zcodeIdentityOpenAI)
	applyZCodeIdentityHeaders(h2, account, zcodeIdentityOpenAI)

	// 同一账号：session-id 恒定，trace-id 逐请求变化。
	require.Equal(t, zcodeSessionIDForAccount(account.ID), getHeaderRaw(h1, "X-Session-Id"))
	require.Equal(t, getHeaderRaw(h1, "X-Session-Id"), getHeaderRaw(h2, "X-Session-Id"))
	require.NotEqual(t, getHeaderRaw(h1, "X-Zcode-Trace-Id"), getHeaderRaw(h2, "X-Zcode-Trace-Id"))
}

func TestApplyZCodeIdentityHeaders_UAVariesByKind(t *testing.T) {
	account := zcodeIdentityTestAccount(PlatformZhipu)
	hOpenAI, hAnthropic := http.Header{}, http.Header{}
	applyZCodeIdentityHeaders(hOpenAI, account, zcodeIdentityOpenAI)
	applyZCodeIdentityHeaders(hAnthropic, account, zcodeIdentityAnthropic)

	// 两条 SDK 栈 UA 后缀不同（与真实 ZCode Desktop 抓包一致）。
	require.Equal(t, "ZCode/3.2.5 ai-sdk/provider-utils/4.0.27 runtime/node.js/24",
		getHeaderRaw(hOpenAI, "User-Agent"))
	require.Equal(t, "ZCode/3.2.5 ai-sdk/anthropic/3.0.81",
		getHeaderRaw(hAnthropic, "User-Agent"))
	require.NotEqual(t, getHeaderRaw(hOpenAI, "User-Agent"), getHeaderRaw(hAnthropic, "User-Agent"))
}

func TestApplyZCodeIdentityHeaders_NilSafety(t *testing.T) {
	// h / account 任一为 nil 都必须安全 no-op。
	require.NotPanics(t, func() {
		applyZCodeIdentityHeaders(nil, zcodeIdentityTestAccount(PlatformZhipu), zcodeIdentityOpenAI)
		applyZCodeIdentityHeaders(http.Header{}, nil, zcodeIdentityOpenAI)
		applyZCodeIdentityHeaders(nil, nil, zcodeIdentityAnthropic)
	})
	h := http.Header{}
	applyZCodeIdentityHeaders(h, nil, zcodeIdentityOpenAI)
	require.Empty(t, h)
}
