package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIChatFailingWriter struct {
	gin.ResponseWriter
	failAfter int
	writes    int
}

func (w *openAIChatFailingWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed: client disconnected")
	}
	w.writes++
	return w.ResponseWriter.Write(p)
}

func TestForwardAsChatCompletions_ResponsesSupportedFallsBackWhenStructuredInputRequiresString(t *testing.T) {
	for name, rejection := range map[string]string{
		"OpenAI invalid type": `{"error":{"type":"invalid_request_error","code":"invalid_type","param":"input","message":"Invalid type for 'input': expected a string, but got an array instead."}}`,
		"vLLM validation":     `{"error":{"message":"1 validation error: [{'type':'string_type', 'loc':('body','input','str'), 'msg':'Input should be a valid string', 'input':[{'role':'user','content':'hi'}, {'role':'assistant','content':[{'type':'output_text'}]}, ResponseFunctionToolCall(type='function_call'), {'type':'function_call_output'}]}]"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"text","text":"checking"},{"type":"thinking","thinking":"think"}],"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"done"},{"role":"user","content":"what now"}],"stream":false}`)
			originalBody := append([]byte(nil), body...)
			logs, releaseLogs := captureStructuredLog(t)
			defer releaseLogs()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						rejection,
					)),
				},
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_raw_fallback"}},
					Body: io.NopCloser(strings.NewReader(
						`{"id":"chatcmpl_fallback","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
					)),
				},
			}}
			svc := &OpenAIGatewayService{
				cfg:          structuredInputRetryTestConfig(),
				httpUpstream: upstream,
			}
			account := structuredInputRetryTestAccount()

			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
			require.True(t, gjson.GetBytes(upstream.bodies[0], "input").IsArray())
			require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
			require.JSONEq(t, string(body), string(upstream.bodies[1]))
			require.Equal(t, originalBody, body)
			require.Equal(t, "/v1/chat/completions", result.UpstreamEndpoint)
			require.Equal(t, "/v1/chat/completions", GetActualOpenAIUpstreamEndpoint(c))
			require.Equal(t, "ok", gjson.Get(rec.Body.String(), "choices.0.message.content").String())
			require.True(t, logs.ContainsMessage("structured Responses input rejected"))
			logs.mu.Lock()
			defer logs.mu.Unlock()
			for _, event := range logs.events {
				require.NotContains(t, event.Fields, "upstream_message")
				require.NotContains(t, event.Fields, "upstream_body")
				require.NotContains(t, event.Message, rejection)
			}
		})
	}
}

func TestConvertedResponsesInputStringRejectionIsNarrow(t *testing.T) {
	matching := []byte(`{"error":{"code":"invalid_type","param":"input","message":"Expected a string for input, but got an object instead."}}`)
	require.True(t, isConvertedResponsesInputStringRejection(http.StatusBadRequest, matching))
	require.True(t, isConvertedResponsesInputStringRejection(http.StatusBadRequest, []byte(`{"error":{"type":"invalid_type","param":"input","message":"Invalid type for 'input': expected a string, but got an array instead."}}`)))

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "nested content validation", status: http.StatusBadRequest, body: `{"error":{"message":"'loc':('body','input',0,'content','str'), 'msg':'Input should be a valid string'"}}`},
		{name: "other field validation", status: http.StatusBadRequest, body: `{"error":{"message":"'loc':('body','model','str'), 'msg':'Input should be a valid string'"}}`},
		{name: "generic string validation", status: http.StatusBadRequest, body: `{"error":{"message":"Input should be a valid string"}}`},
		{name: "wrong status", status: http.StatusUnprocessableEntity, body: string(matching)},
		{name: "authentication", status: http.StatusUnauthorized, body: string(matching)},
		{name: "authorization", status: http.StatusForbidden, body: string(matching)},
		{name: "rate limit", status: http.StatusTooManyRequests, body: string(matching)},
		{name: "server error", status: http.StatusInternalServerError, body: string(matching)},
		{name: "wrong parameter", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_type","param":"tools","message":"Expected a string, but got an array instead."}}`},
		{name: "wrong code", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_request_error","param":"input","message":"Expected a string, but got an array instead."}}`},
		{name: "unrelated input error", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_type","param":"input","message":"Input is too long."}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, isConvertedResponsesInputStringRejection(tc.status, []byte(tc.body)))
		})
	}
}

