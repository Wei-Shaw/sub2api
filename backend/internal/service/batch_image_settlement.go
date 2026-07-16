package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	batchImageSettlementRequestPrefix = "batch_image_settlement:"
	batchImageSettlementRetryDelay    = time.Minute
	batchImageSettlementMaxRetries    = 5
	batchImageCostEpsilon             = 0.00000001
)

type BatchImagePricingResolver interface {
	BatchImageUnitPrice(ctx context.Context, job *BatchImageJob) (float64, error)
}

type BatchImageModelPricingResolver struct {
	Resolver *ModelPricingResolver
}

func (r *BatchImageModelPricingResolver) BatchImageUnitPrice(ctx context.Context, job *BatchImageJob) (float64, error) {
	if r == nil || r.Resolver == nil || job == nil || strings.TrimSpace(job.Model) == "" {
		return 0, ErrBatchImageSettlementPricingMissing
	}
	resolved := r.Resolver.Resolve(ctx, PricingInput{Model: job.Model})
	if resolved == nil {
		return 0, ErrBatchImageSettlementPricingMissing
	}
	switch resolved.Mode {
	case BillingModeImage, BillingModePerRequest:
		if resolved.DefaultPerRequestPrice > 0 {
			return resolved.DefaultPerRequestPrice, nil
		}
		if len(resolved.RequestTiers) == 1 && resolved.RequestTiers[0].PerRequestPrice != nil && *resolved.RequestTiers[0].PerRequestPrice >= 0 {
			return *resolved.RequestTiers[0].PerRequestPrice, nil
		}
	case BillingModeToken:
		if resolved.BasePricing != nil && (resolved.BasePricing.ImageOutputPriceExplicit || resolved.BasePricing.ImageOutputPricePerToken > 0) {
			return resolved.BasePricing.ImageOutputPricePerToken, nil
		}
	}
	return 0, ErrBatchImageSettlementPricingMissing
}

type BatchImageSettlementService struct {
	Repo         BatchImageRepository
	BillingRepo  UsageBillingRepository
	UsageLogRepo UsageLogRepository
	Pricing      BatchImagePricingResolver
	AuthCache    APIKeyAuthCacheInvalidator
	Config       *config.Config
}

type BatchImageSettlementResult struct {
	BatchID        string
	SuccessCount   int
	FailCount      int
	ActualCost     float64
	ManifestHash   string
	RequestID      string
	AlreadySettled bool
}

