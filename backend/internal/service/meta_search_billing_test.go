//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMetaForwardResponsesCountsWebSearchCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jsonBody := `{"id":"resp_meta","object":"response","model":"muse-spark-1.3","output":[{"type":"web_search_call","id":"ws1","status":"completed"},{"type":"web_search_call","id":"ws2","status":"completed"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":1}}`
	sseBody := strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws1","status":"completed"}}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_meta","object":"response","model":"muse-spark-1.3","output":[{"type":"web_search_call","id":"ws1","status":"completed"}],"usage":{"input_tokens":2,"output_tokens":1}}}`,
		"",
		"",
	}, "\n")
	for _, tc := range []struct {
		name, stream, contentType, body string
		want                            int
	}{
		{"non-stream", "false", "application/json", jsonBody, 2},
		{"stream", "true", "text/event-stream", sseBody, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"muse-spark-1.3","input":"news","tools":[{"type":"web_search"}],"stream":` + tc.stream + `}`)
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{tc.contentType}},
				Body:       io.NopCloser(strings.NewReader(tc.body)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			result, err := svc.Forward(context.Background(), c, metaGatewayTestAccount("muse-spark-1.3"), body)
			require.NoError(t, err)
			require.Equal(t, tc.want, result.SearchCount)
		})
	}
}

func TestMetaCompatPathsCountWebSearchCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sseBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_meta","object":"response","created_at":1794469676.0,"model":"muse-spark-1.3","output":[]}}`,
		"",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_call_1","status":"completed","action":{"type":"search","query":"go release"}}}`,
		"",
		`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"content_index":0,"delta":"ok"}`,
		"",
		`data: {"type":"response.output_item.done","output_index":1,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_meta","object":"response","created_at":1794469677.0,"model":"muse-spark-1.3","status":"completed","output":[{"type":"web_search_call","id":"ws_call_1","status":"completed","action":{"type":"search","query":"go release"}},{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":1}}}`,
		"",
		"",
	}, "\n")
	for _, tc := range []struct {
		name, path, body string
		forward          func(*OpenAIGatewayService, *gin.Context, []byte) (*OpenAIForwardResult, error)
	}{
		{"chat buffered", "/v1/chat/completions", `{"model":"muse-spark-1.3","messages":[{"role":"user","content":"news"}],"stream":false}`, chatForward},
		{"chat stream", "/v1/chat/completions", `{"model":"muse-spark-1.3","messages":[{"role":"user","content":"news"}],"stream":true}`, chatForward},
		{"messages buffered", "/v1/messages", `{"model":"muse-spark-1.3","max_tokens":64,"messages":[{"role":"user","content":"news"}],"stream":false}`, messagesForward},
		{"messages stream", "/v1/messages", `{"model":"muse-spark-1.3","max_tokens":64,"messages":[{"role":"user","content":"news"}],"stream":true}`, messagesForward},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(sseBody)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")

			result, err := tc.forward(svc, c, []byte(tc.body))
			require.NoError(t, err)
			require.Equal(t, 1, result.SearchCount)
		})
	}
}

func chatForward(svc *OpenAIGatewayService, c *gin.Context, body []byte) (*OpenAIForwardResult, error) {
	return svc.ForwardAsChatCompletions(context.Background(), c, metaGatewayTestAccount("muse-spark-1.3"), body, "", "")
}

func messagesForward(svc *OpenAIGatewayService, c *gin.Context, body []byte) (*OpenAIForwardResult, error) {
	return svc.ForwardAsAnthropic(context.Background(), c, metaGatewayTestAccount("muse-spark-1.3"), body, "", "")
}

func TestMetaGroupDefaultsWebSearchGroundingPrice(t *testing.T) {
	meta := &Group{Platform: PlatformMeta}
	require.InDelta(t, 2.5, *meta.GetSearchPricePer1k(), 1e-12)

	custom := 1.0
	require.InDelta(t, 1.0, *(&Group{Platform: PlatformMeta, SearchPricePer1k: &custom}).GetSearchPricePer1k(), 1e-12)
	require.Nil(t, (&Group{Platform: PlatformGrok}).GetSearchPricePer1k())

	billing := NewBillingService(nil, nil)
	cost := billing.CalculateSearchCost(4, meta.GetSearchPricePer1k(), 1)
	require.InDelta(t, 0.01, cost.TotalCost, 1e-12)
}
