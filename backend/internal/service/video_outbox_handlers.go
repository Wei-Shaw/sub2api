package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	VideoOutboxEventCapture = "video.capture_content"
	VideoOutboxEventArchive = "video.archive_asset"
	VideoOutboxEventCache   = "billing.invalidate_cache"
	VideoOutboxEventLow     = "billing.notify_low_balance"
	VideoOutboxEventOverrun = "billing.notify_reservation_overrun"
	VideoOutboxEventReview  = "billing.reservation_review_required"
	// VideoOutboxEventReservationExpired is emitted by the reservation reaper.
	// It is an operator-audit event and must be acknowledged, not dead-lettered
	// as an unknown event.
	VideoOutboxEventReservationExpired = "billing.reservation_expired"
)

type VideoOutboxHandlers struct {
	video       *VideoGatewayService
	outbox      DomainOutboxRepository
	billing     *BillingCacheService
	notifier    *BalanceNotifyService
	transaction VideoOutboxSideEffectRepository
	dedupMu     sync.Mutex
	dedup       map[string]struct{}
}

func NewVideoOutboxHandlers(video *VideoGatewayService, outbox DomainOutboxRepository, billing *BillingCacheService, notifier *BalanceNotifyService) *VideoOutboxHandlers {
	h := &VideoOutboxHandlers{video: video, outbox: outbox, billing: billing, notifier: notifier, dedup: make(map[string]struct{})}
	if video != nil {
		if repo, ok := video.repo.(VideoOutboxSideEffectRepository); ok {
			h.transaction = repo
		}
	}
	return h
}

func (h *VideoOutboxHandlers) Registry() DomainOutboxHandlerRegistry {
	return DomainOutboxHandlerRegistry{
		VideoOutboxEventCapture:            h,
		VideoOutboxEventArchive:            h,
		VideoOutboxEventCache:              h,
		VideoOutboxEventLow:                h,
		VideoOutboxEventOverrun:            h,
		VideoOutboxEventReview:             h,
		VideoOutboxEventReservationExpired: h,
	}
}

func (h *VideoOutboxHandlers) Handle(ctx context.Context, event *DomainOutboxEvent) error {
	if h == nil || event == nil {
		return NonRetryableDomainOutboxError(errors.New("video outbox handler is required"))
	}
	switch event.EventType {
	case VideoOutboxEventCapture:
		return h.capture(ctx, event)
	case VideoOutboxEventArchive:
		return h.archive(ctx, event)
	case VideoOutboxEventCache:
		return h.invalidateCache(ctx, event)
	case VideoOutboxEventLow:
		return h.notifyLowBalance(ctx, event)
	case VideoOutboxEventOverrun, VideoOutboxEventReview, VideoOutboxEventReservationExpired:
		// These events are durable audit/notification hooks. A configured notifier
		// may consume them; absent one they are still safely acknowledged.
		return h.notifyAudit(ctx, event)
	default:
		return NonRetryableDomainOutboxError(fmt.Errorf("%w: %s", ErrDomainOutboxUnknownEvent, event.EventType))
	}
}

func (h *VideoOutboxHandlers) capture(ctx context.Context, event *DomainOutboxEvent) error {
	task, err := h.task(ctx, event)
	if err != nil {
		return RetryableDomainOutboxError(err)
	}
	if task.Status != VideoStatusSucceeded {
		return NonRetryableDomainOutboxError(fmt.Errorf("capture requires succeeded video task, got %s", task.Status))
	}
	if task.CaptureStatus == "succeeded" {
		return nil
	}
	if h.video == nil {
		return RetryableDomainOutboxError(errors.New("video service is required for capture"))
	}
	if h.video.generationCollector == nil {
		return RetryableDomainOutboxError(errors.New("generation content collector is required for capture"))
	}
	if err := h.video.CollectVideoTaskGenerationContent(ctx, task); err != nil {
		return RetryableDomainOutboxError(err)
	}
	return nil
}

func (h *VideoOutboxHandlers) archive(ctx context.Context, event *DomainOutboxEvent) error {
	task, err := h.task(ctx, event)
	if err != nil {
		return RetryableDomainOutboxError(err)
	}
	if task.ArchiveStatus == "succeeded" || task.LocalAssetPath != "" {
		return nil
	}
	if h.video == nil {
		return RetryableDomainOutboxError(errors.New("video service is required for archive"))
	}
	if err := h.video.ArchiveSucceededVideoResultWithError(ctx, task); err != nil {
		return RetryableDomainOutboxError(err)
	}
	return nil
}

