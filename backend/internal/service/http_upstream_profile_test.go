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

func TestOpenAIHTTPUpstreamProfile_APIKeyStreamOnly(t *testing.T) {
	apiKey := &Account{Type: AccountTypeAPIKey}
	oauth := &Account{Type: AccountTypeOAuth}

	if got := openAIHTTPUpstreamProfile(apiKey, true); got != HTTPUpstreamProfileOpenAIAPIKeyStream {
		t.Fatalf("API-key stream profile = %q, want %q", got, HTTPUpstreamProfileOpenAIAPIKeyStream)
	}
	if got := openAIHTTPUpstreamProfile(apiKey, false); got != HTTPUpstreamProfileOpenAI {
		t.Fatalf("API-key non-stream profile = %q, want %q", got, HTTPUpstreamProfileOpenAI)
	}
	if got := openAIHTTPUpstreamProfile(oauth, true); got != HTTPUpstreamProfileOpenAI {
		t.Fatalf("OAuth stream profile = %q, want %q", got, HTTPUpstreamProfileOpenAI)
	}
}
