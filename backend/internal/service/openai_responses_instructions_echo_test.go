package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const instructionsEchoMarker = "UPSTREAM-ECHOED-INSTRUCTIONS"

func instructionsEchoUpstreamSSE(echo string) string {
	return strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_echo","status":"in_progress","instructions":` + echo + `}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"OK"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_echo","status":"completed","instructions":` + echo + `,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":11,"output_tokens":5,"total_tokens":16}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
}

func instructionsEchoUpstream(echo string) *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_echo"}},
		Body:       io.NopCloser(strings.NewReader(instructionsEchoUpstreamSSE(echo))),
	}}
}

func instructionsEchoOAuthAccount(passthrough bool) *Account {
	account := &Account{
		ID: 501, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
		Status:      StatusActive, Schedulable: true, RateMultiplier: f64p(1),
	}
	if passthrough {
		account.Extra = map[string]any{"openai_passthrough": true}
	}
	return account
}

func instructionsEchoForward(t *testing.T, account *Account, upstream *httpUpstreamRecorder, body string) (*OpenAIForwardResult, *httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "OpenAI/Python 2.8.0")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	result, err := svc.Forward(context.Background(), c, account, []byte(body))
	return result, rec, err
}

func sseResponseInstructions(t *testing.T, body, eventType string) gjson.Result {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || gjson.Get(data, "type").String() != eventType {
			continue
		}
		return gjson.Get(data, "response.instructions")
	}
	t.Fatalf("event %s not found in body: %s", eventType, body)
	return gjson.Result{}
}

func TestOpenAIResponsesInstructionsEchoRestoreHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name        string
		clientBody  string
		upstream    string
		wantRestore string
	}{
		{name: "missing then injected restores null", clientBody: `{"model":"m"}`, upstream: `{"instructions":"injected"}`, wantRestore: "null"},
		{name: "blank then injected restores blank", clientBody: `{"instructions":" "}`, upstream: `{"instructions":"injected"}`, wantRestore: `" "`},
		{name: "client text appended restores client text", clientBody: `{"instructions":"mine"}`, upstream: `{"instructions":"mine\n\nbridge"}`, wantRestore: `"mine"`},
		{name: "unchanged text keeps upstream echo", clientBody: `{"instructions":"mine"}`, upstream: `{"instructions":"mine"}`, wantRestore: ""},
		{name: "escaped spelling of same text keeps upstream echo", clientBody: `{"instructions":"a\/b"}`, upstream: `{"instructions":"a/b"}`, wantRestore: ""},
		{name: "reformatted object keeps upstream echo", clientBody: "{\"instructions\": {\n  \"k\": [1, 2]\n}}", upstream: `{"instructions":{"k":[1,2]}}`, wantRestore: ""},
		{name: "multi-line object then injected restores single line", clientBody: "{\"instructions\": [\n  {\"role\": \"developer\", \"content\": \"x\"}\n]}", upstream: `{"instructions":"injected"}`, wantRestore: `[{"role":"developer","content":"x"}]`},
		{name: "missing on both sides keeps upstream echo", clientBody: `{"model":"m"}`, upstream: `{"model":"m"}`, wantRestore: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			rememberOpenAIResponsesClientInstructions(c, []byte(tt.clientBody))
			syncOpenAIResponsesInstructionsEcho(c, []byte(tt.upstream))
			require.Equal(t, tt.wantRestore, openAIResponsesInstructionsEchoRestoreValue(c))
		})
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	event := []byte(`{"type":"response.created","response":{"id":"r","instructions":"injected"}}`)
	delta := []byte(`{"type":"response.output_text.delta","delta":"the word \"instructions\" in text"}`)
	require.Equal(t, event, restoreOpenAIResponsesInstructionsEchoInEvent(c, event), "unregistered context must not rewrite")

	rememberOpenAIResponsesClientInstructions(c, []byte(`{"model":"m"}`))
	syncOpenAIResponsesInstructionsEcho(c, []byte(`{"instructions":"injected"}`))
	require.JSONEq(t, `{"type":"response.created","response":{"id":"r","instructions":null}}`, string(restoreOpenAIResponsesInstructionsEchoInEvent(c, event)))
	require.Equal(t, delta, restoreOpenAIResponsesInstructionsEchoInEvent(c, delta))
	require.JSONEq(t, `{"id":"r","instructions":null}`, string(restoreOpenAIResponsesInstructionsEchoInResponse(c, []byte(`{"id":"r","instructions":"injected"}`))))
	sseBody := "event: response.created\ndata: " + string(event) + "\n\ndata: " + string(delta) + "\n"
	restoredSSE := restoreOpenAIResponsesInstructionsEchoInSSEBody(c, sseBody)
	require.NotContains(t, restoredSSE, `"instructions":"injected"`)
	require.Contains(t, restoredSSE, `"instructions":null`)
	require.Contains(t, restoredSSE, string(delta))

	rememberOpenAIResponsesClientInstructions(c, []byte(`{"model":"m"}`))
	require.Equal(t, event, restoreOpenAIResponsesInstructionsEchoInEvent(c, event), "a new attempt clears the previous restore value")
}

func TestOpenAIGatewayService_OAuthPassthrough_CodexBlankInstructionsForwardedAsIs(t *testing.T) {
	for _, raw := range []string{`""`, `" "`, `{"text":"x"}`} {
		t.Run(raw, func(t *testing.T) {
			upstream := instructionsEchoUpstream(raw)
			result, rec, err := instructionsEchoForward(t, instructionsEchoOAuthAccount(true), upstream,
				`{"model":"gpt-5.3-codex","stream":true,"instructions":`+raw+`,"input":[{"type":"message","role":"user","content":"Hi"}]}`)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, raw, gjson.GetBytes(upstream.lastBody, "instructions").Raw)
			require.JSONEq(t, raw, sseResponseInstructions(t, rec.Body.String(), "response.completed").Raw)
		})
	}
}

