package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestFilterOpenCodeResponsesSSEFrame_RewritesImageGenerationToMessage(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":2,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		"",
	}
	out, _, _, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})
	require.True(t, keep)
	require.Contains(t, out, "response.output_item.added")
	require.Contains(t, out, "response.content_part.added")
	require.Contains(t, out, "response.output_text.delta")
	require.Contains(t, out, "response.output_text.done")
	require.Contains(t, out, "response.content_part.done")
	require.Contains(t, out, "response.output_item.done")
	require.Contains(t, out, `"output_index":2`)
	require.Contains(t, out, "sub2api-image://img_")
	require.NotContains(t, out, `"type":"function_call"`)
	require.NotContains(t, out, `"name":"bash"`)
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, pngB64)
}

func TestFilterOpenCodeResponsesSSEFrame_DropsImageProgressAndAdded(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, eventType := range []string{
		"response.image_generation_call.in_progress",
		"response.image_generation_call.generating",
		"response.image_generation_call.partial_image",
		"response.image_generation_call.completed",
	} {
		progressFrame := []string{
			"event: " + eventType,
			`data: {"type":"` + eventType + `","output_index":0,"partial_image_index":0,"partial_image_b64":"` + pngB64 + `"}`,
			"",
		}
		frame, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), progressFrame, store, openCodeImageRewriteOptions{})
		require.False(t, keep, eventType)
		require.True(t, hasData, eventType)
		require.Contains(t, data, eventType)
		require.Empty(t, frame)
	}

	addedFrame := []string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"ig_1","type":"image_generation_call","status":"in_progress"}}`,
		"",
	}
	frame, _, _, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), addedFrame, store, openCodeImageRewriteOptions{})
	require.False(t, keep)
	require.Empty(t, frame)
}

func TestFilterOpenCodeResponsesSSEFrame_DropsEventOnlyImageProgress(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.image_generation_call.partial_image",
		`data: {"output_index":0,"partial_image_index":0,"partial_image_b64":"` + pngB64 + `"}`,
		"",
	}

	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{})

	require.False(t, keep)
	require.True(t, hasData)
	require.Contains(t, data, "partial_image_b64")
	require.Empty(t, out)
}

func TestFilterOpenCodeResponsesSSEFrame_DropsImageEventWithoutData(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.image_generation_call.partial_image",
		"",
	}

	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{})

	require.False(t, keep)
	require.False(t, hasData)
	require.Empty(t, data)
	require.Empty(t, out)
}

func TestFilterOpenCodeResponsesSSEFrame_RewritesEventOnlyImageDone(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.output_item.done",
		`data: {"output_index":2,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		"",
	}

	out, _, _, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})

	require.True(t, keep)
	require.Contains(t, out, "response.output_item.added")
	require.Contains(t, out, "sub2api-image://img_")
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, pngB64)
}

func TestFilterOpenCodeResponsesSSEFrame_RewritesDataOnlyImageDoneWithoutEventOrType(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		`data: {"output_index":2,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		"",
	}

	out, _, _, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})

	require.True(t, keep)
	require.Contains(t, out, "response.output_item.added")
	require.Contains(t, out, "sub2api-image://img_")
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, pngB64)
}

func TestFilterOpenCodeResponsesSSEFrame_DropsDataOnlyPartialImageWithoutEventOrType(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		`data: {"output_index":0,"partial_image_index":0,"partial_image_b64":"` + pngB64 + `"}`,
		"",
	}

	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{})

	require.False(t, keep)
	require.True(t, hasData)
	require.Contains(t, data, "partial_image_b64")
	require.Empty(t, out)
	require.NotContains(t, out, "partial_image_b64")
	require.NotContains(t, out, pngB64)
}

func TestFilterOpenCodeResponsesSSEFrame_DropsDataOnlyTerminalOutputImageWithoutEventOrType(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		`data: {"response":{"output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}]}}`,
		"",
	}

	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{})

	require.False(t, keep)
	require.True(t, hasData)
	require.Contains(t, data, "image_generation_call")
	require.Empty(t, out)
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, pngB64)
}

func TestFilterOpenCodeResponsesSSEFrame_DropsDataOnlyMalformedImageDone(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		`data: {"type":"response.output_item.done","output_index":2,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `"`,
		"",
	}

	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{})

	require.False(t, keep)
	require.True(t, hasData)
	require.Contains(t, data, "image_generation_call")
	require.Empty(t, out)
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, "result")
	require.NotContains(t, out, pngB64)
}

