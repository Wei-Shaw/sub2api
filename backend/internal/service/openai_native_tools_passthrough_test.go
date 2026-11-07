package service

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIExplicitPassthroughPreservesNativeClientTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		name := "non-streaming"
		if stream {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","tools":[{"type":"custom","name":"exec","format":{"type":"text"}},{"type":"tool_search"}],"tool_choice":{"type":"custom","name":"exec"},"input":[{"type":"custom_tool_call","id":"ctc_old","call_id":"call_old","name":"exec","input":"pwd"},{"type":"custom_tool_call_output","call_id":"call_old","output":"ok"}]}`)
			if stream {
				body = append([]byte(`{"stream":true,`), body[1:]...)
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			response := `{"id":"resp_native","status":"completed","output":[{"type":"custom_tool_call","id":"ctc_new","call_id":"call_new","name":"exec","input":"pwd"}],"usage":{"input_tokens":1,"output_tokens":1}}`
			contentType := "application/json"
			if stream {
				contentType = "text/event-stream"
				response = "data: " + `{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"type":"custom_tool_call","id":"ctc_new","call_id":"call_new","name":"exec","input":""}}` + "\n\n" +
					"data: " + `{"type":"response.custom_tool_call_input.delta","sequence_number":1,"output_index":0,"item_id":"ctc_new","delta":"pwd"}` + "\n\n" +
					"data: " + `{"type":"response.completed","sequence_number":2,"response":` + response + "}\n\n"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response))}}
			svc := openAIClientToolsTestService(upstream)
			account := &Account{ID: 8182, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}, Extra: map[string]any{"openai_passthrough": true}}
			_, err := svc.forwardOpenAIPassthrough(context.Background(), c, account, body, body, "gpt-5.4", false, nil, stream, time.Now())
			require.NoError(t, err)
			for _, field := range []string{"tools", "input", "tool_choice"} {
				require.JSONEq(t, gjson.GetBytes(body, field).Raw, gjson.GetBytes(upstream.lastBody, field).Raw, field)
			}
			_, adapted := openAIResponsesClientToolMapping(c)
			require.False(t, adapted)
			require.Contains(t, recorder.Body.String(), `"type":"custom_tool_call"`)
			if stream {
				require.Contains(t, recorder.Body.String(), `"type":"response.custom_tool_call_input.delta"`)
			}
		})
	}
}

func TestResponsesClientToolStreamFlushesInputBeforeUpstreamDone(t *testing.T) {
	source, upstream := io.Pipe()
	body := newGrokResponsesClientToolStreamBody(source, apicompat.ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}}, 0)
	t.Cleanup(func() { _ = upstream.Close(); _ = body.Close() })
	result := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") && gjson.Get(line[6:], "type").String() == "response.custom_tool_call_input.delta" {
				result <- gjson.Get(line[6:], "delta").String()
				return
			}
		}
		result <- "stream ended before input delta"
	}()
	writeDone := make(chan error, 1)
	go func() {
		_, err := io.WriteString(upstream, "data: "+`{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"type":"function_call","id":"fc_a","call_id":"call_a","name":"exec"}}`+"\n\n"+
			"data: "+`{"type":"response.function_call_arguments.delta","sequence_number":1,"item_id":"fc_a","delta":"{\"input\":\"hello"}`+"\n\n")
		writeDone <- err
		// Keep the source open: no arguments.done, completed or EOF is sent.
	}()
	select {
	case delta := <-result:
		require.Equal(t, "hello", delta)
	case <-time.After(3 * time.Second):
		t.Fatal("input delta was buffered while the upstream remained open")
	}
	select {
	case err := <-writeDone:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("upstream write did not finish")
	}
}
