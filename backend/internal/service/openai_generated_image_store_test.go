package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

var fixedNow = time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
var pngBytes = []byte("\x89PNG\r\n\x1a\nimage-bytes")
var pngB64 = base64.StdEncoding.EncodeToString(pngBytes)

type incrementingRandReader struct{ next byte }

func (r *incrementingRandReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.next
		r.next++
	}
	return len(p), nil
}

type blockingCollisionRandReader struct {
	collisionReads int
	collisionBytes []byte

	mu      sync.Mutex
	calls   int
	ready   chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingCollisionRandReader(collisionReads int, collisionBytes []byte) *blockingCollisionRandReader {
	return &blockingCollisionRandReader{
		collisionReads: collisionReads,
		collisionBytes: collisionBytes,
		ready:          make(chan struct{}),
		release:        make(chan struct{}),
	}
}

func (r *blockingCollisionRandReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	call := r.calls
	r.calls++
	if call+1 == r.collisionReads {
		r.once.Do(func() { close(r.ready) })
	}
	r.mu.Unlock()

	if call < r.collisionReads {
		<-r.release
		copy(p, r.collisionBytes)
		return len(p), nil
	}
	for i := range p {
		p[i] = byte(call + i)
	}
	return len(p), nil
}

func newTestOpenAIGeneratedImageStore(t *testing.T, now time.Time) *OpenAIGeneratedImageStore {
	t.Helper()
	store := NewOpenAIGeneratedImageStore(t.TempDir())
	store.now = func() time.Time { return now }
	store.rand = &incrementingRandReader{}
	store.maxEncodedBytes = 32 << 20
	store.maxDecodedBytes = 20 << 20
	store.maxRehydrateBytes = 20 << 20
	return store
}

func TestOpenAIGeneratedImageStore_SaveLoadAndExpire(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)

	rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{
		Base64:       pngB64,
		OutputFormat: "png",
		SourceItemID: "ig_123",
	})
	require.NoError(t, err)
	require.Regexp(t, `^img_[A-Za-z0-9_-]{32,}$`, rec.ID)
	require.Equal(t, "png", rec.Format)
	require.Equal(t, "image/png", rec.MIME)
	require.Equal(t, fixedNow.Add(time.Hour), rec.ExpiresAt)

	loaded, data, err := store.Load(context.Background(), rec.ID)
	require.NoError(t, err)
	require.Equal(t, rec.ID, loaded.ID)
	require.Equal(t, pngBytes, data)

	store.now = func() time.Time { return fixedNow.Add(time.Hour + time.Second) }
	_, _, err = store.Load(context.Background(), rec.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageExpired)
}

func TestOpenAIGeneratedImageStore_RejectsMalformedAndUnsupportedImages(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	_, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: "%%%", OutputFormat: "png"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageInvalid)

	gif := base64.StdEncoding.EncodeToString([]byte("GIF89a"))
	_, err = store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: gif, OutputFormat: "gif"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageInvalid)
}

func TestOpenAIGeneratedImageStore_RejectsOversizedImages(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.maxDecodedBytes = 4
	_, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageTooLarge)
}

func TestOpenAIGeneratedImageStore_RejectsOversizedRehydrateFile(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	store.maxRehydrateBytes = int64(len(pngBytes) - 1)

	_, _, err = store.Load(context.Background(), rec.ID)

	require.ErrorIs(t, err, errOpenAIGeneratedImageTooLarge)
}

func TestOpenAIGeneratedImageStore_LoadByFilenameIgnoresRehydrateLimit(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	store.maxRehydrateBytes = int64(len(pngBytes) - 1)

	_, _, err = store.Load(context.Background(), rec.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageTooLarge)
	loaded, data, err := store.LoadByFilename(context.Background(), rec.Filename)

	require.NoError(t, err)
	require.Equal(t, rec.ID, loaded.ID)
	require.Equal(t, pngBytes, data)
}

