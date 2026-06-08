//go:build unit

package gateway

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
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
	p := newMock(domain.PlatformAnthropic, domain.PlatformAnthropic)
	r.Register(p)

	got, ok := r.Get(domain.PlatformAnthropic)
	if !ok {
		t.Fatal("expected provider to be registered")
	}
	if got.Platform() != domain.PlatformAnthropic {
		t.Fatalf("got platform %q, want %q", got.Platform(), domain.PlatformAnthropic)
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
	r.Register(newMock(domain.PlatformOpenAI, domain.PlatformOpenAI))
	r.Unregister(domain.PlatformOpenAI)

	_, ok := r.Get(domain.PlatformOpenAI)
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
	r.Register(newMock(domain.PlatformAnthropic, domain.PlatformAnthropic))
	r.Register(newMock(domain.PlatformAntigravity, domain.PlatformAnthropic, domain.PlatformGemini))
	r.Register(newMock(domain.PlatformOpenAI, domain.PlatformOpenAI))

	got := r.ForProtocol(domain.PlatformAnthropic)
	if len(got) != 2 {
		t.Fatalf("ForProtocol(anthropic): got %d providers, want 2", len(got))
	}

	got = r.ForProtocol(domain.PlatformGemini)
	if len(got) != 1 || got[0].Platform() != domain.PlatformAntigravity {
		t.Fatalf("ForProtocol(gemini): got %v, want [antigravity]", got)
	}

	got = r.ForProtocol(domain.PlatformOpenAI)
	if len(got) != 1 || got[0].Platform() != domain.PlatformOpenAI {
		t.Fatalf("ForProtocol(openai): got %v, want [openai]", got)
	}

	got = r.ForProtocol("unknown")
	if len(got) != 0 {
		t.Fatalf("ForProtocol(unknown): got %d providers, want 0", len(got))
	}
}

func TestHasProvider(t *testing.T) {
	r := NewProviderRegistry()
	r.Register(newMock(domain.PlatformAntigravity, domain.PlatformAnthropic, domain.PlatformGemini))

	tests := []struct {
		platform string
		protocol string
		want     bool
	}{
		{domain.PlatformAntigravity, domain.PlatformAnthropic, true},
		{domain.PlatformAntigravity, domain.PlatformGemini, true},
		{domain.PlatformAntigravity, domain.PlatformOpenAI, false},
		{domain.PlatformOpenAI, domain.PlatformOpenAI, false},
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
	r.Register(newMock(domain.PlatformAnthropic, domain.PlatformAnthropic))
	r.Register(newMock(domain.PlatformAnthropic, domain.PlatformAnthropic, domain.PlatformGemini))

	got, ok := r.Get(domain.PlatformAnthropic)
	if !ok {
		t.Fatal("expected provider to exist")
	}
	if len(got.Protocols()) != 2 {
		t.Fatalf("expected overwritten provider with 2 protocols, got %d",
			len(got.Protocols()))
	}
}
