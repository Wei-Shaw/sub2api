// Package ops contains the port interfaces for the Ops bounded context: the
// core OpsRepository (read-model + write-model contract) and the smaller
// OpsIngressRejectRepository. All method signatures reference pure-scalar
// read/write models defined in internal/domain, so the repository layer can
// implement these contracts without importing internal/service. The service
// package keeps a type alias to each interface so existing call sites and
// test stubs continue to satisfy them.
package ops

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// OpsRepository is the core Ops/dashboard BC repository contract (38 methods).
type OpsRepository interface {
	InsertErrorLog(ctx context.Context, input *domain.OpsInsertErrorLogInput) (int64, error)
	BatchInsertErrorLogs(ctx context.Context, inputs []*domain.OpsInsertErrorLogInput) (int64, error)
	ListErrorLogs(ctx context.Context, filter *domain.OpsErrorLogFilter) (*domain.OpsErrorLogList, error)
	GetErrorLogByID(ctx context.Context, id int64) (*domain.OpsErrorLogDetail, error)
	ListRequestDetails(ctx context.Context, filter *domain.OpsRequestDetailFilter) ([]*domain.OpsRequestDetail, int64, error)
	BatchInsertSystemLogs(ctx context.Context, inputs []*domain.OpsInsertSystemLogInput) (int64, error)
	ListSystemLogs(ctx context.Context, filter *domain.OpsSystemLogFilter) (*domain.OpsSystemLogList, error)
	DeleteSystemLogs(ctx context.Context, filter *domain.OpsSystemLogCleanupFilter) (int64, error)
	InsertSystemLogCleanupAudit(ctx context.Context, input *domain.OpsSystemLogCleanupAudit) error

	UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64, resolvedAt *time.Time) error

	// Lightweight window stats (for realtime WS / quick sampling).
	GetWindowStats(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsWindowStats, error)
	// Lightweight realtime traffic summary (for the Ops dashboard header card).
	GetRealtimeTrafficSummary(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsRealtimeTrafficSummary, error)

	GetDashboardOverview(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsDashboardOverview, error)
	GetThroughputTrend(ctx context.Context, filter *domain.OpsDashboardFilter, bucketSeconds int) (*domain.OpsThroughputTrendResponse, error)
	GetLatencyHistogram(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsLatencyHistogramResponse, error)
	GetErrorTrend(ctx context.Context, filter *domain.OpsDashboardFilter, bucketSeconds int) (*domain.OpsErrorTrendResponse, error)
	GetErrorDistribution(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsErrorDistributionResponse, error)
	GetOpenAITokenStats(ctx context.Context, filter *domain.OpsOpenAITokenStatsFilter) (*domain.OpsOpenAITokenStatsResponse, error)

	InsertSystemMetrics(ctx context.Context, input *domain.OpsInsertSystemMetricsInput) error
	GetLatestSystemMetrics(ctx context.Context, windowMinutes int) (*domain.OpsSystemMetricsSnapshot, error)

	UpsertJobHeartbeat(ctx context.Context, input *domain.OpsUpsertJobHeartbeatInput) error
	ListJobHeartbeats(ctx context.Context) ([]*domain.OpsJobHeartbeat, error)

	// Alerts (rules + events)
	ListAlertRules(ctx context.Context) ([]*domain.OpsAlertRule, error)
	CreateAlertRule(ctx context.Context, input *domain.OpsAlertRule) (*domain.OpsAlertRule, error)
	UpdateAlertRule(ctx context.Context, input *domain.OpsAlertRule) (*domain.OpsAlertRule, error)
	DeleteAlertRule(ctx context.Context, id int64) error

	ListAlertEvents(ctx context.Context, filter *domain.OpsAlertEventFilter) ([]*domain.OpsAlertEvent, error)
	GetAlertEventByID(ctx context.Context, eventID int64) (*domain.OpsAlertEvent, error)
	GetActiveAlertEvent(ctx context.Context, ruleID int64) (*domain.OpsAlertEvent, error)
	GetLatestAlertEvent(ctx context.Context, ruleID int64) (*domain.OpsAlertEvent, error)
	CreateAlertEvent(ctx context.Context, event *domain.OpsAlertEvent) (*domain.OpsAlertEvent, error)
	UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error
	UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error

	// Alert silences
	CreateAlertSilence(ctx context.Context, input *domain.OpsAlertSilence) (*domain.OpsAlertSilence, error)
	IsAlertSilenced(ctx context.Context, ruleID int64, platform string, groupID *int64, region *string, now time.Time) (bool, error)

	// Pre-aggregation (hourly/daily) used for long-window dashboard performance.
	UpsertHourlyMetrics(ctx context.Context, startTime, endTime time.Time) error
	UpsertDailyMetrics(ctx context.Context, startTime, endTime time.Time) error
	GetLatestHourlyBucketStart(ctx context.Context) (time.Time, bool, error)
	GetLatestDailyBucketDate(ctx context.Context) (time.Time, bool, error)
}

// OpsIngressRejectRepository is the ingress-reject aggregating repository contract.
type OpsIngressRejectRepository interface {
	BatchUpsertIngressRejects(ctx context.Context, items []*domain.OpsIngressRejectAggregate) error
	ListIngressRejects(ctx context.Context, filter *domain.OpsIngressRejectFilter) (*domain.OpsIngressRejectList, error)
}