func TestOpenAIGeneratedImageStore_RejectsDecodedSizeMismatch(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	rec.DecodedBytes++
	meta, err := json.Marshal(rec)
	require.NoError(t, err)
	metaPath, err := safeOpenAIGeneratedImagePath(store.root, rec.ID+".json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metaPath, meta, 0o600))

	_, _, err = store.Load(context.Background(), rec.ID)

	require.ErrorIs(t, err, errOpenAIGeneratedImageInvalid)
}

func TestOpenAIGeneratedImageStore_RejectsSHA256Mismatch(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	imagePath, err := safeOpenAIGeneratedImagePath(store.root, rec.Filename)
	require.NoError(t, err)
	mutated := append([]byte(nil), pngBytes...)
	mutated[len(mutated)-1] ^= 0x01
	require.NoError(t, os.WriteFile(imagePath, mutated, 0o600))

	_, _, err = store.Load(context.Background(), rec.ID)

	require.ErrorIs(t, err, errOpenAIGeneratedImageInvalid)
}

func TestOpenAIGeneratedImageStore_RejectsOversizedEncodedInputBeforeDecode(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.maxEncodedBytes = 8
	oversized := base64.StdEncoding.EncodeToString([]byte(string(pngBytes) + strings.Repeat("x", 64)))
	_, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: oversized, OutputFormat: "png"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageTooLarge)
}

func TestOpenAIGeneratedImageStore_RejectsInvalidIDOnLoad(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range []string{"img_short", "../img_abcdefghijklmnopqrstuvwxyzABCDEF", `img_abc\def`, "img_abc%2fdef", "img_abc\x00def"} {
		_, _, err := store.Load(context.Background(), id)
		require.Error(t, err, id)
	}
}

func TestOpenAIGeneratedImageStore_ValidateFilenameRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../img_a.png", `img_a\b.png`, "img_a%2f.png", "img_a\x00.png", "img_short.png"} {
		_, _, err := validateOpenAIGeneratedImageFilename(name)
		require.Error(t, err, name)
	}
}

func TestOpenAIGeneratedImageStore_CleanupDeletesOnlyExpired(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.cleanupInterval = 24 * time.Hour
	expired, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)

	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	fresh, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)

	store.now = func() time.Time { return fixedNow.Add(2*time.Hour + time.Minute) }
	removed, err := store.Cleanup(context.Background(), 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, removed, 1)

	_, _, err = store.Load(context.Background(), expired.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageNotFound)
	_, _, err = store.Load(context.Background(), fresh.ID)
	require.NoError(t, err)
}

func TestOpenAIGeneratedImageStore_CleanupKeepsMetadataWhenImageRemoveFails(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }

	imagePath, err := safeOpenAIGeneratedImagePath(store.root, rec.Filename)
	require.NoError(t, err)
	metaPath, err := safeOpenAIGeneratedImagePath(store.root, rec.ID+".json")
	require.NoError(t, err)
	removeErr := errors.New("remove image failed")
	originalRemoveFile := openAIGeneratedImageRemoveFile
	openAIGeneratedImageRemoveFile = func(path string) error {
		if path == imagePath {
			return removeErr
		}
		return originalRemoveFile(path)
	}
	t.Cleanup(func() { openAIGeneratedImageRemoveFile = originalRemoveFile })

	removed, err := store.Cleanup(context.Background(), 10)
	require.ErrorIs(t, err, removeErr)
	require.Equal(t, 0, removed)
	_, err = os.Stat(metaPath)
	require.NoError(t, err)
	_, _, err = store.Load(context.Background(), rec.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageExpired)
}

