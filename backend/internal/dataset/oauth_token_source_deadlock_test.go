//go:build unit

package dataset

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// TestNewOAuthHTTPClientDoesNotMutateDefaultClient confirms the refactored
// newOAuthHTTPClient creates an independent HTTP client rather than mutating
// the shared http.DefaultClient, preventing OAuth refresh deadlocks.
func TestNewOAuthHTTPClientDoesNotMutateDefaultClient(t *testing.T) {
	ctx := context.Background()

	initial := &oauth2.Token{
		AccessToken: "stale-access", RefreshToken: "offline-refresh", TokenType: "Bearer",
		Expiry: time.Now().Add(time.Hour),
	}
	tokenPath := filepath.Join(t.TempDir(), "dataset.token")
	config := &oauth2.Config{ClientID: "client", ClientSecret: "secret", Endpoint: oauth2.Endpoint{}}

	tokenSource, err := newPersistingTokenSource(ctx, config, initial, tokenPath)
	require.NoError(t, err)

	oauthClient := newOAuthHTTPClient(ctx, tokenSource)
	require.NotEqual(t, http.DefaultClient, oauthClient, "newOAuthHTTPClient must allocate an independent client")
	require.NotEqual(t, http.DefaultClient.Transport, oauthClient.Transport, "newOAuthHTTPClient must not mutate DefaultClient.Transport")
}

// TestNewOAuthHTTPClientRefreshesExpiredTokenWithoutDeadlock exercises the full
// token-refresh path with an expired token to confirm it does not deadlock.
func TestNewOAuthHTTPClientRefreshesExpiredTokenWithoutDeadlock(t *testing.T) {
	ctx := context.Background()
	var tokenRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
			}
			require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"access_token":"fresh-access","expires_in":3600,"token_type":"Bearer"}`)
		} else if r.URL.Path == "/api" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("ok"))
		} else {
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	initial := &oauth2.Token{
		AccessToken: "expired-access", RefreshToken: "offline-refresh", TokenType: "Bearer",
		Expiry: time.Now().Add(-time.Hour),
	}
	tokenPath := filepath.Join(t.TempDir(), "dataset.token")
	config := &oauth2.Config{
		ClientID: "client", ClientSecret: "secret", Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"},
	}
	tokenSource, err := newPersistingTokenSource(ctx, config, initial, tokenPath)
	require.NoError(t, err)

	oauthClient := newOAuthHTTPClient(ctx, tokenSource)

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := oauthClient.Get(srv.URL + "/api")
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("token refresh with expired seed did not return within 5 seconds (deadlock suspected)")
	}

	require.Equal(t, 1, tokenRequests, "expired token should trigger exactly one refresh")
	persisted, err := tokenFromFile(tokenPath)
	require.NoError(t, err)
	require.Equal(t, "fresh-access", persisted.AccessToken)
}
