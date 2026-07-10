package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	videoAssetMaxBytes        = 200 << 20 // 200 MiB
	videoAssetDownloadTimeout = 60 * time.Second
	videoAssetRetentionDays   = 30
)

// Overridable for unit tests (production uses validateExternalVideoURL).
var videoAssetURLValidator = validateExternalVideoURL

func videoAssetDataDir() string {
	if dir := strings.TrimSpace(os.Getenv("DATA_DIR")); dir != "" {
		return dir
	}
	dockerDataDir := "/app/data"
	if info, err := os.Stat(dockerDataDir); err == nil && info.IsDir() {
		return dockerDataDir
	}
	return "."
}

// ArchiveSucceededVideoResult downloads result_url into DATA_DIR/assets/video/{id}/.
// Failures are logged and never returned as fatal to billing/worker flow.
func (s *VideoGatewayService) ArchiveSucceededVideoResult(ctx context.Context, task *VideoTask) {
	_ = s.archiveSucceededVideoResult(ctx, task)
}

// ArchiveSucceededVideoResultWithError exposes the durable failure to the
// outbox consumer while the legacy worker keeps its fail-open void method.
func (s *VideoGatewayService) ArchiveSucceededVideoResultWithError(ctx context.Context, task *VideoTask) error {
	return s.archiveSucceededVideoResult(ctx, task)
}

func (s *VideoGatewayService) archiveSucceededVideoResult(ctx context.Context, task *VideoTask) error {
	if s == nil || task == nil || task.ID <= 0 {
		return nil
	}
	if strings.TrimSpace(task.LocalAssetPath) != "" {
		return nil
	}
	if task.Status != VideoStatusSucceeded || strings.TrimSpace(task.ResultURL) == "" {
		return fmt.Errorf("video task has no archivable result")
	}
	if err := videoAssetURLValidator(task.ResultURL); err != nil {
		slog.Warn("video asset archive skipped: result_url failed validation",
			"task_id", task.ID, "error", sanitizeOutboxLogError(err))
		return err
	}

	root := filepath.Join(videoAssetDataDir(), "assets", "video", fmt.Sprintf("%d", task.ID))
	if err := os.MkdirAll(root, 0o750); err != nil {
		slog.Warn("video asset archive mkdir failed", "task_id", task.ID, "error", sanitizeOutboxLogError(err))
		return err
	}

	ext := guessVideoExt(task.ResultURL)
	dest := filepath.Join(root, "result"+ext)
	tmp := dest + ".tmp"

	client := &http.Client{Timeout: videoAssetDownloadTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, task.ResultURL, nil)
	if err != nil {
		slog.Warn("video asset archive request build failed", "task_id", task.ID, "error", sanitizeOutboxLogError(err))
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("video asset archive download failed", "task_id", task.ID, "error", sanitizeOutboxLogError(err))
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("video asset archive bad status", "task_id", task.ID, "status", resp.StatusCode)
		return fmt.Errorf("video asset archive bad status: %s", resp.Status)
	}

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		slog.Warn("video asset archive open failed", "task_id", task.ID, "error", sanitizeOutboxLogError(err))
		return err
	}
	limited := io.LimitReader(resp.Body, videoAssetMaxBytes+1)
	n, copyErr := io.Copy(f, limited)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		slog.Warn("video asset archive write failed", "task_id", task.ID, "error", sanitizeOutboxLogError(firstErr(copyErr, closeErr)))
		return firstErr(copyErr, closeErr)
	}
	if n > videoAssetMaxBytes {
		_ = os.Remove(tmp)
		slog.Warn("video asset archive exceeded size cap", "task_id", task.ID, "bytes", n)
		return fmt.Errorf("video asset archive exceeds size cap: %d", n)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		slog.Warn("video asset archive rename failed", "task_id", task.ID, "error", sanitizeOutboxLogError(err))
		return err
	}

	rel := filepath.ToSlash(filepath.Join("assets", "video", fmt.Sprintf("%d", task.ID), "result"+ext))
	now := time.Now().UTC()
	if err := s.repo.SetTaskLocalAsset(ctx, task.ID, rel, now); err != nil {
		slog.Warn("video asset archive db update failed", "task_id", task.ID, "error", sanitizeOutboxLogError(err))
		return err
	}
	task.LocalAssetPath = rel
	task.LocalAssetSavedAt = &now
	return nil
}

// ResolveLocalAssetAbsPath returns absolute path under DATA_DIR for a stored relative path.
func ResolveLocalAssetAbsPath(rel string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("invalid local asset path")
	}
	if !strings.HasPrefix(rel, "assets/video/") {
		return "", fmt.Errorf("local asset path outside video assets")
	}
	abs := filepath.Join(videoAssetDataDir(), filepath.FromSlash(rel))
	return abs, nil
}

// CleanupExpiredVideoAssets removes archived files older than retention and clears DB paths.
func (s *VideoGatewayService) CleanupExpiredVideoAssets(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -videoAssetRetentionDays)
	tasks, err := s.repo.ListExpiredLocalAssets(ctx, cutoff, 50)
	if err != nil {
		slog.Warn("video asset cleanup list failed", "error", err.Error())
		return
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if abs, err := ResolveLocalAssetAbsPath(task.LocalAssetPath); err == nil {
			_ = os.Remove(abs)
			_ = os.Remove(filepath.Dir(abs))
		}
		_ = s.repo.ClearTaskLocalAsset(ctx, task.ID)
	}
}

func guessVideoExt(rawURL string) string {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.Contains(lower, ".webm"):
		return ".webm"
	case strings.Contains(lower, ".mov"):
		return ".mov"
	case strings.Contains(lower, ".mkv"):
		return ".mkv"
	default:
		return ".mp4"
	}
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("unknown error")
}
