package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTruncateUsageRequestPrompt(t *testing.T) {
	short := "a prompt"
	require.Equal(t, short, TruncateUsageRequestPrompt(short))

	long := strings.Repeat("你", MaxUsageRequestPromptLength+10)
	got := TruncateUsageRequestPrompt(long)
	require.Equal(t, MaxUsageRequestPromptLength, len([]rune(got)))
	require.Equal(t, strings.Repeat("你", MaxUsageRequestPromptLength), got)
}

func TestOpenAIImagesRequestRequestParametersTruncatesPrompt(t *testing.T) {
	params := (&OpenAIImagesRequest{Prompt: strings.Repeat("p", MaxUsageRequestPromptLength+1)}).RequestParameters()
	prompt, ok := params["prompt"].(string)
	require.True(t, ok)
	require.Equal(t, MaxUsageRequestPromptLength, len(prompt))
}
