package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestHeaderPassthroughEnabled(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestHeaderPassthroughExtraKey: true}}
	require.True(t, account.IsOpenAIRequestHeaderPassthroughEnabled())

	account.Platform = PlatformAnthropic
	require.False(t, account.IsOpenAIRequestHeaderPassthroughEnabled())

	account.Platform = PlatformOpenAI
	account.Extra[openAIRequestHeaderPassthroughExtraKey] = "true"
	require.False(t, account.IsOpenAIRequestHeaderPassthroughEnabled())
}

func TestCopyOpenAIInboundHeadersPassthrough(t *testing.T) {
	src := make(http.Header)
	src.Set("User-Agent", "WorkBuddy/1.2")
	src.Set("originator", "workbuddy")
	src.Set("version", "1.2.3")
	src.Set("x-workbuddy-client", "desktop")
	src.Set("Authorization", "Bearer client-token")
	src.Set("Cookie", "session=client-cookie")
	src.Set("Connection", "keep-alive")

	dst := make(http.Header)
	copyOpenAIInboundHeaders(dst, src, true, false)

	require.Equal(t, "WorkBuddy/1.2", dst.Get("User-Agent"))
	require.Equal(t, "workbuddy", dst.Get("originator"))
	require.Equal(t, "1.2.3", dst.Get("version"))
	require.Equal(t, "desktop", dst.Get("x-workbuddy-client"))
	require.Empty(t, dst.Get("Authorization"))
	require.Empty(t, dst.Get("Cookie"))
	require.Empty(t, dst.Get("Connection"))
}

func TestCopyOpenAIInboundHeadersDefaultUsesAllowlist(t *testing.T) {
	src := make(http.Header)
	src.Set("User-Agent", "client/1.0")
	src.Set("x-workbuddy-client", "desktop")

	dst := make(http.Header)
	copyOpenAIInboundHeaders(dst, src, false, false)

	require.Equal(t, "client/1.0", dst.Get("User-Agent"))
	require.Empty(t, dst.Get("x-workbuddy-client"))
}
