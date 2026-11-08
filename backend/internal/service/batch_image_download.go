package service

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	defaultBatchImageZipMaxItems          = 200
	defaultBatchImageZipMaxBytes          = 512 * 1024 * 1024
	defaultBatchImageDownloadDuration     = 10 * time.Minute
	defaultBatchImageDownloadConcurrency  = 1
	batchImageDownloadScannerMaxLineBytes = 16 * 1024 * 1024
)

var errBatchImageDownloadSizeExceeded = errors.New("batch image download size limit exceeded")

type BatchImageDownloadLimiter interface {
	Acquire(ctx context.Context, userID string, kind string) (BatchImageDownloadPermit, error)
}

type BatchImageDownloadPermit interface {
	Release(ctx context.Context) error
}

type BatchImageContentStream struct {
	Reader        io.ReadCloser
	ContentType   string
	Filename      string
	ContentLength *int64
}

type BatchImageZipOptions struct {
	Status          string
	MaxItems        int
	IncludeManifest bool
}

type BatchImageZipResult struct {
	FileCount  int
	ErrorCount int
}

type BatchImageLineImages struct {
	CustomID     string
	Images       []BatchImageInlineImage
	ErrorCode    string
	ErrorMessage string
}

type BatchImageInlineImage struct {
	MimeType   string
	Extension  string
	Base64Data string
}

type BatchImageDownloadService struct {
	Repo             BatchImageRepository
	ProviderRegistry *BatchImageProviderRegistry
	AccountResolver  BatchImageAccountResolver
	Limiter          BatchImageDownloadLimiter
	Config           *config.Config
}

type batchImageDownloadLimitWriter struct {
	w       io.Writer
	limit   int64
	written int64
}

func (w *batchImageDownloadLimitWriter) Write(p []byte) (int, error) {
	if w == nil || w.w == nil {
		return 0, io.ErrClosedPipe
	}
	if w.limit > 0 && w.written+int64(len(p)) > w.limit {
		return 0, errBatchImageDownloadSizeExceeded
	}
	n, err := w.w.Write(p)
	w.written += int64(n)
	return n, err
}

func NewBatchImageDownloadService(repo BatchImageRepository, accountRepo AccountRepository, limiter BatchImageDownloadLimiter, cfg *config.Config) *BatchImageDownloadService {
	return &BatchImageDownloadService{
		Repo:             repo,
		ProviderRegistry: NewBatchImageProviderRegistryFromConfig(cfg),
		AccountResolver:  &BatchImageAccountRepositoryResolver{Repo: accountRepo},
		Limiter:          limiter,
		Config:           cfg,
	}
}

func (s *BatchImageDownloadService) OpenItemContent(ctx context.Context, owner BatchImageOwner, batchID string, customID string, imageIndex int) (*BatchImageContentStream, error) {
	if imageIndex < 0 {
		return nil, ErrBatchImageItemImageIndexOutOfRange
	}
	job, err := s.getCompletedJob(ctx, owner, batchID)
	if err != nil {
		return nil, err
	}
	item, err := s.Repo.GetBatchImageItemForDownload(ctx, job.BatchID, customID)
	if err != nil {
		return nil, err
	}
	if item.Status != BatchImageItemStatusSuccess {
		return nil, ErrBatchImageItemFailed
	}
	if imageIndex >= item.ImageCount {
		return nil, ErrBatchImageItemImageIndexOutOfRange
	}

	permit, err := s.acquirePermit(ctx, owner.UserID, "item")
	if err != nil {
		return nil, err
	}
	releasePermit := true
	defer func() {
		if releasePermit && permit != nil {
			_ = permit.Release(ctx)
		}
	}()

	provider, account, err := s.providerAndAccount(ctx, job)
	if err != nil {
		return nil, err
	}
	r, _, err := provider.OpenResult(ctx, job, account)
	if err != nil {
		return nil, ErrBatchImageResultMissing.WithCause(err)
	}
	defer func() { _ = r.Close() }()

	line, err := findBatchImageLineImages(r, item.CustomID)
	if err != nil {
		return nil, err
	}
	if imageIndex >= len(line.Images) {
		return nil, ErrBatchImageItemImageIndexOutOfRange
	}
	image := line.Images[imageIndex]
	if strings.TrimSpace(image.Base64Data) == "" {
		return nil, ErrBatchImageResultMissing
	}
	contentType := strings.TrimSpace(image.MimeType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	extension := strings.TrimSpace(image.Extension)
	if extension == "" {
		extension = batchImageFileExtension(contentType)
	}
	if extension == "" {
		extension = "bin"
	}

	reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(image.Base64Data))
	releasePermit = false
	return &BatchImageContentStream{
		Reader:      &batchImagePermitReadCloser{Reader: reader, permit: permit},
		ContentType: contentType,
		Filename:    BatchImageSafeDownloadFilename(item.CustomID, extension),
	}, nil
}