func TestFilterOpenCodeResponsesSSEFrame_DropsDataOnlyMalformedPartialImage(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		`data: {"type":"response.image_generation_call.partial_image","partial_image_b64":"` + pngB64 + `"`,
		"",
	}

	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{})

	require.False(t, keep)
	require.True(t, hasData)
	require.Contains(t, data, "partial_image_b64")
	require.Empty(t, out)
	require.NotContains(t, out, "partial_image_b64")
	require.NotContains(t, out, pngB64)
}

func TestContainsOpenCodeImageGenerationSSE_DetectsEventOnlyMalformedImageFrame(t *testing.T) {
	body := strings.Join([]string{
		"event: response.image_generation_call.partial_image",
		`data: {"output_index":0,"partial_image_index":0,"partial_image_b64":"` + pngB64 + `"}`,
		"",
	}, "\n")

	require.True(t, containsOpenCodeImageGenerationSSE(body))
}

func TestContainsOpenCodeImageGenerationSSE_DetectsDataOnlyMalformedImageFrame(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_item.done","output_index":2,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `"`,
		"",
	}, "\n")

	require.True(t, containsOpenCodeImageGenerationSSE(body))
}

func TestContainsOpenCodeImageGenerationSSE_DetectsDataOnlyImageMarkers(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{
			name: "item image generation call",
			data: `{"output_index":2,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		},
		{
			name: "terminal output image generation call",
			data: `{"response":{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}]}}`,
		},
		{
			name: "partial image",
			data: `{"output_index":0,"partial_image_index":0,"partial_image_b64":"` + pngB64 + `"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Join([]string{"data: " + tc.data, ""}, "\n")

			require.True(t, containsOpenCodeImageGenerationSSE(body))
		})
	}
}

func TestFilterOpenCodeResponsesSSEFrame_ImageDoneWithoutResultEmitsText(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":2,"item":{"id":"ig_1","type":"image_generation_call","status":"completed"}}`,
		"",
	}
	out, _, _, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{})
	require.True(t, keep)
	require.Contains(t, out, "response.output_text.delta")
	require.Contains(t, out, "no image result")
	require.NotContains(t, out, "image_generation_call")
}

func TestFilterOpenCodeResponsesSSEFrame_RewritesCompletedOutputImageAndKeepsTerminalData(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":3,"output_tokens":5}}}`,
		"",
	}
	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})
	require.True(t, keep)
	require.True(t, hasData)
	require.Contains(t, out, "response.output_item.added")
	require.Contains(t, out, "response.completed")
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, pngB64)
	require.Equal(t, "response.completed", gjson.Get(data, "type").String())
	require.Equal(t, int64(3), gjson.Get(data, "response.usage.input_tokens").Int())
	require.Equal(t, "message", gjson.Get(data, "response.output.0.type").String())
}

func TestFilterOpenCodeResponsesSSEFrame_RewritesDoneOutputImageAndKeepsTerminalData(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	frame := []string{
		"event: response.done",
		`data: {"type":"response.done","response":{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":3,"output_tokens":5}}}`,
		"",
	}
	out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})
	require.True(t, keep)
	require.True(t, hasData)
	require.Contains(t, out, "response.output_item.added")
	require.Contains(t, out, "response.done")
	require.NotContains(t, out, "image_generation_call")
	require.NotContains(t, out, pngB64)
	require.Equal(t, "response.done", gjson.Get(data, "type").String())
	require.Equal(t, int64(3), gjson.Get(data, "response.usage.input_tokens").Int())
	require.Equal(t, "message", gjson.Get(data, "response.output.0.type").String())
}

func TestFilterOpenCodeResponsesSSEFrame_RewritesOtherTerminalOutputImagesAndKeepsTerminalData(t *testing.T) {
	for _, eventType := range []string{"response.incomplete", "response.cancelled", "response.canceled"} {
		t.Run(eventType, func(t *testing.T) {
			store := newTestOpenAIGeneratedImageStore(t, fixedNow)
			frame := []string{
				"event: " + eventType,
				`data: {"type":"` + eventType + `","response":{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":3,"output_tokens":5}}}`,
				"",
			}

			out, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(context.Background(), frame, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})

			require.True(t, keep)
			require.True(t, hasData)
			require.Contains(t, out, "response.output_item.added")
			require.Contains(t, out, eventType)
			require.NotContains(t, out, "image_generation_call")
			require.NotContains(t, out, pngB64)
			require.Equal(t, eventType, gjson.Get(data, "type").String())
			require.Equal(t, int64(3), gjson.Get(data, "response.usage.input_tokens").Int())
			require.Equal(t, "message", gjson.Get(data, "response.output.0.type").String())
		})
	}
}

