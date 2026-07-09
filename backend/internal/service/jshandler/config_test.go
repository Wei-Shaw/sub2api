package jshandler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveOneScriptPath_AbsoluteOutsideDataDirRejected(t *testing.T) {
	dataDir := t.TempDir()
	outside := t.TempDir()
	script := filepath.Join(outside, "evil.js")
	require.NoError(t, os.WriteFile(script, []byte("// x"), 0o600))
	_, err := resolveOneScriptPath(script, dataDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "data_dir")
}

func TestResolveOneScriptPath_AbsoluteInsideDataDirOK(t *testing.T) {
	dataDir := t.TempDir()
	script := filepath.Join(dataDir, "ok.js")
	require.NoError(t, os.WriteFile(script, []byte("// x"), 0o600))
	got, err := resolveOneScriptPath(script, dataDir)
	require.NoError(t, err)
	require.Equal(t, script, got)
}