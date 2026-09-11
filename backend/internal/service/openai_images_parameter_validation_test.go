package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayServiceForwardImages_OAuthRejectsMismatchedNativeImageParameters(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		response    string
		param       string
	}{
		{
			name:        "actual size",
			requestBody: `{"model":"gpt-image-2.5-sunburst","prompt":"draw a chart","size":"1024x1024"}`,
			response: fmt.Sprintf(
				`{"model":"gpt-image-2.5-sunburst","size":"1370x1148","data":[{"b64_json":%q,"size":"1370x1148"}]}`,
				encodeOpenAIImageTestPNG(t, 1370, 1148),
			),
			param: "size",
		},
		{
			name:        "quality",
			requestBody: `{"model":"gpt-image-2.5-sunburst","prompt":"draw a chart","quality":"xhigh"}`,
			response: fmt.Sprintf(
				`{"model":"gpt-image-2.5-sunburst","quality":"medium","data":[{"b64_json":%q,"quality":"medium"}]}`,
				encodeOpenAIImageTestPNG(t, 1024, 1024),
			),
			param: "quality",
		},
		{
			name:        "transparent background",
			requestBody: `{"model":"gpt-image-2.5-sunburst","prompt":"draw a logo","background":"transparent","output_format":"png"}`,
			response: fmt.Sprintf(
				`{"model":"gpt-image-2.5-sunburst","background":"opaque","output_format":"png","data":[{"b64_json":%q,"background":"opaque","output_format":"png"}]}`,
				encodeOpenAIImageTestPNG(t, 1024, 1024),
			),
			param: "background",
		},
		{
			name:        "output format",
			requestBody: `{"model":"gpt-image-2.5-sunburst","prompt":"draw a logo","output_format":"png"}`,
			response: fmt.Sprintf(
				`{"model":"gpt-image-2.5-sunburst","output_format":"webp","data":[{"b64_json":%q,"output_format":"webp"}]}`,
				encodeOpenAIImageTestPNG(t, 1024, 1024),
			),
			param: "output_format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := runOpenAIOAuthDirectJSONImageValidationTest(t, tt.requestBody, tt.response)

			require.Nil(t, run.result)
			require.Error(t, run.err)
			require.Equal(t, http.StatusBadGateway, run.recorder.Code)
			require.Equal(t, "upstream_response_mismatch", gjson.Get(run.recorder.Body.String(), "error.type").String())
			require.Equal(t, "image_generation_parameters_mismatch", gjson.Get(run.recorder.Body.String(), "error.code").String())
			require.Equal(t, tt.param, gjson.Get(run.recorder.Body.String(), "error.param").String())

			var upstreamErr *OpenAIImagesUpstreamError
			require.ErrorAs(t, run.err, &upstreamErr)
			require.True(t, upstreamErr.NonRetryable)
			require.False(t, IsOpenAIImagesRetryableUpstreamError(upstreamErr))
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(run.err, &failoverErr))
		})
	}
}

func TestOpenAIGatewayServiceForwardImages_OAuthPreservesImage25TransparentBackgroundOptions(t *testing.T) {
	requestBody := `{"model":"gpt-image-2.5-sunburst","prompt":"draw a transparent logo","size":"1024x1024","quality":"xhigh","background":"transparent","output_format":"png"}`
	encoded := encodeOpenAITransparentImageTestPNG(t, 1024, 1024)
	response := fmt.Sprintf(
		`{"created":1710000012,"model":"gpt-image-2.5-sunburst","size":"1024x1024","quality":"xhigh","background":"transparent","output_format":"png","data":[{"b64_json":%q,"size":"1024x1024","quality":"xhigh","background":"transparent","output_format":"png"}]}`,
		encoded,
	)

	run := runOpenAIOAuthDirectJSONImageValidationTest(t, requestBody, response)

	require.NoError(t, run.err)
	require.NotNil(t, run.result)
	require.Equal(t, "gpt-image-2.5-sunburst", gjson.GetBytes(run.upstream.lastBody, "model").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(run.upstream.lastBody, "size").String())
	require.Equal(t, "xhigh", gjson.GetBytes(run.upstream.lastBody, "quality").String())
	require.Equal(t, "transparent", gjson.GetBytes(run.upstream.lastBody, "background").String())
	require.Equal(t, "png", gjson.GetBytes(run.upstream.lastBody, "output_format").String())
	require.Equal(t, "transparent", gjson.Get(run.recorder.Body.String(), "background").String())
	require.Equal(t, "1024x1024", gjson.Get(run.recorder.Body.String(), "size").String())
	require.Equal(t, "xhigh", gjson.Get(run.recorder.Body.String(), "quality").String())
	require.Equal(t, "gpt-image-2.5-sunburst", gjson.Get(run.recorder.Body.String(), "model").String())
	require.Equal(t, encoded, gjson.Get(run.recorder.Body.String(), "data.0.b64_json").String())
}

