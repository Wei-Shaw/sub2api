package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/batchimage"
)

// BatchImage bounded-context types, errors, constants, free functions, and the
// repository contract live in the domain / port layers; this file re-exports
// them as aliases / forwarders so existing call sites and test stubs keep
// compiling. The aliases preserve type identity, so the moved constants and
// function behavior remain reachable through the service package.

const (
	BatchImageProviderGeminiAPI = domain.BatchImageProviderGeminiAPI
	BatchImageProviderVertex    = domain.BatchImageProviderVertex
)

const (
	BatchImageJobStatusCreated       = domain.BatchImageJobStatusCreated
	BatchImageJobStatusUploading     = domain.BatchImageJobStatusUploading
	BatchImageJobStatusSubmitted     = domain.BatchImageJobStatusSubmitted
	BatchImageJobStatusRunning       = domain.BatchImageJobStatusRunning
	BatchImageJobStatusIndexing      = domain.BatchImageJobStatusIndexing
	BatchImageJobStatusSettling      = domain.BatchImageJobStatusSettling
	BatchImageJobStatusCompleted     = domain.BatchImageJobStatusCompleted
	BatchImageJobStatusFailed        = domain.BatchImageJobStatusFailed
	BatchImageJobStatusCancelled     = domain.BatchImageJobStatusCancelled
	BatchImageJobStatusOutputDeleted = domain.BatchImageJobStatusOutputDeleted
)

const (
	BatchImageItemStatusPending   = domain.BatchImageItemStatusPending
	BatchImageItemStatusSuccess   = domain.BatchImageItemStatusSuccess
	BatchImageItemStatusFailed    = domain.BatchImageItemStatusFailed
	BatchImageItemStatusCancelled = domain.BatchImageItemStatusCancelled
)

