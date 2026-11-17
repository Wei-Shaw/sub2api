package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageLimitTestBody struct {
	*strings.Reader
	closed atomic.Bool
}

func (b *imageLimitTestBody) Close() error {
	b.closed.Store(true)
	return nil
}

type imageLimitDisconnectedWriter struct {
	gin.ResponseWriter
	attempts int
}

func (w *imageLimitDisconnectedWriter) Write([]byte) (int, error) {
	w.attempts++
	return 0, io.ErrClosedPipe
}

func (w *imageLimitDisconnectedWriter) WriteString(string) (int, error) {
	w.attempts++
	return 0, io.ErrClosedPipe
}

func TestImageStreamTransportLimitEmitsErrorAndPreservesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"direct", "responses", "passthrough"} {
		for _, async := range []bool{false, true} {
			for _, limit := range []string{"line", "event_bytes", "event_lines"} {
				t.Run(fmt.Sprintf("%s/async_%t/%s", route, async, limit), func(t *testing.T) {
					cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 1024, SSEEventMaxBytes: 4096, SSEQueueMaxBytes: 128}}
					if async {
						cfg.Gateway.ImageStreamDataIntervalTimeout = 1
					}
					wire := "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"AA==\",\"usage\":{\"input_tokens\":11,\"output_tokens\":7}}\n\n"
					pendingTerminal := "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"AA==\",\"usage\":{\"input_tokens\":999}}\n"
					if route == "responses" {
						wire = "data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":7}}}\n\n" +
							"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"image_generation_call\",\"id\":\"image-pending\",\"result\":\"AA==\"}}\n\n"
						pendingTerminal = "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":999},\"output\":[{\"type\":\"image_generation_call\",\"result\":\"AA==\"}]}}\n"
					}
					wire += pendingTerminal
					switch limit {
					case "line":
						cfg.Gateway.MaxLineSize = 256
						wire += ":" + strings.Repeat("x", 512)
					case "event_bytes":
						cfg.Gateway.SSEEventMaxBytes = 256
						wire += ":" + strings.Repeat("x", 150) + "\n:" + strings.Repeat("x", 150)
					case "event_lines":
						cfg.Gateway.SSEEventMaxLines = 2
						wire += ":first\n:second\n"
					}
					wire += "\n\ndata: [DONE]\n\n"
					body := &imageLimitTestBody{Reader: strings.NewReader(wire)}
					resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
					svc := &OpenAIGatewayService{cfg: cfg}
					var usage OpenAIUsage
					var count int
					var err error
					if route == "responses" {
						usage, count, _, _, err = svc.handleOpenAIImagesOAuthStreamingResponse(resp, c, time.Now(), "b64_json", "image_generation", "gpt-image-2")
					} else {
						var direct *OpenAIImagesRequest
						if route == "direct" {
							direct = &OpenAIImagesRequest{Model: "gpt-image-2", ResponseFormat: "b64_json"}
						}
						usage, count, _, _, err = svc.handleOpenAIImagesStreamingResponse(resp, c, time.Now(), direct)
					}
					require.ErrorIs(t, err, errSSETransportLimit)
					require.True(t, body.closed.Load(), "transport-limit failures must release the upstream body")
					wantUsage := OpenAIUsage{InputTokens: 11, OutputTokens: 7}
					if route == "direct" {
						wantUsage.ImageOutputTokens = 7
					}
					require.Equal(t, wantUsage, usage)
					require.Zero(t, count, "pending image output must not be finalized on a failed stream")
					require.NotContains(t, rec.Body.String(), ".completed")
					require.NotContains(t, rec.Body.String(), "[DONE]")
					requireImageTransportLimitError(t, rec.Body.String())
				})
			}
		}
	}
}

func TestImageStreamPassthroughPreservesRawFrames(t *testing.T) {
	completed := `{"type":"image_generation.completed","b64_json":"AA==","usage":{"input_tokens":11,"output_tokens":7},"unknown":{"keep":true}}`
	for _, async := range []bool{false, true} {
		for _, wire := range []string{
			": heartbeat\r\nevent: image_generation.completed\r\nid: opaque\r\ndata: " + completed + "\r\n\r\n: final comment",
			"event: image_generation.completed\ndata: " + completed,
		} {
			t.Run(fmt.Sprintf("async_%t/bytes_%d", async, len(wire)), func(t *testing.T) {
				cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 1024}}
				if async {
					cfg.Gateway.ImageStreamDataIntervalTimeout = 1
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
				body := &imageLimitTestBody{Reader: strings.NewReader(wire)}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}
				svc := &OpenAIGatewayService{cfg: cfg}
				usage, count, _, _, err := svc.handleOpenAIImagesStreamingResponse(resp, c, time.Now(), nil)
				require.NoError(t, err)
				require.Equal(t, wire, rec.Body.String(), "passthrough must retain CRLF, comments, unknown fields and unterminated EOF bytes")
				require.Equal(t, OpenAIUsage{InputTokens: 11, OutputTokens: 7}, usage)
				require.Equal(t, 1, count)
			})
		}
	}
}

