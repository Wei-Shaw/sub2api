//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUsageInputExcerpt_ChatMessagesUsesLastUserPrompt(t *testing.T) {
	body := []byte(`{
		"messages":[
			{"role":"system","content":"ignore"},
			{"role":"user","content":"first prompt"},
			{"role":"assistant","content":"reply"},
			{"role":"user","content":[{"type":"text","text":"last user prompt"}]}
		]
	}`)

	require.Equal(t, "last user prompt", BuildUsageInputExcerpt(body))
}

func TestBuildUsageInputExcerpt_ResponsesInputRedactsSecrets(t *testing.T) {
	body := []byte(`{
		"input":[{"role":"user","content":[{"type":"input_text","text":"token=sk-proj-1234567890abcdef hello"}]}]
	}`)

	excerpt := BuildUsageInputExcerpt(body)
	require.NotContains(t, excerpt, "sk-proj-1234567890abcdef")
	require.Contains(t, excerpt, "[已脱敏]")
}

func TestBuildUsageInputExcerpt_EmbeddingsArrayInput(t *testing.T) {
	body := []byte(`{"input":["first embedding text","second embedding text"]}`)
	require.Equal(t, "first embedding text second embedding text", BuildUsageInputExcerpt(body))
}

func TestBuildUsageInputExcerpt_GeminiContents(t *testing.T) {
	body := []byte(`{
		"contents":[
			{"role":"model","parts":[{"text":"ignored"}]},
			{"role":"user","parts":[{"text":"gemini prompt"}]}
		]
	}`)

	require.Equal(t, "gemini prompt", BuildUsageInputExcerpt(body))
}

func TestBuildUsageInputExcerpt_TruncatesLongText(t *testing.T) {
	body := []byte(`{"prompt":"` + strings.Repeat("a", maxUsageInputExcerptRunes+20) + `"}`)

	excerpt := BuildUsageInputExcerpt(body)
	require.Len(t, []rune(excerpt), maxUsageInputExcerptRunes)
}
