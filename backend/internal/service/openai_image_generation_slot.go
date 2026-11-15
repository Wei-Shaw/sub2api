package service

import (
	"context"
	"net/http"
	"sync"
)

type openAIImageGenerationSlotKey struct{}

// OpenAIImageGenerationSlot owns a request's image concurrency reservation.
// A durable web job can take ownership so disconnecting the request cannot
// release capacity while the provider is still generating an image.
type OpenAIImageGenerationSlot struct {
	mu       sync.Mutex
	acquire  func(context.Context) (func(), bool)
	release  func()
	acquired bool
}

func WithOpenAIImageGenerationSlot(ctx context.Context, acquire func(context.Context) (func(), bool)) (context.Context, *OpenAIImageGenerationSlot) {
	slot := &OpenAIImageGenerationSlot{acquire: acquire}
	return context.WithValue(ctx, openAIImageGenerationSlotKey{}, slot), slot
}

func imageGenerationConcurrencyError() *OpenAIImagesUpstreamError {
	return &OpenAIImagesUpstreamError{
		StatusCode: http.StatusTooManyRequests, ErrorType: "rate_limit_error", Code: "gateway_concurrency_limit",
		Message: "Image generation concurrency limit exceeded, please retry later",
	}
}

func (s *OpenAIImageGenerationSlot) acquireLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.acquired {
		return nil
	}
	if s.acquire != nil {
		release, acquired := s.acquire(ctx)
		if !acquired {
			if err := ctx.Err(); err != nil {
				return err
			}
			return imageGenerationConcurrencyError()
		}
		s.release = release
	}
	s.acquired = true
	return nil
}

// Acquire reserves capacity for a normal, request-bound image forward. Calling
// it again during the same attempt reuses the existing reservation.
func (s *OpenAIImageGenerationSlot) Acquire(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acquireLocked(ctx)
}

// Release releases only capacity still owned by the request, not a web job.
func (s *OpenAIImageGenerationSlot) Release() {
	s.mu.Lock()
	release := s.release
	s.release, s.acquired = nil, false
	s.mu.Unlock()
	if release != nil {
		release()
	}
}

func takeWebImageGenerationSlot(ctx context.Context) (func(), error) {
	slot, _ := ctx.Value(openAIImageGenerationSlotKey{}).(*OpenAIImageGenerationSlot)
	if slot == nil {
		// Internal administrator tests do not use the public gateway limiter.
		return func() {}, nil
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if err := slot.acquireLocked(ctx); err != nil {
		return nil, err
	}
	release := slot.release
	slot.release, slot.acquired = nil, false
	if release == nil {
		release = func() {}
	}
	return release, nil
}

func releaseWebImageRequestSlot(ctx context.Context) {
	if slot, _ := ctx.Value(openAIImageGenerationSlotKey{}).(*OpenAIImageGenerationSlot); slot != nil {
		slot.Release()
	}
}
