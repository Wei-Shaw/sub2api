package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// #7088: response_format json_object must reach Gemini as responseMimeType.
func TestBuildAntigravityCompatGeminiBody_ResponseFormatJSON(t *testing.T) {
	svc := &AntigravityGatewayService{}
	claudeBody := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	original := []byte(`{"model":"gemini-3.8-flash","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_object"}}`)
	body, err := svc.buildAntigravityCompatGeminiBody(context.Background(), claudeBody, nil, "project-1", "gemini-3.8-flash", original)
	require.NoError(t, err)
	var wrapped map[string]any
	require.NoError(t, json.Unmarshal(body, &wrapped))
	genCfg, ok := wrapped["request"].(map[string]any)["generationConfig"].(map[string]any)
	require.True(t, ok, "generationConfig must exist")
	require.Equal(t, "application/json", genCfg["responseMimeType"])

	plain := []byte(`{"model":"gemini-3.8-flash","messages":[{"role":"user","content":"hi"}]}`)
	body2, err := svc.buildAntigravityCompatGeminiBody(context.Background(), claudeBody, nil, "project-1", "gemini-3.8-flash", plain)
	require.NoError(t, err)
	var wrapped2 map[string]any
	require.NoError(t, json.Unmarshal(body2, &wrapped2))
	if gen2, ok := wrapped2["request"].(map[string]any)["generationConfig"].(map[string]any); ok {
		require.NotEqual(t, "application/json", gen2["responseMimeType"])
	}
}
