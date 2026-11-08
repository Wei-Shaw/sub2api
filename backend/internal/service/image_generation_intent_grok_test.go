package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsImageGenerationIntentForPlatform_GrokCodexDeclarations(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "top-level image_gen namespace is passive",
			body: `{"model":"grok-4.5","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"tool_choice":"auto","input":"write code"}`,
		},
		{
			name: "responses lite additional_tools image_gen is passive",
			body: `{"model":"grok-4.5","tool_choice":"auto","input":[{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}]},{"type":"message","role":"user","content":"write code"}]}`,
		},
		{
			name: "legacy adapter flattened image_gen function declaration is passive",
			body: `{"model":"grok-4.5","tools":[{"type":"function","name":"image_gen.imagegen"}],"input":"write code"}`,
		},
		{
			name: "native image generation declaration remains explicit",
			body: `{"model":"grok-4.5","tools":[{"type":"image_generation"}],"input":"draw a cat"}`,
			want: true,
		},
		{
			name: "native image generation tool choice is explicit",
			body: `{"model":"grok-4.5","tools":[{"type":"image_generation"}],"tool_choice":{"type":"image_generation"},"input":"draw a cat"}`,
			want: true,
		},
		{
			name: "image_gen namespace tool choice is explicit",
			body: `{"model":"grok-4.5","tools":[{"type":"namespace","name":"image_gen"}],"tool_choice":{"type":"namespace","name":"image_gen"},"input":"draw a cat"}`,
			want: true,
		},
		{
			name: "flattened image_gen function tool choice is explicit",
			body: `{"model":"grok-4.5","tools":[{"type":"function","name":"image_gen.imagegen"}],"tool_choice":{"type":"function","name":"image_gen.imagegen"},"input":"draw a cat"}`,
			want: true,
		},
		{
			name: "wrapped image_gen function tool choice is explicit",
			body: `{"model":"grok-4.5","tools":[{"type":"function","name":"image_gen.imagegen"}],"tool_choice":{"tool":{"type":"function","name":"image_gen.imagegen"}},"input":"draw a cat"}`,
			want: true,
		},
		{
			name: "vision input is not image generation",
			body: `{"model":"grok-4.5","input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}]}`,
		},
		{
			name: "ordinary function declaration is not image generation",
			body: `{"model":"grok-4.5","tools":[{"type":"function","name":"lookup"}],"tool_choice":"auto","input":"write code"}`,
		},
		{
			name: "image_gen function call history is not current intent",
			body: `{"model":"grok-4.5","input":[{"type":"function_call","namespace":"image_gen","name":"imagegen","arguments":"{}"}]}`,
		},
		{
			name: "image generation call history is not current intent",
			body: `{"model":"grok-4.5","input":[{"type":"image_generation_call","id":"ig_1"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsImageGenerationIntentForPlatform(
				openAIResponsesEndpoint,
				"grok-4.5",
				[]byte(tt.body),
				PlatformGrok,
			))
		})
	}
}

func TestIsImageGenerationIntentForPlatform_GrokPreservesHardSignals(t *testing.T) {
	require.True(t, IsImageGenerationIntentForPlatform(
		"/v1/images/generations",
		"grok-4.5",
		[]byte(`{"input":"draw"}`),
		PlatformGrok,
	))
	require.True(t, IsImageGenerationIntentForPlatform(
		openAIResponsesEndpoint,
		"gpt-image-2",
		[]byte(`{"input":"draw"}`),
		PlatformGrok,
	))
	require.True(t, IsImageGenerationIntentForPlatform(
		openAIResponsesEndpoint,
		"grok-4.5",
		[]byte(`{"model":"gpt-image-2","input":"draw"}`),
		PlatformGrok,
	))
}

func TestIsImageGenerationIntentForPlatform_OtherPlatformsKeepDeclarationSemantics(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"input":"write code"}`)

	require.True(t, IsImageGenerationIntentForPlatform(openAIResponsesEndpoint, "gpt-5.5", body, PlatformOpenAI))
	require.True(t, IsImageGenerationIntentForPlatform(openAIResponsesEndpoint, "gpt-5.5", body, PlatformAnthropic))
}
