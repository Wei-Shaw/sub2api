package service

import "testing"

// TestExtractContentModerationInput_GatewayProtocols verifies that the raw
// gateway protocol strings (as set by the core GatewayPipeline and forwarded on
// ContentCheckRequest.Protocol) each route to the correct extraction branch.
// This guards the gateway↔plugin protocol contract: a mismatch here means the
// moderation check silently extracts the wrong (or no) content.
func TestExtractContentModerationInput_GatewayProtocols(t *testing.T) {
	anthropicBody := []byte(`{"messages":[{"role":"user","content":"anthropic-text"}]}`)
	chatBody := []byte(`{"messages":[{"role":"user","content":"chat-text"}]}`)
	responsesBody := []byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"responses-text"}]}]}`)
	geminiBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"gemini-text"}]}]}`)
	imagesBody := []byte(`{"prompt":"images-text"}`)

	cases := []struct {
		name     string
		protocol string
		body     []byte
		wantText string
	}{
		{"anthropic", gatewayProtocolAnthropic, anthropicBody, "anthropic-text"},
		{"anthropic_via_openai", gatewayProtocolAnthropicViaOpenAI, anthropicBody, "anthropic-text"},
		{"chat_completions", gatewayProtocolChatCompletions, chatBody, "chat-text"},
		{"responses", gatewayProtocolResponses, responsesBody, "responses-text"},
		{"openai", gatewayProtocolOpenAI, responsesBody, "responses-text"},
		{"gemini", gatewayProtocolGemini, geminiBody, "gemini-text"},
		{"images", gatewayProtocolImages, imagesBody, "images-text"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractContentModerationInput(tc.protocol, tc.body)
			if got.Text != tc.wantText {
				t.Fatalf("protocol %q: got text %q, want %q", tc.protocol, got.Text, tc.wantText)
			}
		})
	}
}

// TestExtractContentModerationInput_AnthropicProtocolUsesAnthropicExtraction
// confirms the anthropic protocol uses the Anthropic-specific extractor:
// a system-reminder-only user message must be skipped (the chat/responses
// extractors would not apply this Anthropic rule), proving anthropic does not
// fall through to the default branch.
func TestExtractContentModerationInput_AnthropicProtocolUsesAnthropicExtraction(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"<system-reminder>ignore me</system-reminder>"}]}`)

	got := ExtractContentModerationInput(gatewayProtocolAnthropic, body)
	if got.Text != "" {
		t.Fatalf("anthropic system-reminder should be skipped, got %q", got.Text)
	}
}

// TestExtractContentModerationInput_ImagesProtocolExtractsPromptAndImages
// confirms the images protocol extracts the prompt and image references, which
// the default branch would miss (it has no "prompt"/"images" handling). Images
// supplied as content objects are routed into Images, proving the dedicated
// images branch ran.
func TestExtractContentModerationInput_ImagesProtocolExtractsPromptAndImages(t *testing.T) {
	body := []byte(`{"prompt":"draw a cat","images":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}`)

	got := ExtractContentModerationInput(gatewayProtocolImages, body)
	if got.Text != "draw a cat" {
		t.Fatalf("images prompt: got %q, want %q", got.Text, "draw a cat")
	}
	if len(got.Images) != 1 || got.Images[0] != "https://example.com/a.png" {
		t.Fatalf("images: got %v, want one https image", got.Images)
	}
}

// TestResolveModerationProtocol_CanonicalAndUnknown confirms canonical plugin
// constants resolve to themselves and unknown protocols fall back to the
// default (empty) branch.
func TestResolveModerationProtocol_CanonicalAndUnknown(t *testing.T) {
	if got := resolveModerationProtocol(ContentModerationProtocolOpenAIChat); got != ContentModerationProtocolOpenAIChat {
		t.Fatalf("canonical chat: got %q", got)
	}
	if got := resolveModerationProtocol("  ANTHROPIC  "); got != ContentModerationProtocolAnthropicMessages {
		t.Fatalf("case/space-insensitive anthropic: got %q", got)
	}
	if got := resolveModerationProtocol("totally-unknown"); got != "" {
		t.Fatalf("unknown protocol should map to empty, got %q", got)
	}
}