func (s *BatchImageDownloadService) StreamZip(ctx context.Context, owner BatchImageOwner, batchID string, opts BatchImageZipOptions, w io.Writer) (*BatchImageZipResult, error) {
	job, err := s.getCompletedJob(ctx, owner, batchID)
	if err != nil {
		return nil, err
	}
	maxItems := opts.MaxItems
	if cap := s.maxZipItems(); maxItems <= 0 || maxItems > cap {
		// 客户端传入的 max_items 不得放大管理员配置的 ZIP 上限。
		maxItems = cap
	}
	if job.SuccessCount > maxItems {
		return nil, ErrBatchImageZipTooManyItems
	}
	successItems, err := s.Repo.ListBatchImageItemsForDownload(ctx, job.BatchID, BatchImageItemStatusSuccess, maxItems+1)
	if err != nil {
		return nil, err
	}
	if len(successItems) > maxItems {
		return nil, ErrBatchImageZipTooManyItems
	}
	failedItems, err := s.Repo.ListBatchImageItemsForDownload(ctx, job.BatchID, BatchImageItemStatusFailed, maxItems)
	if err != nil {
		return nil, err
	}

	permit, err := s.acquirePermit(ctx, owner.UserID, "zip")
	if err != nil {
		return nil, err
	}
	if permit != nil {
		defer func() { _ = permit.Release(ctx) }()
	}

	provider, account, err := s.providerAndAccount(ctx, job)
	if err != nil {
		return nil, err
	}
	r, _, err := provider.OpenResult(ctx, job, account)
	if err != nil {
		return nil, ErrBatchImageResultMissing.WithCause(err)
	}
	defer func() { _ = r.Close() }()

	streamCtx := ctx
	cancel := func() {}
	if d := s.maxDownloadDuration(); d > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, d)
	}
	defer cancel()

	limitedWriter := &batchImageDownloadLimitWriter{w: w, limit: s.maxDownloadBytes()}
	zipWriter := zip.NewWriter(limitedWriter)
	result, manifestFiles, zipErrors, err := s.writeZipImages(streamCtx, zipWriter, r, successItems)
	if err != nil {
		_ = zipWriter.Close()
		if errors.Is(err, errBatchImageDownloadSizeExceeded) {
			return result, ErrBatchImageDownloadTooLarge.WithCause(err)
		}
		return result, ErrBatchImageDownloadFailed.WithCause(err)
	}
	zipErrors = append(zipErrors, batchImageZipErrorsFromItems(failedItems)...)
	if err := writeBatchImageZipJSON(zipWriter, "manifest.json", batchImageZipManifest{
		BatchID:      job.BatchID,
		Model:        job.Model,
		ItemCount:    job.ItemCount,
		SuccessCount: job.SuccessCount,
		FailCount:    job.FailCount,
		Files:        manifestFiles,
	}); err != nil {
		_ = zipWriter.Close()
		if errors.Is(err, errBatchImageDownloadSizeExceeded) {
			return result, ErrBatchImageDownloadTooLarge.WithCause(err)
		}
		return result, ErrBatchImageDownloadFailed.WithCause(err)
	}
	if err := writeBatchImageZipJSON(zipWriter, "errors.json", zipErrors); err != nil {
		_ = zipWriter.Close()
		if errors.Is(err, errBatchImageDownloadSizeExceeded) {
			return result, ErrBatchImageDownloadTooLarge.WithCause(err)
		}
		return result, ErrBatchImageDownloadFailed.WithCause(err)
	}
	result.ErrorCount = len(zipErrors)
	if err := zipWriter.Close(); err != nil {
		if errors.Is(err, errBatchImageDownloadSizeExceeded) {
			return result, ErrBatchImageDownloadTooLarge.WithCause(err)
		}
		return result, ErrBatchImageDownloadFailed.WithCause(err)
	}
	return result, nil
}

