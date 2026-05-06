//go:build unit

package gateway

import (
	"context"
	"net/http"
	"testing"
)

// mockProvider implements GatewayProvider for testing.
type mockProvider struct {
	platform  string
	protocols []string
}

func (m *mockProvider) Platform() string    { return m.platform }
func (m *mockProvider) Protocols() []string { return m.protocols }

func (m *mockProvider) Forward(_ context.Context, _ http.ResponseWriter, _ *ForwardRequest) (*ForwardResult, error) {
	return nil, nil
}

func (m *mockProvider) ShouldFailover(_ context.Context, _ *ForwardRequest, _ error) bool {
	return false
}

func newMock(platform string, protocols ...string) *mockProvider {
	return &mockProvider{platform: platform, protocols: protocols}
}

func TestRegisterAndGet(t *testing.T) {
	r := NewProviderRegistry()
	p := newMock("anthropic", "anthropic")
	r.Register(p)

	got, ok := r.Get("anthropic")
	if !ok {
		t.Fatal("expected provider to be registered")
	}
	if got.Platform() != "anthropic" {
		t.Fatalf("got platform %q, want %q", got.Platform(), "anthropic")
	}
}

func TestGetMissing(t *testing.T) {
	r := NewProviderRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected no provider for unregistered platform")
	}
}

func TestUnregister(t *testing.T) {
	r := NewProviderRegistry()
	r.Register(newMock("openai", "openai"))
	r.Unregister("openai")

	_, ok := r.Get("openai")
	if ok {
		t.Fatal("expected provider to be removed after Unregister")
	}
}

func TestUnregisterNoop(t *testing.T) {
	r := NewProviderRegistry()
	r.Unregister("nonexistent") // must not panic
}

func TestForProtocol(t *testing.T) {
	r := NewProviderRegistry()
	r.Register(newMock("anthropic", "anthropic"))
	r.Register(newMock("antigravity", "anthropic", "gemini"))
	r.Register(newMock("openai", "openai"))

	got := r.ForProtocol("anthropic")
	if len(got) != 2 {
		t.Fatalf("ForProtocol(anthropic): got %d providers, want 2", len(got))
	}

	got = r.ForProtocol("gemini")
	if len(got) != 1 || got[0].Platform() != "antigravity" {
		t.Fatalf("ForProtocol(gemini): got %v, want [antigravity]", got)
	}

	got = r.ForProtocol("openai")
	if len(got) != 1 || got[0].Platform() != "openai" {
		t.Fatalf("ForProtocol(openai): got %v, want [openai]", got)
	}

	got = r.ForProtocol("unknown")
	if len(got) != 0 {
		t.Fatalf("ForProtocol(unknown): got %d providers, want 0", len(got))
	}
}

func TestHasProvider(t *testing.T) {
	r := NewProviderRegistry()
	r.Register(newMock("antigravity", "anthropic", "gemini"))

	tests := []struct {
		platform string
		protocol string
		want     bool
	}{
		{"antigravity", "anthropic", true},
		{"antigravity", "gemini", true},
		{"antigravity", "openai", false},
		{"openai", "openai", false},
	}

	for _, tt := range tests {
		got := r.HasProvider(tt.platform, tt.protocol)
		if got != tt.want {
			t.Errorf("HasProvider(%q, %q) = %v, want %v",
				tt.platform, tt.protocol, got, tt.want)
		}
	}
}

func TestRegisterOverwrite(t *testing.T) {
	r := NewProviderRegistry()
	r.Register(newMock("anthropic", "anthropic"))
	r.Register(newMock("anthropic", "anthropic", "gemini"))

	got, ok := r.Get("anthropic")
	if !ok {
		t.Fatal("expected provider to exist")
	}
	if len(got.Protocols()) != 2 {
		t.Fatalf("expected overwritten provider with 2 protocols, got %d",
			len(got.Protocols()))
	}
}