func (s *BatchImageSettlementService) Settle(ctx context.Context, batchID string) (*BatchImageSettlementResult, error) {
	if s == nil || s.Repo == nil || s.BillingRepo == nil || s.Pricing == nil {
		return nil, ErrBatchImageSettlementBillingFailed.WithCause(errors.New("batch image settlement service is not configured"))
	}
	job, err := s.Repo.GetBatchImageJobByBatchID(ctx, batchID)
	if err != nil {
		return nil, err
	}

	manifestHash := BuildBatchImageSettlementManifestHash(job)
	result := &BatchImageSettlementResult{
		BatchID:      job.BatchID,
		SuccessCount: job.SuccessCount,
		FailCount:    job.FailCount,
		ManifestHash: manifestHash,
		RequestID:    BatchImageCaptureRequestID(job.BatchID),
	}
	if job.ActualCost != nil {
		result.ActualCost = *job.ActualCost
	}
	if job.Status == BatchImageJobStatusCompleted {
		result.AlreadySettled = true
		return result, nil
	}
	if job.Status != BatchImageJobStatusSettling {
		return nil, ErrBatchImageSettlementInvalidStatus
	}
	if job.APIKeyID == nil || *job.APIKeyID <= 0 {
		return nil, ErrBatchImageSettlementMissingAPIKeyID
	}
	if job.AccountID == nil || *job.AccountID <= 0 {
		return nil, ErrBatchImageSettlementMissingAccountID
	}
	// 重试耗尽检查必须先于各类可重复失败的校验（counts/manifest/定价/超冻结），
	// 否则这些错误路径会绕过耗尽出口，settling job 无限 requeue、冻结余额永不释放。
	if isBatchImageSettlementRetryExhausted(job) {
		return nil, s.failExhaustedSettlement(ctx, job, "settlement retry limit reached: "+batchImageDerefString(job.LastErrorCode))
	}
	if job.SuccessCount < 0 || job.FailCount < 0 || job.ItemCount < 0 || job.SuccessCount+job.FailCount > job.ItemCount {
		if failErr := s.recordSettlementFailure(ctx, job, "SETTLEMENT_INVALID_COUNTS",
			fmt.Sprintf("success=%d fail=%d item_count=%d", job.SuccessCount, job.FailCount, job.ItemCount)); failErr != nil {
			return nil, failErr
		}
		return nil, ErrBatchImageSettlementInvalidCounts
	}
	if strings.TrimSpace(batchImageDerefString(job.ManifestHash)) != "" && batchImageDerefString(job.ManifestHash) != manifestHash {
		if failErr := s.recordSettlementFailure(ctx, job, "SETTLEMENT_MANIFEST_CONFLICT", "manifest hash conflict"); failErr != nil {
			return nil, failErr
		}
		return nil, ErrBatchImageSettlementManifestConflict
	}

	unitPrice, err := s.settlementUnitPrice(ctx, job)
	if err == nil && unitPrice < 0 {
		err = ErrBatchImageSettlementPricingMissing
	}
	if err != nil {
		if failErr := s.recordSettlementFailure(ctx, job, "SETTLEMENT_PRICING_MISSING", err.Error()); failErr != nil {
			return nil, failErr
		}
		return nil, err
	}
	actualCost := float64(job.SuccessCount) * unitPrice
	result.ActualCost = actualCost
	holdAmount := job.EstimatedCost
	if job.HoldAmount != nil {
		holdAmount = *job.HoldAmount
	}
	if actualCost-holdAmount > batchImageCostEpsilon {
		msg := fmt.Sprintf("actual cost %.10f exceeds held amount %.10f", actualCost, holdAmount)
		if failErr := s.recordSettlementFailure(ctx, job, "SETTLEMENT_COST_EXCEEDS_HOLD", msg); failErr != nil {
			return nil, failErr
		}
		return nil, ErrBatchImageSettlementCostExceedsHold
	}

	if err := captureBatchImageBalanceHold(ctx, s.BillingRepo, job, actualCost, manifestHash); err != nil {
		msg := truncateBatchImageMessage(err.Error(), batchImageMaxErrorMessageLength)
		if failErr := s.recordSettlementFailure(ctx, job, "SETTLEMENT_BILLING_FAILED", msg); failErr != nil {
			return nil, failErr
		}
		return nil, err
	}
	s.invalidateAuthCache(ctx, job.UserID)

	now := time.Now()
	outputExpiresAt := now.Add(s.outputRetentionAfterTerminal())
	if err := s.Repo.MarkBatchImageJobSettled(ctx, MarkBatchImageJobSettledParams{
		BatchID:         job.BatchID,
		ActualCost:      actualCost,
		ManifestHash:    manifestHash,
		Now:             &now,
		OutputExpiresAt: &outputExpiresAt,
		EventPayload: map[string]any{
			"batch_id":      job.BatchID,
			"request_id":    result.RequestID,
			"success_count": job.SuccessCount,
			"fail_count":    job.FailCount,
			"actual_cost":   actualCost,
			"manifest_hash": manifestHash,
		},
	}); err != nil {
		return nil, err
	}
	s.recordUsageLog(ctx, job, actualCost, result.RequestID, now)

	return result, nil
}

// isBatchImageSettlementRetryExhausted 判断 settling job 是否已达重试上限。
// 必须覆盖所有 SETTLEMENT_* 失败码（而非仅 SETTLEMENT_BILLING_FAILED），
// 否则 SETTLEMENT_COST_EXCEEDS_HOLD / SETTLEMENT_INVALID_COUNTS 等错误会无限 requeue。
func isBatchImageSettlementRetryExhausted(job *BatchImageJob) bool {
	return job != nil &&
		job.Status == BatchImageJobStatusSettling &&
		job.RetryCount >= batchImageSettlementMaxRetries &&
		strings.HasPrefix(batchImageDerefString(job.LastErrorCode), "SETTLEMENT_")
}

