package service

import "testing"

func TestOpenAICompatiblePlatformsIncludeOpenCode(t *testing.T) {
	if !IsOpenAICompatiblePlatform(PlatformOpenCode) {
		t.Fatalf("expected %s to be OpenAI-compatible", PlatformOpenCode)
	}
	if got := DefaultOpenAICompatibleBaseURL(PlatformOpenCode); got != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("unexpected OpenCode base URL: %q", got)
	}
}

func TestOpenAIChatCompletionsOnlyPlatformsIncludeOpenCode(t *testing.T) {
	if !IsOpenAIChatCompletionsOnlyPlatform(PlatformOpenCode) {
		t.Fatalf("expected %s to use Chat Completions upstream", PlatformOpenCode)
	}
	account := &Account{Platform: PlatformOpenCode, Type: AccountTypeAPIKey}
	if !ShouldUseOpenAICompatibleChatCompletions(account) {
		t.Fatalf("expected %s APIKey account to use Chat Completions upstream", PlatformOpenCode)
	}
}
