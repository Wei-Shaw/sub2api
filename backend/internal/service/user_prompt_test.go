package service

import (
	"strings"
	"testing"
)

func TestExtractUserPromptLatestUserOnly(t *testing.T) {
	body := []byte(`{"system":"secret","messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ignore"},{"role":"user","content":[{"type":"text","text":"latest"},{"type":"image","data":"ignore"}]}]}`)
	got := ExtractUserPrompt(body, "chat_completions")
	if got == nil || *got != "latest" {
		t.Fatalf("got %v", got)
	}
}

func TestExtractUserPromptResponsesIgnoresInstructions(t *testing.T) {
	body := []byte(`{
		"instructions":"ignore",
		"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"You are Codex..."}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>...</environment_context>"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ignore"}]},
			{"type":"function_call_output","call_id":"call_1","output":"ignore"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"real user question"}]}
		]
	}`)
	got := ExtractUserPrompt(body, "responses")
	if got == nil || *got != "real user question" {
		t.Fatalf("got %v", got)
	}
}

func TestExtractUserPromptResponsesStringInput(t *testing.T) {
	body := []byte(`{"input":"hello"}`)
	got := ExtractUserPrompt(body, "responses")
	if got == nil || *got != "hello" {
		t.Fatalf("got %v", got)
	}
}

func TestExtractUserPromptResponsesRemovesCodexImageAttachmentMetadata(t *testing.T) {
	body := []byte(`{
		"input":[{
			"type":"message",
			"role":"user",
			"content":[
				{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="},
				{"type":"input_text","text":"<image name=[Image #1] path=\"D:\\\\WinTemp\\\\codex-clipboard.png\">\n</image>\n[Image #1] explain version checking"}
			]
		}]
	}`)
	got := ExtractUserPrompt(body, "responses")
	if got == nil || *got != "explain version checking" {
		t.Fatalf("got %v", got)
	}
}

func TestNormalizeUserPromptPreservesUserAuthoredImageMarkup(t *testing.T) {
	prompt := `<image src="example.png"></image>
[Image #1] is part of this example`
	got := NormalizeUserPrompt(prompt)
	if got == nil || *got != prompt {
		t.Fatalf("got %v", got)
	}
}

func TestExtractUserPromptEmbeddingsInput(t *testing.T) {
	body := []byte(`{"model":"text-embedding-3-small","input":["first","latest"]}`)
	got := ExtractUserPrompt(body, "openai_embeddings")
	if got == nil || *got != "first\nlatest" {
		t.Fatalf("got %v", got)
	}
}

func TestNormalizeUserPromptTruncatesByRunes(t *testing.T) {
	got := NormalizeUserPrompt(strings.Repeat("中", MaxStoredUserPromptRunes+10))
	if got == nil || !strings.HasSuffix(*got, userPromptTruncationMark) || len([]rune(*got)) != MaxStoredUserPromptRunes {
		t.Fatalf("unexpected result length=%d", len([]rune(*got)))
	}
}
