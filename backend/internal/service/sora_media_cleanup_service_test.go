//go:build unit

package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSoraMediaCleanupService_RunCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:      "local",
				LocalPath: tmpDir,
				Cleanup: config.SoraStorageCleanupConfig{
					Enabled:       true,
					RetentionDays: 1,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	storage := NewSoraMediaStorage(cfg)
	require.NoError(t, storage.EnsureLocalDirs())

	oldImage := filepath.Join(storage.ImageRoot(), "old.png")
	newVideo := filepath.Join(storage.VideoRoot(), "new.mp4")
	require.NoError(t, os.WriteFile(oldImage, []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(newVideo, []byte("new"), 0o644))

	oldTime := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(oldImage, oldTime, oldTime))

	cleanup := NewSoraMediaCleanupService(storage, cfg)
	cleanup.runCleanup()

	require.NoFileExists(t, oldImage)
	require.FileExists(t, newVideo)
REDACTED
