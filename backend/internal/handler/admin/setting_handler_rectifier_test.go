//go:build unit

package admin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanRectifierPatterns(t *testing.T) {
	t.Run("returns non-nil empty slice for nil input (distinguishes from missing field)", func(t *testing.T) {
		got, err := cleanRectifierPatterns(nil)
		require.NoError(t, err)
		require.NotNil(t, got, "must return non-nil so callers can distinguish 'user cleared' from 'field absent'")
		require.Empty(t, got)
	})

	t.Run("returns non-nil empty slice for empty input", func(t *testing.T) {
		got, err := cleanRectifierPatterns([]string{})
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})

	t.Run("trims whitespace", func(t *testing.T) {
		got, err := cleanRectifierPatterns([]string{"  hello  ", "\tworld\n"})
		require.NoError(t, err)
		require.Equal(t, []string{"hello", "world"}, got)
	})

	t.Run("filters out empty / whitespace-only entries", func(t *testing.T) {
		got, err := cleanRectifierPatterns([]string{"", "   ", "\t", "kept"})
		require.NoError(t, err)
		require.Equal(t, []string{"kept"}, got)
	})

	t.Run("rejects too many patterns", func(t *testing.T) {
		input := make([]string, rectifierMaxPatterns+1)
		for i := range input {
			input[i] = "x"
		}
		_, err := cleanRectifierPatterns(input)
		require.Error(t, err)
		require.Contains(t, err.Error(), "too many patterns")
	})

	t.Run("rejects pattern over length limit", func(t *testing.T) {
		long := strings.Repeat("a", rectifierMaxPatternLen+1)
		_, err := cleanRectifierPatterns([]string{long})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pattern too long")
	})

	t.Run("accepts pattern at exactly length limit", func(t *testing.T) {
		atLimit := strings.Repeat("a", rectifierMaxPatternLen)
		got, err := cleanRectifierPatterns([]string{atLimit})
		require.NoError(t, err)
		require.Equal(t, []string{atLimit}, got)
	})

	t.Run("accepts at exactly maxPatterns count", func(t *testing.T) {
		input := make([]string, rectifierMaxPatterns)
		for i := range input {
			input[i] = "x"
		}
		got, err := cleanRectifierPatterns(input)
		require.NoError(t, err)
		require.Len(t, got, rectifierMaxPatterns)
	})
}
