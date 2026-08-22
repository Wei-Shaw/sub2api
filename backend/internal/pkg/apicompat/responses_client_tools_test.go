package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAdaptResponsesClientTools_LowersDeclarationsHistoryChoiceAndNamespaces(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "custom", "name": "exec", "format": map[string]any{"type": "grammar"REDACTEDREDACTED,
			map[string]any{"type": "tool_search"REDACTED,
			map[string]any{"type": "namespace", "name": "team", "tools": []any{map[string]any{"type": "function", "name": "send"REDACTEDREDACTEDREDACTED,
	REDACTED,
		"tool_choice": map[string]any{"type": "custom", "name": "exec"REDACTED,
		"input": []any{
			map[string]any{"type": "custom_tool_call", "id": "ctc_client", "call_id": "c1", "name": "exec", "input": "dir"REDACTED,
			map[string]any{"type": "custom_tool_call_output", "id": "ctco_client", "call_id": "c1", "output": "ok"REDACTED,
			map[string]any{"type": "tool_search_call", "id": "tsc_client", "call_id": "s1", "arguments": map[string]any{"query": "git"REDACTEDREDACTED,
			map[string]any{"type": "tool_search_output", "id": "tso_client", "call_id": "s1", "output": map[string]any{"groups": []string{"git"REDACTEDREDACTEDREDACTED,
			map[string]any{"type": "function_call", "call_id": "n1", "namespace": "team", "name": "send", "arguments": "{REDACTED"REDACTED,
	REDACTED,
REDACTED

	mapping, changed, err := AdaptResponsesClientTools(req)
REDACTED
	require.True(t, changed)
	require.True(t, mapping.CustomTools["exec"])
	require.True(t, mapping.ToolSearch)
	require.Equal(t, ResponsesNamespaceName{Namespace: "team", Name: "send"REDACTED, mapping.NamespaceTools["team__send"])

	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Len(t, tools, 3)
	exec := requireResponsesClientToolValue[map[string]any](t, tools[0])
	require.Equal(t, "function", exec["type"])
	parameters := requireResponsesClientToolValue[json.RawMessage](t, exec["parameters"])
	require.JSONEq(t, customToolInputSchema, string(parameters))
	search := requireResponsesClientToolValue[map[string]any](t, tools[1])
	require.Equal(t, toolSearchProxyName, search["name"])
	namespaceTool := requireResponsesClientToolValue[map[string]any](t, tools[2])
	require.Equal(t, "team__send", namespaceTool["name"])

	choice := requireResponsesClientToolValue[map[string]any](t, req["tool_choice"])
	require.Equal(t, "function", choice["type"])
	input := requireResponsesClientToolValue[[]any](t, req["input"])
	customCall := requireResponsesClientToolValue[map[string]any](t, input[0])
	require.Equal(t, "function_call", customCall["type"])
	require.NotContains(t, customCall, "id")
	require.JSONEq(t, `{"input":"dir"REDACTED`, requireResponsesClientToolValue[string](t, customCall["arguments"]))
	customOutput := requireResponsesClientToolValue[map[string]any](t, input[1])
	require.Equal(t, "function_call_output", customOutput["type"])
	require.NotContains(t, customOutput, "id")
	searchCall := requireResponsesClientToolValue[map[string]any](t, input[2])
	require.Equal(t, "function_call", searchCall["type"])
	require.NotContains(t, searchCall, "id")
	require.Equal(t, toolSearchProxyName, searchCall["name"])
	require.JSONEq(t, `{"query":"git"REDACTED`, requireResponsesClientToolValue[string](t, searchCall["arguments"]))
	searchOutput := requireResponsesClientToolValue[map[string]any](t, input[3])
	require.Equal(t, "function_call_output", searchOutput["type"])
	require.NotContains(t, searchOutput, "id")
	require.JSONEq(t, `{"groups":["git"]REDACTED`, requireResponsesClientToolValue[string](t, searchOutput["output"]))
	namespaceCall := requireResponsesClientToolValue[map[string]any](t, input[4])
	require.Equal(t, "team__send", namespaceCall["name"])
REDACTED

func TestAdaptResponsesClientTools_RemovesDeferredFlagsWhenToolSearchIsLowered(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "tool_search"REDACTED,
			map[string]any{"type": "function", "name": "shell", "defer_loading": trueREDACTED,
			map[string]any{"type": "function", "name": "apply_patch"REDACTED,
	REDACTED,
REDACTED

	_, changed, err := AdaptResponsesClientTools(req)
REDACTED
	require.True(t, changed)
	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Equal(t, toolSearchProxyName, requireResponsesClientToolValue[map[string]any](t, tools[0])["name"])
	require.NotContains(t, requireResponsesClientToolValue[map[string]any](t, tools[1]), "defer_loading")
REDACTED

func TestStripResponsesDeferredToolFlags_PreservesFlagsWithBuiltInToolSearch(t *testing.T) {
	tools := []any{
		map[string]any{"type": "tool_search"REDACTED,
		map[string]any{"type": "function", "name": "shell", "defer_loading": trueREDACTED,
REDACTED

	require.False(t, stripResponsesDeferredToolFlags(tools))
	require.Equal(t, true, requireResponsesClientToolValue[map[string]any](t, tools[1])["defer_loading"])
REDACTED

func TestAdaptResponsesClientTools_LowersDiscoveredToolSearchOutput(t *testing.T) {
	requestJSON := `{
		"tools":[{"type":"tool_search"REDACTED],
		"input":[
			{"type":"tool_search_call","id":"tsc_client","call_id":"call_search","arguments":{"query":"codex app"REDACTED,"execution":"client","status":"completed"REDACTED,
			{"type":"tool_search_output","id":"tso_client","call_id":"call_search","execution":"client","status":"completed","tools":[
				{"type":"namespace","name":"codex_app","tools":[{"type":"function","name":"load_workspace_dependencies","description":"Load workspace dependencies","parameters":{"type":"object","properties":{REDACTED,"additionalProperties":falseREDACTEDREDACTED]REDACTED,
				{"type":"namespace","name":"multi_agent_v1","tools":[
					{"type":"function","name":"spawn_agent","description":"Spawn an agent","parameters":{"type":"object","properties":{"message":{"type":"string"REDACTEDREDACTED,"required":["message"],"additionalProperties":falseREDACTEDREDACTED,
					{"type":"function","name":"wait_agent","description":"Wait for agents","parameters":{"type":"object","properties":{REDACTED,"additionalProperties":falseREDACTEDREDACTED
				]REDACTED
			]REDACTED
		]
REDACTED`

	type adaptedRequest struct {
		req     map[string]any
		mapping ResponsesClientToolMapping
REDACTED
	adapt := func() adaptedRequest {
		var req map[string]any
		require.NoError(t, json.Unmarshal([]byte(requestJSON), &req))
		mapping, changed, err := AdaptResponsesClientTools(req)
	REDACTED
		require.True(t, changed)
		return adaptedRequest{req: req, mapping: mappingREDACTED
REDACTED

	first := adapt()
	second := adapt()
	firstInput := requireResponsesClientToolValue[[]any](t, first.req["input"])
	secondInput := requireResponsesClientToolValue[[]any](t, second.req["input"])

	tools := requireResponsesClientToolValue[[]any](t, first.req["tools"])
	require.Len(t, tools, 4)
	require.Equal(t, []string{
		"tool_search",
		"codex_app__load_workspace_dependencies",
		"multi_agent_v1__spawn_agent",
		"multi_agent_v1__wait_agent",
REDACTED, responsesClientToolNames(t, tools))
	require.Equal(t, ResponsesNamespaceName{Namespace: "multi_agent_v1", Name: "spawn_agent"REDACTED, first.mapping.NamespaceTools["multi_agent_v1__spawn_agent"])
	require.Equal(t, ResponsesNamespaceName{Namespace: "multi_agent_v1", Name: "wait_agent"REDACTED, first.mapping.NamespaceTools["multi_agent_v1__wait_agent"])

	call := requireResponsesClientToolValue[map[string]any](t, firstInput[0])
	require.Equal(t, "function_call", call["type"])
	require.Equal(t, toolSearchProxyName, call["name"])
	require.JSONEq(t, `{"query":"codex app"REDACTED`, requireResponsesClientToolValue[string](t, call["arguments"]))
	require.NotContains(t, call, "execution")

	output := requireResponsesClientToolValue[map[string]any](t, firstInput[1])
	require.Equal(t, map[string]any{
		"type":    "function_call_output",
		"call_id": "call_search",
		"output":  output["output"],
REDACTED, output)
	outputText := requireResponsesClientToolValue[string](t, output["output"])
	require.JSONEq(t, `[
		{"type":"namespace","name":"codex_app","tools":[{"type":"function","name":"load_workspace_dependencies","description":"Load workspace dependencies","parameters":{"type":"object","properties":{REDACTED,"additionalProperties":falseREDACTEDREDACTED]REDACTED,
		{"type":"namespace","name":"multi_agent_v1","tools":[
			{"type":"function","name":"spawn_agent","description":"Spawn an agent","parameters":{"type":"object","properties":{"message":{"type":"string"REDACTEDREDACTED,"required":["message"],"additionalProperties":falseREDACTEDREDACTED,
			{"type":"function","name":"wait_agent","description":"Wait for agents","parameters":{"type":"object","properties":{REDACTED,"additionalProperties":falseREDACTEDREDACTED
		]REDACTED
	]`, outputText)
	secondOutput := requireResponsesClientToolValue[map[string]any](t, secondInput[1])
	require.Equal(t, outputText, secondOutput["output"], "tool discovery output encoding must be deterministic")

	restored, changed, err := RestoreResponsesClientToolPayload(
		[]byte(`{"output":[{"type":"function_call","name":"multi_agent_v1__spawn_agent","call_id":"call_spawn","arguments":"{\"message\":\"work\"REDACTED"REDACTED]REDACTED`),
		first.mapping,
	)
REDACTED
	require.True(t, changed)
	require.JSONEq(t, `{"output":[{"type":"function_call","name":"spawn_agent","namespace":"multi_agent_v1","call_id":"call_spawn","arguments":"{\"message\":\"work\"REDACTED"REDACTED]REDACTED`, string(restored))
REDACTED

func TestAdaptResponsesClientTools_PromotesDirectDiscoveryAndDeduplicatesIdenticalDeclarations(t *testing.T) {
	direct := map[string]any{
		"type": "function", "name": "inspect_result", "description": "Inspect a result",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{REDACTEDREDACTED,
REDACTED
	custom := map[string]any{
		"type": "custom", "name": "run_script", "description": "Run a script",
		"format": map[string]any{"type": "grammar"REDACTED,
REDACTED
	namespace := map[string]any{
		"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{
			"type": "function", "name": "spawn_agent", "parameters": map[string]any{"type": "object"REDACTED,
REDACTED
REDACTED
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "static_first", "parameters": map[string]any{"type": "object"REDACTEDREDACTED,
			map[string]any{"type": "tool_search"REDACTED,
	REDACTED,
		"input": []any{
			map[string]any{"type": "tool_search_output", "status": "completed", "call_id": "search_1", "tools": []any{direct, custom, namespaceREDACTEDREDACTED,
			map[string]any{"type": "tool_search_output", "status": "completed", "call_id": "search_2", "tools": []any{copyClientTool(direct), copyClientTool(custom), copyClientTool(namespace)REDACTEDREDACTED,
	REDACTED,
REDACTED

	mapping, changed, err := AdaptResponsesClientTools(req)
REDACTED
	require.True(t, changed)
	require.True(t, mapping.CustomTools["run_script"])
	require.Equal(t, ResponsesNamespaceName{Namespace: "multi_agent_v1", Name: "spawn_agent"REDACTED, mapping.NamespaceTools["multi_agent_v1__spawn_agent"])
	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Equal(t, []string{"static_first", "tool_search", "inspect_result", "run_script", "multi_agent_v1__spawn_agent"REDACTED, responsesClientToolNames(t, tools))
	customTool := requireResponsesClientToolValue[map[string]any](t, tools[3])
	require.Equal(t, "function", customTool["type"])
	require.NotContains(t, customTool, "format")
	for _, raw := range requireResponsesClientToolValue[[]any](t, req["input"]) {
		item := requireResponsesClientToolValue[map[string]any](t, raw)
		require.Equal(t, "function_call_output", item["type"])
		require.NotContains(t, item, "tools")
		require.NotContains(t, item, "status")
REDACTED
REDACTED

func TestAdaptResponsesClientTools_RejectsDiscoveredSchemaAndNamespaceCollisions(t *testing.T) {
	tests := []struct {
		name        string
		staticTools []any
		discovered  []any
REDACTED{
		{
			name: "direct schema collision",
			staticTools: []any{map[string]any{
				"type": "function", "name": "inspect", "parameters": map[string]any{"type": "object"REDACTED,
	REDACTED
			discovered: []any{map[string]any{
				"type": "function", "name": "inspect", "parameters": map[string]any{"type": "string"REDACTED,
	REDACTED
	REDACTED,
		{
			name: "namespace schema collision",
			staticTools: []any{map[string]any{
				"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{
					"type": "function", "name": "spawn_agent", "parameters": map[string]any{"type": "object"REDACTED,
		REDACTED
	REDACTED
			discovered: []any{map[string]any{
				"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{
					"type": "function", "name": "spawn_agent", "parameters": map[string]any{"type": "string"REDACTED,
		REDACTED
	REDACTED
	REDACTED,
		{
			name:        "flattened namespace collision",
			staticTools: []any{map[string]any{"type": "function", "name": "multi_agent_v1__spawn_agent"REDACTEDREDACTED,
			discovered: []any{map[string]any{
				"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{"type": "function", "name": "spawn_agent"REDACTEDREDACTED,
	REDACTED
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := map[string]any{
				"tools": append(tt.staticTools, map[string]any{"type": "tool_search"REDACTED),
				"input": []any{map[string]any{
					"type": "tool_search_output", "status": "completed", "tools": tt.discovered,
		REDACTED
		REDACTED
			_, _, err := AdaptResponsesClientTools(req)
			require.ErrorContains(t, err, "conflicts")
	REDACTED)
REDACTED
REDACTED

func TestAdaptResponsesClientTools_DoesNotPromoteUnusableDiscoveries(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{"type": "tool_search"REDACTEDREDACTED,
		"input": []any{
			map[string]any{"type": "tool_search_output", "call_id": "search_in_progress", "status": "in_progress", "tools": []any{map[string]any{"type": "function", "name": "not_ready"REDACTEDREDACTEDREDACTED,
			map[string]any{"type": "tool_search_output", "call_id": "search_malformed", "status": "completed", "tools": []any{map[string]any{"type": "function"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED

	_, changed, err := AdaptResponsesClientTools(req)
REDACTED
	require.True(t, changed, "the static tool_search declaration is still lowered")
	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Equal(t, []string{"tool_search"REDACTED, responsesClientToolNames(t, tools))
REDACTED

func responsesClientToolNames(t *testing.T, tools []any) []string {
REDACTED
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		tool := requireResponsesClientToolValue[map[string]any](t, raw)
		names = append(names, requireResponsesClientToolValue[string](t, tool["name"]))
REDACTED
	return names
REDACTED

func TestAdaptResponsesClientTools_ToolSearchOutputEdgeCases(t *testing.T) {
	unencodableOutput := make(chan struct{REDACTED)
	tests := []struct {
		name             string
		item             map[string]any
		wantOutput       any
		wantOutputExists bool
		wantPrivateKeys  []string
		wantExactOutput  bool
		wantErr          bool
REDACTED{
		{
			name:             "absent tools and output is rejected",
			item:             map[string]any{"type": "tool_search_output", "call_id": "call_empty", "status": "completed"REDACTED,
			wantOutputExists: false,
			wantErr:          true,
	REDACTED,
		{
			name: "preexisting string output wins",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_legacy", "output": "legacy",
				"tools": []any{map[string]any{"type": "function", "name": "ignored"REDACTEDREDACTED, "execution": "client",
		REDACTED,
			wantOutput:       "legacy",
			wantOutputExists: true,
			wantExactOutput:  true,
	REDACTED,
		{
			name: "preexisting object output remains legacy representation",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_object", "output": map[string]any{"groups": []any{"github"REDACTEDREDACTED,
				"tools": []any{map[string]any{"type": "function", "name": "ignored"REDACTEDREDACTED,
		REDACTED,
			wantOutput:       `{"groups":["github"]REDACTED`,
			wantOutputExists: true,
			wantExactOutput:  true,
	REDACTED,
		{
			name: "unencodable preexisting output is rejected",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_bad_output", "output": unencodableOutput,
				"tools": []any{map[string]any{"type": "function", "name": "retained"REDACTEDREDACTED, "status": "completed", "execution": "client",
		REDACTED,
			wantOutput: unencodableOutput,
			wantErr:    true,
	REDACTED,
		{
			name: "empty tools array is a valid empty output",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_empty_tools",
				"tools": []any{REDACTED, "status": "completed", "execution": "client",
		REDACTED,
			wantOutput:       `[]`,
			wantOutputExists: true,
			wantExactOutput:  true,
	REDACTED,
		{
			name: "non-array tools value is serialized directly",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_malformed",
				"tools": map[string]any{"unexpected": trueREDACTED, "status": "completed", "execution": "client",
		REDACTED,
			wantOutput:       `{"unexpected":trueREDACTED`,
			wantOutputExists: true,
			wantExactOutput:  true,
	REDACTED,
		{
			name: "unencodable tools is rejected",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_unencodable", "tools": make(chan struct{REDACTED), "status": "completed",
		REDACTED,
			wantErr: true,
	REDACTED,
		{
			name: "missing call id is rejected",
			item: map[string]any{
				"type": "tool_search_output", "tools": []any{REDACTED, "status": "completed",
		REDACTED,
			wantErr: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := map[string]any{
				"tools": []any{map[string]any{"type": "tool_search"REDACTEDREDACTED,
				"input": []any{tt.itemREDACTED,
		REDACTED
			_, changed, err := AdaptResponsesClientTools(req)
			if tt.wantErr {
			REDACTED
				return
		REDACTED
		REDACTED
			require.True(t, changed)
			input := requireResponsesClientToolValue[[]any](t, req["input"])
			output := requireResponsesClientToolValue[map[string]any](t, input[0])
			require.Equal(t, "function_call_output", output["type"])
			actualOutput, outputExists := output["output"]
			require.Equal(t, tt.wantOutputExists, outputExists)
			if tt.wantOutputExists {
				require.Equal(t, tt.wantOutput, actualOutput)
		REDACTED
			if tt.wantExactOutput {
				require.Equal(t, map[string]any{
					"type":    "function_call_output",
					"call_id": output["call_id"],
					"output":  tt.wantOutput,
			REDACTED, output)
		REDACTED
			if len(tt.wantPrivateKeys) > 0 {
				for _, key := range tt.wantPrivateKeys {
					require.Contains(t, output, key)
			REDACTED
		REDACTED else {
				require.NotContains(t, output, "tools")
				require.NotContains(t, output, "status")
				require.NotContains(t, output, "execution")
		REDACTED
	REDACTED)
REDACTED
REDACTED

func requireResponsesClientToolValue[T any](t *testing.T, value any) T {
REDACTED
	typed, ok := value.(T)
	require.True(t, ok, "unexpected value type %T", value)
	return typed
REDACTED

func TestAdaptResponsesClientTools_RejectsAmbiguousNames(t *testing.T) {
	cases := []map[string]any{
		{"tools": []any{map[string]any{"type": "custom", "name": "same"REDACTED, map[string]any{"type": "function", "name": "same"REDACTEDREDACTEDREDACTED,
		{"tools": []any{map[string]any{"type": "tool_search"REDACTED, map[string]any{"type": "function", "name": "tool_search"REDACTEDREDACTEDREDACTED,
		{"tools": []any{map[string]any{"type": "function", "name": "team__send"REDACTED, map[string]any{"type": "namespace", "name": "team", "tools": []any{map[string]any{"type": "function", "name": "send"REDACTEDREDACTEDREDACTEDREDACTEDREDACTED,
REDACTED
	for _, req := range cases {
		_, _, err := AdaptResponsesClientTools(req)
	REDACTED
REDACTED
REDACTED

func TestAdaptResponsesClientToolsWithInheritedMapping_LowersFollowupHistoryWithoutTools(t *testing.T) {
	req := map[string]any{
		"input": []any{
			map[string]any{
				"type": "custom_tool_call", "name": "exec",
				"call_id": "call_1", "input": "pwd",
		REDACTED,
			map[string]any{
				"type": "custom_tool_call_output", "call_id": "call_1",
				"id":     "ctco_client_output_1",
				"output": []any{map[string]any{"type": "input_text", "text": "ok"REDACTEDREDACTED,
		REDACTED,
	REDACTED,
REDACTED
	inherited := ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": trueREDACTEDREDACTED

	mapping, changed, err := AdaptResponsesClientToolsWithInheritedMapping(req, inherited)

REDACTED
	require.True(t, changed)
	require.Equal(t, inherited, mapping)
	items := requireResponsesClientToolValue[[]any](t, req["input"])
	call := requireResponsesClientToolValue[map[string]any](t, items[0])
	require.Equal(t, "function_call", call["type"])
	require.JSONEq(t, `{"input":"pwd"REDACTED`, requireResponsesClientToolValue[string](t, call["arguments"]))
	require.NotContains(t, call, "input")
	output := requireResponsesClientToolValue[map[string]any](t, items[1])
	require.Equal(t, "function_call_output", output["type"])
	require.NotContains(t, output, "id")
	require.JSONEq(t, `[{"text":"ok","type":"input_text"REDACTED]`, requireResponsesClientToolValue[string](t, output["output"]))
REDACTED

func TestAdaptResponsesClientToolsWithInheritedMapping_PromotesOmittedToolsDiscoveryIntoEffectiveDeclarations(t *testing.T) {
	req := map[string]any{
		"input": []any{map[string]any{
			"type": "tool_search_output", "call_id": "call_search", "status": "completed", "execution": "client",
			"tools": []any{map[string]any{
				"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{
					"type": "function", "name": "spawn_agent", "parameters": map[string]any{"type": "object"REDACTED,
		REDACTED
	REDACTED
REDACTED
REDACTED
	inherited := ResponsesClientToolMapping{
		ToolSearch: true,
		NamespaceTools: map[string]ResponsesNamespaceName{
			"codex_app__read_resource": {Namespace: "codex_app", Name: "read_resource"REDACTED,
	REDACTED,
REDACTED
	lowered := []any{
		map[string]any{"type": "function", "name": "static_first", "parameters": map[string]any{"type": "object"REDACTEDREDACTED,
		map[string]any{"type": "function", "name": "tool_search", "parameters": json.RawMessage(toolSearchProxySchema)REDACTED,
		map[string]any{"type": "function", "name": "codex_app__read_resource", "parameters": map[string]any{"type": "object"REDACTEDREDACTED,
REDACTED

	mapping, changed, err := AdaptResponsesClientToolsWithInheritedMapping(req, inherited, lowered)
REDACTED
	require.True(t, changed)
	require.True(t, mapping.ToolSearch)
	require.Equal(t, ResponsesNamespaceName{Namespace: "codex_app", Name: "read_resource"REDACTED, mapping.NamespaceTools["codex_app__read_resource"])
	require.Equal(t, ResponsesNamespaceName{Namespace: "multi_agent_v1", Name: "spawn_agent"REDACTED, mapping.NamespaceTools["multi_agent_v1__spawn_agent"])
	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Equal(t, []string{
		"static_first", "tool_search", "codex_app__read_resource", "multi_agent_v1__spawn_agent",
REDACTED, responsesClientToolNames(t, tools))
	output := requireResponsesClientToolValue[map[string]any](t, requireResponsesClientToolValue[[]any](t, req["input"])[0])
	require.Equal(t, "function_call_output", output["type"])
	require.IsType(t, "", output["output"])
	require.NotContains(t, output, "tools")
	require.NotContains(t, output, "status")
	require.NotContains(t, output, "execution")
REDACTED

func TestAdaptResponsesClientToolsWithInheritedMapping_ExplicitToolsReplaceInheritedMapping(t *testing.T) {
	req := map[string]any{
		"tools": []any{REDACTED,
		"input": []any{map[string]any{
			"type": "custom_tool_call", "name": "exec", "input": "pwd",
REDACTED
REDACTED

	mapping, changed, err := AdaptResponsesClientToolsWithInheritedMapping(
		req,
		ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": trueREDACTEDREDACTED,
	)

REDACTED
	require.False(t, changed)
	require.Empty(t, mapping)
	items := requireResponsesClientToolValue[[]any](t, req["input"])
	call := requireResponsesClientToolValue[map[string]any](t, items[0])
	require.Equal(t, "custom_tool_call", call["type"])
REDACTED

func TestAdaptResponsesClientToolsWithInheritedMapping_ExplicitToolResetDoesNotPromoteDiscovery(t *testing.T) {
	for _, reset := range []any{nil, []any{REDACTEDREDACTED {
		req := map[string]any{
			"tools": reset,
			"input": []any{map[string]any{
				"type": "tool_search_output", "call_id": "call_reset", "status": "completed",
				"tools": []any{map[string]any{"type": "function", "name": "must_not_promote"REDACTEDREDACTED,
	REDACTED
	REDACTED
		mapping, changed, err := AdaptResponsesClientToolsWithInheritedMapping(
			req,
			ResponsesClientToolMapping{ToolSearch: trueREDACTED,
			[]any{map[string]any{"type": "function", "name": "tool_search"REDACTEDREDACTED,
		)
	REDACTED
		require.False(t, changed)
		require.Empty(t, mapping)
		item := requireResponsesClientToolValue[map[string]any](t, requireResponsesClientToolValue[[]any](t, req["input"])[0])
		require.Equal(t, "tool_search_output", item["type"])
REDACTED
REDACTED

func TestRestoreResponsesClientToolPayload_RestoresClientAndNamespaceCalls(t *testing.T) {
	mapping := ResponsesClientToolMapping{
		CustomTools: map[string]bool{"exec": trueREDACTED, ToolSearch: true,
		NamespaceTools: map[string]ResponsesNamespaceName{"team__send": {Namespace: "team", Name: "send"REDACTEDREDACTED,
REDACTED
	payload := []byte(`{"id":"resp","output":[{"type":"function_call","id":"i1","call_id":"c1","name":"exec","arguments":"{\"input\":\"dir\"REDACTED","namespace":"ignore"REDACTED,{"type":"function_call","id":"i2","call_id":"s1","name":"tool_search","arguments":"{\"query\":\"git\"REDACTED"REDACTED,{"type":"function_call","id":"i3","call_id":"n1","name":"team__send","arguments":"{REDACTED"REDACTED]REDACTED`)

	restored, changed, err := RestoreResponsesClientToolPayload(payload, mapping)
REDACTED
	require.True(t, changed)
	require.JSONEq(t, `{"id":"resp","output":[{"type":"custom_tool_call","id":"i1","call_id":"c1","name":"exec","input":"dir"REDACTED,{"type":"tool_search_call","id":"i2","call_id":"s1","execution":"client","arguments":{"query":"git"REDACTEDREDACTED,{"type":"function_call","id":"i3","call_id":"n1","name":"send","namespace":"team","arguments":"{REDACTED"REDACTED]REDACTED`, string(restored))
REDACTED

func TestResponsesClientToolStreamRestorer_CustomToolBuffersWrapperAndSequences(t *testing.T) {
	restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": trueREDACTEDREDACTED)
	added := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.added", SequenceNumber: 7, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "i1", CallID: "c1", Name: "exec", Status: "in_progress"REDACTEDREDACTED)
	require.Len(t, added, 1)
	require.Equal(t, 7, added[0].SequenceNumber)
	require.Equal(t, "custom_tool_call", added[0].Item.Type)
	require.Empty(t, restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", SequenceNumber: 8, ItemID: "i1", Delta: `{"input":"di`REDACTED))
	done := restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", SequenceNumber: 9, ItemID: "i1", CallID: "c1", Name: "exec", Arguments: `{"input":"dir"REDACTED`REDACTED)
	require.Len(t, done, 2)
	require.Equal(t, 8, done[0].SequenceNumber)
	require.Equal(t, "response.custom_tool_call_input.delta", done[0].Type)
	require.Equal(t, "dir", done[0].Delta)
	require.Equal(t, 9, done[1].SequenceNumber)
	require.Equal(t, "response.custom_tool_call_input.done", done[1].Type)
	require.Equal(t, "dir", done[1].Input)
	closed := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.done", SequenceNumber: 10, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "i1", CallID: "c1", Name: "exec", Arguments: `{"input":"dir"REDACTED`, Status: "completed"REDACTEDREDACTED)
	require.Equal(t, 10, closed[0].SequenceNumber)
	require.Equal(t, "custom_tool_call", closed[0].Item.Type)
	require.Equal(t, "dir", closed[0].Item.Input)
REDACTED

func TestResponsesClientToolStreamRestorer_ToolSearchAndFunction(t *testing.T) {
	restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{ToolSearch: trueREDACTED)
	search := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.added", SequenceNumber: 0, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "s1", CallID: "c1", Name: "tool_search", Status: "in_progress"REDACTEDREDACTED)
	require.Equal(t, "tool_search_call", search[0].Item.Type)
	require.Empty(t, restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", SequenceNumber: 1, ItemID: "s1", Delta: `{"query":"git"REDACTED`REDACTED))
	require.Empty(t, restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", SequenceNumber: 2, ItemID: "s1", Arguments: `{"query":"git"REDACTED`REDACTED))
	closed := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.done", SequenceNumber: 3, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "s1", CallID: "c1", Name: "tool_search", Status: "completed"REDACTEDREDACTED)
	require.Equal(t, 1, closed[0].SequenceNumber)
	require.Equal(t, "tool_search_call", closed[0].Item.Type)
	require.JSONEq(t, `{"query":"git"REDACTED`, string(toolSearchCallArgumentsJSON(closed[0].Item.Arguments)))

	function := restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", SequenceNumber: 4, ItemID: "plain", Name: "plain", Arguments: "{REDACTED"REDACTED)
	require.Len(t, function, 1)
	require.Equal(t, "response.function_call_arguments.done", function[0].Type)
	require.Equal(t, 2, function[0].SequenceNumber)
REDACTED

func TestResponsesClientToolStreamRestorer_RestoresNamespaceLifecycle(t *testing.T) {
	restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{
		NamespaceTools: map[string]ResponsesNamespaceName{
			"browser__open": {Namespace: "browser", Name: "open"REDACTED,
	REDACTED,
REDACTED)

	added, changed, err := restorer.RestoreEvent([]byte(`{"type":"response.output_item.added","sequence_number":4,"output_index":0,"item":{"type":"function_call","id":"i1","call_id":"c1","name":"browser__open","arguments":"","status":"in_progress"REDACTEDREDACTED`))
REDACTED
	require.True(t, changed)
	require.Len(t, added, 1)
	require.Equal(t, "open", gjson.GetBytes(added[0], "item.name").String())
	require.Equal(t, "browser", gjson.GetBytes(added[0], "item.namespace").String())

	delta, changed, err := restorer.RestoreEvent([]byte(`{"type":"response.function_call_arguments.delta","sequence_number":5,"output_index":0,"item_id":"i1","name":"browser__open","delta":"{\"url\":"REDACTED`))
REDACTED
	require.True(t, changed)
	require.Len(t, delta, 1)
	require.Equal(t, "open", gjson.GetBytes(delta[0], "name").String())

	done, changed, err := restorer.RestoreEvent([]byte(`{"type":"response.function_call_arguments.done","sequence_number":6,"output_index":0,"item_id":"i1","name":"browser__open","arguments":"{REDACTED"REDACTED`))
REDACTED
	require.True(t, changed)
	require.Len(t, done, 1)
	require.Equal(t, "open", gjson.GetBytes(done[0], "name").String())
REDACTED

func TestResponsesClientToolStreamRestorer_RawEventsPreserveUnknownFieldsAndOutputFallback(t *testing.T) {
	restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": trueREDACTEDREDACTED)
	passthrough, changed, err := restorer.RestoreEvent([]byte(`{"type":"response.created","sequence_number":4,"response":{"id":"r"REDACTED,"upstream_extension":{"keep":trueREDACTEDREDACTED`))
REDACTED
	require.False(t, changed)
	require.Len(t, passthrough, 1)
	require.Contains(t, string(passthrough[0]), `"upstream_extension":{"keep":trueREDACTED`)

	restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.added", SequenceNumber: 5, OutputIndex: 9, Item: &ResponsesOutput{Type: "function_call", ID: "item", CallID: "call", Name: "exec"REDACTEDREDACTED)
	// Some upstreams omit every tool identity field on later argument chunks.
	require.Empty(t, restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", SequenceNumber: 6, OutputIndex: 9, Delta: `{"input":"pwd"REDACTED`REDACTED))
	done := restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", SequenceNumber: 7, OutputIndex: 9REDACTED)
	require.Len(t, done, 2)
	require.Equal(t, "pwd", done[1].Input)
REDACTED

func TestResponsesClientToolStreamRestorer_RestoresAllTerminalEvents(t *testing.T) {
	for _, eventType := range []string{
		"response.completed",
		"response.done",
		"response.incomplete",
		"response.failed",
		"response.cancelled",
		"response.canceled",
REDACTED {
		t.Run(eventType, func(t *testing.T) {
			restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": trueREDACTEDREDACTED)
			payload := []byte(`{"type":"` + eventType + `","sequence_number":7,"response":{"id":"resp_tools","output":[{"type":"function_call","id":"item_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"pwd\"REDACTED"REDACTED]REDACTEDREDACTED`)

			restored, changed, err := restorer.RestoreEvent(payload)

		REDACTED
			require.True(t, changed)
			require.Len(t, restored, 1)
			require.Equal(t, eventType, gjson.GetBytes(restored[0], "type").String())
			require.Equal(t, int64(7), gjson.GetBytes(restored[0], "sequence_number").Int())
			require.Equal(t, "custom_tool_call", gjson.GetBytes(restored[0], "response.output.0.type").String())
			require.Equal(t, "pwd", gjson.GetBytes(restored[0], "response.output.0.input").String())
			require.False(t, gjson.GetBytes(restored[0], "response.output.0.arguments").Exists())
	REDACTED)
REDACTED
REDACTED