func TestConvertedResponsesInputStringRejectionValidationBoundaries(t *testing.T) {
	validation := `{'type': 'string_type', 'loc': ('body', 'input', 'str'), 'msg': 'Input should be a valid string', 'input': [{'role': 'user', 'content': 'hello'}]}`
	for _, test := range []struct {
		name    string
		message string
		want    bool
	}{
		{name: "complete Pydantic error", message: "1 validation error: [" + validation + "]", want: true},
		{name: "multiple Pydantic errors", message: "2 validation errors for Request: [" + strings.Replace(validation, "'input', 'str'", "'model', 'str'", 1) + ", " + validation + "]", want: true},
		{name: "double quoted keys", message: "1 validation error: [" + strings.ReplaceAll(validation, "'", `"`) + "]", want: true},
		{name: "surrounding whitespace", message: " \n1 validation error: [" + validation + "]\n ", want: true},
		{name: "escaped quote in history", message: `1 validation error: [{'type':'string_type','loc':('body','input','str'),'msg':'Input should be a valid string','input':[{'content':'can\'t [close] this }'}]}]`, want: true},
		{name: "bare loc and msg", message: `'loc':('body','input','str'), 'msg':'Input should be a valid string'`},
		{name: "echoed dictionary in unrelated error", message: "Invalid history: " + validation},
		{name: "echoed fake loc and msg in input", message: `1 validation error: [{'type':'string_type','loc':('body','model','str'),'msg':'Input should be a valid string','input':{'loc':('body','input','str'),'msg':'Input should be a valid string'}}]`},
		{name: "echoed fake loc and msg in quoted history", message: `1 validation error: [{'type':'string_type','loc':('body','input',0,'content','str'),'msg':'Input should be a valid string','input':"'loc':('body','input','str'), 'msg':'Input should be a valid string'"}]`},
		{name: "echoed complete error in quoted history", message: `1 validation error: [{'type':'string_type','loc':('body','model','str'),'msg':'Input should be a valid string','input':"1 validation error: [{'type':'string_type','loc':('body','input','str'),'msg':'Input should be a valid string'}]"}]`},
		{name: "truncated error list", message: "1 validation error: [" + validation},
		{name: "truncated quoted input", message: `1 validation error: [{'type':'string_type','loc':('body','input','str'),'msg':'Input should be a valid string','input':'history}]`},
		{name: "mismatched closing bracket", message: "1 validation error: [" + strings.TrimSuffix(validation, "}") + ")]"},
		{name: "elided validation list", message: "240 validation errors: ... " + validation},
		{name: "incorrect error count", message: "2 validation errors: [" + validation + "]"},
		{name: "overflowed error count", message: "999999999999999999999999 validation errors: [" + validation + "]"},
		{name: "duplicate location", message: "1 validation error: [" + strings.Replace(validation, "'input':", "'loc': ('body', 'model', 'str'), 'input':", 1) + "]"},
		{name: "wrong validation type", message: "1 validation error: [" + strings.Replace(validation, "string_type", "value_error", 1) + "]"},
		{name: "missing validation type", message: "1 validation error: [" + strings.Replace(validation, "'type': 'string_type', ", "", 1) + "]"},
		{name: "location and message in different errors", message: "2 validation errors: [" + strings.Replace(validation, "Input should be a valid string", "Invalid history", 1) + ", " + strings.Replace(validation, "'input', 'str'", "'model', 'str'", 1) + "]"},
		{name: "trailing echoed text", message: "1 validation error: [" + validation + "] echoed history"},
		{name: "structured string array", message: "Expected a string, but got an array instead.", want: true},
		{name: "structured object", message: "Invalid type for 'input': expected a string, but got an object instead.", want: true},
		{name: "structured echoed history", message: "Invalid history containing 'expected a string' and 'got an array'"},
		{name: "structured truncated message", message: "Expected a string, but got an array"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"error": map[string]string{"message": test.message, "code": "invalid_type", "param": "input"}})
			require.NoError(t, err)
			require.Equal(t, test.want, isConvertedResponsesInputStringRejection(http.StatusBadRequest, body))
			if test.want {
				require.False(t, isConvertedResponsesInputStringRejection(http.StatusBadRequest, body[:len(body)-1]))
				require.False(t, isConvertedResponsesInputStringRejection(http.StatusBadRequest, append(body, []byte(" trailing")...)))
			}
		})
	}
}

func TestForwardAsChatCompletions_StructuredInputRetryRequiresCompleteErrorBody(t *testing.T) {
	rejection := `{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`
	limit := int(openAIUpstreamErrorBodyReadLimitForConfig(structuredInputRetryTestConfig()))
	for _, test := range []struct {
		name    string
		payload string
		readErr error
		unknown bool
		want    bool
	}{
		{name: "complete JSON followed by read failure", payload: rejection, readErr: io.ErrUnexpectedEOF},
		{name: "truncated JSON", payload: rejection[:len(rejection)-1]},
		{name: "exact read limit", payload: rejection + strings.Repeat(" ", limit-len(rejection)), want: true},
		{name: "one byte beyond read limit", payload: rejection + strings.Repeat(" ", limit-len(rejection)+1)},
		{name: "valid JSON prefix before discarded suffix", payload: rejection + strings.Repeat(" ", limit-len(rejection)) + "trailing"},
		{name: "unknown capability still retries after read failure", payload: rejection, readErr: io.ErrUnexpectedEOF, unknown: true, want: true},
		{name: "unknown capability still retries beyond read limit", payload: rejection + strings.Repeat(" ", limit-len(rejection)+1), unknown: true, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}]}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			errorBody := io.NopCloser(strings.NewReader(test.payload))
			if test.readErr != nil {
				errorBody = &openAIChatStreamReadErrorCloser{payload: []byte(test.payload), err: test.readErr}
			}
			account := structuredInputRetryTestAccount()
			status := http.StatusBadRequest
			if test.unknown {
				delete(account.Extra, openai_compat.ExtraKeyResponsesSupported)
				status = http.StatusNotFound
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: errorBody},
				{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"chatcmpl_retry","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))},
			}}
			svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			if test.want {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, upstream.requests, 2)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
				require.Len(t, upstream.requests, 1)
			}
		})
	}
}

type openAIChatFallbackDelayReader struct {
	io.Reader
	elapsed time.Duration
}

func (reader *openAIChatFallbackDelayReader) Read(buffer []byte) (int, error) {
	if reader.elapsed == 0 {
		started := time.Now()
		time.Sleep(30 * time.Millisecond)
		reader.elapsed = time.Since(started)
	}
	return reader.Reader.Read(buffer)
}

