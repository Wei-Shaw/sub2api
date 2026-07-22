//go:build unit

package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPatchGrokResponsesBodyWithClientToolsLowersCodexProtocol(t *testing.T) {
	t.Parallel()

	body := grokClientToolProtocolRequest(false)
	patched, mapping, err := patchGrokResponsesBodyWithClientTools(body, "grok-4.5")
REDACTED
	require.True(t, json.Valid(patched))
	require.True(t, mapping.CustomTools["apply_patch"])
	require.True(t, mapping.ToolSearch)
	require.Equal(t, "collaboration", mapping.NamespaceTools["collaboration__send_message"].Namespace)
	require.Equal(t, "send_message", mapping.NamespaceTools["collaboration__send_message"].Name)

	tools := gjson.GetBytes(patched, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "function", tools[0].Get("type").String())
	require.Equal(t, "apply_patch", tools[0].Get("name").String())
	require.Equal(t, "string", tools[0].Get("parameters.properties.input.type").String())
	require.False(t, tools[0].Get("format").Exists())
	require.Equal(t, "function", tools[1].Get("type").String())
	require.Equal(t, "tool_search", tools[1].Get("name").String())
	require.Equal(t, "function", tools[2].Get("type").String())
	require.Equal(t, "collaboration__send_message", tools[2].Get("name").String())
	require.False(t, gjson.GetBytes(patched, `tools.#(type=="custom")`).Exists())
	require.False(t, gjson.GetBytes(patched, `tools.#(type=="namespace")`).Exists())
	require.False(t, gjson.GetBytes(patched, `tools.#(type=="tool_search")`).Exists())

	require.Equal(t, "function", gjson.GetBytes(patched, "tool_choice.type").String())
	require.Equal(t, "apply_patch", gjson.GetBytes(patched, "tool_choice.name").String())
	require.Equal(t, "function_call", gjson.GetBytes(patched, "input.0.type").String())
	require.JSONEq(t, `{"input":"*** Begin Patch"REDACTED`, gjson.GetBytes(patched, "input.0.arguments").String())
	require.False(t, gjson.GetBytes(patched, "input.0.input").Exists())
	require.Equal(t, "function_call_output", gjson.GetBytes(patched, "input.1.type").String())
	require.Equal(t, "function_call", gjson.GetBytes(patched, "input.2.type").String())
	require.Equal(t, "tool_search", gjson.GetBytes(patched, "input.2.name").String())
	require.JSONEq(t, `{"query":"github"REDACTED`, gjson.GetBytes(patched, "input.2.arguments").String())
	require.False(t, gjson.GetBytes(patched, "input.2.execution").Exists())
	require.Equal(t, "function_call_output", gjson.GetBytes(patched, "input.3.type").String())
	require.JSONEq(t, `{"groups":["github"]REDACTED`, gjson.GetBytes(patched, "input.3.output").String())
	require.Equal(t, "function_call", gjson.GetBytes(patched, "input.4.type").String())
	require.Equal(t, "collaboration__send_message", gjson.GetBytes(patched, "input.4.name").String())
	require.False(t, gjson.GetBytes(patched, "input.4.namespace").Exists())
REDACTED

func TestPatchGrokResponsesBodyWithClientToolsRewritesEveryToolChoice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		choice   string
		wantName string
		wantType string
		wantNoNS bool
REDACTED{
		{
			name:     "custom",
			choice:   `{"type":"custom","name":"apply_patch"REDACTED`,
			wantName: "apply_patch",
			wantType: "function",
	REDACTED,
		{
			name:     "tool search",
			choice:   `{"type":"tool_search"REDACTED`,
			wantName: "tool_search",
			wantType: "function",
	REDACTED,
		{
			name:     "namespace function",
			choice:   `{"type":"function","namespace":"collaboration","name":"send_message"REDACTED`,
			wantName: "collaboration__send_message",
			wantType: "function",
			wantNoNS: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := []byte(fmt.Sprintf(`{
				"model":"grok","input":"hello",
				"tools":[
					{"type":"custom","name":"apply_patch"REDACTED,
					{"type":"tool_search"REDACTED,
					{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"send_message","parameters":{"type":"object"REDACTEDREDACTED]REDACTED
				],
				"tool_choice":%s
		REDACTED`, tt.choice))

			patched, _, err := patchGrokResponsesBodyWithClientTools(body, "grok-4.5")
		REDACTED
			require.Equal(t, tt.wantType, gjson.GetBytes(patched, "tool_choice.type").String())
			require.Equal(t, tt.wantName, gjson.GetBytes(patched, "tool_choice.name").String())
			if tt.wantNoNS {
				require.False(t, gjson.GetBytes(patched, "tool_choice.namespace").Exists())
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestPatchGrokResponsesBodyWithClientToolsRejectsTrailingJSONDocument(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"grok","input":"hello","tools":[{"type":"custom","name":"apply_patch"REDACTED]REDACTED {"ignored":trueREDACTED`)
	patched, mapping, err := patchGrokResponsesBodyWithClientTools(body, "grok-4.5")

REDACTED
	require.Contains(t, strings.ToLower(err.Error()), "invalid json")
	require.Nil(t, patched)
	require.Empty(t, mapping.CustomTools)
REDACTED

func TestClearGrokResponsesClientToolMappingRemovesStaleContextState(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	setGrokResponsesClientToolMapping(c, apicompat.ResponsesClientToolMapping{
		CustomTools: map[string]bool{"stale_tool": trueREDACTED,
REDACTED)

	_, seeded := grokResponsesClientToolMapping(c)
	require.True(t, seeded)
	clearGrokResponsesClientToolMapping(c)
	_, remains := grokResponsesClientToolMapping(c)
	require.False(t, remains)
REDACTED

func TestForwardGrokResponsesClientToolNameConflictReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"grok","stream":false,"input":"hello",
		"tools":[
			{"type":"custom","name":"duplicate"REDACTED,
			{"type":"function","name":"duplicate","parameters":{"type":"object"REDACTEDREDACTED
		]
REDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{REDACTED
	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := grokProtocolAPIKeyAccount(7101)

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", false, time.Now())

REDACTED
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
	require.Equal(t, "tools", gjson.Get(recorder.Body.String(), "error.param").String())
	require.Contains(t, gjson.Get(recorder.Body.String(), "error.message").String(), "conflicts")
	require.Empty(t, upstream.requests, "an ambiguous request must not reach xAI")
REDACTED

func TestForwardGrokResponsesOAuthRestoresClientToolsNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := grokClientToolProtocolRequest(false)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: 7102REDACTED)

	account := grokProtocolOAuthAccount(7102)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{"application/json"REDACTED,
			"Xai-Request-Id": []string{"protocol-oauth"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_protocol_oauth","object":"response","model":"grok-4.5","status":"completed",
			"output":[
				{"type":"function_call","id":"item_custom","call_id":"call_custom","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"REDACTED","namespace":"must_not_leak"REDACTED,
				{"type":"function_call","id":"item_search","call_id":"call_search","name":"tool_search","arguments":"{\"query\":\"github\"REDACTED"REDACTED,
				{"type":"function_call","id":"item_namespace","call_id":"call_namespace","name":"collaboration__send_message","arguments":"{\"target\":\"root\"REDACTED"REDACTED
			],
			"usage":{"input_tokens":9,"output_tokens":3,"total_tokens":12REDACTED
	REDACTED`)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", false, time.Now())

REDACTED
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, "resp_protocol_oauth", result.ResponseID)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer oauth-protocol-token", upstream.lastReq.Header.Get("Authorization"))
	assertGrokProtocolRequestLowered(t, upstream.lastBody)

	response := recorder.Body.Bytes()
	require.Equal(t, "custom_tool_call", gjson.GetBytes(response, "output.0.type").String())
	require.Equal(t, "*** Begin Patch", gjson.GetBytes(response, "output.0.input").String())
	require.False(t, gjson.GetBytes(response, "output.0.arguments").Exists())
	require.False(t, gjson.GetBytes(response, "output.0.namespace").Exists())
	require.Equal(t, "tool_search_call", gjson.GetBytes(response, "output.1.type").String())
	require.Equal(t, "client", gjson.GetBytes(response, "output.1.execution").String())
	require.Equal(t, "github", gjson.GetBytes(response, "output.1.arguments.query").String())
	require.False(t, gjson.GetBytes(response, "output.1.name").Exists())
	require.Equal(t, "function_call", gjson.GetBytes(response, "output.2.type").String())
	require.Equal(t, "collaboration", gjson.GetBytes(response, "output.2.namespace").String())
	require.Equal(t, "send_message", gjson.GetBytes(response, "output.2.name").String())
REDACTED

func TestForwardGrokResponsesAPIKeyRestoresClientToolsFromSSEForNonStreamingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := grokClientToolProtocolRequest(false)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{"text/event-stream"REDACTED,
			"Xai-Request-Id": []string{"protocol-api-key-sse-nonstream"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(grokProtocolUpstreamSSE())),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := grokProtocolAPIKeyAccount(7104)

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", false, time.Now())

REDACTED
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, "resp_protocol_stream", result.ResponseID)
	assertGrokProtocolRequestLowered(t, upstream.lastBody)

	response := recorder.Body.Bytes()
	require.True(t, json.Valid(response))
	require.Equal(t, "custom_tool_call", gjson.GetBytes(response, "output.0.type").String())
	require.Equal(t, "*** Begin Patch", gjson.GetBytes(response, "output.0.input").String())
	require.Equal(t, "tool_search_call", gjson.GetBytes(response, "output.1.type").String())
	require.Equal(t, "client", gjson.GetBytes(response, "output.1.execution").String())
	require.Equal(t, "collaboration", gjson.GetBytes(response, "output.2.namespace").String())
	require.Equal(t, "send_message", gjson.GetBytes(response, "output.2.name").String())
REDACTED

func TestForwardGrokResponsesAPIKeyRestoresClientToolsStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := grokClientToolProtocolRequest(true)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{"text/event-stream"REDACTED,
			"Xai-Request-Id": []string{"protocol-api-key"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(grokProtocolUpstreamSSE())),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := grokProtocolAPIKeyAccount(7103)

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", true, time.Now())

REDACTED
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, "resp_protocol_stream", result.ResponseID)
	require.Equal(t, "https://api.x.ai/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer xai-protocol-key", upstream.lastReq.Header.Get("Authorization"))
	assertGrokProtocolRequestLowered(t, upstream.lastBody)

	frames := parseGrokProtocolSSEFrames(t, recorder.Body.String())
	require.NotEmpty(t, frames)
	for index, frame := range frames {
		require.Equal(t, frame.event, gjson.GetBytes(frame.data, "type").String(), "SSE event field must follow the restored data.type")
		require.Equal(t, 40+index, int(gjson.GetBytes(frame.data, "sequence_number").Int()), "sequence_number must be continuous after suppressed and expanded events")
REDACTED

	created := requireGrokProtocolFrame(t, frames, "response.created", "", "")
	require.True(t, gjson.GetBytes(created.data, "upstream_extension.preserved").Bool())
	customAdded := requireGrokProtocolFrame(t, frames, "response.output_item.added", "item.type", "custom_tool_call")
	require.Equal(t, "apply_patch", gjson.GetBytes(customAdded.data, "item.name").String())
	customInputDelta := requireGrokProtocolFrame(t, frames, "response.custom_tool_call_input.delta", "", "")
	require.Equal(t, "*** Begin Patch", gjson.GetBytes(customInputDelta.data, "delta").String())
	customInputDone := requireGrokProtocolFrame(t, frames, "response.custom_tool_call_input.done", "", "")
	require.Equal(t, "*** Begin Patch", gjson.GetBytes(customInputDone.data, "input").String())
	customDone := requireGrokProtocolFrame(t, frames, "response.output_item.done", "item.type", "custom_tool_call")
	require.Equal(t, "*** Begin Patch", gjson.GetBytes(customDone.data, "item.input").String())

	namespaceAdded := requireGrokProtocolFrame(t, frames, "response.output_item.added", "item.namespace", "collaboration")
	require.Equal(t, "send_message", gjson.GetBytes(namespaceAdded.data, "item.name").String())
	namespaceDone := requireGrokProtocolFrame(t, frames, "response.output_item.done", "item.namespace", "collaboration")
	require.Equal(t, "send_message", gjson.GetBytes(namespaceDone.data, "item.name").String())
	namespaceArgumentsDone := requireGrokProtocolFrame(t, frames, "response.function_call_arguments.done", "name", "send_message")
	require.Equal(t, "response.function_call_arguments.done", gjson.GetBytes(namespaceArgumentsDone.data, "type").String())
	require.False(t, gjson.GetBytes(namespaceArgumentsDone.data, "namespace").Exists())

	searchAdded := requireGrokProtocolFrame(t, frames, "response.output_item.added", "item.type", "tool_search_call")
	require.Equal(t, "client", gjson.GetBytes(searchAdded.data, "item.execution").String())
	searchDone := requireGrokProtocolFrame(t, frames, "response.output_item.done", "item.type", "tool_search_call")
	require.Equal(t, "github", gjson.GetBytes(searchDone.data, "item.arguments.query").String())

	for _, frame := range frames {
		itemID := gjson.GetBytes(frame.data, "item_id").String()
		if itemID == "item_custom" || itemID == "item_search" {
			require.NotContains(t, frame.event, "function_call_arguments", "client-only proxy argument events must not leak")
	REDACTED
REDACTED
	completed := requireGrokProtocolFrame(t, frames, "response.completed", "", "")
	require.Equal(t, "custom_tool_call", gjson.GetBytes(completed.data, "response.output.0.type").String())
	require.Equal(t, "tool_search_call", gjson.GetBytes(completed.data, "response.output.1.type").String())
	require.Equal(t, "collaboration", gjson.GetBytes(completed.data, "response.output.2.namespace").String())
REDACTED

func TestGrokResponsesClientToolStreamBodyFlushesFrameBeforeEOF(t *testing.T) {
	sourceReader, sourceWriter := io.Pipe()
	body := newGrokResponsesClientToolStreamBody(sourceReader, apicompat.ResponsesClientToolMapping{
		CustomTools: map[string]bool{"apply_patch": trueREDACTED,
REDACTED, defaultMaxLineSize)
	defer func() { _ = body.Close() REDACTED()
	defer func() { _ = sourceWriter.Close() REDACTED()

	type readResult struct {
		frame string
		err   error
REDACTED
	read := make(chan readResult, 1)
	go func() {
		reader := bufio.NewReader(body)
		var frame strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				read <- readResult{err: errREDACTED
				return
		REDACTED
			frame.WriteString(line)
			if strings.TrimSpace(line) == "" {
				read <- readResult{frame: frame.String()REDACTED
				return
		REDACTED
	REDACTED
REDACTED()

	firstFrame := "event: response.created\n" +
		`data: {"type":"response.created","sequence_number":0,"response":{"id":"flush-before-eof"REDACTEDREDACTED` + "\n\n"
	_, err := sourceWriter.Write([]byte(firstFrame))
REDACTED

	select {
	case result := <-read:
		require.NoError(t, result.err)
		require.Contains(t, result.frame, "flush-before-eof")
		require.Contains(t, result.frame, "event: response.created")
	case <-time.After(3 * time.Second):
		t.Fatal("first transformed SSE frame was not flushed while the upstream connection remained open")
REDACTED
REDACTED

type grokProtocolSSEFrame struct {
	event string
	data  []byte
REDACTED

func grokClientToolProtocolRequest(stream bool) []byte {
	return []byte(fmt.Sprintf(`{
		"model":"grok","stream":%t,
		"tools":[
			{"type":"custom","name":"apply_patch","description":"apply a patch","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"REDACTEDREDACTED,
			{"type":"tool_search"REDACTED,
			{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"send_message","description":"send a message","parameters":{"type":"object","properties":{"target":{"type":"string"REDACTEDREDACTEDREDACTEDREDACTED]REDACTED
		],
		"tool_choice":{"type":"custom","name":"apply_patch"REDACTED,
		"input":[
			{"type":"custom_tool_call","id":"old_custom","call_id":"old_custom_call","name":"apply_patch","input":"*** Begin Patch"REDACTED,
			{"type":"custom_tool_call_output","call_id":"old_custom_call","output":"Done!"REDACTED,
			{"type":"tool_search_call","id":"old_search","call_id":"old_search_call","arguments":{"query":"github"REDACTED,"execution":"client"REDACTED,
			{"type":"tool_search_output","call_id":"old_search_call","output":{"groups":["github"]REDACTEDREDACTED,
			{"type":"function_call","id":"old_namespace","call_id":"old_namespace_call","namespace":"collaboration","name":"send_message","arguments":"{\"target\":\"root\"REDACTED"REDACTED,
			{"type":"function_call_output","call_id":"old_namespace_call","output":"ok"REDACTED,
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"REDACTED]REDACTED
		]
REDACTED`, stream))
REDACTED

func grokProtocolOAuthAccount(id int64) *Account {
REDACTED
		ID: id, Name: "grok-oauth-protocol", Platform: PlatformGrok, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
REDACTED
			"access_token": "oauth-protocol-token", "refresh_token": "refresh-token",
			"expires_at": time.Now().Add(2 * grokTokenRefreshSkew).UTC().Format(time.RFC3339),
			"base_url":   xai.DefaultCLIBaseURL, "subscription_tier": "supergrok",
	REDACTED,
REDACTED
REDACTED

func grokProtocolAPIKeyAccount(id int64) *Account {
REDACTED
		ID: id, Name: "grok-api-key-protocol", Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
REDACTED"api_key": "xai-protocol-key", "base_url": "https://api.x.ai/v1"REDACTED,
REDACTED
REDACTED

func assertGrokProtocolRequestLowered(t *testing.T, body []byte) {
REDACTED
	require.True(t, json.Valid(body))
	require.False(t, gjson.GetBytes(body, `tools.#(type=="custom")`).Exists())
	require.False(t, gjson.GetBytes(body, `tools.#(type=="namespace")`).Exists())
	require.False(t, gjson.GetBytes(body, `tools.#(type=="tool_search")`).Exists())
	require.True(t, gjson.GetBytes(body, `tools.#(name=="apply_patch")`).Exists())
	require.True(t, gjson.GetBytes(body, `tools.#(name=="tool_search")`).Exists())
	require.True(t, gjson.GetBytes(body, `tools.#(name=="collaboration__send_message")`).Exists())
	require.Equal(t, "function", gjson.GetBytes(body, "tool_choice.type").String())
	require.Equal(t, "apply_patch", gjson.GetBytes(body, "tool_choice.name").String())
	require.Equal(t, "function_call", gjson.GetBytes(body, "input.0.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(body, "input.1.type").String())
	require.Equal(t, "function_call", gjson.GetBytes(body, "input.2.type").String())
	require.Equal(t, "tool_search", gjson.GetBytes(body, "input.2.name").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(body, "input.3.type").String())
	require.Equal(t, "collaboration__send_message", gjson.GetBytes(body, "input.4.name").String())
	require.False(t, gjson.GetBytes(body, "input.4.namespace").Exists())
REDACTED

func grokProtocolUpstreamSSE() string {
	events := []string{
		`{"type":"response.created","sequence_number":40,"response":{"id":"resp_protocol_stream","model":"grok-4.5"REDACTED,"upstream_extension":{"preserved":trueREDACTEDREDACTED`,
		`{"type":"response.output_item.added","sequence_number":41,"output_index":0,"item":{"type":"function_call","id":"item_custom","call_id":"call_custom","name":"apply_patch","arguments":"","status":"in_progress"REDACTEDREDACTED`,
		`{"type":"response.function_call_arguments.delta","sequence_number":42,"output_index":0,"item_id":"item_custom","delta":"{\"input\":\"*** Begin"REDACTED`,
		`{"type":"response.function_call_arguments.delta","sequence_number":43,"output_index":0,"item_id":"item_custom","delta":" Patch\"REDACTED"REDACTED`,
		`{"type":"response.function_call_arguments.done","sequence_number":44,"output_index":0,"item_id":"item_custom","call_id":"call_custom","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"REDACTED"REDACTED`,
		`{"type":"response.output_item.done","sequence_number":45,"output_index":0,"item":{"type":"function_call","id":"item_custom","call_id":"call_custom","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"REDACTED","status":"completed"REDACTEDREDACTED`,
		`{"type":"response.output_item.added","sequence_number":46,"output_index":1,"item":{"type":"function_call","id":"item_namespace","call_id":"call_namespace","name":"collaboration__send_message","arguments":"","status":"in_progress"REDACTEDREDACTED`,
		`{"type":"response.function_call_arguments.done","sequence_number":47,"output_index":1,"item_id":"item_namespace","call_id":"call_namespace","name":"collaboration__send_message","arguments":"{\"target\":\"root\"REDACTED"REDACTED`,
		`{"type":"response.output_item.done","sequence_number":48,"output_index":1,"item":{"type":"function_call","id":"item_namespace","call_id":"call_namespace","name":"collaboration__send_message","arguments":"{\"target\":\"root\"REDACTED","status":"completed"REDACTEDREDACTED`,
		`{"type":"response.output_item.added","sequence_number":49,"output_index":2,"item":{"type":"function_call","id":"item_search","call_id":"call_search","name":"tool_search","arguments":"","status":"in_progress"REDACTEDREDACTED`,
		`{"type":"response.function_call_arguments.delta","sequence_number":50,"output_index":2,"item_id":"item_search","delta":"{\"query\":\"github\"REDACTED"REDACTED`,
		`{"type":"response.function_call_arguments.done","sequence_number":51,"output_index":2,"item_id":"item_search","call_id":"call_search","name":"tool_search","arguments":"{\"query\":\"github\"REDACTED"REDACTED`,
		`{"type":"response.output_item.done","sequence_number":52,"output_index":2,"item":{"type":"function_call","id":"item_search","call_id":"call_search","name":"tool_search","arguments":"{\"query\":\"github\"REDACTED","status":"completed"REDACTEDREDACTED`,
		`{"type":"response.completed","sequence_number":53,"response":{"id":"resp_protocol_stream","object":"response","model":"grok-4.5","status":"completed","output":[{"type":"function_call","id":"item_custom","call_id":"call_custom","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"REDACTED"REDACTED,{"type":"function_call","id":"item_search","call_id":"call_search","name":"tool_search","arguments":"{\"query\":\"github\"REDACTED"REDACTED,{"type":"function_call","id":"item_namespace","call_id":"call_namespace","name":"collaboration__send_message","arguments":"{\"target\":\"root\"REDACTED"REDACTED],"usage":{"input_tokens":11,"output_tokens":4,"total_tokens":15REDACTEDREDACTEDREDACTED`,
REDACTED
	var out strings.Builder
	for _, event := range events {
		typ := gjson.Get(event, "type").String()
		fmt.Fprintf(&out, "event: %s\ndata: %s\n\n", typ, event)
REDACTED
	return out.String()
REDACTED

func parseGrokProtocolSSEFrames(t *testing.T, body string) []grokProtocolSSEFrame {
REDACTED
	var frames []grokProtocolSSEFrame
	event := ""
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		if value, ok := extractOpenAISSEEventLine(line); ok {
			event = strings.TrimSpace(value)
			continue
	REDACTED
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || strings.TrimSpace(data) == "[DONE]" {
			continue
	REDACTED
		require.NotEmpty(t, event, "every data frame from this upstream should retain an event field")
		require.JSONEq(t, data, data)
		frames = append(frames, grokProtocolSSEFrame{event: event, data: []byte(data)REDACTED)
		event = ""
REDACTED
	return frames
REDACTED

func requireGrokProtocolFrame(t *testing.T, frames []grokProtocolSSEFrame, eventType, path, value string) grokProtocolSSEFrame {
REDACTED
	for _, frame := range frames {
		if frame.event != eventType {
			continue
	REDACTED
		if path == "" || gjson.GetBytes(frame.data, path).String() == value {
			return frame
	REDACTED
REDACTED
	t.Fatalf("missing SSE frame event=%q %s=%q", eventType, path, value)
	return grokProtocolSSEFrame{REDACTED
REDACTED
