package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const syntheticHeartbeat = `{"type":"response.output_text.delta","item_id":"SSE-Keep-Alive","SSE-Keep-Alive":true,"delta":"\u200b"}`

func TestSyntheticHeartbeatOutputBoundary(t *testing.T) {
	for _, classify := range []func(string, string) bool{openAIStreamDataStartsClientOutput, openAIStreamDataStartsVisibleOutput, openAIStreamDataStartsSemanticTTFT} {
		require.False(t, classify(syntheticHeartbeat, "response.output_text.delta"))
		require.False(t, classify(syntheticHeartbeat, ""))
		for _, payload := range []string{
			strings.Replace(syntheticHeartbeat, `"SSE-Keep-Alive":true`, `"SSE-Keep-Alive":false`, 1),
			strings.Replace(syntheticHeartbeat, `"SSE-Keep-Alive":true`, `"SSE-Keep-Alive":"true"`, 1),
			strings.Replace(syntheticHeartbeat, `"item_id":"SSE-Keep-Alive"`, `"item_id":"msg_1"`, 1),
			strings.Replace(syntheticHeartbeat, `"delta":"\u200b"`, `"delta":"\u200banswer"`, 1),
			`{"type":"response.output_text.delta","delta":" "}`,
			`{"type":"response.output_text.delta","delta":"\u200b"}`,
		} {
			require.True(t, classify(payload, "response.output_text.delta"), payload)
		}
	}
	require.False(t, openAIStreamIsSyntheticHeartbeat(syntheticHeartbeat, "response.function_call_arguments.delta"))
	require.False(t, openAIStreamIsSyntheticHeartbeat(syntheticHeartbeat+":", "response.output_text.delta"))
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
		body := "data: " + syntheticHeartbeat + "\n\n" +
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
