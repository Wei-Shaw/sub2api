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

func (s *responsesImageStatusStoreStub) GetResponsesImageStatuses(context.Context, []string) (map[string]*ResponsesImageStatus, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.status == nil {
		return map[string]*ResponsesImageStatus{}, nil
	}
	return map[string]*ResponsesImageStatus{s.status.RequestID: s.status}, nil
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
		ImageOutputURLs:  []string{"https://upstream.example/a.png"},
		ImageOutputTexts: []string{"image is ready"},
	})
	svc.SucceedResponsesImageStatus(ctx, &OpenAIForwardResult{
		ImageOutputURLs:    []string{"https://upstream.example/a.png"},
		ImageOutputCosURLs: []string{"https://cos.example/a.png"},
		ImageOutputTexts:   []string{"image is ready"},
	})

	require.Equal(t, ResponsesImageStatusTTL, store.ttl)
	require.NotNil(t, store.status)
	require.Equal(t, "img-2", store.status.RequestID)
	require.Equal(t, ResponsesImageStatusSucceeded, store.status.Status)
	require.Equal(t, 100, store.status.Progress)
	require.Equal(t, []string{"https://upstream.example/a.png"}, store.status.URLs)
	require.Equal(t, []string{"https://cos.example/a.png"}, store.status.COSURLs)
	require.Equal(t, []string{"image is ready"}, store.status.Texts)
}

func TestResponsesImageStatusIgnoresCanceledContext(t *testing.T) {
	store := &responsesImageStatusStoreStub{}
	svc := &OpenAIGatewayService{responsesImageStatusStore: store}
	ctx := WithResponsesImageStatusRequestID(context.Background(), "img-canceled")
	ctx, cancel := context.WithCancel(ctx)
	cancel()

	svc.BeginResponsesImageStatus(ctx, "img-canceled")
	svc.MarkResponsesImageStatusRunning(ctx, "img-canceled")
	svc.SucceedResponsesImageStatus(ctx, &OpenAIForwardResult{
		ImageOutputURLs:  []string{"https://upstream.example/canceled.png"},
		ImageOutputTexts: []string{"completed after disconnect"},
	})

	require.NotNil(t, store.status)
	require.Equal(t, "img-canceled", store.status.RequestID)
	require.Equal(t, ResponsesImageStatusSucceeded, store.status.Status)
	require.Equal(t, 100, store.status.Progress)
	require.Equal(t, []string{"https://upstream.example/canceled.png"}, store.status.URLs)
	require.Equal(t, []string{"completed after disconnect"}, store.status.Texts)
}