func TestForwardAsChatCompletions_RawFallbackPreservesResponsesTiming(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("unknown=%t/stream=%t", unknown, stream), func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream))
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
				account := structuredInputRetryTestAccount()
				status := http.StatusBadRequest
				if unknown {
					delete(account.Extra, openai_compat.ExtraKeyResponsesSupported)
					status = http.StatusNotFound
				}
				initialBody := &openAIChatFallbackDelayReader{Reader: strings.NewReader(`{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`)}
				responseBody := `{"id":"chatcmpl_retry","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
				contentType := "application/json"
				if stream {
					responseBody = "data: {\"id\":\"chatcmpl_retry\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n"
					contentType = "text/event-stream"
				}
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(initialBody)},
					{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(responseBody))},
				}}
				svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
				result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, upstream.requests, 2)
				require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
				require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
				require.GreaterOrEqual(t, result.Duration, initialBody.elapsed)
				if stream {
					require.NotNil(t, result.FirstTokenMs)
					require.GreaterOrEqual(t, *result.FirstTokenMs, int(initialBody.elapsed.Milliseconds()))
				}
			})
		}
	}
}

func TestForwardAsChatCompletions_StructuredInputRejectionDoesNotRetryIneligibleRequests(t *testing.T) {
	rejection := `{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`
	for _, test := range []struct {
		name          string
		mode          string
		unknown       bool
		platform      string
		accountType   string
		responsesBody bool
		nativeIngress bool
		status        int
		rejection     string
	}{
		{name: "forced Responses", mode: "force_responses"},
		{name: "forced Responses without probe", mode: "force_responses", unknown: true},
		{name: "unknown support", unknown: true},
		{name: "Responses shaped Chat ingress", responsesBody: true},
		{name: "native Responses ingress", responsesBody: true, nativeIngress: true},
		{name: "native CN Responses protocol", platform: PlatformDeepseek},
		{name: "OAuth", accountType: AccountTypeOAuth},
		{name: "authentication", status: http.StatusUnauthorized},
		{name: "authorization", status: http.StatusForbidden},
		{name: "rate limit", status: http.StatusTooManyRequests},
		{name: "unrelated validation", rejection: `{"error":{"code":"invalid_type","param":"tools","message":"Expected a string, but got an array instead."}}`},
		{name: "nested validation", rejection: `{"error":{"message":"'loc':('body','input',0,'content','str'), 'msg':'Input should be a valid string'"}}`},
		{name: "echoed validation", rejection: `{"error":{"message":"1 validation error: [{'type':'string_type','loc':('body','model','str'),'msg':'Input should be a valid string','input':{'loc':('body','input','str'),'msg':'Input should be a valid string'}}]"}}`},
		{name: "echoed structured error", rejection: `{"error":{"code":"invalid_type","param":"input","message":"Invalid history containing expected a string but got an array"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
			if test.responsesBody {
				body = []byte(`{"model":"gpt-5.4","input":[{"role":"user","content":"hello"}],"stream":false}`)
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			account := structuredInputRetryTestAccount()
			account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: true, openai_compat.ExtraKeyResponsesMode: test.mode}
			if test.unknown {
				delete(account.Extra, openai_compat.ExtraKeyResponsesSupported)
			}
			if test.platform != "" {
				account.Platform = test.platform
				account.Credentials["api_protocol"] = APIProtocolResponses
			}
			if test.accountType != "" {
				account.Type = test.accountType
				account.Credentials["access_token"] = "test-token"
			}
			status := test.status
			if status == 0 {
				status = http.StatusBadRequest
			}
			errorBody := rejection
			if test.rejection != "" {
				errorBody = test.rejection
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(errorBody)),
			}}
			svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
			var result *OpenAIForwardResult
			var err error
			if test.nativeIngress {
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				result, err = svc.Forward(context.Background(), c, account, body)
			} else {
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			}
			require.Error(t, err)
			require.Nil(t, result)
			require.Len(t, upstream.requests, 1)
			require.True(t, strings.HasSuffix(upstream.requests[0].URL.Path, "/responses"))
			var failover *UpstreamFailoverError
			if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests {
				require.ErrorAs(t, err, &failover)
				require.Equal(t, status, failover.StatusCode)
			} else {
				require.False(t, errors.As(err, &failover))
				require.True(t, c.Writer.Written())
			}
		})
	}
}

