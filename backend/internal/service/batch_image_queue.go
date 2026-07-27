package service

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/port/batchimage"
)

var (
	ErrBatchImageQueueEmpty          = domain.ErrBatchImageQueueEmpty
	ErrBatchImageAlreadyQueued       = domain.ErrBatchImageAlreadyQueued
	ErrBatchImageLockNotAcquired     = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_LOCK_NOT_ACQUIRED", "batch image job lock was not acquired")
	ErrInvalidBatchImageQueuePayload = domain.ErrInvalidBatchImageQueuePayload
)

type ReservedBatchImageJob = domain.ReservedBatchImageJob

type BatchImageJobLock = batchimage.BatchImageJobLock
type BatchImageQueue = batchimage.BatchImageQueue

type BatchImageService struct {
	repo  BatchImageRepository
	queue BatchImageQueue
}

func NewBatchImageService(repo BatchImageRepository, queue BatchImageQueue) *BatchImageService {
	return &BatchImageService{repo: repo, queue: queue}
}

func (s *BatchImageService) EnqueueBatchImageJob(ctx context.Context, batchID string) error {
	if !IsValidBatchImageID(batchID) {
		return ErrInvalidBatchImageQueuePayload
	}
	if s == nil || s.queue == nil {
		return infraerrors.New(http.StatusInternalServerError, "BATCH_IMAGE_QUEUE_NOT_CONFIGURED", "batch image queue is not configured")
	}
	if s.repo != nil {
		if _, err := s.repo.GetBatchImageJobByBatchID(ctx, batchID); err != nil {
			return err
		}
	}
	return s.queue.Enqueue(ctx, batchID)
}

func IsValidBatchImageID(batchID string) bool {
	return domain.IsValidBatchImageID(batchID)
}
