package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const agentTestEnvelope = `{"project":"example-project","model":"gemini-3.8-flash","requestId":"example-request","unknown":{"integer":9007199254740993},"request":{"contents":[{"role":"user","parts":[{"text":"Find current information"}]}],"tools":[{"googleSearch":{},"functionDeclarations":[{"name":"client_tool","parameters":{"type":"OBJECT"}}]}],"toolConfig":{"includeServerSideToolInvocations":true},"unknownRequestField":true}}`

func agentTestResponse(parts, grounding string) *http.Response {
	if grounding == "" {
		grounding = `{}`
	}
	data := `{"response":{"responseId":"example-response","candidates":[{"content":{"role":"model","parts":` + parts + `},"finishReason":"STOP","groundingMetadata":` + grounding + `}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"cachedContentTokenCount":1,"thoughtsTokenCount":1,"totalTokenCount":13}}}`
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + data + "\n\n"))}
}

func agentTestCall(id, name, query string) string {
	return string(agentJSON(map[string]any{"functionCall": map[string]any{"id": id, "name": name, "args": map[string]any{"query": query}}, "thoughtSignature": "fake-signature-" + id}))
}

func TestPrepareAgentSearchIsolation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		body, active, err := prepareAgentSearch([]byte(agentTestEnvelope), enabled)
		require.NoError(t, err)
		require.Equal(t, enabled, active)
		require.NotContains(t, string(body), `"googleSearch"`)
		require.Contains(t, string(body), `"client_tool"`)
		require.Equal(t, enabled, strings.Contains(string(body), antigravityWebSearchBrokerTool))
		require.NotContains(t, string(body), "includeServerSideToolInvocations")
		require.Equal(t, "9007199254740993", gjson.GetBytes(body, "unknown.integer").Raw)
		require.True(t, gjson.GetBytes(body, "request.unknownRequestField").Bool())
	}
	pure := strings.Replace(agentTestEnvelope, `{"googleSearch":{},"functionDeclarations":[{"name":"client_tool","parameters":{"type":"OBJECT"}}]}`, `{"googleSearch":{}}`, 1)
	body, active, err := prepareAgentSearch([]byte(pure), true)
	require.NoError(t, err)
	require.True(t, active)
	require.JSONEq(t, pure, string(body))
	functions := strings.Replace(agentTestEnvelope, `"googleSearch":{},`, "", 1)
	body, active, err = prepareAgentSearch([]byte(functions), true)
	require.NoError(t, err)
	require.False(t, active)
	require.JSONEq(t, functions, string(body))
}

func TestAgentToolChoice(t *testing.T) {
	body, _, err := prepareAgentSearch([]byte(agentTestEnvelope), true)
	require.NoError(t, err)
	for _, tc := range []struct {
		choice     string
		active     bool
		mode, name string
	}{
		{`"none"`, false, "NONE", ""},
		{`"required"`, true, "ANY", ""},
		{`{"type":"web_search"}`, true, "ANY", antigravityWebSearchBrokerTool},
		{`{"type":"function","name":"client_tool"}`, true, "ANY", "client_tool"},
	} {
		configured, active, err := configureAgentToolChoice(body, []byte(`{"tool_choice":`+tc.choice+`}`))
		require.NoError(t, err)
		require.Equal(t, tc.active, active)
		require.Equal(t, tc.mode, gjson.GetBytes(configured, "request.toolConfig.functionCallingConfig.mode").String())
		require.Equal(t, tc.name, gjson.GetBytes(configured, "request.toolConfig.functionCallingConfig.allowedFunctionNames.0").String())
		if !active {
			require.NotContains(t, string(configured), antigravityWebSearchBrokerTool)
		}
	}
}

