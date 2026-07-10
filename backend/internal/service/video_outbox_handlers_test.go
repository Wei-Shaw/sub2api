package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type videoOutboxRepoFake struct {
	VideoGatewayRepository
	task      *VideoTask
	completed int
	dead      int
}

func (r *videoOutboxRepoFake) GetTask(context.Context, int64) (*VideoTask, error) {
	if r.task == nil {
		return nil, ErrVideoTaskNotFound
	}
	clone := *r.task
	return &clone, nil
}
func (r *videoOutboxRepoFake) CompleteVideoOutboxSideEffect(context.Context, int64, string, time.Time, int64, string) (bool, error) {
	r.completed++
	return true, nil
}
func (r *videoOutboxRepoFake) DeadVideoOutboxSideEffect(context.Context, int64, string, time.Time, int64, string, string) (bool, error) {
	r.dead++
	return true, nil
}

func TestVideoOutboxHandlersUnknownEventIsNonRetryable(t *testing.T) {
	h := NewVideoOutboxHandlers(nil, nil, nil, nil)
	err := h.Handle(context.Background(), &DomainOutboxEvent{ID: 1, AggregateID: 10, EventType: "not-supported"})
	var dead *DomainOutboxDeadError
	if !errors.As(err, &dead) {
		t.Fatalf("error = %T %v, want DomainOutboxDeadError", err, err)
	}
}

func TestVideoOutboxHandlersRegistryIncludesAllDomainEvents(t *testing.T) {
	h := NewVideoOutboxHandlers(nil, nil, nil, nil)
	registry := h.Registry()
	for _, eventType := range []string{VideoOutboxEventCapture, VideoOutboxEventArchive, VideoOutboxEventCache, VideoOutboxEventLow, VideoOutboxEventOverrun, VideoOutboxEventReview, VideoOutboxEventReservationExpired} {
		if registry[eventType] == nil {
			t.Fatalf("missing handler for %s", eventType)
		}
	}
}

func TestVideoOutboxHandlersAcknowledgesReservationExpiredAuditEvent(t *testing.T) {
	h := NewVideoOutboxHandlers(nil, nil, nil, nil)
	err := h.Handle(context.Background(), &DomainOutboxEvent{ID: 1, AggregateID: 10, DedupKey: "expired-10", EventType: VideoOutboxEventReservationExpired})
	if err != nil {
		t.Fatalf("reservation-expired event must be acknowledged as an audit event: %v", err)
	}
}

func TestVideoOutboxHandlersCaptureAndArchiveReplayAreNoOpsAfterSuccess(t *testing.T) {
	repo := &videoOutboxRepoFake{task: &VideoTask{ID: 7, Status: VideoStatusSucceeded, CaptureStatus: "succeeded", ArchiveStatus: "succeeded", LocalAssetPath: "assets/video/7/result.mp4"}}
	video := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	h := NewVideoOutboxHandlers(video, nil, nil, nil)
	for _, eventType := range []string{VideoOutboxEventCapture, VideoOutboxEventArchive} {
		if err := h.Handle(context.Background(), &DomainOutboxEvent{ID: 1, AggregateID: 7, EventType: eventType}); err != nil {
			t.Fatalf("%s: %v", eventType, err)
		}
	}
	if _, err := h.Complete(context.Background(), &DomainOutboxEvent{ID: 1, AggregateID: 7, EventType: VideoOutboxEventCapture}, "worker", time.Now()); err != nil {
		t.Fatal(err)
	}
	if repo.completed != 1 {
		t.Fatalf("transactional completion calls = %d", repo.completed)
	}
}

func TestVideoOutboxHandlersCaptureWithoutCollectorIsRetryable(t *testing.T) {
	repo := &videoOutboxRepoFake{task: &VideoTask{ID: 7, Status: VideoStatusSucceeded, CaptureStatus: "pending"}}
	video := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, &config.Config{Gateway: config.GatewayConfig{ContentCapture: config.ContentCaptureConfig{Enabled: true}}})
	h := NewVideoOutboxHandlers(video, nil, nil, nil)
	err := h.Handle(context.Background(), &DomainOutboxEvent{ID: 1, AggregateID: 7, EventType: VideoOutboxEventCapture})
	var retry *DomainOutboxRetryError
	if !errors.As(err, &retry) {
		t.Fatalf("error = %T %v, want retry", err, err)
	}
}
