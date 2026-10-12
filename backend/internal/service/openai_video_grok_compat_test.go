//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIVideoGrokCompatNativeCompletedUnchanged(t *testing.T) {
	body := []byte(`{ "id":"task-1", "status":"completed", "model":"provider-video", "seconds":"8", "size":"1280x720", "metadata":{"keep":true} }`)
	for _, protocol := range []string{"", "grok"} {
		t.Run(protocol, func(t *testing.T) {
			account := &Account{}
			if protocol != "" {
				account.Extra = map[string]any{"openai_video_protocol": protocol}
			}
			got := normalizeOpenAIVideoResponse(account, body)
			require.Equal(t, body, got)
			completion := openAIVideoCompletion(got, "task-1")
			require.NotNil(t, completion, "native completed requires no video.url")
			require.Equal(t, 1, completion.VideoCount)
			require.Equal(t, 8, completion.VideoDurationSeconds)
			usage := openAIVideoUsage(GrokMediaEndpointVideoStatus, GrokMediaRequestInfo{}, got, "task-1")
			require.Equal(t, 1, usage.VideoCount)
			require.Equal(t, 8, usage.VideoDurationSeconds)
			require.Equal(t, "720p", usage.VideoResolution)
		})
	}
}

func TestOpenAIVideoGrokCompatDoneWithoutVideoURLNotBillable(t *testing.T) {
	for _, protocol := range []string{"", "grok"} {
		for _, tc := range []struct {
			name  string
			extra string
		}{
			{"missing_video", ""},
			{"null_video", `,"video":null`},
			{"missing_url", `,"video":{}`},
			{"null_url", `,"video":{"url":null}`},
			{"empty_url", `,"video":{"url":""}`},
			{"blank_url", `,"video":{"url":" \t "}`},
			{"unrelated_url", `,"metadata":{"url":"https://video.example/clip.mp4"}`},
			{"invalid_flat_urls", `,"url":null,"video_url":" \t ","result_url":123,"download_url":{}`},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				account := &Account{}
				if protocol != "" {
					account.Extra = map[string]any{"openai_video_protocol": protocol}
				}
				body := []byte(`{"id":"task-1","status":"done","seconds":"8","size":"1280x720"` + tc.extra + `}`)
				got := normalizeOpenAIVideoResponse(account, body)
				require.Equal(t, body, got)
				require.Nil(t, openAIVideoCompletion(got, "task-1"))
				usage := openAIVideoUsage(GrokMediaEndpointVideoStatus, GrokMediaRequestInfo{
					DurationSeconds: 8, Resolution: "720p",
				}, got, "task-1")
				require.Equal(t, "task-1", usage.ResponseID)
				require.Zero(t, usage.VideoCount)
				require.Zero(t, usage.VideoDurationSeconds)
				require.Empty(t, usage.VideoResolution)
			})
		}
	}
}

func TestOpenAIVideoGrokStatusCompatibilityIsExplicit(t *testing.T) {
	for _, tc := range []struct {
		status string
		url    string
		want   string
	}{
		{"pending", "", "queued"},
		{"processing", "", "in_progress"},
		{"done", "https://video.example/video.mp4", "completed"},
		{"done", "", "done"},
		{"failed", "", "failed"},
	} {
		body := []byte(`{"id":"task-1","status":"` + tc.status + `","video":{"url":"` + tc.url + `"}}`)
		a := openAIVideoTestAccount()
		require.Equal(t, body, normalizeOpenAIVideoResponse(a, body))
		a.Extra = map[string]any{"openai_video_protocol": "grok"}
		got := normalizeOpenAIVideoResponse(a, body)
		require.Equal(t, tc.want, gjson.GetBytes(got, "status").String())
		if tc.want == "completed" {
			require.NotNil(t, openAIVideoCompletion(got, "task-1"))
		} else {
			require.Nil(t, openAIVideoCompletion(got, "task-1"))
		}
	}
}

