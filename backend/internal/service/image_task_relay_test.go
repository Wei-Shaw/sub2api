package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type imageTaskRelayHTTPStub struct {
	responses []*http.Response
	requests  []*http.Request
}

func (s *imageTaskRelayHTTPStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.requests = append(s.requests, req)
	if len(s.responses) == 0 {
		return nil, io.EOF
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

func (s *imageTaskRelayHTTPStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func imageTaskRelayResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestPollImageTaskRelayPollsUntilCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &imageTaskRelayHTTPStub{responses: []*http.Response{
		imageTaskRelayResponse(http.StatusOK, `{"status":"processing","task_id":"up-task-1"}`),
		imageTaskRelayResponse(http.StatusOK, `{"status":"completed","result":{"data":[{"url":"https://upstream.test/a.png"}]}}`),
	}}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	account := &Account{
		ID:       7,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			imageAsyncTaskRelayEnabledKey: true,
			imageAsyncTaskStatusURLKey:    "/v1/images/status/{task_id}",
			imageAsyncTaskPollIntervalKey: "1",
		},
	}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(nil))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := svc.pollImageTaskRelay(
		ctx,
		ginContext,
		account,
		"https://relay.test",
		http.Header{"Authorization": []string{"Bearer secret"}},
		[]byte(`{"task_id":"up-task-1","status":"queued"}`),
	)
	require.NoError(t, err)
	require.True(t, result.relayed)
	require.JSONEq(t, `{"data":[{"url":"https://upstream.test/a.png"}]}`, string(result.body))
	require.Len(t, stub.requests, 2)
	require.Equal(t, http.MethodGet, stub.requests[0].Method)
	require.Equal(t, "https://relay.test/v1/images/status/up-task-1", stub.requests[0].URL.String())
	require.Equal(t, "Bearer secret", stub.requests[0].Header.Get("Authorization"))
}

func TestPollImageTaskRelayLeavesSynchronousImageResponseUntouched(t *testing.T) {
	stub := &imageTaskRelayHTTPStub{}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	initial := []byte(`{"created":123,"data":[{"b64_json":"aW1hZ2U="}]}`)

	result, err := svc.pollImageTaskRelay(
		context.Background(),
		nil,
		account,
		"https://relay.test",
		nil,
		initial,
	)
	require.NoError(t, err)
	require.True(t, result.relayed)
	require.Equal(t, string(initial), string(result.body))
	require.Empty(t, stub.requests)
}

func TestImageTaskRelayAccountSettings(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{
		imageAsyncTaskRelayEnabledKey: true,
		imageAsyncTaskPollIntervalKey: 90,
	}}
	require.True(t, account.AsyncImageTaskRelayEnabled())
	require.True(t, account.AsyncImageTaskUpstreamAsyncEnabled())
	require.Equal(t, 60*time.Second, account.AsyncImageTaskPollInterval())
	require.False(t, (&Account{Platform: PlatformOpenAI}).AsyncImageTaskRelayEnabled())
	require.True(t, (&Account{Platform: PlatformOpenAI}).AsyncImageTaskUpstreamAsyncEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Extra: map[string]any{
		imageAsyncTaskUpstreamAsyncKey: false,
	}}).AsyncImageTaskUpstreamAsyncEnabled())
}

func TestPollImageTaskRelayPrefersUpstreamPollURL(t *testing.T) {
	stub := &imageTaskRelayHTTPStub{responses: []*http.Response{
		imageTaskRelayResponse(http.StatusOK, `{"status":"completed","data":[{"url":"https://upstream.test/a.png"}]}`),
	}}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{
		imageAsyncTaskPollIntervalKey: 1,
	}}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(nil))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := svc.pollImageTaskRelay(
		ctx,
		ginContext,
		account,
		"https://relay.test",
		http.Header{"Authorization": []string{"Bearer secret"}},
		[]byte(`{"task_id":"up-task-1","status":"queued","poll_url":"/v1/image-tasks/{task_id}"}`),
	)
	require.NoError(t, err)
	require.True(t, result.relayed)
	require.Len(t, stub.requests, 1)
	require.Equal(t, "https://relay.test/v1/image-tasks/up-task-1", stub.requests[0].URL.String())
}

func TestImageTaskRelayAcceptsCommonQueuedStatuses(t *testing.T) {
	for _, status := range []string{"accepted", "submitted", "scheduled", "in_queue", "waiting", "started", "generating", "not_started"} {
		require.True(t, imageTaskRelayPending(status), status)
	}
	require.False(t, imageTaskRelayPending("completed"))
}

func TestImageTaskRelayFindsNestedTaskID(t *testing.T) {
	for _, body := range []string{
		`{"data":{"id":"data-task"},"status":"submitted"}`,
		`{"result":{"task_id":"result-task"},"state":"queued"}`,
	} {
		taskID, ok := imageTaskRelayID([]byte(body))
		require.True(t, ok)
		require.NotEmpty(t, taskID)
	}
}

func TestNormalizeImageTaskRelayResultRemovesUpstreamTaskMetadata(t *testing.T) {
	result, ok := normalizeImageTaskRelayResult([]byte(`{"id":"up-task-1","status":"completed","data":[{"url":"https://upstream.test/a.png"}]}`))
	require.True(t, ok)
	require.JSONEq(t, `{"data":[{"url":"https://upstream.test/a.png"}]}`, string(result))
}

func TestNormalizeImageTaskRelayResultWrapsOutputArray(t *testing.T) {
	result, ok := normalizeImageTaskRelayResult([]byte(`{"status":"completed","output":[{"image_url":"https://upstream.test/a.png"}]}`))
	require.True(t, ok)
	require.JSONEq(t, `{"created":`+strings.TrimSpace(gjson.GetBytes(result, "created").String())+`,"data":[{"url":"https://upstream.test/a.png"}]}`, string(result))
}

func TestMarkOpenAIImagesRequestAsync(t *testing.T) {
	rewritten, contentType, err := markOpenAIImagesRequestAsync(
		[]byte(`{"model":"gpt-image-2","async":false}`),
		"application/json",
	)
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	require.JSONEq(t, `{"model":"gpt-image-2","async":true}`, string(rewritten))
}

func TestMarkOpenAIImagesRequestAsyncLeavesMultipartUntouched(t *testing.T) {
	body := []byte("multipart body")
	rewritten, contentType, err := markOpenAIImagesRequestAsync(body, "multipart/form-data; boundary=test")
	require.NoError(t, err)
	require.Equal(t, string(body), string(rewritten))
	require.Equal(t, "multipart/form-data; boundary=test", contentType)
}