var (
	ErrBatchImageJobNotFound = domain.ErrBatchImageJobNotFound
	ErrBatchImageJobExists   = domain.ErrBatchImageJobExists
	ErrBatchImageItemExists  = domain.ErrBatchImageItemExists

	ErrBatchImageInvalidTransition = domain.ErrBatchImageInvalidTransition
	ErrBatchImageInvalidProvider   = domain.ErrBatchImageInvalidProvider

	ErrBatchImageMissingProviderJobName = domain.ErrBatchImageMissingProviderJobName
	ErrBatchImageMissingAccountID       = domain.ErrBatchImageMissingAccountID
	ErrBatchImageUnsupportedProvider    = domain.ErrBatchImageUnsupportedProvider
	ErrBatchImageIndexOutputMissing     = domain.ErrBatchImageIndexOutputMissing
	ErrBatchImageIndexParseFailed       = domain.ErrBatchImageIndexParseFailed
	ErrBatchImageIndexNoResultLines     = domain.ErrBatchImageIndexNoResultLines
	ErrBatchImageDuplicateCustomID      = domain.ErrBatchImageDuplicateCustomID
	ErrBatchImageIndexStateConflict     = domain.ErrBatchImageIndexStateConflict

	ErrBatchImageSettlementInvalidStatus    = domain.ErrBatchImageSettlementInvalidStatus
	ErrBatchImageSettlementManifestConflict = domain.ErrBatchImageSettlementManifestConflict
	ErrBatchImageSettlementPricingMissing   = domain.ErrBatchImageSettlementPricingMissing
	ErrBatchImageSettlementBillingFailed    = domain.ErrBatchImageSettlementBillingFailed
	ErrBatchImageAlreadySettled             = domain.ErrBatchImageAlreadySettled
	ErrBatchImageSettlementMissingAPIKeyID  = domain.ErrBatchImageSettlementMissingAPIKeyID
	ErrBatchImageSettlementMissingAccountID = domain.ErrBatchImageSettlementMissingAccountID
	ErrBatchImageSettlementInvalidCounts    = domain.ErrBatchImageSettlementInvalidCounts
	ErrBatchImageSettlementCostExceedsHold  = domain.ErrBatchImageSettlementCostExceedsHold
	ErrBatchImageBillingHoldFailed          = domain.ErrBatchImageBillingHoldFailed
	ErrBatchImageInsufficientBalance        = domain.ErrBatchImageInsufficientBalance

	ErrBatchImageDisabled                   = domain.ErrBatchImageDisabled
	ErrBatchImageGroupDisabled              = domain.ErrBatchImageGroupDisabled
	ErrBatchImageInvalidModel               = domain.ErrBatchImageInvalidModel
	ErrBatchImageNoAccountAvailable         = domain.ErrBatchImageNoAccountAvailable
	ErrBatchImageInvalidItems               = domain.ErrBatchImageInvalidItems
	ErrBatchImageDuplicateCustomIDInRequest = domain.ErrBatchImageDuplicateCustomIDInRequest
	ErrBatchImagePromptTooLong              = domain.ErrBatchImagePromptTooLong
	ErrBatchImageInvalidReferenceImage      = domain.ErrBatchImageInvalidReferenceImage
	ErrBatchImageTooManyReferenceImages     = domain.ErrBatchImageTooManyReferenceImages
	ErrBatchImageReferenceImagesTooLarge    = domain.ErrBatchImageReferenceImagesTooLarge
	ErrBatchImageTooManyOutputImages        = domain.ErrBatchImageTooManyOutputImages
	ErrBatchImageProviderSubmitFailed       = domain.ErrBatchImageProviderSubmitFailed
	ErrBatchImageQueueFailed                = domain.ErrBatchImageQueueFailed
	ErrBatchImageIdempotencyConflict        = domain.ErrBatchImageIdempotencyConflict
	ErrBatchImageCancelFailed               = domain.ErrBatchImageCancelFailed
	ErrBatchImageVertexGCSBucketMissing     = domain.ErrBatchImageVertexGCSBucketMissing

	ErrBatchImageNotReady                 = domain.ErrBatchImageNotReady
	ErrBatchImageOutputDeleted            = domain.ErrBatchImageOutputDeleted
	ErrBatchImageItemNotFound             = domain.ErrBatchImageItemNotFound
	ErrBatchImageItemFailed               = domain.ErrBatchImageItemFailed
	ErrBatchImageResultMissing            = domain.ErrBatchImageResultMissing
	ErrBatchImageDownloadLimited          = domain.ErrBatchImageDownloadLimited
	ErrBatchImageDownloadFailed           = domain.ErrBatchImageDownloadFailed
	ErrBatchImageDownloadTooLarge         = domain.ErrBatchImageDownloadTooLarge
	ErrBatchImageItemImageIndexOutOfRange = domain.ErrBatchImageItemImageIndexOutOfRange
	ErrBatchImageZipTooManyItems          = domain.ErrBatchImageZipTooManyItems
	ErrBatchImageOutputDeleteNotReady     = domain.ErrBatchImageOutputDeleteNotReady
	ErrBatchImageRecordDeleteNotReady     = domain.ErrBatchImageRecordDeleteNotReady
	ErrBatchImageCleanupFailed            = domain.ErrBatchImageCleanupFailed
	ErrBatchImageCleanupUnsafePath        = domain.ErrBatchImageCleanupUnsafePath
	ErrBatchImageProviderCleanupFailed    = domain.ErrBatchImageProviderCleanupFailed
)

type BatchImageJob = domain.BatchImageJob
type CreateBatchImageJobParams = domain.CreateBatchImageJobParams
type BatchImageItem = domain.BatchImageItem
type CreateBatchImageItemParams = domain.CreateBatchImageItemParams
type BatchImageItemFilter = domain.BatchImageItemFilter
type BatchImageJobFilter = domain.BatchImageJobFilter
type BatchImageCounts = domain.BatchImageCounts
type UpdateBatchImageJobProviderSubmitParams = domain.UpdateBatchImageJobProviderSubmitParams
type BatchImageTransitionOptions = domain.BatchImageTransitionOptions
type MarkBatchImageJobSettledParams = domain.MarkBatchImageJobSettledParams
type BatchImageEvent = domain.BatchImageEvent

type BatchImageRepository = batchimage.BatchImageRepository

func NewBatchImageID() (string, error) { return domain.NewBatchImageID() }
func IsSupportedBatchImageProvider(provider string) bool {
	return domain.IsSupportedBatchImageProvider(provider)
}
func IsTerminalBatchImageJobStatus(status string) bool {
	return domain.IsTerminalBatchImageJobStatus(status)
}
func CanTransitionBatchImageJob(from, to string) bool {
	return domain.CanTransitionBatchImageJob(from, to)
}
