//go:build unit

package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestGoogleDriveTokenSourceRefreshesExpiredTokenAndPersistsIt(t *testing.T) {
	var tokenRequests int
	var apiSawFreshToken bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
			}
			require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
			require.Equal(t, "offline-refresh", r.Form.Get("refresh_token"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"access_token":"fresh-access","expires_in":3600,"token_type":"Bearer"}`)
		case "/drive/v3/files":
			apiSawFreshToken = r.Header.Get("Authorization") == "Bearer fresh-access"
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"files":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tokenPath := filepath.Join(t.TempDir(), "dataset.token")
	initial := &oauth2.Token{
		AccessToken:  "expired-access",
		RefreshToken: "offline-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	}
	encoded, err := json.Marshal(initial)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenPath, encoded, 0600))

	config := &oauth2.Config{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		Endpoint:     google.Endpoint,
		Scopes:       []string{"https://www.googleapis.com/auth/drive.file"},
	}
	config.Endpoint.TokenURL = srv.URL + "/token"
	store, err := newPersistingTokenSource(context.Background(), config, initial, tokenPath)
	require.NoError(t, err)

	client := oauth2.NewClient(context.Background(), store)
	response, err := client.Get(srv.URL + "/drive/v3/files")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, 1, tokenRequests)
	require.True(t, apiSawFreshToken)

	persisted, err := tokenFromFile(tokenPath)
	require.NoError(t, err)
	require.Equal(t, "fresh-access", persisted.AccessToken)
	require.Equal(t, "offline-refresh", persisted.RefreshToken)
	require.False(t, persisted.Expiry.IsZero())
	require.True(t, persisted.Expiry.After(time.Now()))
	info, err := os.Stat(tokenPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestGoogleDriveTokenSourceRefreshesTokenWithoutExpiry(t *testing.T) {
	var tokenRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		tokenRequests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"fresh-access","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer srv.Close()

	initial := &oauth2.Token{AccessToken: "old-access", RefreshToken: "offline-refresh", TokenType: "Bearer"}
	config := &oauth2.Config{ClientID: "client", ClientSecret: "secret", Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"}}
	store, err := newPersistingTokenSource(context.Background(), config, initial, filepath.Join(t.TempDir(), "token.json"))
	require.NoError(t, err)

	refreshed, err := store.Token()
	require.NoError(t, err)
	require.Equal(t, "fresh-access", refreshed.AccessToken)
	require.False(t, refreshed.Expiry.IsZero())
	require.Equal(t, 1, tokenRequests)
}

func TestGoogleDriveTokenSourceDoesNotPersistWhenRefreshFails(t *testing.T) {
	var tokenRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenRequests++
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse token form: %v", err)
		}
		require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		require.Equal(t, "offline-refresh", r.Form.Get("refresh_token"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"[REDACTED]"}`)
	}))
	defer srv.Close()

	tokenPath := filepath.Join(t.TempDir(), "dataset.token")
	initial := &oauth2.Token{
		AccessToken: "expired-access", RefreshToken: "offline-refresh", TokenType: "Bearer", Expiry: time.Now().Add(-time.Hour),
	}
	encoded, err := json.Marshal(initial)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenPath, encoded, 0600))

	config := &oauth2.Config{ClientID: "client", ClientSecret: "secret", Endpoint: oauth2.Endpoint{TokenURL: srv.URL}}
	store, err := newPersistingTokenSource(context.Background(), config, initial, tokenPath)
	require.NoError(t, err)
	_, err = store.Token()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "[REDACTED]")
	require.True(t, tokenRequests >= 1, "OAuth token source must attempt refresh at least once")
	actual, err := os.ReadFile(tokenPath)
	require.NoError(t, err)
	require.Equal(t, encoded, actual)
}

