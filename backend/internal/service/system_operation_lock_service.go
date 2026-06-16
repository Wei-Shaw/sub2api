package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	systemOperationLockScope = "admin.system.operations.global_lock"
	systemOperationLockKey   = "global-system-operation-lock"
)

var (
	ErrSystemOperationBusy = infraerrors.Conflict("SYSTEM_OPERATION_BUSY", "another system operation is in progress")
)

type SystemOperationLock struct {
	recordID    int64
	operationID string

	stopOnce sync.Once
	stopCh   chan struct{}
}

func (l *SystemOperationLock) OperationID() string {
	if l == nil {
		return ""
	}
	return l.operationID
}

type SystemOperationLockService struct {
	repo IdempotencyRepository

	lease         time.Duration
	renewInterval time.Duration
	ttl           time.Duration
}

func NewSystemOperationLockService(repo IdempotencyRepository, cfg IdempotencyConfig) *SystemOperationLockService {
	lease := cfg.ProcessingTimeout
	if lease <= 0 {
		lease = 30 * time.Second
	}
	renewInterval := lease / 3
	if renewInterval < time.Second {
		renewInterval = time.Second
	}
	ttl := cfg.SystemOperationTTL
	if ttl <= 0 {
		ttl = time.Hour
	}

	return &SystemOperationLockService{
		repo:          repo,
		lease:         lease,
		renewInterval: renewInterval,
		ttl:           ttl,
	}
}

func (s *SystemOperationLockService) Acquire(ctx context.Context, operationID string) (*SystemOperationLock, error) {
	if s == nil || s.repo == nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	if operationID == "" {
		return nil, infraerrors.BadRequest("SYSTEM_OPERATION_ID_REQUIRED", "operation id is required")
	}

	now := time.Now()
	expiresAt := now.Add(s.ttl)
	lockedUntil := now.Add(s.lease)
	keyHash := HashIdempotencyKey(systemOperationLockKey)

	record := &IdempotencyRecord{
		Scope:              systemOperationLockScope,
		IdempotencyKeyHash: keyHash,
		RequestFingerprint: operationID,
		Status:             IdempotencyStatusProcessing,
		LockedUntil:        &lockedUntil,
		ExpiresAt:          expiresAt,
	}

	owner, err := s.repo.CreateProcessing(ctx, record)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail.WithCause(err)
	}
	if !owner {
		existing, getErr := s.repo.GetByScopeAndKeyHash(ctx, systemOperationLockScope, keyHash)
		if getErr != nil {
			return nil, ErrIdempotencyStoreUnavail.WithCause(getErr)
		}
		if existing == nil {
			return nil, ErrIdempotencyStoreUnavail
		}
		if existing.Status == IdempotencyStatusProcessing && existing.LockedUntil != nil && existing.LockedUntil.After(now) {
			return nil, s.busyError(existing.RequestFingerprint, existing.LockedUntil, now)
		}
		reclaimed, reclaimErr := s.repo.TryReclaim(
			ctx,
			existing.ID,
			existing.Status,
			now,
			lockedUntil,
			expiresAt,
		)
		if reclaimErr != nil {
			return nil, ErrIdempotencyStoreUnavail.WithCause(reclaimErr)
		}
		if !reclaimed {
			latest, _ := s.repo.GetByScopeAndKeyHash(ctx, systemOperationLockScope, keyHash)
			if latest != nil {
				return nil, s.busyError(latest.RequestFingerprint, latest.LockedUntil, now)
			}
			return nil, ErrSystemOperationBusy
		}
		record.ID = existing.ID
	}

	if record.ID == 0 {
		return nil, ErrIdempotencyStoreUnavail
	}

	lock := &SystemOperationLock{
		recordID:    record.ID,
		operationID: operationID,
		stopCh:      make(chan struct{}),
	}
	go s.renewLoop(lock)

	return lock, nil
}

func (s *SystemOperationLockService) Release(ctx context.Context, lock *SystemOperationLock, succeeded bool, failureReason string) error {
	if s == nil || s.repo == nil || lock == nil {
		return nil
	}

	lock.stopOnce.Do(func() {
		close(lock.stopCh)
	})

	if ctx == nil {
		ctx = context.Background()
	}

	expiresAt := time.Now().Add(s.ttl)
	if succeeded {
		responseBody := fmt.Sprintf(`{"operation_id":"%s","released":true}`, lock.operationID)
		return s.repo.MarkSucceeded(ctx, lock.recordID, 200, responseBody, expiresAt)
	}

	reason := failureReason
	if reason == "" {
		reason = "SYSTEM_OPERATION_FAILED"
	}
	return s.repo.MarkFailedRetryable(ctx, lock.recordID, reason, time.Now(), expiresAt)
}

func (s *SystemOperationLockService) renewLoop(lock *SystemOperationLock) {
	ticker := time.NewTicker(s.renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			ok, err := s.repo.ExtendProcessingLock(
				ctx,
				lock.recordID,
				lock.operationID,
				now.Add(s.lease),
				now.Add(s.ttl),
			)
			cancel()
			if err != nil {
				logger.LegacyPrintf("service.system_operation_lock", "[SystemOperationLock] renew failed operation_id=%s err=%v", lock.operationID, err)
				// 瞬时故障不应导致续租协程退出，下一轮继续尝试续租。
				continue
			}
			if !ok {
				logger.LegacyPrintf("service.system_operation_lock", "[SystemOperationLock] renew stopped operation_id=%s reason=ownership_lost", lock.operationID)
				return
			}
		case <-lock.stopCh:
			return
		}
	}
}

func (s *SystemOperationLockService) busyError(operationID string, lockedUntil *time.Time, now time.Time) error {
	metadata := make(map[string]string)
	if operationID != "" {
		metadata["operation_id"] = operationID
	}
	if lockedUntil != nil {
		sec := int(lockedUntil.Sub(now).Seconds())
		if sec <= 0 {
			sec = 1
		}
		metadata["retry_after"] = strconv.Itoa(sec)
	}
	if len(metadata) == 0 {
		return ErrSystemOperationBusy
	}
	return ErrSystemOperationBusy.WithMetadata(metadata)
}