// recordSettlementFailure 记录一次结算失败并递增 retry_count。
// 重试达到上限时立即走耗尽出口（释放冻结余额并转 failed）；
// 返回非 nil 时调用方应直接返回该错误。
func (s *BatchImageSettlementService) recordSettlementFailure(ctx context.Context, job *BatchImageJob, code, message string) error {
	retryCount, recordErr := s.Repo.SetBatchImageJobSettlementFailed(ctx, job.BatchID, code, truncateBatchImageMessage(message, batchImageMaxErrorMessageLength))
	if recordErr != nil {
		logger.L().Warn("batch_image.settlement_failure_record_failed",
			zap.String("batch_id", job.BatchID),
			zap.String("code", code),
			zap.Error(recordErr),
		)
		return nil
	}
	job.RetryCount = retryCount
	job.LastErrorCode = &code
	if retryCount >= batchImageSettlementMaxRetries {
		return s.failExhaustedSettlement(ctx, job, message)
	}
	return nil
}

func (s *BatchImageSettlementService) failExhaustedSettlement(ctx context.Context, job *BatchImageJob, message string) error {
	if s == nil || s.Repo == nil {
		return ErrBatchImageSettlementBillingFailed
	}
	// 释放指纹必须与其余所有释放点（processor/Cancel/recovery）一致地使用 RequestHash：
	// 它们共享同一 request id，payloadHash 不同会触发 ErrUsageBillingRequestConflict，
	// 导致后续 Cancel/重试永远失败、terminal job 变成毒消息。
	if err := releaseBatchImageBalanceHold(ctx, s.BillingRepo, job, batchImageDerefString(job.RequestHash)); err != nil {
		msg := truncateBatchImageMessage(err.Error(), batchImageMaxErrorMessageLength)
		if _, recordErr := s.Repo.SetBatchImageJobSettlementFailed(ctx, job.BatchID, "SETTLEMENT_RELEASE_FAILED", msg); recordErr != nil {
			logger.L().Warn("batch_image.settlement_release_failure_record_failed",
				zap.String("batch_id", job.BatchID),
				zap.Error(recordErr),
			)
		}
		return ErrBatchImageSettlementBillingFailed.WithCause(err)
	}
	s.invalidateAuthCache(ctx, job.UserID)
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "settlement billing retry limit reached"
	}
	if err := s.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusFailed, BatchImageTransitionOptions{
		ErrorCode:    batchImageStringPtr("SETTLEMENT_BILLING_RETRY_EXHAUSTED"),
		ErrorMessage: batchImageStringPtr(msg),
		EventType:    "settlement_retry_exhausted",
		EventPayload: map[string]any{
			"batch_id":    job.BatchID,
			"retry_count": job.RetryCount,
		},
	}); err != nil {
		return err
	}
	return ErrBatchImageSettlementBillingFailed
}

func (s *BatchImageSettlementService) recordUsageLog(ctx context.Context, job *BatchImageJob, actualCost float64, requestID string, createdAt time.Time) {
	if s == nil || s.UsageLogRepo == nil || job == nil || job.APIKeyID == nil || job.AccountID == nil {
		return
	}
	billingMode := string(BillingModeImage)
	accountRateMultiplier := job.AccountRateMultiplier
	inboundEndpoint := "/v1/images/batches"
	upstreamEndpoint := "vertex:batchPredictionJobs"
	imageSize := "1K"
	usageLog := &UsageLog{
		UserID:                job.UserID,
		APIKeyID:              *job.APIKeyID,
		AccountID:             *job.AccountID,
		RequestID:             strings.TrimSpace(requestID),
		Model:                 job.Model,
		RequestedModel:        job.Model,
		InboundEndpoint:       &inboundEndpoint,
		UpstreamEndpoint:      &upstreamEndpoint,
		ImageCount:            job.SuccessCount,
		ImageOutputCost:       actualCost,
		TotalCost:             actualCost,
		ActualCost:            actualCost,
		RateMultiplier:        job.GroupRateMultiplier * job.BatchDiscountMultiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           BillingTypeBalance,
		RequestType:           RequestTypeSync,
		BillingMode:           &billingMode,
		ImageSize:             &imageSize,
		CreatedAt:             createdAt,
	}
	writeUsageLogBestEffort(ctx, s.UsageLogRepo, usageLog, "service.batch_image_settlement")
}

