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

func TestChatInputAudioCaseInsensitiveTextSibling(t *testing.T) {
	audio := `{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}`
	for _, sibling := range []string{
		`{"TYPE":"text","TEXT":"transcribe"}`,
		`{"Type":"text","Text":"transcribe"}`,
		`{"type":"text","TEXT":""}`,
	} {
		for _, audioFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/audioFirst=%t", sibling, audioFirst), func(t *testing.T) {
				content := fmt.Sprintf("[%s,%s]", sibling, audio)
				textIndex, audioIndex := 0, 1
				if audioFirst {
					content = fmt.Sprintf("[%s,%s]", audio, sibling)
					textIndex, audioIndex = 1, 0
				}
				request := &ChatCompletionsRequest{Model: "gemini-3.1-pro-high", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(content)}}}
				converted, err := ChatCompletionsToResponsesForGemini(request)
				require.NoError(t, err)
				var items []ResponsesInputItem
				require.NoError(t, json.Unmarshal(converted.Input, &items))
				require.Len(t, items, 1)
				var parts []ResponsesContentPart
				require.NoError(t, json.Unmarshal(items[0].Content, &parts))
				var expected ChatContentPart
				require.NoError(t, json.Unmarshal([]byte(sibling), &expected))
				if expected.Text == "" {
					require.Len(t, parts, 1)
					audioIndex = 0
				} else {
					require.Len(t, parts, 2)
					require.Equal(t, "input_text", parts[textIndex].Type)
					require.Equal(t, expected.Text, parts[textIndex].Text)
				}
				require.Equal(t, "input_file", parts[audioIndex].Type)
				require.Equal(t, "data:audio/wav;base64,YXVkaW8=", parts[audioIndex].FileData)
				require.Equal(t, map[int]map[int]string{0: {audioIndex: parts[audioIndex].FileData}}, converted.chatInputAudio)
				require.Equal(t, content, string(request.Messages[0].Content))
				_, err = ChatCompletionsToResponses(request)
				require.ErrorIs(t, err, ErrUnsupportedInputAudio)
			})
		}
	}
}

func TestChatInputAudioMalformedSibling(t *testing.T) {
	for _, sibling := range []string{`{"TYPE":"text","TEXT":null}`, `{"TYPE":"text","TEXT":123}`, `{"TYPE":"text"}`} {
		t.Run(sibling, func(t *testing.T) {
			content := `[` + sibling + `,{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}]`
			request := &ChatCompletionsRequest{Model: "gemini-3.1-pro-high", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(content)}}}
			converted, err := ChatCompletionsToResponsesForGemini(request)
			require.Nil(t, converted)
			require.ErrorContains(t, err, "invalid input_audio message content")
			require.Equal(t, content, string(request.Messages[0].Content))
		})
	}
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

func TestChatInputAudioDuplicateContentFields(t *testing.T) {
	audio := `{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}`
	for _, content := range []string{
		`[{"type":"input_audio","type":"text","text":"hidden","input_audio":{"data":"YXVkaW8=","format":"wav"}}]`,
		`[{"type":"input_audio","TYPE":"text","text":"hidden"}]`,
		`[{"type":"input_audio","type":123,"text":"hidden"}]`,
		`[{"type":123,"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}]`,
		`[{"TYPE":"input_audio","type":"text","text":"hidden"}]`,
		`[{"ty\u0070e":"input_audio","type":"text","text":"hidden"}]`,
		`[{"type":"text","type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}]`,
		`[{"type":"input_audio","input_audio":null,"INPUT_AUDIO":{"data":"YXVkaW8=","format":"wav"}}]`,
		`[{"type":"input_audio","input_audio":{"data":"","DATA":"YXVkaW8=","format":"wav"}}]`,
		`[{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"pcm16","FORMAT":"wav"}}]`,
		`[{"type":"text","text":"first","TEXT":"last"},` + audio + `]`,
		`[{"TYPE":"text","TEXT":"first","text":"last"},` + audio + `]`,
		`[{"TYPE":"text","TYPE":"text","TEXT":"transcribe"},` + audio + `]`,
		`[{"TYPE":"text","TEXT":"first","TEXT":"last"},` + audio + `]`,
		`[{"type":"image_url","image_url":{"url":"https://first.example/image","URL":"https://last.example/image"}},` + audio + `]`,
		`[{"type":"file","file":{"file_data":"data:application/pdf;base64,cGRm","FILE_DATA":"data:application/pdf;base64,bGFzdA=="}},` + audio + `]`,
		`[{"type":"file","file":null,"FILE":{"file_id":"file-1"}},` + audio + `]`,
	} {
		t.Run(content, func(t *testing.T) {
			request := &ChatCompletionsRequest{Model: "gemini-3.1-pro-high", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(content)}}}
			for _, convert := range []func(*ChatCompletionsRequest) (*ResponsesRequest, error){ChatCompletionsToResponses, ChatCompletionsToResponsesForGemini} {
				converted, err := convert(request)
				require.Nil(t, converted)
				require.ErrorContains(t, err, "input_audio")
			}
			require.Equal(t, content, string(request.Messages[0].Content))
		})
	}
}

