package cursor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccessTokenExpiryReadsJWTExp(t *testing.T) {
	exp := time.Unix(1_700_000_000, 0).UTC()
	token := fakeJWT(map[string]any{"exp": exp.Unix()})

	got := AccessTokenExpiry(token)
	if got == nil {
		t.Fatal("expected expiry")
	}
	if !got.Equal(exp) {
		t.Fatalf("expiry=%s want %s", got, exp)
	}

	prefixed := AccessTokenExpiry("user-123::" + token)
	if prefixed == nil || !prefixed.Equal(exp) {
		t.Fatalf("prefixed expiry=%v", prefixed)
	}
}

func TestAccessTokenExpiryRejectsNonJWT(t *testing.T) {
	if got := AccessTokenExpiry("not-a-jwt"); got != nil {
		t.Fatalf("got %v", got)
	}
	if got := AccessTokenExpiry(""); got != nil {
		t.Fatalf("empty: %v", got)
	}
}

func TestTokenRefreshResultExpiresAtPrefersExpiresIn(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	result := TokenRefreshResult{ExpiresIn: 120, AccessToken: fakeJWT(map[string]any{"exp": now.Add(time.Hour).Unix()})}
	got := result.ExpiresAt(now)
	if !got.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("got %s", got)
	}
}

func TestRefreshSessionPersistsRotatedRefreshToken(t *testing.T) {
	var gotBody tokenRefreshRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer srv.Close()

	old := oauthTokenURL
	oauthTokenURL = srv.URL
	defer func() { oauthTokenURL = old }()

	result, err := RefreshSession(context.Background(), srv.Client(), "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if gotBody.GrantType != "refresh_token" || gotBody.RefreshToken != "old-refresh" {
		t.Fatalf("request=%+v", gotBody)
	}
	if result.AccessToken != "new-access" || result.RefreshToken != "new-refresh" || result.ExpiresIn != 3600 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRefreshSessionRejectsEmptyAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"","expires_in":60}`)
	}))
	defer srv.Close()

	old := oauthTokenURL
	oauthTokenURL = srv.URL
	defer func() { oauthTokenURL = old }()

	_, err := RefreshSession(context.Background(), srv.Client(), "rt")
	if err == nil || !strings.Contains(err.Error(), "empty access token") {
		t.Fatalf("err=%v", err)
	}
}

func fakeJWT(claims map[string]any) string {
	payload, _ := json.Marshal(claims)
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
