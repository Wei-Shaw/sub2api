//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func qianDreamTestAccount() *Account {
	return &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"openai_video_protocol": "qiandream"},
		Credentials: map[string]any{
			"base_url": "https://qiandream.example",
			"api_key":  "qian-key",
			"model_mapping": map[string]any{
				"client-video": "qian-video-model",
			},
		},
	}
}

func TestQianDreamVideoURLAndBodyAdapter(t *testing.T) {
	account := qianDreamTestAccount()
	createURL, err := buildGrokMediaURL(account, &config.Config{}, GrokMediaEndpointVideosGenerations, "")
	require.NoError(t, err)
	require.Equal(t, "https://qiandream.example/v1/video/generations", createURL)
	statusURL, err := buildGrokMediaURL(account, &config.Config{}, GrokMediaEndpointVideoStatus, "task-1")
	require.NoError(t, err)
	require.Equal(t, "https://qiandream.example/v1/video/generations/task-1", statusURL)
	contentURL, err := buildGrokMediaURL(account, &config.Config{}, GrokMediaEndpointVideoContent, "task-1")
	require.NoError(t, err)
	require.Equal(t, statusURL, contentURL)

	body := []byte(`{"model":"client-video","prompt":"turn around and smile","seconds":6,"size":"1280x720","aspect_ratio":"16:9","input_reference":{"image_url":{"url":"https://example.com/reference.png"}},"n":2}`)
	prepared, contentType, info, mapped, err := prepareQianDreamVideoBody(account, body, GrokMediaRequestInfo{})
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	require.Equal(t, "qian-video-model", mapped)
	require.Equal(t, 6, info.DurationSeconds)
	require.Equal(t, VideoBillingResolution720P, info.Resolution)
	require.Equal(t, "qian-video-model", gjson.GetBytes(prepared, "model").String())
	require.Equal(t, 6, int(gjson.GetBytes(prepared, "duration").Int()))
	require.Equal(t, VideoBillingResolution720P, gjson.GetBytes(prepared, "resolution").String())
	require.Equal(t, "16:9", gjson.GetBytes(prepared, "ratio").String())
	require.Equal(t, "https://example.com/reference.png", gjson.GetBytes(prepared, "images.0").String())
	require.False(t, gjson.GetBytes(prepared, "seconds").Exists())
	require.False(t, gjson.GetBytes(prepared, "size").Exists())
	require.False(t, gjson.GetBytes(prepared, "input_reference").Exists())
}

func TestQianDreamVideoResponseNormalizationAndBillingGate(t *testing.T) {
	account := qianDreamTestAccount()
	pending := normalizeOpenAIVideoResponse(account, []byte(`{"data":{"id":"task-7","status":"processing"}}`))
	require.Equal(t, "task-7", gjson.GetBytes(pending, "id").String())
	require.Equal(t, "in_progress", gjson.GetBytes(pending, "status").String())
	require.Nil(t, openAIVideoCompletionForAccount(account, pending, "task-7"))

	completed := normalizeOpenAIVideoResponse(account, []byte(`{"data":[{"id":"task-7","status":"succeeded","url":"https://cdn.example/task-7.mp4","duration":6,"resolution":"720p"}]}`))
	require.Equal(t, "task-7", gjson.GetBytes(completed, "id").String())
	require.Equal(t, "completed", gjson.GetBytes(completed, "status").String())
	require.Equal(t, "https://cdn.example/task-7.mp4", gjson.GetBytes(completed, "video.url").String())
	result := openAIVideoCompletionForAccount(account, completed, "task-7")
	require.NotNil(t, result)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, 6, result.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution720P, result.VideoResolution)
	require.Equal(t, "https://cdn.example/task-7.mp4", result.VideoURL)

	withoutURL := normalizeOpenAIVideoResponse(account, []byte(`{"id":"task-7","status":"completed","duration":6,"resolution":"720p"}`))
	require.Nil(t, openAIVideoCompletionForAccount(account, withoutURL, "task-7"))
}

func TestQianDreamVideoDoesNotChangeNativeOpenAIVideoURL(t *testing.T) {
	account := openAIVideoTestAccount()
	got, err := buildGrokMediaURL(account, &config.Config{}, GrokMediaEndpointVideosGenerations, "")
	require.NoError(t, err)
	require.Equal(t, "https://relay.example/v1/videos", got)
}
