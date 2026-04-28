package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	errOpenAIGeneratedImageNotFound = errors.New("openai generated image not found")
	errOpenAIGeneratedImageExpired  = errors.New("openai generated image expired")
	errOpenAIGeneratedImageInvalid  = errors.New("openai generated image invalid")
	errOpenAIGeneratedImageTooLarge = errors.New("openai generated image too large")

	ErrOpenAIGeneratedImageNotFound = errOpenAIGeneratedImageNotFound
	ErrOpenAIGeneratedImageExpired  = errOpenAIGeneratedImageExpired
	ErrOpenAIGeneratedImageInvalid  = errOpenAIGeneratedImageInvalid
	ErrOpenAIGeneratedImageTooLarge = errOpenAIGeneratedImageTooLarge

	openAIGeneratedImageIDPattern       = regexp.MustCompile(`^img_[A-Za-z0-9_-]{32,}$`)
	openAIGeneratedImageFilenamePattern = regexp.MustCompile(`^(img_[A-Za-z0-9_-]{32,})\.(png|jpe?g|webp)$`)
	openAIGeneratedImageTempPattern     = regexp.MustCompile(`^\.(img_[A-Za-z0-9_-]{32,}\.(?:png|jpe?g|webp|json))\.[^.]+\.tmp$`)
	openAIGeneratedImageEntryInfo       = func(entry os.DirEntry) (os.FileInfo, error) { return entry.Info() }
	openAIGeneratedImageLinkFile        = os.Link
	openAIGeneratedImageRemoveFile      = os.Remove
)

const (
	defaultOpenAIGeneratedImageMaxEncodedBytes   int64 = 32 << 20
	defaultOpenAIGeneratedImageMaxDecodedBytes   int64 = 20 << 20
	defaultOpenAIGeneratedImageMaxRehydrateBytes int64 = 20 << 20
	defaultOpenAIGeneratedImageMaxTotalBytes     int64 = 512 << 20
	defaultOpenAIGeneratedImageCleanupLimit            = 100
	defaultOpenAIGeneratedImageCleanupInterval         = time.Minute
)

type OpenAIGeneratedImageStore struct {
	mu                sync.Mutex
	root              string
	now               func() time.Time
	rand              io.Reader
	maxAge            time.Duration
	maxEncodedBytes   int64
	maxDecodedBytes   int64
	maxRehydrateBytes int64
	maxTotalBytes     int64
	cleanupLimit      int
	lastCleanup       time.Time
	cleanupInterval   time.Duration
}

type OpenAIGeneratedImageSaveInput struct {
	Base64       string
	OutputFormat string
	SourceItemID string
}