func TestAntigravityAgentDisabledDoesNotInjectOrLoop(t *testing.T) {
	t.Setenv("ANTIGRAVITY_AGENT_WEB_SEARCH_ENABLED", "false")
	var requests [][]byte
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{agentTestResponse(`[{"text":"No search"}]`, "")}, onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
		raw, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		requests = append(requests, raw)
	}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{"model":"gemini-3.1-pro-high","input":"Example","tools":[{"type":"web_search"},{"type":"function","name":"client_tool","parameters":{"type":"object"}}]}`)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
	_, err := svc.ForwardAsResponses(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
	require.NoError(t, err)
	require.Len(t, requests, 1)
	require.NotContains(t, string(requests[0]), antigravityWebSearchBrokerTool)
	require.NotContains(t, string(requests[0]), `"googleSearch"`)
	require.Contains(t, string(requests[0]), `"client_tool"`)
}

func TestAntigravityAgentSearchErrorIsPaired(t *testing.T) {
	t.Setenv("ANTIGRAVITY_AGENT_WEB_SEARCH_ENABLED", "true")
	var requests [][]byte
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{
		{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"synthetic rejection"}}`))},
		agentTestResponse(`[{"text":"Search was unavailable"}]`, ""),
	}, onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
		raw, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		requests = append(requests, raw)
	}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", []byte(`{}`))
	prepared, _, err := prepareAgentSearch([]byte(agentTestEnvelope), true)
	require.NoError(t, err)
	call := &antigravityCompatUpstreamCall{geminiBody: prepared, request: antigravityCompatRequest{originalModel: "gemini-3.8-flash"}}
	resp, err := svc.runAntigravityWebSearchAgentLoop(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), call, agentTestResponse(`[`+agentTestCall("search-error", antigravityWebSearchBrokerTool, "example")+`]`, ""))
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
	require.Len(t, requests, 2)
	require.Equal(t, "search-error", gjson.GetBytes(requests[1], "request.contents.2.parts.0.functionResponse.id").String())
	require.Equal(t, "upstream_rejected_search", gjson.GetBytes(requests[1], "request.contents.2.parts.0.functionResponse.response.error.code").String())
	require.Empty(t, recorder.Body.String())
	require.Empty(t, call.agentResult.searches)
}

func TestAntigravityAgentRoundLimitLeavesResponseUncommitted(t *testing.T) {
	t.Setenv("ANTIGRAVITY_AGENT_MAX_TOOL_ROUNDS", "1")
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{agentTestResponse(`[{"text":"Search result"}]`, ""), agentTestResponse(`[`+agentTestCall("search-again", antigravityWebSearchBrokerTool, "again")+`]`, "")}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", []byte(`{}`))
	prepared, _, err := prepareAgentSearch([]byte(agentTestEnvelope), true)
	require.NoError(t, err)
	call := &antigravityCompatUpstreamCall{geminiBody: prepared, request: antigravityCompatRequest{originalModel: "gemini-3.8-flash"}}
	_, err = svc.runAntigravityWebSearchAgentLoop(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), call, agentTestResponse(`[`+agentTestCall("search-first", antigravityWebSearchBrokerTool, "first")+`]`, ""))
	require.ErrorContains(t, err, "round limit")
	require.Empty(t, recorder.Body.String())
}

func TestAgentSearchRoundTripPreservesSignaturesAndPairs(t *testing.T) {
	parts := `[` + agentTestCall("search-a", antigravityWebSearchBrokerTool, "alpha") + `,` + agentTestCall("search-b", antigravityWebSearchBrokerTool, "beta") + `,` + agentTestCall("client-a", "client_tool", "unused") + `,{"thought":true,"thoughtSignature":"fake-tail-signature"}]`
	turn, err := readAntigravityAgentTurn(context.Background(), agentTestResponse(parts, ""), 0)
	require.NoError(t, err)
	body, err := appendAgentToolRound([]byte(agentTestEnvelope), turn.content, map[string]json.RawMessage{"search-a": agentJSON(map[string]string{"text": "alpha result"}), "search-b": agentJSON(map[string]string{"text": "beta result"})})
	require.NoError(t, err)
	require.Equal(t, "fake-signature-search-a", gjson.GetBytes(body, "request.contents.1.parts.0.thoughtSignature").String())
	require.Equal(t, "fake-tail-signature", gjson.GetBytes(body, "request.contents.1.parts.3.thoughtSignature").String())
	require.True(t, gjson.GetBytes(body, "request.contents.1.parts.3.text").Exists())
	require.Equal(t, "search-a", gjson.GetBytes(body, "request.contents.2.parts.0.functionResponse.id").String())
	require.Equal(t, "alpha result", gjson.GetBytes(body, "request.contents.2.parts.0.functionResponse.response.text").String())
	require.Equal(t, "beta result", gjson.GetBytes(body, "request.contents.2.parts.1.functionResponse.response.text").String())
	require.True(t, gjson.GetBytes(body, "request.contents.2.parts.2.functionResponse.response.deferred").Bool())
	require.Equal(t, "9007199254740993", gjson.GetBytes(body, "unknown.integer").Raw)
}

func TestAgentTurnRejectsMalformedTruncatedAndOversizedStreams(t *testing.T) {
	for _, input := range []string{"data: {invalid}\n\n", "data: {}\n\n", `data: {"response":{"candidates":[]}}` + "\n\n", strings.Repeat(": heartbeat\n", antigravityAgentMaxResponseBytes/12+1)} {
		resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(input))}
		_, err := readAntigravityAgentTurn(context.Background(), resp, 0)
		require.Error(t, err)
	}
}