func TestForwardAsChatCompletions_StructuredInputRetryPreservesStreamingAndMapping(t *testing.T) {
	for _, useAccountMapping := range []bool{false, true} {
		t.Run(fmt.Sprintf("account_mapping=%t", useAccountMapping), func(t *testing.T) {
			body := []byte(`{"model":"client-model","messages":[{"role":"user","content":"hello"}],"stream":true,"stream_options":{"include_usage":false},"vendor_option":"preserve"}`)
			originalBody := append([]byte(nil), body...)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			account := structuredInputRetryTestAccount()
			account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: true, openai_compat.ExtraKeyResponsesMode: "auto"}
			mappedModel := "gpt-5.4"
			if useAccountMapping {
				account.Credentials["model_mapping"] = map[string]any{"client-model": mappedModel}
			}
			stream := "data: {\"id\":\"chatcmpl_retry\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-5.4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":4,\"total_tokens\":13}}\n\ndata: [DONE]\n\n"
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`))},
				{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))},
			}}
			svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
			defaultMappedModel := mappedModel
			if useAccountMapping {
				defaultMappedModel = ""
			}
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", defaultMappedModel)
			require.NoError(t, err)
			require.Len(t, upstream.requests, 2)
			for _, outbound := range upstream.bodies {
				require.Equal(t, mappedModel, gjson.GetBytes(outbound, "model").String())
				require.True(t, gjson.GetBytes(outbound, "stream").Bool())
			}
			require.JSONEq(t, gjson.GetBytes(body, "messages").Raw, gjson.GetBytes(upstream.bodies[1], "messages").Raw)
			require.Equal(t, "preserve", gjson.GetBytes(upstream.bodies[1], "vendor_option").String())
			require.True(t, gjson.GetBytes(upstream.bodies[1], "stream_options.include_usage").Bool())
			require.Equal(t, originalBody, body)
			require.True(t, result.Stream)
			require.Equal(t, mappedModel, result.BillingModel)
			require.Equal(t, mappedModel, result.UpstreamModel)
			require.Equal(t, 9, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			require.Equal(t, "/v1/chat/completions", result.UpstreamEndpoint)
			require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
			require.Contains(t, rec.Body.String(), "data: [DONE]")
		})
	}
}

func TestForwardAsChatCompletions_StructuredInputRetryIsBoundedAndPreservesRawErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests} {
		t.Run(fmt.Sprintf("status=%d", status), func(t *testing.T) {
			rejection := `{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`
			response := func(status int) *http.Response {
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(rejection))}
			}
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
			account := structuredInputRetryTestAccount()
			account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: true}
			forward := func(retry bool) (*httptest.ResponseRecorder, error) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
				upstream := &httpUpstreamRecorder{responses: []*http.Response{response(status)}}
				if retry {
					upstream.responses = append([]*http.Response{response(http.StatusBadRequest)}, upstream.responses...)
				}
				svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
				var result *OpenAIForwardResult
				var err error
				if retry {
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					require.Len(t, upstream.requests, 2)
					require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
				} else {
					result, err = svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
					require.Len(t, upstream.requests, 1)
				}
				require.Nil(t, result)
				require.Error(t, err)
				return rec, err
			}
			baseline, baselineErr := forward(false)
			retried, retryErr := forward(true)
			require.Equal(t, baseline.Code, retried.Code)
			require.Equal(t, baseline.Body.String(), retried.Body.String())
			require.Equal(t, baselineErr.Error(), retryErr.Error())
			require.IsType(t, baselineErr, retryErr)
		})
	}
}

func structuredInputRetryTestConfig() *config.Config {
	return &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled:           false,
		AllowInsecureHTTP: true,
	}}}
}

func structuredInputRetryTestAccount() *Account {
	return &Account{
		ID:          101,
		Name:        "raw-openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
		Extra:       map[string]any{openai_compat.ExtraKeyResponsesSupported: true},
	}
}

type openAIChatStreamReadErrorCloser struct {
	payload []byte
	err     error
	sent    bool
}

func (r *openAIChatStreamReadErrorCloser) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.payload), nil
	}
	return 0, r.err
}

func (r *openAIChatStreamReadErrorCloser) Close() error { return nil }

func TestHandleChatStreamingResponse_ClassifiesHTTP2ReadError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"x-request-id": []string{"upstream-rid"},
		},
		Body: &openAIChatStreamReadErrorCloser{
			payload: []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"),
			err:     errors.New("stream error: stream ID 5; INTERNAL_ERROR; received from peer"),
		},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	result, err := svc.handleChatStreamingResponse(
		resp,
		c,
		&Account{ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI},
		"gpt-5.6-sol",
		"gpt-5.6-sol",
		"gpt-5.6-sol",
		time.Now(),
		0,
	)

	require.Error(t, err)
	require.NotNil(t, result)
	require.True(t, c.Writer.Written(), "partial output must make replay unsafe")
	code, message, ok := OpenAIUpstreamStreamReadErrorDetails(err)
	require.True(t, ok)
	require.Equal(t, OpenAIUpstreamHTTP2StreamErrorCode, code)
	require.Equal(t, "Upstream HTTP/2 stream failed", message)
	require.NotContains(t, message, "stream ID")
	require.NotContains(t, message, "INTERNAL_ERROR")
}

func TestNormalizeResponsesRequestServiceTier(t *testing.T) {
	t.Parallel()

	req := &apicompat.ResponsesRequest{ServiceTier: " fast "}
	normalizeResponsesRequestServiceTier(req)
	require.Equal(t, "priority", req.ServiceTier)

	req.ServiceTier = "flex"
	normalizeResponsesRequestServiceTier(req)
	require.Equal(t, "flex", req.ServiceTier)

	// OpenAI 官方合法 tier 应被透传保留。
	req.ServiceTier = "auto"
	normalizeResponsesRequestServiceTier(req)
	require.Equal(t, "auto", req.ServiceTier)

	req.ServiceTier = "default"
	normalizeResponsesRequestServiceTier(req)
	require.Equal(t, "default", req.ServiceTier)

	req.ServiceTier = "scale"
	normalizeResponsesRequestServiceTier(req)
	require.Equal(t, "scale", req.ServiceTier)

	// 真未知值仍被剥离。
	req.ServiceTier = "turbo"
	normalizeResponsesRequestServiceTier(req)
	require.Empty(t, req.ServiceTier)
}

func TestNormalizeResponsesBodyServiceTier(t *testing.T) {
	t.Parallel()

	body, tier, err := normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.1","service_tier":"fast"}`))
	require.NoError(t, err)
	require.Equal(t, "priority", tier)
	require.Equal(t, "priority", gjson.GetBytes(body, "service_tier").String())

	body, tier, err = normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.1","service_tier":"flex"}`))
	require.NoError(t, err)
	require.Equal(t, "flex", tier)
	require.Equal(t, "flex", gjson.GetBytes(body, "service_tier").String())

	// OpenAI 官方 tier 直接保留在 body 中（透传上游）。
	body, tier, err = normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.1","service_tier":"auto"}`))
	require.NoError(t, err)
	require.Equal(t, "auto", tier)
	require.Equal(t, "auto", gjson.GetBytes(body, "service_tier").String())

	body, tier, err = normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.1","service_tier":"default"}`))
	require.NoError(t, err)
	require.Equal(t, "default", tier)
	require.Equal(t, "default", gjson.GetBytes(body, "service_tier").String())

	body, tier, err = normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.1","service_tier":"scale"}`))
	require.NoError(t, err)
	require.Equal(t, "scale", tier)
	require.Equal(t, "scale", gjson.GetBytes(body, "service_tier").String())

	body, tier, err = normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.6-sol","service_tier":"ultrafast"}`))
	require.NoError(t, err)
	require.Equal(t, "ultrafast", tier)
	require.Equal(t, "ultrafast", gjson.GetBytes(body, "service_tier").String())

	// 真未知值才会被删除。
	body, tier, err = normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.1","service_tier":"turbo"}`))
	require.NoError(t, err)
	require.Empty(t, tier)
	require.False(t, gjson.GetBytes(body, "service_tier").Exists())
}

func TestForwardAsChatCompletions_UnknownModelWithoutMessagesDispatchKeepsRequestedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt6","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_chat_unknown_model"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"model not found"}}`)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, "gpt6", gjson.GetBytes(upstream.lastBody, "model").String())
	require.NotEqual(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestForwardAsChatCompletions_APIKeyPropagatesPromptCacheKeyInResponsesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: 99})

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_chat_prompt_cache"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop before response parsing"}}`)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          2,
		Name:        "openai-compatible",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-compatible",
		},
		Extra: map[string]any{
			"openai_responses_supported": true,
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "cache-key-123", "gpt-5.4")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, "cache-key-123", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "https://api.openai.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-compatible", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, generateSessionUUID(isolateOpenAISessionID(99, "cache-key-123")), upstream.lastReq.Header.Get("session_id"))
}

func TestForwardAsChatCompletions_APIKeyAutoDerivesStableIsolatedPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	response := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop before response parsing"}}`)),
		}
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{response(), response(), response()}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 2, Name: "openai-compatible", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-compatible"},
		Extra:       map[string]any{"openai_responses_supported": true},
	}
	firstBody := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"be concise"},{"role":"user","content":"hello"}],"stream":false}`)
	appendedBody := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"be concise"},{"role":"user","content":"hello"},{"role":"assistant","content":"hi"},{"role":"user","content":"continue"}],"stream":false}`)

	forward := func(apiKeyID int64, body []byte) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("api_key", &APIKey{ID: apiKeyID})
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.4")
		require.Error(t, err)
		require.Nil(t, result)
	}

	forward(99, firstBody)
	forward(99, appendedBody)
	forward(100, appendedBody)

	firstKey := gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String()
	appendedKey := gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").String()
	otherTenantKey := gjson.GetBytes(upstream.bodies[2], "prompt_cache_key").String()
	require.NotEmpty(t, firstKey)
	require.Equal(t, firstKey, appendedKey)
	require.NotEqual(t, firstKey, otherTenantKey)
	require.Equal(t, generateSessionUUID(firstKey), upstream.requests[0].Header.Get("session_id"))
	require.Equal(t, upstream.requests[0].Header.Get("session_id"), upstream.requests[1].Header.Get("session_id"))
	require.Equal(t, generateSessionUUID(otherTenantKey), upstream.requests[2].Header.Get("session_id"))
	require.NotEqual(t, upstream.requests[1].Header.Get("session_id"), upstream.requests[2].Header.Get("session_id"))
}

