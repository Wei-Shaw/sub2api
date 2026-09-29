package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	batchImageHoldRequestPrefix    = "batch_image_hold:"
	batchImageCaptureRequestPrefix = "batch_image_capture:"
	batchImageReleaseRequestPrefix = "batch_image_release:"
)

func BatchImageHoldRequestID(batchID string) string {
	return batchImageHoldRequestPrefix + strings.TrimSpace(batchID)
}

func BatchImageCaptureRequestID(batchID string) string {
	return batchImageCaptureRequestPrefix + strings.TrimSpace(batchID)
}

func BatchImageReleaseRequestID(batchID string) string {
	return batchImageReleaseRequestPrefix + strings.TrimSpace(batchID)
}

func buildBatchImageHoldCommand(job *BatchImageJob, requestID string, actualAmount float64, payloadHash string) (*BatchImageBalanceHoldCommand, error) {
	if job == nil {
		return nil, ErrBatchImageBillingHoldFailed
	}
	if job.APIKeyID == nil || *job.APIKeyID <= 0 {
		return nil, ErrBatchImageSettlementMissingAPIKeyID
	}
	holdAmount := job.EstimatedCost
	if job.HoldAmount != nil {
		holdAmount = *job.HoldAmount
	}
	if holdAmount < 0 {
		holdAmount = 0
	}
	if actualAmount < 0 {
		actualAmount = 0
	}
	return &BatchImageBalanceHoldCommand{
		RequestID:          requestID,
		APIKeyID:           *job.APIKeyID,
		UserID:             job.UserID,
		BatchID:            job.BatchID,
		HoldAmount:         holdAmount,
		ActualAmount:       actualAmount,
		RequestPayloadHash: strings.TrimSpace(payloadHash),
	}, nil
}

func reserveBatchImageBalanceHold(ctx context.Context, repo UsageBillingRepository, job *BatchImageJob, payloadHash string) error {
	if repo == nil {
		return ErrBatchImageBillingHoldFailed.WithCause(errors.New("batch image billing repository is not configured"))
	}
	cmd, err := buildBatchImageHoldCommand(job, BatchImageHoldRequestID(job.BatchID), 0, payloadHash)
	if err != nil {
		return err
	}
	if cmd.HoldAmount <= 0 {
		return nil
	}
	if _, err := repo.ReserveBatchImageBalance(ctx, cmd); err != nil {
		if errors.Is(err, ErrBatchImageInsufficientBalance) {
			return ErrBatchImageInsufficientBalance
		}
		return ErrBatchImageBillingHoldFailed.WithCause(err)
	}
	return nil
}

// captureBatchImageBalanceHold 核销冻结额，并在同一事务里把 limitCosts 计入 API Key 额度与窗口。
func captureBatchImageBalanceHold(ctx context.Context, repo UsageBillingRepository, job *BatchImageJob, actualAmount float64, payloadHash string, limitCosts batchImageAPIKeyLimitCosts) (*BatchImageBalanceHoldResult, error) {
	if repo == nil {
		return nil, ErrBatchImageSettlementBillingFailed.WithCause(errors.New("batch image billing repository is not configured"))
	}
	cmd, err := buildBatchImageHoldCommand(job, BatchImageCaptureRequestID(job.BatchID), actualAmount, payloadHash)
	if err != nil {
		return nil, err
	}
	cmd.APIKeyQuotaCost = limitCosts.QuotaCost
	cmd.APIKeyRateLimitCost = limitCosts.RateLimitCost
	result, err := repo.CaptureBatchImageBalance(ctx, cmd)
	if err != nil {
		return nil, ErrBatchImageSettlementBillingFailed.WithCause(err)
	}
	return result, nil
}

// batchImageAPIKeyLimitCosts 是核销金额需要计入的 API Key 总额度 / 5h·1d·7d 窗口用量。
type batchImageAPIKeyLimitCosts struct {
	QuotaCost     float64
	RateLimitCost float64
}

// resolveBatchImageAPIKeyLimitCosts 与 buildUsageBillingCommand 同一口径：
// Key 设了总额度才累加 quota_used，配置了窗口才累加 5h/1d/7d；
// 简易模式只在显式开启 Key 窗口计费时累加窗口，否则不计。
func resolveBatchImageAPIKeyLimitCosts(cfg *config.Config, apiKey *APIKey, actualCost float64) batchImageAPIKeyLimitCosts {
	if apiKey == nil || actualCost <= 0 {
		return batchImageAPIKeyLimitCosts{}
	}
	if cfg != nil && cfg.RunMode == config.RunModeSimple {
		if simpleModeKeyRateLimitBillingEnabled(cfg, apiKey) {
			return batchImageAPIKeyLimitCosts{RateLimitCost: actualCost}
		}
		return batchImageAPIKeyLimitCosts{}
	}
	var costs batchImageAPIKeyLimitCosts
	if apiKey.Quota > 0 {
		costs.QuotaCost = actualCost
	}
	if apiKey.HasRateLimits() {
		costs.RateLimitCost = actualCost
	}
	return costs
}

func releaseBatchImageBalanceHold(ctx context.Context, repo UsageBillingRepository, job *BatchImageJob, payloadHash string) error {
	if repo == nil || job == nil {
		return nil
	}
	cmd, err := buildBatchImageHoldCommand(job, BatchImageReleaseRequestID(job.BatchID), 0, payloadHash)
	if err != nil {
		return err
	}
	if cmd.HoldAmount <= 0 {
		return nil
	}
	if _, err := repo.ReleaseBatchImageBalance(ctx, cmd); err != nil {
		// 同一 release request id 出现指纹冲突，说明此前已有一次携带不同
		// payloadHash 的释放成功提交（资金已归还）。视为幂等成功，
		// 避免历史指纹不一致的 job 永远卡在释放失败的毒消息循环里。
		if errors.Is(err, ErrUsageBillingRequestConflict) {
			logger.L().Warn("batch_image.release_fingerprint_conflict_treated_as_released",
				zap.String("batch_id", job.BatchID),
			)
			return nil
		}
		return ErrBatchImageBillingHoldFailed.WithCause(err)
	}
	return nil
}
