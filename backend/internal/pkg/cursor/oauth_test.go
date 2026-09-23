package cursor

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateAuthParamsPKCES256(t *testing.T) {
	params, err := GenerateAuthParams()
	require.NoError(t, err)
	require.NotEmpty(t, params.Verifier)
	require.NotEmpty(t, params.UUID)

	// Challenge is the S256 hash of the verifier.
	sum := sha256.Sum256([]byte(params.Verifier))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), params.Challenge)

	// Login URL carries the challenge/uuid pair plus the CLI target.
	require.Contains(t, params.LoginURL, "challenge="+params.Challenge)
	require.Contains(t, params.LoginURL, "uuid="+params.UUID)
	require.Contains(t, params.LoginURL, "mode=login")
	require.Contains(t, params.LoginURL, "redirectTarget=cli")

	// Each attempt gets fresh material.
	second, err := GenerateAuthParams()
	require.NoError(t, err)
	require.NotEqual(t, params.Verifier, second.Verifier)
	require.NotEqual(t, params.UUID, second.UUID)
}

func withOAuthTestBase(t *testing.T, srv *httptest.Server) {
	t.Helper()
	previousPoll, previousExchange := authPollBaseURL, userAPIKeyBaseURL
	authPollBaseURL, userAPIKeyBaseURL = srv.URL, srv.URL
	t.Cleanup(func() {
		authPollBaseURL, userAPIKeyBaseURL = previousPoll, previousExchange
	})
}

func TestPollAuthSessionPendingAndSuccess(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		require.Equal(t, EndpointAuthPoll, r.URL.Path)
		require.Equal(t, "uuid-1", r.URL.Query().Get("uuid"))
		require.Equal(t, "verifier-1", r.URL.Query().Get("verifier"))
		if attempts == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"accessToken":  "at",
			"refreshToken": "rt",
		})
	}))
	defer srv.Close()
	withOAuthTestBase(t, srv)

	// Pending maps to the sentinel error.
	_, err := PollAuthSession(context.Background(), srv.Client(), "uuid-1", "verifier-1")
	require.ErrorIs(t, err, ErrAuthPending)

	// Completed login returns the token pair.
	session, err := PollAuthSession(context.Background(), srv.Client(), "uuid-1", "verifier-1")
	require.NoError(t, err)
	require.Equal(t, "at", session.AccessToken)
	require.Equal(t, "rt", session.RefreshToken)
}

func TestPollAuthSessionErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()
	withOAuthTestBase(t, srv)

	_, err := PollAuthSession(context.Background(), srv.Client(), "u", "v")
	require.ErrorContains(t, err, "status 403")
	require.NotErrorIs(t, err, ErrAuthPending)
}

func TestRefreshViaUserAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, EndpointUserAPIKey, r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer rt-old", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]string{
			"accessToken":  "at-new",
			"refreshToken": "rt-new",
		})
	}))
	defer srv.Close()
	withOAuthTestBase(t, srv)

	result, err := RefreshViaUserAPIKey(context.Background(), srv.Client(), "rt-old")
	require.NoError(t, err)
	require.Equal(t, "at-new", result.AccessToken)
	require.Equal(t, "rt-new", result.RefreshToken)
	require.Zero(t, result.ExpiresIn, "exchange endpoint returns no expires_in")
}

func TestRefreshViaUserAPIKeyRejectsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`invalid token`))
	}))
	defer srv.Close()
	withOAuthTestBase(t, srv)

	_, err := RefreshViaUserAPIKey(context.Background(), srv.Client(), "rt")
	require.ErrorContains(t, err, "status 401")

	_, err = RefreshViaUserAPIKey(context.Background(), srv.Client(), "  ")
	require.ErrorContains(t, err, "missing refresh token")
}

func TestBuildHeadersCLIProfileWithoutMachineIDs(t *testing.T) {
	h := BuildHeaders(Credentials{AccessToken: "at"})
	require.Equal(t, "cli", h["x-cursor-client-type"])
	require.Equal(t, DefaultCLIClientVersion, h["x-cursor-client-version"],
		"CLI profile must send the CLI-style version; agentn rejects IDE versions with ERROR_OUTDATED_CLIENT")
	require.NotContains(t, h, "x-cursor-checksum")
	require.NotContains(t, h, "x-cursor-client-layout")
	require.NotContains(t, h, "x-cursor-client-os")
	require.Equal(t, "true", h["x-ghost-mode"])
	require.Equal(t, "Bearer at", h["authorization"])
	require.NotEmpty(t, h["x-client-key"])

	// An explicit client_version credential overrides the CLI default.
	custom := BuildHeaders(Credentials{AccessToken: "at", ClientVersion: "cli-custom"})
	require.Equal(t, "cli-custom", custom["x-cursor-client-version"])

	// A machine id (either kind) selects the IDE profile.
	ide := BuildHeaders(Credentials{AccessToken: "at", MachineID: "m"})
	require.Equal(t, "ide", ide["x-cursor-client-type"])
	require.Contains(t, ide, "x-cursor-checksum")
	require.Equal(t, "false", ide["x-ghost-mode"])

	macOnly := BuildHeaders(Credentials{AccessToken: "at", MacMachineID: "mm"})
	require.Equal(t, "ide", macOnly["x-cursor-client-type"])
	require.Contains(t, macOnly, "x-cursor-checksum")
}