func TestForwardAsChatCompletions_ResponsesShapeDoesNotAutoDerivePromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	response := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop before response parsing"}}`)),
		}
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{response(), response(), response()}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 2, Name: "openai-compatible", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-compatible"},
		Extra:       map[string]any{"openai_responses_supported": true},
	}
	firstBody := []byte(`{"model":"gpt-5.4","input":[{"role":"user","content":[{"type":"input_text","text":"first unrelated input"}]}],"stream":false}`)
	secondBody := []byte(`{"model":"gpt-5.4","input":[{"role":"user","content":[{"type":"input_text","text":"second unrelated input"}]}],"stream":false}`)

	forward := func(body []byte, promptCacheKey string) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		c.Set("api_key", &APIKey{ID: 99})
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, promptCacheKey, "gpt-5.4")
		require.Error(t, err)
		require.Nil(t, result)
	}

	forward(firstBody, "")
	forward(secondBody, "")
	forward(secondBody, "explicit-responses-key")

	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").Exists())
	require.Empty(t, upstream.requests[0].Header.Get("session_id"))
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.Empty(t, upstream.requests[1].Header.Get("session_id"))
	require.Equal(t, "explicit-responses-key", gjson.GetBytes(upstream.bodies[2], "prompt_cache_key").String())
	require.Equal(t, generateSessionUUID(isolateOpenAISessionID(99, "explicit-responses-key")), upstream.requests[2].Header.Get("session_id"))
}

func TestForwardAsChatCompletions_OAuthDoesNotInjectDefaultInstructions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_chat_no_default_instructions"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop before response parsing"}}`)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          3,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.4")
	require.Error(t, err)
	require.Nil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
	require.Equal(t, "", gjson.GetBytes(upstream.lastBody, "instructions").String())
	require.NotContains(t, string(upstream.lastBody), "Communicate with the user by streaming thinking")
}

func forwardOAuthChatCompletionsForUpstreamBody(t *testing.T, body []byte) []byte {
	t.Helper()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_chat_system_promotion"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop before response parsing"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          4,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.4")
	require.Error(t, err)
	require.Nil(t, result)
	require.NotEmpty(t, upstream.lastBody)
	return upstream.lastBody
}

func TestForwardAsChatCompletions_OAuthPromotesSystemMessageWithoutDuplication(t *testing.T) {
	const systemPrompt = "Unique system prefix for token accounting."
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"` + systemPrompt + `"},{"role":"user","content":"hello"}],"stream":false}`)

	upstreamBody := forwardOAuthChatCompletionsForUpstreamBody(t, body)

	require.Equal(t, systemPrompt, gjson.GetBytes(upstreamBody, "instructions").String())
	require.Equal(t, int64(1), gjson.GetBytes(upstreamBody, "input.#").Int())
	require.Equal(t, "user", gjson.GetBytes(upstreamBody, "input.0.role").String())
	require.Equal(t, 1, strings.Count(string(upstreamBody), systemPrompt))
}

func TestForwardAsChatCompletions_OAuthJsonObjectKeepsSystemMessageInInput(t *testing.T) {
	const systemPrompt = "Return JSON only."
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"` + systemPrompt + `"},{"role":"user","content":"symbol data"}],"response_format":{"type":" JSON_OBJECT "},"stream":false}`)

	upstreamBody := forwardOAuthChatCompletionsForUpstreamBody(t, body)

	require.Equal(t, systemPrompt, gjson.GetBytes(upstreamBody, "instructions").String())
	require.Equal(t, int64(2), gjson.GetBytes(upstreamBody, "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstreamBody, "input.0.role").String())
	require.Equal(t, systemPrompt, gjson.GetBytes(upstreamBody, "input.0.content").String())
	require.Equal(t, 2, strings.Count(string(upstreamBody), systemPrompt))
}

func TestForwardAsChatCompletions_OAuthKeepsMixedSystemContentInInput(t *testing.T) {
	const systemPrompt = "Inspect this reference image."
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":[{"type":"text","text":"` + systemPrompt + `"},{"type":"image_url","image_url":{"url":"https://example.com/reference.png"}}]},{"role":"user","content":"hello"}],"stream":false}`)

	upstreamBody := forwardOAuthChatCompletionsForUpstreamBody(t, body)

	require.Equal(t, systemPrompt, gjson.GetBytes(upstreamBody, "instructions").String())
	require.Equal(t, int64(2), gjson.GetBytes(upstreamBody, "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstreamBody, "input.0.role").String())
	require.Equal(t, int64(2), gjson.GetBytes(upstreamBody, "input.0.content.#").Int())
	require.Equal(t, "input_image", gjson.GetBytes(upstreamBody, "input.0.content.1.type").String())
	require.Equal(t, "https://example.com/reference.png", gjson.GetBytes(upstreamBody, "input.0.content.1.image_url").String())
}

func TestForwardAsChatCompletions_ClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAIChatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":11,"output_tokens":5,"total_tokens":16,"input_tokens_details":{"cached_tokens":4}}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_disconnect"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Equal(t, 4, result.Usage.CacheReadInputTokens)
}

