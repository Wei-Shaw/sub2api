package service

import (
	"context"
	"testing"
)

func TestWithHTTPUpstreamProfile_DefaultKeepsContext(t *testing.T) {
	ctx := context.Background()
	got := WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileDefault)
	if got != ctx {
		t.Fatal("default profile should not wrap context")
	}
}

func TestWithHTTPUpstreamProfile_OpenAI(t *testing.T) {
	ctx := WithHTTPUpstreamProfile(context.TODO(), HTTPUpstreamProfileOpenAI)
	if profile := HTTPUpstreamProfileFromContext(ctx); profile != HTTPUpstreamProfileOpenAI {
		t.Fatalf("expected profile %q, got %q", HTTPUpstreamProfileOpenAI, profile)
	}
}

func TestWithHTTPUpstreamProfile_OllamaAnthropic(t *testing.T) {
	ctx := WithHTTPUpstreamProfile(context.TODO(), HTTPUpstreamProfileOllamaAnthropic)
	if profile := HTTPUpstreamProfileFromContext(ctx); profile != HTTPUpstreamProfileOllamaAnthropic {
		t.Fatalf("expected profile %q, got %q", HTTPUpstreamProfileOllamaAnthropic, profile)
	}
}

func TestWithOllamaAnthropicHTTPUpstreamProfile_ScopedToTargetAccount(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    HTTPUpstreamProfile
	}{
		{name: "nil account", want: HTTPUpstreamProfileDefault},
		{
			name: "ollama anthropic api key",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					"base_url": "https://ollama.com",
				},
			},
			want: HTTPUpstreamProfileOllamaAnthropic,
		},
		{
			name: "non ollama anthropic api key",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					"base_url": "https://api.anthropic.com",
				},
			},
			want: HTTPUpstreamProfileDefault,
		},
		{
			name: "ollama openai api key",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					"base_url": "https://ollama.com",
				},
			},
			want: HTTPUpstreamProfileDefault,
		},
		{
			name: "ollama anthropic oauth",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"base_url": "https://ollama.com",
				},
			},
			want: HTTPUpstreamProfileDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := withOllamaAnthropicHTTPUpstreamProfile(context.Background(), tt.account)
			if profile := HTTPUpstreamProfileFromContext(ctx); profile != tt.want {
				t.Fatalf("expected profile %q, got %q", tt.want, profile)
			}
		})
	}
}

func TestWithHTTPUpstreamRedirectsDisabled(t *testing.T) {
	//nolint:staticcheck // Exercises the defensive nil-context fallback.
	ctx := WithHTTPUpstreamRedirectsDisabled(nil)
	if !HTTPUpstreamRedirectsDisabled(ctx) {
		t.Fatal("expected redirects to be disabled")
	}
	if HTTPUpstreamRedirectsDisabled(context.Background()) {
		t.Fatal("redirects should remain enabled by default")
	}
}