func TestOpenAIGatewayService_OAuthPassthrough_MissingInstructionsForwardedWithoutField(t *testing.T) {
	upstream := instructionsEchoUpstream("null")
	result, rec, err := instructionsEchoForward(t, instructionsEchoOAuthAccount(true), upstream,
		`{"model":"gpt-6.1-sol","stream":true,"input":[{"type":"message","role":"user","content":"Hi"}]}`)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
	require.Equal(t, gjson.Null, sseResponseInstructions(t, rec.Body.String(), "response.created").Type)
	require.Equal(t, gjson.Null, sseResponseInstructions(t, rec.Body.String(), "response.completed").Type)
}

func TestOpenAIGatewayService_OAuthPassthrough_InjectedCodexInstructionsEchoRestored(t *testing.T) {
	upstream := instructionsEchoUpstream(`"` + instructionsEchoMarker + `"`)
	result, rec, err := instructionsEchoForward(t, instructionsEchoOAuthAccount(true), upstream,
		`{"model":"gpt-5.3-codex","stream":true,"input":[{"type":"message","role":"user","content":"Hi"}]}`)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, gjson.GetBytes(upstream.lastBody, "instructions").String())
	require.NotContains(t, rec.Body.String(), instructionsEchoMarker)
	require.Equal(t, gjson.Null, sseResponseInstructions(t, rec.Body.String(), "response.created").Type)
	require.Equal(t, gjson.Null, sseResponseInstructions(t, rec.Body.String(), "response.completed").Type)

	nonStreamUpstream := instructionsEchoUpstream(`"` + instructionsEchoMarker + `"`)
	result, rec, err = instructionsEchoForward(t, instructionsEchoOAuthAccount(true), nonStreamUpstream,
		`{"model":"gpt-5.3-codex","stream":false,"input":[{"type":"message","role":"user","content":"Hi"}]}`)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotContains(t, rec.Body.String(), instructionsEchoMarker)
	instructions := gjson.Get(rec.Body.String(), "instructions")
	require.True(t, instructions.Exists())
	require.Equal(t, gjson.Null, instructions.Type)
}

func TestOpenAIGatewayService_OAuthResponses_MultiLineInstructionsEchoKeepsSSEFraming(t *testing.T) {
	upstream := instructionsEchoUpstream(`"` + instructionsEchoMarker + `"`)
	result, rec, err := instructionsEchoForward(t, instructionsEchoOAuthAccount(false), upstream,
		"{\"model\":\"gpt-6.1-sol\",\"stream\":true,\"instructions\": [\n  {\"role\": \"developer\", \"content\": \"x\"}\n],\"input\":[{\"type\":\"message\",\"role\":\"user\",\"content\":\"Hi\"}]}")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotContains(t, rec.Body.String(), instructionsEchoMarker)
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, ":") {
			continue
		}
		data, ok := extractOpenAISSEDataLine(line)
		require.True(t, ok, "unexpected non-data line %q", line)
		if data == "[DONE]" {
			continue
		}
		require.True(t, gjson.Valid(data), "data line must stay a complete JSON document: %q", data)
	}
	require.JSONEq(t, `[{"role":"developer","content":"x"}]`, sseResponseInstructions(t, rec.Body.String(), "response.completed").Raw)
}

func TestOpenAIGatewayService_OAuthResponses_InjectedInstructionsEchoRestored(t *testing.T) {
	t.Run("stream", func(t *testing.T) {
		upstream := instructionsEchoUpstream(`"` + instructionsEchoMarker + `"`)
		result, rec, err := instructionsEchoForward(t, instructionsEchoOAuthAccount(false), upstream,
			`{"model":"gpt-6.1-sol","stream":true,"input":[{"type":"message","role":"user","content":"Hi"}]}`)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEmpty(t, gjson.GetBytes(upstream.lastBody, "instructions").String())
		require.NotContains(t, rec.Body.String(), instructionsEchoMarker)
		require.Equal(t, gjson.Null, sseResponseInstructions(t, rec.Body.String(), "response.created").Type)
		require.Equal(t, gjson.Null, sseResponseInstructions(t, rec.Body.String(), "response.completed").Type)
	})

	t.Run("non-stream", func(t *testing.T) {
		upstream := instructionsEchoUpstream(`"` + instructionsEchoMarker + `"`)
		result, rec, err := instructionsEchoForward(t, instructionsEchoOAuthAccount(false), upstream,
			`{"model":"gpt-6.1-sol","stream":false,"input":[{"type":"message","role":"user","content":"Hi"}]}`)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotContains(t, rec.Body.String(), instructionsEchoMarker)
		instructions := gjson.Get(rec.Body.String(), "instructions")
		require.True(t, instructions.Exists())
		require.Equal(t, gjson.Null, instructions.Type)
	})

	t.Run("client instructions kept", func(t *testing.T) {
		upstream := instructionsEchoUpstream(`"client rule"`)
		result, rec, err := instructionsEchoForward(t, instructionsEchoOAuthAccount(false), upstream,
			`{"model":"gpt-6.1-sol","stream":true,"instructions":"client rule","input":[{"type":"message","role":"user","content":"Hi"}]}`)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "client rule", gjson.GetBytes(upstream.lastBody, "instructions").String())
		require.Equal(t, "client rule", sseResponseInstructions(t, rec.Body.String(), "response.completed").String())
	})
}
