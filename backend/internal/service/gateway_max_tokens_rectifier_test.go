//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	maxTokensModelLimitError   = "max_tokens: 131072 > 128000, which is the maximum allowed number of output tokens for claude-opus-5-5"
	maxTokensNonStreamingError = "non-streaming requests cannot set max_tokens to 128000 or above; use stream=true or a lower max_tokens"
)

func TestParseMaxTokensLimitError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message string
		want    int64
		ok      bool
	}{
		{"model limit", maxTokensModelLimitError, 128000, true},
		{"model limit other model", "max_tokens: 70000 > 64000, which is the maximum allowed number of output tokens for claude-sonnet-4-5-20250929", 64000, true},
		{"model limit uppercase", strings.ToUpper(maxTokensModelLimitError), 128000, true},
		{"non-streaming", maxTokensNonStreamingError, 127999, true},
		{"model limit without numbers", "max_tokens: too large > limit, which is the maximum allowed number of output tokens", 0, false},
		{"model limit requested not above limit", "max_tokens: 1000 > 128000, which is the maximum allowed number of output tokens for x", 0, false},
		{"model limit overflow", "max_tokens: 99999999999999999999 > 128000, which is the maximum allowed number of output tokens for x", 0, false},
		{"non-streaming without number", "non-streaming requests cannot set max_tokens to a large value or above", 0, false},
		{"budget error", "thinking.budget_tokens must be >= 1024", 0, false},
		{"budget vs max_tokens", "`max_tokens` must be greater than `thinking.budget_tokens`", 0, false},
		{"context window", "prompt is too long: 210000 tokens > 200000 maximum", 0, false},
		{"input + max_tokens over context", "input length and `max_tokens` exceed context limit: 190000 + 64000 > 200000, decrease input length or `max_tokens` and try again", 0, false},
		{"field required", "max_tokens: Field required", 0, false},
		{"empty", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseMaxTokensLimitError(tc.message)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRectifyMaxTokens(t *testing.T) {
	t.Run("lowers max_tokens without thinking", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-5-5","max_tokens":131072,"messages":[{"role":"user","content":"hi"}]}`)
		got, changed := RectifyMaxTokens(body, 128000)
		require.True(t, changed)
		require.Equal(t, int64(128000), gjson.GetBytes(got, "max_tokens").Int())
		require.False(t, gjson.GetBytes(got, "thinking").Exists())
		require.Equal(t, "hi", gjson.GetBytes(got, "messages.0.content").String())
	})

	t.Run("non-streaming limit", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":128000,"stream":false,"messages":[]}`)
		limit, ok := parseMaxTokensLimitError(maxTokensNonStreamingError)
		require.True(t, ok)
		got, changed := RectifyMaxTokens(body, limit)
		require.True(t, changed)
		require.Equal(t, int64(127999), gjson.GetBytes(got, "max_tokens").Int())
	})

	t.Run("keeps budget already below new max_tokens", func(t *testing.T) {
		body := []byte(`{"max_tokens":131072,"thinking":{"type":"enabled","budget_tokens":32000}}`)
		got, changed := RectifyMaxTokens(body, 128000)
		require.True(t, changed)
		require.Equal(t, int64(128000), gjson.GetBytes(got, "max_tokens").Int())
		require.Equal(t, int64(32000), gjson.GetBytes(got, "thinking.budget_tokens").Int())
		require.Equal(t, "enabled", gjson.GetBytes(got, "thinking.type").String())
	})

	t.Run("scales budget at or above new max_tokens", func(t *testing.T) {
		body := []byte(`{"max_tokens":200000,"thinking":{"type":"enabled","budget_tokens":130000}}`)
		got, changed := RectifyMaxTokens(body, 127999)
		require.True(t, changed)
		require.Equal(t, int64(127999), gjson.GetBytes(got, "max_tokens").Int())
		budget := gjson.GetBytes(got, "thinking.budget_tokens").Int()
		require.Equal(t, int64(83199), budget)
		require.Less(t, budget, gjson.GetBytes(got, "max_tokens").Int())
	})

	t.Run("budget equal to new max_tokens", func(t *testing.T) {
		body := []byte(`{"max_tokens":131072,"thinking":{"type":"enabled","budget_tokens":128000}}`)
		got, changed := RectifyMaxTokens(body, 128000)
		require.True(t, changed)
		budget := gjson.GetBytes(got, "thinking.budget_tokens").Int()
		require.Less(t, budget, int64(128000))
		require.GreaterOrEqual(t, budget, int64(maxTokensRectifyMinBudgetTokens))
	})

	t.Run("budget not below original max_tokens is clamped", func(t *testing.T) {
		body := []byte(`{"max_tokens":131072,"thinking":{"type":"enabled","budget_tokens":140000}}`)
		got, changed := RectifyMaxTokens(body, 128000)
		require.True(t, changed)
		require.Equal(t, int64(127999), gjson.GetBytes(got, "thinking.budget_tokens").Int())
	})

	t.Run("no legal budget fits below limit", func(t *testing.T) {
		body := []byte(`{"max_tokens":4096,"thinking":{"type":"enabled","budget_tokens":4000}}`)
		got, changed := RectifyMaxTokens(body, 1024)
		require.False(t, changed)
		require.Equal(t, body, got)
	})

	t.Run("adaptive only changes max_tokens", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-5-5","max_tokens":131072,"thinking":{"type":"adaptive"},"output_config":{"effort":"max"}}`)
		got, changed := RectifyMaxTokens(body, 128000)
		require.True(t, changed)
		require.Equal(t, int64(128000), gjson.GetBytes(got, "max_tokens").Int())
		require.JSONEq(t, `{"type":"adaptive"}`, gjson.GetBytes(got, "thinking").Raw)
		require.Equal(t, "max", gjson.GetBytes(got, "output_config.effort").String())
	})

	t.Run("adaptive with stray budget_tokens is left untouched", func(t *testing.T) {
		body := []byte(`{"max_tokens":131072,"thinking":{"type":"adaptive","budget_tokens":130000}}`)
		got, changed := RectifyMaxTokens(body, 128000)
		require.True(t, changed)
		require.Equal(t, int64(128000), gjson.GetBytes(got, "max_tokens").Int())
		require.JSONEq(t, `{"type":"adaptive","budget_tokens":130000}`, gjson.GetBytes(got, "thinking").Raw)
	})

	t.Run("disabled thinking only changes max_tokens", func(t *testing.T) {
		body := []byte(`{"max_tokens":131072,"thinking":{"type":"disabled"}}`)
		got, changed := RectifyMaxTokens(body, 128000)
		require.True(t, changed)
		require.Equal(t, int64(128000), gjson.GetBytes(got, "max_tokens").Int())
		require.JSONEq(t, `{"type":"disabled"}`, gjson.GetBytes(got, "thinking").Raw)
	})

	t.Run("already within limit", func(t *testing.T) {
		body := []byte(`{"max_tokens":128000}`)
		got, changed := RectifyMaxTokens(body, 128000)
		require.False(t, changed)
		require.Equal(t, body, got)
	})

	t.Run("missing max_tokens", func(t *testing.T) {
		body := []byte(`{"messages":[]}`)
		got, changed := RectifyMaxTokens(body, 128000)
		require.False(t, changed)
		require.Equal(t, body, got)
	})
}

