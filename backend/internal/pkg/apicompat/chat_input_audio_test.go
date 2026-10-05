package apicompat

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatInputAudioProviderScope(t *testing.T) {
	for _, role := range []string{"user", "assistant", "system", "developer", "tool"} {
		t.Run(role, func(t *testing.T) {
			req := &ChatCompletionsRequest{Model: "gemini-3.1-pro-high", Messages: []ChatMessage{{Role: role, Content: json.RawMessage(`[{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}]`)}}}
			_, err := ChatCompletionsToResponses(req)
			require.ErrorIs(t, err, ErrUnsupportedInputAudio)
			converted, err := ChatCompletionsToResponsesForGemini(req)
			if role != "user" {
				require.ErrorIs(t, err, ErrUnsupportedInputAudio)
				return
			}
			require.NoError(t, err)
			require.Contains(t, string(converted.Input), `"type":"input_file"`)
			require.Contains(t, string(converted.Input), "data:audio/wav;base64,YXVkaW8=")
			require.NotContains(t, string(converted.Input), "input_image")
			require.Contains(t, string(req.Messages[0].Content), "input_audio")
		})
	}
}

func TestChatInputAudioMalformedSibling(t *testing.T) {
	for _, role := range []string{"user", "assistant", "system", "developer", "tool", "function", "unknown"} {
		for _, sibling := range []string{`{"type":"text","text":123}`, `{"type":"image_url","image_url":"invalid"}`, `{"type":"file","file":"invalid"}`, `{"type":123}`} {
			t.Run(role+"/"+sibling, func(t *testing.T) {
				for _, audioFirst := range []bool{false, true} {
					audio := `{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}`
					content := fmt.Sprintf("[%s,%s]", sibling, audio)
					if audioFirst {
						content = fmt.Sprintf("[%s,%s]", audio, sibling)
					}
					req := &ChatCompletionsRequest{Model: "gemini-3.8-flash", Messages: []ChatMessage{{Role: role, Content: json.RawMessage(content)}}}
					_, err := ChatCompletionsToResponses(req)
					require.ErrorIs(t, err, ErrUnsupportedInputAudio)
					_, err = ChatCompletionsToResponsesForGemini(req)
					if role != "user" {
						require.ErrorIs(t, err, ErrUnsupportedInputAudio)
					} else {
						require.ErrorContains(t, err, "input_audio")
					}
					require.Equal(t, content, string(req.Messages[0].Content))
				}
			})
		}
	}
}

func TestChatInputAudioDoesNotChangeOtherParts(t *testing.T) {
	req := &ChatCompletionsRequest{Model: "gemini-3.1-pro-high", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:audio/ogg;base64,YXVkaW8="}},{"type":"file","file":{"file_data":"data:application/pdf;base64,cGRm"}},{"type":"unknown"}]`)}}}
	standard, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)
	gemini, err := ChatCompletionsToResponsesForGemini(req)
	require.NoError(t, err)
	require.Equal(t, standard, gemini)
}
