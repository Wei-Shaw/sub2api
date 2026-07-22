//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestShouldRecordBailianVideoUsage(t *testing.T) {
	require.True(t, shouldRecordBailianVideoUsage(service.BailianVideoEndpointGeneration, "wan2.7-t2v"))
	require.False(t, shouldRecordBailianVideoUsage(service.BailianVideoEndpointGeneration, "  "))
	require.False(t, shouldRecordBailianVideoUsage(service.BailianVideoEndpointStatus, "wan2.7-t2v"))
}

func TestBailianVideoScheduleModel(t *testing.T) {
	account := &service.Account{ID: 1, Platform: service.PlatformBailian}
	require.Equal(t, "wan2.7-t2v", bailianVideoScheduleModel(account, "wan2.7-t2v", nil))
	require.Equal(t, "upstream-model", bailianVideoScheduleModel(account, "wan2.7-t2v", &service.OpenAIForwardResult{UpstreamModel: "upstream-model"}))
	require.Equal(t, "wan2.7-t2v", bailianVideoScheduleModel(nil, " wan2.7-t2v ", nil))
}
