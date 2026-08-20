package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func newResponsesProbeAccount(id int64) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Name:        "compat-upstream",
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat-upstream.example/v1",
		},
	}
}

// runResponsesProbe 跑一次探测，返回落库的 extra 更新；未落库时返回 nil。
func runResponsesProbe(t *testing.T, status int, body string) map[string]any {
	t.Helper()
	account := newResponsesProbeAccount(4200)
	// 带缓冲且不阻塞：探测决定不落标时通道应保持为空。
	updateCalls := make(chan map[string]any, 1)
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	svc := &AccountTestService{
		accountRepo: repo,
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}},
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

	select {
	case updates := <-updateCalls:
		return updates
	default:
		return nil
	}
}

// issue #5371：账号一旦被落标为「不支持 Responses」，网关就长期改走
// /v1/chat/completions，Codex 的 prompt 缓存前缀被打散；而探测只在账号创建/更新时
// 跑一次，标记不会自动恢复。因此判据不成立的响应绝不能落标。
func TestProbeOpenAIAPIKeyResponsesSupport_InconclusiveResponseKeepsUnknown(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			// 探测请求自带 openaiResponsesProbeMaxOutputTokens 预算，推理型模型可能
			// 把预算烧在 reasoning 上就被截断——没有 function_call 是预算不足所致。
			name: "incomplete_max_output_tokens",
			body: `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},` +
				`"output":[{"type":"reasoning","summary":[]}]}`,
		},
		{
			// HTTP 200 携带的失败响应是上游瞬时故障，不构成工具能力证据。
			name: "failed_status_on_http_200",
			body: `{"status":"failed","error":{"code":"server_error","message":"upstream hiccup"},"output":[]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Nil(t, runResponsesProbe(t, http.StatusOK, tc.body),
				"判据不成立时必须保持 unknown，不得落标")
		})
	}
}

// 对照不变式：能下结论的响应仍要落标，否则「不落标」写宽了就等于把整个探测废掉。
func TestProbeOpenAIAPIKeyResponsesSupport_ConclusiveResponsesStillPersist(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{
			name:   "completed_with_function_call",
			status: http.StatusOK,
			body:   `{"status":"completed","output":[{"type":"function_call","name":"probe_ping"}]}`,
			want:   true,
		},
		{
			// 火山方舟 coding/v3 × kimi-k2.6：端点在、跑完了、就是不产出 function_call。
			// 这正是探测要抓的目标，必须继续落标为不支持。
			name:   "completed_reasoning_only",
			status: http.StatusOK,
			body:   `{"status":"completed","output":[{"type":"reasoning"}]}`,
			want:   false,
		},
		{
			// 非 max_output_tokens 的截断（如内容过滤）不在放行范围内，维持原判定。
			name:   "incomplete_other_reason",
			status: http.StatusOK,
			body:   `{"status":"incomplete","incomplete_details":{"reason":"content_filter"},"output":[]}`,
			want:   false,
		},
		{
			// 响应体没有 status 字段（第三方兼容上游常见）时维持既有行为。
			name:   "no_status_field",
			status: http.StatusOK,
			body:   `{"output":[{"type":"function_call","name":"probe_ping"}]}`,
			want:   true,
		},
		{
			name:   "endpoint_absent_404",
			status: http.StatusNotFound,
			body:   `{"error":{"message":"Not Found"}}`,
			want:   false,
		},
		{
			// 非 2xx 的结论只看状态码：body 里的 status=failed 不该让它变成"不下结论"。
			name:   "server_error_stays_conservative_true",
			status: http.StatusInternalServerError,
			body:   `{"status":"failed"}`,
			want:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updates := runResponsesProbe(t, tc.status, tc.body)
			require.NotNil(t, updates, "能下结论的响应必须落标")
			require.Equal(t, tc.want, updates[openai_compat.ExtraKeyResponsesSupported])
		})
	}
}

func TestResponsesProbeVerdictIsConclusive(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"200_completed", 200, `{"status":"completed","output":[]}`, true},
		{"200_incomplete_max_output_tokens", 200,
			`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}`, false},
		{"200_incomplete_content_filter", 200,
			`{"status":"incomplete","incomplete_details":{"reason":"content_filter"}}`, true},
		{"200_incomplete_without_reason", 200, `{"status":"incomplete"}`, true},
		{"200_failed", 200, `{"status":"failed"}`, false},
		{"200_no_status_field", 200, `{"output":[]}`, true},
		{"200_non_json", 200, `not-json`, true},
		{"200_empty_body", 200, ``, true},
		// 非 2xx 只看状态码，不读 body。
		{"404_ignores_body_status", 404, `{"status":"failed"}`, true},
		{"500_ignores_body_status", 500, `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, responsesProbeVerdictIsConclusive(tc.status, []byte(tc.body)))
		})
	}
}
