//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func bailianVideoTestAccount() *Account {
	return &Account{
		ID:       11,
		Platform: PlatformBailian,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "dashscope-key",
		},
	}
}

func bailianVideoTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, nil)
	return c, recorder
}

func bailianVideoJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestForwardBailianVideoGenerationCreatesTask(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		response: bailianVideoJSONResponse(http.StatusOK,
			`{"output":{"task_status":"PENDING","task_id":"task-abc"},"request_id":"req-1"}`),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := bailianVideoTestContext(http.MethodPost, "https://api.example/v1/videos/generations")

	clientBody := []byte(`{"model":"wan2.7-t2v","prompt":"a cat","duration":10,"resolution":"1080p"}`)
	result, err := svc.ForwardBailianVideo(
		context.Background(), c, bailianVideoTestAccount(),
		BailianVideoEndpointGeneration, "", clientBody,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "task-abc", result.ResponseID)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, VideoBillingResolution1080P, result.VideoResolution)
	require.Equal(t, 10, result.VideoDurationSeconds)
	require.Equal(t, "wan2.7-t2v", result.Model)

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, http.MethodPost, req.Method)
	require.Equal(t, "https://dashscope.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis", req.URL.String())
	require.Equal(t, "Bearer dashscope-key", req.Header.Get("Authorization"))
	require.Equal(t, "enable", req.Header.Get("X-DashScope-Async"))
	require.Equal(t, "application/json", req.Header.Get("Content-Type"))

	sentBody, readErr := io.ReadAll(req.Body)
	require.NoError(t, readErr)
	require.Equal(t, "wan2.7-t2v", gjson.GetBytes(sentBody, "model").String())
	require.Equal(t, "a cat", gjson.GetBytes(sentBody, "input.prompt").String())
	require.Equal(t, "1080P", gjson.GetBytes(sentBody, "parameters.resolution").String())
	require.EqualValues(t, 10, gjson.GetBytes(sentBody, "parameters.duration").Int())

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "task-abc", gjson.Get(recorder.Body.String(), "output.task_id").String())
}

func TestForwardBailianVideoGenerationUsesAccountBaseURL(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		response: bailianVideoJSONResponse(http.StatusOK,
			`{"output":{"task_status":"PENDING","task_id":"t1"}}`),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, _ := bailianVideoTestContext(http.MethodPost, "https://api.example/v1/videos/generations")

	account := bailianVideoTestAccount()
	account.Credentials["base_url"] = "https://ws-1.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"

	_, err := svc.ForwardBailianVideo(
		context.Background(), c, account,
		BailianVideoEndpointGeneration, "", []byte(`{"model":"happyhorse-1.1-t2v","prompt":"x"}`),
	)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	// The pasted compatible-mode suffix must be stripped before the native path.
	require.Equal(t,
		"https://ws-1.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis",
		upstream.requests[0].URL.String(),
	)
}

func TestForwardBailianVideoStatusPassesThroughVerbatim(t *testing.T) {
	statusBody := `{"output":{"task_id":"task-abc","task_status":"SUCCEEDED","video_url":"https://oss.example/video.mp4?sig=1"},"usage":{"duration":10,"SR":1080,"video_count":1},"request_id":"req-2"}`
	upstream := &grokMediaContentUpstreamStub{
		response: bailianVideoJSONResponse(http.StatusOK, statusBody),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := bailianVideoTestContext(http.MethodGet, "https://api.example/v1/videos/task-abc")

	result, err := svc.ForwardBailianVideo(
		context.Background(), c, bailianVideoTestAccount(),
		BailianVideoEndpointStatus, "task-abc", nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	// Lookups never bill: no video usage fields on the result.
	require.Zero(t, result.VideoCount)
	require.Empty(t, result.ResponseID)

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, http.MethodGet, req.Method)
	require.Equal(t, "https://dashscope.aliyuncs.com/api/v1/tasks/task-abc", req.URL.String())
	require.Empty(t, req.Header.Get("X-DashScope-Async"))

	// The signed OSS URL reaches the client untouched.
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, statusBody, recorder.Body.String())
}

func TestForwardBailianVideoGeneration400PassesThroughWithoutFailover(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		response: bailianVideoJSONResponse(http.StatusBadRequest,
			`{"code":"InvalidParameter","message":"duration is out of range","request_id":"r"}`),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := bailianVideoTestContext(http.MethodPost, "https://api.example/v1/videos/generations")

	_, err := svc.ForwardBailianVideo(
		context.Background(), c, bailianVideoTestAccount(),
		BailianVideoEndpointGeneration, "", []byte(`{"model":"wan2.7-t2v","prompt":"x","duration":99}`),
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "InvalidParameter")
}

func TestForwardBailianVideoGeneration429Failover(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		response: bailianVideoJSONResponse(http.StatusTooManyRequests,
			`{"code":"Throttling.RateQuota","message":"rate limited"}`),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := bailianVideoTestContext(http.MethodPost, "https://api.example/v1/videos/generations")

	_, err := svc.ForwardBailianVideo(
		context.Background(), c, bailianVideoTestAccount(),
		BailianVideoEndpointGeneration, "", []byte(`{"model":"wan2.7-t2v","prompt":"x"}`),
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	// Failover errors must not write a response: the handler retries elsewhere.
	require.Zero(t, recorder.Body.Len())
}

func TestForwardBailianVideoStatusUpstreamErrorNeverFailsOver(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		response: bailianVideoJSONResponse(http.StatusInternalServerError,
			`{"code":"InternalError","message":"boom"}`),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := bailianVideoTestContext(http.MethodGet, "https://api.example/v1/videos/task-abc")

	_, err := svc.ForwardBailianVideo(
		context.Background(), c, bailianVideoTestAccount(),
		BailianVideoEndpointStatus, "task-abc", nil,
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}