func TestHandleStreamingResponse_OpenCodeRewritesImageGenerationDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
	}, "\n")))}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	body := rec.Body.String()
	require.Contains(t, body, "response.output_item.added")
	require.Contains(t, body, "response.output_text.delta")
	require.Contains(t, body, "sub2api-image://img_")
	require.NotContains(t, body, "image_generation_call")
	require.NotContains(t, body, pngB64)
}

func TestHandleStreamingResponse_OpenCodeRewritesCompletedOnlyImageFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
	}, "\n")))}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	body := rec.Body.String()
	require.Contains(t, body, "response.output_item.added")
	require.Contains(t, body, "response.completed")
	require.Contains(t, body, "sub2api-image://img_")
	require.NotContains(t, body, "image_generation_call")
	require.NotContains(t, body, pngB64)
}

func TestHandleStreamingResponse_OpenCodeTerminalImageFallbackUsesEventTypeWhenDataTypeMissing(t *testing.T) {
	for _, eventType := range []string{"response.incomplete", "response.cancelled", "response.canceled"} {
		t.Run(eventType, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			store := newTestOpenAIGeneratedImageStore(t, fixedNow)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "opencode/1.0")
			svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(strings.Join([]string{
				"event: " + eventType,
				`data: {"response":{"id":"resp_1","model":"gpt-5.5","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
				"",
			}, "\n")))}

			result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")

			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.usage)
			require.Equal(t, 1, result.usage.InputTokens)
			require.Equal(t, 2, result.usage.OutputTokens)
			body := rec.Body.String()
			require.Contains(t, body, "response.output_item.added")
			require.Contains(t, body, eventType)
			require.Contains(t, body, "sub2api-image://img_")
			require.NotContains(t, body, "image_generation_call")
			require.NotContains(t, body, pngB64)
		})
	}
}

func TestHandleStreamingResponse_OpenCodePreservesSyntheticFramesWhenReplacingModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
	}, "\n")))}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.1")
	require.NoError(t, err)
	body := rec.Body.String()
	require.Contains(t, body, "response.output_item.added")
	require.Contains(t, body, "sub2api-image://img_")
	require.Contains(t, body, "gpt-5.5")
	require.NotContains(t, body, "image_generation_call")
	require.NotContains(t, body, pngB64)
}

func TestHandleStreamingResponse_OpenCodeResponseFailedBeforeOutputReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid-opencode-failed"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"upstream processing failed"}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "upstream processing failed")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestHandleStreamingResponse_OpenCodeDedupesImageDoneAndTerminalOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		"",
		"data: [DONE]",
	}, "\n")))}

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	body := rec.Body.String()
	require.Equal(t, 1, strings.Count(body, "event: response.output_item.added"))
	require.NotContains(t, body, `"name":"bash"`)
	require.NotContains(t, body, "image_generation_call")
	require.NotContains(t, body, pngB64)
	require.Equal(t, 1, countGeneratedImageMetadataFiles(t, store))
}

func TestHandleStreamingResponse_OpenCodeSyntheticFrameRefreshesKeepaliveIdle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1, MaxLineSize: defaultMaxLineSize}}
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: cfg, generatedImageStore: store}
	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("event: response.created\n" + `data: {"type":"response.created","response":{"id":"resp_1"}}` + "\n\n"))
		time.Sleep(1100 * time.Millisecond)
		_, _ = pw.Write([]byte("event: response.output_item.done\n" + `data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}` + "\n\n"))
		time.Sleep(700 * time.Millisecond)
		_, _ = pw.Write([]byte("event: response.completed\n" + `data: {"type":"response.completed","response":{"id":"resp_1","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}` + "\n\n"))
	}()

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")
	_ = pr.Close()

	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(rec.Body.String(), ":\n\n"))
}

func TestHandleStreamingResponse_NonOpenCodeKeepsImageGenerationCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "curl/8.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
			"data: [DONE]",
		}, "\n"))),
	}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), "image_generation_call")
	require.Contains(t, rec.Body.String(), pngB64)
}

func TestContainsOpenCodeImageGenerationSSE_DetectsTerminalOutputImage(t *testing.T) {
	body := strings.Join([]string{
		"event: response.incomplete",
		`data: {"type":"response.incomplete","response":{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}]}}`,
		"",
	}, "\n")

	require.True(t, containsOpenCodeImageGenerationSSE(body))
}

func countGeneratedImageMetadataFiles(t *testing.T, store *OpenAIGeneratedImageStore) int {
	t.Helper()
	entries, err := os.ReadDir(store.root)
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}
