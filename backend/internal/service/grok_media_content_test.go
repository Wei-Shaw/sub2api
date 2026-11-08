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
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type grokMediaContentUpstreamStub struct {
	request   *http.Request
	requests  []*http.Request
	response  *http.Response
	responses []*http.Response
}

func (s *grokMediaContentUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = req
	s.requests = append(s.requests, req)
	if len(s.responses) > 0 {
		resp := s.responses[0]
		s.responses = s.responses[1:]
		return resp, nil
	}
	return s.response, nil
}

func (s *grokMediaContentUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func grokMediaContentTestAccount() *Account {
	return &Account{
		ID:       9,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://relay.example/v1",
		},
	}
}

func grokMediaContentTestContext(method, target string, headers map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		c.Request.Header.Set(name, value)
	}
	return c, recorder
}

func grokMediaContentStatusResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestForwardGrokMediaContentUsesUpstreamCredentialAndStreamsRange(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		responses: []*http.Response{grokMediaContentStatusResponse(`{"status":"completed"}`), {
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Type":   []string{"video/mp4"},
				"Content-Length": []string{"13"},
				"Content-Range":  []string{"bytes 0-12/100"},
				"Accept-Ranges":  []string{"bytes"},
				"Content-Disposition": []string{
					`attachment; filename="task-1.mp4"`,
				},
			},
			Body: io.NopCloser(strings.NewReader("video-payload")),
		}},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", map[string]string{
		"Range": "bytes=0-12",
	})

	result, err := svc.ForwardGrokMedia(
		context.Background(), c, grokMediaContentTestAccount(),
		GrokMediaEndpointVideoContent, "task-1", nil, "",
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "video-payload", recorder.Body.String())
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://relay.example/v1/videos/task-1", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "https://relay.example/v1/videos/task-1/content", upstream.requests[1].URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, "bytes=0-12", upstream.requests[1].Header.Get("Range"))
	require.Equal(t, "*/*", upstream.requests[1].Header.Get("Accept"))
	require.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
	require.Equal(t, "13", recorder.Header().Get("Content-Length"))
	require.Equal(t, "bytes 0-12/100", recorder.Header().Get("Content-Range"))
	require.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	require.Equal(t, `attachment; filename="task-1.mp4"`, recorder.Header().Get("Content-Disposition"))
	require.True(t, IsResponseCommitted(c))
}

func TestForwardGrokMediaContentStreamsFullResponseWithSafeDefaults(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		responses: []*http.Response{grokMediaContentStatusResponse(`{"status":"completed"}`), {
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Set-Cookie": []string{"secret=upstream"}, "X-Upstream-Secret": []string{"hidden"}},
			Body:          io.NopCloser(strings.NewReader("full-video")),
			ContentLength: -1,
		}},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)

	_, err := svc.ForwardGrokMedia(
		context.Background(), c, grokMediaContentTestAccount(),
		GrokMediaEndpointVideoContent, "task-1", nil, "",
	)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "full-video", recorder.Body.String())
	require.Len(t, upstream.requests, 2)
	require.Empty(t, upstream.requests[1].Header.Get("Range"))
	require.Equal(t, "application/octet-stream", recorder.Header().Get("Content-Type"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
	require.Empty(t, recorder.Header().Get("Set-Cookie"))
	require.Empty(t, recorder.Header().Get("X-Upstream-Secret"))
	require.True(t, IsResponseCommitted(c))
}

func TestForwardGrokMediaContentPreservesRangeNotSatisfiable(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		responses: []*http.Response{grokMediaContentStatusResponse(`{"status":"completed"}`), {
			StatusCode: http.StatusRequestedRangeNotSatisfiable,
			Header: http.Header{
				"Content-Type":   []string{"text/plain"},
				"Content-Length": []string{"11"},
				"Content-Range":  []string{"bytes */100"},
				"Accept-Ranges":  []string{"bytes"},
			},
			Body: io.NopCloser(strings.NewReader("bad-range!!")),
		}},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", map[string]string{
		"Range": "bytes=500-600",
	})

	_, err := svc.ForwardGrokMedia(
		context.Background(), c, grokMediaContentTestAccount(),
		GrokMediaEndpointVideoContent, "task-1", nil, "",
	)

	require.NoError(t, err)
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, recorder.Code)
	require.Equal(t, "bad-range!!", recorder.Body.String())
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "bytes=500-600", upstream.requests[1].Header.Get("Range"))
	require.Equal(t, "bytes */100", recorder.Header().Get("Content-Range"))
	require.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	require.True(t, IsResponseCommitted(c))
}

func TestForwardGrokMediaContentFetchesValidatedSignedURLWithoutCredentials(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		responses: []*http.Response{
			grokMediaContentStatusResponse(`{"status":"done","video":{"url":"https://vidgen.x.ai/signed-token/xai-video-task-1.mp4"}}`),
			{
				StatusCode: http.StatusPartialContent,
				Header: http.Header{
					"Content-Type":   []string{"video/mp4"},
					"Content-Length": []string{"13"},
					"Content-Range":  []string{"bytes 0-12/100"},
				},
				Body: io.NopCloser(strings.NewReader("video-payload")),
			},
		},
	}
	account := grokMediaContentTestAccount()
	account.Credentials[credKeyHeaderOverrideEnabled] = true
	account.Credentials[credKeyHeaderOverrides] = map[string]any{"user-agent": "private-agent"}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", map[string]string{
		"Range": "bytes=0-12",
	})

	_, err := svc.ForwardGrokMedia(
		context.Background(), c, account,
		GrokMediaEndpointVideoContent, "task-1", nil, "",
	)

	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "video-payload", recorder.Body.String())
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://relay.example/v1/videos/task-1", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "private-agent", upstream.requests[0].Header.Get("User-Agent"))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.requests[0].Context()))
	require.Equal(t, "https://vidgen.x.ai/signed-token/xai-video-task-1.mp4", upstream.requests[1].URL.String())
	require.Empty(t, upstream.requests[1].Header.Get("Authorization"))
	require.Empty(t, upstream.requests[1].Header.Get("User-Agent"))
	require.Equal(t, "bytes=0-12", upstream.requests[1].Header.Get("Range"))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.requests[1].Context()))
}

