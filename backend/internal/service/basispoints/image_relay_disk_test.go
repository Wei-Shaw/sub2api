package basispoints

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestImageRelayStoresPrivateFilesAndRemovesExpiredImages(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	data := relayTestPNG(t)
	out, err := r.Rewrite(relayTestRequest(t, data), "scope")
	require.NoError(t, err)
	url := relayTestURL(t, out)
	token := strings.TrimPrefix(url, r.baseURL+ImageRelayPath)
	img := r.entries[token]
	actual, err := os.ReadFile(img.path)
	require.NoError(t, err)
	require.Equal(t, data, actual)
	info, err := os.Stat(img.path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	info, err = os.Stat(r.dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	require.Zero(t, r.reservedBytes)
	require.Zero(t, r.reservedEntries)
	img.expires = time.Now().Add(-time.Second)
	r.expire(token, img)
	_, err = os.Stat(img.path)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, r.Close())
	_, err = os.Stat(r.dir)
	require.True(t, os.IsNotExist(err))
	_, err = r.Rewrite(relayTestRequest(t, data), "scope")
	require.ErrorIs(t, err, ErrImageRelayStorage)
}

func TestImageRelayDiskReservationsAndFailuresReleaseQuota(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	raw := "data:image/png;base64," + base64.StdEncoding.EncodeToString(relayTestPNG(t))
	r.reservedBytes = imageRelayMaxBytes
	_, _, err = r.storeImage(raw, "scope")
	require.ErrorIs(t, err, ErrImageRelayFull)
	files, err := os.ReadDir(r.dir)
	require.NoError(t, err)
	require.Empty(t, files, "full quota must reject before creating a file")
	r.reservedBytes = 0
	_, err = r.Rewrite([]byte(`{"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,PRIVATE_INVALID"}]}]}`), "scope")
	require.Error(t, err)
	require.Zero(t, r.reservedBytes)
	require.Zero(t, r.reservedEntries)
	files, err = os.ReadDir(r.dir)
	require.NoError(t, err)
	require.Empty(t, files)
	require.NoError(t, os.Remove(r.dir))
	require.NoError(t, os.WriteFile(r.dir, []byte("blocked"), 0600))
	_, _, err = r.storeImage(raw, "scope")
	require.ErrorIs(t, err, ErrImageRelayStorage)
	require.NotContains(t, err.Error(), r.dir)
	require.Zero(t, r.reservedBytes)
	require.Zero(t, r.reservedEntries)
}

func TestImageRelayReclaimsCrashedSessionDirectories(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "session-old")
	live := filepath.Join(root, "session-live")
	unrelated := filepath.Join(root, "keep")
	for _, dir := range []string{old, live, unrelated} {
		require.NoError(t, os.Mkdir(dir, 0700))
	}
	stale := time.Now().Add(-imageRelayTTL - 2*imageRelayCleanupInterval)
	require.NoError(t, os.Chtimes(old, stale, stale))
	require.NoError(t, os.Chtimes(unrelated, stale, stale))
	r, err := NewImageRelay("https://images.example", root)
	require.NoError(t, err)
	defer func() { require.NoError(t, r.Close()) }()
	_, err = os.Stat(old)
	require.True(t, os.IsNotExist(err))
	for _, dir := range []string{live, unrelated} {
		_, err = os.Stat(dir)
		require.NoError(t, err)
	}
}

func TestImageRelayDownloadConcurrencyIsBounded(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	for i := 0; i < cap(r.downloads); i++ {
		r.downloads <- struct{}{}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://images.example"+ImageRelayPath+strings.Repeat("a", 43), nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
}

type blockedImageWriter struct {
	header  http.Header
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	data    bytes.Buffer
}

func (w *blockedImageWriter) Header() http.Header { return w.header }
func (w *blockedImageWriter) WriteHeader(int)     {}
func (w *blockedImageWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.data.Write(p)
}

func TestImageRelayExpiryDuringDownloadPreservesReplacementAndQuota(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	data := relayTestPNG(t)
	raw := relayTestRequest(t, data)
	out, err := r.Rewrite(raw, "scope")
	require.NoError(t, err)
	url := relayTestURL(t, out)
	token := strings.TrimPrefix(url, r.baseURL+ImageRelayPath)
	old := r.entries[token]
	w := &blockedImageWriter{header: make(http.Header), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(w.release) }); <-done })
	go func() {
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		close(done)
	}()
	select {
	case <-w.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not start")
	}
	r.mu.Lock()
	old.expires = time.Now().Add(-time.Second)
	r.pruneLocked(time.Now())
	r.mu.Unlock()
	require.EqualValues(t, len(data), r.bytes, "open downloads must retain disk quota")
	require.Equal(t, 1, r.retiredEntries)
	_, err = r.Rewrite(raw, "scope")
	require.NoError(t, err)
	fresh := r.entries[token]
	require.NotEqual(t, old.path, fresh.path)
	require.EqualValues(t, len(data)*2, r.bytes)
	release.Do(func() { close(w.release) })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not finish")
	}
	require.Equal(t, data, w.data.Bytes())
	require.EqualValues(t, len(data), r.bytes)
	require.Zero(t, r.retiredEntries)
	_, err = os.Stat(old.path)
	require.True(t, os.IsNotExist(err))
	fetched := httptest.NewRecorder()
	r.ServeHTTP(fetched, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, data, fetched.Body.Bytes())
}