func TestForwardAsChatCompletions_BufferedContextWindowResponseFailedReturnsErrorWithoutFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"large prompt"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`event: response.failed`,
		`data: {"type":"response.failed","response":{"id":"resp_failed","object":"response","model":"gpt-5.5","status":"failed","output":[],"error":{"code":"upstream_error","message":"input exceeds the context window"}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_failed_buffered"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")
	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.True(t, c.Writer.Written())
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "input exceeds the context window")
}

func TestForwardAsChatCompletions_StreamContextWindowResponseFailedReturnsErrorWithoutFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"` + strings.Repeat("large prompt ", 6000) + `"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_failed","model":"gpt-5.5","status":"in_progress","output":[]}}`,
		"",
		`event: response.failed`,
		`data: {"type":"response.failed","response":{"id":"resp_failed","object":"response","model":"gpt-5.5","status":"failed","output":[],"error":{"code":"upstream_error","message":"input exceeds the context window"}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_failed_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")
	require.Error(t, err)
	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.True(t, c.Writer.Written())
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	require.Contains(t, rec.Body.String(), "input exceeds the context window")
	require.NotContains(t, rec.Body.String(), "[DONE]")
}

func TestForwardAsChatCompletions_StreamBareErrorAfterOutputDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"partial"}`,
		"",
		`event: error`,
		`data: {"type":"error","error":{"type":"server_error","code":"server_error","message":"temporary upstream failure"}}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")

	require.Error(t, err)
	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Contains(t, rec.Body.String(), "partial")
	require.Contains(t, rec.Body.String(), "temporary upstream failure")
	require.NotContains(t, rec.Body.String(), "[DONE]")
}

func TestForwardAsChatCompletions_StreamCyberPolicyNoFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"` + strings.Repeat("large prompt ", 6000) + `"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_cyber","model":"gpt-5.5","status":"in_progress","output":[]}}`,
		"",
		`event: response.failed`,
		`data: {"type":"response.failed","response":{"id":"resp_cyber","object":"response","model":"gpt-5.5","status":"failed","output":[],"error":{"code":"cyber_policy","message":"flagged for cyber policy"}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_cyber"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "cyber must NOT trigger failover")
	require.NotNil(t, GetOpsCyberPolicy(c), "cyber mark must be set")
	respBody := rec.Body.String()
	require.Contains(t, respBody, `"error"`)
	require.Contains(t, respBody, `"cyber_policy"`)
	require.Contains(t, respBody, "data: [DONE]")
}

func TestForwardAsChatCompletions_StreamsUsageWithoutClientStreamOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":13,"output_tokens":7,"total_tokens":20,"input_tokens_details":{"cached_tokens":5}}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_usage_no_stream_options"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 13, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 5, result.Usage.CacheReadInputTokens)

	responseBody := rec.Body.String()
	require.Contains(t, responseBody, `"usage"`)
	require.Contains(t, responseBody, `"prompt_tokens":13`)
	require.Contains(t, responseBody, `"completion_tokens":7`)
	require.Contains(t, responseBody, `"cached_tokens":5`)
}

func TestForwardAsChatCompletions_StreamsTopLevelTerminalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_top","model":"gpt-5.4","status":"in_progress","output":[]}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_top","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}]},"usage":{"input_tokens":21,"output_tokens":9,"total_tokens":30,"input_tokens_details":{"cached_tokens":4}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_top_level_usage"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 21, result.Usage.InputTokens)
	require.Equal(t, 9, result.Usage.OutputTokens)
	require.Equal(t, 4, result.Usage.CacheReadInputTokens)

	responseBody := rec.Body.String()
	require.Contains(t, responseBody, `"usage"`)
	require.Contains(t, responseBody, `"prompt_tokens":21`)
	require.Contains(t, responseBody, `"completion_tokens":9`)
	require.Contains(t, responseBody, `"cached_tokens":4`)
}

func TestForwardAsChatCompletions_BufferedTopLevelTerminalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_top_buffered","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}]},"usage":{"input_tokens":18,"output_tokens":6,"total_tokens":24,"input_tokens_details":{"cached_tokens":3}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_buffered_top_level_usage"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 18, result.Usage.InputTokens)
	require.Equal(t, 6, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)

	responseBody := rec.Body.String()
	require.Contains(t, responseBody, `"usage"`)
	require.Contains(t, responseBody, `"prompt_tokens":18`)
	require.Contains(t, responseBody, `"completion_tokens":6`)
	require.Contains(t, responseBody, `"cached_tokens":3`)
}

func TestForwardAsChatCompletions_TerminalUsageWithoutUpstreamCloseReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAIChatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":17,"output_tokens":8,"total_tokens":25,"input_tokens_details":{"cached_tokens":6}}}}` + "\n\n")
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
	}()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_terminal_no_close"}},
		Body:       upstreamStream,
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
	}
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: err}
	}()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 17, got.result.Usage.InputTokens)
		require.Equal(t, 8, got.result.Usage.OutputTokens)
		require.Equal(t, 6, got.result.Usage.CacheReadInputTokens)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsChatCompletions should return after terminal usage event even if upstream keeps the connection open")
	}
}

