package service

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// compactProbeSSESuccessBody 是原生 v2 压缩成功的最小 SSE 形态：
// output_item.done 携带 compaction item + response.completed。
const compactProbeSSESuccessBody = "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\",\"id\":\"cmp_probe\",\"encrypted_content\":\"blob\"}}\n\n" +
	"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_probe\",\"output\":[]}}\n\n"

func TestAccountTestService_TestAccountConnection_OpenAICompactOAuthSuccessPersistsSupport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":               "oauth-token",
			"chatgpt_account_id":         "chatgpt-acc",
			"chatgpt_account_is_fedramp": true,
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-probe"}},
		Body:       io.NopCloser(strings.NewReader(compactProbeSSESuccessBody)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
	require.NoError(t, err)

	// 原生 v2：探测普通 /responses 线，不再打已下线的 /responses/compact。
	require.Equal(t, chatgptCodexAPIURL, upstream.lastReq.URL.String())
	require.Equal(t, "chatgpt.com", upstream.lastReq.Host)
	require.Equal(t, "text/event-stream", upstream.lastReq.Header.Get("Accept"))
	require.Contains(t, upstream.lastReq.Header.Get("x-codex-beta-features"), "remote_compaction_v2")
	require.NotEmpty(t, upstream.lastReq.Header.Get("Session_Id"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, codexCLIUserAgent, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "chatgpt-acc", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "true", upstream.lastReq.Header.Get("x-openai-fedramp"))
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
	inputItems := gjson.GetBytes(upstream.lastBody, "input").Array()
	require.NotEmpty(t, inputItems)
	require.Equal(t, "compaction_trigger", inputItems[len(inputItems)-1].Get("type").String())

	updates := <-updateCalls
	require.Equal(t, true, updates["openai_compact_supported"])
	require.Equal(t, http.StatusOK, updates["openai_compact_last_status"])
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_TestAccountConnection_OpenAICompactOAuth404MarksUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          2,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`404 page not found`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/2/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
	require.Error(t, err)

	updates := <-updateCalls
	require.Equal(t, false, updates["openai_compact_supported"])
	require.Equal(t, http.StatusNotFound, updates["openai_compact_last_status"])
	require.Contains(t, rec.Body.String(), `"type":"error"`)
}

func TestAccountTestService_TestAccountConnection_OpenAICompactAPIKeyUsesNativeResponsesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          3,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://example.com/v1",
			// post-#5641：compact_model_mapping 仅作用于 legacy /responses/compact，
			// 原生 v2 探测不应用它。
			"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"},
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(compactProbeSSESuccessBody)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/3/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
	require.NoError(t, err)

	require.Equal(t, "https://example.com/v1/responses", upstream.lastReq.URL.String())
	requireOpenAICodexProbeHeaders(t, upstream.lastReq.Header)
	require.Contains(t, upstream.lastReq.Header.Get("x-codex-beta-features"), "remote_compaction_v2")
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String(),
		"原生 v2 探测不应用 compact_model_mapping")
	updates := <-updateCalls
	require.Equal(t, true, updates["openai_compact_supported"])
}

func TestAccountTestService_TestAccountConnection_OpenAICompactAPIKeyDefaultBaseURLUsesResponsesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          4,
		Name:        "openai-apikey-default",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(compactProbeSSESuccessBody)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/4/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/responses", upstream.lastReq.URL.String())
	<-updateCalls
}

func TestAccountTestService_TestAccountConnection_OpenAICompact2xxWithoutItemMarksUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          5,
		Name:        "openai-oauth-no-item",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	// 200 但流里没有 compaction item：链路吞掉了 compaction_trigger 的形态
	//（#5478 的 "got 0 items"），必须判定为不支持。
	noItemBody := "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"id\":\"msg_1\"}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_x\",\"output\":[]}}\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(noItemBody)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/5/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
	require.Error(t, err)

	updates := <-updateCalls
	require.Equal(t, false, updates["openai_compact_supported"])
	require.Contains(t, rec.Body.String(), `"type":"error"`)
}

// 探测与真实转发走同一 /responses 端点，出站身份必须与真实 Codex 同构：
// session/thread 为 UUID、携带 x-codex-installation-id（收敛账号用收敛值）。
func TestAccountTestService_TestAccountConnection_OpenAICompactProbeIdentityMatchesRealTraffic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          6,
		Name:        "openai-oauth-identity",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
		// 收敛是显式 opt-in（#5610），这里显式开启以验证探测身份与真实流量同构。
		Extra: map[string]any{"codex_fingerprint_mode": "session"},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(compactProbeSSESuccessBody)),
	}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/6/test", bytes.NewReader(nil))

	require.NoError(t, svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact))

	// 显式 session 收敛模式：出站身份 = 账号级收敛值
	converged := resolveConvergedSessionID(&account)
	require.Equal(t, converged, upstream.lastReq.Header.Get("session-id"))
	require.Equal(t, converged, upstream.lastReq.Header.Get("session_id"))
	require.Equal(t, resolveConvergedInstallationID(&account), upstream.lastReq.Header.Get("x-codex-installation-id"),
		"真实 Codex 每个请求必带 installation-id，探测不得缺失")
	require.NotContains(t, upstream.lastReq.Header.Get("session-id"), "probe_compact",
		"探测标识不得是可被上游一眼识别的字面量")
	<-updateCalls
}

func TestCompactProbeSessionID_IsUUIDShaped(t *testing.T) {
	for _, id := range []int64{0, 1, 987654} {
		got := compactProbeSessionID(id)
		_, err := uuid.Parse(got)
		require.NoError(t, err, "探测会话标识必须是 UUID 形态: %s", got)
	}
	require.Equal(t, compactProbeSessionID(7), compactProbeSessionID(7), "同账号应稳定复用同一会话")
	require.NotEqual(t, compactProbeSessionID(7), compactProbeSessionID(8))
}
