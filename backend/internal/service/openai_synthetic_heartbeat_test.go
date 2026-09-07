package service

import (
	"errors"
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

const syntheticHeartbeat = `{"type":"response.output_text.delta","item_id":"SSE-Keep-Alive","SSE-Keep-Alive":true,"delta":"\u200b"}`

const emptyMessagePreamble = "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\",\"content\":[]}}\n\n" +
	"data: {\"type\":\"response.content_part.added\",\"part\":{\"type\":\"output_text\",\"text\":\"\"}}\n\n" +
	"data: {\"type\":\"response.output_text.delta\",\"delta\":\" \"}\n\n"

func TestSyntheticHeartbeatOutputBoundary(t *testing.T) {
	for _, classify := range []func(string, string) bool{openAIStreamDataStartsClientOutput, openAIStreamDataStartsVisibleOutput, openAIStreamDataStartsSemanticTTFT} {
		require.False(t, classify(syntheticHeartbeat, "response.output_text.delta"))
		require.False(t, classify(syntheticHeartbeat, ""))
		for _, payload := range []string{
			strings.Replace(syntheticHeartbeat, `"SSE-Keep-Alive":true`, `"SSE-Keep-Alive":false`, 1),
			strings.Replace(syntheticHeartbeat, `"SSE-Keep-Alive":true`, `"SSE-Keep-Alive":"true"`, 1),
			strings.Replace(syntheticHeartbeat, `"item_id":"SSE-Keep-Alive"`, `"item_id":"msg_1"`, 1),
			`{"type":"response.output_text.delta","delta":" "}`,
			`{"type":"response.output_text.delta","delta":"\u200b"}`,
		} {
			require.False(t, classify(payload, "response.output_text.delta"), payload)
		}
		require.True(t, classify(strings.Replace(syntheticHeartbeat, `"delta":"\u200b"`, `"delta":"\u200banswer"`, 1), "response.output_text.delta"))
	}
	require.False(t, openAIStreamTextDeltaIsOnlyFiller(syntheticHeartbeat, "response.function_call_arguments.delta"))
	require.False(t, openAIStreamTextDeltaIsOnlyFiller(syntheticHeartbeat+":", "response.output_text.delta"))
}

func TestSyntheticHeartbeatPreservesStreamRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, withHeartbeat := range []bool{false, true} {
			for _, realOutput := range []bool{false, true} {
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n"
				if withHeartbeat {
					body += "data: " + syntheticHeartbeat + "\n\n"
				}
				body += emptyMessagePreamble
				if realOutput {
					body += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n"
				}
				body += "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"An error occurred while processing your request.\"}}}\n\n"
				resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
				account := &Account{ID: 1, Platform: PlatformOpenAI, Name: "test"}
				var err error
				if passthrough {
					_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-6-astra")
				} else {
					_, err = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-6-astra")
				}
				require.Error(t, err)
				var fe *UpstreamFailoverError
				require.Equal(t, !realOutput, errors.As(err, &fe), "passthrough=%v heartbeat=%v output=%v", passthrough, withHeartbeat, realOutput)
				require.Equal(t, realOutput, c.Writer.Written())
				if !realOutput {
					require.Empty(t, rec.Body.String())
				} else {
					require.Contains(t, rec.Body.String(), "OK")
				}
			}
		}
	}
}

func TestSyntheticHeartbeatPreservesSuccessfulStream(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		body := "data: " + syntheticHeartbeat + "\n\n" + emptyMessagePreamble +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
		resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
		account := &Account{ID: 1, Platform: PlatformOpenAI, Name: "test"}
		if passthrough {
			result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-6-astra")
			require.NoError(t, err)
			require.NotNil(t, result)
		} else {
			result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-6-astra")
			require.NoError(t, err)
			require.NotNil(t, result)
		}
		require.Equal(t, body, rec.Body.String())
	}
}

// Exercise actual HTTP response delivery around both streaming adapters. Only
// account selection/retry allowance is mocked; the adapters decide replay safety.
func TestPreparationPrefixHTTPRecovery(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, mode := range []string{"retry_success", "exhausted", "real_output"} {
			t.Run(fmt.Sprintf("passthrough=%v/%s", passthrough, mode), func(t *testing.T) {
				prefix := "data: " + syntheticHeartbeat + "\n\n" + emptyMessagePreamble
				failed := prefix
				if mode == "real_output" {
					failed += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
				}
				failed += "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"upstream processing failed\"}}}\n\n"
				good := "data: {\"type\":\"response.output_text.delta\",\"delta\":\" \"}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\" OK\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					if calls.Add(1) == 1 || mode == "exhausted" {
						_, _ = io.WriteString(w, failed)
					} else {
						_, _ = io.WriteString(w, good)
					}
				}))
				defer upstream.Close()
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
				engine := gin.New()
				engine.POST("/v1/responses", func(c *gin.Context) {
					for attempt := 0; attempt < 2; attempt++ {
						resp, err := http.Get(upstream.URL)
						if err != nil {
							c.Status(502)
							return
						}
						account := &Account{ID: int64(attempt + 1), Platform: PlatformOpenAI, Name: "mock"}
						if passthrough {
							_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-6-astra")
						} else {
							_, err = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-6-astra")
						}
						_ = resp.Body.Close()
						if err == nil {
							return
						}
						var fe *UpstreamFailoverError
						if !errors.As(err, &fe) {
							return
						}
						if attempt == 1 {
							c.JSON(502, gin.H{"error": "mock recovery exhausted"})
							return
						}
					}
				})
				proxy := httptest.NewServer(engine)
				defer proxy.Close()
				resp, err := http.Post(proxy.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"gpt-6-astra","stream":true}`))
				require.NoError(t, err)
				data, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				require.NoError(t, err)
				switch mode {
				case "retry_success":
					require.Equal(t, int32(2), calls.Load())
					require.Equal(t, 200, resp.StatusCode)
					require.Equal(t, good, string(data))
				case "exhausted":
					require.Equal(t, int32(2), calls.Load())
					require.Equal(t, 502, resp.StatusCode)
					require.NotContains(t, string(data), "SSE-Keep-Alive")
				case "real_output":
					require.Equal(t, int32(1), calls.Load())
					require.Contains(t, string(data), "partial")
					require.NotContains(t, string(data), `"delta":"OK"`)
				}
			})
		}
	}
}
