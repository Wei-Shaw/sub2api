package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const cpaLegacyCyberEvent = `{"type":"error","code":"cyber_policy","message":"blocked by CPA","sequence_number":0}`

type cyberBlockCheckingWriter struct {
	gin.ResponseWriter
	t       *testing.T
	store   *comboCacheAndStore
	root    string
	seen    strings.Builder
	checked bool
}

func (w *cyberBlockCheckingWriter) Write(body []byte) (int, error) {
	_, _ = w.seen.Write(body)
	if !w.checked && strings.Contains(w.seen.String(), `"code":"cyber_policy"`) {
		w.checked = true
		require.True(w.t, w.store.store.blocked[w.root], "cyber block must exist before the failure bytes reach the client")
		w.store.events = append(w.store.events, "downstream_write")
	}
	return w.ResponseWriter.Write(body)
}

func (w *cyberBlockCheckingWriter) WriteString(body string) (int, error) {
	return w.Write([]byte(body))
}

func openAITestSSEData(t *testing.T, event string) string {
	t.Helper()
	for _, line := range strings.Split(event, "\n") {
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	t.Fatalf("SSE event has no data line: %q", event)
	return ""
}

func TestCPALegacyCyberErrorFieldsReachEveryConsumer(t *testing.T) {
	hit, code, message := detectOpenAICyberPolicy([]byte(cpaLegacyCyberEvent))
	require.True(t, hit)
	require.Equal(t, "cyber_policy", code)
	require.Equal(t, "blocked by CPA", message)

	httpSSE := buildOpenAIResponseFailedSSE("resp_cpa", "gpt-5.6-sol", []byte(cpaLegacyCyberEvent), "")
	httpPayload := openAITestSSEData(t, httpSSE)
	require.Equal(t, "response.failed", gjson.Get(httpPayload, "type").String())
	require.Equal(t, "resp_cpa", gjson.Get(httpPayload, "response.id").String())
	require.Equal(t, "gpt-5.6-sol", gjson.Get(httpPayload, "response.model").String())
	require.Equal(t, "response", gjson.Get(httpPayload, "response.object").String())
	require.Equal(t, "failed", gjson.Get(httpPayload, "response.status").String())
	require.Equal(t, "[]", gjson.Get(httpPayload, "response.output").Raw)
	require.Equal(t, "cyber_policy", gjson.Get(httpPayload, "response.error.code").String())
	require.Equal(t, "blocked by CPA", gjson.Get(httpPayload, "response.error.message").String())

	wsEvent := buildOpenAIWSHTTPBridgeFailedEvent("resp_cpa", "gpt-5.6-sol", []byte(cpaLegacyCyberEvent), "")
	require.Equal(t, "cyber_policy", gjson.GetBytes(wsEvent, "response.error.code").String())
	require.Equal(t, "blocked by CPA", gjson.GetBytes(wsEvent, "response.error.message").String())

	require.Equal(t, "cyber_policy", openAIStreamFailedEventErrorCode([]byte(cpaLegacyCyberEvent)))
	require.Equal(t, http.StatusBadRequest, openAIStreamFailedEventSemanticStatus([]byte(cpaLegacyCyberEvent), "blocked by CPA"))
	require.False(t, openAIStreamErrorEventShouldFailover([]byte(cpaLegacyCyberEvent), "please retry after policy review"))

	wsCode, wsType, wsMessage := parseOpenAIWSErrorEventFields([]byte(cpaLegacyCyberEvent))
	require.Equal(t, "cyber_policy", wsCode)
	require.Empty(t, wsType)
	require.Equal(t, "blocked by CPA", wsMessage)
}

func TestCPALegacyCapacityCodeIsRewrittenOnlyForClientDelivery(t *testing.T) {
	payload := []byte(`{"type":"error","code":"server_is_overloaded","message":"Our servers are currently overloaded"}`)

	updated, changed := sanitizeOpenAICapacityShedErrorCodeForClient(payload)

	require.True(t, changed)
	require.Equal(t, openAICapacityShedRetryableClientCode, gjson.GetBytes(updated, "code").String())
	require.Equal(t, "Our servers are currently overloaded", gjson.GetBytes(updated, "message").String())
}

func TestCPALegacyCyberBareErrorEOFPreservesCodeAndSkipsAccountMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stream := "event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_cpa","status":"in_progress"}}` + "\n\n" +
		"event: error\n" +
		"data: " + cpaLegacyCyberEvent + "\n\n"

	tests := []struct {
		name string
		run  func(*OpenAIGatewayService, *gin.Context, *http.Response, *Account) error
	}{
		{
			name: "native SSE",
			run: func(svc *OpenAIGatewayService, c *gin.Context, resp *http.Response, account *Account) error {
				_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
				return err
			},
		},
		{
			name: "passthrough SSE",
			run: func(svc *OpenAIGatewayService, c *gin.Context, resp *http.Response, account *Account) error {
				_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &openAIAuthPolicyAccountRepo{}
			combo := &comboCacheAndStore{}
			svc := enabledCyberSessionTestService(combo)
			svc.cfg = &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
			svc.rateLimitService = &RateLimitService{accountRepo: repo}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			requestBody := []byte(`{"input":"trigger"}`)
			BindCyberSessionIdentity(c, svc.PrepareCyberSessionIdentity(c.Request.Context(), 7, 11, c, requestBody, "203.0.113.1", "client/1.0"))
			writer := &cyberBlockCheckingWriter{ResponseWriter: c.Writer, t: t, store: combo, root: getCyberSessionIdentity(c).lineageRoot}
			c.Writer = writer
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(stream)),
			}

			err := tt.run(svc, c, resp, &Account{ID: 991, Platform: PlatformOpenAI, Type: AccountTypeOAuth})
			require.True(t, writer.checked, "must observe a real downstream failure write")

			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.Zero(t, repo.setErrorCalls)
			require.Zero(t, repo.tempCalls)
			require.NotNil(t, GetOpsCyberPolicy(c))
			require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.failed"`))
			require.Contains(t, recorder.Body.String(), `"id":"resp_cpa"`)
			require.Contains(t, recorder.Body.String(), `"code":"cyber_policy"`)
			require.Contains(t, recorder.Body.String(), `"message":"blocked by CPA"`)
			require.Equal(t, []string{"lineage_bind", "lineage_bind", "block_set", "downstream_write"}, combo.events)
		})
	}
}

