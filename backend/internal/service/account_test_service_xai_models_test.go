package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type xaiModelsHTTPUpstreamStub struct {
	requests  []*http.Request
	response  *http.Response
	responses []*http.Response
}

func (s *xaiModelsHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.requests = append(s.requests, req)
	if len(s.responses) > 0 {
		resp := s.responses[0]
		s.responses = s.responses[1:]
		return resp, nil
	}
	return s.response, nil
}

func (s *xaiModelsHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}

func TestAccountTestServiceFetchXAIAvailableModels(t *testing.T) {
	upstream := &xaiModelsHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"object": "list",
				"data": [
					{"id": "grok-code-fast-1", "display_name": "Grok Code Fast 1"},
					{"id": "grok-4.3-fast", "display_name": "Grok 4.3 Fast"}
				]
			}`)),
			Header: make(http.Header),
		},
	}
	svc := NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}},
		nil,
	)

	models, err := svc.FetchXAIAvailableModels(context.Background(), &Account{
		ID:       9,
		Platform: PlatformXAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "xai-key",
			"base_url": "http://example.test/v1/responses",
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.3-fast", "grok-code-fast-1"}, []string{models[0].ID, models[1].ID})
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "GET", upstream.requests[0].Method)
	require.Equal(t, "http://example.test/v1/models", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer xai-key", upstream.requests[0].Header.Get("Authorization"))
}

func TestAccountTestServiceXAIImageGenerationUsesImagesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &xaiModelsHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"data": [
					{"b64_json": "QUJD", "revised_prompt": "draw a small robot"}
				]
			}`)),
			Header: make(http.Header),
		},
	}
	svc := NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{},
		nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/9/test", nil)

	err := svc.testXAIAccountConnection(c, &Account{
		ID:       9,
		Platform: PlatformXAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "xai-key",
			"base_url": "https://api.x.ai/v1/responses",
		},
	}, "grok-imagine-image", "draw a small robot")

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, http.MethodPost, req.Method)
	require.Equal(t, "https://api.x.ai/v1/images/generations", req.URL.String())
	require.Equal(t, "Bearer xai-key", req.Header.Get("Authorization"))
	require.Equal(t, "application/json", req.Header.Get("Accept"))

	bodyBytes, readErr := io.ReadAll(req.Body)
	require.NoError(t, readErr)
	var body map[string]any
	require.NoError(t, json.Unmarshal(bodyBytes, &body))
	require.Equal(t, "grok-imagine-image", body["model"])
	require.Equal(t, "draw a small robot", body["prompt"])
	require.Equal(t, "b64_json", body["response_format"])
	require.EqualValues(t, 1, body["n"])

	output := recorder.Body.String()
	require.Contains(t, output, "data:image/png;base64,QUJD")
	require.Contains(t, output, `"success":true`)
}

func TestAccountTestServiceXAIVideoGenerationCreatesAndPollsTask(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &xaiModelsHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"request_id":"video-task-1"}`)),
				Header:     make(http.Header),
			},
			{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"status": "completed",
					"video": {"url": "https://cdn.example.test/video-task-1.mp4"}
				}`)),
				Header: make(http.Header),
			},
			{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`video-bytes`)),
				Header:     http.Header{"Content-Type": []string{"video/mp4"}},
			},
		},
	}
	svc := NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{},
		nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/9/test", nil)

	err := svc.testXAIAccountConnection(c, &Account{
		ID:       9,
		Platform: PlatformXAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "xai-key",
			"base_url": "https://api.x.ai/v1",
		},
	}, "grok-imagine-video", "make a video")

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)

	createReq := upstream.requests[0]
	require.Equal(t, http.MethodPost, createReq.Method)
	require.Equal(t, "https://api.x.ai/v1/videos/generations", createReq.URL.String())
	require.Equal(t, "Bearer xai-key", createReq.Header.Get("Authorization"))
	require.Equal(t, "application/json", createReq.Header.Get("Accept"))

	bodyBytes, readErr := io.ReadAll(createReq.Body)
	require.NoError(t, readErr)
	var body map[string]any
	require.NoError(t, json.Unmarshal(bodyBytes, &body))
	require.Equal(t, "grok-imagine-video", body["model"])
	require.Equal(t, "make a video", body["prompt"])
	require.EqualValues(t, 2, body["duration"])

	pollReq := upstream.requests[1]
	require.Equal(t, http.MethodGet, pollReq.Method)
	require.Equal(t, "https://api.x.ai/v1/videos/video-task-1", pollReq.URL.String())
	require.Equal(t, "Bearer xai-key", pollReq.Header.Get("Authorization"))
	require.Equal(t, "application/json", pollReq.Header.Get("Accept"))

	mediaReq := upstream.requests[2]
	require.Equal(t, http.MethodGet, mediaReq.Method)
	require.Equal(t, "https://cdn.example.test/video-task-1.mp4", mediaReq.URL.String())
	require.Empty(t, mediaReq.Header.Get("Authorization"))
	require.Contains(t, mediaReq.Header.Get("Accept"), "video/*")

	output := recorder.Body.String()
	require.Contains(t, output, `"video_url":"data:video/mp4;base64,dmlkZW8tYnl0ZXM="`)
	require.Contains(t, output, `"source_url":"https://cdn.example.test/video-task-1.mp4"`)
	require.Contains(t, output, `"success":true`)
}

