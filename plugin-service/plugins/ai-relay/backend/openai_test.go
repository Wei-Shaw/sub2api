package backend

import "testing"

func TestOpenAIAdapterIsTransparent(t *testing.T) {
	adapter := NewOpenAIAdapter()
	if got := adapter.Platform(); got != "openai" {
		t.Fatalf("Platform() = %q", got)
	}
	if got := adapter.Descriptor(); got != (PlatformDescriptor{Key: "openai", DisplayName: "OpenAI", Protocol: "transparent", DefaultBaseURL: "https://api.openai.com/v1"}) {
		t.Fatalf("Descriptor() = %#v", got)
	}
}
