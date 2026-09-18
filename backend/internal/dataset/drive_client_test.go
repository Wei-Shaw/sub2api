//go:build unit

package dataset

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestNewDriveClient_RequiresToken(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	credPath := filepath.Join(tmpDir, "creds.json")
	tokenPath := credPath + ".token"

	// Write valid credentials with redirect_uris
	cred := map[string]any{
		"installed": map[string]any{
			"client_id":     "test-client-id",
			"client_secret": "test-secret",
			"auth_uri":      "https://accounts.google.com/o/oauth2/auth",
			"token_uri":     "https://oauth2.googleapis.com/token",
			"redirect_uris": []string{"urn:ietf:wg:oauth:2.0:oob"},
		},
	}
	credJSON, err := json.Marshal(cred)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(credPath, credJSON, 0600))

	// Call without token file should fail with production-safe error
	_, err = NewDriveClient(ctx, credPath, "test-folder-id")
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

	// Write valid credentials with redirect_uris
	cred := map[string]any{
		"installed": map[string]any{
			"client_id":     "test-client-id",
			"client_secret": "test-secret",
			"auth_uri":      "https://accounts.google.com/o/oauth2/auth",
			"token_uri":     "https://oauth2.googleapis.com/token",
			"redirect_uris": []string{"urn:ietf:wg:oauth:2.0:oob"},
		},
	}
	credJSON, err := json.Marshal(cred)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(credPath, credJSON, 0600))

	// Write valid token
	token := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
	}
	tokenJSON, err := json.Marshal(token)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenPath, tokenJSON, 0600))

	// Should succeed (network call will fail but client init passes)
	client, err := NewDriveClient(ctx, credPath, "test-folder-id")
	if err != nil {
		// Network error is acceptable in unit test
		require.Contains(t, err.Error(), "unable to retrieve parent folder")
		return
	}
	require.NotNil(t, client)
}

func TestNewDriveClient_InlineCredentialsRequireExplicitTokenPath(t *testing.T) {
	ctx := context.Background()
	credJSON := validCredentialsJSON(t)

	_, err := NewDriveClient(ctx, string(credJSON), "test-folder-id")

	require.Error(t, err)
	require.Contains(t, err.Error(), "OAuth token path is required")
	require.Contains(t, err.Error(), "DATASET_GOOGLE_DRIVE_TOKEN_PATH")
	require.NotContains(t, err.Error(), "test-secret")
}

func TestNewDriveClientWithTokenPath_AcceptsInlineCredentials(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "dataset.token")

	tokenJSON, err := json.Marshal(&oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
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
			"client_id":     "test-client-id",
			"client_secret": "test-secret",
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
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token.json")

	// Missing file
	_, err := tokenFromFile(tokenPath)
	require.Error(t, err)

	// Valid token
	token := &oauth2.Token{
		AccessToken:  "test-access",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
	}
	tokenJSON, err := json.Marshal(token)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenPath, tokenJSON, 0600))

	loaded, err := tokenFromFile(tokenPath)
	require.NoError(t, err)
	require.Equal(t, "test-access", loaded.AccessToken)
	require.Equal(t, "test-refresh", loaded.RefreshToken)
}
