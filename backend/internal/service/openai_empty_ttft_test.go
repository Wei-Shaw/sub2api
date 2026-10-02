package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAITTFTEmptyEventClassification(t *testing.T) {
	for _, data := range []string{
		`{"type":"response.output_text.delta","delta":"","SSE-Keep-Alive":true}`,
		`{"type":"response.output_text.delta","delta":""}`,
		`{"type":"response.output_text.delta"}`,
		`{"type":"response.output_text.delta","delta":null}`,
		`{"type":"response.output_text.delta","delta":{}}`,
		`{"type":"response.output_text.delta","delta":[]}`,
		`{"type":"response.output_text.delta","delta":42}`,
		`{"type":"response.output_text.delta","delta":true}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[]}}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[{}],"encrypted_content":""}}`,
		`{"type":"response.output_item.added","item":{"type":"message","content":[]}}`,
		`{"type":"response.output_item.added","item":{"type":"function_call","name":"test","arguments":""}}`,
		`{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`,
		`{"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":""}}`,
		`{"type":"response.output_item.done","item":{"type":"image_generation_call","result":""}}`,
		`{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`{"type":"response.done","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`{"type":"error","error":{"code":"content_policy","message":"blocked"}}`,
		`{"type":"response.failed","response":{"error":{"code":"content_policy","message":"blocked"}}}`,
		`{"type":"response.output_text.delta","delta":"broken"`,
	} {
		t.Run(data, func(t *testing.T) {
			for _, mode := range []string{OpenAITTFTModeSemantic, OpenAITTFTModeVisible} {
				require.False(t, openAIStreamDataStartsTTFT(data, "", mode), mode)
			}
			require.False(t, isOpenAIWSTokenEvent(gjson.Get(data, "type").String(), []byte(data)))
		})
	}
}

func TestOpenAIResponsesEmptyDeltaPreservesFailureBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, tc := range []struct {
			name         string
			content      string
			code         string
			wantFailover bool
		}{
			{name: "empty before retryable failure", code: "server_error", wantFailover: true},
			{name: "empty before forced failure", code: "content_policy_violation"},
			{name: "content before retryable failure", content: "OK", code: "server_error"},
		} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", tc.name, passthrough), func(t *testing.T) {
				data := `{"type":"response.output_text.delta","delta":""}`
				sse := "data: " + data + "\n\n"
				if tc.content != "" {
					sse += "data: " + fmt.Sprintf(`{"type":"response.output_text.delta","delta":%q}`, tc.content) + "\n\n"
				}
				terminal := fmt.Sprintf(`{"type":"response.failed","response":{"id":"resp_failure","error":{"code":%q,"message":"synthetic failure"},"usage":{"input_tokens":2,"output_tokens":1}}}`, tc.code)
				sse += "data: " + terminal + "\n\n"
				svc := &OpenAIGatewayService{cfg: &config.Config{}}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(sse))}
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
				var firstTokenMs *int
				var usage *OpenAIUsage
				var err error
				if passthrough {
					var result *openaiStreamingResultPassthrough
					result, err = svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "test-model", "test-model")
					require.NotNil(t, result)
					firstTokenMs, usage = result.firstTokenMs, result.usage
				} else {
					var result *openaiStreamingResult
					result, err = svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "test-model", "test-model")
					require.NotNil(t, result)
					firstTokenMs, usage = result.firstTokenMs, result.usage
				}
				var failover *UpstreamFailoverError
				if tc.wantFailover {
					require.ErrorAs(t, err, &failover)
					require.Empty(t, recorder.Body.String())
				} else {
					require.NotErrorAs(t, err, &failover)
					require.Contains(t, recorder.Body.String(), data)
					require.Contains(t, recorder.Body.String(), `"type":"response.failed"`)
				}
				if tc.content == "" {
					require.Nil(t, firstTokenMs)
				} else {
					require.NotNil(t, firstTokenMs)
				}
				require.Equal(t, 2, usage.InputTokens)
				require.Equal(t, 1, usage.OutputTokens)
			})
		}
	}
}

func TestOpenAITTFTErrorIsNotContent(t *testing.T) {
	for _, mode := range []string{OpenAITTFTModeSemantic, OpenAITTFTModeVisible} {
		require.False(t, openAIStreamDataStartsTTFT(`{"type":"response.failed"}`, "response.failed", mode))
	}
}

func TestOpenAIResponsesEmptyEventsLeaveTTFTUnobserved(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, mode := range []string{OpenAITTFTModeSemantic, OpenAITTFTModeVisible} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", mode, passthrough), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					result := runSyntheticVisibleTTFTStream(t, passthrough, time.Second, 0, mode,
						`{"type":"response.output_text.delta","delta":"","SSE-Keep-Alive":true}`)
					require.Nil(t, result.firstTokenMs)
				})
			})
		}
	}
}

func TestOpenAIResponsesEmptyEventsWaitForContent(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, mode := range []string{OpenAITTFTModeSemantic, OpenAITTFTModeVisible} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", mode, passthrough), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					result := runSyntheticVisibleTTFTStream(t, passthrough, time.Second, 0, mode,
						`{"type":"response.output_text.delta","delta":" "}`)
					require.NotNil(t, result.firstTokenMs)
					require.Equal(t, 1000, *result.firstTokenMs)
				})
			})
		}
	}
}
