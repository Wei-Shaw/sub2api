package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultBatchImageInputRetentionAfterTerminal  = 24 * time.Hour
	defaultBatchImageOutputRetentionAfterTerminal = 72 * time.Hour
	defaultBatchImageCleanupInterval              = 30 * time.Minute
	defaultBatchImageCleanupBatchSize             = 100
)

type BatchImageCleanupService struct {
	Repo             BatchImageRepository
	ProviderRegistry *BatchImageProviderRegistry
	AccountResolver  BatchImageAccountResolver
	Config           *config.Config

	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
}

func NewBatchImageCleanupService(repo BatchImageRepository, accountRepo AccountRepository, cfg *config.Config) *BatchImageCleanupService {
	return &BatchImageCleanupService{
		Repo:             repo,
		ProviderRegistry: NewBatchImageProviderRegistryFromConfig(cfg),
		AccountResolver:  &BatchImageAccountRepositoryResolver{Repo: accountRepo},
		Config:           cfg,
	}
}

// appendCleanupEvent 追加清理审计事件；事件写入失败不阻断清理流程，但必须留痕。
func (s *BatchImageCleanupService) appendCleanupEvent(ctx context.Context, batchID, eventType string, payload any) {
	if err := s.Repo.AppendBatchImageEvent(ctx, batchID, eventType, payload); err != nil {
		logger.L().Warn("batch_image.cleanup_event_failed",
			zap.String("batch_id", batchID),
			zap.String("event_type", eventType),
			zap.Error(err),
		)
	}
}

func (s *BatchImageCleanupService) DeleteOutputsForOwner(ctx context.Context, owner BatchImageOwner, batchID string) (*BatchImagePublicBatch, error) {
	job, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return nil, err
	}
	if job.Status == BatchImageJobStatusOutputDeleted || job.OutputDeletedAt != nil {
		return BatchImageJobToPublic(job), nil
	}
	if job.Status != BatchImageJobStatusCompleted {
		return nil, ErrBatchImageOutputDeleteNotReady
	}
	s.appendCleanupEvent(ctx, job.BatchID, "manual_output_delete_requested", map[string]any{
		"batch_id":       job.BatchID,
		"cleanup_target": "output",
		"reason":         "manual",
	})
	if err := s.cleanupJob(ctx, job, CleanupTargetOutput, "manual"); err != nil {
		return nil, err
	}
	updated, err := s.Repo.GetBatchImageJobByBatchIDForOwner(ctx, owner.UserID, owner.APIKeyID, batchID)
	if err != nil {
		return nil, err
	}
	return BatchImageJobToPublic(updated), nil
}

func (s *BatchImageCleanupService) CleanupInput(ctx context.Context, batchID string) error {
	job, err := s.Repo.GetBatchImageJobByBatchID(ctx, batchID)
	if err != nil {
		return err
	}
	return s.cleanupJob(ctx, job, CleanupTargetInput, "ttl")
}

func (s *BatchImageCleanupService) CleanupOutput(ctx context.Context, batchID string, reason string) error {
	job, err := s.Repo.GetBatchImageJobByBatchID(ctx, batchID)
	if err != nil {
		return err
	}
	return s.cleanupJob(ctx, job, CleanupTargetOutput, reason)
}

func (s *BatchImageCleanupService) RunOnce(ctx context.Context, now time.Time) (BatchImageCleanupRunResult, error) {
	if s == nil || s.Repo == nil {
		return BatchImageCleanupRunResult{}, ErrBatchImageCleanupFailed
	}
	if now.IsZero() {
		now = time.Now()
	}
	limit := s.cleanupBatchSize()
	result := BatchImageCleanupRunResult{}
	inputCutoff := now.Add(-s.inputRetentionAfterTerminal())
	inputJobs, err := s.Repo.ListBatchImageJobsDueForInputCleanup(ctx, inputCutoff, limit)
	if err != nil {
		return result, err
	}
	for _, job := range inputJobs {
		if job == nil {
			continue
		}
		if err := s.cleanupJob(ctx, job, CleanupTargetInput, "ttl"); err != nil {
			result.Failures++
			continue
		}
		result.InputCleaned++
	}
	outputJobs, err := s.Repo.ListBatchImageJobsDueForOutputCleanup(ctx, now, limit)
	if err != nil {
		return result, err
	}
	for _, job := range outputJobs {
		if job == nil {
			continue
		}
		if err := s.cleanupJob(ctx, job, CleanupTargetOutput, "expired"); err != nil {
			result.Failures++
			continue
		}
		result.OutputCleaned++
	}
	return result, nil
}

