package service

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) GetDashboardOverview(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}

	// Resolve query mode (requested via query param, or DB default).
	filter.QueryMode = s.resolveOpsQueryMode(ctx, filter.QueryMode)

	overview, err := s.opsRepo.GetDashboardOverview(ctx, filter)
	if err != nil && shouldFallbackOpsPreagg(filter, err) {
		rawFilter := cloneOpsFilterWithMode(filter, OpsQueryModeRaw)
		overview, err = s.opsRepo.GetDashboardOverview(ctx, rawFilter)
	}
	if err != nil {
		if errors.Is(err, ErrOpsPreaggregatedNotPopulated) {
			return nil, infraerrors.Conflict("OPS_PREAGG_NOT_READY", "Pre-aggregated ops metrics are not populated yet")
		}
		return nil, err
	}

	// Best-effort system health + jobs; dashboard metrics should still render if these are missing.
	if metrics, err := s.opsRepo.GetLatestSystemMetrics(ctx, 1); err == nil {
		// Attach config-derived limits so the UI can show "current / max" for connection pools.
		// These are best-effort and should never block the dashboard rendering.
		if s != nil && s.cfg != nil {
			if s.cfg.Database.MaxOpenConns > 0 {
				metrics.DBMaxOpenConns = intPtr(s.cfg.Database.MaxOpenConns)
			}
			if s.cfg.Redis.PoolSize > 0 {
				metrics.RedisPoolSize = intPtr(s.cfg.Redis.PoolSize)
			}
		}
		overview.SystemMetrics = metrics
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("[Ops] GetLatestSystemMetrics failed: %v", err)
	}

	if heartbeats, err := s.opsRepo.ListJobHeartbeats(ctx); err == nil {
		overview.JobHeartbeats = heartbeats
	} else {
		log.Printf("[Ops] ListJobHeartbeats failed: %v", err)
	}

	overview.HealthScore = computeDashboardHealthScore(time.Now().UTC(), overview)

	return overview, nil
}

func (s *OpsService) resolveOpsQueryMode(ctx context.Context, requested OpsQueryMode) OpsQueryMode {
	if requested.IsValid() {
		// Allow "auto" to be disabled via config until preagg is proven stable in production.
		// Forced `preagg` via query param still works.
		if requested == OpsQueryModeAuto && s != nil && s.cfg != nil && !s.cfg.Ops.UsePreaggregatedTables {
			return OpsQueryModeRaw
		}
		return requested
	}

	mode := OpsQueryModeAuto
	if s != nil && s.settingRepo != nil {
		if raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsQueryModeDefault); err == nil {
			mode = ParseOpsQueryMode(raw)
		}
	}

	if mode == OpsQueryModeAuto && s != nil && s.cfg != nil && !s.cfg.Ops.UsePreaggregatedTables {
		return OpsQueryModeRaw
	}
	return mode
}
