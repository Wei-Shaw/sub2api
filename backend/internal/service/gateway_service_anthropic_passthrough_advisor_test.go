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

type recordedAdvisorRequest struct {
	method  string
	url     string
	body    []byte
	headers http.Header
}

type scriptedAdvisorResponse struct {
	status int
	body   string
}

type scriptedAdvisorUpstream struct {
	responses []scriptedAdvisorResponse
	calls     []recordedAdvisorRequest
}

func (u *scriptedAdvisorUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	rec := recordedAdvisorRequest{
		method:  req.Method,
		url:     req.URL.String(),
		headers: req.Header.Clone(),
	}
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		rec.body = b
		req.Body = io.NopCloser(bytes.NewReader(b))
	}
	u.calls = append(u.calls, rec)

	idx := len(u.calls) - 1
	if idx >= len(u.responses) {
		idx = len(u.responses) - 1
	}
	scripted := u.responses[idx]
	return &http.Response{
		StatusCode: scripted.status,
		Header:     http.Header{"x-request-id": []string{"rid-test"}},
		Body:       io.NopCloser(strings.NewReader(scripted.body)),
	}, nil
}

func (u *scriptedAdvisorUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func newAdvisorPassthroughTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14,"+AdvisorBetaToken)
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

func newAdvisorEnabledSvc(upstream *scriptedAdvisorUpstream) *GatewayService {
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	settingRepo := &settingRepoStub{values: map[string]string{
		SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true}`,
	}}
	return &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		deferredService:      &DeferredService{},
		settingService:       NewSettingService(settingRepo, cfg),
	}
}

func newAdvisorAccountForTest() *Account {
	return &Account{
		ID:          4153,
		Name:        "anthropic-advisor-passthrough",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
		},
		Extra: map[string]any{
			"anthropic_passthrough": true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func TestAnthropicAPIKeyPassthrough_AdvisorRectifier_RetriesWithoutAdvisor(t *testing.T) {
	advisor400Body := `{"type":"error","error":{"type":"invalid_request_error","message":"Unexpected value(s) ` + "`advisor-tool-2026-03-01`" + ` for the ` + "`anthropic-beta`" + ` header. Please consult our documentation at docs.claude.com or try again without the header."}}`
	successBody := `{"type":"message","id":"msg-1","content":[{"type":"text","text":"ok"}]}`

	upstream := &scriptedAdvisorUpstream{responses: []scriptedAdvisorResponse{
		{status: http.StatusBadRequest, body: advisor400Body},
		{status: http.StatusOK, body: successBody},
	}}

	svc := newAdvisorEnabledSvc(upstream)
	c, _ := newAdvisorPassthroughTestContext()

	body := []byte(`{"model":"claude-3-7-sonnet-20250219","stream":false,"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"advisor_20260301","name":"advisor"},{"type":"web_search_20250305","name":"web_search"}]}`)

	result, err := svc.forwardAnthropicAPIKeyPassthroughWithInput(context.Background(), c, newAdvisorAccountForTest(), anthropicPassthroughForwardInput{
		Body:          body,
		RequestModel:  "claude-3-7-sonnet-20250219",
		OriginalModel: "claude-3-7-sonnet-20250219",
		RequestStream: false,
	})
	require.NoError(t, err, "advisor rectifier retry success should return nil error")
	require.NotNil(t, result, "advisor rectifier retry success should return ForwardResult")

	require.Len(t, upstream.calls, 2, "advisor rectifier should trigger exactly one retry")

	betaSecond := getHeaderRaw(upstream.calls[1].headers, "anthropic-beta")
	require.NotContains(t, strings.ToLower(betaSecond), strings.ToLower(AdvisorBetaToken), "retry request should strip advisor beta token")
	require.Equal(t, "interleaved-thinking-2025-05-14", betaSecond, "non-advisor beta tokens should be retained")

	tools := gjson.GetBytes(upstream.calls[1].body, "tools").Array()
	for _, tool := range tools {
		require.NotEqual(t, AdvisorToolType, tool.Get("type").String(), "retry body should not contain advisor tool")
	}
	hasWebSearch := false
	for _, tool := range tools {
		if tool.Get("type").String() == "web_search_20250305" {
			hasWebSearch = true
		}
	}
	require.True(t, hasWebSearch, "retry body should retain non-advisor tools")

	rawEvents, exists := c.Get(OpsUpstreamErrorsKey)
	require.True(t, exists, "should record at least one ops event")
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.NotEmpty(t, events)

	advisorEv := findEventByKind(events, "advisor_tool_unsupported")
	require.NotNil(t, advisorEv, "should record advisor_tool_unsupported ops event")
	require.True(t, advisorEv.Passthrough, "passthrough advisor event must have Passthrough=true")
	require.Equal(t, http.StatusBadRequest, advisorEv.UpstreamStatusCode)
	require.Equal(t, int64(4153), advisorEv.AccountID)
}

func TestAnthropicAPIKeyPassthrough_AdvisorRectifier_RetryStill400(t *testing.T) {
	advisor400Body := `{"type":"error","error":{"type":"invalid_request_error","message":"Unexpected value(s) ` + "`advisor-tool-2026-03-01`" + ` for the ` + "`anthropic-beta`" + ` header."}}`

	upstream := &scriptedAdvisorUpstream{responses: []scriptedAdvisorResponse{
		{status: http.StatusBadRequest, body: advisor400Body},
		{status: http.StatusBadRequest, body: advisor400Body},
		{status: http.StatusBadRequest, body: advisor400Body},
	}}

	svc := newAdvisorEnabledSvc(upstream)
	c, _ := newAdvisorPassthroughTestContext()

	body := []byte(`{"model":"claude-3-7-sonnet-20250219","stream":false,"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"advisor_20260301","name":"advisor"}]}`)

	_, err := svc.forwardAnthropicAPIKeyPassthroughWithInput(context.Background(), c, newAdvisorAccountForTest(), anthropicPassthroughForwardInput{
		Body:          body,
		RequestModel:  "claude-3-7-sonnet-20250219",
		OriginalModel: "claude-3-7-sonnet-20250219",
		RequestStream: false,
	})
	require.Error(t, err, "persistent 400 should return error, no infinite loop")
	require.Equal(t, 2, len(upstream.calls), "advisor rectifier should retry exactly once; total upstream calls should be 2")
}

func TestAnthropicAPIKeyPassthrough_NonAdvisor400_DoesNotRectify(t *testing.T) {
	other400Body := `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long"}}`

	upstream := &scriptedAdvisorUpstream{responses: []scriptedAdvisorResponse{
		{status: http.StatusBadRequest, body: other400Body},
	}}

	svc := newAdvisorEnabledSvc(upstream)
	c, _ := newAdvisorPassthroughTestContext()

	body := []byte(`{"model":"claude-3-7-sonnet-20250219","stream":false,"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"advisor_20260301","name":"advisor"}]}`)

	_, err := svc.forwardAnthropicAPIKeyPassthroughWithInput(context.Background(), c, newAdvisorAccountForTest(), anthropicPassthroughForwardInput{
		Body:          body,
		RequestModel:  "claude-3-7-sonnet-20250219",
		OriginalModel: "claude-3-7-sonnet-20250219",
		RequestStream: false,
	})
	require.Error(t, err, "non-advisor 400 should also return error to client")
	require.Equal(t, 1, len(upstream.calls), "non-advisor 400 should not trigger advisor retry")

	rawEvents, exists := c.Get(OpsUpstreamErrorsKey)
	if exists {
		events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
		require.True(t, ok)
		require.Nil(t, findEventByKind(events, "advisor_tool_unsupported"), "non-advisor error should not record advisor rectifier event")
	}
}

func TestShouldRectifyAdvisorToolError_PassthroughContext(t *testing.T) {
	advisor400Body := []byte(`{"error":{"message":"Unexpected value(s) ` + "`advisor-tool-2026-03-01`" + ` for the ` + "`anthropic-beta`" + ` header."}}`)
	nonAdvisor400Body := []byte(`{"error":{"message":"prompt is too long"}}`)

	t.Run("advisor body and both switches on returns true", func(t *testing.T) {
		repo := &settingRepoStub{values: map[string]string{
			SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true}`,
		}}
		svc := &GatewayService{settingService: NewSettingService(repo, nil)}
		require.True(t, svc.shouldRectifyAdvisorToolError(context.Background(), advisor400Body))
	})

	t.Run("advisor body but subswitch off returns false", func(t *testing.T) {
		repo := &settingRepoStub{values: map[string]string{
			SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":false}`,
		}}
		svc := &GatewayService{settingService: NewSettingService(repo, nil)}
		require.False(t, svc.shouldRectifyAdvisorToolError(context.Background(), advisor400Body))
	})

	t.Run("non-advisor body with both switches on returns false", func(t *testing.T) {
		repo := &settingRepoStub{values: map[string]string{
			SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true}`,
		}}
		svc := &GatewayService{settingService: NewSettingService(repo, nil)}
		require.False(t, svc.shouldRectifyAdvisorToolError(context.Background(), nonAdvisor400Body))
	})
}

func findEventByKind(events []*OpsUpstreamErrorEvent, kind string) *OpsUpstreamErrorEvent {
	for _, ev := range events {
		if ev != nil && ev.Kind == kind {
			return ev
		}
	}
	return nil
}
