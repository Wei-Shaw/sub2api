package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageRequestDumpStore_PutGetHas_MasksAuth(t *testing.T) {
	dir := t.TempDir()
	store := NewUsageRequestDumpStore(dir)

	err := store.Put(&UsageRequestDump{
		RequestID: "req-abc_01",
		ReqHeaders: map[string]string{
			"Authorization": "Bearer secret",
			"X-Api-Key":     "k",
			"Content-Type":  "application/json",
		},
		ReqBody: `{"model":"gpt"}`,
	})
	require.NoError(t, err)
	require.True(t, store.Has("req-abc_01"))
	require.False(t, store.Has("missing"))

	got, err := store.Get("req-abc_01")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "***", got.ReqHeaders["Authorization"])
	require.Equal(t, "***", got.ReqHeaders["X-Api-Key"])
	require.Equal(t, "application/json", got.ReqHeaders["Content-Type"])
	require.Equal(t, `{"model":"gpt"}`, got.ReqBody)

	_, err = os.Stat(filepath.Join(dir, "req-abc_01.json"))
	require.NoError(t, err)
}

func TestMaskSensitiveHeaders(t *testing.T) {
	in := map[string]string{
		"authorization": "a",
		"Cookie":        "b",
		"x-api-key":     "c",
		"api-key":       "d",
		"Accept":        "e",
	}
	out := MaskSensitiveHeaders(in)
	require.Equal(t, "***", out["authorization"])
	require.Equal(t, "***", out["Cookie"])
	require.Equal(t, "***", out["x-api-key"])
	require.Equal(t, "***", out["api-key"])
	require.Equal(t, "e", out["Accept"])
}

func TestSanitizeDumpKey(t *testing.T) {
	require.Equal(t, "ab-cd_01", sanitizeDumpKey("ab-cd_01"))
	require.Equal(t, "a_b", sanitizeDumpKey("a/b"))
	require.Equal(t, "", sanitizeDumpKey(".."))
}

func TestUsageRequestDumpStore_GetMissing(t *testing.T) {
	store := NewUsageRequestDumpStore(t.TempDir())
	got, err := store.Get("nope")
	require.NoError(t, err)
	require.Nil(t, got)
}
