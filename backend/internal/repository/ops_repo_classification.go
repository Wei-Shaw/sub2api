package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetErrorClassificationStats(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsErrorClassificationStats, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil || filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}

	usageJoin, usageWhere, usageArgs, _ := buildUsageWhere(filter, filter.StartTime.UTC(), filter.EndTime.UTC(), 1)
	var successCount int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs ul "+usageJoin+" "+usageWhere, usageArgs...).Scan(&successCount); err != nil {
		return nil, err
	}

	errorWhere, errorArgs, _ := buildErrorWhere(filter, filter.StartTime.UTC(), filter.EndTime.UTC(), 1)
	q := `
SELECT
  COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400),
  COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400 AND counts_toward_sla),
  COUNT(*) FILTER (WHERE final_outcome = 'platform_failed' AND counts_toward_sla),
  COUNT(*) FILTER (WHERE final_outcome = 'provider_failed' AND counts_toward_sla),
  COUNT(*) FILTER (WHERE final_outcome = 'unknown_failed' AND counts_toward_sla),
  COUNT(*) FILTER (WHERE error_category = 'platform_capacity' AND counts_toward_sla),
  COUNT(*) FILTER (WHERE alert_family = 'compatibility' AND COALESCE(status_code, 0) >= 400),
  COUNT(*) FILTER (WHERE final_outcome = 'client_rejected'),
  COUNT(*) FILTER (WHERE final_outcome = 'business_limited'),
  COUNT(*) FILTER (WHERE final_outcome = 'cancelled'),
  COUNT(*) FILTER (WHERE final_outcome = 'security_blocked'),
  COUNT(*) FILTER (WHERE final_outcome = 'recovered' AND alert_family = 'provider_health')
FROM ops_error_logs
` + errorWhere + `
  AND is_count_tokens = FALSE`

	out := &service.OpsErrorClassificationStats{SuccessCount: successCount, DataAsOf: filter.EndTime.UTC()}
	if err := r.db.QueryRowContext(ctx, q, errorArgs...).Scan(
		&out.FinalErrorCount,
		&out.SLAFailureCount,
		&out.PlatformFailureCount,
		&out.ProviderFailureCount,
		&out.UnknownFailureCount,
		&out.PlatformCapacityCount,
		&out.CompatibilityCount,
		&out.ClientRejectedCount,
		&out.BusinessLimitedCount,
		&out.CancelledCount,
		&out.SecurityBlockedCount,
		&out.RecoveredProviderCount,
	); err != nil {
		return nil, err
	}
	return out, nil
}
