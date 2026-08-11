//go:build unit

package service

import (
	"encoding/json"
	"testing"
)

func TestAllowedMonitorEndpointPath(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		path     string
		want     bool
	}{
		{name: "origin for OpenAI", provider: MonitorProviderOpenAI, path: "", want: true},
		{name: "origin for Gemini", provider: MonitorProviderGemini, path: "/", want: true},
		{name: "Antigravity route for Gemini", provider: MonitorProviderGemini, path: "/antigravity", want: true},
		{name: "Antigravity route for Anthropic", provider: MonitorProviderAnthropic, path: "/antigravity", want: true},
		{name: "Antigravity route with trailing slash", provider: MonitorProviderGemini, path: "/antigravity/", want: true},
		{name: "Antigravity route for OpenAI", provider: MonitorProviderOpenAI, path: "/antigravity", want: false},
		{name: "arbitrary Gemini path", provider: MonitorProviderGemini, path: "/v1beta", want: false},
		{name: "malformed Antigravity route", provider: MonitorProviderGemini, path: "/antigravity//", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAllowedMonitorEndpointPath(tt.provider, tt.path); got != tt.want {
				t.Fatalf("isAllowedMonitorEndpointPath(%q, %q) = %v, want %v", tt.provider, tt.path, got, tt.want)
			}
		})
	}
}

func TestApplyMonitorUpdateRejectsIncompatibleAntigravityRoute(t *testing.T) {
	provider := MonitorProviderOpenAI
	existing := &ChannelMonitor{
		Provider:        MonitorProviderGemini,
		Endpoint:        "https://example.com/antigravity",
		PrimaryModel:    "gemini-2.5-flash",
		IntervalSeconds: 60,
	}

	if err := applyMonitorUpdate(existing, ChannelMonitorUpdateParams{Provider: &provider}); err == nil {
		t.Fatal("provider-only update must reject a stale Antigravity route")
	}
}

func TestJoinURLPreservesAntigravityRoutePrefix(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "Gemini",
			path: "/v1beta/models/gemini-x:generateContent",
			want: "https://example.com/antigravity/v1beta/models/gemini-x:generateContent",
		},
		{
			name: "Claude",
			path: "/v1/messages",
			want: "https://example.com/antigravity/v1/messages",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinURL("https://example.com/antigravity", tt.path); got != tt.want {
				t.Fatalf("joinURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGeminiMonitorBodyIncludesExplicitUserRole(t *testing.T) {
	adapter := providerAdapters[MonitorProviderGemini]
	body, err := adapter.buildBody("gemini-3.6-flash", "Reply with only 7.")
	if err != nil {
		t.Fatalf("buildBody() error = %v", err)
	}

	var payload struct {
		Contents []struct {
			Role string `json:"role"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(payload.Contents) != 1 {
		t.Fatalf("contents length = %d, want 1", len(payload.Contents))
	}
	if payload.Contents[0].Role != "user" {
		t.Fatalf("contents[0].role = %q, want user", payload.Contents[0].Role)
	}
}
