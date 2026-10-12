//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openAIVideoTestAccount() *Account {
	a := grokMediaContentTestAccount()
	a.Platform = PlatformOpenAI
	return a
}

func TestOpenAIVideoCapability(t *testing.T) {
	a := openAIVideoTestAccount()
	require.True(t, a.SupportsOpenAIEndpointCapability(OpenAIEndpointCapability("videos")))
	require.False(t, a.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityGrokMediaGeneration))
	a.Type = AccountTypeOAuth
	require.False(t, a.SupportsOpenAIEndpointCapability(OpenAIEndpointCapability("videos")))
	a.Platform = PlatformGrok
	require.False(t, a.SupportsOpenAIEndpointCapability(OpenAIEndpointCapability("videos")))
}

func TestOpenAIVideoURL(t *testing.T) {
	for _, base := range []string{"https://relay.example", "https://relay.example/v1", "https://relay.example/v1/"} {
		a := openAIVideoTestAccount()
		a.Credentials["base_url"] = base
		for _, tc := range []struct {
			endpoint GrokMediaEndpoint
			id, path string
		}{
			{GrokMediaEndpointVideosGenerations, "", "/v1/videos"},
			{GrokMediaEndpointVideoStatus, "task-1", "/v1/videos/task-1"},
			{GrokMediaEndpointVideoContent, "task-1", "/v1/videos/task-1/content"},
		} {
			got, err := buildGrokMediaURL(a, &config.Config{}, tc.endpoint, tc.id)
			require.NoError(t, err)
			require.Equal(t, "https://relay.example"+tc.path, got)
		}
	}
	a := openAIVideoTestAccount()
	for _, id := range []string{"", ".", "..", "../models", "task?key=x", "task#fragment", "task%2fcontent"} {
		_, err := buildGrokMediaURL(a, &config.Config{}, GrokMediaEndpointVideoStatus, id)
		require.Error(t, err, "id=%q", id)
	}
	_, err := buildGrokMediaURL(a, &config.Config{}, GrokMediaEndpointVideosEdits, "")
	require.Error(t, err)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = true
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"allowed.example"}
	_, err = buildGrokMediaURL(a, cfg, GrokMediaEndpointVideosGenerations, "")
	require.Error(t, err)
}

func TestOpenAIVideoCreateJSONPreservesProtocolAndMapping(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"task-1","status":"queued","seconds":"6"}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	a := openAIVideoTestAccount()
	a.Extra = map[string]any{"openai_video_protocol": "grok"}
	a.Credentials["model_mapping"] = map[string]any{"client-video": "provider-video"}
	c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	body := []byte(`{"model":"client-video","prompt":"waves","seconds":"6","size":"1280x720","input_reference":"https://example.com/ref.png","custom_option":true}`)
	result, err := svc.ForwardGrokMedia(context.Background(), c, a, GrokMediaEndpointVideosGenerations, "", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "https://relay.example/v1/videos", upstream.request.URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.request.Header.Get("Authorization"))
	forwarded, err := io.ReadAll(upstream.request.Body)
	require.NoError(t, err)
	require.Equal(t, "provider-video", gjson.GetBytes(forwarded, "model").String())
	require.Equal(t, "6", gjson.GetBytes(forwarded, "seconds").String())
	require.Equal(t, "16:9", gjson.GetBytes(forwarded, "aspect_ratio").String())
	require.False(t, gjson.GetBytes(forwarded, "size").Exists())
	require.True(t, gjson.GetBytes(forwarded, "custom_option").Bool())
	require.Equal(t, "https://example.com/ref.png", gjson.GetBytes(forwarded, "input_reference").String())
	require.Equal(t, "client-video", result.Model)
	require.Equal(t, "provider-video", result.UpstreamModel)
	require.Equal(t, "task-1", result.ResponseID)
	require.Equal(t, 6, result.VideoDurationSeconds)
	require.Equal(t, "720p", result.VideoResolution)
	require.Zero(t, result.VideoCount, "create must not bill")
}