func TestCPALegacyCyberWSHTTPBridgeEOFPreservesCodeAndSkipsAccountMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_cpa_ws\",\"status\":\"in_progress\"}}\n\n" +
		"event: error\ndata: " + cpaLegacyCyberEvent + "\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}}
	repo := &openAIAuthPolicyAccountRepo{}
	combo := &comboCacheAndStore{}
	svc := enabledCyberSessionTestService(combo)
	svc.cfg = &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc.httpUpstream = upstream
	svc.rateLimitService = &RateLimitService{accountRepo: repo}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	request := []byte(`{"type":"response.create","model":"gpt-5.6-sol","input":"hi"}`)
	BindCyberSessionIdentity(c, svc.PrepareCyberSessionIdentity(c.Request.Context(), 7, 11, c, request, "203.0.113.1", "client/1.0"))
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c,
		&Account{ID: 992, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1},
		"sk-test", request, len(request), "gpt-5.6-sol", "", "", "", "", 1,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			if gjson.GetBytes(message, "response.error.code").String() == "cyber_policy" {
				combo.events = append(combo.events, "downstream_write")
			}
			return nil
		},
	)

	require.EqualError(t, err, "blocked by CPA")
	require.NotNil(t, result)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.tempCalls)
	require.NotNil(t, GetOpsCyberPolicy(c))
	require.Len(t, writes, 2)
	require.Equal(t, "response.failed", gjson.GetBytes(writes[1], "type").String())
	require.Equal(t, "resp_cpa_ws", gjson.GetBytes(writes[1], "response.id").String())
	require.Equal(t, "cyber_policy", gjson.GetBytes(writes[1], "response.error.code").String())
	require.Equal(t, "blocked by CPA", gjson.GetBytes(writes[1], "response.error.message").String())
	require.Equal(t, []string{"lineage_bind", "lineage_bind", "block_set", "downstream_write"}, combo.events)
}

