package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	BatchImageProviderGeminiAPI = "gemini_api"
	BatchImageProviderVertex    = "vertex"
)

const (
	BatchImageJobStatusCreated       = "created"
	BatchImageJobStatusUploading     = "uploading"
	BatchImageJobStatusSubmitted     = "submitted"
	BatchImageJobStatusRunning       = "running"
	BatchImageJobStatusIndexing      = "indexing"
	BatchImageJobStatusSettling      = "settling"
	BatchImageJobStatusCompleted     = "completed"
	BatchImageJobStatusFailed        = "failed"
	BatchImageJobStatusCancelled     = "cancelled"
	BatchImageJobStatusOutputDeleted = "output_deleted"
)

const (
	BatchImageItemStatusPending   = "pending"
	BatchImageItemStatusSuccess   = "success"
	BatchImageItemStatusFailed    = "failed"
	BatchImageItemStatusCancelled = "cancelled"
)

var (
	ErrBatchImageJobNotFound = infraerrors.New(http.StatusNotFound, "BATCH_IMAGE_JOB_NOT_FOUND", "batch image job not found")
	ErrBatchImageJobExists   = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_JOB_EXISTS", "batch image job already exists")
	ErrBatchImageItemExists  = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_ITEM_EXISTS", "batch image item already exists")

	ErrBatchImageInvalidTransition = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_TRANSITION", "invalid batch image job status transition")
	ErrBatchImageInvalidProvider   = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_PROVIDER", "invalid batch image provider")

	ErrBatchImageMissingProviderJobName = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_MISSING_PROVIDER_JOB_NAME", "batch image provider job name is missing")
	ErrBatchImageMissingAccountID       = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_MISSING_ACCOUNT_ID", "batch image account id is missing")
	ErrBatchImageUnsupportedProvider    = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_UNSUPPORTED_PROVIDER", "unsupported batch image provider")
	ErrBatchImageIndexOutputMissing     = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_INDEX_OUTPUT_MISSING", "batch image provider output is missing")
	ErrBatchImageIndexParseFailed       = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_INDEX_PARSE_FAILED", "batch image provider output parse failed")
	ErrBatchImageIndexNoResultLines     = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_INDEX_NO_RESULT_LINES", "batch image provider output has no result lines")
	ErrBatchImageDuplicateCustomID      = infraerrors.New(http.StatusBadGateway, "DUPLICATE_CUSTOM_ID_IN_OUTPUT", "batch image provider output contains duplicate custom id")

	ErrBatchImageSettlementInvalidStatus    = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_INVALID_STATUS", "batch image job is not ready for settlement")
	ErrBatchImageSettlementManifestConflict = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_SETTLEMENT_MANIFEST_CONFLICT", "batch image settlement manifest hash conflict")
	ErrBatchImageSettlementPricingMissing   = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_PRICING_MISSING", "batch image settlement pricing is missing")
	ErrBatchImageSettlementBillingFailed    = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_SETTLEMENT_BILLING_FAILED", "batch image settlement billing failed")
	ErrBatchImageAlreadySettled             = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_ALREADY_SETTLED", "batch image job is already settled")
	ErrBatchImageSettlementMissingAPIKeyID  = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_MISSING_API_KEY_ID", "batch image settlement api key id is missing")
	ErrBatchImageSettlementMissingAccountID = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_MISSING_ACCOUNT_ID", "batch image settlement account id is missing")
	ErrBatchImageSettlementInvalidCounts    = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_INVALID_COUNTS", "batch image settlement counts are invalid")
	ErrBatchImageSettlementCostExceedsHold  = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_SETTLEMENT_COST_EXCEEDS_HOLD", "batch image settlement cost exceeds held balance")
	ErrBatchImageBillingHoldFailed          = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_BILLING_HOLD_FAILED", "batch image balance hold failed")
	ErrBatchImageInsufficientBalance        = infraerrors.New(http.StatusPaymentRequired, "BATCH_IMAGE_INSUFFICIENT_BALANCE", "insufficient balance for batch image hold")

	ErrBatchImageDisabled                   = infraerrors.New(http.StatusNotFound, "BATCH_IMAGE_DISABLED", "batch image API is disabled")
	ErrBatchImageGroupDisabled              = infraerrors.New(http.StatusForbidden, "BATCH_IMAGE_GROUP_DISABLED", "batch image API is disabled for this group")
	ErrBatchImageInvalidModel               = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_MODEL", "batch image model is required")
	ErrBatchImageNoAccountAvailable         = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_NO_ACCOUNT_AVAILABLE", "no compatible batch image account is available")
	ErrBatchImageInvalidItems               = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_ITEMS", "batch image items are invalid")
	ErrBatchImageDuplicateCustomIDInRequest = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_DUPLICATE_CUSTOM_ID", "batch image custom ids must be unique")
	ErrBatchImagePromptTooLong              = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_PROMPT_TOO_LONG", "batch image prompt is too long")
	ErrBatchImageProviderSubmitFailed       = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_PROVIDER_SUBMIT_FAILED", "batch image provider submit failed")
	ErrBatchImageQueueFailed                = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_QUEUE_FAILED", "batch image queue failed")
	ErrBatchImageIdempotencyConflict        = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_IDEMPOTENCY_CONFLICT", "idempotency key reused with different batch image request")
	ErrBatchImageCancelFailed               = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_CANCEL_FAILED", "batch image cancel failed")
	ErrBatchImageVertexGCSBucketMissing     = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_VERTEX_GCS_BUCKET_MISSING", "Vertex managed GCS bucket is not configured")

	ErrBatchImageNotReady                 = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_NOT_READY", "batch image job is not completed")
	ErrBatchImageOutputDeleted            = infraerrors.New(http.StatusGone, "BATCH_IMAGE_OUTPUT_DELETED", "batch image output has been deleted")
	ErrBatchImageItemNotFound             = infraerrors.New(http.StatusNotFound, "BATCH_IMAGE_ITEM_NOT_FOUND", "batch image item not found")
	ErrBatchImageItemFailed               = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_ITEM_FAILED", "batch image item did not succeed")
	ErrBatchImageResultMissing            = infraerrors.New(http.StatusInternalServerError, "BATCH_IMAGE_RESULT_MISSING", "batch image result is missing")
	ErrBatchImageDownloadLimited          = infraerrors.New(http.StatusTooManyRequests, "BATCH_IMAGE_DOWNLOAD_LIMITED", "too many batch image downloads")
	ErrBatchImageDownloadFailed           = infraerrors.New(http.StatusInternalServerError, "BATCH_IMAGE_DOWNLOAD_FAILED", "batch image download failed")
	ErrBatchImageItemImageIndexOutOfRange = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_ITEM_IMAGE_INDEX_OUT_OF_RANGE", "batch image item image index is out of range")
	ErrBatchImageZipTooManyItems          = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_ZIP_TOO_MANY_ITEMS", "batch image ZIP contains too many items; use single item downloads")
	ErrBatchImageOutputDeleteNotReady     = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_OUTPUT_DELETE_NOT_READY", "batch image output can only be deleted after completion")
	ErrBatchImageRecordDeleteNotReady     = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_RECORD_DELETE_NOT_READY", "batch image record can only be deleted after the job finishes")
	ErrBatchImageCleanupFailed            = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_CLEANUP_FAILED", "batch image cleanup failed")
	ErrBatchImageCleanupUnsafePath        = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_CLEANUP_UNSAFE_PATH", "batch image cleanup path is unsafe")
	ErrBatchImageProviderCleanupFailed    = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_PROVIDER_CLEANUP_FAILED", "batch image provider cleanup failed")
)

