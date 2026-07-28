package handler

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPlaygroundChatRequestNormalize(t *testing.T) {
	req := PlaygroundChatRequest{MaxTokens: playgroundMaxTokens + 1}
	req.normalize()

	require.Equal(t, playgroundDefaultModel, req.Model)
	require.Equal(t, playgroundDefaultPrompt, req.Prompt)
	require.Equal(t, playgroundMaxTokens, req.MaxTokens)
}

func TestPlaygroundChatRequestValidateRejectsLongPrompt(t *testing.T) {
	req := PlaygroundChatRequest{Prompt: strings.Repeat("a", playgroundMaxPromptRunes+1)}
	require.Error(t, req.validate())
}

func TestPlaygroundUsesOpenAIGateway(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     bool
	}{
		{name: "openai", platform: service.PlatformOpenAI, want: true},
		{name: "grok", platform: service.PlatformGrok, want: true},
		{name: "anthropic", platform: service.PlatformAnthropic, want: false},
		{name: "gemini", platform: service.PlatformGemini, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey := &service.APIKey{Group: &service.Group{Platform: tt.platform}}
			require.Equal(t, tt.want, playgroundUsesOpenAIGateway(apiKey))
		})
	}

	require.False(t, playgroundUsesOpenAIGateway(nil))
	require.False(t, playgroundUsesOpenAIGateway(&service.APIKey{}))
}

func TestPrependUniqueModel(t *testing.T) {
	models := prependUniqueModel([]string{"gpt-5.4", "gpt-5.5", "gpt-5.4"}, "gpt-5.5")
	require.Equal(t, []string{"gpt-5.5", "gpt-5.4"}, models)
}