func (s *BatchImageDownloadService) writeZipImages(ctx context.Context, zipWriter *zip.Writer, resultReader io.Reader, successItems []*BatchImageItem) (*BatchImageZipResult, []batchImageZipManifestFile, []batchImageZipError, error) {
	successByID := make(map[string]*BatchImageItem, len(successItems))
	missing := make(map[string]struct{}, len(successItems))
	for _, item := range successItems {
		if item == nil {
			continue
		}
		successByID[item.CustomID] = item
		missing[item.CustomID] = struct{}{}
	}
	scanner := bufio.NewScanner(resultReader)
	scanner.Buffer(make([]byte, 0, 64*1024), batchImageDownloadScannerMaxLineBytes)

	result := &BatchImageZipResult{}
	var manifestFiles []batchImageZipManifestFile
	var zipErrors []batchImageZipError
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return result, manifestFiles, zipErrors, err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		images, err := ExtractBatchImagePartsFromResultLine([]byte(line))
		if err != nil {
			return result, manifestFiles, zipErrors, err
		}
		item := successByID[images.CustomID]
		if item == nil {
			continue
		}
		delete(missing, images.CustomID)
		if len(images.Images) == 0 {
			zipErrors = append(zipErrors, batchImageZipError{CustomID: images.CustomID, Code: "EMPTY_IMAGE_OUTPUT", Message: "provider response contained no image output"})
			continue
		}
		for idx, image := range images.Images {
			extension := image.Extension
			if extension == "" {
				extension = "bin"
			}
			filename := batchImageZipImageFilename(item.CustomID, idx, extension)
			entry, err := zipWriter.CreateHeader(&zip.FileHeader{Name: filename, Method: zip.Deflate})
			if err != nil {
				return result, manifestFiles, zipErrors, err
			}
			decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(image.Base64Data))
			if _, err := io.Copy(entry, decoder); err != nil {
				zipErrors = append(zipErrors, batchImageZipError{CustomID: item.CustomID, Code: "IMAGE_DECODE_FAILED", Message: "image data could not be decoded"})
				continue
			}
			result.FileCount++
			manifestFiles = append(manifestFiles, batchImageZipManifestFile{
				CustomID:   item.CustomID,
				Filename:   filename,
				MimeType:   image.MimeType,
				ImageIndex: idx,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return result, manifestFiles, zipErrors, err
	}
	missingIDs := make([]string, 0, len(missing))
	for customID := range missing {
		missingIDs = append(missingIDs, customID)
	}
	sort.Strings(missingIDs)
	for _, customID := range missingIDs {
		zipErrors = append(zipErrors, batchImageZipError{CustomID: customID, Code: "RESULT_MISSING", Message: "provider result was not found for item"})
	}
	return result, manifestFiles, zipErrors, nil
}

func (s *BatchImageDownloadService) getCompletedJob(ctx context.Context, owner BatchImageOwner, batchID string) (*BatchImageJob, error) {
	if s == nil || s.Repo == nil {
		return nil, ErrBatchImageDownloadFailed
	}
	job, err := s.Repo.GetBatchImageJobForDownload(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case BatchImageJobStatusCompleted:
		return job, nil
	case BatchImageJobStatusOutputDeleted:
		return nil, ErrBatchImageOutputDeleted
	default:
		return nil, ErrBatchImageNotReady
	}
}

func (s *BatchImageDownloadService) providerAndAccount(ctx context.Context, job *BatchImageJob) (BatchImageProvider, *Account, error) {
	if s == nil || s.ProviderRegistry == nil || s.AccountResolver == nil || job == nil {
		return nil, nil, ErrBatchImageDownloadFailed
	}
	provider, ok := s.ProviderRegistry.Get(job.Provider)
	if !ok || provider == nil {
		return nil, nil, ErrBatchImageUnsupportedProvider
	}
	if job.AccountID == nil || *job.AccountID <= 0 {
		return nil, nil, ErrBatchImageMissingAccountID
	}
	account, err := s.AccountResolver.ResolveBatchImageAccount(ctx, *job.AccountID)
	if err != nil {
		return nil, nil, ErrBatchImageDownloadFailed
	}
	if !provider.SupportsAccount(account) {
		return nil, nil, ErrBatchImageProviderUnsupportedAccount
	}
	return provider, account, nil
}

func (s *BatchImageDownloadService) acquirePermit(ctx context.Context, userID int64, kind string) (BatchImageDownloadPermit, error) {
	if s == nil || s.Limiter == nil {
		return nil, nil
	}
	permit, err := s.Limiter.Acquire(ctx, fmt.Sprintf("%d", userID), kind)
	if err != nil {
		if infraerrors.Code(err) == http.StatusTooManyRequests {
			return nil, ErrBatchImageDownloadLimited
		}
		return nil, ErrBatchImageDownloadLimited.WithCause(err)
	}
	return permit, nil
}

func (s *BatchImageDownloadService) maxZipItems() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxDownloadItemsZip > 0 {
		return s.Config.BatchImage.MaxDownloadItemsZip
	}
	return defaultBatchImageZipMaxItems
}

