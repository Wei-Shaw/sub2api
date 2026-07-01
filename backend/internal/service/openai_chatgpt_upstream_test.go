package service

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIChatGPTCodexResponsesURL(t *testing.T) {
	require.Equal(t, chatgptCodexURL, buildOpenAIChatGPTCodexResponsesURL(""))
	require.Equal(t, "https://chatgpt.com/backend-api/codex/responses", buildOpenAIChatGPTCodexResponsesURL("https://chatgpt.com"))
	require.Equal(t, "http://127.0.0.1:8787/backend-api/codex/responses", buildOpenAIChatGPTCodexResponsesURL("http://127.0.0.1:8787/backend-api/codex"))
	require.Equal(t, "http://127.0.0.1:8787/backend-api/codex/responses", buildOpenAIChatGPTCodexResponsesURL("http://127.0.0.1:8787/backend-api/codex/responses"))
}

func TestShouldSetOpenAIChatGPTHostHeader(t *testing.T) {
	u, err := url.Parse("https://chatgpt.com/backend-api/codex/responses")
	require.NoError(t, err)
	require.True(t, shouldSetOpenAIChatGPTHostHeader(u))

	sidecar, err := url.Parse("http://127.0.0.1:8787/backend-api/codex/responses")
	require.NoError(t, err)
	require.False(t, shouldSetOpenAIChatGPTHostHeader(sidecar))
}
