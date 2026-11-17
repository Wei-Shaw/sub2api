//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func chatObservationResponse(t *testing.T, stream bool, model string, conflict, readError, invalidTool bool) *http.Response {
	t.Helper()
	encode := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return string(b)
	}
	usage := map[string]int{"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30}
	payload := map[string]any{"id": "chatcmpl_observed", "service_tier": "default", "usage": usage}
	if model != "" {
		payload["model"] = model
	}
	payload["choices"] = []any{map[string]any{
		"index": 0, "message": map[string]string{"role": "assistant", "content": "observed response"}, "finish_reason": "stop",
	}}
	wire := encode(payload)
	contentType := "application/json"
	if stream {
		contentType = "text/event-stream"
		payload["choices"] = []any{map[string]any{
			"index": 0, "delta": map[string]string{"role": "assistant", "content": "observed response"}, "finish_reason": nil,
		}}
		if invalidTool {
			payload["choices"] = []any{map[string]any{"index": 0, "delta": map[string]any{
				"tool_calls": []any{map[string]any{"index": 0, "id": "call_bad", "type": "function",
					"function": map[string]string{"name": "lookup", "arguments": "{"}}},
			}, "finish_reason": nil}}
		}
		wire = "data: " + encode(payload) + "\n\n"
		if conflict {
			payload["model"] = "conflicting-model"
			wire += "data: " + encode(payload) + "\n\n"
		}
		if !readError {
			wire += "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":10,\"total_tokens\":30}}\n\n" +
				"data: [DONE]\n\n"
		}
	}
	var body io.ReadCloser = io.NopCloser(strings.NewReader(wire))
	if readError {
		body = &errTailReader{data: []byte(wire), err: io.ErrUnexpectedEOF}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{
		"Content-Type": []string{contentType}, "X-Request-Id": []string{"chat-observed"},
	}, Body: body}
}

func TestChatFallbackResponseObservationReachesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"messages", "responses"} {
		for _, tt := range []struct {
			name        string
			stream      bool
			model       string
			conflict    bool
			readError   bool
			disconnect  bool
			invalidTool bool
		}{
			{name: "buffered matching", model: "gpt-5.4"},
			{name: "buffered mismatch", model: "reported-model"},
			{name: "buffered undeclared"},
			{name: "stream matching", stream: true, model: "gpt-5.4"},
			{name: "stream mismatch", stream: true, model: "reported-model"},
			{name: "stream undeclared", stream: true},
			{name: "stream conflicting", stream: true, model: "reported-model", conflict: true},
			{name: "stream read error", stream: true, model: "reported-model", readError: true},
			{name: "client disconnected", stream: true, model: "reported-model", disconnect: true},
			{name: "invalid tool arguments", stream: true, model: "reported-model", invalidTool: true},
		} {
			if tt.invalidTool && endpoint != "responses" {
				continue
			}
			t.Run(endpoint+"/"+tt.name, func(t *testing.T) {
				request := map[string]any{"model": "gpt-5.4", "stream": tt.stream}
				if endpoint == "messages" {
					request["max_tokens"] = 32
					request["messages"] = []map[string]string{{"role": "user", "content": "hi"}}
				} else {
					request["input"] = "hi"
				}
				body, err := json.Marshal(request)
				require.NoError(t, err)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, strings.NewReader(string(body)))
				c.Request.Header.Set("Content-Type", "application/json")
				if tt.disconnect {
					c.Writer = &failWriteResponseWriter{ResponseWriter: c.Writer}
				}
				upstream := &httpUpstreamRecorder{resp: chatObservationResponse(t, tt.stream, tt.model, tt.conflict, tt.readError, tt.invalidTool)}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				account := forceChatMessagesFallbackAccount()
				var result *OpenAIForwardResult
				if endpoint == "messages" {
					result, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
				} else {
					account = forceChatResponsesFallbackAccount()
					result, err = svc.Forward(context.Background(), c, account, body)
				}
				if tt.readError || tt.invalidTool {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				require.NotNil(t, result)
				require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
				require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Equal(t, "gpt-5.4", result.Model)
				require.Equal(t, "gpt-5.4", result.UpstreamModel)
				require.Equal(t, 20, result.Usage.InputTokens)
				require.Equal(t, 10, result.Usage.OutputTokens)
				if !tt.disconnect && !tt.invalidTool {
					require.Contains(t, rec.Body.String(), "observed response")
				}
				require.Equal(t, tt.model, observedUpstreamResponseModel(c), "the shared pipeline already saw the upstream model")
				wantTier := "default"
				if tt.model == "" {
					wantTier = ""
				}
				require.Equal(t, wantTier, result.UpstreamResponseServiceTier)

				usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
				usageSvc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
				require.NoError(t, usageSvc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
					Result: result, APIKey: &APIKey{ID: 10}, User: &User{ID: 20}, Account: account,
				}))
				require.NotNil(t, usageRepo.lastLog)
				log := usageRepo.lastLog
				if tt.model == "" {
					require.Nil(t, log.UpstreamResponseModel)
					require.Nil(t, log.UpstreamModelMismatch)
				} else {
					require.NotNil(t, log.UpstreamResponseModel)
					require.Equal(t, tt.model, *log.UpstreamResponseModel)
					require.NotNil(t, log.UpstreamModelMismatch)
					require.Equal(t, tt.model != "gpt-5.4", *log.UpstreamModelMismatch)
				}
				require.Equal(t, tt.model, result.UpstreamResponseModel)
				require.Equal(t, tt.conflict, result.UpstreamResponseModelConflict)
			})
		}
	}
}
