package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUsageCursor(t *testing.T) {
	for _, raw := range []string{"v1:0:0", "v1:18446744073709551615:9223372036854775807", "v1:4294967296:42"} {
		cursor, err := ParseUsageCursor(raw)
		require.NoError(t, err)
		require.Equal(t, raw, cursor.String())
	}
	cursor, err := ParseUsageCursor("")
	require.NoError(t, err)
	require.Equal(t, "v1:0:0", cursor.String())
	for _, raw := range []string{"foo", "v2:1:1", "v1:-1:1", "v1:1:-1", "v1:18446744073709551616:1", "v1:1:9223372036854775808", "v1:1:1:1"} {
		_, err := ParseUsageCursor(raw)
		require.Error(t, err, raw)
	}
}
