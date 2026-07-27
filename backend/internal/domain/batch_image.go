package domain

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
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
	ErrBatchImageIndexStateConflict     = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_INDEX_STATE_CONFLICT", "batch image job is no longer in indexing state")

	ErrBatchImageSettlementInvalidStatus    = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_INVALID_STATUS", "batch image job is not ready for settlement")
	ErrBatchImageSettlementManifestConflict = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_SETTLEMENT_MANIFEST_CONFLICT", "batch image settlement manifest hash conflict")
	ErrBatchImageSettlementPricingMissing   = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_PRICING_MISSING", "batch image settlement pricing is missing")
	ErrBatchImageSettlementBillingFailed    = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_SETTLEMENT_BILLING_FAILED", "batch image settlement billing failed")
	ErrBatchImageAlreadySettled             = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_ALREADY_SETTLED", "batch image job is already settled")
	ErrBatchImageSettlementMissingAPIKeyID  = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_MISSING_API_KEY_ID", "batch image settlement api key id is missing")
	ErrBatchImageSettlementMissingAccountID = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_MISSING_ACCOUNT_ID", "batch image settlement account id is missing")
	ErrBatchImageSettlementInvalidCounts    = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_INVALID_COUNTS", "batch image settlement counts are invalid")
	ErrBatchImageBillingHoldFailed          = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_BILLING_HOLD_FAILED", "batch image balance hold failed")

	ErrBatchImageDisabled                   = infraerrors.New(http.StatusNotFound, "BATCH_IMAGE_DISABLED", "batch image API is disabled")
	ErrBatchImageGroupDisabled              = infraerrors.New(http.StatusForbidden, "BATCH_IMAGE_GROUP_DISABLED", "batch image API is disabled for this group")
	ErrBatchImageInvalidModel               = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_MODEL", "batch image model is required")
	ErrBatchImageNoAccountAvailable         = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_NO_ACCOUNT_AVAILABLE", "no compatible batch image account is available")
	ErrBatchImageInvalidItems               = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_ITEMS", "batch image items are invalid")
	ErrBatchImageDuplicateCustomIDInRequest = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_DUPLICATE_CUSTOM_ID", "batch image custom ids must be unique")
	ErrBatchImagePromptTooLong              = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_PROMPT_TOO_LONG", "batch image prompt is too long")
	ErrBatchImageInvalidReferenceImage      = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_REFERENCE_IMAGE", "batch image reference image is invalid")
	ErrBatchImageTooManyReferenceImages     = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_TOO_MANY_REFERENCE_IMAGES", "too many batch image reference images for this model")
	ErrBatchImageReferenceImagesTooLarge    = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_REFERENCE_IMAGES_TOO_LARGE", "batch image reference images are too large")
	ErrBatchImageTooManyOutputImages        = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_TOO_MANY_OUTPUT_IMAGES", "too many batch image output images")
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
	ErrBatchImageDownloadTooLarge         = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_DOWNLOAD_TOO_LARGE", "batch image download is too large")
	ErrBatchImageItemImageIndexOutOfRange = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_ITEM_IMAGE_INDEX_OUT_OF_RANGE", "batch image item image index is out of range")
	ErrBatchImageZipTooManyItems          = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_ZIP_TOO_MANY_ITEMS", "batch image ZIP contains too many items; use single item downloads")
	ErrBatchImageOutputDeleteNotReady     = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_OUTPUT_DELETE_NOT_READY", "batch image output can only be deleted after completion")
	ErrBatchImageRecordDeleteNotReady     = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_RECORD_DELETE_NOT_READY", "batch image record can only be deleted after the job finishes")
	ErrBatchImageCleanupFailed            = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_CLEANUP_FAILED", "batch image cleanup failed")
	ErrBatchImageCleanupUnsafePath        = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_CLEANUP_UNSAFE_PATH", "batch image cleanup path is unsafe")
	ErrBatchImageProviderCleanupFailed    = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_PROVIDER_CLEANUP_FAILED", "batch image provider cleanup failed")

	ErrBatchImageQueueEmpty          = infraerrors.New(http.StatusNotFound, "BATCH_IMAGE_QUEUE_EMPTY", "batch image queue is empty")
	ErrBatchImageAlreadyQueued       = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_ALREADY_QUEUED", "batch image job is already queued")
	ErrInvalidBatchImageQueuePayload = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_QUEUE_INVALID_PAYLOAD", "invalid batch image queue payload")
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
}

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
}

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
}

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
}

