package repository

import (
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestResolveProtocolModeWithCodexCompatibilityForcesHTTP1(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIHTTP2.Enabled = true
	svc, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
	if !ok {
		t.Fatal("NewHTTPUpstream returned an unexpected implementation")
	}

	got := svc.resolveProtocolModeWithCodexProfile(
		service.HTTPUpstreamProfileOpenAI,
		"direct",
		nil,
		service.ResolveCodexTransportProfile(service.CodexTransportProfileCompatibility),
		true,
	)
	if got != upstreamProtocolModeOpenAIH1 {
		t.Fatalf("compatibility protocol mode = %q, want %q", got, upstreamProtocolModeOpenAIH1)
	}
}

func TestResolveProtocolModeWithCodexHTTP2KeepsHTTP2Preference(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIHTTP2.Enabled = true
	svc, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
	if !ok {
		t.Fatal("NewHTTPUpstream returned an unexpected implementation")
	}
	proxy, err := url.Parse("http://proxy.example:8080")
	if err != nil {
		t.Fatal(err)
	}

	got := svc.resolveProtocolModeWithCodexProfile(
		service.HTTPUpstreamProfileOpenAI,
		proxy.String(),
		proxy,
		service.ResolveCodexTransportProfile(service.CodexTransportProfileHTTP2),
		true,
	)
	if got != upstreamProtocolModeOpenAIH2 {
		t.Fatalf("http2 protocol mode = %q, want %q", got, upstreamProtocolModeOpenAIH2)
	}
}