func TestCPALegacyCyberBareErrorWithoutCreatedBindsSyntheticResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stream := "event: error\ndata: " + cpaLegacyCyberEvent + "\n\n"
	combo := &comboCacheAndStore{}
	svc := enabledCyberSessionTestService(combo)
	svc.cfg = &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	requestBody := []byte(`{"input":"trigger"}`)
	identity := svc.PrepareCyberSessionIdentity(c.Request.Context(), 7, 11, c, requestBody, "203.0.113.1", "client/1.0")
	BindCyberSessionIdentity(c, identity)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(stream)),
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 993, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")

	require.Error(t, err)
	_, failedPayload, ok := extractOpenAISSETerminalEvent(recorder.Body.String())
	require.True(t, ok)
	responseID := gjson.GetBytes(failedPayload, "response.id").String()
	require.NotEmpty(t, responseID)
	require.Equal(t, identity.lineageRoot, combo.lineages[fakeCyberLineageKey(7, 11, responseID)])
	require.True(t, combo.store.blocked[identity.lineageRoot])
}

func TestPreviousResponseLineageBlocksContinuationAfterCPALegacyCyber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	combo := &comboCacheAndStore{}
	svc := enabledCyberSessionTestService(combo)
	svc.cfg = &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	account := &Account{ID: 994, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	firstBody := []byte(`{"input":"first turn"}`)
	firstRecorder := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRecorder)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	firstIdentity := svc.PrepareCyberSessionIdentity(firstCtx.Request.Context(), 7, 11, firstCtx, firstBody, "203.0.113.1", "client/1.0")
	BindCyberSessionIdentity(firstCtx, firstIdentity)
	firstStream := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		``,
	}, "\n")
	firstResp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(firstStream))}
	_, err := svc.handleStreamingResponse(firstCtx.Request.Context(), firstResp, firstCtx, account, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
	require.NoError(t, err)
	require.Equal(t, firstIdentity.lineageRoot, combo.lineages[fakeCyberLineageKey(7, 11, "resp_1")])
	require.False(t, combo.store.blocked[firstIdentity.lineageRoot])

	secondBody := []byte(`{"previous_response_id":"resp_1","input":"trigger"}`)
	secondRecorder := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRecorder)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	secondIdentity := svc.PrepareCyberSessionIdentity(secondCtx.Request.Context(), 7, 11, secondCtx, secondBody, "203.0.113.1", "client/1.0")
	require.Equal(t, firstIdentity.lineageRoot, secondIdentity.lineageRoot)
	BindCyberSessionIdentity(secondCtx, secondIdentity)
	secondStream := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_2","status":"in_progress"}}`,
		``,
		`event: error`,
		`data: ` + cpaLegacyCyberEvent,
		``,
	}, "\n")
	secondResp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(secondStream))}
	_, err = svc.handleStreamingResponse(secondCtx.Request.Context(), secondResp, secondCtx, account, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
	require.Error(t, err)
	require.Equal(t, firstIdentity.lineageRoot, combo.lineages[fakeCyberLineageKey(7, 11, "resp_2")])
	require.True(t, combo.store.blocked[firstIdentity.lineageRoot])

	for _, previousResponseID := range []string{"resp_1", "resp_2"} {
		t.Run(previousResponseID, func(t *testing.T) {
			body := []byte(`{"previous_response_id":"` + previousResponseID + `","input":"continue"}`)
			c, _ := newCyberBlockTestCtx(nil, string(body))
			identity := svc.PrepareCyberSessionIdentity(c.Request.Context(), 7, 11, c, body, "203.0.113.1", "client/1.0")
			BindCyberSessionIdentity(c, identity)
			require.Equal(t, firstIdentity.lineageRoot, svc.FindCyberSessionBlockedForRequest(c.Request.Context(), 11, c, body, "203.0.113.1", "client/1.0"))
		})
	}

	newCtx, newBody := newCyberBlockTestCtx(nil, `{"input":"new independent turn"}`)
	newIdentity := svc.PrepareCyberSessionIdentity(newCtx.Request.Context(), 7, 11, newCtx, newBody, "203.0.113.1", "client/1.0")
	BindCyberSessionIdentity(newCtx, newIdentity)
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(newCtx.Request.Context(), 11, newCtx, newBody, "203.0.113.1", "client/1.0"))
}