func TestOpenAIGeneratedImageStore_SaveContinuesAfterOpportunisticCleanupFailureAndThrottles(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.cleanupLimit = 10
	store.cleanupInterval = time.Minute
	store.maxTotalBytes = 1 << 20
	expired, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)

	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	imagePath, err := safeOpenAIGeneratedImagePath(store.root, expired.Filename)
	require.NoError(t, err)
	removeErr := errors.New("cleanup image remove failed")
	removeCalls := 0
	originalRemoveFile := openAIGeneratedImageRemoveFile
	openAIGeneratedImageRemoveFile = func(path string) error {
		if path == imagePath {
			removeCalls++
			return removeErr
		}
		return originalRemoveFile(path)
	}
	t.Cleanup(func() { openAIGeneratedImageRemoveFile = originalRemoveFile })

	fresh, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	require.Equal(t, 1, removeCalls)
	_, _, err = store.Load(context.Background(), fresh.ID)
	require.NoError(t, err)

	secondFresh, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	require.Equal(t, 1, removeCalls)
	_, _, err = store.Load(context.Background(), secondFresh.ID)
	require.NoError(t, err)
}

func TestOpenAIGeneratedImageStore_CleanupDeletesOnlyExpiredOrphanImages(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	require.NoError(t, os.MkdirAll(store.root, 0o755))
	expiredName := "img_abcdefghijklmnopqrstuvwxyzABCDEF.png"
	freshName := "img_abcdefghijklmnopqrstuvwxyzABCDEG.png"
	expiredPath, err := safeOpenAIGeneratedImagePath(store.root, expiredName)
	require.NoError(t, err)
	freshPath, err := safeOpenAIGeneratedImagePath(store.root, freshName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(expiredPath, pngBytes, 0o600))
	require.NoError(t, os.WriteFile(freshPath, pngBytes, 0o600))
	require.NoError(t, os.Chtimes(expiredPath, fixedNow.Add(-2*time.Hour), fixedNow.Add(-2*time.Hour)))
	require.NoError(t, os.Chtimes(freshPath, fixedNow.Add(-30*time.Minute), fixedNow.Add(-30*time.Minute)))

	removed, err := store.Cleanup(context.Background(), 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, removed, 1)
	_, err = os.Stat(expiredPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(freshPath)
	require.NoError(t, err)
}

func TestOpenAIGeneratedImageStore_CleanupDeletesOnlyStaleTempFiles(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	require.NoError(t, os.MkdirAll(store.root, 0o755))
	staleTemp := filepath.Join(store.root, ".img_abcdefghijklmnopqrstuvwxyzABCDEF.png.123.tmp")
	freshTemp := filepath.Join(store.root, ".img_abcdefghijklmnopqrstuvwxyzABCDEG.png.456.tmp")
	staleMetaTemp := filepath.Join(store.root, ".img_abcdefghijklmnopqrstuvwxyzABCDEH.json.789.tmp")
	freshMetaTemp := filepath.Join(store.root, ".img_abcdefghijklmnopqrstuvwxyzABCDEI.json.987.tmp")
	unrelatedHidden := filepath.Join(store.root, ".not-sub2api.tmp")
	require.NoError(t, os.WriteFile(staleTemp, []byte("stale"), 0o600))
	require.NoError(t, os.WriteFile(freshTemp, []byte("fresh"), 0o600))
	require.NoError(t, os.WriteFile(staleMetaTemp, []byte("stale-meta"), 0o600))
	require.NoError(t, os.WriteFile(freshMetaTemp, []byte("fresh-meta"), 0o600))
	require.NoError(t, os.WriteFile(unrelatedHidden, []byte("keep"), 0o600))
	require.NoError(t, os.Chtimes(staleTemp, fixedNow.Add(-2*time.Hour), fixedNow.Add(-2*time.Hour)))
	require.NoError(t, os.Chtimes(freshTemp, fixedNow.Add(-30*time.Minute), fixedNow.Add(-30*time.Minute)))
	require.NoError(t, os.Chtimes(staleMetaTemp, fixedNow.Add(-2*time.Hour), fixedNow.Add(-2*time.Hour)))
	require.NoError(t, os.Chtimes(freshMetaTemp, fixedNow.Add(-30*time.Minute), fixedNow.Add(-30*time.Minute)))
	require.NoError(t, os.Chtimes(unrelatedHidden, fixedNow.Add(-2*time.Hour), fixedNow.Add(-2*time.Hour)))

	removed, err := store.Cleanup(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 2, removed)
	_, err = os.Stat(staleTemp)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(staleMetaTemp)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(freshTemp)
	require.NoError(t, err)
	_, err = os.Stat(freshMetaTemp)
	require.NoError(t, err)
	_, err = os.Stat(unrelatedHidden)
	require.NoError(t, err)
}

func TestOpenAIGeneratedImageStore_CleanupMetadataFilenameMismatchDoesNotDeleteOtherImage(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.cleanupInterval = 24 * time.Hour
	expired, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)

	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	fresh, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)

	expired.Filename = fresh.Filename
	meta, err := json.Marshal(expired)
	require.NoError(t, err)
	metaPath, err := safeOpenAIGeneratedImagePath(store.root, expired.ID+".json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metaPath, meta, 0o600))

	removed, err := store.Cleanup(context.Background(), 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, removed, 1)
	_, _, err = store.Load(context.Background(), expired.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageNotFound)
	_, _, err = store.Load(context.Background(), fresh.ID)
	require.NoError(t, err)
}