func (s *BatchImageSettlementService) invalidateAuthCache(ctx context.Context, userID int64) {
	if s != nil && s.AuthCache != nil && userID > 0 {
		s.AuthCache.InvalidateAuthCacheByUserID(ctx, userID)
	}
}

func (s *BatchImageSettlementService) settlementUnitPrice(ctx context.Context, job *BatchImageJob) (float64, error) {
	if job != nil && job.PricingSnapshotVersion >= 1 {
		if job.BillableUnitPrice < 0 {
			return 0, ErrBatchImageSettlementPricingMissing
		}
		return job.BillableUnitPrice, nil
	}
	unitPrice, err := s.Pricing.BatchImageUnitPrice(ctx, job)
	if err != nil {
		return 0, err
	}
	return unitPrice, nil
}

func (s *BatchImageSettlementService) outputRetentionAfterTerminal() time.Duration {
	if s != nil && s.Config != nil && s.Config.BatchImage.OutputRetentionAfterTerminalHours > 0 {
		return time.Duration(s.Config.BatchImage.OutputRetentionAfterTerminalHours) * time.Hour
	}
	return 72 * time.Hour
}

func BatchImageSettlementRequestID(batchID string) string {
	return batchImageSettlementRequestPrefix + strings.TrimSpace(batchID)
}

func BuildBatchImageSettlementManifestHash(job *BatchImageJob) string {
	if job == nil {
		return ""
	}
	parts := []string{
		strings.TrimSpace(job.BatchID),
		strings.TrimSpace(job.Provider),
		strings.TrimSpace(job.Model),
		batchImageDerefString(job.ProviderJobName),
		batchImageDerefString(job.ProviderOutputRef),
		strconv.Itoa(job.SuccessCount),
		strconv.Itoa(job.FailCount),
		strconv.Itoa(job.ItemCount),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

type BatchImagePipelineProcessor struct {
	ProviderProcessor *BatchImageProviderProcessor
	SettlementService *BatchImageSettlementService
	RetryDelay        time.Duration
}

func (p *BatchImagePipelineProcessor) Process(ctx context.Context, batchID string) (BatchImageProcessResult, error) {
	if p == nil || p.ProviderProcessor == nil {
		return BatchImageProcessResult{}, errors.New("batch image pipeline processor is not configured")
	}
	job, err := p.ProviderProcessor.Repo.GetBatchImageJobByBatchID(ctx, batchID)
	if err != nil {
		return BatchImageProcessResult{}, err
	}
	if job.Status == BatchImageJobStatusSettling {
		if p.SettlementService == nil {
			return BatchImageProcessResult{Terminal: true}, nil
		}
		_, err := p.SettlementService.Settle(ctx, batchID)
		if err != nil {
			if errors.Is(err, ErrBatchImageSettlementBillingFailed) {
				updated, getErr := p.ProviderProcessor.Repo.GetBatchImageJobByBatchID(ctx, batchID)
				if getErr == nil && IsTerminalBatchImageJobStatus(updated.Status) {
					return BatchImageProcessResult{Terminal: true}, nil
				}
				delay := p.RetryDelay
				if delay <= 0 {
					delay = batchImageSettlementRetryDelay
				}
				return BatchImageProcessResult{RequeueAfter: delay}, nil
			}
			return BatchImageProcessResult{}, err
		}
		return BatchImageProcessResult{Terminal: true}, nil
	}
	return p.ProviderProcessor.Process(ctx, batchID)
}

func (r *BatchImageSettlementResult) String() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("batch_id=%s success=%d fail=%d actual_cost=%0.10f already_settled=%t",
		r.BatchID, r.SuccessCount, r.FailCount, r.ActualCost, r.AlreadySettled)
}