func TestOpenAIVideoCreateMultipartKeepsReferenceBytes(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "client-video"))
	require.NoError(t, writer.WriteField("prompt", "waves"))
	require.NoError(t, writer.WriteField("seconds", "4"))
	require.NoError(t, writer.WriteField("size", "720x1280"))
	part, err := writer.CreateFormFile("input_reference", "frame.png")
	require.NoError(t, err)
	image := []byte{0, 1, 2, 255, 13, 10}
	_, err = part.Write(image)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"task-2","status":"queued"}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	a := openAIVideoTestAccount()
	a.Credentials["model_mapping"] = map[string]any{"client-video": "provider-video"}
	c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	result, err := svc.ForwardGrokMedia(context.Background(), c, a, GrokMediaEndpointVideosGenerations, "", body.Bytes(), writer.FormDataContentType())
	require.NoError(t, err)
	_, params, err := mime.ParseMediaType(upstream.request.Header.Get("Content-Type"))
	require.NoError(t, err)
	form, err := multipart.NewReader(upstream.request.Body, params["boundary"]).ReadForm(1024 * 1024)
	require.NoError(t, err)
	defer form.RemoveAll()
	require.Equal(t, []string{"provider-video"}, form.Value["model"])
	require.Equal(t, []string{"4"}, form.Value["seconds"])
	file, err := form.File["input_reference"][0].Open()
	require.NoError(t, err)
	defer file.Close()
	got, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, image, got)
	require.Equal(t, 4, result.VideoDurationSeconds)
	require.Equal(t, "720p", result.VideoResolution)
}

func TestOpenAIVideoStatusBillingAndProtectedContent(t *testing.T) {
	for _, status := range []string{"queued", "in_progress", "failed", "done", "completed"} {
		t.Run(status, func(t *testing.T) {
			raw := `{"id":"task-1","status":"` + status + `","model":"upstream-video","seconds":"6","size":"1280x720"}`
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(raw)}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1", nil)
			result, err := svc.ForwardGrokMedia(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideoStatus, "task-1", nil, "")
			require.NoError(t, err)
			require.Equal(t, status, gjson.Get(recorder.Body.String(), "status").String(), "preserve provider status")
			if status == "completed" {
				require.Equal(t, 1, result.VideoCount)
				require.Equal(t, 6, result.VideoDurationSeconds)
				require.Equal(t, "upstream-video", result.UpstreamModel)
				require.Empty(t, result.BillingModel, "handler must use the create-time client model")
			} else {
				require.Zero(t, result.VideoCount)
			}
		})
	}
}

func TestOpenAIVideoContentUsesConfiguredHostOnly(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"id":"task-1","status":"completed","seconds":"6","video":{"url":"http://127.0.0.1/private"}}`),
		{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Content-Range": []string{"bytes 0-3/100"}}, Body: io.NopCloser(strings.NewReader("mp4!"))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", map[string]string{"Range": "bytes=0-3"})
	result, err := svc.ForwardGrokMedia(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://relay.example/v1/videos/task-1/content", upstream.requests[1].URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, "bytes=0-3", upstream.requests[1].Header.Get("Range"))
	require.Equal(t, "mp4!", recorder.Body.String())
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "/v1/videos/task-1/content", GetActualOpenAIUpstreamEndpoint(c))
	require.Equal(t, 1, result.VideoCount)
}

func TestOpenAIVideoCreateCommitsOnlyAfterTaskSaved(t *testing.T) {
	for _, fail := range []bool{false, true} {
		upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"task-1","status":"queued"}`)}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
		calls := 0
		_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, openAIVideoTestAccount(),
			[]byte(`{"model":"video","prompt":"waves","seconds":"6","size":"1280x720"}`), "application/json",
			func(result *OpenAIForwardResult) error {
				calls++
				require.False(t, c.Writer.Written())
				require.Empty(t, recorder.Body.String())
				require.Equal(t, "task-1", result.ResponseID)
				if fail {
					return errors.New("cache unavailable")
				}
				return nil
			})
		require.Equal(t, 1, calls)
		require.Equal(t, "/v1/videos", GetActualOpenAIUpstreamEndpoint(c))
		if fail {
			require.Error(t, err)
			require.False(t, c.Writer.Written())
			require.Empty(t, recorder.Body.String())
		} else {
			require.NoError(t, err)
			require.Equal(t, "task-1", gjson.Get(recorder.Body.String(), "id").String())
		}
	}
}