func TestForwardGrokMediaContentFollowsAuthenticatedSub2APIRelay(t *testing.T) {
	for _, statusURL := range []string{
		`/v1/videos/task-1/content`,
		`https://relay.example/v1/videos/task-1/content`,
	} {
		t.Run(statusURL, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{
				responses: []*http.Response{
					grokMediaContentStatusResponse(`{"status":"completed","video":{"url":"` + statusURL + `"}}`),
					{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"video/mp4"}},
						Body:       io.NopCloser(strings.NewReader("video-payload")),
					},
				},
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)

			_, err := svc.ForwardGrokMedia(
				context.Background(), c, grokMediaContentTestAccount(),
				GrokMediaEndpointVideoContent, "task-1", nil, "",
			)

			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "video-payload", recorder.Body.String())
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "https://relay.example/v1/videos/task-1/content", upstream.requests[1].URL.String())
			require.Equal(t, "Bearer upstream-key", upstream.requests[1].Header.Get("Authorization"))
		})
	}
}

func TestForwardGrokMediaContentRejectsUntrustedSignedURL(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		responses: []*http.Response{
			grokMediaContentStatusResponse(`{"status":"done","video":{"url":"http://169.` + `254.169.254/latest/meta-data"}}`),
		},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, _ := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)

	_, err := svc.ForwardGrokMedia(
		context.Background(), c, grokMediaContentTestAccount(),
		GrokMediaEndpointVideoContent, "task-1", nil, "",
	)

	require.ErrorContains(t, err, "unsupported video content URL")
	require.Len(t, upstream.requests, 1)
}

func TestGrokMediaSignedVideoContentURLRejectsDeceptiveOrigins(t *testing.T) {
	for _, rawURL := range []string{
		"https://vidgen.x.ai.attacker.invalid/video.mp4",
		"https://vidgen.x.ai" + "@attacker.invalid/video.mp4",
		"https://vidgen.x.ai:444/video.mp4",
		"http://vidgen.x.ai/video.mp4",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := grokMediaSignedVideoContentURL([]byte(`{"video":{"url":"`+rawURL+`"}}`), "task-1")
			require.ErrorContains(t, err, "unsupported video content URL")
		})
	}
}

func TestGrokMediaSignedVideoContentURLRejectsDifferentRelayTask(t *testing.T) {
	_, err := grokMediaSignedVideoContentURL(
		[]byte(`{"video":{"url":"/v1/videos/task-2/content"}}`),
		"task-1",
	)

	require.ErrorContains(t, err, "unsupported video content URL")
}

func TestForwardGrokVideoStatusRewritesOnlyProtectedContentURL(t *testing.T) {
	statusBody := `{"id":"task-1","status":"completed","url":"https://relay.example/v1/videos/task-1/content","download_url":"/v1/videos/task-1/content","video_url":"https://vidgen.x.ai/task-1.mp4","counter":9007199254740993}`
	upstream := &grokMediaContentUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(statusBody)),
		},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1", map[string]string{
		"X-Forwarded-Host":  "malicious.invalid",
		"X-Forwarded-Proto": "https",
	})

	_, err := svc.ForwardGrokMedia(
		context.Background(), c, grokMediaContentTestAccount(),
		GrokMediaEndpointVideoStatus, "task-1", nil, "",
	)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "/v1/videos/task-1/content", gjson.Get(recorder.Body.String(), "url").String())
	require.Equal(t, "/v1/videos/task-1/content", gjson.Get(recorder.Body.String(), "download_url").String())
	require.Equal(t, "https://vidgen.x.ai/task-1.mp4", gjson.Get(recorder.Body.String(), "video_url").String())
	require.Equal(t, "9007199254740993", gjson.Get(recorder.Body.String(), "counter").String())
	require.NotContains(t, recorder.Body.String(), "malicious.invalid")
}

func TestRewriteGrokMediaVideoContentURLsPreservesOtherIDsAndHandlesNestedEscapedID(t *testing.T) {
	body := []byte(`{"nested":[{"url":"https://relay.example/v1/videos/task%2Fone/content"},{"url":"https://relay.example/v1/videos/task-two/content"}]}`)

	rewritten := rewriteGrokMediaVideoContentURLs(body, "task/one", "/v1/videos/task%2Fone/content")

	require.Equal(t, "/v1/videos/task%2Fone/content", gjson.GetBytes(rewritten, "nested.0.url").String())
	require.Equal(t, "https://relay.example/v1/videos/task-two/content", gjson.GetBytes(rewritten, "nested.1.url").String())
}

func TestRewriteGrokMediaVideoContentURLsRewritesSignedVideoURL(t *testing.T) {
	body := []byte(`{"status":"done","video":{"url":"https://vidgen.x.ai/signed-token/xai-video-request-1.mp4","duration":8}}`)

	rewritten := rewriteGrokMediaVideoContentURLs(body, "request-1", "/v1/videos/request-1/content")

	require.Equal(t, "/v1/videos/request-1/content", gjson.GetBytes(rewritten, "video.url").String())
	require.Equal(t, "8", gjson.GetBytes(rewritten, "video.duration").String())
	require.Equal(t, "done", gjson.GetBytes(rewritten, "status").String())
}
