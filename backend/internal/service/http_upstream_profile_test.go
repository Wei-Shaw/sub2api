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

func TestWithHTTPUpstreamProfile_OpenAICompact(t *testing.T) {
	ctx := WithHTTPUpstreamProfile(context.TODO(), HTTPUpstreamProfileOpenAICompact)
	if profile := HTTPUpstreamProfileFromContext(ctx); profile != HTTPUpstreamProfileOpenAICompact {
		t.Fatalf("expected profile %q, got %q", HTTPUpstreamProfileOpenAICompact, profile)
	}
	if !IsHTTPUpstreamProfileOpenAI(HTTPUpstreamProfileOpenAICompact) {
		t.Fatal("compact profile should use OpenAI upstream policy")
	}
	if !IsHTTPUpstreamProfileCompact(HTTPUpstreamProfileOpenAICompact) {
		t.Fatal("compact profile should be marked compact")
	}
}