func (s *BatchImageDownloadService) maxDownloadBytes() int64 {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxDownloadBytesPerRequest > 0 {
		return s.Config.BatchImage.MaxDownloadBytesPerRequest
	}
	return defaultBatchImageZipMaxBytes
}

func (s *BatchImageDownloadService) maxDownloadDuration() time.Duration {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxDownloadDurationSeconds > 0 {
		return time.Duration(s.Config.BatchImage.MaxDownloadDurationSeconds) * time.Second
	}
	return defaultBatchImageDownloadDuration
}

func ExtractBatchImagePartsFromResultLine(line []byte) (*BatchImageLineImages, error) {
	var obj map[string]any
	if err := json.Unmarshal(line, &obj); err != nil {
		return nil, ErrBatchImageIndexParseFailed.WithCause(err)
	}
	customID := batchImageFirstNonEmptyString(
		batchImageMapString(obj, "key"),
		batchImageMapString(obj, "custom_id"),
		batchImageMapString(obj, "customId"),
		batchImageNestedString(obj, "request", "key"),
	)
	if customID == "" {
		return nil, ErrBatchImageIndexParseFailed.WithCause(fmt.Errorf("missing custom id"))
	}
	out := &BatchImageLineImages{CustomID: customID}
	out.Images = append(out.Images, extractBatchImageInlineImages(batchImageNestedAny(obj, "response", "candidates"))...)
	out.Images = append(out.Images, extractBatchImageInlineImages(obj["candidates"])...)
	if len(out.Images) > 0 {
		return out, nil
	}
	if code, message, ok := batchImageFailureFromProviderFields(obj); ok {
		out.ErrorCode = code
		out.ErrorMessage = truncateBatchImageMessage(message, batchImageMaxErrorMessageLength)
		return out, nil
	}
	if _, hasResponse := obj["response"]; hasResponse || batchImageHasCandidates(obj) {
		out.ErrorCode = "EMPTY_IMAGE_OUTPUT"
		out.ErrorMessage = "provider response contained no image output"
		return out, nil
	}
	out.ErrorCode = "PROVIDER_ITEM_FAILED"
	out.ErrorMessage = "provider result line contained no image output"
	return out, nil
}

func extractBatchImageInlineImages(raw any) []BatchImageInlineImage {
	candidates, ok := raw.([]any)
	if !ok {
		return nil
	}
	var images []BatchImageInlineImage
	for _, candidateRaw := range candidates {
		candidate, ok := candidateRaw.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := batchImageNestedAny(candidate, "content", "parts").([]any)
		if !ok {
			continue
		}
		for _, partRaw := range parts {
			part, ok := partRaw.(map[string]any)
			if !ok {
				continue
			}
			inline, ok := firstMap(part["inlineData"], part["inline_data"])
			if !ok {
				continue
			}
			data := strings.TrimSpace(batchImageMapString(inline, "data"))
			mime := strings.TrimSpace(batchImageFirstNonEmptyString(batchImageMapString(inline, "mimeType"), batchImageMapString(inline, "mime_type")))
			if data == "" || !strings.HasPrefix(strings.ToLower(mime), "image/") {
				continue
			}
			images = append(images, BatchImageInlineImage{
				MimeType:   mime,
				Extension:  batchImageFileExtension(mime),
				Base64Data: data,
			})
		}
	}
	return images
}

