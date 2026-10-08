package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

func TestOpenAICacheWriteOutputCapture_HTTPHandlersKeepRawEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// The passthrough SSE path deduplicates this arguments string for clients.
	// Billing evidence must still describe the original upstream output.
	response := `{"id":"resp_evidence","model":"upstream","status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}{}"}],"usage":{"input_tokens":100,"output_tokens":10}}`
	want := buildOpenAICacheWriteOutputEvidence([]byte(response))
	if !want.Valid {
		t.Fatal("fixture must provide complete output evidence")
	}
	for _, mode := range []string{"json", "stream", "buffered"} {
		for _, passthrough := range []bool{false, true} {
			name := mode
			if passthrough {
				name += "/passthrough"
			}
			t.Run(name, func(t *testing.T) {
				body, contentType := response, "application/json"
				if mode != "json" {
					body = "data: " + `{"type":"response.completed","response":` + response + `}` + "\n\n"
					contentType = "text/event-stream"
				}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
				svc := &OpenAIGatewayService{cfg: &config.Config{}}
				account := &Account{ID: 1, Type: AccountTypeAPIKey}
				var evidence openAICacheWriteOutputEvidence
				var err error
				if mode == "stream" {
					if passthrough {
						var result *openaiStreamingResultPassthrough
						result, err = svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "client", "upstream")
						if result != nil {
							evidence = result.cacheWriteOutputEvidence
						}
					} else {
						var result *openaiStreamingResult
						result, err = svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "client", "upstream")
						if result != nil {
							evidence = result.cacheWriteOutputEvidence
						}
					}
				} else if passthrough {
					var result *openaiNonStreamingResultPassthrough
					result, err = svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "client", "upstream")
					if result != nil {
						evidence = result.cacheWriteOutputEvidence
					}
				} else {
					var result *openaiNonStreamingResult
					result, err = svc.handleNonStreamingResponse(context.Background(), resp, c, account, "client", "upstream")
					if result != nil {
						evidence = result.cacheWriteOutputEvidence
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(evidence, want) {
					t.Fatal("response handler lost or rewrote the original output evidence")
				}
				if mode == "stream" && passthrough && strings.Contains(recorder.Body.String(), `"arguments":"{}{}"`) {
					t.Fatal("fixture must exercise client-facing argument normalization")
				}
			})
		}
	}
}

func TestOpenAICacheWriteOutputCapture_TypedConversionCannotCreateEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{"chat", "messages"} {
		for _, stream := range []bool{false, true} {
			name := protocol
			if stream {
				name += "/stream"
			}
			t.Run(name, func(t *testing.T) {
				// Typed Responses parsing discards future_payload. Capturing after
				// conversion would incorrectly accept this unknown output shape.
				body := "data: " + `{"type":"response.completed","response":{"id":"resp_evidence","model":"test-model","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}],"future_payload":"unknown"}],"usage":{"input_tokens":100,"output_tokens":10}}}` + "\n\n"
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
				svc := &OpenAIGatewayService{cfg: &config.Config{}}
				account := &Account{ID: 1, Type: AccountTypeAPIKey}
				var result *OpenAIForwardResult
				var err error
				switch {
				case protocol == "chat" && stream:
					result, err = svc.handleChatStreamingResponse(resp, c, account, "test-model", "test-model", "test-model", time.Now(), 0)
				case protocol == "chat":
					result, err = svc.handleChatBufferedStreamingResponse(resp, c, account, "test-model", "test-model", "test-model", time.Now())
				case stream:
					result, err = svc.handleAnthropicStreamingResponse(resp, c, account, "test-model", "test-model", "test-model", time.Now())
				default:
					result, err = svc.handleAnthropicBufferedStreamingResponse(resp, c, account, "test-model", "test-model", "test-model", time.Now())
				}
				if err != nil {
					t.Fatal(err)
				}
				if result == nil || result.CacheWriteOutputEvidence.Valid {
					t.Fatal("unknown raw output must remain unavailable after typed conversion")
				}
			})
		}
	}
}
