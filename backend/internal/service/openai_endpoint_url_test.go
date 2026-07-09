package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIEndpointURLGeminiOfficialCompatibleBase(t *testing.T) {
	t.Parallel()

	baseURL := "https://generativelanguage.googleapis.com/v1beta/openai"
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/openai/models", buildOpenAIEndpointURL(baseURL, "/v1/models"))
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions", buildOpenAIEndpointURL(baseURL, "/v1/chat/completions"))
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/openai/images/generations", buildOpenAIEndpointURL(baseURL, "/v1/images/generations"))
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/openai/responses", buildOpenAIEndpointURL(baseURL, "/v1/responses"))
}
