package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/stretchr/testify/require"
)

func TestWebImageJobStorageUsesDataDirAndExplicitOverride(t *testing.T) {
	dataDir, override := t.TempDir(), t.TempDir()
	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", "")
	store, err := configuredWebImageJobStore()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dataDir, "web-image-jobs"), store.dir)
	key := jobTestKey(t.Name())
	_, err = store.run(context.Background(), key, func(context.Context) (chatgptwebimage.Result, error) {
		return chatgptwebimage.Result{}, nil
	})
	require.NoError(t, err)
	info, err := os.Stat(store.dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	require.FileExists(t, filepath.Join(store.dir, key+".json"))

	t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", override)
	explicit, err := configuredWebImageJobStore()
	require.NoError(t, err)
	require.Equal(t, override, explicit.dir)
	require.NotSame(t, store, explicit)
	// Equivalent absolute paths must share the same in-process job store.
	t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", override+string(filepath.Separator)+".")
	alias, err := configuredWebImageJobStore()
	require.NoError(t, err)
	require.Same(t, explicit, alias)
}

func TestWebImageJobStorageRejectsRelativeExplicitOverride(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", "relative-state-dir")
	_, err := configuredWebImageJobStore()
	require.ErrorContains(t, err, "must be absolute")
}
