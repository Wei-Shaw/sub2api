package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAIProvisionUsageDemandStub struct {
	demand OpenAIProvisionDemand
}

func (s openAIProvisionUsageDemandStub) GetOpenAIProvisionDemand(context.Context, time.Time, time.Time) (OpenAIProvisionDemand, error) {
	return s.demand, nil
}

type openAIProvisionCapacityDemandStub struct {
	deniedUsers int64
	recordedID  atomic.Int64
}

func (s *openAIProvisionCapacityDemandStub) RecordOpenAICapacityDenied(_ context.Context, userID int64) error {
	s.recordedID.Store(userID)
	return nil
}

func (s *openAIProvisionCapacityDemandStub) GetOpenAICapacityDeniedUsers(context.Context, time.Time) (int64, error) {
	return s.deniedUsers, nil
}

func TestOpenAIProvisionDemandServiceMergesRecentCapacityDenials(t *testing.T) {
	store := &openAIProvisionCapacityDemandStub{deniedUsers: 2}
	svc := NewOpenAIProvisionDemandService(openAIProvisionUsageDemandStub{
		demand: OpenAIProvisionDemand{ActiveUsers: 3, Requests: 7, Tokens: 42},
	}, store)

	demand, err := svc.GetOpenAIProvisionDemand(context.Background(), time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	require.Equal(t, OpenAIProvisionDemand{
		ActiveUsers:            3,
		Requests:               7,
		Tokens:                 42,
		CapacityDeniedUsers:    2,
		CapacityDeniedRequests: 2,
	}, demand)

	svc.RecordOpenAICapacityDenied(context.Background(), 19)
	require.Eventually(t, func() bool { return store.recordedID.Load() == 19 }, time.Second, time.Millisecond)
}