type BatchImageJob struct {
	ID                int64
	BatchID           string
	UserID            int64
	APIKeyID          *int64
	AccountID         *int64
	Provider          string
	Model             string
	TaskName          string
	ParentBatchID     *string
	Status            string
	ProviderJobName   *string
	ProviderInputRef  *string
	ProviderOutputRef *string
	GCSInputURI       *string
	GCSOutputURI      *string

	ItemCount      int
	SuccessCount   int
	FailCount      int
	CancelledCount int

	EstimatedCost           float64
	HoldAmount              *float64
	ActualCost              *float64
	BaseUnitPrice           float64
	GroupRateMultiplier     float64
	AccountRateMultiplier   float64
	BatchDiscountMultiplier float64
	HoldMultiplier          float64
	BillableUnitPrice       float64
	HoldUnitPrice           float64
	PricingSnapshotVersion  int
	Currency                string
	HoldID                  *string

	IdempotencyKey *string
	RequestHash    *string
	ManifestHash   *string

	RetryCount int
	Version    int

	OutputExpiresAt *time.Time
	InputDeletedAt  *time.Time
	OutputDeletedAt *time.Time
	DownloadedAt    *time.Time
	UserDeletedAt   *time.Time

	LastErrorCode    *string
	LastErrorMessage *string

	CreatedAt   time.Time
	UpdatedAt   time.Time
	SubmittedAt *time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	SettledAt   *time.Time
REDACTED

type CreateBatchImageJobParams struct {
	BatchID           string
	UserID            int64
	APIKeyID          *int64
	AccountID         *int64
	Provider          string
	Model             string
	TaskName          string
	ParentBatchID     *string
	Status            string
	ProviderJobName   *string
	ProviderInputRef  *string
	ProviderOutputRef *string
	GCSInputURI       *string
	GCSOutputURI      *string

	ItemCount      int
	SuccessCount   int
	FailCount      int
	CancelledCount int

	EstimatedCost           float64
	HoldAmount              *float64
	ActualCost              *float64
	BaseUnitPrice           float64
	GroupRateMultiplier     float64
	AccountRateMultiplier   float64
	BatchDiscountMultiplier float64
	HoldMultiplier          float64
	BillableUnitPrice       float64
	HoldUnitPrice           float64
	PricingSnapshotVersion  int
	Currency                string
	HoldID                  *string

	IdempotencyKey *string
	RequestHash    *string
	ManifestHash   *string

	RetryCount int

	OutputExpiresAt *time.Time
REDACTED

type BatchImageItem struct {
	ID                   int64
	JobID                string
	CustomID             string
	Status               string
	RequestHash          *string
	PromptPreview        *string
	ProviderSourceObject *string
	SourceLineNumber     *int
	SourceByteOffset     *int64
	SourceByteLength     *int64
	MimeType             *string
	FileExtension        *string
	ImageCount           int
	ErrorCode            *string
	ErrorMessage         *string
	BilledAmount         *float64
	CreatedAt            time.Time
	IndexedAt            *time.Time
REDACTED

type CreateBatchImageItemParams struct {
	JobID                string
	CustomID             string
	Status               string
	RequestHash          *string
	PromptPreview        *string
	ProviderSourceObject *string
	SourceLineNumber     *int
	SourceByteOffset     *int64
	SourceByteLength     *int64
	MimeType             *string
	FileExtension        *string
	ImageCount           int
	ErrorCode            *string
	ErrorMessage         *string
	BilledAmount         *float64
	IndexedAt            *time.Time
REDACTED

type BatchImageItemFilter struct {
	Status string
	Limit  int
	Offset int
REDACTED

type BatchImageJobFilter struct {
	Status         string
	TaskNameLike   string
	Downloaded     *bool
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	ExcludeDeleted bool
	Limit          int
	Offset         int
REDACTED

type BatchImageCounts struct {
	SuccessCount int
	FailCount    int
REDACTED

type UpdateBatchImageJobProviderSubmitParams struct {
	BatchID           string
	ProviderJobName   string
	ProviderInputRef  string
	ProviderOutputRef string
	GCSInputURI       string
	GCSOutputURI      string
	EventPayload      any
REDACTED

type BatchImageTransitionOptions struct {
	EventType    string
	EventPayload any
	ErrorCode    *string
	ErrorMessage *string
	Now          *time.Time
REDACTED

type MarkBatchImageJobSettledParams struct {
	BatchID         string
	ActualCost      float64
	ManifestHash    string
	EventPayload    any
	Now             *time.Time
	OutputExpiresAt *time.Time
REDACTED

type BatchImageEvent struct {
	ID        int64
	JobID     string
	EventType string
	Payload   []byte
	EventHash *string
	CreatedAt time.Time
REDACTED

type BatchImageRepository interface {
	CreateBatchImageJob(ctx context.Context, params CreateBatchImageJobParams) (*BatchImageJob, error)
	GetBatchImageJobByBatchID(ctx context.Context, batchID string) (*BatchImageJob, error)
	GetBatchImageJobByIdempotencyKey(ctx context.Context, userID, apiKeyID int64, key string) (*BatchImageJob, error)
	GetBatchImageJobByBatchIDForOwner(ctx context.Context, userID, apiKeyID int64, batchID string) (*BatchImageJob, error)
	GetBatchImageJobByID(ctx context.Context, id int64) (*BatchImageJob, error)
	ListBatchImageJobsForOwner(ctx context.Context, userID, apiKeyID int64, filter BatchImageJobFilter) ([]*BatchImageJob, error)
	TransitionBatchImageJobStatus(ctx context.Context, batchID, toStatus string, opts BatchImageTransitionOptions) error
	UpdateBatchImageJobProviderOutputRef(ctx context.Context, batchID, providerOutputRef string) error
	UpdateBatchImageJobProviderSubmit(ctx context.Context, params UpdateBatchImageJobProviderSubmitParams) error
	RecordBatchImageJobSubmitFailure(ctx context.Context, batchID, code, message string, markFailed bool) error
	MarkBatchImageJobSettled(ctx context.Context, params MarkBatchImageJobSettledParams) error
	SetBatchImageJobSettlementFailed(ctx context.Context, batchID, code, message string) (int, error)
	CreateBatchImageItem(ctx context.Context, params CreateBatchImageItemParams) (*BatchImageItem, error)
	BulkCreateBatchImageItems(ctx context.Context, params []CreateBatchImageItemParams) error
	ReplaceBatchImageItemsForJob(ctx context.Context, batchID string, items []CreateBatchImageItemParams, counts BatchImageCounts) error
	ListBatchImageItems(ctx context.Context, batchID string, filter BatchImageItemFilter) ([]*BatchImageItem, error)
	ListBatchImageItemsForOwner(ctx context.Context, userID, apiKeyID int64, batchID string, filter BatchImageItemFilter) ([]*BatchImageItem, error)
	GetBatchImageJobForDownload(ctx context.Context, userID, apiKeyID int64, batchID string) (*BatchImageJob, error)
	GetBatchImageItemForDownload(ctx context.Context, batchID, customID string) (*BatchImageItem, error)
	ListBatchImageItemsForDownload(ctx context.Context, batchID string, status string, limit int) ([]*BatchImageItem, error)
	ListBatchImageJobsDueForInputCleanup(ctx context.Context, cutoff time.Time, limit int) ([]*BatchImageJob, error)
	ListBatchImageJobsDueForOutputCleanup(ctx context.Context, now time.Time, limit int) ([]*BatchImageJob, error)
	ListStaleUnsubmittedBatchImageJobs(ctx context.Context, cutoff time.Time, limit int) ([]*BatchImageJob, error)
	MarkBatchImageInputDeleted(ctx context.Context, batchID string, deletedAt time.Time) error
	MarkBatchImageOutputDeleted(ctx context.Context, batchID string, deletedAt time.Time) error
	MarkBatchImageDownloaded(ctx context.Context, batchID string, downloadedAt time.Time) error
	MarkBatchImageJobUserDeleted(ctx context.Context, userID, apiKeyID int64, batchID string, deletedAt time.Time) error
	SetBatchImageOutputExpiresAt(ctx context.Context, batchID string, expiresAt time.Time) error
	RecordBatchImageCleanupFailure(ctx context.Context, batchID, code, message string) error
	AppendBatchImageEvent(ctx context.Context, batchID, eventType string, payload any) error
REDACTED

func NewBatchImageID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
REDACTED
	return "imgbatch_" + hex.EncodeToString(b[:]), nil
REDACTED

func IsSupportedBatchImageProvider(provider string) bool {
	switch provider {
	case BatchImageProviderGeminiAPI, BatchImageProviderVertex:
		return true
	default:
		return false
REDACTED
REDACTED

func IsTerminalBatchImageJobStatus(status string) bool {
	switch status {
	case BatchImageJobStatusCompleted, BatchImageJobStatusFailed, BatchImageJobStatusCancelled, BatchImageJobStatusOutputDeleted:
		return true
	default:
		return false
REDACTED
REDACTED

func CanTransitionBatchImageJob(from, to string) bool {
	if from == "" || to == "" {
		return false
REDACTED
	if IsTerminalBatchImageJobStatus(from) {
		return to == BatchImageJobStatusOutputDeleted &&
			from != BatchImageJobStatusOutputDeleted &&
			(from == BatchImageJobStatusCompleted || from == BatchImageJobStatusFailed || from == BatchImageJobStatusCancelled)
REDACTED
	if to == BatchImageJobStatusFailed {
		return true
REDACTED

	allowed := map[string]map[string]struct{REDACTED{
		BatchImageJobStatusCreated: {
			BatchImageJobStatusUploading: {REDACTED,
			BatchImageJobStatusSubmitted: {REDACTED,
			BatchImageJobStatusCancelled: {REDACTED,
	REDACTED,
		BatchImageJobStatusUploading: {
			BatchImageJobStatusSubmitted: {REDACTED,
			BatchImageJobStatusCancelled: {REDACTED,
	REDACTED,
		BatchImageJobStatusSubmitted: {
			BatchImageJobStatusRunning:   {REDACTED,
			BatchImageJobStatusIndexing:  {REDACTED,
			BatchImageJobStatusFailed:    {REDACTED,
			BatchImageJobStatusCancelled: {REDACTED,
	REDACTED,
		BatchImageJobStatusRunning: {
			BatchImageJobStatusRunning:   {REDACTED,
			BatchImageJobStatusIndexing:  {REDACTED,
			BatchImageJobStatusFailed:    {REDACTED,
			BatchImageJobStatusCancelled: {REDACTED,
	REDACTED,
		BatchImageJobStatusIndexing: {
			BatchImageJobStatusSettling: {REDACTED,
			BatchImageJobStatusFailed:   {REDACTED,
	REDACTED,
		BatchImageJobStatusSettling: {
			BatchImageJobStatusCompleted: {REDACTED,
	REDACTED,
REDACTED
	_, ok := allowed[from][to]
	return ok
REDACTED