type maxTokensRectifierSettingRepo struct {
	fakeSettingRepo
}

func (r *maxTokensRectifierSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := r.vals[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

type maxTokensRectifierUpstream struct {
	bodies    [][]byte
	responses []*http.Response
}

func (u *maxTokensRectifierUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	b, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	u.bodies = append(u.bodies, b)
	if len(u.responses) == 0 {
		panic("maxTokensRectifierUpstream: unexpected extra upstream request")
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

func (u *maxTokensRectifierUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func maxTokensErrorResponse(message string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"type":"error","error":{"type":"invalid_request_error","message":"` + message + `"}}`)),
	}
}

func maxTokensOKResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`)),
	}
}

func runMaxTokensRectifierForward(t *testing.T, rectifierSettings string, body string, responses ...*http.Response) (*maxTokensRectifierUpstream, *ParsedRequest, *ForwardResult, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), PlatformAnthropic)
	require.NoError(t, err)

	upstream := &maxTokensRectifierUpstream{responses: responses}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	vals := map[string]string{}
	if rectifierSettings != "" {
		vals[SettingKeyRectifierSettings] = rectifierSettings
	}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		deferredService:      &DeferredService{},
		settingService:       &SettingService{settingRepo: &maxTokensRectifierSettingRepo{fakeSettingRepo{vals: vals}}},
	}
	account := &Account{
		ID:          601,
		Name:        "anthropic-apikey-max-tokens",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, parsed)
	return upstream, parsed, result, err
}

func TestGatewayService_Forward_MaxTokensRectifierRetriesOnce(t *testing.T) {
	body := `{"model":"claude-opus-5-5","max_tokens":131072,"stream":false,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"hello"}]}`
	upstream, parsed, result, err := runMaxTokensRectifierForward(t, "", body,
		maxTokensErrorResponse(maxTokensModelLimitError),
		maxTokensOKResponse(),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, int64(131072), gjson.GetBytes(upstream.bodies[0], "max_tokens").Int())
	require.Equal(t, int64(128000), gjson.GetBytes(upstream.bodies[1], "max_tokens").Int())
	require.Equal(t, "adaptive", gjson.GetBytes(upstream.bodies[1], "thinking.type").String())
	require.Equal(t, int64(128000), gjson.GetBytes(parsed.Body.Bytes(), "max_tokens").Int())
}