func TestOpenAIGatewayServiceForwardImages_OAuthStreamingRejectsMismatchedNativeImageParameters(t *testing.T) {
	tests := []struct {
		name            string
		requestFields   string
		observedFields  string
		resultPNGWidth  int
		resultPNGHeight int
		param           string
	}{
		{
			name:            "actual size",
			requestFields:   `"size":"1024x1024"`,
			observedFields:  `"size":"1370x1148"`,
			resultPNGWidth:  1370,
			resultPNGHeight: 1148,
			param:           "size",
		},
		{
			name:            "quality",
			requestFields:   `"quality":"xhigh"`,
			observedFields:  `"quality":"medium"`,
			resultPNGWidth:  1024,
			resultPNGHeight: 1024,
			param:           "quality",
		},
		{
			name:            "transparent background",
			requestFields:   `"background":"transparent"`,
			observedFields:  `"background":"opaque"`,
			resultPNGWidth:  1024,
			resultPNGHeight: 1024,
			param:           "background",
		},
		{
			name:            "output format",
			requestFields:   `"output_format":"png"`,
			observedFields:  `"output_format":"webp"`,
			resultPNGWidth:  1024,
			resultPNGHeight: 1024,
			param:           "output_format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeOpenAIImageTestPNG(t, tt.resultPNGWidth, tt.resultPNGHeight)
			response := fmt.Sprintf(
				"data: {\"type\":\"response.created\",\"response\":{\"created_at\":1710000013,\"tools\":[{\"type\":\"image_generation\",\"model\":\"gpt-image-2-codex\",%s}]}}\n\n"+
					"data: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":%q,\"partial_image_index\":0}\n\n"+
					"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000013,\"tools\":[{\"type\":\"image_generation\",\"model\":\"gpt-image-2-codex\",%s}],\"output\":[{\"type\":\"image_generation_call\",\"result\":%q,%s}]}}\n\n"+
					"data: [DONE]\n\n",
				tt.observedFields,
				result,
				tt.observedFields,
				result,
				tt.observedFields,
			)
			request := fmt.Sprintf(
				`{"model":"gpt-image-2.5-sunburst","prompt":"draw a test image","stream":true,%s}`,
				tt.requestFields,
			)

			run := runOpenAIOAuthStreamingImageValidationTest(t, request, response)
			require.Nil(t, run.result)
			require.Error(t, run.err)
			require.Contains(t, run.recorder.Body.String(), "event: error")
			require.NotContains(t, run.recorder.Body.String(), "event: image_generation.partial_image")
			require.NotContains(t, run.recorder.Body.String(), "event: image_generation.completed")
			events := parseOpenAIImageTestSSEEvents(run.recorder.Body.String())
			errorEvent, ok := findOpenAIImageTestSSEEvent(events, "error")
			require.True(t, ok)
			require.Equal(t, "upstream_response_mismatch", gjson.Get(errorEvent.Data, "error.type").String())
			require.Equal(t, "image_generation_parameters_mismatch", gjson.Get(errorEvent.Data, "error.code").String())
			require.Equal(t, tt.param, gjson.Get(errorEvent.Data, "error.param").String())

			var upstreamErr *OpenAIImagesUpstreamError
			require.ErrorAs(t, run.err, &upstreamErr)
			require.True(t, upstreamErr.NonRetryable)
			require.False(t, IsOpenAIImagesRetryableUpstreamError(upstreamErr))
		})
	}
}