func TestChatInputAudioRawRequestMasking(t *testing.T) {
	audio := `[{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}]`
	for _, fields := range []string{
		`"messages":[{"role":"user","content":` + audio + `,"content":"hidden"}]`,
		`"messages":[{"role":"user","content":` + audio + `,"CONTENT":null}]`,
		`"messages":[{"role":"user","content":"hidden","Content":` + audio + `}]`,
		`"messages":[{"role":"assistant","ROLE":"user","content":` + audio + `}]`,
		`"messages":[{"role":"user","content":` + audio + `}],"messages":[{"role":"user","content":"hidden"}]`,
		`"messages":[{"role":"user","content":` + audio + `}],"MESSAGES":null`,
		`"messages":[],"Messages":[{"role":"user","content":` + audio + `}]`,
	} {
		t.Run(fields, func(t *testing.T) {
			var request ChatCompletionsRequest
			require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.1-pro-high",`+fields+`}`), &request))
			for _, convert := range []func(*ChatCompletionsRequest) (*ResponsesRequest, error){ChatCompletionsToResponses, ChatCompletionsToResponsesForGemini} {
				converted, err := convert(&request)
				require.Nil(t, converted)
				require.ErrorContains(t, err, "input_audio")
			}
		})
	}
}

func TestChatInputAudioMaskingLastWinsReachable(t *testing.T) {
	var request ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":[{"type":"input_audio","type":"text","text":"hidden"}]}]}`), &request))
	var parts []ChatContentPart
	require.NoError(t, json.Unmarshal(request.Messages[0].Content, &parts))
	require.Equal(t, "text", parts[0].Type)
	require.Equal(t, "hidden", parts[0].Text)
	_, err := ChatCompletionsToResponsesForGemini(&request)
	require.ErrorContains(t, err, "input_audio")

	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":[{"type":"input_audio"}],"content":"hidden"}]}`), &request))
	require.JSONEq(t, `"hidden"`, string(request.Messages[0].Content))
	_, err = ChatCompletionsToResponsesForGemini(&request)
	require.ErrorContains(t, err, "input_audio")

	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":[{"type":"input_audio"}]}],"messages":[]}`), &request))
	require.Empty(t, request.Messages)
	_, err = ChatCompletionsToResponsesForGemini(&request)
	require.ErrorContains(t, err, "input_audio")
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"reset"}]}`), &request))
	_, err = ChatCompletionsToResponsesForGemini(&request)
	require.NoError(t, err)
}

func TestChatInputAudioDuplicateNegativeControls(t *testing.T) {
	for _, fields := range []string{
		`"messages":[{"role":"user","content":[{"type":"unknown","type":"text","text":"input_audio"}]}]`,
		`"messages":[{"role":"user","content":"input_audio","content":"kept"}],"messages":[{"role":"user","content":"last"}]`,
		`"messages":[{"role":"user","content":[{"type":"text","text":"{\"type\":\"input_audio\"}"}]}]`,
		`"metadata":{"type":"input_audio","type":"text"},"messages":[{"role":"user","content":"kept"}]`,
	} {
		t.Run(fields, func(t *testing.T) {
			var request ChatCompletionsRequest
			require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.1-pro-high",`+fields+`}`), &request))
			standard, err := ChatCompletionsToResponses(&request)
			require.NoError(t, err)
			gemini, err := ChatCompletionsToResponsesForGemini(&request)
			require.NoError(t, err)
			require.Equal(t, standard, gemini)
		})
	}
}

func TestChatInputAudioAliasesAndUnrelatedDuplicatesPreserved(t *testing.T) {
	var request ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"old","model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"old","content":"kept"},{"ROLE":"user","CONTENT":[{"TYPE":"input_audio","INPUT_AUDIO":{"DATA":"YXVkaW8=","FORMAT":"wav","metadata":1,"metadata":2}},{"type":"text","text":"tail","metadata":1,"metadata":2}]}]}`), &request))
	converted, err := ChatCompletionsToResponsesForGemini(&request)
	require.NoError(t, err)
	require.Contains(t, string(converted.Input), "data:audio/wav;base64,YXVkaW8=")
	require.Contains(t, string(converted.Input), "kept")
	require.Contains(t, string(converted.Input), "tail")
	_, err = ChatCompletionsToResponses(&request)
	require.ErrorIs(t, err, ErrUnsupportedInputAudio)
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"reset"}]}`), &request))
	_, err = ChatCompletionsToResponsesForGemini(&request)
	require.NoError(t, err)
}
