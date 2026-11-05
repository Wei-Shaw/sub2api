//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAntigravityConnectionTestModel(t *testing.T) {
	t.Parallel()

	require.Equal(t, "claude-sonnet-4-6", antigravityConnectionTestModel(""))
	require.Equal(t, "gemini-3.1-pro-preview", antigravityConnectionTestModel("gemini-3.1-pro-preview"))
}

func TestAntigravityBuildGeminiTestRequest_IncludesValidatedToolConfig(t *testing.T) {
	t.Parallel()

	svc := &AntigravityGatewayService{}
	raw, err := svc.buildGeminiTestRequestWithPrompt("proj-123", "gemini-3.8-flash-high", "")
	require.NoError(t, err)

	var v1Req map[string]any
	require.NoError(t, json.Unmarshal(raw, &v1Req))
	require.Equal(t, "proj-123", v1Req["project"])
	require.Equal(t, "gemini-3.8-flash-high", v1Req["model"])

	inner, ok := v1Req["request"].(map[string]any)
	require.True(t, ok)
	toolCfg, ok := inner["toolConfig"].(map[string]any)
	require.True(t, ok, "expected toolConfig to be present in Gemini test request")
	fcCfg, ok := toolCfg["functionCallingConfig"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "VALIDATED", fcCfg["mode"])
}

func TestAntigravityBuildGeminiTestRequestWithPrompt_ImageModel(t *testing.T) {
	t.Parallel()

	svc := &AntigravityGatewayService{}
	raw, err := svc.buildGeminiTestRequestWithPrompt("proj-123", "gemini-3.1-flash-image", "a cute cat")
	require.NoError(t, err)

	var v1Req map[string]any
	require.NoError(t, json.Unmarshal(raw, &v1Req))
	inner, ok := v1Req["request"].(map[string]any)
	require.True(t, ok)

	genCfg, ok := inner["generationConfig"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"TEXT", "IMAGE"}, genCfg["responseModalities"])

	contents, ok := inner["contents"].([]any)
	require.True(t, ok)
	require.Len(t, contents, 1)
	firstContent := contents[0].(map[string]any)
	parts := firstContent["parts"].([]any)
	require.Equal(t, "a cute cat", parts[0].(map[string]any)["text"])
}

func TestExtractContentFromSSEResponse_ExtractsTextAndInlineImages(t *testing.T) {
	t.Parallel()

	sse := "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Here is your image\"},{\"inlineData\":{\"mimeType\":\"image/png\",\"data\":\"iVBORw0KGgo=\"}}]}}]}}\n\n"
	text, images := extractContentFromSSEResponse([]byte(sse))
	require.Equal(t, "Here is your image", text)
	require.Len(t, images, 1)
	require.Equal(t, "image/png", images[0].MimeType)
	require.Equal(t, "data:image/png;base64,iVBORw0KGgo=", images[0].ImageURL)
}

func TestAntigravityShouldUseGeminiTestBuilder_UsesMappedModel(t *testing.T) {
	t.Parallel()

	require.True(t, shouldUseAntigravityGeminiTestRequest("custom-alias", "gemini-2.5-flash"))
	require.True(t, shouldUseAntigravityGeminiTestRequest("gemini-3.1-pro", "gemini-pro-agent"))
	require.False(t, shouldUseAntigravityGeminiTestRequest("gemini-alias", "claude-sonnet-4-6"))
	require.False(t, shouldUseAntigravityGeminiTestRequest("claude-sonnet-4-6", "claude-sonnet-4-6"))
}
