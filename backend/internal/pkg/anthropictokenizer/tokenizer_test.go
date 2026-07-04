//go:build unit

package anthropictokenizer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountTokensMatchesAnthropicReferenceExamples(t *testing.T) {
	require.Equal(t, 3, CountTokens("hello world!"))
	require.Equal(t, 1, CountTokens("™"))
	require.Equal(t, 1, CountTokens("ϰ"))
	require.Equal(t, 1, CountTokens("<EOT>"))
}
