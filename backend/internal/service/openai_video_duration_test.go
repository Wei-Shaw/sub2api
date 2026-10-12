//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIVideoNativeThirtySecondsRequestAndResponse(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"id":"native-30","status":"queued","seconds":"30"}`),
		grokMediaContentStatusResponse(`{"id":"native-30","status":"completed","seconds":"30","size":"1280x720"}`),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := openAIVideoTestAccount()
	c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	result, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, account,
		[]byte(`{"model":"native-video","seconds":"30","size":"1280x720"}`), "application/json",
		func(created *OpenAIForwardResult) error {
			require.Equal(t, 30, created.VideoDurationSeconds)
			return nil
		})
	require.NoError(t, err)
	require.Equal(t, 30, result.VideoDurationSeconds)
	body, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	require.Equal(t, "30", gjson.GetBytes(body, "seconds").String())
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/native-30", nil)
	result, err = svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoStatus, "native-30", nil, "")
	require.NoError(t, err)
	require.Equal(t, 30, result.VideoDurationSeconds)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, "completed", gjson.Get(recorder.Body.String(), "status").String())
}

func TestOpenAIVideoExplicitGrokRejectsSixteenSeconds(t *testing.T) {
	account := openAIVideoTestAccount()
	account.Extra = map[string]any{"openai_video_protocol": "grok"}
	upstream := &grokMediaContentUpstreamStub{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, account,
		[]byte(`{"model":"grok-imagine-video","seconds":"16","size":"1280x720"}`), "application/json",
		func(*OpenAIForwardResult) error {
			t.Fatal("invalid create must not persist")
			return nil
		})
	require.ErrorContains(t, err, "1 and 15")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Empty(t, upstream.requests)
	require.ErrorContains(t, validateOpenAIVideoResponseBilling([]byte(`{"status":"completed","seconds":"16"}`), account), "1 and 15")
}