func TestImageStreamTransportLimitSkipsDisconnectedClient(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		for _, async := range []bool{false, true} {
			t.Run(fmt.Sprintf("oauth_%t/async_%t", oauth, async), func(t *testing.T) {
				cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 32}}
				if async {
					cfg.Gateway.ImageStreamDataIntervalTimeout = 1
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil).WithContext(ctx)
				body := &imageLimitTestBody{Reader: strings.NewReader(":" + strings.Repeat("x", 64))}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}
				svc := &OpenAIGatewayService{cfg: cfg}
				var err error
				if oauth {
					_, _, _, _, err = svc.handleOpenAIImagesOAuthStreamingResponse(resp, c, time.Now(), "b64_json", "image_generation", "gpt-image-2")
				} else {
					_, _, _, _, err = svc.handleOpenAIImagesStreamingResponse(resp, c, time.Now(), nil)
				}
				require.ErrorIs(t, err, errSSETransportLimit)
				require.True(t, body.closed.Load())
				require.Empty(t, rec.Body.String())
			})
		}
	}
}

func TestImageStreamTransportLimitDoesNotRetryFailedClientWriter(t *testing.T) {
	for _, route := range []string{"direct", "responses", "passthrough"} {
		for _, async := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/async_%t", route, async), func(t *testing.T) {
				cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 512}}
				if async {
					cfg.Gateway.ImageStreamDataIntervalTimeout = 1
				}
				wire := "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"AA==\",\"usage\":{\"input_tokens\":11,\"output_tokens\":7}}\n\n"
				if route == "responses" {
					wire = "data: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"AA==\",\"usage\":{\"input_tokens\":11,\"output_tokens\":7}}\n\n"
				}
				wire += ":" + strings.Repeat("x", 1024)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				writer := &imageLimitDisconnectedWriter{ResponseWriter: c.Writer}
				c.Writer = writer
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
				body := &imageLimitTestBody{Reader: strings.NewReader(wire)}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}
				svc := &OpenAIGatewayService{cfg: cfg}
				var usage OpenAIUsage
				var err error
				if route == "responses" {
					usage, _, _, _, err = svc.handleOpenAIImagesOAuthStreamingResponse(resp, c, time.Now(), "b64_json", "image_generation", "gpt-image-2")
				} else {
					var direct *OpenAIImagesRequest
					if route == "direct" {
						direct = &OpenAIImagesRequest{Model: "gpt-image-2"}
					}
					usage, _, _, _, err = svc.handleOpenAIImagesStreamingResponse(resp, c, time.Now(), direct)
				}
				require.ErrorIs(t, err, errSSETransportLimit)
				require.True(t, body.closed.Load())
				require.Equal(t, 11, usage.InputTokens)
				require.Equal(t, 7, usage.OutputTokens)
				require.Equal(t, 1, writer.attempts, "do not write a transport-limit error after the client writer has failed")
			})
		}
	}
}

func requireImageTransportLimitError(t *testing.T, wire string) {
	t.Helper()
	errorEvents := 0
	for _, frame := range strings.Split(strings.ReplaceAll(wire, "\r\n", "\n"), "\n\n") {
		var eventName string
		var data []string
		for _, line := range strings.Split(frame, "\n") {
			if value, ok := strings.CutPrefix(line, "event:"); ok {
				eventName = strings.TrimSpace(value)
			}
			if value, ok := strings.CutPrefix(line, "data:"); ok {
				data = append(data, strings.TrimPrefix(value, " "))
			}
		}
		if eventName != "error" {
			continue
		}
		errorEvents++
		var payload struct {
			Type  string `json:"type"`
			Error struct {
				Type    string `json:"type"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.Join(data, "\n")), &payload), "the error must be a standalone, parseable SSE event")
		require.Equal(t, "error", payload.Type)
		require.Equal(t, "upstream_error", payload.Error.Type)
		require.Equal(t, "response_too_large", payload.Error.Code)
		require.NotEmpty(t, payload.Error.Message)
	}
	require.Equal(t, 1, errorEvents)
}
