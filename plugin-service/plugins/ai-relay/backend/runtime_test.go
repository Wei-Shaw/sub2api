package backend

import "testing"

func TestResolvePublicBaseURLDefaultsToLocalHost(t *testing.T) {
	t.Setenv("PLUGIN_AI_RELAY_PUBLIC_BASE_URL", "")
	t.Setenv("PLUGIN_SERVER_PORT", "18091")

	if got := resolvePublicBaseURL(); got != "http://127.0.0.1:18091" {
		t.Fatalf("resolvePublicBaseURL() = %q, want %q", got, "http://127.0.0.1:18091")
	}
}

func TestResolvePublicBaseURLUsesExplicitValue(t *testing.T) {
	t.Setenv("PLUGIN_AI_RELAY_PUBLIC_BASE_URL", "http://plugin-server:19091/")

	if got := resolvePublicBaseURL(); got != "http://plugin-server:19091" {
		t.Fatalf("resolvePublicBaseURL() = %q, want %q", got, "http://plugin-server:19091")
	}
}

func TestResolvePublicBaseURLFallsBackForInvalidPort(t *testing.T) {
	t.Setenv("PLUGIN_AI_RELAY_PUBLIC_BASE_URL", "")
	t.Setenv("PLUGIN_SERVER_PORT", "invalid")

	if got := resolvePublicBaseURL(); got != "http://127.0.0.1:8091" {
		t.Fatalf("resolvePublicBaseURL() = %q, want %q", got, "http://127.0.0.1:8091")
	}
}
