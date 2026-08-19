package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWSNextAttemptMessageUsesCurrentTurnPayload(t *testing.T) {
	firstMessage := []byte(`{"type":"response.create","input":"first"REDACTED`)
	currentTurn := []byte(`{"type":"response.create","input":"turn-281"REDACTED`)

	next, ok := openAIWSNextAttemptMessage(firstMessage, currentTurn, true)

	require.True(t, ok)
	require.Equal(t, currentTurn, next)
	next[0] = 'x'
	require.Equal(t, byte('{'), currentTurn[0], "retry payload must be cloned")
REDACTED

func TestOpenAIWSNextAttemptMessageRejectsMissingCurrentTurnPayload(t *testing.T) {
	next, ok := openAIWSNextAttemptMessage([]byte(`{"type":"response.create"REDACTED`), nil, true)

	require.False(t, ok)
	require.Nil(t, next)
REDACTED

func TestOpenAIWSNextAttemptMessageKeepsInitialMessageForFirstTurnFailover(t *testing.T) {
	firstMessage := []byte(`{"type":"response.create","input":"first"REDACTED`)

	next, ok := openAIWSNextAttemptMessage(firstMessage, nil, false)

	require.True(t, ok)
	require.Equal(t, firstMessage, next)
REDACTED
