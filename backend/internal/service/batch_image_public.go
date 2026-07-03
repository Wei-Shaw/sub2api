package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	defaultBatchImageMaxItems       = 500
	defaultBatchImageMaxPromptChars = 8000
	defaultBatchImageResponseMime   = "image/png"
	defaultBatchImageImageSize      = "1K"
	maxBatchImagePublicErrorChars   = 500
)

type BatchImageAccountSelectionRepository interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
	ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error)
REDACTED

type BatchImageSubmitRequest struct {
	Model            string                 `json:"model"`
	Provider         string                 `json:"provider"`
	Items            []BatchImageSubmitItem `json:"items"`
	ResponseMimeType string                 `json:"response_mime_type"`
	AspectRatio      string                 `json:"aspect_ratio"`
	ImageSize        string                 `json:"image_size"`
	Metadata         map[string]string      `json:"metadata"`
REDACTED

type BatchImageSubmitItem struct {
	CustomID string `json:"custom_id"`
	Prompt   string `json:"prompt"`
REDACTED

type BatchImageOwner struct {
	UserID   int64
	APIKeyID int64
	GroupID  *int64
REDACTED

type BatchImagePublicService struct {
	Repo             BatchImageRepository
	AccountRepo      BatchImageAccountSelectionRepository
	Queue            BatchImageQueue
	ProviderRegistry *BatchImageProviderRegistry
	Pricing          BatchImagePricingResolver
	Config           *config.Config
REDACTED

type BatchImagePublicBatch struct {
	ID              string   `json:"id"`
	Object          string   `json:"object"`
	Status          string   `json:"status"`
	Model           string   `json:"model"`
	Provider        string   `json:"provider"`
	ItemCount       int      `json:"item_count"`
	SuccessCount    int      `json:"success_count"`
	FailCount       int      `json:"fail_count"`
	EstimatedCost   float64  `json:"estimated_cost"`
	ActualCost      *float64 `json:"actual_cost"`
	CreatedAt       int64    `json:"created_at"`
	SubmittedAt     *int64   `json:"submitted_at"`
	SettledAt       *int64   `json:"settled_at"`
	OutputDeletedAt *int64   `json:"output_deleted_at,omitempty"`
REDACTED

type BatchImagePublicItem struct {
	CustomID      string                 `json:"custom_id"`
	Status        string                 `json:"status"`
	MimeType      *string                `json:"mime_type"`
	FileExtension *string                `json:"file_extension"`
	ImageCount    int                    `json:"image_count"`
	Error         *BatchImagePublicError `json:"error"`
REDACTED

type BatchImagePublicError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
REDACTED

type BatchImagePublicItemsResponse struct {
	Object  string                 `json:"object"`
	Data    []BatchImagePublicItem `json:"data"`
	HasMore bool                   `json:"has_more"`
REDACTED

type BatchImageItemsQuery struct {
	Status string
	Limit  int
	Cursor string
REDACTED

func NewBatchImagePublicService(repo BatchImageRepository, accountRepo AccountRepository, queue BatchImageQueue, pricing *BatchImageModelPricingResolver, cfg *config.Config) *BatchImagePublicService {
	return &BatchImagePublicService{
		Repo:             repo,
		AccountRepo:      accountRepo,
		Queue:            queue,
		ProviderRegistry: NewDefaultBatchImageProviderRegistry(),
		Pricing:          pricing,
		Config:           cfg,
REDACTED
REDACTED

func (s *BatchImagePublicService) Submit(ctx context.Context, owner BatchImageOwner, req BatchImageSubmitRequest, idempotencyKey string) (*BatchImagePublicBatch, error) {
	if !s.enabled() {
		return nil, ErrBatchImageDisabled
REDACTED
	normalized, err := s.validateSubmitRequest(req)
	if err != nil {
		return nil, err
REDACTED
	requestHash := HashBatchImageSubmitRequest(normalized)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey != "" {
		existing, err := s.Repo.GetBatchImageJobByIdempotencyKey(ctx, owner.UserID, owner.APIKeyID, idempotencyKey)
		if err == nil {
			if batchImageDerefString(existing.RequestHash) != requestHash {
				return nil, ErrBatchImageIdempotencyConflict
		REDACTED
			if existing.Status == BatchImageJobStatusSubmitted && s.Queue != nil {
				if enqueueErr := s.Queue.Enqueue(ctx, existing.BatchID); enqueueErr != nil && !errors.Is(enqueueErr, ErrBatchImageAlreadyQueued) {
					_ = s.Repo.RecordBatchImageJobSubmitFailure(ctx, existing.BatchID, "QUEUE_FAILED", sanitizeBatchImagePublicMessage(enqueueErr.Error()), false)
					return nil, ErrBatchImageQueueFailed
			REDACTED
		REDACTED
			return BatchImageJobToPublic(existing), nil
	REDACTED
		if !errors.Is(err, ErrBatchImageJobNotFound) {
			return nil, err
	REDACTED
REDACTED

	provider, account, err := s.selectProviderAndAccount(ctx, owner, normalized.Provider, normalized.Model)
	if err != nil {
		return nil, err
REDACTED
	estimatedCost := s.estimateCost(ctx, normalized, provider.Name())
	batchID, err := NewBatchImageID()
	if err != nil {
		return nil, err
REDACTED
	apiKeyID := owner.APIKeyID
	accountID := account.ID
	job, err := s.Repo.CreateBatchImageJob(ctx, CreateBatchImageJobParams{
		BatchID:        batchID,
		UserID:         owner.UserID,
		APIKeyID:       &apiKeyID,
		AccountID:      &accountID,
		Provider:       provider.Name(),
		Model:          normalized.Model,
		Status:         BatchImageJobStatusCreated,
		ItemCount:      len(normalized.Items),
		EstimatedCost:  estimatedCost,
		Currency:       "USD",
		IdempotencyKey: batchImageOptionalStringPtr(idempotencyKey),
		RequestHash:    batchImageStringPtr(requestHash),
REDACTED)
	if err != nil {
		return nil, err
REDACTED

	input := BatchImageInput{
		BatchID:          job.BatchID,
		Model:            normalized.Model,
		DisplayName:      job.BatchID,
		ResponseMimeType: normalized.ResponseMimeType,
		AspectRatio:      normalized.AspectRatio,
		ImageSize:        normalized.ImageSize,
		Metadata:         normalized.Metadata,
		Items:            make([]BatchImageInputItem, 0, len(normalized.Items)),
REDACTED
	for _, item := range normalized.Items {
		input.Items = append(input.Items, BatchImageInputItem{CustomID: item.CustomID, Prompt: item.PromptREDACTED)
REDACTED

	providerJob, err := provider.Submit(ctx, job, account, input)
	if err != nil {
		_ = s.Repo.RecordBatchImageJobSubmitFailure(ctx, job.BatchID, "PROVIDER_SUBMIT_FAILED", sanitizeBatchImagePublicMessage(err.Error()), true)
		return nil, ErrBatchImageProviderSubmitFailed
REDACTED
	if providerJob == nil || strings.TrimSpace(providerJob.ProviderJobName) == "" {
		_ = s.Repo.RecordBatchImageJobSubmitFailure(ctx, job.BatchID, "PROVIDER_SUBMIT_FAILED", "provider job name missing", true)
		return nil, ErrBatchImageProviderSubmitFailed
REDACTED

	if err := s.Repo.UpdateBatchImageJobProviderSubmit(ctx, UpdateBatchImageJobProviderSubmitParams{
		BatchID:           job.BatchID,
		ProviderJobName:   providerJob.ProviderJobName,
		ProviderInputRef:  providerJob.ProviderInputRef,
		ProviderOutputRef: providerJob.ProviderOutputRef,
		GCSInputURI:       batchImageGCSRef(provider.Name(), providerJob.ProviderInputRef),
		GCSOutputURI:      batchImageGCSRef(provider.Name(), providerJob.ProviderOutputRef),
		EventPayload:      map[string]any{"provider": provider.Name()REDACTED,
REDACTED); err != nil {
		return nil, err
REDACTED

	if s.Queue != nil {
		if err := s.Queue.Enqueue(ctx, job.BatchID); err != nil && !errors.Is(err, ErrBatchImageAlreadyQueued) {
			_ = s.Repo.RecordBatchImageJobSubmitFailure(ctx, job.BatchID, "QUEUE_FAILED", sanitizeBatchImagePublicMessage(err.Error()), false)
			return nil, ErrBatchImageQueueFailed
	REDACTED
REDACTED

	created, err := s.Repo.GetBatchImageJobByBatchID(ctx, job.BatchID)
	if err != nil {
		return nil, err
REDACTED
	return BatchImageJobToPublic(created), nil
REDACTED

func (s *BatchImagePublicService) Get(ctx context.Context, owner BatchImageOwner, batchID string) (*BatchImagePublicBatch, error) {
	job, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return nil, err
REDACTED
	return BatchImageJobToPublic(job), nil
REDACTED

func (s *BatchImagePublicService) ListItems(ctx context.Context, owner BatchImageOwner, batchID string, query BatchImageItemsQuery) (*BatchImagePublicItemsResponse, error) {
	filter := BatchImageItemFilter{Limit: query.Limit, Offset: parseBatchImageCursor(query.Cursor)REDACTED
	switch strings.TrimSpace(query.Status) {
	case "", "all":
	case "succeeded", "success":
		filter.Status = BatchImageItemStatusSuccess
	case "failed":
		filter.Status = BatchImageItemStatusFailed
	default:
		return nil, ErrBatchImageInvalidItems
REDACTED
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
REDACTED
	items, err := s.Repo.ListBatchImageItemsForOwner(ctx, owner.UserID, owner.APIKeyID, batchID, filter)
	if err != nil {
		return nil, err
REDACTED
	data := make([]BatchImagePublicItem, 0, len(items))
	for _, item := range items {
		data = append(data, BatchImageItemToPublic(item))
REDACTED
	return &BatchImagePublicItemsResponse{
		Object:  "list",
		Data:    data,
		HasMore: len(data) == filter.Limit,
REDACTED, nil
REDACTED

func (s *BatchImagePublicService) Cancel(ctx context.Context, owner BatchImageOwner, batchID string) (*BatchImagePublicBatch, error) {
	job, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return nil, err
REDACTED
	if isBatchImageProcessorDoneStatus(job.Status) {
		return BatchImageJobToPublic(job), nil
REDACTED
	if job.ProviderJobName != nil && strings.TrimSpace(*job.ProviderJobName) != "" {
		provider, ok := s.ProviderRegistry.Get(job.Provider)
		if !ok || provider == nil {
			return nil, ErrBatchImageUnsupportedProvider
	REDACTED
		if job.AccountID == nil {
			return nil, ErrBatchImageCancelFailed
	REDACTED
		account, err := s.AccountRepo.GetByID(ctx, *job.AccountID)
		if err != nil {
			return nil, ErrBatchImageCancelFailed
	REDACTED
		if err := provider.Cancel(ctx, job, account); err != nil {
			return nil, ErrBatchImageCancelFailed
	REDACTED
REDACTED
	if err := s.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusCancelled, BatchImageTransitionOptions{
		EventType:    "job_cancelled",
		EventPayload: map[string]any{"batch_id": job.BatchIDREDACTED,
REDACTED); err != nil {
		return nil, err
REDACTED
	updated, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return nil, err
REDACTED
	return BatchImageJobToPublic(updated), nil
REDACTED

func (s *BatchImagePublicService) validateSubmitRequest(req BatchImageSubmitRequest) (BatchImageSubmitRequest, error) {
	req.Model = strings.TrimSpace(req.Model)
	req.Provider = strings.TrimSpace(req.Provider)
	req.ResponseMimeType = strings.TrimSpace(req.ResponseMimeType)
	req.AspectRatio = strings.TrimSpace(req.AspectRatio)
	req.ImageSize = strings.TrimSpace(req.ImageSize)
	if req.Model == "" {
		return req, ErrBatchImageInvalidModel
REDACTED
	if req.Provider != "" && !IsSupportedBatchImageProvider(req.Provider) {
		return req, ErrBatchImageUnsupportedProvider
REDACTED
	if len(req.Items) == 0 {
		return req, ErrBatchImageInvalidItems
REDACTED
	maxItems := s.maxItems()
	if len(req.Items) > maxItems {
		return req, ErrBatchImageInvalidItems
REDACTED
	if req.ResponseMimeType == "" {
		req.ResponseMimeType = s.defaultResponseMimeType()
REDACTED
	if req.ImageSize == "" {
		req.ImageSize = s.defaultImageSize()
REDACTED
	if req.Provider == BatchImageProviderVertex && (strings.EqualFold(req.ImageSize, "2K") || strings.EqualFold(req.ImageSize, "4K")) {
		return req, ErrBatchImageInvalidItems
REDACTED
	req.Metadata = sanitizeBatchImageMetadata(req.Metadata)

	seen := make(map[string]struct{REDACTED, len(req.Items))
	for i := range req.Items {
		req.Items[i].CustomID = strings.TrimSpace(req.Items[i].CustomID)
		if req.Items[i].CustomID == "" {
			req.Items[i].CustomID = fmt.Sprintf("item_%06d", i+1)
	REDACTED
		req.Items[i].Prompt = strings.TrimSpace(req.Items[i].Prompt)
		if req.Items[i].Prompt == "" {
			return req, ErrBatchImageInvalidItems
	REDACTED
		if len(req.Items[i].Prompt) > s.maxPromptChars() {
			return req, ErrBatchImagePromptTooLong
	REDACTED
		if _, ok := seen[req.Items[i].CustomID]; ok {
			return req, ErrBatchImageDuplicateCustomIDInRequest
	REDACTED
		seen[req.Items[i].CustomID] = struct{REDACTED{REDACTED
REDACTED
	return req, nil
REDACTED

func (s *BatchImagePublicService) selectProviderAndAccount(ctx context.Context, owner BatchImageOwner, requestedProvider, model string) (BatchImageProvider, *Account, error) {
	providers := []string{requestedProviderREDACTED
	if strings.TrimSpace(requestedProvider) == "" {
		providers = []string{BatchImageProviderGeminiAPI, BatchImageProviderVertexREDACTED
REDACTED
	for _, providerName := range providers {
		provider, ok := s.ProviderRegistry.Get(providerName)
		if !ok || provider == nil {
			continue
	REDACTED
		accounts, err := s.listCandidateAccounts(ctx, owner.GroupID, batchImageProviderPlatform(providerName))
		if err != nil {
			return nil, nil, err
	REDACTED
		sort.SliceStable(accounts, func(i, j int) bool {
			if accounts[i].Priority != accounts[j].Priority {
				return accounts[i].Priority > accounts[j].Priority
		REDACTED
			return accounts[i].ID < accounts[j].ID
	REDACTED)
		for i := range accounts {
			account := accounts[i]
			if !account.IsSchedulable() || !account.IsModelSupported(model) {
				continue
		REDACTED
			if provider.SupportsAccount(&account) {
				return provider, &account, nil
		REDACTED
	REDACTED
REDACTED
	if requestedProvider != "" {
		return nil, nil, ErrBatchImageNoAccountAvailable
REDACTED
	return nil, nil, ErrBatchImageNoAccountAvailable
REDACTED

func (s *BatchImagePublicService) listCandidateAccounts(ctx context.Context, groupID *int64, platform string) ([]Account, error) {
	if s.AccountRepo == nil {
		return nil, ErrBatchImageNoAccountAvailable
REDACTED
	if groupID != nil && *groupID > 0 {
		return s.AccountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, platform)
REDACTED
	return s.AccountRepo.ListSchedulableByPlatform(ctx, platform)
REDACTED

func (s *BatchImagePublicService) estimateCost(ctx context.Context, req BatchImageSubmitRequest, provider string) float64 {
	if s.Pricing == nil {
		return 0
REDACTED
	unit, err := s.Pricing.BatchImageUnitPrice(ctx, &BatchImageJob{Provider: provider, Model: req.ModelREDACTED)
	if err != nil || unit < 0 {
		return 0
REDACTED
	return unit * float64(len(req.Items))
REDACTED

func (s *BatchImagePublicService) enabled() bool {
	return s != nil && s.Repo != nil && s.AccountRepo != nil && s.Config != nil && s.Config.BatchImage.Enabled
REDACTED

func (s *BatchImagePublicService) maxItems() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxItemsPerJobDefault > 0 {
		return s.Config.BatchImage.MaxItemsPerJobDefault
REDACTED
	return defaultBatchImageMaxItems
REDACTED

func (s *BatchImagePublicService) maxPromptChars() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxPromptCharsPerItem > 0 {
		return s.Config.BatchImage.MaxPromptCharsPerItem
REDACTED
	return defaultBatchImageMaxPromptChars
REDACTED

func (s *BatchImagePublicService) defaultResponseMimeType() string {
	if s != nil && s.Config != nil && strings.TrimSpace(s.Config.BatchImage.DefaultResponseMimeType) != "" {
		return strings.TrimSpace(s.Config.BatchImage.DefaultResponseMimeType)
REDACTED
	return defaultBatchImageResponseMime
REDACTED

func (s *BatchImagePublicService) defaultImageSize() string {
	if s != nil && s.Config != nil && strings.TrimSpace(s.Config.BatchImage.DefaultImageSize) != "" {
		return strings.TrimSpace(s.Config.BatchImage.DefaultImageSize)
REDACTED
	return defaultBatchImageImageSize
REDACTED

func BatchImageJobToPublic(job *BatchImageJob) *BatchImagePublicBatch {
	if job == nil {
		return nil
REDACTED
	return &BatchImagePublicBatch{
		ID:              job.BatchID,
		Object:          "image.batch",
		Status:          PublicBatchImageStatus(job.Status),
		Model:           job.Model,
		Provider:        job.Provider,
		ItemCount:       job.ItemCount,
		SuccessCount:    job.SuccessCount,
		FailCount:       job.FailCount,
		EstimatedCost:   job.EstimatedCost,
		ActualCost:      job.ActualCost,
		CreatedAt:       job.CreatedAt.Unix(),
		SubmittedAt:     batchImageUnixPtr(job.SubmittedAt),
		SettledAt:       batchImageUnixPtr(job.SettledAt),
		OutputDeletedAt: batchImageUnixPtr(job.OutputDeletedAt),
REDACTED
REDACTED

func BatchImageItemToPublic(item *BatchImageItem) BatchImagePublicItem {
	out := BatchImagePublicItem{
		CustomID:      item.CustomID,
		Status:        "failed",
		MimeType:      item.MimeType,
		FileExtension: item.FileExtension,
		ImageCount:    item.ImageCount,
REDACTED
	if item.Status == BatchImageItemStatusSuccess {
		out.Status = "succeeded"
		return out
REDACTED
	out.Error = &BatchImagePublicError{
		Code:    batchImageDerefString(item.ErrorCode),
		Message: sanitizeBatchImagePublicMessage(batchImageDerefString(item.ErrorMessage)),
REDACTED
	return out
REDACTED

func PublicBatchImageStatus(status string) string {
	switch status {
	case BatchImageJobStatusCreated, BatchImageJobStatusUploading, BatchImageJobStatusSubmitted:
		return "queued"
	case BatchImageJobStatusRunning:
		return "running"
	case BatchImageJobStatusIndexing:
		return "processing_results"
	case BatchImageJobStatusSettling:
		return "settling"
	case BatchImageJobStatusCompleted:
		return "completed"
	case BatchImageJobStatusFailed:
		return "failed"
	case BatchImageJobStatusCancelled:
		return "cancelled"
	case BatchImageJobStatusOutputDeleted:
		return "output_deleted"
	default:
		return status
REDACTED
REDACTED

func HashBatchImageSubmitRequest(req BatchImageSubmitRequest) string {
	req.Metadata = sanitizeBatchImageMetadata(req.Metadata)
	b, _ := json.Marshal(req)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
REDACTED

func batchImageProviderPlatform(provider string) string {
	switch provider {
	case BatchImageProviderGeminiAPI, BatchImageProviderVertex:
		return PlatformGemini
	default:
		return PlatformGemini
REDACTED
REDACTED

func batchImageGCSRef(provider, ref string) string {
	if provider == BatchImageProviderVertex && strings.HasPrefix(strings.TrimSpace(ref), "gs://") {
		return strings.TrimSpace(ref)
REDACTED
	return ""
REDACTED

func sanitizeBatchImageMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
REDACTED
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
REDACTED
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		key := strings.TrimSpace(k)
		if key == "" || len(key) > 64 {
			continue
	REDACTED
		value := strings.TrimSpace(in[k])
		if len(value) > 256 {
			value = value[:256]
	REDACTED
		out[key] = value
		if len(out) >= 20 {
			break
	REDACTED
REDACTED
	return out
REDACTED

func sanitizeBatchImagePublicMessage(message string) string {
	message = strings.TrimSpace(message)
	for _, marker := range []string{"gs://", "files/", "projects/"REDACTED {
		if strings.Contains(message, marker) {
			message = "upstream provider operation failed"
			break
	REDACTED
REDACTED
	if len(message) > maxBatchImagePublicErrorChars {
		message = message[:maxBatchImagePublicErrorChars]
REDACTED
	return message
REDACTED

func batchImageUnixPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
REDACTED
	v := t.Unix()
	return &v
REDACTED

func parseBatchImageCursor(cursor string) int {
	offset, err := strconv.Atoi(strings.TrimSpace(cursor))
	if err != nil || offset < 0 {
		return 0
REDACTED
	return offset
REDACTED
