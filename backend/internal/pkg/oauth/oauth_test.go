package oauth

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuthorizeURLMatchesClaudeCodeCLI(t *testing.T) {
	const want = "https://claude.com/cai/oauth/authorize"
	if AuthorizeURL != want {
		t.Fatalf("AuthorizeURL = %q, want %q", AuthorizeURL, want)
	}

	authURL := BuildAuthorizationURL("state-value", "challenge-value", ScopeOAuth)
	if !strings.HasPrefix(authURL, want+"?") {
		t.Fatalf("BuildAuthorizationURL() = %q, want prefix %q", authURL, want+"?")
	}
	for _, part := range []string{
		"code=true",
		"client_id=" + ClientID,
		"response_type=code",
		"code_challenge=challenge-value",
		"code_challenge_method=S256",
		"state=state-value",
	} {
		if !strings.Contains(authURL, part) {
			t.Fatalf("BuildAuthorizationURL() missing %q\nURL: %s", part, authURL)
		}
	}
}

func TestSessionStore_Stop_Idempotent(t *testing.T) {
	store := NewSessionStore()

	store.Stop()
	store.Stop()

	select {
	case <-store.stopCh:
		// ok
	case <-time.After(time.Second):
		t.Fatal("stopCh 未关闭")
	}
}

func TestSessionStore_Stop_Concurrent(t *testing.T) {
	store := NewSessionStore()

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Stop()
		}()
	}

	wg.Wait()

	select {
	case <-store.stopCh:
		// ok
	case <-time.After(time.Second):
		t.Fatal("stopCh 未关闭")
	}
}
