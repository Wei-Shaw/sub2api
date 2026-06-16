package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSubmitUsageRecordTaskRunsTask(t *testing.T) {
	var ran bool
	h := &GatewayHandler{}
	h.submitUsageRecordTask(service.UsageRecordTask(func(ctx context.Context) {
		ran = true
	}))

	require.True(t, ran)
}

func TestOpenAISubmitUsageRecordTaskRunsTask(t *testing.T) {
	var ran bool
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(service.UsageRecordTask(func(ctx context.Context) {
		ran = true
	}))

	require.True(t, ran)
}