func TestDriveClientRefreshesInvalidTokenAndRetriesSameBatchOnce(t *testing.T) {
	ctx := context.Background()
	var tokenRequests int
	var apiAttempts int
	var authorization []string
	var multipartParts [][]byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			tokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
			}
			if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "offline-refresh" {
				t.Errorf("unexpected refresh request")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"access_token":"fresh-access","expires_in":3600,"token_type":"Bearer"}`)
		case strings.HasSuffix(r.URL.Path, "/files"):
			apiAttempts++
			authorization = append(authorization, r.Header.Get("Authorization"))
			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "multipart/related" {
				t.Errorf("unexpected upload content type %q: %v", mediaType, err)
				http.Error(w, "invalid content type", http.StatusBadRequest)
				return
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			var parts [][]byte
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("read upload part: %v", err)
					http.Error(w, "invalid multipart body", http.StatusBadRequest)
					return
				}
				data, err := io.ReadAll(part)
				if err != nil {
					t.Errorf("read upload part body: %v", err)
					http.Error(w, "invalid multipart body", http.StatusBadRequest)
					return
				}
				parts = append(parts, data)
			}
			multipartParts = append(multipartParts, parts...)
			if apiAttempts == 1 {
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				http.Error(w, `{"error":{"code":401,"message":"invalid_token"}}`, http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"uploaded-file","name":"dataset.jsonl"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	initial := &oauth2.Token{
		AccessToken:  "stale-access",
		RefreshToken: "offline-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}
	tokenPath := filepath.Join(t.TempDir(), "dataset.token")
	config := &oauth2.Config{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		Endpoint: oauth2.Endpoint{
			AuthURL:  srv.URL + "/auth",
			TokenURL: srv.URL + "/token",
		},
	}
	tokenSource, err := newPersistingTokenSource(ctx, config, initial, tokenPath)
	require.NoError(t, err)

	httpClient := newOAuthHTTPClient(ctx, tokenSource)
	service, err := drive.NewService(ctx,
		option.WithEndpoint(srv.URL+"/"),
		option.WithHTTPClient(httpClient),
	)
	require.NoError(t, err)
	client := &DriveClient{service: service, folderID: "test-folder", tokenSource: tokenSource}

	entry := DatasetEntry{
		Timestamp: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC),
		Endpoint:  "chat.completions",
		Request:   map[string]any{"model": "example-model"},
	}
	fileID, err := client.UploadBatch(ctx, []DatasetEntry{entry})
	require.NoError(t, err)
	require.Equal(t, "uploaded-file", fileID)
	require.Equal(t, 2, apiAttempts, "an invalid_token response should cause exactly one retry")
	require.Equal(t, 1, tokenRequests, "the rejected access token should be refreshed exactly once")
	require.Equal(t, []string{"Bearer stale-access", "Bearer fresh-access"}, authorization)
	require.Len(t, multipartParts, 4, "both attempts should contain metadata and media parts")
	require.Contains(t, string(multipartParts[1]), "example-model")
	require.Contains(t, string(multipartParts[1]), "chat.completions")
	require.Equal(t, multipartParts[0], multipartParts[2], "metadata should be unchanged on retry")
	require.Equal(t, multipartParts[1], multipartParts[3], "the exact JSONL batch should be retried")

	persisted, err := tokenFromFile(tokenPath)
	require.NoError(t, err)
	require.Equal(t, "fresh-access", persisted.AccessToken)
	require.Equal(t, "offline-refresh", persisted.RefreshToken)
}

func TestInvalidDriveTokenErrorRequiresExactSignal(t *testing.T) {
	require.True(t, hasInvalidTokenChallenge(`Bearer error="invalid_token"`))
	require.False(t, hasInvalidTokenChallenge(`Bearer error="invalid_token_documentation"`))

	tests := []struct {
		name  string
		err   *googleapi.Error
		valid bool
	}{
		{
			name: "exact challenge",
			err: &googleapi.Error{
				Code:   http.StatusUnauthorized,
				Header: http.Header{"Www-Authenticate": []string{`Bearer error="invalid_token"`}},
			},
			valid: true,
		},
		{
			name: "exact JSON error",
			err: &googleapi.Error{
				Code: http.StatusUnauthorized,
				Body: `{"error":{"message":"invalid_token"}}`,
			},
			valid: true,
		},
		{
			name: "incidental body mention",
			err: &googleapi.Error{
				Code: http.StatusUnauthorized,
				Body: `{"error":{"message":"token rejected; see invalid_token documentation"}}`,
			},
		},
		{
			name: "unrelated unauthorized",
			err: &googleapi.Error{
				Code: http.StatusUnauthorized,
				Body: `{"error":{"message":"account_disabled"}}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.valid, isInvalidDriveTokenError(tt.err))
		})
	}
}