func TestAgentTurnCancellationAndIdleTimeout(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		reader, writer := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		if cancelRequest {
			cancel()
		}
		_, err := readAntigravityAgentTurn(ctx, &http.Response{StatusCode: 200, Body: reader}, 10*time.Millisecond)
		cancel()
		_ = writer.Close()
		require.Error(t, err)
		if cancelRequest {
			require.ErrorIs(t, err, context.Canceled)
		} else {
			require.Contains(t, err.Error(), "interval timeout")
		}
	}
}

func TestAntigravityAgentMultipleSearchesAndClientTools(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Setenv("ANTIGRAVITY_AGENT_WEB_SEARCH_ENABLED", "true")
		parts := `[` + agentTestCall("search-a", antigravityWebSearchBrokerTool, "alpha") + `,` + agentTestCall("search-b", antigravityWebSearchBrokerTool, "beta") + `,` + agentTestCall("client-pending", "client_tool", "pending") + `]`
		grounding := `{"webSearchQueries":["example query"],"groundingChunks":[{"web":{"uri":"https://example.com/source","title":"Example source"}}],"groundingSupports":[{"segment":{"startIndex":0,"endIndex":5,"text":"alpha"},"groundingChunkIndices":[0]}]}`
		var requests [][]byte
		upstream := &queuedHTTPUpstreamStub{
			responses: []*http.Response{agentTestResponse(parts, ""), agentTestResponse(`[{"text":"alpha result","thoughtSignature":"fake-search-signature-a"}]`, grounding), agentTestResponse(`[{"text":"beta result"}]`, grounding), agentTestResponse(`[`+agentTestCall("client-final", "client_tool", "complete")+`]`, "")},
			onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
				raw, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				requests = append(requests, raw)
			},
		}
		svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
		body := agentJSON(map[string]any{"model": "gemini-3.1-pro-high", "input": "Find current information", "stream": stream, "tools": []any{map[string]any{"type": "web_search"}, map[string]any{"type": "function", "name": "client_tool", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}})
		c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
		result, err := svc.ForwardAsResponses(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
		require.NoError(t, err)
		require.Len(t, requests, 4)
		for _, request := range requests {
			require.False(t, strings.Contains(string(request), `"googleSearch"`) && strings.Contains(string(request), `"functionDeclarations"`))
		}
		require.Equal(t, "alpha", gjson.GetBytes(requests[1], "request.contents.0.parts.0.text").String())
		require.Equal(t, "beta", gjson.GetBytes(requests[2], "request.contents.0.parts.0.text").String())
		require.Contains(t, string(requests[3]), "fake-search-signature-a")
		require.True(t, gjson.GetBytes(requests[3], "request.contents.2.parts.2.functionResponse.response.deferred").Bool())
		require.Equal(t, 36, result.Usage.InputTokens)
		require.Equal(t, 12, result.Usage.OutputTokens)
		require.Equal(t, 4, result.Usage.CacheReadInputTokens)
		require.NotContains(t, recorder.Body.String(), antigravityWebSearchBrokerTool)
		require.Contains(t, recorder.Body.String(), `"url_citation"`)
		require.Contains(t, recorder.Body.String(), `"client_tool"`)
		if stream {
			sequence, searches, created, completed := 0, 0, 0, 0
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				event := gjson.Parse(strings.TrimPrefix(line, "data: "))
				require.True(t, event.Get("sequence_number").Exists())
				require.Equal(t, int64(sequence), event.Get("sequence_number").Int())
				sequence++
				switch event.Get("type").String() {
				case "response.created":
					created++
				case "response.completed":
					completed++
				case "response.web_search_call.completed":
					searches++
					require.True(t, event.Get("output_index").Exists())
				}
			}
			require.Equal(t, 2, searches)
			require.Equal(t, 1, created)
			require.Equal(t, 1, completed)
		} else {
			var response apicompat.ResponsesResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "alpha", response.Output[0].Action.Query)
			require.Equal(t, "beta", response.Output[1].Action.Query)
			require.Equal(t, 52, response.Usage.TotalTokens)
			part := response.Output[3].Content[0]
			for _, annotation := range part.Annotations {
				require.Greater(t, annotation.EndIndex, annotation.StartIndex)
				require.Equal(t, "Example source", string([]rune(part.Text)[annotation.StartIndex:annotation.EndIndex]))
			}
		}
	}
}