func TestOpenAIVideoContentURLRewritePreservesUnrelatedFields(t *testing.T) {
	body := []byte(`{"id":"task-1","video":{"url":"https://signed.example/movie"},"video_url":"https://signed.example/movie","url":"https://signed.example/movie","result_url":"https://signed.example/movie","download_url":"https://signed.example/movie","metadata":{"url":"https://example.com"},"data":{"url":"https://example.com"}}`)
	got := rewriteOpenAIVideoContentURLs(body, "task-1", "/v1/videos/task-1/content")
	for _, path := range []string{"video.url", "video_url", "url", "result_url", "download_url"} {
		require.Equal(t, "/v1/videos/task-1/content", gjson.GetBytes(got, path).String())
	}
	for _, path := range []string{"metadata.url", "data.url"} {
		require.Equal(t, "https://example.com", gjson.GetBytes(got, path).String())
	}
}

func TestOpenAIVideoGrokFlatDoneAliasesAreExplicit(t *testing.T) {
	for _, alias := range []string{"url", "video_url", "result_url", "download_url"} {
		t.Run(alias, func(t *testing.T) {
			body := []byte(`{"id":"paid-task","status":"done","` + alias + `":"https://signed.example/clip.mp4?token=test"}`)
			account := openAIVideoTestAccount()
			require.Equal(t, body, normalizeOpenAIVideoResponse(account, body))
			account.Extra = map[string]any{"openai_video_protocol": "grok"}
			require.Equal(t, "completed", gjson.GetBytes(normalizeOpenAIVideoResponse(account, body), "status").String())
			require.False(t, IsGrokVideoStatusBillable(body), "native Grok billing must still require nested video.url")
		})
	}
}

func TestOpenAIVideoGrokFlatDoneStatusAndContent(t *testing.T) {
	// Real relay shape after restart: no nested video, duration, or resolution.
	const body = `{"id":"paid-task","task_id":"paid-task","status":"done","url":"https://signed.example/clip.mp4?token=test","video_url":"https://signed.example/clip.mp4?token=test","result_url":"https://signed.example/clip.mp4?token=test","download_url":"https://signed.example/clip.mp4?token=test"}`
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(body),
		grokMediaContentStatusResponse(body),
		{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-3/100"}}, Body: io.NopCloser(strings.NewReader("mp4!"))},
	}}
	// No billing/repository services are installed: forwarding must only read.
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := openAIVideoTestAccount()
	account.Extra = map[string]any{"openai_video_protocol": "grok"}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/paid-task", nil)
	_, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoStatus, "paid-task", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "completed", gjson.Get(recorder.Body.String(), "status").String())
	for _, alias := range []string{"url", "video_url", "result_url", "download_url"} {
		require.Equal(t, "/v1/videos/paid-task/content", gjson.Get(recorder.Body.String(), alias).String())
	}
	require.NotContains(t, recorder.Body.String(), "signed.example")
	require.NotContains(t, recorder.Body.String(), "token=test")
	c, recorder = grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/paid-task/content", map[string]string{"Range": "bytes=0-3"})
	_, err = svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoContent, "paid-task", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "mp4!", recorder.Body.String())
	require.Equal(t, "bytes 0-3/100", recorder.Header().Get("Content-Range"))
	require.Len(t, upstream.requests, 3)
	for _, request := range upstream.requests {
		require.Equal(t, http.MethodGet, request.Method, "poll/download must not generate another task")
		require.Equal(t, "relay.example", request.URL.Host, "never fetch signed aliases directly")
		require.Equal(t, "Bearer upstream-key", request.Header.Get("Authorization"))
	}
	require.Equal(t, "/v1/videos/paid-task/content", upstream.requests[2].URL.Path)
	require.Equal(t, "bytes=0-3", upstream.requests[2].Header.Get("Range"))
}

