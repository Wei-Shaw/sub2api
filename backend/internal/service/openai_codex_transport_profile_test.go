package service

import (
	"context"
	"testing"
)

func TestResolveCodexTransportProfile(t *testing.T) {
	tests := []struct {
		name string
		h2   bool
		ws   bool
	}{
		{CodexTransportProfileCLI, true, true},
		{CodexTransportProfileHTTP2, true, false},
		{CodexTransportProfileWebSocket, true, true},
		{CodexTransportProfileCompatibility, false, false},
	}
	for _, tt := range tests {
		profile := ResolveCodexTransportProfile(tt.name)
		if profile.ID != tt.name || profile.HTTP2Preferred != tt.h2 || profile.WebSocketPreferred != tt.ws {
			t.Fatalf("ResolveCodexTransportProfile(%q) = %#v", tt.name, profile)
		}
		if profile.TLSFingerprintProfile != "" {
			t.Fatalf("profile %q must not claim an unverified TLS fingerprint", tt.name)
		}
	}
}

func TestCodexTransportProfileContext(t *testing.T) {
	profile := ResolveCodexTransportProfile(CodexTransportProfileWebSocket)
	ctx := WithCodexTransportProfile(context.Background(), profile)
	got := CodexTransportProfileFromContext(ctx)
	if got.ID != profile.ID || !got.WebSocketPreferred {
		t.Fatalf("context profile = %#v", got)
	}
}