func findBatchImageLineImages(r io.Reader, customID string) (*BatchImageLineImages, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), batchImageDownloadScannerMaxLineBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parsed, err := ExtractBatchImagePartsFromResultLine([]byte(line))
		if err != nil {
			return nil, err
		}
		if parsed.CustomID != customID {
			continue
		}
		if len(parsed.Images) == 0 {
			if parsed.ErrorCode != "" {
				return nil, ErrBatchImageItemFailed
			}
			return nil, ErrBatchImageResultMissing
		}
		return parsed, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, ErrBatchImageDownloadFailed.WithCause(err)
	}
	return nil, ErrBatchImageResultMissing
}

func BatchImageSafeDownloadFilename(customID, extension string) string {
	base := sanitizeBatchImageFilenameBase(customID)
	extension = sanitizeBatchImageFilenameExtension(extension)
	if extension == "" {
		extension = "bin"
	}
	return base + "." + extension
}

func BatchImageContentDispositionAttachment(filename string) string {
	filename = strings.ReplaceAll(filename, "\\", "_")
	filename = strings.ReplaceAll(filename, `"`, "_")
	filename = sanitizeBatchImageFilenameBase(strings.TrimSuffix(filename, filepath.Ext(filename))) + filepath.Ext(filename)
	return `attachment; filename="` + filename + `"`
}

func sanitizeBatchImageFilenameBase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "image"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == 0:
			_ = b.WriteByte('_')
		case unicode.IsControl(r):
			_ = b.WriteByte('_')
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.':
			_, _ = b.WriteRune(r)
		default:
			_ = b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), ". ")
	for strings.Contains(out, "..") {
		out = strings.ReplaceAll(out, "..", "_")
	}
	out = strings.Trim(out, ". ")
	if out == "" {
		out = "image"
	}
	if len(out) > 120 {
		out = strings.TrimRight(out[:120], ". ")
	}
	if out == "" {
		out = "image"
	}
	return out
}

func sanitizeBatchImageFilenameExtension(extension string) string {
	extension = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(extension)), ".")
	var b strings.Builder
	for _, r := range extension {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			_, _ = b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func batchImageZipImageFilename(customID string, imageIndex int, extension string) string {
	base := sanitizeBatchImageFilenameBase(customID)
	if imageIndex > 0 {
		base = fmt.Sprintf("%s_%d", base, imageIndex+1)
	}
	return "images/" + BatchImageSafeDownloadFilename(base, extension)
}

func writeBatchImageZipJSON(zipWriter *zip.Writer, name string, value any) error {
	entry, err := zipWriter.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

type batchImageZipManifest struct {
	BatchID      string                      `json:"batch_id"`
	Model        string                      `json:"model"`
	ItemCount    int                         `json:"item_count"`
	SuccessCount int                         `json:"success_count"`
	FailCount    int                         `json:"fail_count"`
	Files        []batchImageZipManifestFile `json:"files"`
}

type batchImageZipManifestFile struct {
	CustomID   string `json:"custom_id"`
	Filename   string `json:"filename"`
	MimeType   string `json:"mime_type"`
	ImageIndex int    `json:"image_index"`
}

type batchImageZipError struct {
	CustomID string `json:"custom_id"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func batchImageZipErrorsFromItems(items []*BatchImageItem) []batchImageZipError {
	out := make([]batchImageZipError, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, batchImageZipError{
			CustomID: item.CustomID,
			Code:     batchImageDerefString(item.ErrorCode),
			Message:  sanitizeBatchImagePublicMessage(batchImageDerefString(item.ErrorMessage)),
		})
	}
	return out
}

type batchImagePermitReadCloser struct {
	io.Reader
	permit BatchImageDownloadPermit
	once   sync.Once
	err    error
}

func (r *batchImagePermitReadCloser) Close() error {
	r.once.Do(func() {
		if r.permit != nil {
			r.err = r.permit.Release(context.Background())
		}
	})
	return r.err
}
