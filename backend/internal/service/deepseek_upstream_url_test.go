package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildDeepseekToolsURL_Endpoints(t *testing.T) {
	account := &Account{
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://relay.example.com",
		},
	}
	cases := []struct {
		endpoint string
		fileID   string
		want     string
	}{
		{"search", "", "https://relay.example.com/v1/tools/search"},
		{"fetch", "", "https://relay.example.com/v1/tools/fetch"},
		{"ocr", "", "https://relay.example.com/v1/tools/ocr"},
		{"asr", "", "https://relay.example.com/v1/tools/asr"},
		{"tts", "", "https://relay.example.com/v1/tools/tts"},
		{"files_create", "", "https://relay.example.com/v1/files"},
		{"files_list", "", "https://relay.example.com/v1/files"},
		{"files_get", "file-abc_123", "https://relay.example.com/v1/files/file-abc_123"},
		{"files_delete", "file-abc_123", "https://relay.example.com/v1/files/file-abc_123"},
	}
	for _, tc := range cases {
		got, err := buildDeepseekToolsURL(account, tc.endpoint, tc.fileID)
		require.NoError(t, err, "endpoint %s", tc.endpoint)
		require.Equal(t, tc.want, got, "endpoint %s", tc.endpoint)
	}
}

func TestBuildDeepseekToolsURL_EmptyBaseFallsBackToProfileDefault(t *testing.T) {
	account := &Account{
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{},
	}
	got, err := buildDeepseekToolsURL(account, "search", "")
	require.NoError(t, err)
	require.Equal(t, DefaultDeepseekBaseURL+"/v1/tools/search", got)

	profile := LookupProviderProfile(PlatformDeepseek)
	require.NotNil(t, profile)
	require.Equal(t, DefaultDeepseekBaseURL, profile.Modes[AccountModePayG].BaseURLs[APIProtocolChatCompletions])
}

func TestBuildDeepseekToolsURL_TrimsTrailingSlash(t *testing.T) {
	account := &Account{
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://relay.example.com/",
		},
	}
	got, err := buildDeepseekToolsURL(account, "ocr", "")
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/v1/tools/ocr", got)
}

func TestBuildDeepseekToolsURL_BaseWithV1SuffixNoDoubleV1(t *testing.T) {
	account := &Account{
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://relay.example.com/v1/",
		},
	}
	got, err := buildDeepseekToolsURL(account, "search", "")
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/v1/tools/search", got)

	got, err = buildDeepseekToolsURL(account, "files_get", "file-1")
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/v1/files/file-1", got)
}

func TestBuildDeepseekToolsURL_RejectsUnsupportedEndpoint(t *testing.T) {
	account := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	_, err := buildDeepseekToolsURL(account, "unknown", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported deepseek tools endpoint")
}

func TestBuildDeepseekToolsURL_RejectsInvalidFileID(t *testing.T) {
	account := &Account{
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://relay.example.com",
		},
	}
	for _, bad := range []string{"", "file/../etc", "file id", "file%20id", "file/../../secret"} {
		_, err := buildDeepseekToolsURL(account, "files_get", bad)
		require.Error(t, err, "fileID %q", bad)
		require.Contains(t, err.Error(), "invalid file id")

		_, err = buildDeepseekToolsURL(account, "files_delete", bad)
		require.Error(t, err, "fileID %q", bad)
	}
}
