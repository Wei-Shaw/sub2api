package repository

import (
	"fmt"
	"testing"

	servicepkg "github.com/Wei-Shaw/sub2api/internal/service"
)

func TestOpenAIHTTP2FallbackStateCacheIsBounded(t *testing.T) {
	service := &httpUpstreamService{openAIHTTP2Fallbacks: make(map[string]*openAIHTTP2FallbackState)}
	for i := 0; i < openAIHTTP2FallbackMaxEntries+100; i++ {
		service.getOrCreateOpenAIHTTP2FallbackState(fmt.Sprintf("http://proxy-%d.example", i))
	}

	service.openAIHTTP2FallbackMu.Lock()
	count := len(service.openAIHTTP2Fallbacks)
	service.openAIHTTP2FallbackMu.Unlock()
	if count != openAIHTTP2FallbackMaxEntries {
		t.Fatalf("expected %d fallback states, got %d", openAIHTTP2FallbackMaxEntries, count)
	}
}

func TestOpenAIHTTP2SuccessDeletesFallbackState(t *testing.T) {
	service := &httpUpstreamService{openAIHTTP2Fallbacks: make(map[string]*openAIHTTP2FallbackState)}
	const proxyKey = "http://proxy.example"
	service.getOrCreateOpenAIHTTP2FallbackState(proxyKey)
	service.recordOpenAIHTTP2Success(servicepkg.HTTPUpstreamProfileOpenAI, upstreamProtocolModeOpenAIH2, proxyKey)

	service.openAIHTTP2FallbackMu.Lock()
	count := len(service.openAIHTTP2Fallbacks)
	service.openAIHTTP2FallbackMu.Unlock()
	if count != 0 {
		t.Fatalf("expected successful HTTP/2 request to clear fallback state, got %d", count)
	}
}