func (s *BatchImageCleanupService) Start() {
	if s == nil || s.Repo == nil || s.Config == nil || !s.Config.BatchImage.Enabled || s.cleanupInterval() <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(s.cleanupInterval())
		defer ticker.Stop()
		for {
			_, _ = s.RunOnce(ctx, time.Now())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *BatchImageCleanupService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *BatchImageCleanupService) cleanupJob(ctx context.Context, job *BatchImageJob, target CleanupTarget, reason string) error {
	if job == nil {
		return ErrBatchImageJobNotFound
	}
	switch target {
	case CleanupTargetInput:
		if job.InputDeletedAt != nil {
			return nil
		}
		if !IsTerminalBatchImageJobStatus(job.Status) {
			return ErrBatchImageCleanupFailed
		}
		s.appendCleanupEvent(ctx, job.BatchID, "input_cleanup_started", cleanupEventPayload(job.BatchID, target, reason, nil))
	case CleanupTargetOutput:
		if job.OutputDeletedAt != nil || job.Status == BatchImageJobStatusOutputDeleted {
			return nil
		}
		if job.Status != BatchImageJobStatusCompleted && job.Status != BatchImageJobStatusFailed && job.Status != BatchImageJobStatusCancelled {
			return ErrBatchImageOutputDeleteNotReady
		}
		s.appendCleanupEvent(ctx, job.BatchID, "output_cleanup_started", cleanupEventPayload(job.BatchID, target, reason, nil))
	default:
		return ErrUnsupportedCleanupTarget
	}

	if err := s.callProviderCleanup(ctx, job, target); err != nil {
		code := cleanupFailureCode(err)
		msg := sanitizeBatchImagePublicMessage(err.Error())
		if recordErr := s.Repo.RecordBatchImageCleanupFailure(ctx, job.BatchID, code, msg); recordErr != nil {
			logger.L().Warn("batch_image.cleanup_failure_record_failed",
				zap.String("batch_id", job.BatchID),
				zap.Error(recordErr),
			)
		}
		event := string(target) + "_cleanup_failed"
		s.appendCleanupEvent(ctx, job.BatchID, event, map[string]any{"batch_id": job.BatchID, "cleanup_target": string(target), "reason": reason, "error_code": code})
		if errors.Is(err, ErrBatchImageProviderUnsafeCleanupPath) {
			return ErrBatchImageCleanupUnsafePath
		}
		return ErrBatchImageProviderCleanupFailed
	}

	deletedAt := time.Now()
	if target == CleanupTargetInput {
		return s.Repo.MarkBatchImageInputDeleted(ctx, job.BatchID, deletedAt)
	}
	return s.Repo.MarkBatchImageOutputDeleted(ctx, job.BatchID, deletedAt)
}

func (s *BatchImageCleanupService) callProviderCleanup(ctx context.Context, job *BatchImageJob, target CleanupTarget) error {
	if s == nil || s.ProviderRegistry == nil || s.AccountResolver == nil {
		return ErrBatchImageCleanupFailed
	}
	provider, ok := s.ProviderRegistry.Get(job.Provider)
	if !ok || provider == nil {
		return ErrBatchImageUnsupportedProvider
	}
	if job.AccountID == nil || *job.AccountID <= 0 {
		return ErrBatchImageMissingAccountID
	}
	account, err := s.AccountResolver.ResolveBatchImageAccount(ctx, *job.AccountID)
	if err != nil {
		return err
	}
	if err := provider.Cleanup(ctx, job, account, target); err != nil {
		if cleanupErrorIsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func (s *BatchImageCleanupService) inputRetentionAfterTerminal() time.Duration {
	if s != nil && s.Config != nil && s.Config.BatchImage.InputRetentionAfterTerminalHours > 0 {
		return time.Duration(s.Config.BatchImage.InputRetentionAfterTerminalHours) * time.Hour
	}
	return defaultBatchImageInputRetentionAfterTerminal
}

func (s *BatchImageCleanupService) cleanupInterval() time.Duration {
	if s != nil && s.Config != nil && s.Config.BatchImage.CleanupIntervalMinutes > 0 {
		return time.Duration(s.Config.BatchImage.CleanupIntervalMinutes) * time.Minute
	}
	return defaultBatchImageCleanupInterval
}

func (s *BatchImageCleanupService) cleanupBatchSize() int {
	if s != nil && s.Config != nil && s.Config.BatchImage.CleanupBatchSize > 0 {
		return s.Config.BatchImage.CleanupBatchSize
	}
	return defaultBatchImageCleanupBatchSize
}

type BatchImageCleanupRunResult struct {
	InputCleaned  int
	OutputCleaned int
	Failures      int
}

func cleanupEventPayload(batchID string, target CleanupTarget, reason string, deletedAt *time.Time) map[string]any {
	payload := map[string]any{
		"batch_id":       batchID,
		"cleanup_target": string(target),
		"reason":         reason,
	}
	if deletedAt != nil {
		payload["deleted_at"] = deletedAt.UTC().Format(time.RFC3339)
	}
	return payload
}

func cleanupErrorIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	reason := strings.ToUpper(infraerrors.Reason(err))
	msg := strings.ToUpper(err.Error())
	return strings.Contains(reason, "NOT_FOUND") || strings.Contains(msg, "NOT FOUND") || strings.Contains(msg, "404")
}

func cleanupFailureCode(err error) string {
	if errors.Is(err, ErrBatchImageProviderUnsafeCleanupPath) {
		return "BATCH_IMAGE_CLEANUP_UNSAFE_PATH"
	}
	reason := strings.TrimSpace(infraerrors.Reason(err))
	if reason != "" {
		return reason
	}
	return "BATCH_IMAGE_PROVIDER_CLEANUP_FAILED"
}
