package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestDecideResponsesProbeSupport(t *testing.T) {
	fnCall := []byte(`{"output":[{"type":"reasoning"REDACTED,{"type":"function_call","name":"probe_ping"REDACTED]REDACTED`)
	reasoningOnly := []byte(`{"output":[{"type":"reasoning"REDACTED]REDACTED`)

	cases := []struct {
		name   string
		status int
		body   []byte
		want   bool
REDACTED{
		// Endpoint clearly absent on third-party OpenAI-compatible upstreams.
		{"404 endpoint absent", 404, fnCall, falseREDACTED,
		{"405 method not allowed", 405, fnCall, falseREDACTED,
		// 2xx: tool capability is judged by presence of a function_call output item.
		{"200 with function_call", 200, fnCall, trueREDACTED,
		// Volcengine Ark coding/v3 × kimi-k2.6: reasoning only, no function_call.
		{"200 reasoning only", 200, reasoningOnly, falseREDACTED,
		{"200 invalid json", 200, []byte("not-json"), falseREDACTED,
		{"200 no output field", 200, []byte(`{"status":"completed"REDACTED`), falseREDACTED,
		// Non-2xx (other than 404/405): endpoint exists, capability undecidable -> conservative true.
		{"400 conservative true", 400, reasoningOnly, trueREDACTED,
		{"401 conservative true", 401, nil, trueREDACTED,
		{"500 conservative true", 500, nil, trueREDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, decideResponsesProbeSupport(tc.status, tc.body))
	REDACTED)
REDACTED
REDACTED

func TestResponsesProbeBodyHasFunctionCall(t *testing.T) {
	require.True(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"function_call"REDACTED]REDACTED`)))
	require.True(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"reasoning"REDACTED,{"type":"function_call"REDACTED]REDACTED`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"reasoning"REDACTED]REDACTED`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[]REDACTED`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{REDACTED`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`garbage`)))
REDACTED

func TestSelectResponsesProbeModel(t *testing.T) {
	// No model_mapping -> fall back to DefaultTestModel (OpenAI official APIKey).
	require.Equal(t, openai.DefaultTestModel, selectResponsesProbeModel(&Account{REDACTED))

	// model_mapping values are upstream models; pick first by sort for reproducibility.
	acct := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{
			"client-b": "zeta-model",
			"client-a": "alpha-model",
	REDACTED,
REDACTEDREDACTED
	require.Equal(t, "alpha-model", selectResponsesProbeModel(acct))

	// Wildcard / blank upstream values are skipped.
	acctWild := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{
			"a": "*",
			"b": "  ",
			"c": "real-model",
	REDACTED,
REDACTEDREDACTED
	require.Equal(t, "real-model", selectResponsesProbeModel(acctWild))

	// Only wildcard mappings -> DefaultTestModel.
	acctAllWild := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{"a": "gpt-*"REDACTED,
REDACTEDREDACTED
	require.Equal(t, openai.DefaultTestModel, selectResponsesProbeModel(acctAllWild))
REDACTED
