package apicompat

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicRequestBridges_ImageSources(t *testing.T) {
	const imageURL = "https://example.com/screenshot.png?version=1&format=png"
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jkXcAAAAASUVORK5CYII="
	for _, source := range []struct {
		name string
		raw  string
		want string
	}{
		{"url", `{"type":"url","url":"` + imageURL + `"}`, imageURL},
		{"base64_control", `{"type":"base64","media_type":"image/png","data":"` + png + `"}`, "data:image/png;base64," + png},
	} {
		image := `{"type":"image","source":` + source.raw + `}`
		for _, context := range []struct {
			name     string
			messages string
		}{
			{"user_image", `[{"role":"user","content":[` + image + `,{"type":"text","text":"Describe this image."}]}]`},
			{"tool_result_image", `[
				{"role":"user","content":"Inspect the screenshot."},
				{"role":"assistant","content":[{"type":"tool_use","id":"toolu_image","name":"inspect","input":{}}]},
				{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_image","content":[` + image + `,{"type":"text","text":"The screenshot is attached."}]}]}
			]`},
		} {
			raw := fmt.Sprintf(`{"model":"gpt-4o","max_tokens":1024,"tools":[{"name":"inspect","input_schema":{"type":"object","properties":{}}}],"messages":%s}`, context.messages)
			t.Run(source.name+"/"+context.name+"/chat_completions", func(t *testing.T) {
				var req AnthropicRequest
				require.NoError(t, json.Unmarshal([]byte(raw), &req))
				out, err := AnthropicToChatCompletionsRequest(&req)
				require.NoError(t, err)
				wire, err := json.Marshal(out)
				require.NoError(t, err)
				var upstream ChatCompletionsRequest
				require.NoError(t, json.Unmarshal(wire, &upstream))
				var got []string
				for _, msg := range upstream.Messages {
					var parts []ChatContentPart
					if json.Unmarshal(msg.Content, &parts) != nil {
						continue
					}
					for _, part := range parts {
						if part.Type == "image_url" && part.ImageURL != nil {
							got = append(got, part.ImageURL.URL)
						}
					}
				}
				require.Equal(t, []string{source.want}, got, "the image must reach the upstream as image_url")
			})
			t.Run(source.name+"/"+context.name+"/responses", func(t *testing.T) {
				var req AnthropicRequest
				require.NoError(t, json.Unmarshal([]byte(raw), &req))
				out, err := AnthropicToResponses(&req)
				require.NoError(t, err)
				wire, err := json.Marshal(out)
				require.NoError(t, err)
				var upstream ResponsesRequest
				require.NoError(t, json.Unmarshal(wire, &upstream))
				var items []ResponsesInputItem
				require.NoError(t, json.Unmarshal(upstream.Input, &items))
				var got []string
				for _, item := range items {
					if len(item.Content) == 0 {
						continue
					}
					var parts []ResponsesContentPart
					require.NoError(t, json.Unmarshal(item.Content, &parts))
					for _, part := range parts {
						if part.Type == "input_image" {
							got = append(got, part.ImageURL)
						}
					}
				}
				require.Equal(t, []string{source.want}, got, "the image must reach the upstream as input_image")
			})
		}
	}
}
