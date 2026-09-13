package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/stretchr/testify/require"
)

func TestParseDevinCallbackExtractsCodeAndState(t *testing.T) {
	code, state := parseDevinCallback("http://127.0.0.1:59653/callback?code=abc&state=session-1", "")
	require.Equal(t, "abc", code)
	require.Equal(t, "session-1", state)
}

func TestParseDevinCallbackAcceptsQueryString(t *testing.T) {
	code, state := parseDevinCallback("?code=xyz&state=st", "fallback")
	require.Equal(t, "xyz", code)
	require.Equal(t, "st", state)
}

func TestParseDevinCallbackKeepsBareCode(t *testing.T) {
	code, state := parseDevinCallback("plain-code", "existing-state")
	require.Equal(t, "plain-code", code)
	require.Equal(t, "existing-state", state)
}

func TestNormalizeSessionTokenAddsPrefix(t *testing.T) {
	require.Equal(t, "devin-session-token$jwt", devin.NormalizeSessionToken("jwt"))
	require.Equal(t, "devin-session-token$jwt", devin.NormalizeSessionToken("devin-session-token$jwt"))
}
