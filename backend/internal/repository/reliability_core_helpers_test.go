package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJSONValuesEqual_UsesArbitraryPrecisionNumericSemantics(t *testing.T) {
	for _, right := range []json.RawMessage{
		json.RawMessage(`{"value":1.0,"nested":[1e0,{"n":1}]}`),
		json.RawMessage(`{"value":1e0,"nested":[1,{"n":1.00}]}`),
	} {
		require.True(t, jsonValuesEqual(
			json.RawMessage(`{"value":1,"nested":[1.0,{"n":1e0}]}`),
			right,
		))
	}

	require.False(t, jsonValuesEqual(
		json.RawMessage(`{"value":9007199254740992}`),
		json.RawMessage(`{"value":9007199254740993}`),
	))
}

func TestSanitizeOutboxError_RemovesURLUserinfoQueryAndFragment(t *testing.T) {
	got := sanitizeOutboxError(
		"request https://url-user:url-password@example.test/path?token=query-secret#fragment-secret " +
			"fragment https://example.test/path#fragment-only-secret",
	)

	require.NotContains(t, got, "url-user")
	require.NotContains(t, got, "url-password")
	require.NotContains(t, got, "query-secret")
	require.NotContains(t, got, "fragment-secret")
	require.NotContains(t, got, "fragment-only-secret")
	require.Contains(t, got, "https://example.test/path")
}

func TestNormalizePostgresTime_UsesUTCAndTruncatesSubMicrosecondPrecision(t *testing.T) {
	input := time.Unix(1_700_000_000, 123_456_789).In(time.FixedZone("UTC+8", 8*60*60))
	require.Equal(t,
		time.Unix(1_700_000_000, 123_456_000).UTC(),
		normalizePostgresTime(input),
	)
}
