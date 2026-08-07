package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountGrokNativeSearchCallsFromJSONBytes(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, countGrokNativeSearchCallsFromJSONBytes(nil))
	require.Equal(t, 0, countGrokNativeSearchCallsFromJSONBytes([]byte(`{"output":[]REDACTED`)))
	body := []byte(`{"output":[
		{"type":"web_search_call","id":"ws1","status":"completed"REDACTED,
		{"type":"x_search_call","id":"xs1"REDACTED,
		{"type":"function_call","name":"tool_search","call_id":"ts1"REDACTED,
		{"type":"function_call","name":"lookup","call_id":"other"REDACTED
	]REDACTED`)
	require.Equal(t, 3, countGrokNativeSearchCallsFromJSONBytes(body))
REDACTED

func TestCountGrokNativeSearchCallsFromSSEBodyDedups(t *testing.T) {
	t.Parallel()
	sse := stringsJoin(
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws1","call_id":"c1"REDACTEDREDACTED`,
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws1","call_id":"c1"REDACTEDREDACTED`,
		`data: {"type":"response.completed","response":{"output":[{"type":"web_search_call","id":"ws1","call_id":"c1"REDACTED,{"type":"x_search_call","id":"xs1","call_id":"c2"REDACTED]REDACTEDREDACTED`,
	)
	require.Equal(t, 2, countGrokNativeSearchCallsFromSSEBody(sse))
REDACTED

func stringsJoin(lines ...string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n\n"
REDACTED
	return out
REDACTED
