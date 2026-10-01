package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/stretchr/testify/require"
)

func TestCursorOAuthStartAuthGeneratesPKCEParams(t *testing.T) {
	svc := NewCursorOAuthService(nil)

	result, err := svc.StartAuth(context.Background())
	require.NoError(t, err)
	require.Contains(t, result.URL, "loginDeepControl")
	require.Contains(t, result.URL, "uuid="+result.UUID)
	require.Contains(t, result.URL, "challenge=")
	require.NotEqual(t, result.Verifier, "")
}

func TestCursorOAuthPollAuthPendingAndDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("uuid") == "pending" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"accessToken":  "at-1",
			"refreshToken": "rt-1",
		})
	}))
	defer srv.Close()

	restore := cursor.SetAuthEndpointsForTest(srv.URL, srv.URL)
	t.Cleanup(restore)

	svc := NewCursorOAuthService(nil)
	svc.SetHTTPClientFactoryForTest(func(string) (*http.Client, error) { return srv.Client(), nil })

	pending, err := svc.PollAuth(context.Background(), "pending", "v", nil)
	require.NoError(t, err)
	require.False(t, pending.Done)

	done, err := svc.PollAuth(context.Background(), "ready", "v", nil)
	require.NoError(t, err)
	require.True(t, done.Done)
	require.Equal(t, "at-1", done.Credentials["access_token"])
	require.Equal(t, "rt-1", done.Credentials["refresh_token"])
	require.Equal(t, cursor.TokenKindDeepControl, done.Credentials["token_kind"])
}

func TestCursorOAuthPollAuthRejectsMissingParams(t *testing.T) {
	svc := NewCursorOAuthService(nil)
	_, err := svc.PollAuth(context.Background(), "", "v", nil)
	require.ErrorContains(t, err, "uuid and verifier")
}

// TestCursorTokenRefresherRefreshesAllKindsViaOAuthToken verifies that both
// credential origins refresh through /oauth/token. Live testing showed
// deep-control refresh tokens (browser login) and pasted session tokens use
// the same endpoint; exchange_user_api_key serves User API Keys only and
// rejects refresh tokens outright.
func TestCursorTokenRefresherRefreshesAllKindsViaOAuthToken(t *testing.T) {
	var oauthTokenCalls, exchangeCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			oauthTokenCalls++
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token":  "at-refreshed",
				"refresh_token": "",
			})
		case cursor.EndpointUserAPIKey:
			exchangeCalls++
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	restore := cursor.SetAuthEndpointsForTest(srv.URL, srv.URL)
	t.Cleanup(restore)
	cursor.SetOAuthTokenURLForTest(srv.URL + "/oauth/token")
	t.Cleanup(func() { cursor.SetOAuthTokenURLForTest("") })

	for _, kind := range []string{cursor.TokenKindDeepControl, cursor.TokenKindSession, ""} {
		account := &Account{
			ID:       51,
			Platform: PlatformCursor,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"access_token":  "stale",
				"refresh_token": "rt",
				"token_kind":    kind,
			},
		}

		refresher := NewCursorTokenRefresher()
		refresher.httpClient = srv.Client()
		creds, err := refresher.Refresh(context.Background(), account)
		require.NoError(t, err)
		require.Equal(t, "at-refreshed", creds["access_token"])
		if kind != "" {
			require.Equal(t, kind, creds["token_kind"], "token_kind must survive refresh")
		}
	}
	require.Equal(t, 3, oauthTokenCalls)
	require.Zero(t, exchangeCalls, "refresh must never touch exchange_user_api_key")
}
