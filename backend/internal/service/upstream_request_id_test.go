package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamRequestIDFromHeaders_UnconfiguredAccountRecordsNothing(t *testing.T) {
	h := http.Header{}
	h.Set("X-Client-Request-ID", "sub2api-client")
	h.Set("X-Request-ID", "sub2api-local")
	h.Set("X-Oneapi-Request-Id", "oneapi-1")
	h.Set("Request-Id", "req_official")
	h.Set("xai-request-id", "xai-1")
	h.Set("x-goog-request-id", "goog-1")

	require.Equal(t, "", UpstreamRequestIDFromHeaders(nil, h))
	for _, platform := range []string{PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok} {
		require.Equal(t, "", UpstreamRequestIDFromHeaders(&Account{Platform: platform}, h), platform)
	}
	blank := &Account{Platform: PlatformOpenAI, Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "   "}}
	require.Equal(t, "", UpstreamRequestIDFromHeaders(blank, h))
}

func TestUpstreamRequestIDFromHeaders_ReadsOnlyConfiguredHeader(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Extra:    map[string]any{AccountExtraUpstreamRequestIDHeader: " x-oneapi-request-id "},
	}
	h := http.Header{}
	h.Set("X-Request-ID", "passthrough-from-real-upstream")
	require.Equal(t, "", UpstreamRequestIDFromHeaders(account, h))

	h.Set("X-Oneapi-Request-Id", " oneapi-2 ")
	require.Equal(t, "oneapi-2", UpstreamRequestIDFromHeaders(account, h))
	require.Equal(t, "", UpstreamRequestIDFromHeaders(account, nil))

	official := &Account{Platform: PlatformAnthropic, Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "request-id"}}
	only := http.Header{}
	only.Set("Request-Id", "req_official")
	require.Equal(t, "req_official", UpstreamRequestIDFromHeaders(official, only))
}

func TestUsageUpstreamRequestIDPtr(t *testing.T) {
	account := &Account{Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "X-Request-ID"}}
	h := http.Header{}
	h.Set("X-Request-ID", strings.Repeat("a", 200))
	require.Nil(t, usageUpstreamRequestIDPtr(account, h, true))
	require.Nil(t, usageUpstreamRequestIDPtr(account, http.Header{}, false))
	require.Nil(t, usageUpstreamRequestIDPtr(nil, h, false))
	require.Nil(t, usageUpstreamRequestIDPtr(&Account{}, h, false))

	got := usageUpstreamRequestIDPtr(account, h, false)
	require.NotNil(t, got)
	require.Len(t, *got, maxUsageUpstreamRequestIDLen)
}

func TestValidateUpstreamRequestIDHeaderExtra(t *testing.T) {
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(nil))
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{}))

	blank := map[string]any{AccountExtraUpstreamRequestIDHeader: "   "}
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(blank))
	_, present := blank[AccountExtraUpstreamRequestIDHeader]
	require.False(t, present, "blank header name must be removed")

	valid := map[string]any{AccountExtraUpstreamRequestIDHeader: " X-Oneapi-Request-Id "}
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(valid))
	require.Equal(t, "X-Oneapi-Request-Id", valid[AccountExtraUpstreamRequestIDHeader])

	require.Error(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: 1}))
	require.Error(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: "X Request Id"}))
	require.Error(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: "X-Request-Id:"}))
	require.Error(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: strings.Repeat("x", 65)}))
}