func TestForwardAsChatCompletions_EventNamedTerminalWithoutUpstreamCloseReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(strings.Join([]string{
		`event: response.created`,
		`data: {"response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]}}`,
		``,
		`event: response.output_text.delta`,
		`data: {"delta":"ok"}`,
		``,
		`event: response.completed`,
		`data: {"response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":17,"output_tokens":8,"total_tokens":25,"input_tokens_details":{"cached_tokens":6}}}}`,
		``,
		``,
	}, "\n"))
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
	}()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_event_named_terminal"}},
		Body:       upstreamStream,
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
	}
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: err}
	}()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 17, got.result.Usage.InputTokens)
		require.Equal(t, 8, got.result.Usage.OutputTokens)
		require.Equal(t, 6, got.result.Usage.CacheReadInputTokens)
		require.Contains(t, rec.Body.String(), `"content":"ok"`)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsChatCompletions should use SSE event names when data payloads omit type")
	}
}

func TestForwardAsChatCompletions_EventTypeDoesNotLeakAcrossFrames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`event: response.created`,
		`data: {"response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]}}`,
		``,
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		``,
		`event: response.completed`,
		`data: {"response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":17,"output_tokens":8,"total_tokens":25,"input_tokens_details":{"cached_tokens":6}}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_event_boundary"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), `"content":"ok"`)
	require.Contains(t, rec.Body.String(), `data: [DONE]`)
}

func TestForwardAsChatCompletions_BufferedTerminalWithoutUpstreamCloseReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":17,"output_tokens":8,"total_tokens":25,"input_tokens_details":{"cached_tokens":6}}}}` + "\n\n")
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
	}()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_buffered_terminal_no_close"}},
		Body:       upstreamStream,
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
	}
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: err}
	}()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 17, got.result.Usage.InputTokens)
		require.Equal(t, 8, got.result.Usage.OutputTokens)
		require.Equal(t, 6, got.result.Usage.CacheReadInputTokens)
		require.Contains(t, rec.Body.String(), `"finish_reason":"stop"`)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsChatCompletions buffered response should return after terminal usage event even if upstream keeps the connection open")
	}
}

func TestForwardAsChatCompletions_DoneSentinelWithoutTerminalReturnsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := "data: [DONE]\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_missing_terminal"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing terminal event")
	require.NotNil(t, result)
	require.Zero(t, result.Usage.InputTokens)
	require.Zero(t, result.Usage.OutputTokens)
}

func TestForwardAsChatCompletions_UpstreamRequestIgnoresClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	reqCtx, cancel := context.WithCancel(context.Background())
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(reqCtx)
	c.Request.Header.Set("Content-Type", "application/json")
	cancel()

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_chat_ctx"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsChatCompletions(reqCtx, c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.NoError(t, upstream.lastReq.Context().Err())
}

// TestBuildChatStreamErrorSSE verifies F4: the error chunk payload follows the
// OpenAI chat streaming error convention so third-party clients stop retrying.
func TestBuildChatStreamErrorSSE(t *testing.T) {
	got := buildChatStreamErrorSSE("cyber_policy", "blocked by policy")
	require.True(t, strings.HasPrefix(got, "data: "), "must be an SSE data frame")
	payload := strings.TrimSuffix(strings.TrimPrefix(got, "data: "), "\n\n")
	require.Equal(t, "invalid_request_error", gjson.Get(payload, "error.type").String())
	require.Equal(t, "cyber_policy", gjson.Get(payload, "error.code").String())
	require.Equal(t, "blocked by policy", gjson.Get(payload, "error.message").String())
}

func TestGPT6RawChatRejectsReasoningToolCalls(t *testing.T) {
	for _, model := range []string{"gpt-6-sol", "gpt-6-luna"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"public": model}}}
		svc := &OpenAIGatewayService{}
		_, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, []byte(`{"model":"public","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}`), "")
		require.ErrorContains(t, err, "requires Responses")
		require.Equal(t, 400, rec.Code)
	}
}

func TestGPT6ReasoningModeAndSamplingCompatibility(t *testing.T) {
	for _, model := range []string{"gpt-6-sol", "gpt-6-luna"} {
		body := []byte(`{"model":"` + model + `","reasoning":{"mode":"pro","effort":"max"},"temperature":0.7,"top_p":0.9,"top_logprobs":2,"include":["reasoning.encrypted_content","message.output_text.logprobs"],"prompt_cache_options":{"ttl":"30m"}}`)
		out, changed, err := normalizeOpenAIResponsesReasoningMode(body, "")
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, "pro", gjson.GetBytes(out, "reasoning.mode").String())
		require.Equal(t, "max", gjson.GetBytes(out, "reasoning.effort").String())
		require.False(t, gjson.GetBytes(out, "temperature").Exists())
		require.False(t, gjson.GetBytes(out, "top_p").Exists())
		require.False(t, gjson.GetBytes(out, "top_logprobs").Exists())
		require.Equal(t, "reasoning.encrypted_content", gjson.GetBytes(out, "include.0").String())
		require.Equal(t, "30m", gjson.GetBytes(out, "prompt_cache_options.ttl").String())
		require.Equal(t, "max", normalizeOpenAIReasoningEffortForModel("max", model))
		d := newConfiguredCodexModelDescriptor(model)
		require.NotNil(t, d.DefaultReasoningLevel)
		require.Equal(t, "medium", *d.DefaultReasoningLevel)
		require.EqualValues(t, 872000, d.MaxContextWindow)
		require.Len(t, d.SupportedReasoningLevels, 5)
		require.Len(t, d.ServiceTiers, 1)
	}
}

func TestGPT6RawChatNoneToolsAreForwarded(t *testing.T) {
	for _, model := range []string{"gpt-6-sol", "gpt-6-luna"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body := []byte(`{"model":"` + model + `","reasoning_effort":"none","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"chat_1","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":10,"prompt_tokens_details":{"cached_tokens":200,"cache_write_tokens":300}}}`))}}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://api.openai.com"}}
		result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "none", gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
		require.Len(t, gjson.GetBytes(upstream.lastBody, "tools").Array(), 1)
		require.Equal(t, 1000, result.Usage.InputTokens)
		require.Equal(t, 300, result.Usage.CacheCreationInputTokens)
	}
}