func TestAntigravityAgentRepeatedSearchIsNotExecuted(t *testing.T) {
	var requests [][]byte
	decision := func(id string) *http.Response {
		return agentTestResponse(`[`+agentTestCall(id, antigravityWebSearchBrokerTool, "same query")+`]`, "")
	}
	search := func() *http.Response {
		return agentTestResponse(`[{"text":"Example result"}]`, `{"webSearchQueries":["same query"]}`)
	}
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{search(), decision("second"), search(), decision("third"), agentTestResponse(`[{"text":"Final answer"}]`, "")}, onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
		raw, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		requests = append(requests, raw)
	}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", []byte(`{}`))
	prepared, _, err := prepareAgentSearch([]byte(agentTestEnvelope), true)
	require.NoError(t, err)
	call := &antigravityCompatUpstreamCall{geminiBody: prepared, request: antigravityCompatRequest{originalModel: "gemini-3.8-flash"}}
	resp, err := svc.runAntigravityWebSearchAgentLoop(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), call, decision("first"))
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Len(t, requests, 5)
	require.Contains(t, string(requests[4]), `"code":"repeated_query"`)
	require.Len(t, call.agentResult.searches, 2)
	require.Empty(t, recorder.Body.String())
}

func TestAntigravityAgentNativeSearchStaysNative(t *testing.T) {
	t.Setenv("ANTIGRAVITY_AGENT_WEB_SEARCH_ENABLED", "true")
	var sent []byte
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{agentTestResponse(`[{"text":"Example result"}]`, `{"webSearchQueries":["example query"],"groundingChunks":[{"web":{"uri":"https://example.org/","title":"Example"}}]}`)}, onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
		raw, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		sent = raw
	}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{"model":"gemini-3.1-pro-high","input":"Search the web","tools":[{"type":"web_search"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
	_, err := svc.ForwardAsResponses(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
	require.NoError(t, err)
	require.Contains(t, string(sent), `"googleSearch"`)
	require.NotContains(t, string(sent), `"functionDeclarations"`)
	require.Contains(t, recorder.Body.String(), `"web_search_call"`)
	require.Contains(t, recorder.Body.String(), `"url_citation"`)
}

func TestAgentSearchRestrictionsAreNotSilentlyDiscarded(t *testing.T) {
	body, _, err := prepareAgentSearch([]byte(agentTestEnvelope), true)
	require.NoError(t, err)
	for _, option := range []string{`"external_web_access":false`, `"filters":{"allowed_domains":["example.org"]}`, `"user_location":{"type":"approximate","country":"US"}`} {
		_, _, err := configureAgentToolChoice(body, []byte(`{"tools":[{"type":"web_search",`+option+`}]}`))
		require.Error(t, err)
	}
}

func TestAgentExternalWebAccessRequiresBoolean(t *testing.T) {
	body, _, err := prepareAgentSearch([]byte(agentTestEnvelope), true)
	require.NoError(t, err)
	for _, kind := range []string{"web_search", "web_search_preview", "web_search_preview_2025_03_11"} {
		for _, tc := range []struct {
			name, value string
			allowed     bool
		}{
			{"omitted", "", true},
			{"true", "true", true},
			{"true_whitespace", "\n true \t", true},
			{"false", "false", false},
			{"false_whitespace", "\n false \t", false},
			{"string_false", `"false"`, false},
			{"string_true", `"true"`, false},
			{"null", "null", false},
			{"number", "0", false},
			{"object", "{}", false},
			{"array", "[]", false},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				option := ""
				if tc.value != "" {
					option = `,"external_web_access":` + tc.value
				}
				original := []byte(`{"tools":[{"type":"` + kind + `"` + option + `}]}`)
				_, active, err := configureAgentToolChoice(body, original)
				if tc.allowed {
					require.NoError(t, err)
					require.True(t, active)
				} else {
					require.Error(t, err)
					require.False(t, active)
				}
			})
		}
	}
}

func TestAgentExternalWebAccessRejectedBeforeUpstream(t *testing.T) {
	t.Setenv("ANTIGRAVITY_AGENT_WEB_SEARCH_ENABLED", "true")
	for _, value := range []string{"false", "\n false \t", `"false"`, `"true"`, "null", "0", "{}", "[]"} {
		for _, mixed := range []bool{false, true} {
			upstream := &queuedHTTPUpstreamStub{}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			clientTool := ""
			if mixed {
				clientTool = `,{"type":"function","name":"client_tool","parameters":{"type":"object"}}`
			}
			body := []byte(`{"model":"gemini-3.1-pro-high","input":"Example","tools":[{"type":"web_search","external_web_access":` + value + `}` + clientTool + `]}`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
			_, err := svc.ForwardAsResponses(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Zero(t, upstream.callCount)
		}
	}
}
