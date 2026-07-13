package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenResponsesNamespaces_RewritesDeclarationHistoryAndChoice(t *testing.T) {
	req := map[string]any{
		"model": "gpt-5.5",
		"tools": []any{
			map[string]any{"type": "function", "name": "plain", "description": "keep"REDACTED,
			map[string]any{
				"type": "namespace",
				"name": "collaboration",
				"tools": []any{
					map[string]any{"type": "function", "name": "spawn_agent", "description": "spawn", "parameters": map[string]any{"type": "object"REDACTEDREDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		"tool_choice": map[string]any{"type": "function", "name": "spawn_agent", "namespace": "collaboration"REDACTED,
		"input": []any{
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "spawn_agent", "namespace": "collaboration", "arguments": "{REDACTED"REDACTED,
			map[string]any{"type": "message", "role": "user", "content": "hi", "name": "spawn_agent", "namespace": "collaboration"REDACTED,
	REDACTED,
REDACTED

	names, changed, err := FlattenResponsesNamespaces(req)
REDACTED
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "spawn_agent"REDACTED, names["collaboration__spawn_agent"])

	tools := req["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "plain", tools[0].(map[string]any)["name"])
	require.Equal(t, "collaboration__spawn_agent", tools[1].(map[string]any)["name"])
	require.Equal(t, "spawn", tools[1].(map[string]any)["description"])

	choice := req["tool_choice"].(map[string]any)
	require.Equal(t, "collaboration__spawn_agent", choice["name"])
	require.NotContains(t, choice, "namespace")

	call := req["input"].([]any)[0].(map[string]any)
	require.Equal(t, "collaboration__spawn_agent", call["name"])
	require.NotContains(t, call, "namespace")
	message := req["input"].([]any)[1].(map[string]any)
	require.Equal(t, "spawn_agent", message["name"])
	require.Equal(t, "collaboration", message["namespace"])
	require.Equal(t, "gpt-5.5", req["model"])
REDACTED

func TestFlattenResponsesNamespaces_RejectsFlatNameCollision(t *testing.T) {
	req := map[string]any{"tools": []any{
		map[string]any{"type": "function", "name": "collaboration__spawn_agent"REDACTED,
		map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
			map[string]any{"type": "function", "name": "spawn_agent"REDACTED,
REDACTED
REDACTEDREDACTED

	_, _, err := FlattenResponsesNamespaces(req)
	require.ErrorContains(t, err, "conflicts with a top-level tool")
REDACTED

func TestFlattenResponsesNamespaces_NamespaceGroupChoiceFallsBackToAuto(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration", "tools": []any{
				map[string]any{"type": "function", "name": "spawn_agent"REDACTED,
				map[string]any{"type": "function", "name": "send_message"REDACTED,
		REDACTED,
REDACTED
		"tool_choice": map[string]any{"type": "namespace", "name": "collaboration"REDACTED,
REDACTED

	_, changed, err := FlattenResponsesNamespaces(req)
REDACTED
	require.True(t, changed)
	require.Equal(t, "auto", req["tool_choice"])
REDACTED

func TestFlattenResponsesNamespacesExcept_PreservesBuiltInNamespaceAndChoice(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "namespace", "name": "image_gen", "tools": []any{
				map[string]any{"type": "function", "name": "imagegen"REDACTED,
	REDACTED
			map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
				map[string]any{"type": "function", "name": "spawn_agent"REDACTED,
	REDACTED
	REDACTED,
		"tool_choice": map[string]any{"type": "namespace", "name": "image_gen"REDACTED,
REDACTED

	names, changed, err := FlattenResponsesNamespacesExcept(req, map[string]bool{"image_gen": trueREDACTED)
REDACTED
	require.True(t, changed)
	require.Contains(t, names, "collaboration__spawn_agent")
	tools := req["tools"].([]any)
	require.Equal(t, "namespace", tools[0].(map[string]any)["type"])
	require.Equal(t, "image_gen", tools[0].(map[string]any)["name"])
	require.Equal(t, "function", tools[1].(map[string]any)["type"])
	require.Equal(t, "collaboration__spawn_agent", tools[1].(map[string]any)["name"])
	require.Equal(t, map[string]any{"type": "namespace", "name": "image_gen"REDACTED, req["tool_choice"])
REDACTED

func TestFlattenResponsesNamespaces_RejectsNamespaceCollision(t *testing.T) {
	req := map[string]any{"tools": []any{
		map[string]any{"type": "namespace", "name": "a", "tools": []any{
			map[string]any{"type": "function", "name": "b__c"REDACTED,
REDACTED
		map[string]any{"type": "namespace", "name": "a__b", "tools": []any{
			map[string]any{"type": "function", "name": "c"REDACTED,
REDACTED
REDACTEDREDACTED

	_, _, err := FlattenResponsesNamespaces(req)
	require.ErrorContains(t, err, "both flatten")
REDACTED

func TestRestoreResponsesNamespaceCalls_RewritesOnlyFunctionCalls(t *testing.T) {
	payload := []byte(`{"type":"response.completed","response":{"output":[{"type":"function_call","name":"collaboration__spawn_agent","call_id":"call_1","arguments":"{REDACTED","extra":"keep"REDACTED,{"type":"function_call","name":"plain","arguments":"{REDACTED"REDACTED,{"type":"message","name":"collaboration__spawn_agent","content":"<tag>&value</tag>"REDACTED]REDACTEDREDACTED`)
	names := map[string]ResponsesNamespaceName{
		"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"REDACTED,
REDACTED

	got, changed, err := RestoreResponsesNamespaceCalls(payload, names)
REDACTED
	require.True(t, changed)
	require.JSONEq(t, `{"type":"response.completed","response":{"output":[{"type":"function_call","name":"spawn_agent","namespace":"collaboration","call_id":"call_1","arguments":"{REDACTED","extra":"keep"REDACTED,{"type":"function_call","name":"plain","arguments":"{REDACTED"REDACTED,{"type":"message","name":"collaboration__spawn_agent","content":"<tag>&value</tag>"REDACTED]REDACTEDREDACTED`, string(got))
	require.Contains(t, string(got), "<tag>&value</tag>")
	require.NotContains(t, string(got), `\u003c`)
REDACTED

func TestRestoreResponsesNamespaceCalls_RewritesLifecycleItems(t *testing.T) {
	for _, eventType := range []string{"response.output_item.added", "response.output_item.done"REDACTED {
		t.Run(eventType, func(t *testing.T) {
			payload := []byte(`{"type":"` + eventType + `","item":{"type":"function_call","name":"collaboration__spawn_agent","arguments":"{REDACTED"REDACTEDREDACTED`)
			got, changed, err := RestoreResponsesNamespaceCalls(payload, map[string]ResponsesNamespaceName{
				"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"REDACTED,
		REDACTED)
		REDACTED
			require.True(t, changed)
			require.JSONEq(t, `{"type":"`+eventType+`","item":{"type":"function_call","name":"spawn_agent","namespace":"collaboration","arguments":"{REDACTED"REDACTEDREDACTED`, string(got))
	REDACTED)
REDACTED
REDACTED
