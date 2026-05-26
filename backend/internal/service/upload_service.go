// Package service contains application services. UploadService persists
// admin-uploaded image files to a configured directory, content-addressed
// by SHA-256 so re-uploads of the same file dedupe to a single object.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Upload size / type constants. Kept package-level so handler validation and
// service persistence agree on a single source of truth.
const (
	// UploadMaxImageSizeBytes caps a single upload at 5 MiB. Larger payloads
	// are rejected before SaveImage touches disk.
	UploadMaxImageSizeBytes int64 = 5 << 20

	// UploadFilenameHashHexLen is the hex-prefix length of SHA-256 used as the
	// stored filename. 16 hex chars = 64 bits = enough to dedupe at our scale
	// without bloating filenames.
	UploadFilenameHashHexLen = 16
)

// uploadAllowedMIME maps accepted Content-Type values to the canonical file
// extension stored on disk. MIME types not in this map are rejected.
var uploadAllowedMIME = map[string]string{
	"image/jpeg":    "jpg",
	"image/jpg":     "jpg",
	"image/png":     "png",
	"image/webp":    "webp",
	"image/gif":     "gif",
	"image/svg+xml": "svg",
}

// UploadServiceErrors surfaced to callers. Handler maps these to HTTP 400.
var (
	ErrUploadFileTooLarge     = errors.New("upload: file exceeds maximum size")
	ErrUploadUnsupportedMIME  = errors.New("upload: unsupported MIME type")
	ErrUploadEmptyFile        = errors.New("upload: empty file")
	ErrUploadDirNotConfigured = errors.New("upload: storage directory not configured")
)

// UploadService persists images to a flat directory keyed by content hash.
// Reads stay simple (any process can serve the file by name), and dedupe
// happens automatically because the same bytes always map to the same name.
type UploadService struct {
	uploadDir string
}

// NewUploadService creates an UploadService rooted at uploadDir. The directory
// is created on first SaveImage call (lazy mkdir keeps construction cheap and
// failure-tolerant on read-only test environments).
func NewUploadService(uploadDir string) *UploadService {
	return &UploadService{uploadDir: strings.TrimSpace(uploadDir)}
}

// UploadDir returns the configured storage directory; used by the public
// download handler to resolve filenames back to disk paths.
func (s *UploadService) UploadDir() string {
	return s.uploadDir
}

// SaveImage validates and stores an image. file must not be nil. mimeType
// must be one of UploadAllowedMIME keys. size is the declared content-length
// (multipart Header.Size) used for an early reject before reading the body.
//
// On success returns the bare filename (no directory). The handler turns
// this into a public URL of the form "/api/v1/uploads/<filename>".
func (s *UploadService) SaveImage(ctx context.Context, file io.Reader, mimeType string, size int64) (string, error) {
	if s.uploadDir == "" {
		return "", ErrUploadDirNotConfigured
	}
	if size <= 0 {
		return "", ErrUploadEmptyFile
	}
	if size > UploadMaxImageSizeBytes {
		return "", fmt.Errorf("%w: %d > %d", ErrUploadFileTooLarge, size, UploadMaxImageSizeBytes)
	}
	ext, ok := uploadAllowedMIME[strings.ToLower(strings.TrimSpace(mimeType))]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUploadUnsupportedMIME, mimeType)
	}

	// Read full body into memory while computing SHA-256. Cap at the size
	// limit + 1 so a multipart file lying about its content-length still
	// can't blow past the budget.
	limited := io.LimitReader(file, UploadMaxImageSizeBytes+1)
	hasher := sha256.New()
	buf, err := io.ReadAll(io.TeeReader(limited, hasher))
	if err != nil {
		return "", fmt.Errorf("upload: read body: %w", err)
	}
	if int64(len(buf)) > UploadMaxImageSizeBytes {
		return "", fmt.Errorf("%w: actual %d > %d", ErrUploadFileTooLarge, len(buf), UploadMaxImageSizeBytes)
	}
	if len(buf) == 0 {
		return "", ErrUploadEmptyFile
	}

	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return "", fmt.Errorf("upload: mkdir %s: %w", s.uploadDir, err)
	}

	digest := hex.EncodeToString(hasher.Sum(nil))[:UploadFilenameHashHexLen]
	filename := digest + "." + ext
	full := filepath.Join(s.uploadDir, filename)

	// Skip writing if a file with the same content hash already exists.
	// Two uploaders posting identical bytes share storage.
	if _, err := os.Stat(full); err == nil {
		return filename, nil
	}

	// Write atomically: write to <name>.tmp then rename, so a partial write
	// (process crash mid-upload) never leaves a half-written file under the
	// final name where the public download handler would read it.
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return "", fmt.Errorf("upload: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("upload: rename %s: %w", full, err)
	}

	_ = ctx // ctx accepted for API symmetry / future cancellation hooks.
	return filename, nil
}
