package service

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVideoPreviewCachePublishesOnlyCompleteVideoAndExpires(t *testing.T) {
	root := t.TempDir()
	cache := newVideoPreviewCache(root)
	cache.maxBytes = 1 << 20
	cache.spaceOK = func(string) bool { return true }
	now := time.Now().Truncate(time.Second)
	key := videoPreviewCacheKey(12, 34, "grok-video:task-1")
	video := make([]byte, 24)
	video[3] = 24
	copy(video[4:12], []byte("ftypisom"))
	copy(video[16:20], []byte("isom"))

	require.Error(t, cache.save(context.Background(), key, func(_ context.Context, w http.ResponseWriter) error {
		w.Header().Set("Content-Length", "999")
		_, err := w.Write(video)
		return err
	}, now))
	// The fake length must be rejected and leave no final directory.
	_, err := os.Stat(filepath.Join(root, key))
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, cache.save(context.Background(), key, func(_ context.Context, w http.ResponseWriter) error {
		w.Header().Set("Content-Length", "24")
		_, err := w.Write(video)
		return err
	}, now))
	file, contentType, err := cache.open(key, now.Add(time.Second))
	require.NoError(t, err)
	defer file.Close()
	require.Equal(t, "video/mp4", contentType)
	got, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, video, got)

	require.NoError(t, cache.cleanup(now.Add(videoPreviewCacheRetention+time.Second)))
	_, _, err = cache.open(key, now.Add(videoPreviewCacheRetention+time.Second))
	require.ErrorIs(t, err, os.ErrNotExist)
}