func TestOpenAIGeneratedImageStore_CleanupLimitCountsNonJSONEntriesAndDeletesCorruptMetadata(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	require.NoError(t, os.MkdirAll(store.root, 0o755))
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("000_orphan_%d.tmp", i)
		path, err := safeOpenAIGeneratedImagePath(store.root, name)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, []byte("orphan"), 0o600))
	}

	expired, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	removed, err := store.Cleanup(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, 0, removed)
	_, _, err = store.Load(context.Background(), expired.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageExpired)

	corruptID, err := generateOpenAIGeneratedImageID(bytes.NewReader(bytes.Repeat([]byte{9}, 24)))
	require.NoError(t, err)
	corruptMetaPath, err := safeOpenAIGeneratedImagePath(store.root, corruptID+".json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(corruptMetaPath, []byte("not-json"), 0o600))
	removed, err = store.Cleanup(context.Background(), 100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, removed, 1)
	_, err = os.Stat(corruptMetaPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestOpenAIGeneratedImageStore_DirectoryTotalBytesIgnoresVanishedEntries(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	require.NoError(t, os.MkdirAll(store.root, 0o755))
	keptPath := filepath.Join(store.root, "kept.tmp")
	vanishedName := "vanished.tmp"
	vanishedPath := filepath.Join(store.root, vanishedName)
	require.NoError(t, os.WriteFile(keptPath, []byte("kept"), 0o600))
	require.NoError(t, os.WriteFile(vanishedPath, []byte("vanished"), 0o600))

	originalEntryInfo := openAIGeneratedImageEntryInfo
	openAIGeneratedImageEntryInfo = func(entry os.DirEntry) (os.FileInfo, error) {
		if entry.Name() == vanishedName {
			return nil, os.ErrNotExist
		}
		return originalEntryInfo(entry)
	}
	t.Cleanup(func() { openAIGeneratedImageEntryInfo = originalEntryInfo })

	total, err := store.directoryTotalBytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(len("kept")), total)
}

func TestOpenAIGeneratedImageStore_PublishNoClobberFallsBackWhenHardlinkUnsupported(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.png")
	originalLinkFile := openAIGeneratedImageLinkFile
	openAIGeneratedImageLinkFile = func(_, _ string) error { return os.ErrInvalid }
	t.Cleanup(func() { openAIGeneratedImageLinkFile = originalLinkFile })

	require.NoError(t, atomicWriteOpenAIGeneratedImageFile(target, []byte("fallback")))
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, []byte("fallback"), data)
}

