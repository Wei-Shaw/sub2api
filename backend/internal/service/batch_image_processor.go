package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	BatchImageParsedStatusSucceeded = "succeeded"
	BatchImageParsedStatusFailed    = "failed"

	defaultBatchImageProcessorRequeue = 30 * time.Second
	batchImageProviderErrorRequeue    = time.Minute
	batchImageMaxErrorMessageLength   = 1000
)

type BatchImageAccountResolver interface {
	ResolveBatchImageAccount(ctx context.Context, accountID int64) (*Account, error)
REDACTED

type BatchImageAccountLookup interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
REDACTED

type BatchImageAccountRepositoryResolver struct {
	Repo BatchImageAccountLookup
REDACTED

func (r *BatchImageAccountRepositoryResolver) ResolveBatchImageAccount(ctx context.Context, accountID int64) (*Account, error) {
	if r == nil || r.Repo == nil {
		return nil, ErrAccountNotFound
REDACTED
	return r.Repo.GetByID(ctx, accountID)
REDACTED

type BatchImageProviderProcessor struct {
	Repo             BatchImageRepository
	ProviderRegistry *BatchImageProviderRegistry
	AccountResolver  BatchImageAccountResolver
	Indexer          *BatchImageResultIndexer
	BillingRepo      UsageBillingRepository
	AuthCache        APIKeyAuthCacheInvalidator
	DefaultRequeue   time.Duration
REDACTED

func (p *BatchImageProviderProcessor) Process(ctx context.Context, batchID string) (BatchImageProcessResult, error) {
	if p == nil || p.Repo == nil || p.ProviderRegistry == nil || p.AccountResolver == nil {
		return BatchImageProcessResult{REDACTED, infraerrors.New(http.StatusInternalServerError, "BATCH_IMAGE_PROCESSOR_NOT_CONFIGURED", "batch image processor is not configured")
REDACTED

	job, err := p.Repo.GetBatchImageJobByBatchID(ctx, batchID)
	if err != nil {
		return BatchImageProcessResult{REDACTED, err
REDACTED
	if isBatchImageProcessorDoneStatus(job.Status) {
		if err := p.releaseTerminalHold(ctx, job); err != nil {
			return BatchImageProcessResult{REDACTED, err
	REDACTED
		return BatchImageProcessResult{Terminal: trueREDACTED, nil
REDACTED

	provider, ok := p.ProviderRegistry.Get(job.Provider)
	if !ok || provider == nil {
		return BatchImageProcessResult{REDACTED, ErrBatchImageUnsupportedProvider
REDACTED
	if job.AccountID == nil || *job.AccountID <= 0 {
		return BatchImageProcessResult{REDACTED, ErrBatchImageMissingAccountID
REDACTED
	account, err := p.AccountResolver.ResolveBatchImageAccount(ctx, *job.AccountID)
	if err != nil {
		return BatchImageProcessResult{REDACTED, err
REDACTED
	if !provider.SupportsAccount(account) {
		return BatchImageProcessResult{REDACTED, ErrBatchImageProviderUnsupportedAccount
REDACTED
	if strings.TrimSpace(batchImageDerefString(job.ProviderJobName)) == "" {
		return BatchImageProcessResult{REDACTED, ErrBatchImageMissingProviderJobName
REDACTED

	if job.Status == BatchImageJobStatusIndexing {
		return p.indexAndSettle(ctx, job, provider, account)
REDACTED

	status, err := provider.Get(ctx, job, account)
	if err != nil {
		logger.L().Warn("batch_image.provider_status_check_failed",
			zap.String("batch_id", job.BatchID),
			zap.String("provider", job.Provider),
			zap.String("provider_job_name", batchImageDerefString(job.ProviderJobName)),
			zap.Error(err),
		)
		return BatchImageProcessResult{RequeueAfter: batchImageProviderErrorRequeueREDACTED, nil
REDACTED
	if status == nil {
		return BatchImageProcessResult{RequeueAfter: p.requeueDelay(0)REDACTED, nil
REDACTED
	if err := p.persistProviderOutputRef(ctx, job, status.ProviderOutputRef); err != nil {
		return BatchImageProcessResult{REDACTED, err
REDACTED

	switch status.InternalState {
	case BatchProviderStateQueued:
		return BatchImageProcessResult{RequeueAfter: p.requeueDelay(status.SuggestedRequeueAfter)REDACTED, nil
	case BatchProviderStateRunning:
		if job.Status != BatchImageJobStatusRunning {
			if err := p.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusRunning, BatchImageTransitionOptions{
				EventType:    "provider_status_checked",
				EventPayload: map[string]any{"provider_state": status.RawStateREDACTED,
		REDACTED); err != nil {
				return BatchImageProcessResult{REDACTED, err
		REDACTED
			job.Status = BatchImageJobStatusRunning
	REDACTED
		return BatchImageProcessResult{RequeueAfter: p.requeueDelay(status.SuggestedRequeueAfter)REDACTED, nil
	case BatchProviderStateSucceeded:
		if job.Status != BatchImageJobStatusIndexing {
			if err := p.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusIndexing, BatchImageTransitionOptions{
				EventType:    "indexing_started",
				EventPayload: map[string]any{"provider_state": status.RawStateREDACTED,
		REDACTED); err != nil {
				return BatchImageProcessResult{REDACTED, err
		REDACTED
			job.Status = BatchImageJobStatusIndexing
	REDACTED
		return p.indexAndSettle(ctx, job, provider, account)
	case BatchProviderStateFailed, BatchProviderStateExpired:
		code := strings.TrimSpace(status.ErrorCode)
		if code == "" && status.InternalState == BatchProviderStateExpired {
			code = "PROVIDER_BATCH_EXPIRED"
	REDACTED
		if code == "" {
			code = "PROVIDER_BATCH_FAILED"
	REDACTED
		msg := truncateBatchImageMessage(status.ErrorMessage, batchImageMaxErrorMessageLength)
		if err := p.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusFailed, BatchImageTransitionOptions{
			EventType:    "job_failed",
			EventPayload: map[string]any{"provider_state": status.RawState, "error_code": codeREDACTED,
			ErrorCode:    batchImageStringPtr(code),
			ErrorMessage: batchImageOptionalStringPtr(msg),
	REDACTED); err != nil {
			return BatchImageProcessResult{REDACTED, err
	REDACTED
		job.Status = BatchImageJobStatusFailed
		if err := p.releaseTerminalHold(ctx, job); err != nil {
			return BatchImageProcessResult{REDACTED, err
	REDACTED
		return BatchImageProcessResult{Terminal: trueREDACTED, nil
	case BatchProviderStateCancelled:
		if err := p.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusCancelled, BatchImageTransitionOptions{
			EventType:    "job_failed",
			EventPayload: map[string]any{"provider_state": status.RawState, "error_code": "PROVIDER_BATCH_CANCELLED"REDACTED,
	REDACTED); err != nil {
			return BatchImageProcessResult{REDACTED, err
	REDACTED
		job.Status = BatchImageJobStatusCancelled
		if err := p.releaseTerminalHold(ctx, job); err != nil {
			return BatchImageProcessResult{REDACTED, err
	REDACTED
		return BatchImageProcessResult{Terminal: trueREDACTED, nil
	default:
		return BatchImageProcessResult{RequeueAfter: p.requeueDelay(status.SuggestedRequeueAfter)REDACTED, nil
REDACTED
REDACTED

func (p *BatchImageProviderProcessor) indexAndSettle(ctx context.Context, job *BatchImageJob, provider BatchImageProvider, account *Account) (BatchImageProcessResult, error) {
	indexer := p.Indexer
	if indexer == nil {
		indexer = &BatchImageResultIndexer{Repo: p.RepoREDACTED
REDACTED
	if indexer.Repo == nil {
		indexer.Repo = p.Repo
REDACTED

	result, err := indexer.Index(ctx, job, provider, account)
	if err != nil {
		if errors.Is(err, ErrBatchImageIndexOutputMissing) {
			return BatchImageProcessResult{REDACTED, err
	REDACTED
		// job 状态已被并发方推进（如已进入 settling/终态）：不是索引数据问题，
		// 短延迟 requeue 让下一轮按最新状态处理，不能误转 failed。
		if errors.Is(err, ErrBatchImageIndexStateConflict) {
			return BatchImageProcessResult{RequeueAfter: time.MillisecondREDACTED, nil
	REDACTED
		code := "INDEX_PARSE_FAILED"
		if errors.Is(err, ErrBatchImageDuplicateCustomID) {
			code = "DUPLICATE_CUSTOM_ID_IN_OUTPUT"
	REDACTED
		msg := truncateBatchImageMessage(err.Error(), batchImageMaxErrorMessageLength)
		transitionErr := p.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusFailed, BatchImageTransitionOptions{
			EventType:    "indexing_failed",
			EventPayload: map[string]any{"error_code": codeREDACTED,
			ErrorCode:    batchImageStringPtr(code),
			ErrorMessage: batchImageOptionalStringPtr(msg),
	REDACTED)
		if transitionErr != nil {
			return BatchImageProcessResult{REDACTED, transitionErr
	REDACTED
		job.Status = BatchImageJobStatusFailed
		if err := p.releaseTerminalHold(ctx, job); err != nil {
			return BatchImageProcessResult{REDACTED, err
	REDACTED
		return BatchImageProcessResult{Terminal: trueREDACTED, nil
REDACTED

	if err := p.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusSettling, BatchImageTransitionOptions{
		EventType: "indexing_completed",
		EventPayload: map[string]any{
			"success_count": result.SuccessCount,
			"fail_count":    result.FailCount,
			"total_count":   result.TotalCount,
	REDACTED,
REDACTED); err != nil {
		return BatchImageProcessResult{REDACTED, err
REDACTED
	return BatchImageProcessResult{RequeueAfter: time.MillisecondREDACTED, nil
REDACTED

func (p *BatchImageProviderProcessor) releaseTerminalHold(ctx context.Context, job *BatchImageJob) error {
	if p == nil || job == nil {
		return nil
REDACTED
	if job.Status != BatchImageJobStatusFailed && job.Status != BatchImageJobStatusCancelled {
		return nil
REDACTED
	if err := releaseBatchImageBalanceHold(ctx, p.BillingRepo, job, batchImageDerefString(job.RequestHash)); err != nil {
		return err
REDACTED
	if p.AuthCache != nil && job.UserID > 0 {
		p.AuthCache.InvalidateAuthCacheByUserID(ctx, job.UserID)
REDACTED
	return nil
REDACTED

func (p *BatchImageProviderProcessor) persistProviderOutputRef(ctx context.Context, job *BatchImageJob, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" || job == nil || batchImageDerefString(job.ProviderOutputRef) == ref {
		return nil
REDACTED
	if err := p.Repo.UpdateBatchImageJobProviderOutputRef(ctx, job.BatchID, ref); err != nil {
		return err
REDACTED
	job.ProviderOutputRef = &ref
	return nil
REDACTED

func (p *BatchImageProviderProcessor) requeueDelay(suggested time.Duration) time.Duration {
	if suggested > 0 {
		return suggested
REDACTED
	if p != nil && p.DefaultRequeue > 0 {
		return p.DefaultRequeue
REDACTED
	return defaultBatchImageProcessorRequeue
REDACTED

func isBatchImageProcessorDoneStatus(status string) bool {
	if status == BatchImageJobStatusSettling {
		return true
REDACTED
	return IsTerminalBatchImageJobStatus(status)
REDACTED

type BatchImageIndexResult struct {
	SuccessCount int
	FailCount    int
	TotalCount   int
REDACTED

type BatchImageResultIndexer struct {
	Repo BatchImageRepository
REDACTED

func (i *BatchImageResultIndexer) Index(ctx context.Context, job *BatchImageJob, provider BatchImageProvider, account *Account) (*BatchImageIndexResult, error) {
	if i == nil || i.Repo == nil || job == nil || provider == nil {
		return nil, ErrBatchImageIndexOutputMissing
REDACTED
	expected, err := i.listExpectedCustomIDs(ctx, job.BatchID)
	if err != nil {
		return nil, err
REDACTED

	r, _, err := provider.OpenResult(ctx, job, account)
	if err != nil {
		return nil, ErrBatchImageIndexOutputMissing.WithCause(err)
REDACTED
	defer func() { _ = r.Close() REDACTED()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	seen := make(map[string]int)
	unknownCount := 0
	var items []CreateBatchImageItemParams
	result := &BatchImageIndexResult{REDACTED
	lineNumber := 0
	now := time.Now()
	sourceObject := batchImageDerefString(job.ProviderOutputRef)
	if sourceObject == "" {
		sourceObject = batchImageDerefString(job.ProviderJobName)
REDACTED

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
	REDACTED
		parsed, err := ParseBatchImageResultLine([]byte(line), lineNumber)
		if err != nil {
			return nil, err
	REDACTED
		// 与提交时的 custom_id 集对账：provider 输出中未知/多余的行不能进入 item 表，
		// 否则 success+fail > item_count 会让结算永远校验失败。
		if len(expected) > 0 {
			if _, ok := expected[parsed.CustomID]; !ok {
				unknownCount++
				continue
		REDACTED
	REDACTED
		if firstLine, ok := seen[parsed.CustomID]; ok {
			return nil, ErrBatchImageDuplicateCustomID.WithCause(fmt.Errorf("custom id %q duplicated at lines %d and %d", parsed.CustomID, firstLine, lineNumber))
	REDACTED
		seen[parsed.CustomID] = lineNumber

		lineNo := parsed.SourceLineNumber
		item := CreateBatchImageItemParams{
			JobID:                job.BatchID,
			CustomID:             parsed.CustomID,
			Status:               BatchImageItemStatusFailed,
			ProviderSourceObject: batchImageOptionalStringPtr(sourceObject),
			SourceLineNumber:     &lineNo,
			ImageCount:           parsed.ImageCount,
			IndexedAt:            &now,
	REDACTED
		if parsed.Status == BatchImageParsedStatusSucceeded {
			item.Status = BatchImageItemStatusSuccess
			item.MimeType = batchImageOptionalStringPtr(parsed.MimeType)
			item.FileExtension = batchImageOptionalStringPtr(parsed.FileExtension)
			result.SuccessCount++
	REDACTED else {
			item.ErrorCode = batchImageOptionalStringPtr(parsed.ErrorCode)
			item.ErrorMessage = batchImageOptionalStringPtr(parsed.ErrorMessage)
			result.FailCount++
	REDACTED
		items = append(items, item)
		result.TotalCount++
REDACTED
	if err := scanner.Err(); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrBatchImageIndexParseFailed.WithCause(err)
	REDACTED
		return nil, err
REDACTED
	// 输出中漏掉的已提交项必须补失败记录，而不是静默消失：
	// 否则用户看不到该项，且只按成功数计费会掩盖 provider 的丢单。
	missingCount := 0
	if len(expected) > 0 {
		missingIDs := make([]string, 0)
		for customID := range expected {
			if _, ok := seen[customID]; !ok {
				missingIDs = append(missingIDs, customID)
		REDACTED
	REDACTED
		sort.Strings(missingIDs)
		for _, customID := range missingIDs {
			items = append(items, CreateBatchImageItemParams{
				JobID:                job.BatchID,
				CustomID:             customID,
				Status:               BatchImageItemStatusFailed,
				ProviderSourceObject: batchImageOptionalStringPtr(sourceObject),
				ErrorCode:            batchImageStringPtr("PROVIDER_RESULT_MISSING"),
				ErrorMessage:         batchImageStringPtr("provider output did not include a result for this item"),
				IndexedAt:            &now,
		REDACTED)
			result.FailCount++
			result.TotalCount++
	REDACTED
		missingCount = len(missingIDs)
REDACTED
	if result.TotalCount == 0 {
		return nil, ErrBatchImageIndexNoResultLines
REDACTED
	if unknownCount > 0 || missingCount > 0 {
		logger.L().Warn("batch_image.index_reconciled",
			zap.String("batch_id", job.BatchID),
			zap.Int("unknown_custom_ids", unknownCount),
			zap.Int("missing_custom_ids", missingCount),
		)
		if err := i.Repo.AppendBatchImageEvent(ctx, job.BatchID, "index_reconciled", map[string]any{
			"batch_id":           job.BatchID,
			"unknown_custom_ids": unknownCount,
			"missing_custom_ids": missingCount,
	REDACTED); err != nil {
			logger.L().Warn("batch_image.index_reconcile_event_failed",
				zap.String("batch_id", job.BatchID),
				zap.Error(err),
			)
	REDACTED
REDACTED
	if err := i.Repo.ReplaceBatchImageItemsForJob(ctx, job.BatchID, items, BatchImageCounts{
		SuccessCount: result.SuccessCount,
		FailCount:    result.FailCount,
REDACTED); err != nil {
		return nil, err
REDACTED
	return result, nil
REDACTED

// listExpectedCustomIDs 返回该 job 当前 item 表中的全部 custom_id 集合，
// 即提交时预创建（或上一轮索引重建）的完整条目清单，用于与 provider 输出对账。
func (i *BatchImageResultIndexer) listExpectedCustomIDs(ctx context.Context, batchID string) (map[string]struct{REDACTED, error) {
	const pageSize = 500
	expected := make(map[string]struct{REDACTED)
	offset := 0
	for {
		page, err := i.Repo.ListBatchImageItems(ctx, batchID, BatchImageItemFilter{Limit: pageSize, Offset: offsetREDACTED)
		if err != nil {
			return nil, err
	REDACTED
		for _, item := range page {
			if item != nil {
				expected[item.CustomID] = struct{REDACTED{REDACTED
		REDACTED
	REDACTED
		if len(page) < pageSize {
			return expected, nil
	REDACTED
		offset += len(page)
REDACTED
REDACTED

type ParsedBatchImageResult struct {
	CustomID      string
	Status        string
	MimeType      string
	FileExtension string
	ImageCount    int

	ErrorCode    string
	ErrorMessage string

	SourceLineNumber int
REDACTED

func ParseBatchImageResultLine(line []byte, lineNumber int) (*ParsedBatchImageResult, error) {
	var obj map[string]any
	if err := json.Unmarshal(line, &obj); err != nil {
		return nil, ErrBatchImageIndexParseFailed.WithCause(fmt.Errorf("line %d: %w", lineNumber, err))
REDACTED

	customID := batchImageFirstNonEmptyString(
		batchImageMapString(obj, "key"),
		batchImageMapString(obj, "custom_id"),
		batchImageMapString(obj, "customId"),
		batchImageNestedString(obj, "request", "key"),
	)
	if customID == "" {
		return nil, ErrBatchImageIndexParseFailed.WithCause(fmt.Errorf("line %d: missing custom id", lineNumber))
REDACTED

	parsed := &ParsedBatchImageResult{
		CustomID:         customID,
		SourceLineNumber: lineNumber,
REDACTED
	imageCount, mimeType := batchImageFindImageParts(obj)
	if imageCount > 0 {
		parsed.Status = BatchImageParsedStatusSucceeded
		parsed.ImageCount = imageCount
		parsed.MimeType = mimeType
		parsed.FileExtension = batchImageFileExtension(mimeType)
		return parsed, nil
REDACTED

	if code, message, ok := batchImageFailureFromProviderFields(obj); ok {
		parsed.Status = BatchImageParsedStatusFailed
		parsed.ErrorCode = code
		parsed.ErrorMessage = truncateBatchImageMessage(message, batchImageMaxErrorMessageLength)
		return parsed, nil
REDACTED

	if _, hasResponse := obj["response"]; hasResponse || batchImageHasCandidates(obj) {
		parsed.Status = BatchImageParsedStatusFailed
		parsed.ErrorCode = "EMPTY_IMAGE_OUTPUT"
		parsed.ErrorMessage = "provider response contained no image output"
		return parsed, nil
REDACTED

	parsed.Status = BatchImageParsedStatusFailed
	parsed.ErrorCode = "PROVIDER_ITEM_FAILED"
	parsed.ErrorMessage = "provider result line contained no image output"
	return parsed, nil
REDACTED

func batchImageFindImageParts(obj map[string]any) (int, string) {
	count, mimeType := batchImageFindImagePartsInCandidates(batchImageNestedAny(obj, "response", "candidates"))
	if count > 0 {
		return count, mimeType
REDACTED
	return batchImageFindImagePartsInCandidates(obj["candidates"])
REDACTED

func batchImageFindImagePartsInCandidates(raw any) (int, string) {
	candidates, ok := raw.([]any)
	if !ok {
		return 0, ""
REDACTED
	count := 0
	firstMime := ""
	for _, candidateRaw := range candidates {
		candidate, ok := candidateRaw.(map[string]any)
		if !ok {
			continue
	REDACTED
		partsRaw := batchImageNestedAny(candidate, "content", "parts")
		parts, ok := partsRaw.([]any)
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
			mime := batchImageFirstNonEmptyString(batchImageMapString(inline, "mimeType"), batchImageMapString(inline, "mime_type"))
			if data == "" || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mime)), "image/") {
				continue
		REDACTED
			count++
			if firstMime == "" {
				firstMime = strings.TrimSpace(mime)
		REDACTED
	REDACTED
REDACTED
	return count, firstMime
REDACTED

func batchImageFailureFromProviderFields(obj map[string]any) (string, string, bool) {
	if status, ok := obj["status"].(map[string]any); ok {
		message := batchImageFirstNonEmptyString(batchImageMapString(status, "message"), batchImageMapString(status, "details"))
		code := batchImageFirstNonEmptyString(batchImageMapString(status, "code"), batchImageMapString(status, "status"))
		return batchImageMapFailureCode(code, message), message, true
REDACTED
	if errObj, ok := obj["error"].(map[string]any); ok {
		message := batchImageFirstNonEmptyString(batchImageMapString(errObj, "message"), batchImageMapString(errObj, "details"))
		code := batchImageFirstNonEmptyString(batchImageMapString(errObj, "code"), batchImageMapString(errObj, "status"))
		return batchImageMapFailureCode(code, message), message, true
REDACTED
	return "", "", false
REDACTED

func batchImageMapFailureCode(code, message string) string {
	text := strings.ToLower(strings.TrimSpace(code + " " + message))
	switch {
	case strings.Contains(text, "safety"), strings.Contains(text, "policy"), strings.Contains(text, "blocked"), strings.Contains(text, "prohibited"):
		return "SAFETY_BLOCKED"
	case strings.Contains(text, "invalid_argument"), strings.Contains(text, "invalid argument"), strings.Contains(text, "bad request"):
		return "INVALID_ARGUMENT"
	case strings.Contains(text, "quota"), strings.Contains(text, "rate"), strings.Contains(text, "resource_exhausted"), strings.Contains(text, "too many requests"):
		return "PROVIDER_RATE_LIMITED"
	default:
		return "PROVIDER_ITEM_FAILED"
REDACTED
REDACTED

func batchImageFileExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/webp":
		return "webp"
	default:
		return ""
REDACTED
REDACTED

func batchImageHasCandidates(obj map[string]any) bool {
	if _, ok := obj["candidates"]; ok {
		return true
REDACTED
	_, ok := batchImageNestedAny(obj, "response", "candidates").([]any)
	return ok
REDACTED

func batchImageMapString(m map[string]any, key string) string {
	if m == nil {
		return ""
REDACTED
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatInt(int64(v), 10)
	default:
		return ""
REDACTED
REDACTED

func batchImageNestedString(m map[string]any, keys ...string) string {
	if nested, ok := batchImageNestedAny(m, keys...).(string); ok {
		return strings.TrimSpace(nested)
REDACTED
	return ""
REDACTED

func batchImageNestedAny(m map[string]any, keys ...string) any {
	var current any = m
	for _, key := range keys {
		cm, ok := current.(map[string]any)
		if !ok {
			return nil
	REDACTED
		current = cm[key]
REDACTED
	return current
REDACTED

func firstMap(values ...any) (map[string]any, bool) {
	for _, value := range values {
		if m, ok := value.(map[string]any); ok {
			return m, true
	REDACTED
REDACTED
	return nil, false
REDACTED

func batchImageFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
	REDACTED
REDACTED
	return ""
REDACTED

func batchImageDerefString(v *string) string {
	if v == nil {
		return ""
REDACTED
	return strings.TrimSpace(*v)
REDACTED

func batchImageStringPtr(v string) *string {
	return &v
REDACTED

func batchImageOptionalStringPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
REDACTED
	return &v
REDACTED

func truncateBatchImageMessage(message string, limit int) string {
	message = strings.TrimSpace(message)
	if limit <= 0 || len(message) <= limit {
		return message
REDACTED
	return message[:limit]
REDACTED