type BatchImageItemFilter struct {
	Status string
	Limit  int
	Offset int
}

type BatchImageJobFilter struct {
	Status         string
	TaskNameLike   string
	Downloaded     *bool
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	ExcludeDeleted bool
	Limit          int
	Offset         int
}

type BatchImageCounts struct {
	SuccessCount int
	FailCount    int
}

type UpdateBatchImageJobProviderSubmitParams struct {
	BatchID           string
	ProviderJobName   string
	ProviderInputRef  string
	ProviderOutputRef string
	GCSInputURI       string
	GCSOutputURI      string
	EventPayload      any
}

type BatchImageTransitionOptions struct {
	EventType    string
	EventPayload any
	ErrorCode    *string
	ErrorMessage *string
	Now          *time.Time
}

type MarkBatchImageJobSettledParams struct {
	BatchID         string
	ActualCost      float64
	ManifestHash    string
	EventPayload    any
	Now             *time.Time
	OutputExpiresAt *time.Time
}

type BatchImageEvent struct {
	ID        int64
	JobID     string
	EventType string
	Payload   []byte
	EventHash *string
	CreatedAt time.Time
}

type ReservedBatchImageJob struct {
	BatchID string
}

func NewBatchImageID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "imgbatch_" + hex.EncodeToString(b[:]), nil
}

func IsSupportedBatchImageProvider(provider string) bool {
	switch provider {
	case BatchImageProviderGeminiAPI, BatchImageProviderVertex:
		return true
	default:
		return false
	}
}

func IsTerminalBatchImageJobStatus(status string) bool {
	switch status {
	case BatchImageJobStatusCompleted, BatchImageJobStatusFailed, BatchImageJobStatusCancelled, BatchImageJobStatusOutputDeleted:
		return true
	default:
		return false
	}
}

func CanTransitionBatchImageJob(from, to string) bool {
	if from == "" || to == "" {
		return false
	}
	if IsTerminalBatchImageJobStatus(from) {
		return to == BatchImageJobStatusOutputDeleted &&
			from != BatchImageJobStatusOutputDeleted &&
			(from == BatchImageJobStatusCompleted || from == BatchImageJobStatusFailed || from == BatchImageJobStatusCancelled)
	}
	if to == BatchImageJobStatusFailed {
		return true
	}

	allowed := map[string]map[string]struct{}{
		BatchImageJobStatusCreated: {
			BatchImageJobStatusUploading: {},
			BatchImageJobStatusSubmitted: {},
			BatchImageJobStatusCancelled: {},
		},
		BatchImageJobStatusUploading: {
			BatchImageJobStatusSubmitted: {},
			BatchImageJobStatusCancelled: {},
		},
		BatchImageJobStatusSubmitted: {
			BatchImageJobStatusRunning:   {},
			BatchImageJobStatusIndexing:  {},
			BatchImageJobStatusFailed:    {},
			BatchImageJobStatusCancelled: {},
		},
		BatchImageJobStatusRunning: {
			BatchImageJobStatusRunning:   {},
			BatchImageJobStatusIndexing:  {},
			BatchImageJobStatusFailed:    {},
			BatchImageJobStatusCancelled: {},
		},
		BatchImageJobStatusIndexing: {
			BatchImageJobStatusSettling: {},
			BatchImageJobStatusFailed:   {},
		},
		BatchImageJobStatusSettling: {
			BatchImageJobStatusCompleted: {},
		},
	}
	_, ok := allowed[from][to]
	return ok
}

func IsValidBatchImageID(batchID string) bool {
	return strings.HasPrefix(batchID, "imgbatch_") && len(batchID) > len("imgbatch_")
}
