package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const delegationEnvelope = `<codex_delegation><source_thread_id>thread-1</source_thread_id><input>do the work</input></codex_delegation>`

func TestNormalizeCodexDelegationBootstrapSupportedTools(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		tool      string
		callID    string
	}{
		{name: "app create missing call id", namespace: "codex_app", tool: "create_thread"},
		{name: "app send empty call id", namespace: "codex_app", tool: "send_message_to_thread", callID: `,"call_id":""`},
		{name: "tui create whitespace call id", namespace: "codex_tui", tool: "create_thread", callID: `,"call_id":"  "`},
		{name: "tui send missing call id", namespace: "codex_tui", tool: "send_message_to_thread"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"` + tt.namespace + `","name":"` + tt.tool + `","output":"` + delegationEnvelope + `"` + tt.callID + `}]}`)
			got, changed := normalizeCodexDelegationBootstrap(body)
			require.True(t, changed)
			require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
			require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
			require.Equal(t, delegationEnvelope, gjson.GetBytes(got, "input.0.content.0.text").String())
			require.False(t, gjson.GetBytes(got, "input.0.call_id").Exists())
		})
	}
}

func TestNormalizeCodexDelegationBootstrapRejectsUnsafeShapes(t *testing.T) {
	tests := []struct {
		name  string
		item  string
		extra string
	}{
		{name: "ordinary missing call id", item: `{"type":"function_call_output","name":"other","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "valid call id", item: `{"type":"function_call_output","call_id":"call-1","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "ambiguous call context", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"function_call"}`},
		{name: "ambiguous built-in call context", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"computer_call"}`},
		{name: "mixed missing call id output", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"computer_call_output","output":"done"}`},
		{name: "mixed tool search output", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"tool_search_output","output":"done"}`},
		{name: "mixed invalid delegation output", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"not an envelope"}`},
		{name: "non string call id", item: `{"type":"function_call_output","call_id":42,"name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "padded type", item: `{"type":" function_call_output ","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "padded name", item: `{"type":"function_call_output","name":" create_thread ","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "padded namespace", item: `{"type":"function_call_output","name":"create_thread","namespace":" codex_app ","output":"` + delegationEnvelope + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5","input":[` + tt.item + `]` + tt.extra + `}`)
			got, changed := normalizeCodexDelegationBootstrap(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		})
	}
}

func TestNormalizeCodexDelegationBootstrapWithHistoricalContext(t *testing.T) {
	body := []byte(`{"model":"gpt-5","previous_response_id":"resp-1","input":[{"type":"function_call_output","namespace":"codex_app","name":"send_message_to_thread","output":"` + delegationEnvelope + `"},{"type":"function_call","call_id":"call-1"},{"type":"function_call_output","call_id":"call-1","output":"done"},{"type":"item_reference","id":"item-1"},{"type":"computer_call","call_id":"call-2"}]}`)

	got, changed := normalizeCodexDelegationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
	require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
	require.Equal(t, "resp-1", gjson.GetBytes(got, "previous_response_id").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "input.2.call_id").String())
	require.Equal(t, "item-1", gjson.GetBytes(got, "input.3.id").String())
}

func TestNormalizeCodexDelegationBootstrapPreviousResponseID(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		changed bool
	}{
		{name: "absent", changed: true},
		{name: "blank string", value: `,"previous_response_id":"  "`, changed: true},
		{name: "non-empty string", value: `,"previous_response_id":"resp-1"`, changed: true},
		{name: "null", value: `,"previous_response_id":null`},
		{name: "number", value: `,"previous_response_id":42`},
		{name: "boolean", value: `,"previous_response_id":false`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]` + tt.value + `}`)
			got, changed := normalizeCodexDelegationBootstrap(body)
			require.Equal(t, tt.changed, changed)
			if !tt.changed {
				require.Equal(t, body, got)
			}
		})
	}
}

func TestNormalizeCodexDelegationBootstrapRejectsDuplicateMembers(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "top-level previous response id",
			body: `{"model":"gpt-5","previous_response_id":"","previous_response_id":"resp-1","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]}`,
		},
		{
			name: "candidate discriminator",
			body: `{"model":"gpt-5","input":[{"type":"message","type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]}`,
		},
		{
			name: "candidate call id",
			body: `{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","call_id":"call-1","call_id":"","output":"` + delegationEnvelope + `"}]}`,
		},
		{
			name: "candidate output",
			body: `{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"not a delegation","output":"` + delegationEnvelope + `"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			got, changed := normalizeCodexDelegationBootstrap(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		})
	}
}

func TestNormalizeCodexDelegationBootstrapPreservesExactNumbers(t *testing.T) {
	body := []byte(`{"model":"gpt-5","metadata":{"integer":9007199254740993},"input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]}`)
	got, changed := normalizeCodexDelegationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.integer").Raw)
}

func TestNormalizeCodexDelegationBootstrapRequiresCompleteEnvelope(t *testing.T) {
	malformed := []string{
		`<codex_delegation><source_thread_id>thread-1</source_thread_id></codex_delegation>`,
		`<codex_delegation><source_thread_id></source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input> </input></codex_delegation>`,
		`prefix<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>suffix`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input><extra>x</extra></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input><nested>work</nested></input></codex_delegation>`,
		`<x:codex_delegation xmlns:x="urn:test"><source_thread_id>thread-1</source_thread_id><input>work</input></x:codex_delegation>`,
		`<x:codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></x:codex_delegation>`,
		`<codex_delegation xmlns="urn:test"><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation xmlns:x="urn:test"><x:source_thread_id>thread-1</x:source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><x:source_thread_id>thread-1</x:source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation xmlns=""><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
	}
	for _, envelope := range malformed {
		body := []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":` + mustJSON(t, envelope) + `}]}`)
		got, changed := normalizeCodexDelegationBootstrap(body)
		require.False(t, changed, envelope)
		require.Equal(t, body, got)
	}
}

func TestNormalizeCodexDelegationBootstrapPreservesOrderAndIsIdempotent(t *testing.T) {
	body := []byte(`{"model":"gpt-5","input":[{"type":"message","role":"user","content":"before"},{"type":"function_call_output","namespace":"codex_tui","name":"send_message_to_thread","output":"` + delegationEnvelope + `"},{"type":"message","role":"user","content":"after"}]}`)
	got, changed := normalizeCodexDelegationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "before", gjson.GetBytes(got, "input.0.content").String())
	require.Equal(t, delegationEnvelope, gjson.GetBytes(got, "input.1.content.0.text").String())
	require.Equal(t, "after", gjson.GetBytes(got, "input.2.content").String())

	again, changedAgain := normalizeCodexDelegationBootstrap(got)
	require.False(t, changedAgain)
	require.Equal(t, got, again)
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
