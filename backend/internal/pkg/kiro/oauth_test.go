//go:build unit

package kiro

import (
	"fmt"
	"testing"
	"time"
)

func TestBuildSocialSignInURLUsesAppPortal(t *testing.T) {
	got := BuildSocialSignInURL("http://localhost:49153", "challenge123", "state456")
	want := "https://app.kiro.dev/signin?code_challenge=challenge123&code_challenge_method=S256&redirect_from=KiroIDE&redirect_uri=http%3A%2F%2Flocalhost%3A49153&state=state456"
	if got != want {
		t.Fatalf("BuildSocialSignInURL() = %q, want %q", got, want)
	}
}

func TestBuildSocialTokenRedirectURI(t *testing.T) {
	got := BuildSocialTokenRedirectURI("http://localhost:49153", "/oauth/callback", "github")
	want := "http://localhost:49153/oauth/callback?login_option=github"
	if got != want {
		t.Fatalf("BuildSocialTokenRedirectURI() = %q, want %q", got, want)
	}
}

func TestSessionStoreGetDeletesExpiredSession(t *testing.T) {
	store := NewSessionStore()
	store.Set("expired", &AuthSession{CreatedAt: time.Now().Add(-2 * sessionTTL)})

	session, ok := store.Get("expired")
	if ok || session != nil {
		t.Fatalf("Get(expired) = (%v, %v), want (nil, false)", session, ok)
	}
	if _, exists := store.data["expired"]; exists {
		t.Fatalf("expired session should be deleted from the store")
	}
}

func TestParseCookieArrayToken_ValidKiroCookies(t *testing.T) {
	cookieJSON := `[
		{"domain":"app.kiro.dev","name":"AccessToken","value":"aoaTestAccessToken123","expirationDate":1779621875.084411},
		{"domain":"app.kiro.dev","name":"RefreshToken","value":"aorTestRefreshToken456","expirationDate":1779621875.084479},
		{"domain":"app.kiro.dev","name":"Idp","value":"Google","expirationDate":1779621875.084501},
		{"domain":"app.kiro.dev","name":"UserId","value":"d-9067c98495.f49804c8-10c1-70ab-e00f-7d2bb88008fd","expirationDate":1779621921.78438},
		{"domain":".app.kiro.dev","name":"kiro-visitor-id","value":"1779017052718-035o707cwa7w","expirationDate":1810553052.718802}
	]`

	token, err := ParseImportedToken(cookieJSON, "")
	if err != nil {
		t.Fatalf("ParseImportedToken(cookie array) error: %v", err)
	}
	if token.AccessToken != "aoaTestAccessToken123" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "aoaTestAccessToken123")
	}
	if token.RefreshToken != "aorTestRefreshToken456" {
		t.Errorf("RefreshToken = %q, want %q", token.RefreshToken, "aorTestRefreshToken456")
	}
	if token.Provider != "Google" {
		t.Errorf("Provider = %q, want %q", token.Provider, "Google")
	}
	if token.AuthMethod != "social" {
		t.Errorf("AuthMethod = %q, want %q", token.AuthMethod, "social")
	}
	if token.Email != "d-9067c98495.f49804c8-10c1-70ab-e00f-7d2bb88008fd" {
		t.Errorf("Email = %q, want UserId value", token.Email)
	}
	if token.ExpiresAt == "" {
		t.Error("ExpiresAt should be set from cookie expirationDate")
	}
	// Verify the expiry time is correct (1779621875 seconds)
	expectedExpiry := time.Unix(1779621875, 0).UTC().Format(time.RFC3339)
	if token.ExpiresAt != expectedExpiry {
		t.Errorf("ExpiresAt = %q, want %q", token.ExpiresAt, expectedExpiry)
	}
}

func TestParseCookieArrayToken_MissingAccessToken(t *testing.T) {
	cookieJSON := `[
		{"domain":"app.kiro.dev","name":"RefreshToken","value":"aorTestRefreshToken456"},
		{"domain":"app.kiro.dev","name":"Idp","value":"Google"}
	]`

	_, err := ParseImportedToken(cookieJSON, "")
	if err == nil {
		t.Fatal("expected error for missing AccessToken, got nil")
	}
}

func TestParseCookieArrayToken_NonKiroCookiesFallThrough(t *testing.T) {
	// Non-kiro cookies should not be parsed as cookie array
	cookieJSON := `[{"domain":"example.com","name":"session","value":"abc123"}]`

	_, err := ParseImportedToken(cookieJSON, "")
	if err == nil {
		t.Fatal("expected error for non-kiro cookie array (no accessToken in TokenData), got nil")
	}
}

func TestParseCookieArrayToken_GitHubProvider(t *testing.T) {
	cookieJSON := `[
		{"domain":"app.kiro.dev","name":"AccessToken","value":"testToken","expirationDate":1779621875},
		{"domain":"app.kiro.dev","name":"Idp","value":"Github"}
	]`

	token, err := ParseImportedToken(cookieJSON, "")
	if err != nil {
		t.Fatalf("ParseImportedToken error: %v", err)
	}
	if token.Provider != "Github" {
		t.Errorf("Provider = %q, want %q", token.Provider, "Github")
	}
}

func TestSessionStoreSetPrunesExpiredSessions(t *testing.T) {
	store := NewSessionStore()
	now := time.Now()
	for i := 0; i < sessionCleanupMin; i++ {
		store.data[fmt.Sprintf("expired-%d", i)] = &AuthSession{CreatedAt: now.Add(-2 * sessionTTL)}
	}
	store.setCount = sessionCleanupEvery - 1

	store.Set("fresh", &AuthSession{CreatedAt: now})

	if len(store.data) != 1 {
		t.Fatalf("store size = %d, want 1", len(store.data))
	}
	if _, ok := store.data["fresh"]; !ok {
		t.Fatalf("fresh session should remain after pruning")
	}
}