func TestOpenAIVideoRejectsOAuthAndOtherMedia(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	a := openAIVideoTestAccount()
	a.Type = AccountTypeOAuth
	_, err := svc.ForwardGrokMedia(context.Background(), c, a, GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"video"}`), "application/json")
	require.Error(t, err)
	a.Type = AccountTypeAPIKey
	for _, endpoint := range []GrokMediaEndpoint{GrokMediaEndpointImagesGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions} {
		_, err := svc.ForwardGrokMedia(context.Background(), c, a, endpoint, "", []byte(`{"model":"video"}`), "application/json")
		require.Error(t, err)
	}
}

func TestOpenAIVideoDurableCreateNeverPublishesImmediateCompletion(t *testing.T) {
	for _, protocol := range []string{"", "grok", "zhongy"} {
		for _, status := range []string{"queued", "in_progress", "completed", "done", "failed"} {
			t.Run(protocol+"/"+status, func(t *testing.T) {
				upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(
					`{"id":"task-1","status":"` + status + `","seconds":"6","video":{"url":"https://signed.example/movie"},"url":"https://signed.example/movie","video_url":"https://signed.example/movie","download_url":"https://signed.example/movie","metadata":{"download":"https://signed.example/movie"}}`)}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
				svc.responseHeaderFilter = responseheaders.CompileHeaderFilter(config.ResponseHeaderConfig{Enabled: true, AdditionalAllowed: []string{"content-location", "link", "refresh"}})
				for _, header := range []string{"Location", "Content-Location", "Link", "Refresh"} {
					upstream.response.Header.Set(header, "https://signed.example/movie")
				}
				account := openAIVideoTestAccount()
				account.Extra = map[string]any{"openai_video_protocol": protocol}
				c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
				saved := 0
				result, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, account,
					[]byte(`{"model":"video","seconds":"6","size":"1280x720"}`), "application/json",
					func(result *OpenAIForwardResult) error {
						require.False(t, c.Writer.Written())
						require.Equal(t, "task-1", result.ResponseID)
						saved++
						return nil
					})
				require.NoError(t, err)
				require.Equal(t, 1, saved)
				require.Zero(t, result.VideoCount, "acceptance is not settlement")
				require.JSONEq(t, `{"id":"task-1","object":"video","status":"queued"}`, recorder.Body.String())
				require.NotContains(t, recorder.Body.String(), "signed.example")
				for _, header := range []string{"Location", "Content-Location", "Link", "Refresh"} {
					require.Empty(t, recorder.Header().Get(header))
				}
			})
		}
	}
}

func TestOpenAIVideoStatusForwardRewritesExternalContentLinks(t *testing.T) {
	for _, protocol := range []string{"", "grok", "zhongy"} {
		t.Run(protocol, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(
				`{"id":"task-1","status":"completed","seconds":"6","video":{"url":"https://signed.example/movie"},"url":"https://signed.example/movie","video_url":"https://signed.example/movie","download_url":"https://signed.example/movie"}`)}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			svc.responseHeaderFilter = responseheaders.CompileHeaderFilter(config.ResponseHeaderConfig{Enabled: true, AdditionalAllowed: []string{"content-location", "link", "refresh"}})
			for _, header := range []string{"Location", "Content-Location", "Link", "Refresh"} {
				upstream.response.Header.Set(header, "https://signed.example/movie")
			}
			account := openAIVideoTestAccount()
			account.Extra = map[string]any{"openai_video_protocol": protocol}
			c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1", nil)
			_, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoStatus, "task-1", nil, "")
			require.NoError(t, err)
			for _, path := range []string{"video.url", "url", "video_url", "download_url"} {
				require.Equal(t, grokMediaContentProxyURL(c, "task-1"), gjson.Get(recorder.Body.String(), path).String())
			}
			require.NotContains(t, recorder.Body.String(), "signed.example")
			for _, header := range []string{"Location", "Content-Location", "Link", "Refresh"} {
				require.Empty(t, recorder.Header().Get(header))
			}
		})
	}
}

func TestOpenAIVideoPricingPreflightRunsBeforeUpstream(t *testing.T) {
	for _, deny := range []bool{false, true} {
		upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"task-1","status":"queued"}`)}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		account := openAIVideoTestAccount()
		account.Extra = map[string]any{"openai_video_protocol": "grok"}
		account.Credentials["model_mapping"] = map[string]any{"client-video": "provider-video"}
		c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
		preflights, saves := 0, 0
		_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, account,
			[]byte(`{"model":"client-video","seconds":"6","size":"1280x720"}`), "application/json",
			func(*OpenAIForwardResult) error {
				require.Equal(t, 1, preflights)
				saves++
				return nil
			}, func(normalized *OpenAIForwardResult) error {
				preflights++
				require.Empty(t, upstream.requests)
				require.Equal(t, "client-video", normalized.Model)
				require.Equal(t, "provider-video", normalized.UpstreamModel)
				require.Equal(t, "720p", normalized.VideoResolution)
				require.Equal(t, 6, normalized.VideoDurationSeconds)
				require.Equal(t, 1, normalized.VideoCount)
				if deny {
					return errors.New("no identified video rate")
				}
				return nil
			})
		require.Equal(t, 1, preflights)
		if deny {
			require.Error(t, err)
			require.Empty(t, upstream.requests)
			require.Zero(t, saves)
			require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
			require.True(t, IsResponseCommitted(c))
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
		} else {
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, 1, saves)
		}
	}
}

