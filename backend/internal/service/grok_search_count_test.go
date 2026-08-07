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

func TestCountGrokNativeSearchCallsInSSEDataDedup_LiveStreamPath(t *testing.T) {
	t.Parallel()
	// Mirrors the live streaming accumulator: item.done then response.completed
	// for the same call_id must bill once (regression for ~2× surcharge).
	seen := make(map[string]struct{REDACTED)
	done := []byte(`{"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws1","call_id":"c1"REDACTEDREDACTED`)
	completed := []byte(`{"type":"response.completed","response":{"output":[{"type":"web_search_call","id":"ws1","call_id":"c1"REDACTED,{"type":"x_search_call","id":"xs1","call_id":"c2"REDACTED]REDACTEDREDACTED`)
	require.Equal(t, 1, countGrokNativeSearchCallsInSSEDataDedup(done, seen))
	require.Equal(t, 1, countGrokNativeSearchCallsInSSEDataDedup(completed, seen))
	// Raw (no-dedup) path still double-counts the same envelope pair.
	require.Equal(t, 1, countGrokNativeSearchCallsInSSEData(done))
	require.Equal(t, 2, countGrokNativeSearchCallsInSSEData(completed))
REDACTED

func stringsJoin(lines ...string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n\n"
REDACTED
	return out
REDACTED
