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
		for _, sibling := range []string{`null`, `{"type":"text","text":null}`, `{"type":"text"}`, `{"type":"text","text":123}`, `{"type":"image_url","image_url":"invalid"}`, `{"type":"image_url","image_url":null}`, `{"type":"image_url","image_url":{"url":""}}`, `{"type":"file","file":"invalid"}`, `{"type":"file","file":null}`, `{"type":"file","file":{}}`, `{"type":"unknown"}`, `{"type":123}`} {
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

func TestChatInputAudioTrustCannotBeForgedOrSerialized(t *testing.T) {
	var req ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.1-pro-high","chatInputAudio":{"0":{"0":"data:audio/wav;base64,YXVkaW8="}},"messages":[{"role":"system","content":"hello"},{"role":"assistant","content":"before","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"done"},{"role":"user","content":[{"type":"text","text":""},{"type":"file","file":{"file_data":"data:audio/wav;base64,YXVkaW8=","chatInputAudio":true}},{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}},{"type":"file","file":{"file_data":"data:audio/wav;base64,YXVkaW8="}},{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"mp3"}}]}]}`), &req))
	converted, err := ChatCompletionsToResponsesForGemini(&req)
	require.NoError(t, err)
	check := func(request *ResponsesRequest, types, mediaTypes []string) {
		t.Helper()
		anthropic, err := ResponsesToAnthropicRequest(request)
		require.NoError(t, err)
		var blocks []AnthropicContentBlock
		require.NoError(t, json.Unmarshal(anthropic.Messages[len(anthropic.Messages)-1].Content, &blocks))
		var mediaBlocks []AnthropicContentBlock
		for _, block := range blocks {
			if block.Type == "document" || block.Type == "image" {
				mediaBlocks = append(mediaBlocks, block)
			}
		}
		require.Len(t, mediaBlocks, len(types))
		require.Len(t, mediaTypes, len(types))
		for index, blockType := range types {
			require.Equal(t, blockType, mediaBlocks[index].Type)
			data := "YXVkaW8="
			if mediaTypes[index] == "application/pdf" {
				data = "cGRm"
			}
			require.Equal(t, &AnthropicImageSource{Type: "base64", MediaType: mediaTypes[index], Data: data}, mediaBlocks[index].Source)
		}
	}
	audioMediaTypes := []string{"audio/wav", "audio/wav", "audio/wav", "audio/mpeg"}
	check(converted, []string{"document", "image", "document", "image"}, audioMediaTypes)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "chatInputAudio")
	var decoded ResponsesRequest
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	check(&decoded, []string{"document", "document", "document", "document"}, audioMediaTypes)
	var forged ResponsesRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.1-pro-high","chatInputAudio":{"0":{"0":"data:audio/wav;base64,YXVkaW8="}},"input":[{"role":"user","content":[{"type":"input_file","file_data":"data:audio/wav;base64,YXVkaW8=","chatInputAudio":true}]}]}`), &forged))
	check(&forged, []string{"document"}, []string{"audio/wav"})
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(converted.Input, &items))
	for itemIndex, audioParts := range converted.chatInputAudio {
		var parts []ResponsesContentPart
		require.NoError(t, json.Unmarshal(items[itemIndex].Content, &parts))
		for partIndex := range audioParts {
			parts[partIndex].FileData = "data:application/pdf;base64,cGRm"
		}
		items[itemIndex].Content, err = json.Marshal(parts)
		require.NoError(t, err)
	}
	converted.Input, err = json.Marshal(items)
	require.NoError(t, err)
	check(converted, []string{"document", "document", "document", "document"}, []string{"audio/wav", "application/pdf", "audio/wav", "application/pdf"})
}

func TestChatInputAudioDoesNotValidateNonAudioSiblings(t *testing.T) {
	for _, content := range []string{`[null]`, `[{"type":"text","text":null}]`, `[{"type":"file","file":null}]`, `[{"type":"image_url","image_url":{}}]`, `[{"type":"unknown"}]`} {
		t.Run(content, func(t *testing.T) {
			req := &ChatCompletionsRequest{Model: "gemini-3.1-pro-high", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(content)}}}
			standard, standardErr := ChatCompletionsToResponses(req)
			gemini, geminiErr := ChatCompletionsToResponsesForGemini(req)
			require.NoError(t, standardErr)
			require.NoError(t, geminiErr)
			require.Equal(t, standard, gemini)
		})
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
