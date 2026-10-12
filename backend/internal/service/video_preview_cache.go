package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v4/disk"
)

const (
	videoPreviewCacheDir        = "/app/data/admin-video-cache"
	videoPreviewCacheRetention  = 7 * 24 * time.Hour
	videoPreviewDownloadTimeout = 3 * time.Minute
	videoPreviewCacheOwnerKey   = "video_preview_owner_key_id"
)

// Only complete media files are published. The cache has no DB, generation or
// billing dependency. Jobs are bounded and never delay a successful response.
type videoPreviewCache struct {
	root             string
	maxBytes         int64
	spaceOK          func(string) bool
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	started, stopped bool
	pending          map[string]bool
	slots            chan struct{}
	wg               sync.WaitGroup
}

func newVideoPreviewCache(root string) *videoPreviewCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &videoPreviewCache{root: root, maxBytes: adminVideoPreviewMaxBytes, ctx: ctx, cancel: cancel,
		pending: make(map[string]bool), slots: make(chan struct{}, 2), spaceOK: func(path string) bool {
			usage, err := disk.Usage(path)
			// Keep a reserve for the live application; two simultaneous downloads
			// cannot consume the last free space on its persistent volume.
			return err == nil && usage.Free > (512<<20)+2*adminVideoPreviewMaxBytes
		}}
}

func videoPreviewCacheKey(accountID, apiKeyID int64, requestID string) string {
	taskID, err := adminVideoTaskID(requestID)
	if err != nil || accountID <= 0 || apiKeyID <= 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", accountID, apiKeyID, taskID)))
	return hex.EncodeToString(sum[:])
}

func (v *videoPreviewCache) start() {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.started || v.stopped {
		return
	}
	v.started = true
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			if err := v.cleanup(time.Now()); err != nil {
				logger.LegacyPrintf("service.video_preview", "video cache cleanup failed")
			}
			select {
			case <-v.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (v *videoPreviewCache) stop() {
	if v == nil {
		return
	}
	v.mu.Lock()
	v.stopped = true
	v.cancel()
	v.mu.Unlock()
	v.wg.Wait()
}

func (v *videoPreviewCache) enqueue(key string, download func(context.Context, http.ResponseWriter) error) {
	if v == nil || key == "" {
		return
	}
	v.start()
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.stopped || v.pending[key] {
		return
	}
	if _, err := os.Lstat(filepath.Join(v.root, key)); err == nil {
		return
	}
	if len(v.pending) >= 32 {
		logger.LegacyPrintf("service.video_preview", "video cache queue full; upstream playback remains available")
		return
	}
	v.pending[key] = true
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		defer func() { v.mu.Lock(); delete(v.pending, key); v.mu.Unlock() }()
		select {
		case v.slots <- struct{}{}:
		case <-v.ctx.Done():
			return
		}
		defer func() { <-v.slots }()
		ctx, cancel := context.WithTimeout(v.ctx, videoPreviewDownloadTimeout)
		defer cancel()
		if err := v.save(ctx, key, download, time.Now()); err != nil && v.ctx.Err() == nil {
			// Never log signed URLs, credentials or an upstream response body.
			logger.LegacyPrintf("service.video_preview", "video cache download failed; upstream playback remains available")
		}
	}()
}

type videoCacheWriter struct {
	file           *os.File
	header         http.Header
	status         int
	written, limit int64
}

func (w *videoCacheWriter) Header() http.Header { return w.header }
func (w *videoCacheWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *videoCacheWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.status != http.StatusOK || w.header.Get("Content-Range") != "" || w.written+int64(len(p)) > w.limit {
		return 0, errors.New("incomplete or oversized video cache response")
	}
	n, err := w.file.Write(p)
	w.written += int64(n)
	return n, err
}

func (v *videoPreviewCache) save(ctx context.Context, key string, download func(context.Context, http.ResponseWriter) error, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(v.root, 0700); err != nil {
		return err
	}
	if !v.spaceOK(v.root) {
		return errors.New("video cache free space reserve reached")
	}
	stage, err := os.MkdirTemp(v.root, ".download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	file, err := os.OpenFile(filepath.Join(stage, "video"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	w := &videoCacheWriter{file: file, header: make(http.Header), limit: v.maxBytes}
	if err := download(ctx, w); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if w.status != http.StatusOK || w.written == 0 {
		return errors.New("video cache response is not complete")
	}
	if raw := w.header.Get("Content-Length"); raw != "" {
		expected, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || expected != w.written {
			return errors.New("truncated video cache response")
		}
	}
	if _, err := videoCacheContentType(file); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chtimes(filepath.Join(stage, "video"), now, now); err != nil {
		return err
	}
	if err := os.Chtimes(stage, now, now); err != nil {
		return err
	}
	// A concurrent instance's non-empty final directory cannot be overwritten.
	// Thus repeated status checks cannot extend the first saved video's lifetime.
	return os.Rename(stage, filepath.Join(v.root, key))
}

func videoCacheContentType(file *os.File) (string, error) {
	var header [512]byte
	n, err := file.ReadAt(header[:], 0)
	if err != nil && err != io.EOF {
		return "", err
	}
	contentType := http.DetectContentType(header[:n])
	if bytesLooksLikeMP4(header[:n]) {
		return "video/mp4", nil
	}
	if !strings.HasPrefix(contentType, "video/") {
		return "", errors.New("downloaded cache is not a video")
	}
	return contentType, nil
}

func (v *videoPreviewCache) open(key string, now time.Time) (*os.File, string, error) {
	if v == nil || key == "" {
		return nil, "", os.ErrNotExist
	}
	dir := filepath.Join(v.root, key)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || !info.ModTime().After(now.Add(-videoPreviewCacheRetention)) {
		return nil, "", os.ErrNotExist
	}
	path := filepath.Join(dir, "video")
	info, err = os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > v.maxBytes {
		return nil, "", os.ErrNotExist
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	contentType, err := videoCacheContentType(file)
	if err != nil {
		file.Close()
		return nil, "", err
	}
	return file, contentType, nil
}

func (v *videoPreviewCache) cleanup(now time.Time) error {
	entries, err := os.ReadDir(v.root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name, ttl := entry.Name(), videoPreviewCacheRetention
		if strings.HasPrefix(name, ".download-") {
			ttl = 2 * videoPreviewDownloadTimeout
		} else {
			if b, err := hex.DecodeString(name); err != nil || len(b) != sha256.Size {
				continue
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.ModTime().After(now.Add(-ttl)) {
			if err := os.RemoveAll(filepath.Join(v.root, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Call only after the existing usage record / administrator access check. No
// public file route is added. A cache hit also works when an account is offline.
func (s *OpenAIGatewayService) ServeCachedAdminVideoPreview(c *gin.Context, log *UsageLog) bool {
	if s == nil || log == nil || len(ListAdminVideoPreviewURLs(log, time.Now())) == 0 {
		return false
	}
	file, contentType, err := s.videoPreviewCache.open(videoPreviewCacheKey(log.AccountID, log.APIKeyID, log.RequestID), time.Now())
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, "video", info.ModTime(), file)
	return true
}
