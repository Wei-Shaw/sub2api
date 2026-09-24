package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractContentModerationInput_SystemOneCollectsStateAndQuestions(t *testing.T) {
	body := []byte(`{
		"model": "jev-1.13",
		"state": "payments failed three days",
		"questions": {
			"department": {"type": "choice", "instructions": "which team", "criteria": {"billing": "payment problems", "technical": "bugs"}},
			"frustration": {"type": "score", "instructions": "how frustrated", "criteria": ["calm", "angry"]}
		}
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolSystemOne, body)

	require.Contains(t, input.Text, "payments failed three days")
	require.Contains(t, input.Text, "which team")
	require.Contains(t, input.Text, "billing: payment problems")
	require.Contains(t, input.Text, "technical: bugs")
	require.Contains(t, input.Text, "how frustrated")
	require.Contains(t, input.Text, "calm")
	require.Contains(t, input.Text, "angry")
	require.Empty(t, input.Images)
}

func TestExtractContentModerationInput_SystemOneEmptyQuestionsYieldsStateOnly(t *testing.T) {
	body := []byte(`{"model": "jev-1.13", "state": "probe state"}`)

	input := ExtractContentModerationInput(ContentModerationProtocolSystemOne, body)

	require.Equal(t, "probe state", input.Text)
}

func TestExtractContentModerationInput_SystemOneEmptyBodyYieldsEmpty(t *testing.T) {
	input := ExtractContentModerationInput(ContentModerationProtocolSystemOne, []byte(`{"model":"jev-1.13"}`))

	require.Empty(t, input.Text)
}

func TestExtractContentModerationInput_SystemOneStateSurvivesLongQuestions(t *testing.T) {
	body := []byte(`{"model":"jev-1.13","state":"SENSITIVE_STATE_MARKER","questions":{"q":{"type":"choice","instructions":"` + strings.Repeat("x", maxModerationInputRunes+1000) + `","criteria":{"x":"y"}}}}`)

	input := ExtractContentModerationInput(ContentModerationProtocolSystemOne, body)

	require.Contains(t, input.Text, "SENSITIVE_STATE_MARKER")
}

func TestExtractContentModerationInput_SystemOneDoesNotFilterReminders(t *testing.T) {
	body := []byte(`{"model":"jev-1.13","state":"<system-reminder>REMINDER_STATE_MARKER</system-reminder>","questions":{"q":{"type":"noul","instructions":"plain"}}}`)

	input := ExtractContentModerationInput(ContentModerationProtocolSystemOne, body)

	require.Contains(t, input.Text, "REMINDER_STATE_MARKER")
}