func TestDriveClientStopsAfterOneRefreshAndRetry(t *testing.T) {
	ctx := context.Background()
	var tokenRequests int
	var apiAttempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			tokenRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"access_token":"fresh","expires_in":3600,"token_type":"Bearer"}`)
		case strings.HasSuffix(r.URL.Path, "/files"):
			apiAttempts++
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			http.Error(w, `{"error":{"message":"invalid_token"}}`, http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	initial := &oauth2.Token{
		AccessToken:  "stale",
		RefreshToken: "[REDACTED]",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}
	tokenSource, err := newPersistingTokenSource(ctx, &oauth2.Config{
		ClientID:     "[REDACTED]",
		ClientSecret: "[REDACTED]",
		Endpoint: oauth2.Endpoint{
			TokenURL: srv.URL + "/token",
		},
	}, initial, filepath.Join(t.TempDir(), "state.token"))
	require.NoError(t, err)

	service, err := drive.NewService(ctx,
		option.WithEndpoint(srv.URL+"/"),
		option.WithHTTPClient(newOAuthHTTPClient(ctx, tokenSource)),
	)
	require.NoError(t, err)
	client := &DriveClient{service: service, folderID: "folder", tokenSource: tokenSource}

	_, err = client.UploadBatch(ctx, []DatasetEntry{{Endpoint: "chat.completions"}})
	require.Error(t, err)
	require.Equal(t, 2, apiAttempts)
	require.Equal(t, 1, tokenRequests, "a second invalid_token response must not trigger another refresh")
}

func TestSafeDriveErrorDoesNotLeakResponseBody(t *testing.T) {
	err := &googleapi.Error{Code: http.StatusUnauthorized, Body: "[REDACTED]"}

	safe := safeDriveError(err)
	require.NotContains(t, safe.Error(), "[REDACTED]")
	require.Contains(t, safe.Error(), "HTTP 401")
}

func TestAtomicTokenWritePreservesExistingStateWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.token")
	previous := []byte("prior state remains intact\n")
	require.NoError(t, os.WriteFile(path, previous, 0600))

	err := writeOAuthTokenAtomicallyWithRename(path, &oauth2.Token{
		AccessToken:  "[REDACTED]",
		RefreshToken: "[REDACTED]",
		Expiry:       time.Now().Add(time.Hour),
	}, func(string, string) error {
		return errors.New("simulated replace failure")
	})
	require.ErrorContains(t, err, "replace OAuth token")

	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, previous, after)
	entries, readDirErr := os.ReadDir(dir)
	require.NoError(t, readDirErr)
	require.Len(t, entries, 1, "temporary file must be cleaned up after replace failure")
}

func TestGoogleDriveTokenSourceUsesSeparateWritableStatePath(t *testing.T) {
	var tokenRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenRequests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"[REDACTED]","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer srv.Close()

	seedPath := filepath.Join(t.TempDir(), "seed.token")
	statePath := filepath.Join(t.TempDir(), "state.token")
	initial := &oauth2.Token{
		AccessToken:  "[REDACTED]",
		RefreshToken: "[REDACTED]",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	}
	seedBytes, err := json.Marshal(initial)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(seedPath, seedBytes, 0600))

	config := &oauth2.Config{
		ClientID:     "[REDACTED]",
		ClientSecret: "[REDACTED]",
		Endpoint:     oauth2.Endpoint{TokenURL: srv.URL},
	}
	store, err := newPersistingTokenSource(context.Background(), config, initial, statePath)
	require.NoError(t, err)
	_, err = store.Token()
	require.NoError(t, err)
	require.Equal(t, 1, tokenRequests)

	seedAfter, err := os.ReadFile(seedPath)
	require.NoError(t, err)
	require.Equal(t, seedBytes, seedAfter, "read-only seed must not be replaced")
	stateAfter, err := tokenFromFile(statePath)
	require.NoError(t, err)
	require.Equal(t, "[REDACTED]", stateAfter.AccessToken)
}

func TestTokenFromPathsPrefersPersistentState(t *testing.T) {
	seedPath := filepath.Join(t.TempDir(), "seed.token")
	statePath := filepath.Join(t.TempDir(), "state.token")
	seed := &oauth2.Token{AccessToken: "seed-value", RefreshToken: "[REDACTED]", TokenType: "Bearer"}
	state := &oauth2.Token{AccessToken: "state-value", RefreshToken: "[REDACTED]", TokenType: "Bearer"}
	seedBytes, err := json.Marshal(seed)
	require.NoError(t, err)
	stateBytes, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(seedPath, seedBytes, 0600))
	require.NoError(t, os.WriteFile(statePath, stateBytes, 0600))

	loaded, err := tokenFromPaths(seedPath, statePath)
	require.NoError(t, err)
	require.Equal(t, state.AccessToken, loaded.AccessToken)
}

func TestNewDriveClient_RequiresToken(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "creds.json")
	tokenPath := credPath + ".token"
	require.NoError(t, os.WriteFile(credPath, validCredentialsJSON(t), 0600))

	_, err := NewDriveClient(ctx, credPath, "test-folder-id")
	require.Error(t, err)
	require.Contains(t, err.Error(), "OAuth token not found")
	require.Contains(t, err.Error(), "provision an offline OAuth token")
	require.Contains(t, err.Error(), tokenPath)
}

func TestNewDriveClient_WithValidToken(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "creds.json")
	tokenPath := credPath + ".token"
	require.NoError(t, os.WriteFile(credPath, validCredentialsJSON(t), 0600))

	tokenJSON, err := json.Marshal(&oauth2.Token{
		AccessToken:  "[REDACTED]",
		RefreshToken: "[REDACTED]",
		TokenType:    "Bearer",
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenPath, tokenJSON, 0600))

	client, err := NewDriveClient(ctx, credPath, "test-folder-id")
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestNewDriveClient_InlineCredentialsRequireExplicitTokenPath(t *testing.T) {
	ctx := context.Background()
	_, err := NewDriveClient(ctx, string(validCredentialsJSON(t)), "test-folder-id")

	require.Error(t, err)
	require.Contains(t, err.Error(), "OAuth token path is required")
	require.Contains(t, err.Error(), "DATASET_GOOGLE_DRIVE_TOKEN_PATH")
	require.NotContains(t, err.Error(), "[REDACTED]")
}

func TestNewDriveClientWithTokenPath_AcceptsInlineCredentials(t *testing.T) {
	ctx := context.Background()
	tokenPath := filepath.Join(t.TempDir(), "dataset.token")
	tokenJSON, err := json.Marshal(&oauth2.Token{
		AccessToken:  "[REDACTED]",
		RefreshToken: "[REDACTED]",
		TokenType:    "Bearer",
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenPath, tokenJSON, 0600))

	client, err := NewDriveClientWithTokenPath(ctx, string(validCredentialsJSON(t)), tokenPath, "test-folder-id")
	require.NoError(t, err)
	require.NotNil(t, client)
}

func validCredentialsJSON(t *testing.T) []byte {
	t.Helper()
	cred := map[string]any{
		"installed": map[string]any{
			"client_id":     "[REDACTED]",
			"client_secret": "[REDACTED]",
			"auth_uri":      "https://accounts.google.com/o/oauth2/auth",
			"token_uri":     "https://oauth2.googleapis.com/token",
			"redirect_uris": []string{"urn:ietf:wg:oauth:2.0:oob"},
		},
	}
	encoded, err := json.Marshal(cred)
	require.NoError(t, err)
	return encoded
}

func TestTokenFromFile(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token.json")
	_, err := tokenFromFile(tokenPath)
	require.Error(t, err)

	token := &oauth2.Token{
		AccessToken:  "[REDACTED]",
		RefreshToken: "[REDACTED]",
		TokenType:    "Bearer",
	}
	tokenJSON, err := json.Marshal(token)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenPath, tokenJSON, 0600))

	loaded, err := tokenFromFile(tokenPath)
	require.NoError(t, err)
	require.Equal(t, token.AccessToken, loaded.AccessToken)
	require.Equal(t, token.RefreshToken, loaded.RefreshToken)
}
