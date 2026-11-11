//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCCInBandErrorsBeforeOutputFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/v1/responses", "/v1/chat/completions"} {
		for _, payload := range []string{`{"error":{"type":"rate_limit_error","code":"RATE_LIMIT","message":"too many requests"}}`, `{"error":{"type":"server_error","message":"unreachable"}}`} {
			t.Run(endpoint+payload, func(t *testing.T) {
				raw := strings.Join([]string{": ping", "", `data: {"choices":[{"delta":{"role":"assistant","content":""}}]}`, "", "data: " + payload, "", "data: [DONE]", ""}, "\n")
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(raw))}}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				body := []byte(`{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}],"input":"hi","stream":true}`)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
				account := forceChatResponsesFallbackAccount()
				var result *OpenAIForwardResult
				var err error
				if endpoint == "/v1/responses" {
					result, err = svc.Forward(context.Background(), c, account, body)
				} else {
					result, err = svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "glm-5.3-flash")
				}
				require.Error(t, err)
				require.Nil(t, result)
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				if strings.Contains(payload, "RATE_LIMIT") {
					require.Equal(t, 429, failover.StatusCode)
				} else {
					require.Equal(t, 502, failover.StatusCode)
				}
				require.Empty(t, rec.Body.String(), "heartbeat and role placeholders must not commit the client response")
			})
		}
	}
}

func TestCCInBandErrorAfterOutputDoesNotCompleteSuccessfully(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/v1/responses", "/v1/chat/completions"} {
		t.Run(endpoint, func(t *testing.T) {
			raw := `data: {"choices":[{"delta":{"content":"partial answer"}}]}` + "\n\n" + `data: {"error":{"type":"server_error","message":"upstream failed"}}` + "\n\ndata: [DONE]\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(raw))}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			body := []byte(`{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}],"input":"hi","stream":true}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
			account := forceChatResponsesFallbackAccount()
			var err error
			if endpoint == "/v1/responses" {
				_, err = svc.Forward(context.Background(), c, account, body)
			} else {
				_, err = svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "glm-5.3-flash")
			}
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			require.Contains(t, rec.Body.String(), "upstream failed")
			require.NotContains(t, rec.Body.String(), "response.completed")
			if endpoint == "/v1/responses" {
				var responseID string
				lastSequence := int64(-1)
				for _, line := range strings.Split(rec.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					payload := gjson.Parse(strings.TrimPrefix(line, "data: "))
					sequence := payload.Get("sequence_number").Int()
					require.Greater(t, sequence, lastSequence)
					lastSequence = sequence
					if payload.Get("type").String() == "response.created" {
						responseID = payload.Get("response.id").String()
					}
					if payload.Get("type").String() == "response.failed" {
						require.NotEmpty(t, responseID)
						require.Equal(t, responseID, payload.Get("response.id").String())
						require.Equal(t, "response", payload.Get("response.object").String())
						require.Positive(t, payload.Get("response.created_at").Int())
					}
				}
			}
		})
	}
}

func TestCCInBandErrorDetection(t *testing.T) {
	for _, tt := range []struct {
		name    string
		payload string
		message string
	}{
		{"chat error", `{"error":{"message":"rate limited"}}`, "rate limited"},
		{"responses failure", `{"type":"response.failed","response":{"error":{"message":"upstream failed"}}}`, "upstream failed"},
		{"error event", `{"type":"error","message":"unavailable"}`, "unavailable"},
		{"null error", `{"error":null,"choices":[]}`, ""},
		{"content mentioning error", `{"choices":[{"delta":{"content":"error"}}]}`, ""},
		{"done", `[DONE]`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			failure := parseCCStreamUpstreamError(tt.payload)
			if tt.message == "" {
				require.Nil(t, failure)
				return
			}
			require.NotNil(t, failure)
			require.Equal(t, tt.message, failure.Message)
			require.Equal(t, tt.payload, string(failure.Payload))
		})
	}
}

func TestCCInBandEmptyStreamFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/v1/responses", "/v1/chat/completions"} {
		t.Run(endpoint, func(t *testing.T) {
			raw := ": ping\n\n" + `data: {"choices":[{"delta":{"role":"assistant","content":""},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(raw))}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			body := []byte(`{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}],"input":"hi","stream":true}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
			account := forceChatResponsesFallbackAccount()
			var err error
			if endpoint == "/v1/responses" {
				_, err = svc.Forward(context.Background(), c, account, body)
			} else {
				_, err = svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "glm-5.3-flash")
			}
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Empty(t, rec.Body.String())
		})
	}
}