func TestOpenAIGatewayServiceForwardImages_OAuthStreamingPreservesImage25TransparentBackgroundOptions(t *testing.T) {
	encoded := encodeOpenAITransparentImageTestPNG(t, 1024, 1024)
	request := `{"model":"gpt-image-2.5-sunburst","prompt":"draw a transparent logo","stream":true,"size":"1024x1024","quality":"xhigh","background":"transparent","output_format":"png"}`
	response := fmt.Sprintf(
		"data: {\"type\":\"response.created\",\"response\":{\"created_at\":1710000014,\"tools\":[{\"type\":\"image_generation\",\"model\":\"gpt-image-2-codex\",\"size\":\"1024x1024\",\"quality\":\"xhigh\",\"background\":\"transparent\",\"output_format\":\"png\"}]}}\n\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000014,\"tools\":[{\"type\":\"image_generation\",\"model\":\"gpt-image-2-codex\",\"size\":\"1024x1024\",\"quality\":\"xhigh\",\"background\":\"transparent\",\"output_format\":\"png\"}],\"output\":[{\"type\":\"image_generation_call\",\"result\":%q,\"size\":\"1024x1024\",\"quality\":\"xhigh\",\"background\":\"transparent\",\"output_format\":\"png\"}]}}\n\n"+
			"data: [DONE]\n\n",
		encoded,
	)

	run := runOpenAIOAuthStreamingImageValidationTest(t, request, response)
	require.NoError(t, run.err)
	require.NotNil(t, run.result)
	require.Equal(t, "gpt-image-2-codex", run.result.UpstreamResponseModel)
	require.Equal(t, "gpt-image-2.5-sunburst", gjson.GetBytes(run.upstream.lastBody, "model").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(run.upstream.lastBody, "size").String())
	require.Equal(t, "xhigh", gjson.GetBytes(run.upstream.lastBody, "quality").String())
	require.Equal(t, "transparent", gjson.GetBytes(run.upstream.lastBody, "background").String())
	require.Equal(t, "png", gjson.GetBytes(run.upstream.lastBody, "output_format").String())
	require.True(t, gjson.GetBytes(run.upstream.lastBody, "stream").Bool())

	events := parseOpenAIImageTestSSEEvents(run.recorder.Body.String())
	completed, ok := findOpenAIImageTestSSEEvent(events, "image_generation.completed")
	require.True(t, ok)
	require.Equal(t, encoded, gjson.Get(completed.Data, "b64_json").String())
	require.Equal(t, "transparent", gjson.Get(completed.Data, "background").String())
	require.Equal(t, "xhigh", gjson.Get(completed.Data, "quality").String())
	require.Equal(t, "1024x1024", gjson.Get(completed.Data, "size").String())
	require.Equal(t, "gpt-image-2.5-sunburst", gjson.Get(completed.Data, "model").String())
}

type openAIOAuthDirectJSONImageValidationRun struct {
	result   *OpenAIForwardResult
	err      error
	recorder *httptest.ResponseRecorder
	upstream *httpUpstreamRecorder
}

func runOpenAIOAuthDirectJSONImageValidationTest(
	t *testing.T,
	requestBody string,
	responseBody string,
) openAIOAuthDirectJSONImageValidationRun {
	t.Helper()
	gin.SetMode(gin.TestMode)
	body := []byte(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("api_key", &APIKey{ID: 97})

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"req_img_parameter_validation"},
		},
		Body: io.NopCloser(strings.NewReader(responseBody)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	account := &Account{
		ID:       97,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token-123",
		},
	}
	result, forwardErr := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
	return openAIOAuthDirectJSONImageValidationRun{
		result:   result,
		err:      forwardErr,
		recorder: rec,
		upstream: upstream,
	}
}

type openAIOAuthStreamingImageValidationRun struct {
	result   *OpenAIForwardResult
	err      error
	recorder *httptest.ResponseRecorder
	upstream *httpUpstreamRecorder
}

func runOpenAIOAuthStreamingImageValidationTest(
	t *testing.T,
	requestBody string,
	responseBody string,
) openAIOAuthStreamingImageValidationRun {
	t.Helper()
	gin.SetMode(gin.TestMode)
	body := []byte(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("api_key", &APIKey{ID: 98})

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"req_img_stream_parameter_validation"},
		},
		Body: io.NopCloser(strings.NewReader(responseBody)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	account := &Account{
		ID:       98,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token-123",
		},
	}
	result, forwardErr := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
	return openAIOAuthStreamingImageValidationRun{
		result:   result,
		err:      forwardErr,
		recorder: rec,
		upstream: upstream,
	}
}

func encodeOpenAITransparentImageTestPNG(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
