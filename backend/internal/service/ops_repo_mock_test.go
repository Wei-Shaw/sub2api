package service

import (
	"context"
	"time"
)

// opsRepoMock is a test-only OpsRepository implementation with optional function hooks.
type opsRepoMock struct {
	InsertErrorLogFn              func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error)
	BatchInsertErrorLogsFn        func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error)
	BatchInsertSystemLogsFn       func(ctx context.Context, inputs []*OpsInsertSystemLogInput) (int64, error)
	ListSystemLogsFn              func(ctx context.Context, filter *OpsSystemLogFilter) (*OpsSystemLogList, error)
	DeleteSystemLogsFn            func(ctx context.Context, filter *OpsSystemLogCleanupFilter) (int64, error)
	InsertSystemLogCleanupAuditFn func(ctx context.Context, input *OpsSystemLogCleanupAudit) error
	LookupDeletedKeyAuditFn       func(ctx context.Context, key string) (*DeletedKeyAuditResult, error)
REDACTED

func (m *opsRepoMock) InsertErrorLog(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
	if m.InsertErrorLogFn != nil {
		return m.InsertErrorLogFn(ctx, input)
REDACTED
	return 0, nil
REDACTED

func (m *opsRepoMock) BatchInsertErrorLogs(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
	if m.BatchInsertErrorLogsFn != nil {
		return m.BatchInsertErrorLogsFn(ctx, inputs)
REDACTED
	return int64(len(inputs)), nil
REDACTED

func (m *opsRepoMock) ListErrorLogs(ctx context.Context, filter *OpsErrorLogFilter) (*OpsErrorLogList, error) {
	return &OpsErrorLogList{Errors: []*OpsErrorLog{REDACTED, Page: 1, PageSize: 20REDACTED, nil
REDACTED

func (m *opsRepoMock) GetErrorLogByID(ctx context.Context, id int64) (*OpsErrorLogDetail, error) {
	return &OpsErrorLogDetail{REDACTED, nil
REDACTED

func (m *opsRepoMock) ListRequestDetails(ctx context.Context, filter *OpsRequestDetailFilter) ([]*OpsRequestDetail, int64, error) {
	return []*OpsRequestDetail{REDACTED, 0, nil
REDACTED

func (m *opsRepoMock) BatchInsertSystemLogs(ctx context.Context, inputs []*OpsInsertSystemLogInput) (int64, error) {
	if m.BatchInsertSystemLogsFn != nil {
		return m.BatchInsertSystemLogsFn(ctx, inputs)
REDACTED
	return int64(len(inputs)), nil
REDACTED

func (m *opsRepoMock) ListSystemLogs(ctx context.Context, filter *OpsSystemLogFilter) (*OpsSystemLogList, error) {
	if m.ListSystemLogsFn != nil {
		return m.ListSystemLogsFn(ctx, filter)
REDACTED
	return &OpsSystemLogList{Logs: []*OpsSystemLog{REDACTED, Total: 0, Page: 1, PageSize: 50REDACTED, nil
REDACTED

func (m *opsRepoMock) DeleteSystemLogs(ctx context.Context, filter *OpsSystemLogCleanupFilter) (int64, error) {
	if m.DeleteSystemLogsFn != nil {
		return m.DeleteSystemLogsFn(ctx, filter)
REDACTED
	return 0, nil
REDACTED

func (m *opsRepoMock) InsertSystemLogCleanupAudit(ctx context.Context, input *OpsSystemLogCleanupAudit) error {
	if m.InsertSystemLogCleanupAuditFn != nil {
		return m.InsertSystemLogCleanupAuditFn(ctx, input)
REDACTED
	return nil
REDACTED

func (m *opsRepoMock) UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64, resolvedAt *time.Time) error {
	return nil
REDACTED

func (m *opsRepoMock) GetWindowStats(ctx context.Context, filter *OpsDashboardFilter) (*OpsWindowStats, error) {
	return &OpsWindowStats{REDACTED, nil
REDACTED

func (m *opsRepoMock) GetRealtimeTrafficSummary(ctx context.Context, filter *OpsDashboardFilter) (*OpsRealtimeTrafficSummary, error) {
	return &OpsRealtimeTrafficSummary{REDACTED, nil
REDACTED

func (m *opsRepoMock) GetDashboardOverview(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
	return &OpsDashboardOverview{REDACTED, nil
REDACTED

func (m *opsRepoMock) GetThroughputTrend(ctx context.Context, filter *OpsDashboardFilter, bucketSeconds int) (*OpsThroughputTrendResponse, error) {
	return &OpsThroughputTrendResponse{REDACTED, nil
REDACTED

func (m *opsRepoMock) GetLatencyHistogram(ctx context.Context, filter *OpsDashboardFilter) (*OpsLatencyHistogramResponse, error) {
	return &OpsLatencyHistogramResponse{REDACTED, nil
REDACTED

func (m *opsRepoMock) GetErrorTrend(ctx context.Context, filter *OpsDashboardFilter, bucketSeconds int) (*OpsErrorTrendResponse, error) {
	return &OpsErrorTrendResponse{REDACTED, nil
REDACTED

func (m *opsRepoMock) GetErrorDistribution(ctx context.Context, filter *OpsDashboardFilter) (*OpsErrorDistributionResponse, error) {
	return &OpsErrorDistributionResponse{REDACTED, nil
REDACTED

func (m *opsRepoMock) GetOpenAITokenStats(ctx context.Context, filter *OpsOpenAITokenStatsFilter) (*OpsOpenAITokenStatsResponse, error) {
	return &OpsOpenAITokenStatsResponse{REDACTED, nil
REDACTED

func (m *opsRepoMock) InsertSystemMetrics(ctx context.Context, input *OpsInsertSystemMetricsInput) error {
	return nil
REDACTED

func (m *opsRepoMock) GetLatestSystemMetrics(ctx context.Context, windowMinutes int) (*OpsSystemMetricsSnapshot, error) {
	return &OpsSystemMetricsSnapshot{REDACTED, nil
REDACTED

func (m *opsRepoMock) UpsertJobHeartbeat(ctx context.Context, input *OpsUpsertJobHeartbeatInput) error {
	return nil
REDACTED

func (m *opsRepoMock) ListJobHeartbeats(ctx context.Context) ([]*OpsJobHeartbeat, error) {
	return []*OpsJobHeartbeat{REDACTED, nil
REDACTED

func (m *opsRepoMock) ListAlertRules(ctx context.Context) ([]*OpsAlertRule, error) {
	return []*OpsAlertRule{REDACTED, nil
REDACTED

func (m *opsRepoMock) CreateAlertRule(ctx context.Context, input *OpsAlertRule) (*OpsAlertRule, error) {
	return input, nil
REDACTED

func (m *opsRepoMock) UpdateAlertRule(ctx context.Context, input *OpsAlertRule) (*OpsAlertRule, error) {
	return input, nil
REDACTED

func (m *opsRepoMock) DeleteAlertRule(ctx context.Context, id int64) error {
	return nil
REDACTED

func (m *opsRepoMock) ListAlertEvents(ctx context.Context, filter *OpsAlertEventFilter) ([]*OpsAlertEvent, error) {
	return []*OpsAlertEvent{REDACTED, nil
REDACTED

func (m *opsRepoMock) GetAlertEventByID(ctx context.Context, eventID int64) (*OpsAlertEvent, error) {
	return &OpsAlertEvent{REDACTED, nil
REDACTED

func (m *opsRepoMock) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	return nil, nil
REDACTED

func (m *opsRepoMock) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	return nil, nil
REDACTED

func (m *opsRepoMock) CreateAlertEvent(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
	return event, nil
REDACTED

func (m *opsRepoMock) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error {
	return nil
REDACTED

func (m *opsRepoMock) UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error {
	return nil
REDACTED

func (m *opsRepoMock) CreateAlertSilence(ctx context.Context, input *OpsAlertSilence) (*OpsAlertSilence, error) {
	return input, nil
REDACTED

func (m *opsRepoMock) IsAlertSilenced(ctx context.Context, ruleID int64, platform string, groupID *int64, region *string, now time.Time) (bool, error) {
	return false, nil
REDACTED

func (m *opsRepoMock) UpsertHourlyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	return nil
REDACTED

func (m *opsRepoMock) UpsertDailyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	return nil
REDACTED

func (m *opsRepoMock) GetLatestHourlyBucketStart(ctx context.Context) (time.Time, bool, error) {
	return time.Time{REDACTED, false, nil
REDACTED

func (m *opsRepoMock) GetLatestDailyBucketDate(ctx context.Context) (time.Time, bool, error) {
	return time.Time{REDACTED, false, nil
REDACTED

func (m *opsRepoMock) LookupDeletedKeyAudit(ctx context.Context, key string) (*DeletedKeyAuditResult, error) {
	if m.LookupDeletedKeyAuditFn != nil {
		return m.LookupDeletedKeyAuditFn(ctx, key)
REDACTED
	return nil, nil
REDACTED

var _ OpsRepository = (*opsRepoMock)(nil)
