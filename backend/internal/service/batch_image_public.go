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
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	defaultBatchImageMaxItems           = 200
	defaultBatchImageMaxOutputImages    = 200
	defaultBatchImageMaxOutputCount     = 4
	defaultBatchImageMaxPromptChars     = 8000
	defaultBatchImageResponseMime       = "image/png"
	defaultBatchImageImageSize          = "1K"
	defaultBatchImageDiscountMultiplier = 0.5
	defaultBatchImageHoldMultiplier     = 0.6
	maxBatchImagePublicErrorChars       = 500
	maxBatchImageReferenceImageBytes    = 10 * 1024 * 1024
	defaultBatchImageMaxReferenceImages = 1000
	defaultBatchImageMaxReferenceBytes  = 128 * 1024 * 1024
)

type BatchImageAccountSelectionRepository interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
	ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error)
REDACTED

type BatchImageGroupPricingRepository interface {
	GetByIDLite(ctx context.Context, id int64) (*Group, error)
REDACTED

type BatchImageUserGroupRateRepository interface {
	GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error)
REDACTED

type BatchImageSubmitRequest struct {
	Model            string                 `json:"model"`
	TaskName         string                 `json:"task_name"`
	ParentBatchID    string                 `json:"parent_batch_id"`
	Provider         string                 `json:"provider"`
	Items            []BatchImageSubmitItem `json:"items"`
	ResponseMimeType string                 `json:"response_mime_type"`
	AspectRatio      string                 `json:"aspect_ratio"`
	ImageSize        string                 `json:"image_size"`
	Metadata         map[string]string      `json:"metadata"`
REDACTED

type BatchImageSubmitItem struct {
	CustomID        string                     `json:"custom_id"`
	Prompt          string                     `json:"prompt"`
	OutputCount     int                        `json:"output_count,omitempty"`
	ReferenceImages []BatchImageReferenceInput `json:"reference_images,omitempty"`
REDACTED

type BatchImageReferenceInput struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	MimeType string `json:"mime_type"`
	Data     []byte `json:"data,omitempty"`
	FileURI  string `json:"file_uri,omitempty"`
REDACTED

type BatchImageOwner struct {
	UserID   int64
	APIKeyID int64
	GroupID  *int64
REDACTED

type BatchImagePublicService struct {
	Repo              BatchImageRepository
	AccountRepo       BatchImageAccountSelectionRepository
	GroupRepo         BatchImageGroupPricingRepository
	UserGroupRateRepo BatchImageUserGroupRateRepository
	Queue             BatchImageQueue
	ProviderRegistry  *BatchImageProviderRegistry
	Pricing           BatchImagePricingResolver
	BillingRepo       UsageBillingRepository
	AuthCache         APIKeyAuthCacheInvalidator
	Config            *config.Config
REDACTED

type BatchImagePricingSnapshot struct {
	BaseUnitPrice           float64
	GroupRateMultiplier     float64
	AccountRateMultiplier   float64
	BatchDiscountMultiplier float64
	HoldMultiplier          float64
	BillableUnitPrice       float64
	HoldUnitPrice           float64
	EstimatedCost           float64
	HoldAmount              float64
REDACTED

type BatchImagePublicBatch struct {
	ID              string   `json:"id"`
	Object          string   `json:"object"`
	TaskName        string   `json:"task_name"`
	ParentBatchID   *string  `json:"parent_batch_id,omitempty"`
	Status          string   `json:"status"`
	Model           string   `json:"model"`
	Provider        string   `json:"provider"`
	ItemCount       int      `json:"item_count"`
	SuccessCount    int      `json:"success_count"`
	FailCount       int      `json:"fail_count"`
	EstimatedCost   float64  `json:"estimated_cost"`
	HoldAmount      float64  `json:"hold_amount"`
	ActualCost      *float64 `json:"actual_cost"`
	CreatedAt       int64    `json:"created_at"`
	SubmittedAt     *int64   `json:"submitted_at"`
	SettledAt       *int64   `json:"settled_at"`
	DownloadedAt    *int64   `json:"downloaded_at,omitempty"`
	OutputDeletedAt *int64   `json:"output_deleted_at,omitempty"`
REDACTED

type BatchImagePublicItem struct {
	CustomID      string                 `json:"custom_id"`
	Status        string                 `json:"status"`
	PromptPreview *string                `json:"prompt_preview,omitempty"`
	MimeType      *string                `json:"mime_type"`
	FileExtension *string                `json:"file_extension"`
	ImageCount    int                    `json:"image_count"`
	Error         *BatchImagePublicError `json:"error"`
REDACTED

type BatchImagePublicError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Source  string `json:"source,omitempty"`
REDACTED

type BatchImagePublicItemsResponse struct {
	Object  string                 `json:"object"`
	Data    []BatchImagePublicItem `json:"data"`
	HasMore bool                   `json:"has_more"`
REDACTED

type BatchImagePublicListResponse struct {
	Object  string                   `json:"object"`
	Data    []*BatchImagePublicBatch `json:"data"`
	HasMore bool                     `json:"has_more"`
REDACTED

type BatchImagePublicModel struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Provider string `json:"provider"`
REDACTED

type BatchImagePublicModelsResponse struct {
	Object string                  `json:"object"`
	Data   []BatchImagePublicModel `json:"data"`
REDACTED

type BatchImageJobsQuery struct {
	Status     string
	TaskName   string
	Downloaded string
	From       string
	To         string
	Limit      int
	Cursor     string
REDACTED

type BatchImageItemsQuery struct {
	Status string
	Limit  int
	Cursor string
REDACTED

func NewBatchImagePublicService(repo BatchImageRepository, accountRepo AccountRepository, groupRepo GroupRepository, userGroupRateRepo UserGroupRateRepository, queue BatchImageQueue, pricing *BatchImageModelPricingResolver, billingRepo UsageBillingRepository, authCache APIKeyAuthCacheInvalidator, cfg *config.Config) *BatchImagePublicService {
	return &BatchImagePublicService{
		Repo:              repo,
		AccountRepo:       accountRepo,
		GroupRepo:         groupRepo,
		UserGroupRateRepo: userGroupRateRepo,
		Queue:             queue,
		ProviderRegistry:  NewBatchImageProviderRegistryFromConfig(cfg),
		Pricing:           pricing,
		BillingRepo:       billingRepo,
		AuthCache:         authCache,
		Config:            cfg,
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
	pricingSnapshot, err := s.resolvePricingSnapshot(ctx, owner, normalized, provider.Name(), account)
	if err != nil {
		return nil, err
REDACTED
	parentBatchID := batchImageOptionalStringPtr(normalized.ParentBatchID)
	if parentBatchID != nil {
		parent, parentErr := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, *parentBatchID)
		if parentErr != nil {
			return nil, parentErr
	REDACTED
		if parent.ParentBatchID != nil && strings.TrimSpace(*parent.ParentBatchID) != "" {
			parentBatchID = batchImageOptionalStringPtr(*parent.ParentBatchID)
	REDACTED
REDACTED
	batchID, err := NewBatchImageID()
	if err != nil {
		return nil, err
REDACTED
	apiKeyID := owner.APIKeyID
	accountID := account.ID
	holdID := BatchImageHoldRequestID(batchID)
	holdAmount := pricingSnapshot.HoldAmount
	job, err := s.Repo.CreateBatchImageJob(ctx, CreateBatchImageJobParams{
		BatchID:                 batchID,
		UserID:                  owner.UserID,
		APIKeyID:                &apiKeyID,
		AccountID:               &accountID,
		Provider:                provider.Name(),
		Model:                   normalized.Model,
		TaskName:                normalized.TaskName,
		ParentBatchID:           parentBatchID,
		Status:                  BatchImageJobStatusCreated,
		ItemCount:               len(normalized.Items),
		EstimatedCost:           pricingSnapshot.EstimatedCost,
		HoldAmount:              &holdAmount,
		BaseUnitPrice:           pricingSnapshot.BaseUnitPrice,
		GroupRateMultiplier:     pricingSnapshot.GroupRateMultiplier,
		AccountRateMultiplier:   pricingSnapshot.AccountRateMultiplier,
		BatchDiscountMultiplier: pricingSnapshot.BatchDiscountMultiplier,
		HoldMultiplier:          pricingSnapshot.HoldMultiplier,
		BillableUnitPrice:       pricingSnapshot.BillableUnitPrice,
		HoldUnitPrice:           pricingSnapshot.HoldUnitPrice,
		PricingSnapshotVersion:  1,
		Currency:                "USD",
		HoldID:                  &holdID,
		IdempotencyKey:          batchImageOptionalStringPtr(idempotencyKey),
		RequestHash:             batchImageStringPtr(requestHash),
REDACTED)
	if err != nil {
		return nil, err
REDACTED
	if err := reserveBatchImageBalanceHold(ctx, s.BillingRepo, job, requestHash); err != nil {
		code := "BILLING_HOLD_FAILED"
		if errors.Is(err, ErrBatchImageInsufficientBalance) {
			code = "INSUFFICIENT_BALANCE"
	REDACTED
		_ = s.Repo.RecordBatchImageJobSubmitFailure(ctx, job.BatchID, code, sanitizeBatchImagePublicMessage(err.Error()), true)
		s.hidePreUpstreamSubmitFailure(ctx, owner, job)
		return nil, err
REDACTED
	s.invalidateAuthCache(ctx, owner.UserID)
	if err := s.createPendingItems(ctx, job.BatchID, requestHash, normalized.Items); err != nil {
		if releaseErr := s.releaseFailedSubmitHold(ctx, job, requestHash); releaseErr != nil {
			return nil, releaseErr
	REDACTED
		_ = s.Repo.RecordBatchImageJobSubmitFailure(ctx, job.BatchID, "ITEM_CREATE_FAILED", sanitizeBatchImagePublicMessage(err.Error()), true)
		s.hidePreUpstreamSubmitFailure(ctx, owner, job)
		return nil, ErrBatchImageQueueFailed
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
		refs := make([]BatchImageReference, 0, len(item.ReferenceImages))
		for _, ref := range item.ReferenceImages {
			refs = append(refs, BatchImageReference{
				ID:       ref.ID,
				Type:     ref.Type,
				MimeType: ref.MimeType,
				Data:     ref.Data,
				FileURI:  ref.FileURI,
		REDACTED)
	REDACTED
		input.Items = append(input.Items, BatchImageInputItem{
			CustomID:        item.CustomID,
			Prompt:          item.Prompt,
			ReferenceImages: refs,
	REDACTED)
REDACTED

	providerJob, err := provider.Submit(ctx, job, account, input)
	if err != nil {
		if releaseErr := s.releaseFailedSubmitHold(ctx, job, requestHash); releaseErr != nil {
			return nil, releaseErr
	REDACTED
		publicErr := batchImageProviderSubmitPublicError(err)
		reason := batchImageProviderSubmitRecordCode(publicErr)
		_ = s.Repo.RecordBatchImageJobSubmitFailure(ctx, job.BatchID, reason, sanitizeBatchImagePublicMessage(err.Error()), true)
		s.hidePreUpstreamSubmitFailure(ctx, owner, job)
		return nil, publicErr
REDACTED
	if providerJob == nil || strings.TrimSpace(providerJob.ProviderJobName) == "" {
		if releaseErr := s.releaseFailedSubmitHold(ctx, job, requestHash); releaseErr != nil {
			return nil, releaseErr
	REDACTED
		_ = s.Repo.RecordBatchImageJobSubmitFailure(ctx, job.BatchID, "PROVIDER_SUBMIT_FAILED", "provider job name missing", true)
		s.hidePreUpstreamSubmitFailure(ctx, owner, job)
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

func (s *BatchImagePublicService) releaseFailedSubmitHold(ctx context.Context, job *BatchImageJob, requestHash string) error {
	if err := releaseBatchImageBalanceHold(ctx, s.BillingRepo, job, requestHash); err != nil {
		_ = s.Repo.RecordBatchImageJobSubmitFailure(ctx, job.BatchID, "BILLING_RELEASE_FAILED", sanitizeBatchImagePublicMessage(err.Error()), true)
		s.enqueueBillingRetry(ctx, job.BatchID)
		return ErrBatchImageBillingHoldFailed
REDACTED
	s.invalidateAuthCache(ctx, job.UserID)
	return nil
REDACTED

func (s *BatchImagePublicService) createPendingItems(ctx context.Context, batchID, requestHash string, items []BatchImageSubmitItem) error {
	if s == nil || s.Repo == nil || len(items) == 0 {
		return nil
REDACTED
	params := make([]CreateBatchImageItemParams, 0, len(items))
	for _, item := range items {
		preview := truncateBatchImageMessage(item.Prompt, s.maxPromptChars())
		params = append(params, CreateBatchImageItemParams{
			JobID:         batchID,
			CustomID:      item.CustomID,
			Status:        BatchImageItemStatusPending,
			RequestHash:   batchImageStringPtr(requestHash),
			PromptPreview: batchImageStringPtr(preview),
			ImageCount:    0,
	REDACTED)
REDACTED
	return s.Repo.BulkCreateBatchImageItems(ctx, params)
REDACTED

func (s *BatchImagePublicService) enqueueBillingRetry(ctx context.Context, batchID string) {
	if s == nil || s.Queue == nil {
		return
REDACTED
	if err := s.Queue.Enqueue(ctx, batchID); err != nil && !errors.Is(err, ErrBatchImageAlreadyQueued) {
		_ = s.Repo.AppendBatchImageEvent(ctx, batchID, "billing_retry_enqueue_failed", map[string]any{
			"batch_id": batchID,
			"error":    sanitizeBatchImagePublicMessage(err.Error()),
	REDACTED)
REDACTED
REDACTED

func (s *BatchImagePublicService) hidePreUpstreamSubmitFailure(ctx context.Context, owner BatchImageOwner, job *BatchImageJob) {
	if s == nil || s.Repo == nil || job == nil || job.ProviderJobName != nil {
		return
REDACTED
	_ = s.Repo.MarkBatchImageJobUserDeleted(ctx, owner.UserID, owner.APIKeyID, job.BatchID, time.Now())
REDACTED

func (s *BatchImagePublicService) Get(ctx context.Context, owner BatchImageOwner, batchID string) (*BatchImagePublicBatch, error) {
	job, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return nil, err
REDACTED
	return BatchImageJobToPublic(job), nil
REDACTED

func (s *BatchImagePublicService) List(ctx context.Context, owner BatchImageOwner, query BatchImageJobsQuery) (*BatchImagePublicListResponse, error) {
	filter := BatchImageJobFilter{Limit: query.Limit, Offset: parseBatchImageCursor(query.Cursor), ExcludeDeleted: trueREDACTED
	filter.TaskNameLike = strings.TrimSpace(query.TaskName)
	switch strings.TrimSpace(query.Status) {
	case "", "all":
	case "queued":
		filter.Status = BatchImageJobStatusSubmitted
	case "processing_results":
		filter.Status = BatchImageJobStatusIndexing
	case "completed":
		filter.Status = BatchImageJobStatusCompleted
	case "failed":
		filter.Status = BatchImageJobStatusFailed
	case "cancelled":
		filter.Status = BatchImageJobStatusCancelled
	case "output_deleted":
		filter.Status = BatchImageJobStatusOutputDeleted
	default:
		filter.Status = strings.TrimSpace(query.Status)
REDACTED
	switch strings.TrimSpace(strings.ToLower(query.Downloaded)) {
	case "", "all":
	case "true", "1", "yes", "downloaded":
		downloaded := true
		filter.Downloaded = &downloaded
	case "false", "0", "no", "not_downloaded":
		downloaded := false
		filter.Downloaded = &downloaded
	default:
		return nil, ErrBatchImageInvalidItems
REDACTED
	if from := parseBatchImageListTime(query.From); from != nil {
		filter.CreatedAfter = from
REDACTED
	if to := parseBatchImageListTime(query.To); to != nil {
		filter.CreatedBefore = to
REDACTED
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
REDACTED
	jobs, err := s.Repo.ListBatchImageJobsForOwner(ctx, owner.UserID, owner.APIKeyID, filter)
	if err != nil {
		return nil, err
REDACTED
	data := make([]*BatchImagePublicBatch, 0, len(jobs))
	for _, job := range jobs {
		data = append(data, BatchImageJobToPublic(job))
REDACTED
	return &BatchImagePublicListResponse{
		Object:  "list",
		Data:    data,
		HasMore: len(data) == filter.Limit,
REDACTED, nil
REDACTED

func (s *BatchImagePublicService) MarkDownloaded(ctx context.Context, owner BatchImageOwner, batchID string) error {
	job, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return err
REDACTED
	return s.Repo.MarkBatchImageDownloaded(ctx, job.BatchID, time.Now())
REDACTED

func (s *BatchImagePublicService) DeleteRecord(ctx context.Context, owner BatchImageOwner, batchID string) error {
	job, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return err
REDACTED
	if !isBatchImageProcessorDoneStatus(job.Status) {
		return ErrBatchImageRecordDeleteNotReady
REDACTED
	return s.Repo.MarkBatchImageJobUserDeleted(ctx, owner.UserID, owner.APIKeyID, job.BatchID, time.Now())
REDACTED

func (s *BatchImagePublicService) ListModels(ctx context.Context, owner BatchImageOwner) (*BatchImagePublicModelsResponse, error) {
	if !s.enabled() {
		return nil, ErrBatchImageDisabled
REDACTED
	if s.Pricing == nil {
		return nil, ErrBatchImageSettlementPricingMissing
REDACTED
	if err := s.ensureGroupAllowsBatchImage(ctx, owner.GroupID); err != nil {
		return nil, err
REDACTED

	modelsByProvider := make(map[string]map[string]struct{REDACTED)
	for _, providerName := range batchImageProviderSelectionOrder("") {
		provider, ok := s.ProviderRegistry.Get(providerName)
		if !ok || provider == nil {
			continue
	REDACTED
		accounts, err := s.listCandidateAccounts(ctx, owner.GroupID, batchImageProviderPlatform(providerName))
		if err != nil {
			return nil, err
	REDACTED
		for i := range accounts {
			account := accounts[i]
			if !account.IsSchedulable() || !provider.SupportsAccount(&account) {
				continue
		REDACTED
			for _, model := range batchImageModelsFromAccountMapping(&account) {
				if _, err := s.Pricing.BatchImageUnitPrice(ctx, &BatchImageJob{Provider: providerName, Model: modelREDACTED); err != nil {
					continue
			REDACTED
				if !account.IsModelSupported(model) {
					continue
			REDACTED
				if modelsByProvider[providerName] == nil {
					modelsByProvider[providerName] = make(map[string]struct{REDACTED)
			REDACTED
				modelsByProvider[providerName][model] = struct{REDACTED{REDACTED
		REDACTED
	REDACTED
REDACTED

	out := make([]BatchImagePublicModel, 0)
	for _, providerName := range batchImageProviderSelectionOrder("") {
		models := make([]string, 0, len(modelsByProvider[providerName]))
		for model := range modelsByProvider[providerName] {
			models = append(models, model)
	REDACTED
		sort.Strings(models)
		for _, model := range models {
			out = append(out, BatchImagePublicModel{
				ID:       model,
				Object:   "image.batch.model",
				Provider: providerName,
		REDACTED)
	REDACTED
REDACTED
	return &BatchImagePublicModelsResponse{Object: "list", Data: outREDACTED, nil
REDACTED

func (s *BatchImagePublicService) ListItems(ctx context.Context, owner BatchImageOwner, batchID string, query BatchImageItemsQuery) (*BatchImagePublicItemsResponse, error) {
	filter := BatchImageItemFilter{Limit: query.Limit, Offset: parseBatchImageCursor(query.Cursor)REDACTED
	switch strings.TrimSpace(query.Status) {
	case "", "all":
	case "succeeded", "success":
		filter.Status = BatchImageItemStatusSuccess
	case "pending":
		filter.Status = BatchImageItemStatusPending
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
		if job.Status == BatchImageJobStatusFailed || job.Status == BatchImageJobStatusCancelled {
			if err := releaseBatchImageBalanceHold(ctx, s.BillingRepo, job, batchImageDerefString(job.RequestHash)); err != nil {
				s.enqueueBillingRetry(ctx, job.BatchID)
				return nil, ErrBatchImageCancelFailed
		REDACTED
			s.invalidateAuthCache(ctx, owner.UserID)
	REDACTED
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
		_ = s.Repo.AppendBatchImageEvent(ctx, job.BatchID, "job_cancel_requested", map[string]any{"batch_id": job.BatchIDREDACTED)
		if s.Queue != nil {
			if err := s.Queue.Enqueue(ctx, job.BatchID); err != nil && !errors.Is(err, ErrBatchImageAlreadyQueued) {
				return nil, ErrBatchImageCancelFailed
		REDACTED
	REDACTED
		updated, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
		if err != nil {
			return nil, err
	REDACTED
		return BatchImageJobToPublic(updated), nil
REDACTED
	if err := s.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusCancelled, BatchImageTransitionOptions{
		EventType:    "job_cancelled",
		EventPayload: map[string]any{"batch_id": job.BatchIDREDACTED,
REDACTED); err != nil {
		return nil, err
REDACTED
	if err := releaseBatchImageBalanceHold(ctx, s.BillingRepo, job, batchImageDerefString(job.RequestHash)); err != nil {
		s.enqueueBillingRetry(ctx, job.BatchID)
		return nil, ErrBatchImageCancelFailed
REDACTED
	s.invalidateAuthCache(ctx, owner.UserID)
	updated, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return nil, err
REDACTED
	return BatchImageJobToPublic(updated), nil
REDACTED

func (s *BatchImagePublicService) validateSubmitRequest(req BatchImageSubmitRequest) (BatchImageSubmitRequest, error) {
	req.Model = strings.TrimSpace(req.Model)
	req.TaskName = strings.TrimSpace(req.TaskName)
	req.ParentBatchID = strings.TrimSpace(req.ParentBatchID)
	req.Provider = strings.TrimSpace(req.Provider)
	req.ResponseMimeType = strings.TrimSpace(req.ResponseMimeType)
	req.AspectRatio = strings.TrimSpace(req.AspectRatio)
	req.ImageSize = strings.TrimSpace(req.ImageSize)
	if req.Model == "" {
		return req, ErrBatchImageInvalidModel
REDACTED
	if req.TaskName == "" {
		req.TaskName = defaultBatchImageTaskName(time.Now())
REDACTED
	if len(req.TaskName) > 255 {
		req.TaskName = truncateBatchImageMessage(req.TaskName, 255)
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
	if !strings.EqualFold(req.ImageSize, defaultBatchImageImageSize) {
		return req, ErrBatchImageInvalidItems
REDACTED
	req.ImageSize = defaultBatchImageImageSize
	req.Metadata = sanitizeBatchImageMetadata(req.Metadata)

	seen := make(map[string]struct{REDACTED, len(req.Items))
	totalReferenceImages := 0
	totalInlineReferenceBytes := 0
	totalOutputImages := 0
	expandedItems := make([]BatchImageSubmitItem, 0, len(req.Items))
	for i := range req.Items {
		req.Items[i].CustomID = strings.TrimSpace(req.Items[i].CustomID)
		if req.Items[i].CustomID == "" {
			req.Items[i].CustomID = fmt.Sprintf("item_%06d", i+1)
	REDACTED
		outputCount := req.Items[i].OutputCount
		if outputCount == 0 {
			outputCount = 1
	REDACTED
		if outputCount < 1 || outputCount > s.maxOutputImagesPerItem() {
			return req, ErrBatchImageInvalidItems
	REDACTED
		totalOutputImages += outputCount
		if totalOutputImages > s.maxOutputImagesPerJob() {
			return req, ErrBatchImageTooManyOutputImages
	REDACTED
		req.Items[i].Prompt = strings.TrimSpace(req.Items[i].Prompt)
		if req.Items[i].Prompt == "" {
			return req, ErrBatchImageInvalidItems
	REDACTED
		if len(req.Items[i].Prompt) > s.maxPromptChars() {
			return req, ErrBatchImagePromptTooLong
	REDACTED
		referenceCount, inlineReferenceBytes, err := normalizeBatchImageReferenceInputs(req.Model, &req.Items[i])
		if err != nil {
			return req, err
	REDACTED
		totalReferenceImages += referenceCount * outputCount
		if totalReferenceImages > s.maxReferenceImagesPerJob() {
			return req, ErrBatchImageTooManyReferenceImages
	REDACTED
		totalInlineReferenceBytes += inlineReferenceBytes * outputCount
		if totalInlineReferenceBytes > s.maxReferenceInlineBytesPerJob() {
			return req, ErrBatchImageReferenceImagesTooLarge
	REDACTED
		for repeatIndex := 1; repeatIndex <= outputCount; repeatIndex++ {
			expanded := req.Items[i]
			expanded.OutputCount = 0
			if outputCount > 1 {
				expanded.CustomID = fmt.Sprintf("%s_%0*d", req.Items[i].CustomID, batchImageRepeatSuffixWidth(outputCount), repeatIndex)
		REDACTED
			if _, ok := seen[expanded.CustomID]; ok {
				return req, ErrBatchImageDuplicateCustomIDInRequest
		REDACTED
			seen[expanded.CustomID] = struct{REDACTED{REDACTED
			expandedItems = append(expandedItems, expanded)
	REDACTED
REDACTED
	req.Items = expandedItems
	return req, nil
REDACTED

func normalizeBatchImageReferenceInputs(model string, item *BatchImageSubmitItem) (int, int, error) {
	if item == nil || len(item.ReferenceImages) == 0 {
		return 0, 0, nil
REDACTED
	maxRefs := maxBatchImageReferenceImagesForModel(model)
	if maxRefs <= 0 || len(item.ReferenceImages) > maxRefs {
		return 0, 0, ErrBatchImageTooManyReferenceImages
REDACTED
	out := make([]BatchImageReferenceInput, 0, len(item.ReferenceImages))
	inlineBytes := 0
	for _, ref := range item.ReferenceImages {
		ref.ID = truncateBatchImageMessage(strings.TrimSpace(ref.ID), 80)
		ref.Type = truncateBatchImageMessage(strings.TrimSpace(ref.Type), 40)
		ref.MimeType = normalizeBatchImageReferenceMimeType(ref.MimeType)
		ref.FileURI = strings.TrimSpace(ref.FileURI)
		if ref.MimeType == "" {
			return 0, 0, ErrBatchImageInvalidReferenceImage
	REDACTED
		if len(ref.Data) == 0 && ref.FileURI == "" {
			return 0, 0, ErrBatchImageInvalidReferenceImage
	REDACTED
		if len(ref.Data) > 0 && ref.FileURI != "" {
			return 0, 0, ErrBatchImageInvalidReferenceImage
	REDACTED
		if len(ref.Data) > maxBatchImageReferenceImageBytes {
			return 0, 0, ErrBatchImageInvalidReferenceImage
	REDACTED
		if ref.FileURI != "" && !strings.HasPrefix(ref.FileURI, "gs://") {
			return 0, 0, ErrBatchImageInvalidReferenceImage
	REDACTED
		inlineBytes += len(ref.Data)
		out = append(out, ref)
REDACTED
	item.ReferenceImages = out
	return len(out), inlineBytes, nil
REDACTED

func normalizeBatchImageReferenceMimeType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	case "image/webp":
		return "image/webp"
	default:
		return ""
REDACTED
REDACTED

func batchImageRepeatSuffixWidth(count int) int {
	if count < 10 {
		return 2
REDACTED
	return len(strconv.Itoa(count))
REDACTED

func maxBatchImageReferenceImagesForModel(model string) int {
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(model, "pro-image") {
		return 14
REDACTED
	if strings.Contains(model, "flash-image") {
		return 3
REDACTED
	return 0
REDACTED

func (s *BatchImagePublicService) selectProviderAndAccount(ctx context.Context, owner BatchImageOwner, requestedProvider, model string) (BatchImageProvider, *Account, error) {
	providers := batchImageProviderSelectionOrder(requestedProvider)
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

func (s *BatchImagePublicService) ensureGroupAllowsBatchImage(ctx context.Context, groupID *int64) error {
	if groupID == nil || *groupID <= 0 {
		return nil
REDACTED
	if s.GroupRepo == nil {
		return ErrBatchImageSettlementPricingMissing
REDACTED
	group, err := s.GroupRepo.GetByIDLite(ctx, *groupID)
	if err != nil || group == nil {
		return ErrBatchImageSettlementPricingMissing
REDACTED
	if !group.AllowBatchImageGeneration {
		return ErrBatchImageGroupDisabled
REDACTED
	if group.Platform != PlatformGemini {
		return ErrBatchImageGroupDisabled
REDACTED
	return nil
REDACTED

func (s *BatchImagePublicService) resolvePricingSnapshot(ctx context.Context, owner BatchImageOwner, req BatchImageSubmitRequest, provider string, account *Account) (*BatchImagePricingSnapshot, error) {
	unit := -1.0
	groupMultiplier := 1.0
	discountMultiplier := defaultBatchImageDiscountMultiplier
	holdMultiplier := defaultBatchImageHoldMultiplier
	if owner.GroupID != nil && *owner.GroupID > 0 {
		if s.GroupRepo == nil {
			return nil, ErrBatchImageSettlementPricingMissing
	REDACTED
		group, err := s.GroupRepo.GetByIDLite(ctx, *owner.GroupID)
		if err != nil || group == nil {
			return nil, ErrBatchImageSettlementPricingMissing
	REDACTED
		if !group.AllowBatchImageGeneration {
			return nil, ErrBatchImageGroupDisabled
	REDACTED
		groupDefaultMultiplier := group.RateMultiplier
		if groupDefaultMultiplier < 0 {
			groupDefaultMultiplier = 0
	REDACTED
		effectiveGroupMultiplier := groupDefaultMultiplier
		if s.UserGroupRateRepo != nil {
			userRate, rateErr := s.UserGroupRateRepo.GetByUserAndGroup(ctx, owner.UserID, group.ID)
			if rateErr != nil {
				return nil, ErrBatchImageSettlementPricingMissing
		REDACTED
			if userRate != nil {
				effectiveGroupMultiplier = *userRate
		REDACTED
	REDACTED
		groupMultiplier = effectiveGroupMultiplier
		if group.ImageRateIndependent {
			groupMultiplier = group.ImageRateMultiplier
	REDACTED
		if groupMultiplier < 0 {
			groupMultiplier = 0
	REDACTED
		discountMultiplier = group.BatchImageDiscountMultiplier
		if discountMultiplier < 0 {
			discountMultiplier = 0
	REDACTED
		if group.BatchImageHoldMultiplier >= 0 {
			holdMultiplier = group.BatchImageHoldMultiplier
	REDACTED
		if configuredUnit := group.GetImagePrice(req.ImageSize); configuredUnit != nil && *configuredUnit >= 0 {
			unit = *configuredUnit
	REDACTED
REDACTED
	if unit < 0 {
		if s.Pricing == nil {
			return nil, ErrBatchImageSettlementPricingMissing
	REDACTED
		resolvedUnit, err := s.Pricing.BatchImageUnitPrice(ctx, &BatchImageJob{Provider: provider, Model: req.ModelREDACTED)
		if err != nil || resolvedUnit < 0 {
			return nil, ErrBatchImageSettlementPricingMissing
	REDACTED
		unit = resolvedUnit
REDACTED
	accountMultiplier := 1.0
	if account != nil {
		accountMultiplier = account.BillingRateMultiplier()
REDACTED
	if accountMultiplier < 0 {
		accountMultiplier = 0
REDACTED
	standardUnitPrice := unit * groupMultiplier * accountMultiplier
	billableUnitPrice := standardUnitPrice * discountMultiplier
	holdUnitPrice := standardUnitPrice * holdMultiplier
	return &BatchImagePricingSnapshot{
		BaseUnitPrice:           unit,
		GroupRateMultiplier:     groupMultiplier,
		AccountRateMultiplier:   accountMultiplier,
		BatchDiscountMultiplier: discountMultiplier,
		HoldMultiplier:          holdMultiplier,
		BillableUnitPrice:       billableUnitPrice,
		HoldUnitPrice:           holdUnitPrice,
		EstimatedCost:           billableUnitPrice * float64(len(req.Items)),
		HoldAmount:              holdUnitPrice * float64(len(req.Items)),
REDACTED, nil
REDACTED

func (s *BatchImagePublicService) enabled() bool {
	return s != nil && s.Repo != nil && s.AccountRepo != nil && s.Config != nil && s.Config.BatchImage.Enabled
REDACTED

func (s *BatchImagePublicService) invalidateAuthCache(ctx context.Context, userID int64) {
	if s != nil && s.AuthCache != nil && userID > 0 {
		s.AuthCache.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED
REDACTED

func (s *BatchImagePublicService) maxItems() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxItemsPerJobDefault > 0 {
		return s.Config.BatchImage.MaxItemsPerJobDefault
REDACTED
	return defaultBatchImageMaxItems
REDACTED

func (s *BatchImagePublicService) maxOutputImagesPerJob() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxOutputImagesPerJob > 0 {
		return s.Config.BatchImage.MaxOutputImagesPerJob
REDACTED
	return defaultBatchImageMaxOutputImages
REDACTED

func (s *BatchImagePublicService) maxOutputImagesPerItem() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxOutputImagesPerItem > 0 {
		return s.Config.BatchImage.MaxOutputImagesPerItem
REDACTED
	return defaultBatchImageMaxOutputCount
REDACTED

func (s *BatchImagePublicService) maxPromptChars() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxPromptCharsPerItem > 0 {
		return s.Config.BatchImage.MaxPromptCharsPerItem
REDACTED
	return defaultBatchImageMaxPromptChars
REDACTED

func (s *BatchImagePublicService) maxReferenceImagesPerJob() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxReferenceImagesPerJob > 0 {
		return s.Config.BatchImage.MaxReferenceImagesPerJob
REDACTED
	return defaultBatchImageMaxReferenceImages
REDACTED

func (s *BatchImagePublicService) maxReferenceInlineBytesPerJob() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.MaxReferenceInlineBytesPerJob > 0 {
		return s.Config.BatchImage.MaxReferenceInlineBytesPerJob
REDACTED
	return defaultBatchImageMaxReferenceBytes
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
	holdAmount := job.EstimatedCost
	if job.HoldAmount != nil {
		holdAmount = *job.HoldAmount
REDACTED
	return &BatchImagePublicBatch{
		ID:              job.BatchID,
		Object:          "image.batch",
		TaskName:        batchImagePublicTaskName(job),
		ParentBatchID:   job.ParentBatchID,
		Status:          PublicBatchImageStatus(job.Status),
		Model:           job.Model,
		Provider:        job.Provider,
		ItemCount:       job.ItemCount,
		SuccessCount:    job.SuccessCount,
		FailCount:       job.FailCount,
		EstimatedCost:   job.EstimatedCost,
		HoldAmount:      holdAmount,
		ActualCost:      job.ActualCost,
		CreatedAt:       job.CreatedAt.Unix(),
		SubmittedAt:     batchImageUnixPtr(job.SubmittedAt),
		SettledAt:       batchImageUnixPtr(job.SettledAt),
		DownloadedAt:    batchImageUnixPtr(job.DownloadedAt),
		OutputDeletedAt: batchImageUnixPtr(job.OutputDeletedAt),
REDACTED
REDACTED

func BatchImageItemToPublic(item *BatchImageItem) BatchImagePublicItem {
	out := BatchImagePublicItem{
		CustomID:      item.CustomID,
		Status:        "failed",
		PromptPreview: item.PromptPreview,
		MimeType:      item.MimeType,
		FileExtension: item.FileExtension,
		ImageCount:    item.ImageCount,
REDACTED
	if item.Status == BatchImageItemStatusPending {
		out.Status = "pending"
		return out
REDACTED
	if item.Status == BatchImageItemStatusSuccess {
		out.Status = "succeeded"
		return out
REDACTED
	out.Error = &BatchImagePublicError{
		Code:    batchImageDerefString(item.ErrorCode),
		Message: sanitizeBatchImagePublicMessage(batchImageDerefString(item.ErrorMessage)),
		Source:  batchImageItemErrorSource(item),
REDACTED
	return out
REDACTED

func batchImageItemErrorSource(item *BatchImageItem) string {
	if item == nil || item.ErrorCode == nil {
		return ""
REDACTED
	code := strings.TrimSpace(*item.ErrorCode)
	if batchImageDerefString(item.ProviderSourceObject) != "" {
		return "provider"
REDACTED
	switch code {
	case "EMPTY_IMAGE_OUTPUT", "PROVIDER_ITEM_FAILED":
		return "provider"
	case "INDEX_OUTPUT_MISSING", "INDEX_PARSE_FAILED", "DUPLICATE_CUSTOM_ID_IN_OUTPUT":
		return "system"
	default:
		return ""
REDACTED
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

func batchImageProviderSelectionOrder(requestedProvider string) []string {
	if strings.TrimSpace(requestedProvider) != "" {
		return []string{strings.TrimSpace(requestedProvider)REDACTED
REDACTED
	return []string{BatchImageProviderGeminiAPI, BatchImageProviderVertexREDACTED
REDACTED

func batchImageModelsFromAccountMapping(account *Account) []string {
	if account == nil {
		return nil
REDACTED
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return nil
REDACTED
	models := make(map[string]struct{REDACTED)
	for model := range mapping {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
	REDACTED
		if strings.ContainsAny(model, "*?") {
			for _, candidate := range defaultBatchImageModelCandidates() {
				if matchWildcard(model, candidate) {
					models[candidate] = struct{REDACTED{REDACTED
			REDACTED
		REDACTED
			continue
	REDACTED
		models[model] = struct{REDACTED{REDACTED
REDACTED
	out := make([]string, 0, len(models))
	for model := range models {
		out = append(out, model)
REDACTED
	sort.Strings(out)
	return out
REDACTED

func defaultBatchImageModelCandidates() []string {
	return []string{
		"gemini-2.0-flash-exp-image-generation",
		"gemini-2.5-flash-image",
		"gemini-3-pro-image",
		"gemini-3-pro-image-preview",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3.1-flash-lite-image",
REDACTED
REDACTED

func batchImageGCSRef(provider, ref string) string {
	if provider == BatchImageProviderVertex && strings.HasPrefix(strings.TrimSpace(ref), "gs://") {
		return strings.TrimSpace(ref)
REDACTED
	return ""
REDACTED

func batchImageProviderSubmitPublicError(err error) error {
	reason := strings.TrimSpace(infraerrors.Reason(err))
	switch reason {
	case "VERTEX_MANAGED_GCS_BUCKET_MISSING":
		return ErrBatchImageVertexGCSBucketMissing
	case "BATCH_IMAGE_PROVIDER_MISSING_API_KEY":
		return ErrBatchImageProviderMissingAPIKey
	case "BATCH_IMAGE_PROVIDER_MISSING_SERVICE_ACCOUNT":
		return ErrBatchImageProviderMissingServiceAccount
	case "BATCH_IMAGE_PROVIDER_UNSUPPORTED_ACCOUNT":
		return ErrBatchImageProviderUnsupportedAccount
	default:
		return ErrBatchImageProviderSubmitFailed
REDACTED
REDACTED

func batchImagePublicTaskName(job *BatchImageJob) string {
	if job == nil {
		return ""
REDACTED
	if strings.TrimSpace(job.TaskName) != "" {
		return strings.TrimSpace(job.TaskName)
REDACTED
	return defaultBatchImageTaskName(job.CreatedAt)
REDACTED

func defaultBatchImageTaskName(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
REDACTED
	return now.Format("2006-01-02 15:04:05")
REDACTED

func batchImageProviderSubmitRecordCode(err error) string {
	reason := strings.TrimSpace(infraerrors.Reason(err))
	if reason == "" || reason == "BATCH_IMAGE_PROVIDER_SUBMIT_FAILED" {
		return "PROVIDER_SUBMIT_FAILED"
REDACTED
	return reason
REDACTED

func parseBatchImageListTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
REDACTED
	if unix, err := strconv.ParseInt(raw, 10, 64); err == nil && unix > 0 {
		t := time.Unix(unix, 0)
		return &t
REDACTED
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t
REDACTED
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return &t
REDACTED
	return nil
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