func TestOpenAIVideoRejectsUnsupportedDurationBeforeSubmit(t *testing.T) {
	for _, value := range []string{"0", "-1", "3601", "1.5", "invalid"} {
		for _, field := range []string{"seconds", "duration"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				body := []byte(`{"model":"video","prompt":"waves","` + field + `":"` + value + `"}`)
				out, _, _, _, err := prepareOpenAIVideoBody(openAIVideoTestAccount(), body, "application/json")
				require.ErrorContains(t, err, "1 and 3600")
				require.Nil(t, out)
				parts := []videoMultipartTestPart{
					videoMultipartTestField("model", "video"),
					videoMultipartTestField(field, value),
					videoMultipartTestField(field, "6"),
				}
				raw, contentType := videoMultipartTestBody(t, parts)
				out, _, _, _, err = prepareOpenAIVideoBody(openAIVideoTestAccount(), raw, contentType)
				require.ErrorContains(t, err, "1 and 3600")
				require.Nil(t, out)
			})
		}
	}
	for _, body := range []string{
		`{"model":"video","seconds":"6","duration":"30"}`,
		`{"model":"video","seconds":null}`,
		`{"model":"video","seconds":30,"seconds":6}`,
		`{"model":"video","seconds":6,"duration":8}`,
		`{"model":"video","seconds":6,"seconds":8}`,
	} {
		_, _, _, _, err := prepareOpenAIVideoBody(openAIVideoTestAccount(), []byte(body), "application/json")
		require.Error(t, err)
	}
}

func TestOpenAIVideoMissingDurationRejectedBeforeUpstream(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	result, err := svc.ForwardGrokMedia(context.Background(), c, openAIVideoTestAccount(),
		GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"video","prompt":"waves","size":"1280x720"}`), "application/json")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Empty(t, upstream.requests)
}

func TestOpenAIVideoResponseDurationCannotBeSilentlyClamped(t *testing.T) {
	for _, extra := range []string{
		`"seconds":3601`, `"seconds":6,"duration":30`, `"video":{"duration":3601}`, `"seconds":6,"seconds":30`,
		`"seconds":6,"size":"1792x1024"`, `"seconds":6,"resolution":"4k"`,
		`"seconds":6,"size":"1280x720","resolution":"480p"`,
	} {
		upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"task-1","status":"completed",` + extra + `}`)}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1", nil)
		result, err := svc.ForwardGrokMedia(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideoStatus, "task-1", nil, "")
		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, http.StatusBadGateway, recorder.Code)
		require.True(t, IsResponseCommitted(c))
	}
}

func TestOpenAIVideoContentCannotBypassCompletionValidation(t *testing.T) {
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"id":"task-1","status":"completed","seconds":3601}`, http.StatusBadGateway},
		{`{"id":"task-1","status":"in_progress","seconds":6}`, http.StatusConflict},
		{`{"id":"task-1","status":"done","seconds":6}`, http.StatusConflict},
	} {
		upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(tc.body)}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)
		result, err := svc.ForwardGrokMedia(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, tc.code, recorder.Code)
		require.Len(t, upstream.requests, 1)
	}
}

func TestOpenAIVideoExpiredContentIsGone(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
			grokMediaContentStatusResponse(`{"id":"task-1","status":"completed","seconds":"6"}`),
			{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"media expired or not found"}`))},
		}}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)
		result, err := svc.ForwardGrokMedia(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, http.StatusGone, recorder.Code)
		require.Equal(t, "video_content_expired", gjson.Get(recorder.Body.String(), "error.type").String())
		require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
		require.True(t, IsResponseCommitted(c))
	}
}
