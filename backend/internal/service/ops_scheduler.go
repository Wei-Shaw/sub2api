package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) GetSchedulerSnapshot(ctx context.Context) (*OpenAIAccountSchedulerSnapshot, error) {
	if s == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_SERVICE_UNAVAILABLE", "Ops service not available")
	}
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	snapshot := OpenAIAccountSchedulerSnapshot{
		GeneratedAt:    time.Now().UTC(),
		ModelCooldowns: []OpenAIAccountModelTransientSnapshot{},
	}
	if s.openAIGatewayService != nil {
		snapshot = s.openAIGatewayService.SnapshotOpenAIAccountSchedulerState()
	}
	if s.gatewayService != nil {
		snapshot.ModelCooldowns = append(snapshot.ModelCooldowns, s.gatewayService.anthropicStreamRecovery().snapshot(time.Now())...)
	}
	return &snapshot, nil
}
