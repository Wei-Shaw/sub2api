package websearch

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTruncateBody_Short(t *testing.T) {
	body := []byte("short body")
	require.Equal(t, "short body", truncateBody(body))
}

func TestTruncateBody_Long(t *testing.T) {
	body := []byte(strings.Repeat("x", 500))
	result := truncateBody(body)
	require.Len(t, result, errorBodyTruncLen+len("...(truncated)"))
	require.True(t, strings.HasSuffix(result, "...(truncated)"))
}

func TestTruncateBody_ExactBoundary(t *testing.T) {
	body := []byte(strings.Repeat("x", errorBodyTruncLen))
	require.Equal(t, string(body), truncateBody(body))
}
