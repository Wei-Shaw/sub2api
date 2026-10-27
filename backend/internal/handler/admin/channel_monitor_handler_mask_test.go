//go:build unit

package admin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskAPIKeyPreservesEmptyState(t *testing.T) {
	require.Empty(t, maskAPIKey(""))
	require.Equal(t, "***", maskAPIKey("key"))
	require.Equal(t, "sk-t***", maskAPIKey("sk-test"))
}
