package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type responsesImageStatusStoreStub struct {
	status *ResponsesImageStatus
	err    error
	sets   int
	ttl    time.Duration
}

func (s *responsesImageStatusStoreStub) GetResponsesImageStatus(context.Context, string) (*ResponsesImageStatus, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.status, nil
}

func (s *responsesImageStatusStoreStub) SetResponsesImageStatus(_ context.Context, status *ResponsesImageStatus, ttl time.Duration) error {
	s.sets++
	s.status = status
	s.ttl = ttl
	return s.err
}

func TestResponsesImageStatusBestEffortIgnoresStoreErrors(t *testing.T) {
	store := &responsesImageStatusStoreStub{err: errors.New("redis down")}
	svc := &OpenAIGatewayService{responsesImageStatusStore: store}

	require.NotPanics(t, func() {
		svc.BeginResponsesImageStatus(context.Background(), "img-1")
		svc.MarkResponsesImageStatusRunning(context.Background(), "img-1")
		svc.FailResponsesImageStatus(context.Background(), "img-1", "boom")
	})
	require.GreaterOrEqual(t, store.sets, 1)
}

func TestResponsesImageStatusLifecyclePatchesExistingRecord(t *testing.T) {
	store := &responsesImageStatusStoreStub{}
	svc := &OpenAIGatewayService{responsesImageStatusStore: store}
	ctx := WithResponsesImageStatusRequestID(context.Background(), "img-2")

	svc.BeginResponsesImageStatus(ctx, "img-2")
	svc.MarkResponsesImageStatusRunning(ctx, "img-2")
	svc.MarkResponsesImageStatusUpstreamDone(ctx, &OpenAIForwardResult{
		ImageOutputURLs: []string{"https://upstream.example/a.png"},
	})
	svc.SucceedResponsesImageStatus(ctx, &OpenAIForwardResult{
		ImageOutputURLs:    []string{"https://upstream.example/a.png"},
		ImageOutputCosURLs: []string{"https://cos.example/a.png"},
	})

	require.Equal(t, ResponsesImageStatusTTL, store.ttl)
	require.NotNil(t, store.status)
	require.Equal(t, "img-2", store.status.RequestID)
	require.Equal(t, ResponsesImageStatusSucceeded, store.status.Status)
	require.Equal(t, 100, store.status.Progress)
	require.Equal(t, []string{"https://upstream.example/a.png"}, store.status.URLs)
	require.Equal(t, []string{"https://cos.example/a.png"}, store.status.COSURLs)
}