func TestAccountTestServiceXAIVideoPreviewUsesImageToVideoPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &xaiModelsHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"request_id":"video-task-1"}`)),
				Header:     make(http.Header),
			},
			{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"status": "completed",
					"video": {"url": "https://cdn.example.test/video-task-1.mp4"}
				}`)),
				Header: make(http.Header),
			},
			{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`video-bytes`)),
				Header:     http.Header{"Content-Type": []string{"video/mp4"}},
			},
		},
	}
	svc := NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{},
		nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/9/test", nil)

	err := svc.testXAIAccountConnection(c, &Account{
		ID:       9,
		Platform: PlatformXAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "xai-key",
			"base_url": "https://api.x.ai/v1",
		},
	}, "grok-imagine-video-1.5-preview", "")

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)

	bodyBytes, readErr := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, readErr)
	var body map[string]any
	require.NoError(t, json.Unmarshal(bodyBytes, &body))
	require.Equal(t, "grok-imagine-video-1.5-preview", body["model"])
	require.Equal(t, defaultXAIImageToVideoPrompt, body["prompt"])
	require.Equal(t, xaiVideoTestResolution, body["resolution"])
	require.EqualValues(t, 2, body["duration"])

	image, ok := body["image"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, defaultXAIImageToVideoURL, image["url"])
	require.Equal(t, "image_url", image["type"])
}

func TestAccountTestServiceXAIVideoPollAcceptsPendingAcceptedResponse(t *testing.T) {
	upstream := &xaiModelsHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader(`{"status":"pending","progress":0}`)),
			Header:     make(http.Header),
		},
	}
	svc := NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{},
		nil,
	)

	result, err := svc.pollXAIVideoTask(context.Background(), &Account{
		ID:       9,
		Platform: PlatformXAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "xai-key",
			"base_url": "https://api.x.ai/v1",
		},
	}, "xai-key", "https://api.x.ai/v1", "video-task-1")

	require.NoError(t, err)
	require.Equal(t, "pending", result.Status)
	require.Equal(t, "0", result.Progress)
	require.Empty(t, result.VideoURL)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.x.ai/v1/videos/video-task-1", upstream.requests[0].URL.String())
}

func TestFormatXAIVideoHTTPErrorExplainsCreditsAndSubscription(t *testing.T) {
	message := formatXAIVideoHTTPError(http.StatusForbidden, []byte(`{
		"code": "The caller does not have permission to execute the specified operation",
		"error": "You have run out of credits or need a Grok subscription. [WKE=personal-team-blocked:spending-limit]"
	}`))

	require.Contains(t, message, "xAI 视频生成权限不足或额度用尽")
	require.Contains(t, message, "增加 credits")
	require.Contains(t, message, "SuperGrok")
}

func TestFormatXAIImageHTTPErrorExplainsCreditsAndSubscription(t *testing.T) {
	message := formatXAIImageHTTPError(http.StatusForbidden, []byte(`{
		"code": "The caller does not have permission to execute the specified operation",
		"error": "You have run out of credits or need a Grok subscription. [WKE=personal-team-blocked:spending-limit]"
	}`))

	require.Contains(t, message, "xAI 图片生成权限不足或额度用尽")
	require.Contains(t, message, "增加 credits")
	require.Contains(t, message, "SuperGrok")
}