func TestGatewayService_Forward_MaxTokensRectifierNonStreamingAdjustsBudget(t *testing.T) {
	body := `{"model":"claude-sonnet-4-5","max_tokens":200000,"stream":false,"thinking":{"type":"enabled","budget_tokens":130000},"messages":[{"role":"user","content":"hello"}]}`
	upstream, _, result, err := runMaxTokensRectifierForward(t, "", body,
		maxTokensErrorResponse(maxTokensNonStreamingError),
		maxTokensOKResponse(),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, int64(127999), gjson.GetBytes(upstream.bodies[1], "max_tokens").Int())
	require.Equal(t, int64(83199), gjson.GetBytes(upstream.bodies[1], "thinking.budget_tokens").Int())
}

func TestGatewayService_Forward_MaxTokensRectifierDoesNotLoop(t *testing.T) {
	body := `{"model":"claude-opus-5-5","max_tokens":131072,"stream":false,"messages":[{"role":"user","content":"hello"}]}`
	upstream, _, _, err := runMaxTokensRectifierForward(t, "", body,
		maxTokensErrorResponse(maxTokensModelLimitError),
		maxTokensErrorResponse("max_tokens: 128000 > 64000, which is the maximum allowed number of output tokens for claude-opus-5-5"),
	)
	require.Error(t, err)
	require.Len(t, upstream.bodies, 2)
}

func TestGatewayService_Forward_MaxTokensRectifierRespectsMasterSwitch(t *testing.T) {
	body := `{"model":"claude-opus-5-5","max_tokens":131072,"stream":false,"messages":[{"role":"user","content":"hello"}]}`
	upstream, _, _, err := runMaxTokensRectifierForward(t,
		`{"enabled":false,"thinking_signature_enabled":true,"thinking_budget_enabled":true}`, body,
		maxTokensErrorResponse(maxTokensModelLimitError),
	)
	require.Error(t, err)
	require.Len(t, upstream.bodies, 1)
}

func TestGatewayService_Forward_MaxTokensRectifierRespectsSubSwitch(t *testing.T) {
	body := `{"model":"claude-opus-5-5","max_tokens":131072,"stream":false,"messages":[{"role":"user","content":"hello"}]}`
	upstream, _, _, err := runMaxTokensRectifierForward(t,
		`{"enabled":true,"thinking_signature_enabled":true,"thinking_budget_enabled":true,"max_tokens_enabled":false}`, body,
		maxTokensErrorResponse(maxTokensModelLimitError),
	)
	require.Error(t, err)
	require.Len(t, upstream.bodies, 1)
}

func TestGatewayService_Forward_MaxTokensRectifierEnabledForLegacySettings(t *testing.T) {
	body := `{"model":"claude-opus-5-5","max_tokens":131072,"stream":false,"messages":[{"role":"user","content":"hello"}]}`
	upstream, _, _, err := runMaxTokensRectifierForward(t,
		`{"enabled":true,"thinking_signature_enabled":true,"thinking_budget_enabled":true,"apikey_signature_enabled":false,"apikey_signature_patterns":null}`, body,
		maxTokensErrorResponse(maxTokensModelLimitError),
		maxTokensOKResponse(),
	)
	require.NoError(t, err)
	require.Len(t, upstream.bodies, 2)
}

func TestGetRectifierSettings_MissingFieldsUseDefaults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored string
		want   RectifierSettings
	}{
		{
			name:   "legacy JSON without max_tokens_enabled",
			stored: `{"enabled":true,"thinking_signature_enabled":false,"thinking_budget_enabled":true,"apikey_signature_enabled":true,"apikey_signature_patterns":["x"]}`,
			want:   RectifierSettings{Enabled: true, ThinkingSignatureEnabled: false, ThinkingBudgetEnabled: true, MaxTokensEnabled: true, APIKeySignatureEnabled: true, APIKeySignaturePatterns: []string{"x"}},
		},
		{
			name:   "explicitly disabled",
			stored: `{"enabled":false,"thinking_signature_enabled":true,"thinking_budget_enabled":true,"max_tokens_enabled":false}`,
			want:   RectifierSettings{Enabled: false, ThinkingSignatureEnabled: true, ThinkingBudgetEnabled: true, MaxTokensEnabled: false},
		},
		{
			name:   "invalid JSON",
			stored: `{`,
			want:   *DefaultRectifierSettings(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{SettingKeyRectifierSettings: tc.stored}}}
			got, err := svc.GetRectifierSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, *got)
		})
	}

	svc := &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{}}}
	got, err := svc.GetRectifierSettings(context.Background())
	require.NoError(t, err)
	require.True(t, got.MaxTokensEnabled)
	require.True(t, svc.IsMaxTokensRectifierEnabled(context.Background()))
}

func TestGatewayService_Forward_MaxTokensRectifierIgnoresUnrelatedError(t *testing.T) {
	body := `{"model":"claude-opus-5-5","max_tokens":131072,"stream":false,"messages":[{"role":"user","content":"hello"}]}`
	upstream, _, _, err := runMaxTokensRectifierForward(t, "", body,
		maxTokensErrorResponse("messages: at least one message is required"),
	)
	require.Error(t, err)
	require.Len(t, upstream.bodies, 1)
}