type OpenAIGeneratedImageRecord struct {
	ID           string    `json:"id"`
	Filename     string    `json:"filename"`
	Format       string    `json:"format"`
	MIME         string    `json:"mime"`
	SourceItemID string    `json:"source_item_id,omitempty"`
	SHA256       string    `json:"sha256"`
	DecodedBytes int64     `json:"decoded_bytes"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func NewOpenAIGeneratedImageStore(root string) *OpenAIGeneratedImageStore {
	cleanRoot := filepath.Clean(root)
	if absRoot, err := filepath.Abs(cleanRoot); err == nil {
		cleanRoot = absRoot
	}
	return &OpenAIGeneratedImageStore{
		root:              cleanRoot,
		now:               time.Now,
		rand:              rand.Reader,
		maxAge:            time.Hour,
		maxEncodedBytes:   defaultOpenAIGeneratedImageMaxEncodedBytes,
		maxDecodedBytes:   defaultOpenAIGeneratedImageMaxDecodedBytes,
		maxRehydrateBytes: defaultOpenAIGeneratedImageMaxRehydrateBytes,
		maxTotalBytes:     defaultOpenAIGeneratedImageMaxTotalBytes,
		cleanupLimit:      defaultOpenAIGeneratedImageCleanupLimit,
		cleanupInterval:   defaultOpenAIGeneratedImageCleanupInterval,
	}
}

func (s *OpenAIGeneratedImageStore) SaveBase64(ctx context.Context, input OpenAIGeneratedImageSaveInput) (OpenAIGeneratedImageRecord, error) {
	if err := ctx.Err(); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}

	encoded := strings.TrimSpace(input.Base64)
	if s.maxEncodedBytes > 0 && int64(len(encoded)) > s.maxEncodedBytes {
		return OpenAIGeneratedImageRecord{}, errOpenAIGeneratedImageTooLarge
	}
	if s.maxDecodedBytes > 0 && int64(base64.StdEncoding.DecodedLen(len(encoded))) > s.maxDecodedBytes {
		return OpenAIGeneratedImageRecord{}, errOpenAIGeneratedImageTooLarge
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, errOpenAIGeneratedImageInvalid
	}
	if s.maxDecodedBytes > 0 && int64(len(data)) > s.maxDecodedBytes {
		return OpenAIGeneratedImageRecord{}, errOpenAIGeneratedImageTooLarge
	}

	format, mime, err := sniffOpenAIGeneratedImage(data, input.OutputFormat)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}

	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	if err := s.maybeCleanupBeforeSave(ctx); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}

	createdAt := s.now().UTC()
	hash := sha256.Sum256(data)
	for attempts := 0; attempts < 16; attempts++ {
		id, err := generateOpenAIGeneratedImageID(s.rand)
		if err != nil {
			return OpenAIGeneratedImageRecord{}, err
		}
		filename := id + "." + format
		imagePath, err := safeOpenAIGeneratedImagePath(s.root, filename)
		if err != nil {
			return OpenAIGeneratedImageRecord{}, err
		}
		metaPath, err := safeOpenAIGeneratedImagePath(s.root, id+".json")
		if err != nil {
			return OpenAIGeneratedImageRecord{}, err
		}
		rec := OpenAIGeneratedImageRecord{
			ID:           id,
			Filename:     filename,
			Format:       format,
			MIME:         mime,
			SourceItemID: input.SourceItemID,
			SHA256:       hex.EncodeToString(hash[:]),
			DecodedBytes: int64(len(data)),
			CreatedAt:    createdAt,
			ExpiresAt:    createdAt.Add(s.maxAge),
		}
		meta, err := json.Marshal(rec)
		if err != nil {
			return OpenAIGeneratedImageRecord{}, err
		}
		collided, err := s.tryPublishGeneratedImage(ctx, imagePath, metaPath, data, meta)
		if err != nil {
			return OpenAIGeneratedImageRecord{}, err
		}
		if collided {
			continue
		}
		return rec, nil
	}

	return OpenAIGeneratedImageRecord{}, fmt.Errorf("generate openai generated image id: %w", errOpenAIGeneratedImageInvalid)
}

func (s *OpenAIGeneratedImageStore) Load(ctx context.Context, id string) (OpenAIGeneratedImageRecord, []byte, error) {
	if err := ctx.Err(); err != nil {
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	if err := validateOpenAIGeneratedImageID(id); err != nil {
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	return s.loadValidated(ctx, id, "", s.effectiveRehydrateMaxBytes())
}

func (s *OpenAIGeneratedImageStore) loadWithMaxRehydrateBytes(ctx context.Context, id string, maxRehydrateBytes int64) (OpenAIGeneratedImageRecord, []byte, error) {
	if err := ctx.Err(); err != nil {
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	if err := validateOpenAIGeneratedImageID(id); err != nil {
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	if maxRehydrateBytes <= 0 {
		maxRehydrateBytes = s.effectiveRehydrateMaxBytes()
	}
	return s.loadValidated(ctx, id, "", maxRehydrateBytes)
}

func (s *OpenAIGeneratedImageStore) LoadByFilename(ctx context.Context, filename string) (OpenAIGeneratedImageRecord, []byte, error) {
	if err := ctx.Err(); err != nil {
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	id, _, err := validateOpenAIGeneratedImageFilename(filename)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	return s.loadValidated(ctx, id, filename, s.effectiveDownloadMaxBytes())
}

func (s *OpenAIGeneratedImageStore) effectiveRehydrateMaxBytes() int64 {
	maxRehydrateBytes := s.maxRehydrateBytes
	if maxRehydrateBytes == 0 {
		maxRehydrateBytes = defaultOpenAIGeneratedImageMaxRehydrateBytes
	}
	return maxRehydrateBytes
}

func (s *OpenAIGeneratedImageStore) effectiveDownloadMaxBytes() int64 {
	maxDownloadBytes := s.maxDecodedBytes
	if maxDownloadBytes == 0 {
		maxDownloadBytes = defaultOpenAIGeneratedImageMaxDecodedBytes
	}
	return maxDownloadBytes
}

func (s *OpenAIGeneratedImageStore) loadValidated(ctx context.Context, id string, requestedFilename string, maxReadBytes int64) (OpenAIGeneratedImageRecord, []byte, error) {
	metaPath, err := safeOpenAIGeneratedImagePath(s.root, id+".json")
	if err != nil {
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageNotFound
		}
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return OpenAIGeneratedImageRecord{}, nil, err
	}

	var rec OpenAIGeneratedImageRecord
	if err := json.Unmarshal(meta, &rec); err != nil {
		return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageNotFound
	}
	if rec.ID != id || rec.Filename == "" {
		return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageNotFound
	}
	filenameID, _, err := validateOpenAIGeneratedImageFilename(rec.Filename)
	if err != nil || filenameID != rec.ID {
		return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageInvalid
	}
	if requestedFilename != "" && rec.Filename != requestedFilename {
		return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageInvalid
	}
	if !s.now().Before(rec.ExpiresAt) {
		return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageExpired
	}

	imagePath, err := safeOpenAIGeneratedImagePath(s.root, rec.Filename)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	if maxReadBytes > 0 && rec.DecodedBytes > maxReadBytes {
		return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageTooLarge
	}
	data, err := readOpenAIGeneratedImageFile(ctx, imagePath, maxReadBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageNotFound
		}
		return OpenAIGeneratedImageRecord{}, nil, err
	}
	if rec.DecodedBytes > 0 && int64(len(data)) != rec.DecodedBytes {
		return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageInvalid
	}
	if rec.SHA256 != "" {
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != rec.SHA256 {
			return OpenAIGeneratedImageRecord{}, nil, errOpenAIGeneratedImageInvalid
		}
	}
	return rec, data, nil
}

func readOpenAIGeneratedImageFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var reader io.Reader = file
	if maxBytes > 0 {
		reader = io.LimitReader(file, maxBytes+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, errOpenAIGeneratedImageTooLarge
	}
	return data, nil
}

func (s *OpenAIGeneratedImageStore) tryPublishGeneratedImage(ctx context.Context, imagePath string, metaPath string, data []byte, meta []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if fileExists(imagePath) || fileExists(metaPath) {
		return true, nil
	}
	if err := s.ensureDirectoryCapacity(ctx, int64(len(data)+len(meta))); err != nil {
		return false, err
	}
	if err := atomicWriteOpenAIGeneratedImageFile(imagePath, data); err != nil {
		if errors.Is(err, os.ErrExist) {
			return true, nil
		}
		return false, err
	}
	if err := atomicWriteOpenAIGeneratedImageFile(metaPath, meta); err != nil {
		_ = os.Remove(imagePath)
		if errors.Is(err, os.ErrExist) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (s *OpenAIGeneratedImageStore) Cleanup(ctx context.Context, limit int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupLocked(ctx, limit)
}

func (s *OpenAIGeneratedImageStore) cleanupLocked(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	now := s.now()
	if limit <= 0 {
		s.lastCleanup = now
		return 0, nil
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.lastCleanup = now
			return 0, nil
		}
		return 0, err
	}

	removed := 0
	scanned := 0
	for _, entry := range entries {
		if scanned >= limit {
			break
		}
		scanned++
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if entry.IsDir() {
			continue
		}
		if tempRemoved, err := s.cleanupTempFileLocked(entry, now); err != nil {
			return removed, err
		} else if tempRemoved > 0 {
			removed += tempRemoved
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			orphanRemoved, err := s.cleanupOrphanImageLocked(entry, now)
			if err != nil {
				return removed, err
			}
			removed += orphanRemoved
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateOpenAIGeneratedImageID(id); err != nil {
			continue
		}
		metaPath, err := safeOpenAIGeneratedImagePath(s.root, entry.Name())
		if err != nil {
			continue
		}
		meta, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var rec OpenAIGeneratedImageRecord
		if err := json.Unmarshal(meta, &rec); err != nil || rec.ID != id {
			if err := openAIGeneratedImageRemoveFile(metaPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return removed, err
			}
			removed++
			continue
		}
		filenameID, _, err := validateOpenAIGeneratedImageFilename(rec.Filename)
		if err != nil || filenameID != rec.ID {
			if err := openAIGeneratedImageRemoveFile(metaPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return removed, err
			}
			removed++
			continue
		}
		if now.Before(rec.ExpiresAt) {
			continue
		}
		if imagePath, err := safeOpenAIGeneratedImagePath(s.root, rec.Filename); err == nil {
			if err := openAIGeneratedImageRemoveFile(imagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return removed, err
			}
		}
		if err := openAIGeneratedImageRemoveFile(metaPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
		removed++
	}
	s.lastCleanup = now
	return removed, nil
}

func (s *OpenAIGeneratedImageStore) cleanupOrphanImageLocked(entry os.DirEntry, now time.Time) (int, error) {
	id, _, err := validateOpenAIGeneratedImageFilename(entry.Name())
	if err != nil {
		return 0, nil
	}
	if !s.isOpenAIGeneratedImageOrphan(id, entry.Name()) {
		return 0, nil
	}
	info, err := openAIGeneratedImageEntryInfo(entry)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	if now.Sub(info.ModTime()) < s.maxAge {
		return 0, nil
	}
	path, err := safeOpenAIGeneratedImagePath(s.root, entry.Name())
	if err != nil {
		return 0, nil
	}
	if err := openAIGeneratedImageRemoveFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	return 1, nil
}

func (s *OpenAIGeneratedImageStore) cleanupTempFileLocked(entry os.DirEntry, now time.Time) (int, error) {
	if !openAIGeneratedImageTempPattern.MatchString(entry.Name()) {
		return 0, nil
	}
	info, err := openAIGeneratedImageEntryInfo(entry)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	if now.Sub(info.ModTime()) < s.maxAge {
		return 0, nil
	}
	path, err := safeOpenAIGeneratedImagePath(s.root, entry.Name())
	if err != nil {
		return 0, nil
	}
	if err := openAIGeneratedImageRemoveFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	return 1, nil
}

func (s *OpenAIGeneratedImageStore) isOpenAIGeneratedImageOrphan(id string, filename string) bool {
	metaPath, err := safeOpenAIGeneratedImagePath(s.root, id+".json")
	if err != nil {
		return false
	}
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	var rec OpenAIGeneratedImageRecord
	if err := json.Unmarshal(meta, &rec); err != nil {
		return true
	}
	return rec.ID != id || rec.Filename != filename
}

func (s *OpenAIGeneratedImageStore) maybeCleanupBeforeSave(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.shouldCleanupBeforeSaveLocked() {
		return nil
	}
	_, err := s.cleanupLocked(ctx, s.cleanupLimit)
	if err != nil && ctx.Err() == nil {
		s.lastCleanup = s.now()
		return nil
	}
	return err
}

func (s *OpenAIGeneratedImageStore) shouldCleanupBeforeSaveLocked() bool {
	if s.cleanupLimit <= 0 {
		return false
	}
	if s.lastCleanup.IsZero() {
		return true
	}
	interval := s.cleanupInterval
	if interval <= 0 {
		interval = defaultOpenAIGeneratedImageCleanupInterval
	}
	return s.now().Sub(s.lastCleanup) >= interval
}

func (s *OpenAIGeneratedImageStore) ensureDirectoryCapacity(ctx context.Context, candidateBytes int64) error {
	maxTotalBytes := s.maxTotalBytes
	if maxTotalBytes == 0 {
		maxTotalBytes = defaultOpenAIGeneratedImageMaxTotalBytes
	}
	if maxTotalBytes > 0 && candidateBytes > maxTotalBytes {
		return errOpenAIGeneratedImageTooLarge
	}
	total, err := s.directoryTotalBytes(ctx)
	if err != nil {
		return err
	}
	if maxTotalBytes > 0 && total+candidateBytes > maxTotalBytes {
		return errOpenAIGeneratedImageTooLarge
	}
	return nil
}

func (s *OpenAIGeneratedImageStore) directoryTotalBytes(ctx context.Context) (int64, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if entry.IsDir() {
			continue
		}
		info, err := openAIGeneratedImageEntryInfo(entry)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func validateOpenAIGeneratedImageFilename(filename string) (id string, format string, err error) {
	if hasUnsafeOpenAIGeneratedImageToken(filename) {
		return "", "", errOpenAIGeneratedImageInvalid
	}
	matches := openAIGeneratedImageFilenamePattern.FindStringSubmatch(filename)
	if matches == nil {
		return "", "", errOpenAIGeneratedImageInvalid
	}
	format = matches[2]
	if format == "jpg" {
		format = "jpeg"
	}
	return matches[1], format, nil
}

func validateOpenAIGeneratedImageID(id string) error {
	if hasUnsafeOpenAIGeneratedImageToken(id) || !openAIGeneratedImageIDPattern.MatchString(id) {
		return errOpenAIGeneratedImageInvalid
	}
	return nil
}

func safeOpenAIGeneratedImagePath(root string, filename string) (string, error) {
	if root == "" || hasUnsafeOpenAIGeneratedImageToken(filename) {
		return "", errOpenAIGeneratedImageInvalid
	}
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	cleanPath, err := filepath.Abs(filepath.Clean(filepath.Join(cleanRoot, filename)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errOpenAIGeneratedImageInvalid
	}
	return cleanPath, nil
}

func generateOpenAIGeneratedImageID(rand io.Reader) (string, error) {
	buf := make([]byte, 24)
	if _, err := io.ReadFull(rand, buf); err != nil {
		return "", err
	}
	return "img_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func sniffOpenAIGeneratedImage(data []byte, requested string) (format string, mime string, err error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if strings.HasPrefix(requested, ".") {
		requested = strings.TrimPrefix(requested, ".")
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return requireOpenAIGeneratedImageFormat("png", "image/png", requested)
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return requireOpenAIGeneratedImageFormat("jpeg", "image/jpeg", requested)
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return requireOpenAIGeneratedImageFormat("webp", "image/webp", requested)
	}
	return "", "", errOpenAIGeneratedImageInvalid
}

func requireOpenAIGeneratedImageFormat(actual string, mime string, requested string) (string, string, error) {
	if requested == "" || requested == actual || (actual == "jpeg" && requested == "jpg") {
		return actual, mime, nil
	}
	return "", "", errOpenAIGeneratedImageInvalid
}

func hasUnsafeOpenAIGeneratedImageToken(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(value, "/") ||
		strings.Contains(value, `\`) ||
		strings.Contains(value, "..") ||
		strings.Contains(value, "\x00") ||
		strings.Contains(lower, "%2f") ||
		strings.Contains(lower, "%5c")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func atomicWriteOpenAIGeneratedImageFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return publishOpenAIGeneratedImageNoClobber(tmpName, path)
}

func publishOpenAIGeneratedImageNoClobber(src string, dst string) error {
	if err := openAIGeneratedImageLinkFile(src, dst); err == nil {
		return nil
	} else if fileExists(dst) {
		return os.ErrExist
	}
	return copyOpenAIGeneratedImageNoClobber(src, dst)
}

func copyOpenAIGeneratedImageNoClobber(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return os.ErrExist
		}
		return err
	}
	cleanup := true
	defer func() {
		_ = out.Close()
		if cleanup {
			_ = os.Remove(dst)
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	cleanup = false
	return nil
}