func TestGPT6UsageHTTPAndWSHaveSameCacheBreakdown(t *testing.T) {
	response := []byte(`{"model":"gpt-6-sol","usage":{"input_tokens":1000,"output_tokens":10,"input_tokens_details":{"cached_tokens":200,"cache_write_tokens":300}}}`)
	httpUsage, ok := extractOpenAIUsageFromJSONBytes(response)
	require.True(t, ok)
	var wsUsage OpenAIUsage
	parseOpenAIWSResponseUsageFromCompletedEvent(append(append([]byte(`{"type":"response.completed","response":`), response...), '}'), &wsUsage)
	require.Equal(t, httpUsage, wsUsage)
	require.Equal(t, 500, httpUsage.InputTokens-httpUsage.CacheReadInputTokens-httpUsage.CacheCreationInputTokens)
}

func TestGPT6ReasoningModeUsesMappedUpstream(t *testing.T) {
	body := []byte(`{"model":"public-model","reasoning":{"mode":"pro","effort":"max"},"temperature":0.5}`)
	out, _, err := normalizeOpenAIResponsesReasoningMode(body, "gpt-6-sol")
	require.NoError(t, err)
	require.Equal(t, "public-model", gjson.GetBytes(out, "model").String())
	require.Equal(t, "pro", gjson.GetBytes(out, "reasoning.mode").String())
	require.Equal(t, "max", gjson.GetBytes(out, "reasoning.effort").String())
	require.False(t, gjson.GetBytes(out, "temperature").Exists())
}

func TestGPT6MappedCompatibilityBridgesKeepReasoningAndTools(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna"} {
		for _, messages := range []bool{false, true} {
			body := []byte(`{"model":"public","reasoning_effort":"max","temperature":0.7,"top_p":0.9,"prompt_cache_options":{"ttl":"30m"},"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"messages":[{"role":"user","content":"hello"}]}`)
			if messages {
				body = []byte(`{"model":"public","max_tokens":1000,"output_config":{"effort":"max"},"temperature":0.7,"top_p":0.9,"tools":[{"name":"lookup","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hello"}]}`)
			}
			response := `data: {"type":"response.completed","response":{"id":"resp_1","model":"` + model + `","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1000,"output_tokens":10,"input_tokens_details":{"cached_tokens":200,"cache_write_tokens":300}}}}` + "\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(response))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://api.openai.com", "model_mapping": map[string]any{"public": model}}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			var result *OpenAIForwardResult
			var err error
			if messages {
				result, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
			} else {
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
			require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "max", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
			require.Len(t, gjson.GetBytes(upstream.lastBody, "tools").Array(), 1)
			require.False(t, gjson.GetBytes(upstream.lastBody, "temperature").Exists())
			require.False(t, gjson.GetBytes(upstream.lastBody, "top_p").Exists())
			if !messages {
				require.Equal(t, "30m", gjson.GetBytes(upstream.lastBody, "prompt_cache_options.ttl").String())
			}
			require.Equal(t, 300, result.Usage.CacheCreationInputTokens)
		}
	}
}

func TestGPT61SolRejectsDisabledReasoningBeforeForwarding(t *testing.T) {
	for _, body := range []string{
		`{"model":"public","reasoning_effort":"none"}`,
		`{"model":"public","reasoning":{"effort":"minimal"}}`,
		`{"model":"public","output_config":{"effort":"none"}}`,
		`{"model":"public","thinking":{"type":"disabled"}}`,
		`{"model":"gpt-6.1-sol-minimal"}`,
	} {
		require.Error(t, validateGPT61SolCompatRequest([]byte(body), "gpt-6.1-sol"))
		require.NoError(t, validateGPT61SolCompatRequest([]byte(body), "gpt-6-sol"))
	}
	for _, field := range []string{`"reasoning_effort":"none"`, `"reasoning_effort":"minimal"`, `"tools":[{"type":"function","function":{"name":"lookup"}}]`} {
		body := []byte(`{"model":"public",` + field + `,"messages":[{"role":"user","content":"hello"}]}`)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"public": "gpt-6.1-sol"}}}
		svc := &OpenAIGatewayService{cfg: &config.Config{}}
		_, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), "gpt-6.1-sol")
	}
}

func TestGPT61SolMappedReasoningModeAndSampling(t *testing.T) {
	body := []byte(`{"model":"public","reasoning":{"mode":"pro","effort":"max"},"temperature":0.5,"top_p":0.9,"logprobs":true,"top_logprobs":2,"include":["message.output_text.logprobs","reasoning.encrypted_content"]}`)
	out, _, err := normalizeOpenAIResponsesReasoningMode(body, "gpt-6.1-sol")
	require.NoError(t, err)
	require.Equal(t, "pro", gjson.GetBytes(out, "reasoning.mode").String())
	require.Equal(t, "max", gjson.GetBytes(out, "reasoning.effort").String())
	for _, field := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
		require.False(t, gjson.GetBytes(out, field).Exists(), field)
	}
	require.Equal(t, "reasoning.encrypted_content", gjson.GetBytes(out, "include.0").String())
}

func TestGPT61SolMessagesEffortAliasesPreserveIntent(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		req := &apicompat.AnthropicRequest{Model: "openai/gpt-6.1-sol-" + effort}
		applyOpenAICompatModelNormalization(req)
		require.Equal(t, "gpt-6.1-sol", req.Model)
		require.Equal(t, effort, req.OutputConfig.Effort)
		_, err := apicompat.AnthropicToResponses(req)
		if effort == "none" || effort == "minimal" {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestGPT61SolOnlyChatFallbackRejectsToolsAndDisabledReasoning(t *testing.T) {
	for _, responses := range []bool{false, true} {
		bodies := []string{
			`{"model":"public","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`,
			`{"model":"public","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"disabled"}}`,
		}
		if responses {
			bodies = []string{
				`{"model":"public","input":"hi","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`,
				`{"model":"public","input":"hi","reasoning":{"effort":"none"}}`,
			}
		}
		for _, body := range bodies {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"public": "gpt-6.1-sol"}}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			var err error
			if responses {
				_, err = svc.forwardResponsesViaRawChatCompletions(context.Background(), c, account, []byte(body), time.Now())
			} else {
				_, err = svc.forwardAnthropicViaRawChatCompletions(context.Background(), c, account, []byte(body), "")
			}
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), "gpt-6.1-sol")
		}
	}
}
