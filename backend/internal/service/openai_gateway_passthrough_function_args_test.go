package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestHandleStreamingResponsePassthroughDeduplicatesFunctionCallArguments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	argsA := `{"cmd":"echo hi","meta":{"nested":[1,{"ok":trueREDACTED],"quote":"aREDACTEDb"REDACTEDREDACTED`
	argsB := `{"path":"/tmp/file","patch":{"ops":[{"op":"replace","value":{"lines":["x","y"]REDACTEDREDACTED]REDACTEDREDACTED`
	upstreamBody := strings.Join([]string{
		passthroughSSEData(`{"type":"response.created","response":{"id":"resp_passthrough_args","model":"gpt-5.4"REDACTEDREDACTED`),
		passthroughSSEData(`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_a","call_id":"call_a","name":"exec_command","arguments":"","status":"in_progress"REDACTEDREDACTED`),
		passthroughSSEData(functionArgsDeltaJSON(0, "fc_a", "call_a", "exec_command", `{"cmd":`)),
		passthroughSSEData(functionArgsDeltaJSON(0, "fc_a", "call_a", "exec_command", `"echo hi","meta":{"nested":[1,{"ok":trueREDACTED],"quote":"aREDACTEDb"REDACTEDREDACTED`)),
		passthroughSSEData(functionArgsDoneJSON(0, "fc_a", "call_a", "exec_command", argsA+argsA)),
		passthroughSSEData(outputItemDoneJSON(0, "fc_a", "call_a", "exec_command", argsA+argsA)),
		passthroughSSEData(`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_b","call_id":"call_b","name":"apply_patch","arguments":"","status":"in_progress"REDACTEDREDACTED`),
		passthroughSSEData(functionArgsDeltaJSON(1, "fc_b", "call_b", "apply_patch", `{"path":"/tmp/file",`)),
		passthroughSSEData(functionArgsDeltaJSON(1, "fc_b", "call_b", "apply_patch", `"patch":{"ops":[{"op":"replace","value":{"lines":["x","y"]REDACTEDREDACTED]REDACTEDREDACTED`)),
		passthroughSSEData(functionArgsDoneJSON(1, "fc_b", "call_b", "apply_patch", argsB+argsB)),
		passthroughSSEData(outputItemDoneJSON(1, "fc_b", "call_b", "apply_patch", argsB+argsB)),
		passthroughSSEData(completedWithFunctionCallsJSON(argsA+argsA, argsB+argsB)),
		"data: [DONE]\n\n",
REDACTED, "")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTED

	svc := &OpenAIGatewayService{REDACTED
	result, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "gpt-5.4", "gpt-5.4")
REDACTED
	require.NotNil(t, result)

	events := collectSSEDataPayloads(t, rec.Body.String())
	require.Equal(t, argsA, accumulateFunctionArgumentDeltas(events, "call_a"))
	require.Equal(t, argsB, accumulateFunctionArgumentDeltas(events, "call_b"))

	require.Equal(t, argsA, gjson.Get(findSSEEvent(t, events, "response.function_call_arguments.done", "call_a"), "arguments").String())
	require.Equal(t, argsB, gjson.Get(findSSEEvent(t, events, "response.function_call_arguments.done", "call_b"), "arguments").String())
	require.Equal(t, argsA, gjson.Get(findSSEEvent(t, events, "response.output_item.done", "call_a"), "item.arguments").String())
	require.Equal(t, argsB, gjson.Get(findSSEEvent(t, events, "response.output_item.done", "call_b"), "item.arguments").String())

	completed := findSSEEvent(t, events, "response.completed", "")
	require.Equal(t, argsA, gjson.Get(completed, "response.output.0.arguments").String())
	require.Equal(t, argsB, gjson.Get(completed, "response.output.1.arguments").String())
	requireJSONArgument(t, gjson.Get(completed, "response.output.0.arguments").String())
	requireJSONArgument(t, gjson.Get(completed, "response.output.1.arguments").String())
REDACTED

func TestForwardResponsesChatCompletionsFallbackKeepsFunctionArgumentsSingle(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"run a command","stream":trueREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		passthroughSSEData(chatToolCallChunkJSON(true, "")),
		"",
		passthroughSSEData(chatToolCallChunkJSON(false, `{"cmd":"echo hi"REDACTED`)),
		"",
		`data: {"id":"chatcmpl_tool","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{REDACTED,"finish_reason":"tool_calls"REDACTED],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5REDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_fallback_tool_args"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED
	account := passthroughArgsFallbackAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
REDACTED
	svc := &OpenAIGatewayService{
		cfg:          passthroughArgsTestConfig(),
		httpUpstream: upstream,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)

	const wantArgs = `{"cmd":"echo hi"REDACTED`
	events := collectSSEDataPayloads(t, rec.Body.String())
	require.Equal(t, wantArgs, accumulateFunctionArgumentDeltas(events, "chatcmpl-tool-a"))
	require.Equal(t, wantArgs, gjson.Get(findSSEEvent(t, events, "response.function_call_arguments.done", "chatcmpl-tool-a"), "arguments").String())
	require.Equal(t, wantArgs, gjson.Get(findSSEEvent(t, events, "response.output_item.done", "chatcmpl-tool-a"), "item.arguments").String())
REDACTED

func passthroughSSEData(payload string) string {
	return "data: " + payload + "\n\n"
REDACTED

func functionArgsDeltaJSON(outputIndex int, itemID, callID, name, delta string) string {
	return fmt.Sprintf(
		`{"type":"response.function_call_arguments.delta","output_index":%d,"item_id":%s,"call_id":%s,"name":%s,"delta":%sREDACTED`,
		outputIndex,
		strconv.Quote(itemID),
		strconv.Quote(callID),
		strconv.Quote(name),
		strconv.Quote(delta),
	)
REDACTED

func functionArgsDoneJSON(outputIndex int, itemID, callID, name, arguments string) string {
	return fmt.Sprintf(
		`{"type":"response.function_call_arguments.done","output_index":%d,"item_id":%s,"call_id":%s,"name":%s,"arguments":%sREDACTED`,
		outputIndex,
		strconv.Quote(itemID),
		strconv.Quote(callID),
		strconv.Quote(name),
		strconv.Quote(arguments),
	)
REDACTED

func outputItemDoneJSON(outputIndex int, itemID, callID, name, arguments string) string {
	return fmt.Sprintf(
		`{"type":"response.output_item.done","output_index":%d,"item":{"type":"function_call","id":%s,"call_id":%s,"name":%s,"arguments":%s,"status":"completed"REDACTEDREDACTED`,
		outputIndex,
		strconv.Quote(itemID),
		strconv.Quote(callID),
		strconv.Quote(name),
		strconv.Quote(arguments),
	)
REDACTED

func completedWithFunctionCallsJSON(argsA, argsB string) string {
	return fmt.Sprintf(
		`{"type":"response.completed","response":{"id":"resp_passthrough_args","status":"completed","output":[{"type":"function_call","id":"fc_a","call_id":"call_a","name":"exec_command","arguments":%s,"status":"completed"REDACTED,{"type":"function_call","id":"fc_b","call_id":"call_b","name":"apply_patch","arguments":%s,"status":"completed"REDACTED],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5REDACTEDREDACTEDREDACTED`,
		strconv.Quote(argsA),
		strconv.Quote(argsB),
	)
REDACTED

func chatToolCallChunkJSON(includeIdentity bool, arguments string) string {
	identity := ""
	functionFields := make([]string, 0, 2)
	if includeIdentity {
		identity = `"id":"chatcmpl-tool-a","type":"function",`
		functionFields = append(functionFields, `"name":"exec_command"`)
REDACTED
	if includeIdentity || arguments != "" {
		functionFields = append(functionFields, `"arguments":`+strconv.Quote(arguments))
REDACTED
	return fmt.Sprintf(
		`{"id":"chatcmpl_tool","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,%s"function":{%sREDACTEDREDACTED]REDACTED,"finish_reason":nullREDACTED]REDACTED`,
		identity,
		strings.Join(functionFields, ","),
	)
REDACTED

func passthroughArgsTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
		REDACTED,
	REDACTED,
REDACTED
REDACTED

func passthroughArgsFallbackAccount() *Account {
REDACTED
		ID:          102,
		Name:        "passthrough-args-openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
	REDACTED,
REDACTED
REDACTED

func collectSSEDataPayloads(t *testing.T, body string) []string {
REDACTED
	scanner := bufio.NewScanner(strings.NewReader(body))
	var events []string
	for scanner.Scan() {
		data, ok := extractOpenAISSEDataLine(scanner.Text())
		if !ok {
			continue
	REDACTED
		if strings.TrimSpace(data) == "[DONE]" {
			continue
	REDACTED
		require.True(t, gjson.Valid(data), "invalid SSE data payload: %s", data)
		events = append(events, data)
REDACTED
	require.NoError(t, scanner.Err())
	return events
REDACTED

func findSSEEvent(t *testing.T, events []string, eventType, callID string) string {
REDACTED
	for _, event := range events {
		if gjson.Get(event, "type").String() != eventType {
			continue
	REDACTED
		if callID == "" ||
			gjson.Get(event, "call_id").String() == callID ||
			gjson.Get(event, "item.call_id").String() == callID {
			return event
	REDACTED
REDACTED
	t.Fatalf("missing event type=%s call_id=%s in %d events", eventType, callID, len(events))
	return ""
REDACTED

func accumulateFunctionArgumentDeltas(events []string, callID string) string {
	var b strings.Builder
	for _, event := range events {
		if gjson.Get(event, "type").String() != "response.function_call_arguments.delta" {
			continue
	REDACTED
		if gjson.Get(event, "call_id").String() != callID {
			continue
	REDACTED
		_, _ = b.WriteString(gjson.Get(event, "delta").String())
REDACTED
	return b.String()
REDACTED

func requireJSONArgument(t *testing.T, arguments string) {
REDACTED
	var decoded any
	require.NoError(t, json.Unmarshal([]byte(arguments), &decoded))
REDACTED
