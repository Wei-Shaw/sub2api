package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"strings"
	"testing"
)

func seedanceAccount() *Account {
	a := openAIVideoTestAccount()
	a.Extra = map[string]any{"openai_video_protocol": "zhongy"}
	return a
}
func TestOpenAIVideoSeedanceCreateThirtySecondsAndFutureModel(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"data":{"task_id":"new-task","status":"processing","seconds":"30"}}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	a := seedanceAccount()
	a.Credentials["model_mapping"] = map[string]any{"future-model": "provider-model"}
	c, rec := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	saved := false
	result, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, a, []byte(`{"model":"future-model","prompt":"waves","duration":30,"size":"1280x720","seed":123}`), "application/json", func(r *OpenAIForwardResult) error {
		saved = true
		require.False(t, c.Writer.Written())
		require.Equal(t, 30, r.VideoDurationSeconds)
		return nil
	})
	require.NoError(t, err)
	require.True(t, saved)
	require.Zero(t, result.VideoCount)
	raw, err := io.ReadAll(upstream.request.Body)
	require.NoError(t, err)
	require.Equal(t, "provider-model", gjson.GetBytes(raw, "model").String())
	require.Equal(t, gjson.String, gjson.GetBytes(raw, "seconds").Type)
	require.Equal(t, "30", gjson.GetBytes(raw, "seconds").String())
	require.False(t, gjson.GetBytes(raw, "duration").Exists())
	require.False(t, gjson.GetBytes(raw, "size").Exists())
	require.Equal(t, "720p", gjson.GetBytes(raw, "resolution").String())
	require.Equal(t, "16:9", gjson.GetBytes(raw, "aspect_ratio").String())
	require.Equal(t, int64(123), gjson.GetBytes(raw, "seed").Int())
	require.Equal(t, "new-task", gjson.Get(rec.Body.String(), "id").String())
	require.Equal(t, "queued", gjson.Get(rec.Body.String(), "status").String())
}

func TestOpenAIVideoSeedanceFixedResolutionModelsDoNotInjectBillingTier(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"data":{"task_id":"h3-task","status":"processing","seconds":"15"}}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, seedanceAccount(), []byte(`{"model":"MiniMax-H3-933-1440p-10-15s","prompt":"waves","seconds":"15","resolution":"1080p"}`), "application/json", func(r *OpenAIForwardResult) error { return nil })
	require.NoError(t, err)
	raw, err := io.ReadAll(upstream.request.Body)
	require.NoError(t, err)
	require.Equal(t, "1440p", fixedOpenAIVideoResolution("MiniMax-H3-933-1440p-10-15s"))
	require.False(t, gjson.GetBytes(raw, "resolution").Exists())
}
func TestOpenAIVideoSeedanceCompletedStatusAndDownload(t *testing.T) {
	for _, ep := range []GrokMediaEndpoint{GrokMediaEndpointVideoStatus, GrokMediaEndpointVideoContent} {
		t.Run(string(ep), func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
				grokMediaContentStatusResponse(`{"data":{"task_id":"task-1","status":"completed","seconds":"30","download_url":"https://download.example/video.mp4"}}`),
				{StatusCode: 206, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Content-Range": []string{"bytes 0-3/100"}}, Body: io.NopCloser(strings.NewReader("mp4!"))},
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			c, rec := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1", map[string]string{"Range": "bytes=0-3"})
			result, err := svc.ForwardGrokMedia(context.Background(), c, seedanceAccount(), ep, "task-1", nil, "")
			require.NoError(t, err)
			require.Equal(t, 1, result.VideoCount)
			require.Equal(t, 30, result.VideoDurationSeconds)
			if ep == GrokMediaEndpointVideoStatus {
				require.Equal(t, "/v1/videos/task-1/content", gjson.Get(rec.Body.String(), "download_url").String())
			} else {
				require.Equal(t, 206, rec.Code)
				require.Equal(t, "mp4!", rec.Body.String())
				require.Equal(t, "bytes=0-3", upstream.requests[1].Header.Get("Range"))
				require.Equal(t, "download.example", upstream.requests[1].URL.Hostname())
				require.Empty(t, upstream.requests[1].Header.Get("Authorization"))
			}
		})
	}
}
func TestOpenAIVideoSeedanceDurationBillingAndErrors(t *testing.T) {
	a := seedanceAccount()
	for _, raw := range []string{"0", "-1", "1.5", "3601"} {
		v := 0
		require.Error(t, validateOpenAIVideoDuration(raw, &v, a))
	}
	v := 0
	require.NoError(t, validateOpenAIVideoDuration("30", &v, a))
	require.Error(t, validateOpenAIVideoDuration("15", &v, a))
	require.Equal(t, 30, NormalizeVideoDurationForPlatform(&Group{Platform: PlatformOpenAI}, 30))
	require.Equal(t, 15, NormalizeVideoDurationForPlatform(&Group{Platform: PlatformGrok}, 30))
	svc := &OpenAIGatewayService{billingService: &BillingService{}}
	price := 0.25
	cost := svc.calculateVideoCostForPlatform(&APIKey{Group: &Group{Platform: PlatformOpenAI}}, "future-model", "720p", 1, 30, &VideoPriceConfig{Price720P: &price}, 1)
	require.InDelta(t, 7.5, cost.TotalCost, 0.0001)
	raw := normalizeZhongYVideoError([]byte(`{"code":"fail_to_fetch_task","message":"{\"error\":{\"code\":\"missing_seconds\",\"message\":\"seconds required\"}}"}`))
	require.Equal(t, "seconds required", gjson.GetBytes(raw, "error.message").String())
}
