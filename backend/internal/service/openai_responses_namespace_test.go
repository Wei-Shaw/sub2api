package service

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestShouldFlattenOpenAIResponsesNamespaces(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	grokOAuth := &Account{Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED

	tests := []struct {
		name               string
		account            *Account
		transport          OpenAIUpstreamTransport
		passthroughEnabled bool
		want               bool
REDACTED{
		{name: "oauth_http", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, want: trueREDACTED,
		{name: "oauth_http_passthrough", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, passthroughEnabled: true, want: trueREDACTED,
		// WSv2 出口原样转发上游事件、不做回程还原，摊平会让客户端收到无法匹配的平名。
		{name: "oauth_wsv2", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: falseREDACTED,
		// 透传账号先于 WSv2 分支经 HTTP 转发返回，仍需摊平。
		{name: "oauth_wsv2_passthrough", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, passthroughEnabled: true, want: trueREDACTED,
		{name: "apikey_http", account: apiKey, transport: OpenAIUpstreamTransportHTTPSSE, want: falseREDACTED,
		{name: "grok_oauth_http", account: grokOAuth, transport: OpenAIUpstreamTransportHTTPSSE, want: falseREDACTED,
		{name: "nil_account", account: nil, transport: OpenAIUpstreamTransportHTTPSSE, want: falseREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldFlattenOpenAIResponsesNamespaces(tt.account, tt.transport, tt.passthroughEnabled))
	REDACTED)
REDACTED
REDACTED

func TestStripOpenAIResponsesInputNamespaces(t *testing.T) {
	body := []byte(`{
		"meta":9007199254740993,
		"scientific":1.25e+42,
		"escaped":"line\\n\\u003ctag\\u003e",
		"tools":[{"type":"function","name":"keep","namespace":"tool-namespace"REDACTED],
		"input":[
			{"type":"function_call","namespace":"n0","name":"one","content":{"namespace":"nested"REDACTED,"large":9007199254740993REDACTED,
			{"type":"message","namespace":"n1","content":[{"type":"input_text","text":"hello","namespace":"nested-content"REDACTED]REDACTED,
			{"type":"custom_tool_call","namespace":"n2","input":"{REDACTED"REDACTED,
			{"type":"function_call_output","namespace":"n3","output":"ok"REDACTED,
			{"type":"item","namespace":"n4"REDACTED,
			{"type":"item","namespace":"n5"REDACTED,
			{"type":"item","namespace":"n6"REDACTED,
			{"type":"item","namespace":"n7"REDACTED
		]
REDACTED`)

	stripped, err := stripOpenAIResponsesInputNamespaces(body)
REDACTED
	for index := 0; index < 8; index++ {
		require.False(t, gjson.GetBytes(stripped, "input."+strconv.Itoa(index)+".namespace").Exists())
REDACTED
	require.Equal(t, "nested", gjson.GetBytes(stripped, "input.0.content.namespace").String())
	require.Equal(t, "nested-content", gjson.GetBytes(stripped, "input.1.content.0.namespace").String())
	require.Equal(t, "tool-namespace", gjson.GetBytes(stripped, "tools.0.namespace").String())
	require.Equal(t, gjson.GetBytes(body, "meta").Raw, gjson.GetBytes(stripped, "meta").Raw)
	require.Equal(t, gjson.GetBytes(body, "scientific").Raw, gjson.GetBytes(stripped, "scientific").Raw)
	require.Equal(t, gjson.GetBytes(body, "escaped").Raw, gjson.GetBytes(stripped, "escaped").Raw)
	require.Equal(t, gjson.GetBytes(body, "input.0.large").Raw, gjson.GetBytes(stripped, "input.0.large").Raw)
REDACTED

func TestStripOpenAIResponsesInputNamespacesLeavesOtherShapesByteExact(t *testing.T) {
	tests := [][]byte{
		[]byte(`{"input":"text","namespace":"top-level"REDACTED`),
		[]byte(`{"input":{"namespace":"single-object"REDACTEDREDACTED`),
		[]byte(`{"input":[{"content":{"namespace":"nested-only"REDACTEDREDACTED],"tools":[{"namespace":"keep"REDACTED]REDACTED`),
REDACTED
	for _, body := range tests {
		stripped, err := stripOpenAIResponsesInputNamespaces(body)
	REDACTED
		require.Equal(t, body, stripped)
REDACTED
REDACTED
