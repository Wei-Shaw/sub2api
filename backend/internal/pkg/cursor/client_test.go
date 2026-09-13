package cursor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPollOncePendingOn404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "login-uuid", r.URL.Query().Get("uuid"))
		require.Equal(t, "verifier", r.URL.Query().Get("verifier"))
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	old := pollURL
	pollURL = server.URL
	t.Cleanup(func() { pollURL = old })

	client, err := NewClient("")
	require.NoError(t, err)
	tokens, pending, err := client.PollOnce(context.Background(), "login-uuid", "verifier")
	require.NoError(t, err)
	require.True(t, pending)
	require.Nil(t, tokens)
}

func TestPollOnceReturnsTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"access","refreshToken":"refresh"}`))
	}))
	t.Cleanup(server.Close)

	old := pollURL
	pollURL = server.URL
	t.Cleanup(func() { pollURL = old })

	client, err := NewClient("")
	require.NoError(t, err)
	tokens, pending, err := client.PollOnce(context.Background(), "login-uuid", "verifier")
	require.NoError(t, err)
	require.False(t, pending)
	require.Equal(t, "access", tokens.AccessToken)
	require.Equal(t, "refresh", tokens.RefreshToken)
}