func (h *VideoOutboxHandlers) invalidateCache(ctx context.Context, event *DomainOutboxEvent) error {
	if h.billing == nil {
		return nil
	}
	if h.isDedup(event.DedupKey) {
		return nil
	}
	var p struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.Unmarshal(event.Payload, &p); err != nil {
		return NonRetryableDomainOutboxError(fmt.Errorf("invalid billing cache payload: %w", err))
	}
	if p.UserID <= 0 {
		return NonRetryableDomainOutboxError(errors.New("billing cache payload user_id is required"))
	}
	if err := h.billing.InvalidateUserBalance(ctx, p.UserID); err != nil {
		return RetryableDomainOutboxError(err)
	}
	h.markDedup(event.DedupKey)
	return nil
}

func (h *VideoOutboxHandlers) notifyLowBalance(ctx context.Context, event *DomainOutboxEvent) error {
	if h.notifier == nil {
		return nil
	}
	// The terminal transaction intentionally stores no raw notification payload;
	// notification implementations may resolve current user policy by user_id.
	var p struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.Unmarshal(event.Payload, &p); err != nil {
		return NonRetryableDomainOutboxError(fmt.Errorf("invalid low-balance payload: %w", err))
	}
	if p.UserID <= 0 {
		return NonRetryableDomainOutboxError(errors.New("low-balance payload user_id is required"))
	}
	if h.isDedup(event.DedupKey) {
		return nil
	}
	user, err := h.videoUser(ctx, p.UserID)
	if err != nil {
		return RetryableDomainOutboxError(err)
	}
	task, taskErr := h.task(ctx, event)
	if taskErr != nil {
		return RetryableDomainOutboxError(taskErr)
	}
	cost := task.CostEstimate
	if cost < 0 {
		cost = 0
	}
	// Resolve policy through the existing service; this path never mutates
	// balance and only emits a notification on a genuine threshold crossing.
	h.notifier.CheckBalanceAfterDeduction(ctx, user, user.Balance+cost, cost)
	h.markDedup(event.DedupKey)
	return nil
}

func (h *VideoOutboxHandlers) notifyAudit(_ context.Context, event *DomainOutboxEvent) error {
	if !h.markDedup(event.DedupKey) {
		return nil
	}
	slog.Info("video billing outbox notification", "event_type", event.EventType, "aggregate_id", event.AggregateID)
	return nil
}

func (h *VideoOutboxHandlers) task(ctx context.Context, event *DomainOutboxEvent) (*VideoTask, error) {
	if h.video == nil || h.video.repo == nil {
		return nil, errors.New("video repository is required")
	}
	if event.AggregateID <= 0 {
		return nil, errors.New("video outbox aggregate id is required")
	}
	return h.video.repo.GetTask(ctx, event.AggregateID)
}

func (h *VideoOutboxHandlers) videoUser(ctx context.Context, id int64) (*User, error) {
	if h.video == nil || h.video.userRepo == nil {
		return nil, errors.New("user repository is required")
	}
	return h.video.userRepo.GetByID(ctx, id)
}

func (h *VideoOutboxHandlers) markDedup(key string) bool {
	h.dedupMu.Lock()
	defer h.dedupMu.Unlock()
	if _, exists := h.dedup[key]; exists {
		return false
	}
	h.dedup[key] = struct{}{}
	return true
}

func (h *VideoOutboxHandlers) isDedup(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	h.dedupMu.Lock()
	defer h.dedupMu.Unlock()
	_, exists := h.dedup[key]
	return exists
}

func (h *VideoOutboxHandlers) Complete(ctx context.Context, event *DomainOutboxEvent, workerID string, completedAt time.Time) (bool, error) {
	effect := ""
	switch event.EventType {
	case VideoOutboxEventCapture:
		effect = "capture"
	case VideoOutboxEventArchive:
		effect = "archive"
	}
	if effect == "" || h.transaction == nil {
		return false, nil
	}
	return true, func() error {
		_, err := h.transaction.CompleteVideoOutboxSideEffect(ctx, event.ID, workerID, completedAt, event.AggregateID, effect)
		return err
	}()
}

func (h *VideoOutboxHandlers) Dead(ctx context.Context, event *DomainOutboxEvent, workerID string, nextAttemptAt time.Time, lastError string) (bool, error) {
	effect := ""
	switch event.EventType {
	case VideoOutboxEventCapture:
		effect = "capture"
	case VideoOutboxEventArchive:
		effect = "archive"
	}
	if effect == "" || h.transaction == nil {
		return false, nil
	}
	return true, func() error {
		_, err := h.transaction.DeadVideoOutboxSideEffect(ctx, event.ID, workerID, nextAttemptAt, event.AggregateID, effect, lastError)
		return err
	}()
}
