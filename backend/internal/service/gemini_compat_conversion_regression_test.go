//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func forwardGeminiCompatFixture(t *testing.T, endpoint, mode string, payloads []string, configure func(*GeminiMessagesCompatService, *gin.Context)) (*ForwardResult, error, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, upstream := newGeminiVertexAPIKeyTestService()
	account := geminiVertexAPIKeyTestAccount("")
	upstreamBody := strings.Join(payloads, "")
	if mode != "json" {
		var stream strings.Builder
		for _, payload := range payloads {
			if mode == "buffered" {
				payload = `{"response":` + payload + `}`
			}
			stream.WriteString("data: " + payload + "\n\n")
		}
		upstreamBody = stream.String()
	}
	upstream.response = geminiEmptyCompletionTestResp(upstreamBody)
	if mode == "buffered" {
		svc.tokenProvider = &GeminiTokenProvider{}
		account = &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test-token", "project_id": "test-project"}}
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(fmt.Sprintf(`{"model":"gemini-2.5-pro","max_tokens":16,"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, mode == "stream"))
	path := "/v1/messages"
	if endpoint == "chat completions" {
		path = "/v1/chat/completions"
	}
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if configure != nil {
		configure(svc, c)
	}
	var result *ForwardResult
	var err error
	if endpoint == "messages" {
		result, err = svc.Forward(context.Background(), c, account, body)
	} else {
		result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body)
	}
	require.Equal(t, 1, upstream.calls)
	return result, err, c, rec
}

func TestGeminiCompatErrorEnvelope_PreservesHTTPClassification(t *testing.T) {
	for _, endpoint := range []string{"messages", "chat completions"} {
		for _, mode := range []string{"stream", "json", "buffered"} {
			for _, status := range []int{400, 404, 429} {
				t.Run(fmt.Sprintf("%s/%s/%d", endpoint, mode, status), func(t *testing.T) {
					payload := fmt.Sprintf(`{"error":{"code":%d,"message":"Invalid request parameter","status":"%s"}}`, status, map[int]string{400: "INVALID_ARGUMENT", 404: "NOT_FOUND", 429: "RESOURCE_EXHAUSTED"}[status])
					result, err, c, rec := forwardGeminiCompatFixture(t, endpoint, mode, []string{payload}, nil)
					require.Error(t, err)
					require.Nil(t, result)
					var failover *UpstreamFailoverError
					if status == 429 {
						require.ErrorAs(t, err, &failover)
						require.Equal(t, status, failover.StatusCode)
						require.True(t, failover.ShouldRetryNextAccount())
						require.False(t, c.Writer.Written())
					} else {
						require.False(t, errors.As(err, &failover), "deterministic request errors must not replay")
						require.Equal(t, status, rec.Code)
						if status == 400 {
							require.Contains(t, rec.Body.String(), "Invalid request parameter")
						}
					}
				})
			}
		}
	}
}

func TestGeminiCompatErrorEnvelope_PreservesTerminalPassthroughBody(t *testing.T) {
	// The keyword is beyond the signal detector's diagnostic-body limit.
	payload := `{"error":{"code":400,"message":"invalid parameter","status":"INVALID_ARGUMENT","details":"` + strings.Repeat("x", 2300) + `match-at-end"}}`
	for _, endpoint := range []string{"messages", "chat completions"} {
		for _, mode := range []string{"stream", "json", "buffered"} {
			t.Run(endpoint+"/"+mode, func(t *testing.T) {
				result, err, _, rec := forwardGeminiCompatFixture(t, endpoint, mode, []string{payload}, func(_ *GeminiMessagesCompatService, c *gin.Context) {
					rule := newNonFailoverPassthroughRule(400, "match-at-end", 422, "configured explanation")
					BindErrorPassthroughService(c, newTestService([]*model.ErrorPassthroughRule{rule}))
				})
				require.Nil(t, result)
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.False(t, errors.As(err, &failover))
				require.Equal(t, 422, rec.Code)
				require.Contains(t, rec.Body.String(), "configured explanation")
			})
		}
	}
}

func TestGeminiCompatErrorEnvelope_RespectsBodyLoggingSettings(t *testing.T) {
	payload := `{"error":{"code":429,"message":"rate limited","status":"RESOURCE_EXHAUSTED","details":"private-request-material"}}`
	for _, endpoint := range []string{"messages", "chat completions"} {
		for _, mode := range []string{"stream", "json", "buffered"} {
			for _, enabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/log=%t", endpoint, mode, enabled), func(t *testing.T) {
					_, err, c, _ := forwardGeminiCompatFixture(t, endpoint, mode, []string{payload}, func(s *GeminiMessagesCompatService, _ *gin.Context) {
						s.cfg.Gateway.LogUpstreamErrorBody = enabled
						s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes = 24
					})
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.JSONEq(t, payload, string(failover.ResponseBody), "classification must retain the full body independently of logged detail")
					events := upstreamErrorEventsFromContext(t, c)
					require.Len(t, events, 1)
					if enabled {
						require.Equal(t, payload[:24], events[0].Detail)
					} else {
						require.Empty(t, events[0].Detail)
					}
					require.NotContains(t, events[0].Message, "private-request-material")
				})
			}
		}
	}
}

func TestGeminiCompatNonStreaming_ThoughtOnlyIsNotBillableContent(t *testing.T) {
	payload := `{"candidates":[{"content":{"parts":[{"text":"reasoning only","thought":true}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"thoughtsTokenCount":20}}`
	for _, endpoint := range []string{"messages", "chat completions"} {
		for _, mode := range []string{"json", "buffered"} {
			t.Run(endpoint+"/"+mode, func(t *testing.T) {
				result, err, c, rec := forwardGeminiCompatFixture(t, endpoint, mode, []string{payload}, nil)
				require.Nil(t, result)
				requireGeminiEmptyCompletionFailover(t, err, rec, c)
			})
		}
	}
}

func TestGeminiCompatBuffered_PreservesContentAndTerminalMetadata(t *testing.T) {
	for _, endpoint := range []string{"messages", "chat completions"} {
		for _, content := range []string{"tool", "image", "text"} {
			t.Run(endpoint+"/"+content, func(t *testing.T) {
				part := map[string]string{
					"tool":  `{"functionCall":{"name":"read","args":{"path":"example"}}}`,
					"image": `{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}`,
					"text":  `{"text":"visible response"}`,
				}[content]
				payloads := []string{`{"candidates":[{"content":{"parts":[` + part + `]}}]}`, `{"candidates":[{"content":{"parts":[{"text":""}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":9}}`}
				result, err, _, rec := forwardGeminiCompatFixture(t, endpoint, "buffered", payloads, nil)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 10, result.Usage.InputTokens)
				require.Equal(t, 9, result.Usage.OutputTokens)
				switch content {
				case "tool":
					require.Contains(t, rec.Body.String(), `"name":"read"`)
					if endpoint == "messages" {
						require.Contains(t, rec.Body.String(), `"tool_use"`)
					} else {
						require.Contains(t, rec.Body.String(), `"tool_calls"`)
					}
				case "image":
					if endpoint == "chat completions" {
						require.Contains(t, rec.Body.String(), "data:image/png;base64,aW1hZ2U=")
					}
				case "text":
					require.Contains(t, rec.Body.String(), "visible response")
				}
			})
		}
	}
}

func TestGeminiCompatBuffered_EmptyMaxTokensPreservesWireFinishAndUsage(t *testing.T) {
	payloads := []string{`{"candidates":[{"content":{"parts":[{"text":""}]}}]}`, `{"candidates":[{"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":10,"thoughtsTokenCount":2}}`}
	for _, endpoint := range []string{"messages", "chat completions"} {
		t.Run(endpoint, func(t *testing.T) {
			result, err, _, rec := forwardGeminiCompatFixture(t, endpoint, "buffered", payloads, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &wire))
			if endpoint == "messages" {
				require.Equal(t, "max_tokens", wire["stop_reason"])
				require.Equal(t, float64(10), wire["usage"].(map[string]any)["input_tokens"])
				require.Equal(t, float64(2), wire["usage"].(map[string]any)["output_tokens"])
			} else {
				require.Contains(t, rec.Body.String(), `"finish_reason":"length"`)
				require.Equal(t, float64(10), wire["usage"].(map[string]any)["prompt_tokens"])
				require.Equal(t, float64(2), wire["usage"].(map[string]any)["completion_tokens"])
			}
		})
	}
}

func TestGeminiCompatBuffered_ConsolidatesVisibleTextWithoutThoughts(t *testing.T) {
	payloads := []string{
		`{"candidates":[{"content":{"parts":[{"text":"private reasoning","thought":true},{"text":"hello"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":" world"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":""}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"thoughtsTokenCount":2}}`,
	}
	for _, endpoint := range []string{"messages", "chat completions"} {
		t.Run(endpoint, func(t *testing.T) {
			result, err, _, rec := forwardGeminiCompatFixture(t, endpoint, "buffered", payloads, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 5, result.Usage.OutputTokens)
			require.Contains(t, rec.Body.String(), "hello world")
			require.Equal(t, 1, strings.Count(rec.Body.String(), "hello world"))
			require.NotContains(t, rec.Body.String(), "private reasoning")
			require.NotContains(t, rec.Body.String(), "hellohello")
		})
	}
}

func TestGeminiCompatBuffered_ConsolidatesIdentifiedToolSnapshots(t *testing.T) {
	payloads := []string{
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call-1","name":"read","args":{}}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call-1","name":"read","args":{"path":"example"}}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":""}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":9}}`,
	}
	for _, endpoint := range []string{"messages", "chat completions"} {
		t.Run(endpoint, func(t *testing.T) {
			result, err, _, rec := forwardGeminiCompatFixture(t, endpoint, "buffered", payloads, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, strings.Count(rec.Body.String(), `"name":"read"`))
			require.Contains(t, rec.Body.String(), "example")
		})
	}
}

func TestGeminiCompatBuffered_PreservesRepeatedParts(t *testing.T) {
	call := `{"functionCall":{"name":"read","args":{}}}`
	image := `{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}`
	for _, endpoint := range []string{"messages", "chat completions"} {
		for _, kind := range []string{"calls in one event", "calls across events", "text deltas", "images"} {
			t.Run(endpoint+"/"+kind, func(t *testing.T) {
				var payloads []string
				switch kind {
				case "calls in one event":
					payloads = []string{`{"candidates":[{"content":{"parts":[` + call + `,` + call + `]}}]}`}
				case "calls across events":
					payloads = []string{`{"candidates":[{"content":{"parts":[` + call + `]}}]}`, `{"candidates":[{"content":{"parts":[` + call + `]}}]}`}
				case "text deltas":
					payloads = []string{`{"candidates":[{"content":{"parts":[{"text":"ha"}]}}]}`, `{"candidates":[{"content":{"parts":[{"text":"ha"}]}}]}`}
				case "images":
					payloads = []string{`{"candidates":[{"content":{"parts":[` + image + `,` + image + `]}}]}`}
				}
				payloads = append(payloads, `{"candidates":[{"content":{"parts":[{"text":""}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":9}}`)
				result, err, _, rec := forwardGeminiCompatFixture(t, endpoint, "buffered", payloads, nil)
				require.NoError(t, err)
				require.NotNil(t, result)
				switch kind {
				case "calls in one event", "calls across events":
					require.Equal(t, 2, strings.Count(rec.Body.String(), `"name":"read"`), "identical unidentified calls can be distinct invocations")
				case "text deltas":
					require.Contains(t, rec.Body.String(), "haha")
				case "images":
					if endpoint == "chat completions" {
						require.Equal(t, 2, strings.Count(rec.Body.String(), "data:image/png;base64,aW1hZ2U="))
					}
				}
			})
		}
	}
}
