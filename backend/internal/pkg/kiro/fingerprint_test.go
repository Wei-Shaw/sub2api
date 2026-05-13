package kiro

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildLoginHeadersStable(t *testing.T) {
	headers1 := BuildLoginHeaders("", "")
	headers2 := BuildLoginHeaders("", "")

	require.Equal(t, headers1["User-Agent"], headers2["User-Agent"])
	require.Equal(t, "application/json, text/plain, */*", headers1["Accept"])
	require.Equal(t, "application/json", headers1["Content-Type"])
	require.True(t, strings.HasPrefix(headers1["User-Agent"], "KiroIDE-"))
	require.Contains(t, headers1["User-Agent"], "KiroIDE-")
}

func TestBuildLoginHeadersUsesProvidedMachineID(t *testing.T) {
	machineIDA := BuildMachineID("refresh-a", "", "")
	machineIDB := BuildMachineID("refresh-b", "", "")
	headers1 := BuildLoginHeaders("account-a", machineIDA)
	headers2 := BuildLoginHeaders("account-a", machineIDA)
	headers3 := BuildLoginHeaders("account-a", machineIDB)

	require.Equal(t, headers1["User-Agent"], headers2["User-Agent"])
	require.NotEqual(t, headers1["User-Agent"], headers3["User-Agent"])
	require.Contains(t, headers1["User-Agent"], "KiroIDE-0.11.")
	require.Contains(t, headers1["User-Agent"], machineIDA)
}

func TestBuildOIDCHeadersUsesProvidedAccountKey(t *testing.T) {
	machineID := BuildMachineID("", "", "oidc-machine")
	headers1 := BuildOIDCHeaders("account-a", machineID)
	headers2 := BuildOIDCHeaders("account-a", machineID)
	headers3 := BuildOIDCHeaders("account-b", machineID)

	require.Equal(t, headers1["User-Agent"], headers2["User-Agent"])
	require.NotEqual(t, headers1["User-Agent"], headers3["User-Agent"])
	require.Contains(t, headers1["User-Agent"], "api/sso-oidc#")
}

func TestBuildAccountKeyFallsBackToAccountIDBeforeRandom(t *testing.T) {
	key1 := BuildAccountKey("", "", "", "", 42)
	key2 := BuildAccountKey("", "", "", "", 42)
	key3 := BuildAccountKey("", "", "", "", 43)

	require.Equal(t, key1, key2)
	require.Equal(t, shortSHA(fmt.Sprintf("account:%d", 42)), key1)
	require.NotEqual(t, key1, key3)
}

func TestBuildMachineID(t *testing.T) {
	require.Equal(t, expectedKiroMachineID("KotlinNativeAPI/token"), BuildMachineID("token", "", ""))
	require.Equal(t, expectedKiroMachineID("KiroAPIKey/key"), BuildMachineID("", "key", ""))
	require.Equal(t, expectedKiroMachineID("KotlinNativeAPI/token"), BuildMachineID("token", "key", "fallback"))

	fallback1 := BuildMachineID("", "", "account:1")
	fallback2 := BuildMachineID("", "", "account:1")
	fallback3 := BuildMachineID("", "", "account:2")
	require.Equal(t, expectedKiroMachineID("KiroFallback/account:1"), fallback1)
	require.Equal(t, fallback1, fallback2)
	require.NotEqual(t, fallback1, fallback3)
	require.Len(t, fallback1, 64)
}

func TestNormalizeMachineID(t *testing.T) {
	hex64 := strings.Repeat("A", 64)
	normalized, ok := NormalizeMachineID(hex64)
	require.True(t, ok)
	require.Equal(t, strings.ToLower(hex64), normalized)

	normalized, ok = NormalizeMachineID("2582956e-cc88-4669-b546-07adbffcb894")
	require.True(t, ok)
	require.Equal(t, "2582956ecc884669b54607adbffcb8942582956ecc884669b54607adbffcb894", normalized)

	_, ok = NormalizeMachineID("not-a-machine-id")
	require.False(t, ok)
	_, ok = NormalizeMachineID(strings.Repeat("g", 64))
	require.False(t, ok)
}

func expectedKiroMachineID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}
