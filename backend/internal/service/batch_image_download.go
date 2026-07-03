package service

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
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
	defaultBatchImageZipMaxItems          = 1000
	defaultBatchImageDownloadDuration     = 10 * time.Minute
	defaultBatchImageDownloadConcurrency  = 2
	batchImageDownloadScannerMaxLineBytes = 16 * 1024 * 1024
)

type BatchImageDownloadLimiter interface {
	Acquire(ctx context.Context, userID string, kind string) (BatchImageDownloadPermit, error)
REDACTED

type BatchImageDownloadPermit interface {
	Release(ctx context.Context) error
REDACTED

type BatchImageContentStream struct {
	Reader        io.ReadCloser
	ContentType   string
	Filename      string
	ContentLength *int64
REDACTED

type BatchImageZipOptions struct {
	Status          string
	MaxItems        int
	IncludeManifest bool
REDACTED

type BatchImageZipResult struct {
	FileCount  int
	ErrorCount int
REDACTED

type BatchImageLineImages struct {
	CustomID     string
	Images       []BatchImageInlineImage
	ErrorCode    string
	ErrorMessage string
REDACTED

type BatchImageInlineImage struct {
	MimeType   string
	Extension  string
	Base64Data string
REDACTED

type BatchImageDownloadService struct {
	Repo             BatchImageRepository
	ProviderRegistry *BatchImageProviderRegistry
	AccountResolver  BatchImageAccountResolver
	Limiter          BatchImageDownloadLimiter
	Config           *config.Config
REDACTED

func NewBatchImageDownloadService(repo BatchImageRepository, accountRepo AccountRepository, limiter BatchImageDownloadLimiter, cfg *config.Config) *BatchImageDownloadService {
	return &BatchImageDownloadService{
		Repo:             repo,
		ProviderRegistry: NewDefaultBatchImageProviderRegistry(),
		AccountResolver:  &BatchImageAccountRepositoryResolver{Repo: accountRepoREDACTED,
		Limiter:          limiter,
		Config:           cfg,
REDACTED
REDACTED

func (s *BatchImageDownloadService) OpenItemContent(ctx context.Context, owner BatchImageOwner, batchID string, customID string, imageIndex int) (*BatchImageContentStream, error) {
	if imageIndex < 0 {
		return nil, ErrBatchImageItemImageIndexOutOfRange
REDACTED
	job, err := s.getCompletedJob(ctx, owner, batchID)
	if err != nil {
		return nil, err
REDACTED
	item, err := s.Repo.GetBatchImageItemForDownload(ctx, job.BatchID, customID)
	if err != nil {
		return nil, err
REDACTED
	if item.Status != BatchImageItemStatusSuccess {
		return nil, ErrBatchImageItemFailed
REDACTED
	if imageIndex >= item.ImageCount {
		return nil, ErrBatchImageItemImageIndexOutOfRange
REDACTED

	permit, err := s.acquirePermit(ctx, owner.UserID, "item")
	if err != nil {
		return nil, err
REDACTED
	releasePermit := true
	defer func() {
		if releasePermit && permit != nil {
			_ = permit.Release(ctx)
	REDACTED
REDACTED()

	provider, account, err := s.providerAndAccount(ctx, job)
	if err != nil {
		return nil, err
REDACTED
	r, _, err := provider.OpenResult(ctx, job, account)
	if err != nil {
		return nil, ErrBatchImageResultMissing.WithCause(err)
REDACTED
	defer r.Close()

	line, err := findBatchImageLineImages(r, item.CustomID)
	if err != nil {
		return nil, err
REDACTED
	if imageIndex >= len(line.Images) {
		return nil, ErrBatchImageItemImageIndexOutOfRange
REDACTED
	image := line.Images[imageIndex]
	if strings.TrimSpace(image.Base64Data) == "" {
		return nil, ErrBatchImageResultMissing
REDACTED
	contentType := strings.TrimSpace(image.MimeType)
	if contentType == "" {
		contentType = "application/octet-stream"
REDACTED
	extension := strings.TrimSpace(image.Extension)
	if extension == "" {
		extension = batchImageFileExtension(contentType)
REDACTED
	if extension == "" {
		extension = "bin"
REDACTED

	reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(image.Base64Data))
	releasePermit = false
	return &BatchImageContentStream{
		Reader:      &batchImagePermitReadCloser{Reader: reader, permit: permitREDACTED,
		ContentType: contentType,
		Filename:    BatchImageSafeDownloadFilename(item.CustomID, extension),
REDACTED, nil
REDACTED

func (s *BatchImageDownloadService) StreamZip(ctx context.Context, owner BatchImageOwner, batchID string, opts BatchImageZipOptions, w io.Writer) (*BatchImageZipResult, error) {
	job, err := s.getCompletedJob(ctx, owner, batchID)
	if err != nil {
		return nil, err
REDACTED
	maxItems := opts.MaxItems
	if maxItems <= 0 {
		maxItems = s.maxZipItems()
REDACTED
	if job.SuccessCount > maxItems {
		return nil, ErrBatchImageZipTooManyItems
REDACTED
	successItems, err := s.Repo.ListBatchImageItemsForDownload(ctx, job.BatchID, BatchImageItemStatusSuccess, maxItems+1)
	if err != nil {
		return nil, err
REDACTED
	if len(successItems) > maxItems {
		return nil, ErrBatchImageZipTooManyItems
REDACTED
	failedItems, err := s.Repo.ListBatchImageItemsForDownload(ctx, job.BatchID, BatchImageItemStatusFailed, maxItems)
	if err != nil {
		return nil, err
REDACTED

	permit, err := s.acquirePermit(ctx, owner.UserID, "zip")
	if err != nil {
		return nil, err
REDACTED
	if permit != nil {
		defer permit.Release(ctx)
REDACTED

	provider, account, err := s.providerAndAccount(ctx, job)
	if err != nil {
		return nil, err
REDACTED
	r, _, err := provider.OpenResult(ctx, job, account)
	if err != nil {
		return nil, ErrBatchImageResultMissing.WithCause(err)
REDACTED
	defer r.Close()

	streamCtx := ctx
	cancel := func() {REDACTED
	if d := s.maxDownloadDuration(); d > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, d)
REDACTED
	defer cancel()

	zipWriter := zip.NewWriter(w)
	result, manifestFiles, zipErrors, err := s.writeZipImages(streamCtx, zipWriter, r, successItems)
	if err != nil {
		_ = zipWriter.Close()
		return result, ErrBatchImageDownloadFailed.WithCause(err)
REDACTED
	zipErrors = append(zipErrors, batchImageZipErrorsFromItems(failedItems)...)
	if err := writeBatchImageZipJSON(zipWriter, "manifest.json", batchImageZipManifest{
		BatchID:      job.BatchID,
		Model:        job.Model,
		ItemCount:    job.ItemCount,
		SuccessCount: job.SuccessCount,
		FailCount:    job.FailCount,
		Files:        manifestFiles,
REDACTED); err != nil {
		_ = zipWriter.Close()
		return result, ErrBatchImageDownloadFailed.WithCause(err)
REDACTED
	if err := writeBatchImageZipJSON(zipWriter, "errors.json", zipErrors); err != nil {
		_ = zipWriter.Close()
		return result, ErrBatchImageDownloadFailed.WithCause(err)
REDACTED
	result.ErrorCount = len(zipErrors)
	if err := zipWriter.Close(); err != nil {
		return result, ErrBatchImageDownloadFailed.WithCause(err)
REDACTED
	return result, nil
REDACTED

func (s *BatchImageDownloadService) writeZipImages(ctx context.Context, zipWriter *zip.Writer, resultReader io.Reader, successItems []*BatchImageItem) (*BatchImageZipResult, []batchImageZipManifestFile, []batchImageZipError, error) {
	successByID := make(map[string]*BatchImageItem, len(successItems))
	missing := make(map[string]struct{REDACTED, len(successItems))
	for _, item := range successItems {
		if item == nil {
			continue
	REDACTED
		successByID[item.CustomID] = item
		missing[item.CustomID] = struct{REDACTED{REDACTED
REDACTED
	scanner := bufio.NewScanner(resultReader)
	scanner.Buffer(make([]byte, 0, 64*1024), batchImageDownloadScannerMaxLineBytes)

	result := &BatchImageZipResult{REDACTED
	var manifestFiles []batchImageZipManifestFile
	var zipErrors []batchImageZipError
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return result, manifestFiles, zipErrors, err
	REDACTED
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
	REDACTED
		images, err := ExtractBatchImagePartsFromResultLine([]byte(line))
		if err != nil {
			return result, manifestFiles, zipErrors, err
	REDACTED
		item := successByID[images.CustomID]
		if item == nil {
			continue
	REDACTED
		delete(missing, images.CustomID)
		if len(images.Images) == 0 {
			zipErrors = append(zipErrors, batchImageZipError{CustomID: images.CustomID, Code: "EMPTY_IMAGE_OUTPUT", Message: "provider response contained no image output"REDACTED)
			continue
	REDACTED
		for idx, image := range images.Images {
			extension := image.Extension
			if extension == "" {
				extension = "bin"
		REDACTED
			filename := batchImageZipImageFilename(item.CustomID, idx, extension)
			entry, err := zipWriter.CreateHeader(&zip.FileHeader{Name: filename, Method: zip.DeflateREDACTED)
			if err != nil {
				return result, manifestFiles, zipErrors, err
		REDACTED
			decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(image.Base64Data))
			if _, err := io.Copy(entry, decoder); err != nil {
				zipErrors = append(zipErrors, batchImageZipError{CustomID: item.CustomID, Code: "IMAGE_DECODE_FAILED", Message: "image data could not be decoded"REDACTED)
				continue
		REDACTED
			result.FileCount++
			manifestFiles = append(manifestFiles, batchImageZipManifestFile{
				CustomID:   item.CustomID,
				Filename:   filename,
				MimeType:   image.MimeType,
				ImageIndex: idx,
		REDACTED)
	REDACTED
REDACTED
	if err := scanner.Err(); err != nil {
		return result, manifestFiles, zipErrors, err
REDACTED
	missingIDs := make([]string, 0, len(missing))
	for customID := range missing {
		missingIDs = append(missingIDs, customID)
REDACTED
	sort.Strings(missingIDs)
	for _, customID := range missingIDs {
		zipErrors = append(zipErrors, batchImageZipError{CustomID: customID, Code: "RESULT_MISSING", Message: "provider result was not found for item"REDACTED)
REDACTED
	return result, manifestFiles, zipErrors, nil
REDACTED

func (s *BatchImageDownloadService) getCompletedJob(ctx context.Context, owner BatchImageOwner, batchID string) (*BatchImageJob, error) {
	if s == nil || s.Repo == nil {
		return nil, ErrBatchImageDownloadFailed
REDACTED
	job, err := s.Repo.GetBatchImageJobForDownload(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return nil, err
REDACTED
	switch job.Status {
	case BatchImageJobStatusCompleted:
		return job, nil
	case BatchImageJobStatusOutputDeleted:
		return nil, ErrBatchImageOutputDeleted
	default:
		return nil, ErrBatchImageNotReady
REDACTED
REDACTED

func (s *BatchImageDownloadService) providerAndAccount(ctx context.Context, job *BatchImageJob) (BatchImageProvider, *Account, error) {
	if s == nil || s.ProviderRegistry == nil || s.AccountResolver == nil || job == nil {
		return nil, nil, ErrBatchImageDownloadFailed
REDACTED
	provider, ok := s.ProviderRegistry.Get(job.Provider)
	if !ok || provider == nil {
		return nil, nil, ErrBatchImageUnsupportedProvider
REDACTED
	if job.AccountID == nil || *job.AccountID <= 0 {
		return nil, nil, ErrBatchImageMissingAccountID
REDACTED
	account, err := s.AccountResolver.ResolveBatchImageAccount(ctx, *job.AccountID)
	if err != nil {
		return nil, nil, ErrBatchImageDownloadFailed
REDACTED
	if !provider.SupportsAccount(account) {
		return nil, nil, ErrBatchImageProviderUnsupportedAccount
REDACTED
	return provider, account, nil
REDACTED

func (s *BatchImageDownloadService) acquirePermit(ctx context.Context, userID int64, kind string) (BatchImageDownloadPermit, error) {
	if s == nil || s.Limiter == nil {
		return nil, nil
REDACTED
	permit, err := s.Limiter.Acquire(ctx, fmt.Sprintf("%d", userID), kind)
	if err != nil {
		if infraerrors.Code(err) == http.StatusTooManyRequests {
			return nil, ErrBatchImageDownloadLimited
	REDACTED
		return nil, ErrBatchImageDownloadLimited.WithCause(err)
REDACTED
	return permit, nil
REDACTED

func (s *BatchImageDownloadService) maxZipItems() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxDownloadItemsZip > 0 {
		return s.Config.BatchImage.MaxDownloadItemsZip
REDACTED
	return defaultBatchImageZipMaxItems
REDACTED

func (s *BatchImageDownloadService) maxDownloadDuration() time.Duration {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxDownloadDurationSeconds > 0 {
		return time.Duration(s.Config.BatchImage.MaxDownloadDurationSeconds) * time.Second
REDACTED
	return defaultBatchImageDownloadDuration
REDACTED

func ExtractBatchImagePartsFromResultLine(line []byte) (*BatchImageLineImages, error) {
	var obj map[string]any
	if err := json.Unmarshal(line, &obj); err != nil {
		return nil, ErrBatchImageIndexParseFailed.WithCause(err)
REDACTED
	customID := batchImageFirstNonEmptyString(
		batchImageMapString(obj, "key"),
		batchImageMapString(obj, "custom_id"),
		batchImageMapString(obj, "customId"),
		batchImageNestedString(obj, "request", "key"),
	)
	if customID == "" {
		return nil, ErrBatchImageIndexParseFailed.WithCause(fmt.Errorf("missing custom id"))
REDACTED
	out := &BatchImageLineImages{CustomID: customIDREDACTED
	out.Images = append(out.Images, extractBatchImageInlineImages(batchImageNestedAny(obj, "response", "candidates"))...)
	out.Images = append(out.Images, extractBatchImageInlineImages(obj["candidates"])...)
	if len(out.Images) > 0 {
		return out, nil
REDACTED
	if code, message, ok := batchImageFailureFromProviderFields(obj); ok {
		out.ErrorCode = code
		out.ErrorMessage = truncateBatchImageMessage(message, batchImageMaxErrorMessageLength)
		return out, nil
REDACTED
	if _, hasResponse := obj["response"]; hasResponse || batchImageHasCandidates(obj) {
		out.ErrorCode = "EMPTY_IMAGE_OUTPUT"
		out.ErrorMessage = "provider response contained no image output"
		return out, nil
REDACTED
	out.ErrorCode = "PROVIDER_ITEM_FAILED"
	out.ErrorMessage = "provider result line contained no image output"
	return out, nil
REDACTED

func extractBatchImageInlineImages(raw any) []BatchImageInlineImage {
	candidates, ok := raw.([]any)
	if !ok {
		return nil
REDACTED
	var images []BatchImageInlineImage
	for _, candidateRaw := range candidates {
		candidate, ok := candidateRaw.(map[string]any)
		if !ok {
			continue
	REDACTED
		parts, ok := batchImageNestedAny(candidate, "content", "parts").([]any)
		if !ok {
			continue
	REDACTED
		for _, partRaw := range parts {
			part, ok := partRaw.(map[string]any)
			if !ok {
				continue
		REDACTED
			inline, ok := firstMap(part["inlineData"], part["inline_data"])
			if !ok {
				continue
		REDACTED
			data := strings.TrimSpace(batchImageMapString(inline, "data"))
			mime := strings.TrimSpace(batchImageFirstNonEmptyString(batchImageMapString(inline, "mimeType"), batchImageMapString(inline, "mime_type")))
			if data == "" || !strings.HasPrefix(strings.ToLower(mime), "image/") {
				continue
		REDACTED
			images = append(images, BatchImageInlineImage{
				MimeType:   mime,
				Extension:  batchImageFileExtension(mime),
				Base64Data: data,
		REDACTED)
	REDACTED
REDACTED
	return images
REDACTED

func findBatchImageLineImages(r io.Reader, customID string) (*BatchImageLineImages, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), batchImageDownloadScannerMaxLineBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
	REDACTED
		parsed, err := ExtractBatchImagePartsFromResultLine([]byte(line))
		if err != nil {
			return nil, err
	REDACTED
		if parsed.CustomID != customID {
			continue
	REDACTED
		if len(parsed.Images) == 0 {
			if parsed.ErrorCode != "" {
				return nil, ErrBatchImageItemFailed
		REDACTED
			return nil, ErrBatchImageResultMissing
	REDACTED
		return parsed, nil
REDACTED
	if err := scanner.Err(); err != nil {
		return nil, ErrBatchImageDownloadFailed.WithCause(err)
REDACTED
	return nil, ErrBatchImageResultMissing
REDACTED

func BatchImageSafeDownloadFilename(customID, extension string) string {
	base := sanitizeBatchImageFilenameBase(customID)
	extension = sanitizeBatchImageFilenameExtension(extension)
	if extension == "" {
		extension = "bin"
REDACTED
	return base + "." + extension
REDACTED

func BatchImageContentDispositionAttachment(filename string) string {
	filename = strings.ReplaceAll(filename, "\\", "_")
	filename = strings.ReplaceAll(filename, `"`, "_")
	filename = sanitizeBatchImageFilenameBase(strings.TrimSuffix(filename, filepath.Ext(filename))) + filepath.Ext(filename)
	return `attachment; filename="` + filename + `"`
REDACTED

func sanitizeBatchImageFilenameBase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "image"
REDACTED
	var b strings.Builder
	for _, r := range value {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == 0:
			b.WriteByte('_')
		case unicode.IsControl(r):
			b.WriteByte('_')
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
	REDACTED
REDACTED
	out := strings.Trim(b.String(), ". ")
	for strings.Contains(out, "..") {
		out = strings.ReplaceAll(out, "..", "_")
REDACTED
	out = strings.Trim(out, ". ")
	if out == "" {
		out = "image"
REDACTED
	if len(out) > 120 {
		out = strings.TrimRight(out[:120], ". ")
REDACTED
	if out == "" {
		out = "image"
REDACTED
	return out
REDACTED

func sanitizeBatchImageFilenameExtension(extension string) string {
	extension = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(extension)), ".")
	var b strings.Builder
	for _, r := range extension {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
	REDACTED
REDACTED
	out := b.String()
	if len(out) > 12 {
		out = out[:12]
REDACTED
	return out
REDACTED

func batchImageZipImageFilename(customID string, imageIndex int, extension string) string {
	base := sanitizeBatchImageFilenameBase(customID)
	if imageIndex > 0 {
		base = fmt.Sprintf("%s_%d", base, imageIndex+1)
REDACTED
	return "images/" + BatchImageSafeDownloadFilename(base, extension)
REDACTED

func writeBatchImageZipJSON(zipWriter *zip.Writer, name string, value any) error {
	entry, err := zipWriter.CreateHeader(&zip.FileHeader{Name: name, Method: zip.DeflateREDACTED)
	if err != nil {
		return err
REDACTED
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
REDACTED

type batchImageZipManifest struct {
	BatchID      string                      `json:"batch_id"`
	Model        string                      `json:"model"`
	ItemCount    int                         `json:"item_count"`
	SuccessCount int                         `json:"success_count"`
	FailCount    int                         `json:"fail_count"`
	Files        []batchImageZipManifestFile `json:"files"`
REDACTED

type batchImageZipManifestFile struct {
	CustomID   string `json:"custom_id"`
	Filename   string `json:"filename"`
	MimeType   string `json:"mime_type"`
	ImageIndex int    `json:"image_index"`
REDACTED

type batchImageZipError struct {
	CustomID string `json:"custom_id"`
	Code     string `json:"code"`
	Message  string `json:"message"`
REDACTED

func batchImageZipErrorsFromItems(items []*BatchImageItem) []batchImageZipError {
	out := make([]batchImageZipError, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
	REDACTED
		out = append(out, batchImageZipError{
			CustomID: item.CustomID,
			Code:     batchImageDerefString(item.ErrorCode),
			Message:  sanitizeBatchImagePublicMessage(batchImageDerefString(item.ErrorMessage)),
	REDACTED)
REDACTED
	return out
REDACTED

type batchImagePermitReadCloser struct {
	io.Reader
	permit BatchImageDownloadPermit
	once   sync.Once
	err    error
REDACTED

func (r *batchImagePermitReadCloser) Close() error {
	r.once.Do(func() {
		if r.permit != nil {
			r.err = r.permit.Release(context.Background())
	REDACTED
REDACTED)
	return r.err
REDACTED