func TestOpenAIGeneratedImageStore_PublishNoClobberFallbackDoesNotOverwriteExistingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.png")
	require.NoError(t, os.WriteFile(target, []byte("existing"), 0o600))
	originalLinkFile := openAIGeneratedImageLinkFile
	openAIGeneratedImageLinkFile = func(_, _ string) error { return os.ErrInvalid }
	t.Cleanup(func() { openAIGeneratedImageLinkFile = originalLinkFile })

	err := atomicWriteOpenAIGeneratedImageFile(target, []byte("new"))
	require.ErrorIs(t, err, os.ErrExist)
	data, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, []byte("existing"), data)
}

func (s *OpenAIGeneratedImageStore) saveDecodedForTest(id string, format string, data []byte) (OpenAIGeneratedImageRecord, error) {
	if err := validateOpenAIGeneratedImageID(id); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	format, mime, err := sniffOpenAIGeneratedImage(data, format)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	filename := id + "." + format
	imagePath, err := safeOpenAIGeneratedImagePath(s.root, filename)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	hash := sha256.Sum256(data)
	rec := OpenAIGeneratedImageRecord{
		ID:           id,
		Filename:     filename,
		Format:       format,
		MIME:         mime,
		SHA256:       hex.EncodeToString(hash[:]),
		DecodedBytes: int64(len(data)),
		CreatedAt:    s.now(),
		ExpiresAt:    s.now().Add(time.Hour),
	}
	if err := os.WriteFile(imagePath, data, 0o600); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	meta, err := json.Marshal(rec)
	if err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	if err := os.WriteFile(filepath.Join(s.root, id+".json"), meta, 0o600); err != nil {
		return OpenAIGeneratedImageRecord{}, err
	}
	return rec, nil
}

func TestOpenAIGeneratedImageStore_SaveRunsThrottledCleanup(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.cleanupLimit = 10
	store.cleanupInterval = time.Minute
	expired, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)

	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	fresh, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)

	_, _, err = store.Load(context.Background(), expired.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageNotFound)
	_, _, err = store.Load(context.Background(), fresh.ID)
	require.NoError(t, err)
}

func TestOpenAIGeneratedImageStore_DefaultLimitsAreSafe(t *testing.T) {
	store := NewOpenAIGeneratedImageStore(t.TempDir())
	require.Positive(t, store.maxEncodedBytes)
	require.Positive(t, store.maxDecodedBytes)
	require.Positive(t, store.maxRehydrateBytes)
	require.Positive(t, store.maxTotalBytes)
	require.Positive(t, store.cleanupLimit)
	require.Positive(t, store.cleanupInterval)
	require.GreaterOrEqual(t, store.maxEncodedBytes, store.maxDecodedBytes)
	require.Greater(t, store.maxTotalBytes, store.maxDecodedBytes)
}

func TestOpenAIGeneratedImageStore_SaveRetriesOnIDCollisionAndDoesNotOverwriteExistingResource(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	collisionBytes := bytes.Repeat([]byte{1}, 24)
	collisionID, err := generateOpenAIGeneratedImageID(bytes.NewReader(collisionBytes))
	require.NoError(t, err)
	oldBytes := []byte("\x89PNG\r\n\x1a\nold-image")
	oldRec, err := store.saveDecodedForTest(collisionID, "png", oldBytes)
	require.NoError(t, err)

	store.rand = io.MultiReader(bytes.NewReader(collisionBytes), bytes.NewReader(bytes.Repeat([]byte{2}, 24)))
	rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	require.NotEqual(t, oldRec.ID, rec.ID)

	loaded, data, err := store.Load(context.Background(), oldRec.ID)
	require.NoError(t, err)
	require.Equal(t, oldRec.ID, loaded.ID)
	require.Equal(t, oldBytes, data)
}

