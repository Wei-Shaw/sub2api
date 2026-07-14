package jshandler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsPathWithinDir(t *testing.T) {
	dataDir := t.TempDir()
	inside := filepath.Join(dataDir, "ok.js")
	require.NoError(t, os.WriteFile(inside, []byte("// x"), 0o600))
	require.True(t, isPathWithinDir(inside, dataDir))

	outside := t.TempDir()
	evil := filepath.Join(outside, "evil.js")
	require.NoError(t, os.WriteFile(evil, []byte("// x"), 0o600))
	require.False(t, isPathWithinDir(evil, dataDir))
}
