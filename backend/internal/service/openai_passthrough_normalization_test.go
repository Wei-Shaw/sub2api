package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIPassthroughOAuthBody_RemovesUnsupportedUser(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello","user":"user_123","metadata":{"user_id":"user_123"REDACTED,"prompt_cache_retention":"24h","safety_identifier":"sid","stream_options":{"include_usage":trueREDACTEDREDACTED`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
REDACTED
	require.True(t, changed)
	for _, field := range openAIChatGPTInternalUnsupportedFields {
		require.False(t, gjson.GetBytes(normalized, field).Exists(), "%s should be stripped", field)
REDACTED
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
	require.False(t, gjson.GetBytes(normalized, "store").Bool())
REDACTED

func TestNormalizeOpenAIPassthroughOAuthBody_NormalizesCompatibilityFields(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","prompt":"hello","commands":["unsupported"],"truncation":"auto","stop_sequences":["END"],"chat_template_kwargs":{"enable_thinking":trueREDACTEDREDACTED`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
REDACTED
	require.True(t, changed)
	require.Equal(t, "hello", gjson.GetBytes(normalized, "input.0.content").String())
	for _, field := range []string{"prompt", "commands", "truncation", "stop_sequences", "chat_template_kwargs"REDACTED {
		require.False(t, gjson.GetBytes(normalized, field).Exists(), field)
REDACTED
REDACTED

func TestNormalizeOpenAIPassthroughOAuthBody_NormalizesReasoningMode(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":"hello","reasoning":{"mode":"pro"REDACTEDREDACTED`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
REDACTED
	require.True(t, changed)
	require.Equal(t, "max", gjson.GetBytes(normalized, "reasoning.effort").String())
	require.False(t, gjson.GetBytes(normalized, "reasoning.mode").Exists())
REDACTED

func TestNormalizeOpenAIOAuthResponsesCompatibilityBody_PreservesExplicitInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":"explicit","prompt":"legacy"REDACTED`)

	normalized, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(body)
REDACTED
	require.True(t, changed)
	require.Equal(t, "explicit", gjson.GetBytes(normalized, "input").String())
	require.False(t, gjson.GetBytes(normalized, "prompt").Exists())
REDACTED

func TestNormalizeOpenAIResponsesWebSocketCompatibilityBody_OnlyStripsOAuthFields(t *testing.T) {
	body := []byte(`{"type":"response.create","prompt":"hello","commands":{REDACTED,"truncation":"auto","stop_sequences":["END"],"chat_template_kwargs":{"enable_thinking":trueREDACTEDREDACTED`)

	oauthBody, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED)
REDACTED
	require.True(t, changed)
	require.Equal(t, "hello", gjson.GetBytes(oauthBody, "input").String())
	for _, field := range []string{"prompt", "commands", "truncation", "stop_sequences", "chat_template_kwargs"REDACTED {
		require.False(t, gjson.GetBytes(oauthBody, field).Exists(), field)
REDACTED

	apiKeyBody, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED)
REDACTED
	require.False(t, changed)
	require.JSONEq(t, string(body), string(apiKeyBody))
REDACTED

func TestNormalizeOpenAIResponsesWebSocketCompatibilityBody_SanitizesNativeItemIDs(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-5.6-sol","input":[` +
		`{"type":"custom_tool_call","id":"fc_wrong_custom","call_id":"call_custom_1","name":"apply_patch","input":"patch"REDACTED,` +
		`{"type":"custom_tool_call","id":"ctc_valid","call_id":"call_custom_2","name":"apply_patch","input":"patch"REDACTED,` +
		`{"type":"tool_search_call","id":"fc_wrong_search","call_id":"call_search_1","arguments":{"query":"docs"REDACTEDREDACTED,` +
		`{"type":"tool_search_call","id":"tsc_valid","call_id":"call_search_2","arguments":{"query":"docs"REDACTEDREDACTED]REDACTED`)

	for _, oauth := range []bool{false, trueREDACTED {
		accountType := AccountTypeAPIKey
		if oauth {
			accountType = AccountTypeOAuth
	REDACTED
		normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{Platform: PlatformOpenAI, Type: accountTypeREDACTED)
	REDACTED
		require.True(t, changed)
		require.Equal(t, "response.create", gjson.GetBytes(normalized, "type").String())
		require.False(t, gjson.GetBytes(normalized, "input.0.id").Exists())
		require.Equal(t, "ctc_valid", gjson.GetBytes(normalized, "input.1.id").String())
		require.False(t, gjson.GetBytes(normalized, "input.2.id").Exists())
		require.Equal(t, "tsc_valid", gjson.GetBytes(normalized, "input.3.id").String())
		// Native Responses call_id values are correlation keys, not item IDs.
		require.Equal(t, "call_custom_1", gjson.GetBytes(normalized, "input.0.call_id").String())
		require.Equal(t, "call_search_1", gjson.GetBytes(normalized, "input.2.call_id").String())
REDACTED
REDACTED

func TestNormalizeOpenAIResponsesWebSocketCompatibilityBody_APIKeyStoreFalseReplay(t *testing.T) {
	body := []byte(`{"type":"response.create","store":false,"parallel_tool_calls":true,"input":[` +
		`{"type":"reasoning","id":"rs_drop","summary":[]REDACTED,` +
		`{"type":"reasoning","id":"rs_keep","call_id":"remove","encrypted_content":"cipher"REDACTED,` +
		`{"type":"message","content":"continue"REDACTED` +
		`]REDACTED`)

	normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED)

REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "parallel_tool_calls").Exists())
	require.Equal(t, int64(2), gjson.GetBytes(normalized, "input.#").Int())
	require.False(t, gjson.GetBytes(normalized, "input.0.id").Exists())
	require.False(t, gjson.GetBytes(normalized, "input.0.call_id").Exists())
	require.True(t, gjson.GetBytes(normalized, "input.0.summary").IsArray())
	require.Equal(t, "message", gjson.GetBytes(normalized, "input.1.type").String())
REDACTED

func TestNormalizeOpenAIResponsesReasoningMode(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantEffort string
REDACTED{
		{name: "pro maps to max", body: `{"reasoning":{"mode":"pro"REDACTEDREDACTED`, wantEffort: "max"REDACTED,
		{name: "explicit effort wins", body: `{"reasoning":{"mode":"pro","effort":"high"REDACTEDREDACTED`, wantEffort: "high"REDACTED,
		{name: "other mode only removed", body: `{"reasoning":{"mode":"standard"REDACTEDREDACTED`REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, changed, err := normalizeOpenAIResponsesReasoningMode([]byte(tt.body))
		REDACTED
			require.True(t, changed)
			require.False(t, gjson.GetBytes(normalized, "reasoning.mode").Exists())
			require.Equal(t, tt.wantEffort, gjson.GetBytes(normalized, "reasoning.effort").String())
	REDACTED)
REDACTED
REDACTED

func TestNormalizeOpenAIResponsesWebSocketCompatibilityBody_ReasoningModeAccountScope(t *testing.T) {
	body := []byte(`{"type":"response.create","reasoning":{"mode":"pro"REDACTEDREDACTED`)
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupTokenREDACTED {
		normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{Platform: PlatformOpenAI, Type: accountTypeREDACTED)
	REDACTED
		require.True(t, changed)
		require.Equal(t, "max", gjson.GetBytes(normalized, "reasoning.effort").String())
		require.False(t, gjson.GetBytes(normalized, "reasoning.mode").Exists())
REDACTED
	apiKeyBody, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED)
REDACTED
	require.False(t, changed)
	require.JSONEq(t, string(body), string(apiKeyBody))
REDACTED

func TestNormalizeOpenAIResponsesWebSocketCompatibilityBody_SanitizesToolSchemas(t *testing.T) {
	body := []byte(`{"type":"response.create","tools":[{"type":"function","name":"search","parameters":{"type":null,"properties":{"q":{"type":"string","pattern":"^(?=.*foo).+$"REDACTEDREDACTEDREDACTEDREDACTED]REDACTED`)
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuthREDACTED {
		normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{Platform: PlatformOpenAI, Type: accountTypeREDACTED)
	REDACTED
		require.True(t, changed)
		require.Equal(t, "object", gjson.GetBytes(normalized, "tools.0.parameters.type").String())
		require.False(t, gjson.GetBytes(normalized, "tools.0.parameters.properties.q.pattern").Exists())
REDACTED
REDACTED

func TestNormalizeOpenAIResponseFormatSchemasBody_PreservesNonStrictOptionalFields(t *testing.T) {
	body := []byte(`{"text":{"format":{"type":"json_schema","strict":false,"schema":{"properties":{"tags":{"items":{"type":"string"REDACTED,"uniqueItems":trueREDACTEDREDACTED,"minProperties":1,"maxProperties":4REDACTEDREDACTEDREDACTEDREDACTED`)

	normalized, changed, err := normalizeOpenAIResponseFormatSchemasBody(body)
REDACTED
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(normalized, "text.format.schema.type").String())
	require.Equal(t, "array", gjson.GetBytes(normalized, "text.format.schema.properties.tags.type").String())
	require.False(t, gjson.GetBytes(normalized, "text.format.schema.minProperties").Exists())
	require.False(t, gjson.GetBytes(normalized, "text.format.schema.properties.tags.uniqueItems").Exists())
	require.Equal(t, int64(4), gjson.GetBytes(normalized, "text.format.schema.maxProperties").Int())
	require.False(t, gjson.GetBytes(normalized, "text.format.schema.required").Exists())
REDACTED

func TestNormalizeOpenAIResponseFormatSchemasBody_DoesNotExpandStrictSchema(t *testing.T) {
	body := []byte(`{"response_format":{"type":"json_schema","json_schema":{"strict":true,"schema":{"properties":{"name":{"type":"string"REDACTEDREDACTEDREDACTEDREDACTEDREDACTEDREDACTED`)

	normalized, changed, err := normalizeOpenAIResponseFormatSchemasBody(body)
REDACTED
	require.True(t, changed) // Safe type inference still applies.
	require.Equal(t, "object", gjson.GetBytes(normalized, "response_format.json_schema.schema.type").String())
	require.False(t, gjson.GetBytes(normalized, "response_format.json_schema.schema.required").Exists())
	require.False(t, gjson.GetBytes(normalized, "response_format.json_schema.schema.additionalProperties").Exists())
REDACTED

func TestNormalizeOpenAIResponseFormatSchemasBody_TraversesNestedSchemaContainers(t *testing.T) {
	body := []byte(`{
		"text":{"format":{"type":"json_schema","schema":{
			"$defs":{"entry":{"properties":{"name":{"type":"string"REDACTEDREDACTED,"minProperties":1,"maxProperties":3REDACTEDREDACTED,
			"additionalProperties":{"items":{"type":"string"REDACTED,"uniqueItems":trueREDACTED,
			"prefixItems":[{"properties":{"id":{"type":"string"REDACTEDREDACTED,"minProperties":1REDACTED],
			"dependentSchemas":{"kind":{"properties":{"value":{"type":"string"REDACTEDREDACTED,"uniqueItems":trueREDACTEDREDACTED,
			"not":{"items":{"type":"string"REDACTED,"minProperties":1REDACTED
	REDACTEDREDACTEDREDACTED
REDACTED`)

	normalized, changed, err := normalizeOpenAIResponseFormatSchemasBody(body)
REDACTED
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(normalized, "text.format.schema.$defs.entry.type").String())
	require.False(t, gjson.GetBytes(normalized, "text.format.schema.$defs.entry.minProperties").Exists())
	require.Equal(t, int64(3), gjson.GetBytes(normalized, "text.format.schema.$defs.entry.maxProperties").Int())
	require.Equal(t, "array", gjson.GetBytes(normalized, "text.format.schema.additionalProperties.type").String())
	require.False(t, gjson.GetBytes(normalized, "text.format.schema.additionalProperties.uniqueItems").Exists())
	require.Equal(t, "object", gjson.GetBytes(normalized, "text.format.schema.prefixItems.0.type").String())
	require.False(t, gjson.GetBytes(normalized, "text.format.schema.prefixItems.0.minProperties").Exists())
	require.Equal(t, "object", gjson.GetBytes(normalized, "text.format.schema.dependentSchemas.kind.type").String())
	require.False(t, gjson.GetBytes(normalized, "text.format.schema.dependentSchemas.kind.uniqueItems").Exists())
	require.Equal(t, "array", gjson.GetBytes(normalized, "text.format.schema.not.type").String())
	require.False(t, gjson.GetBytes(normalized, "text.format.schema.not.minProperties").Exists())
	require.False(t, gjson.GetBytes(normalized, "text.format.schema.required").Exists())
REDACTED

func TestNormalizeOpenAIResponseFormatSchemasBody_PreservesExistingTypeValues(t *testing.T) {
	body := []byte(`{"text":{"format":{"type":"json_schema","schema":{"properties":{"union":{"type":["object","null"],"properties":{"name":{"type":"string"REDACTEDREDACTEDREDACTED,"custom":{"type":{"vendor":"shape"REDACTED,"properties":{"id":{"type":"string"REDACTEDREDACTEDREDACTED,"inferred":{"type":null,"items":{"type":"string"REDACTEDREDACTEDREDACTEDREDACTEDREDACTEDREDACTEDREDACTED`)

	normalized, changed, err := normalizeOpenAIResponseFormatSchemasBody(body)
REDACTED
	require.True(t, changed)
	require.Equal(t, "object", gjson.GetBytes(normalized, "text.format.schema.type").String())
	require.Equal(t, "object", gjson.GetBytes(normalized, "text.format.schema.properties.union.type.0").String())
	require.Equal(t, "null", gjson.GetBytes(normalized, "text.format.schema.properties.union.type.1").String())
	require.True(t, gjson.GetBytes(normalized, "text.format.schema.properties.custom.type").IsObject())
	require.Equal(t, "array", gjson.GetBytes(normalized, "text.format.schema.properties.inferred.type").String())
REDACTED

func TestNormalizeOpenAIPassthroughOAuthBody_CompactRemovesUnsupportedUser(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello","user":"user_123","metadata":{"user_id":"user_123"REDACTED,"stream":true,"store":trueREDACTED`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, true)
REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "user").Exists())
	require.False(t, gjson.GetBytes(normalized, "metadata").Exists())
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
REDACTED

func TestNormalizeOpenAIPassthroughOAuthBody_StringInputWrappedAsArray(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello world"REDACTED`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
REDACTED
	require.True(t, changed)

	input := gjson.GetBytes(normalized, "input")
	require.True(t, input.IsArray(), "string input should be converted to array")
	items := input.Array()
	require.Len(t, items, 1)
	require.Equal(t, "message", items[0].Get("type").String())
	require.Equal(t, "user", items[0].Get("role").String())
	require.Equal(t, "hello world", items[0].Get("content").String())
REDACTED

func TestNormalizeOpenAIPassthroughOAuthBody_EmptyStringInputWrappedAsEmptyArray(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"  "REDACTED`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
REDACTED
	require.True(t, changed)

	input := gjson.GetBytes(normalized, "input")
	require.True(t, input.IsArray())
	require.Len(t, input.Array(), 0, "whitespace-only input should become empty array")
REDACTED

func TestNormalizeOpenAIPassthroughOAuthBody_ObjectInputWrappedAsArray(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":{"type":"message","role":"user","content":"hi"REDACTEDREDACTED`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
REDACTED
	require.True(t, changed)

	input := gjson.GetBytes(normalized, "input")
	require.True(t, input.IsArray(), "object input should be wrapped in array")
	items := input.Array()
	require.Len(t, items, 1)
	require.Equal(t, "message", items[0].Get("type").String())
REDACTED

func TestNormalizeOpenAIPassthroughOAuthBody_ArrayInputUnchanged(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":"hi"REDACTED]REDACTED`)

	normalized, _, err := normalizeOpenAIPassthroughOAuthBody(body, false)
REDACTED

	input := gjson.GetBytes(normalized, "input")
	require.True(t, input.IsArray())
	require.Len(t, input.Array(), 1)
	require.Equal(t, "message", input.Array()[0].Get("type").String())
REDACTED

func TestDetectOpenAIPassthroughInstructionsRejectReason(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
REDACTED{
		{name: "missing is optional", body: `{"model":"gpt-5.1-codex"REDACTED`, want: ""REDACTED,
		{name: "non string remains rejected", body: `{"instructions":{"text":"invalid"REDACTEDREDACTED`, want: "instructions_not_string"REDACTED,
		{name: "empty remains rejected", body: `{"instructions":"  "REDACTED`, want: "instructions_empty"REDACTED,
		{name: "non empty remains accepted", body: `{"instructions":"client guidance"REDACTED`, want: ""REDACTED,
REDACTED {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, detectOpenAIPassthroughInstructionsRejectReason("gpt-5.1-codex", []byte(tt.body)))
	REDACTED)
REDACTED
REDACTED
