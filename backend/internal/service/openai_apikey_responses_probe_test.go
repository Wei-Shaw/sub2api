package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestProbeOpenAIAPIKeyResponsesSupportUsesCodexProbeHeaders(t *testing.T) {
	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          96,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat-upstream.example/v1",
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"output":[{"type":"function_call","name":"probe_ping"}]}`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://compat-upstream.example/v1/responses", upstream.lastReq.URL.String())
	requireOpenAICodexProbeHeaders(t, upstream.lastReq.Header)
	updates := <-updateCalls
	require.Equal(t, true, updates[openai_compat.ExtraKeyResponsesSupported])
}

func TestProbeOpenAIAPIKeyResponsesSupportCNProviders(t *testing.T) {
	tests := []struct {
		name        string
		id          int64
		platform    string
		protocol    string
		wantSupport bool
		wantMode    string
	}{
		{name: "deepseek adaptive supports responses", id: 201, platform: PlatformDeepseek, protocol: APIProtocolAdaptive, wantSupport: true, wantMode: string(openai_compat.ResponsesSupportModeForceResponses)},
		{name: "deepseek chat clears forced responses", id: 202, platform: PlatformDeepseek, protocol: APIProtocolChatCompletions, wantSupport: false, wantMode: string(openai_compat.ResponsesSupportModeAuto)},
		{name: "kimi adaptive supports responses", id: 203, platform: PlatformKimi, protocol: APIProtocolAdaptive, wantSupport: true, wantMode: string(openai_compat.ResponsesSupportModeForceResponses)},
		{name: "kimi responses protocol supports responses", id: 205, platform: PlatformKimi, protocol: APIProtocolResponses, wantSupport: true, wantMode: string(openai_compat.ResponsesSupportModeForceResponses)},
		{name: "zhipu adaptive falls back to chat", id: 204, platform: PlatformZhipu, protocol: APIProtocolAdaptive, wantSupport: false, wantMode: string(openai_compat.ResponsesSupportModeAuto)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateCalls := make(chan map[string]any, 1)
			account := Account{
				ID: tc.id, Platform: tc.platform, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-test", "api_protocol": tc.protocol},
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses),
				},
			}
			repo := &snapshotUpdateAccountRepo{
				stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
				updateExtraCalls:      updateCalls,
			}
			svc := &AccountTestService{accountRepo: repo}

			svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

			updates := <-updateCalls
			require.Equal(t, tc.wantSupport, updates[openai_compat.ExtraKeyResponsesSupported])
			require.Equal(t, tc.wantMode, updates[openai_compat.ExtraKeyResponsesMode])
		})
	}
}

// TestProbeOpenAIAPIKeyResponsesSupportOllamaCloud 验证 C39：ollama_cloud 账号与
// CN 分支同款按协议直接落标，不走网络探测（C31 后 ollama 有原生 /v1/responses，
// 探测的 2xx/404 判定对它无意义）；显式 CC 重置为 auto 防残留强制模式。
func TestProbeOpenAIAPIKeyResponsesSupportOllamaCloud(t *testing.T) {
	tests := []struct {
		name        string
		id          int64
		protocol    string
		wantMode    string
		wantSupport bool
	}{
		{name: "adaptive marks force_responses", id: 221, protocol: APIProtocolAdaptive, wantMode: string(openai_compat.ResponsesSupportModeForceResponses), wantSupport: true},
		{name: "responses protocol marks force_responses", id: 222, protocol: APIProtocolResponses, wantMode: string(openai_compat.ResponsesSupportModeForceResponses), wantSupport: true},
		{name: "chat completions resets to auto", id: 223, protocol: APIProtocolChatCompletions, wantMode: string(openai_compat.ResponsesSupportModeAuto), wantSupport: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateCalls := make(chan map[string]any, 1)
			account := Account{
				ID: tc.id, Platform: PlatformOllamaCloud, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-test", "api_protocol": tc.protocol},
				Extra: map[string]any{
					// 预置与目标相反的残留标记，验证落标会覆盖。
					openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
					openai_compat.ExtraKeyResponsesSupported: false,
				},
			}
			repo := &snapshotUpdateAccountRepo{
				stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
				updateExtraCalls:      updateCalls,
			}
			upstream := &httpUpstreamRecorder{}
			svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}

			svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

			updates := <-updateCalls
			require.Equal(t, tc.wantSupport, updates[openai_compat.ExtraKeyResponsesSupported])
			require.Equal(t, tc.wantMode, updates[openai_compat.ExtraKeyResponsesMode])
			// 不发任何网络探测请求。
			require.Nil(t, upstream.lastReq)
			require.Empty(t, upstream.requests)
		})
	}
}

// failingMarkerRepo 使协议 marker 落标（UpdateExtra）返回错误，用于断言失败会产生
// 可观测信号。
type failingMarkerRepo struct {
	stubOpenAIAccountRepo
	err error
}

func (r *failingMarkerRepo) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	return r.err
}

// TestProbeOpenAIAPIKeyResponsesSupportMarkerPersistFailureLogsWarning 评审证伪回归：
// 协议 marker 落标失败必须有可观测信号。此前 _ = UpdateExtra 把错误完全吞掉——
// 协议更新「成功」而旧 force_responses / auto marker 静默残留，且没有任何日志。
// 注意：ollama_cloud / CN 的请求路由以 credentials.api_protocol 为权威（探针 Extra
// 不得带偏协议决策），因此本修复锁的是可观测性，不是路由正确性。
func TestProbeOpenAIAPIKeyResponsesSupportMarkerPersistFailureLogsWarning(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol string
	}{
		{name: "force_responses落标失败_告警", protocol: APIProtocolAdaptive},
		{name: "重置auto落标失败_告警", protocol: APIProtocolChatCompletions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			defer slog.SetDefault(previousLogger)

			account := Account{
				ID:          231,
				Platform:    PlatformOllamaCloud,
				Type:        AccountTypeAPIKey,
				Name:        "ollama-marker-fail",
				Credentials: map[string]any{"api_key": "sk-test", "api_protocol": tc.protocol},
			}
			repo := &failingMarkerRepo{
				stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
				err:                   errors.New("db write failed"),
			}
			svc := &AccountTestService{accountRepo: repo}

			svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

			output := logs.String()
			require.Contains(t, output, "openai_responses_probe_marker_persist_failed",
				"UpdateExtra 失败必须产生结构化告警")
			require.Contains(t, output, "account_id=231")
			require.Contains(t, output, "db write failed")
		})
	}
}

func TestDecideResponsesProbeSupport(t *testing.T) {
	fnCall := []byte(`{"output":[{"type":"reasoning"},{"type":"function_call","name":"probe_ping"}]}`)
	reasoningOnly := []byte(`{"output":[{"type":"reasoning"}]}`)

	cases := []struct {
		name   string
		status int
		body   []byte
		want   bool
	}{
		// Endpoint clearly absent on third-party OpenAI-compatible upstreams.
		{"404 endpoint absent", 404, fnCall, false},
		{"405 method not allowed", 405, fnCall, false},
		// 2xx: tool capability is judged by presence of a function_call output item.
		{"200 with function_call", 200, fnCall, true},
		// Volcengine Ark coding/v3 × kimi-k2.6: reasoning only, no function_call.
		{"200 reasoning only", 200, reasoningOnly, false},
		{"200 invalid json", 200, []byte("not-json"), false},
		{"200 no output field", 200, []byte(`{"status":"completed"}`), false},
		// Non-2xx (other than 404/405): endpoint exists, capability undecidable -> conservative true.
		{"400 conservative true", 400, reasoningOnly, true},
		{"401 conservative true", 401, nil, true},
		{"500 conservative true", 500, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, decideResponsesProbeSupport(tc.status, tc.body))
		})
	}
}

func TestResponsesProbeBodyHasFunctionCall(t *testing.T) {
	require.True(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"function_call"}]}`)))
	require.True(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"reasoning"},{"type":"function_call"}]}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"reasoning"}]}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[]}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`garbage`)))
}

func TestSelectResponsesProbeModel(t *testing.T) {
	// No model_mapping -> fall back to DefaultTestModel (OpenAI official APIKey).
	require.Equal(t, openai.DefaultTestModel, selectResponsesProbeModel(&Account{}))

	// model_mapping values are upstream models; pick first by sort for reproducibility.
	acct := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{
			"client-b": "zeta-model",
			"client-a": "alpha-model",
		},
	}}
	require.Equal(t, "alpha-model", selectResponsesProbeModel(acct))

	// Wildcard / blank upstream values are skipped.
	acctWild := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{
			"a": "*",
			"b": "  ",
			"c": "real-model",
		},
	}}
	require.Equal(t, "real-model", selectResponsesProbeModel(acctWild))

	// Only wildcard mappings -> DefaultTestModel.
	acctAllWild := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{"a": "gpt-*"},
	}}
	require.Equal(t, openai.DefaultTestModel, selectResponsesProbeModel(acctAllWild))
}