func TestOpenAIGeneratedImageStore_LoadByFilenameRejectsMetadataFilenameMismatch(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	original, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)
	originalFilename := original.Filename

	otherBytes := []byte("\x89PNG\r\n\x1a\nother-image")
	otherB64 := base64.StdEncoding.EncodeToString(otherBytes)
	other, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: otherB64, OutputFormat: "png"})
	require.NoError(t, err)

	original.Filename = other.Filename
	meta, err := json.Marshal(original)
	require.NoError(t, err)
	metaPath, err := safeOpenAIGeneratedImagePath(store.root, original.ID+".json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metaPath, meta, 0o600))

	_, _, err = store.LoadByFilename(context.Background(), originalFilename)
	require.Error(t, err)
	require.ErrorIs(t, err, errOpenAIGeneratedImageInvalid)
}

func TestOpenAIGeneratedImageStore_ConcurrentIDCollisionDoesNotOverwriteOrFail(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.cleanupLimit = 0

	workerCount := 16
	store.rand = newBlockingCollisionRandReader(workerCount, bytes.Repeat([]byte{7}, 24))
	type saveResult struct {
		rec  OpenAIGeneratedImageRecord
		data []byte
		err  error
	}
	results := make([]saveResult, workerCount)
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := []byte("\x89PNG\r\n\x1a\n" + fmt.Sprintf("image-%02d", i))
			rec, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{
				Base64:       base64.StdEncoding.EncodeToString(data),
				OutputFormat: "png",
			})
			results[i] = saveResult{rec: rec, data: data, err: err}
		}()
	}

	<-store.rand.(*blockingCollisionRandReader).ready
	close(store.rand.(*blockingCollisionRandReader).release)
	wg.Wait()

	seen := map[string]bool{}
	for _, result := range results {
		require.NoError(t, result.err)
		require.False(t, seen[result.rec.ID], "duplicate id %s", result.rec.ID)
		seen[result.rec.ID] = true
		_, data, err := store.Load(context.Background(), result.rec.ID)
		require.NoError(t, err)
		require.Equal(t, result.data, data)
	}
}

func TestOpenAIGeneratedImageStore_StartupCleanupKeepsFreshFiles(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	fresh, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)

	removed, err := store.Cleanup(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, removed)
	_, _, err = store.Load(context.Background(), fresh.ID)
	require.NoError(t, err)
}

func TestOpenAIGeneratedImageStore_RejectsDirectoryCapacityOverflowAfterCleanup(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	store.cleanupLimit = 10
	expired, err := store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.NoError(t, err)

	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	store.maxTotalBytes = int64(len(pngBytes) - 1)
	_, err = store.SaveBase64(context.Background(), OpenAIGeneratedImageSaveInput{Base64: pngB64, OutputFormat: "png"})
	require.ErrorIs(t, err, errOpenAIGeneratedImageTooLarge)

	_, _, err = store.Load(context.Background(), expired.ID)
	require.ErrorIs(t, err, errOpenAIGeneratedImageNotFound)
}

func TestOpenAIGeneratedImageStore_ResolveRootUsesDataDirEnv(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA_DIR", dataDir)

	root := resolveOpenAIGeneratedImageRoot(&config.Config{})

	require.Equal(t, filepath.Join(dataDir, "openai-generated-images"), root)
}

func TestOpenAIGeneratedImageStore_RootIsAbsoluteAndStableAcrossCWDChange(t *testing.T) {
	originalWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(originalWD)) }()

	parent := t.TempDir()
	require.NoError(t, os.Chdir(parent))
	store := NewOpenAIGeneratedImageStore("generated-images")
	require.True(t, filepath.IsAbs(store.root))
	root := store.root

	require.NoError(t, os.Chdir(t.TempDir()))
	path, err := safeOpenAIGeneratedImagePath(store.root, "img_abcdefghijklmnopqrstuvwxyzABCDEF.png")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "img_abcdefghijklmnopqrstuvwxyzABCDEF.png"), path)
}