func TestOpenAIVideoGrokRelayGeometryValidation(t *testing.T) {
	account := openAIVideoTestAccount()
	account.Extra = map[string]any{"openai_video_protocol": "grok"}
	for _, tc := range []struct {
		name  string
		body  string
		valid bool
	}{
		{"landscape", `{"size":"16:9","quality":"480p","seconds":"6"}`, true},
		{"portrait", `{"size":"9:16","quality":"720p","resolution":"hd"}`, true},
		{"missing_quality", `{"size":"16:9"}`, false},
		{"unknown_quality", `{"size":"16:9","quality":"4k"}`, false},
		{"conflicting_resolution", `{"size":"16:9","quality":"480p","resolution":"720p"}`, false},
		{"conflicting_aspect", `{"size":"16:9","quality":"480p","aspect_ratio":"9:16"}`, false},
		{"duplicate_size", `{"size":"16:9","quality":"480p","size":"bad"}`, false},
		{"duplicate_quality", `{"size":"16:9","quality":"480p","quality":"720p"}`, false},
		{"duplicate_resolution", `{"size":"16:9","quality":"480p","resolution":"480p","resolution":"720p"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			got := normalizeOpenAIVideoResponse(account, body)
			err := validateOpenAIVideoResponseBilling(got, account)
			if tc.valid {
				require.NoError(t, err)
				require.False(t, gjson.GetBytes(got, "size").Exists())
				require.Equal(t, gjson.GetBytes(body, "size").String(), gjson.GetBytes(got, "aspect_ratio").String())
			} else {
				require.Error(t, err)
			}
			require.Error(t, validateOpenAIVideoOriginalResolution(body, "application/json"), "client requests remain strict")
			require.Equal(t, body, normalizeOpenAIVideoResponse(openAIVideoTestAccount(), body), "native responses remain unchanged")
		})
	}
}

func TestOpenAIVideoGrokRelayCreatePollAndContent(t *testing.T) {
	const accepted = `{"id":"relay-task","status":"queued","model":"grok-imagine-video-1.5","seconds":"6","size":"16:9","quality":"480p"}`
	const completed = `{"id":"relay-task","status":"completed","model":"grok-imagine-video-1.5","seconds":"6","size":"16:9","quality":"480p","metadata":{"video_url":"https://signed.example/clip.mp4?token=secret","keep":true}}`
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(accepted),
		grokMediaContentStatusResponse(completed),
		grokMediaContentStatusResponse(completed),
		{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-3/100"}}, Body: io.NopCloser(strings.NewReader("mp4!"))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := openAIVideoTestAccount()
	account.Extra = map[string]any{"openai_video_protocol": "grok"}
	c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	saved := 0
	_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, account,
		[]byte(`{"model":"grok-imagine-video-1.5","prompt":"waves","seconds":"6","resolution":"480p","aspect_ratio":"16:9"}`), "application/json",
		func(result *OpenAIForwardResult) error {
			saved++
			require.False(t, c.Writer.Written())
			require.Equal(t, "relay-task", result.ResponseID)
			require.Equal(t, "480p", result.VideoResolution)
			require.Equal(t, 6, result.VideoDurationSeconds)
			require.Zero(t, result.VideoCount)
			return nil
		})
	require.NoError(t, err)
	require.Equal(t, 1, saved)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "relay-task", gjson.Get(recorder.Body.String(), "id").String())

	c, recorder = grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/relay-task", nil)
	result, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoStatus, "relay-task", nil, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, 6, result.VideoDurationSeconds)
	require.Equal(t, "480p", result.VideoResolution)
	require.Equal(t, "/v1/videos/relay-task/content", gjson.Get(recorder.Body.String(), "video_url").String())
	require.True(t, gjson.Get(recorder.Body.String(), "metadata.keep").Bool())
	require.NotContains(t, recorder.Body.String(), "signed.example")
	require.NotContains(t, recorder.Body.String(), "token=secret")

	c, recorder = grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/relay-task/content", map[string]string{"Range": "bytes=0-3"})
	_, err = svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoContent, "relay-task", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "mp4!", recorder.Body.String())
	require.Len(t, upstream.requests, 4)
	require.Equal(t, "/v1/videos/relay-task/content", upstream.requests[3].URL.Path)
	for _, req := range upstream.requests[1:] {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "relay.example", req.URL.Host)
	}
}
