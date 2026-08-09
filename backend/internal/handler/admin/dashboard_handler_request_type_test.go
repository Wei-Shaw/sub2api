package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardUsageRepoCapture struct {
	service.UsageLogRepository
	trendRequestType *int16
	trendStream      *bool
	modelRequestType *int16
	modelStream      *bool
	trendMismatch    *bool
	modelMismatch    *bool
	groupMismatch    *bool
	rankingLimit     int
	ranking          []usagestats.UserSpendingRankingItem
	rankingTotal     float64
REDACTED

func (s *dashboardUsageRepoCapture) GetUsageTrendWithUsageFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	filters usagestats.UsageLogFilters,
) ([]usagestats.TrendDataPoint, error) {
	s.trendRequestType = filters.RequestType
	s.trendStream = filters.Stream
	s.trendMismatch = filters.UpstreamModelMismatch
	return []usagestats.TrendDataPoint{REDACTED, nil
REDACTED

func (s *dashboardUsageRepoCapture) GetUsageTrendWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.TrendDataPoint, error) {
	s.trendRequestType = requestType
	s.trendStream = stream
	return []usagestats.TrendDataPoint{REDACTED, nil
REDACTED

func (s *dashboardUsageRepoCapture) GetModelStatsWithUsageFiltersBySource(
	ctx context.Context,
	startTime, endTime time.Time,
	filters usagestats.UsageLogFilters,
	source string,
) ([]usagestats.ModelStat, error) {
	s.modelRequestType = filters.RequestType
	s.modelStream = filters.Stream
	s.modelMismatch = filters.UpstreamModelMismatch
	return []usagestats.ModelStat{REDACTED, nil
REDACTED

func (s *dashboardUsageRepoCapture) GetGroupStatsWithUsageFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	filters usagestats.UsageLogFilters,
) ([]usagestats.GroupStat, error) {
	s.groupMismatch = filters.UpstreamModelMismatch
	return []usagestats.GroupStat{REDACTED, nil
REDACTED

func (s *dashboardUsageRepoCapture) GetModelStatsWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.ModelStat, error) {
	s.modelRequestType = requestType
	s.modelStream = stream
	return []usagestats.ModelStat{REDACTED, nil
REDACTED

func (s *dashboardUsageRepoCapture) GetUserSpendingRanking(
	ctx context.Context,
	startTime, endTime time.Time,
	limit int,
) (*usagestats.UserSpendingRankingResponse, error) {
	s.rankingLimit = limit
	return &usagestats.UserSpendingRankingResponse{
		Ranking:         s.ranking,
		TotalActualCost: s.rankingTotal,
		TotalRequests:   44,
		TotalTokens:     1234,
REDACTED, nil
REDACTED

func newDashboardRequestTypeTestRouter(repo *dashboardUsageRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/trend", handler.GetUsageTrend)
	router.GET("/admin/dashboard/models", handler.GetModelStats)
	router.GET("/admin/dashboard/groups", handler.GetGroupStats)
	router.GET("/admin/dashboard/users-ranking", handler.GetUserSpendingRanking)
	return router
REDACTED

func TestDashboardTrendRequestTypePriority(t *testing.T) {
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?request_type=ws_v2&stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.trendRequestType)
	require.Equal(t, int16(service.RequestTypeWSV2), *repo.trendRequestType)
	require.Nil(t, repo.trendStream)
REDACTED

func TestDashboardTrendInvalidRequestType(t *testing.T) {
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?request_type=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
REDACTED

func TestDashboardTrendInvalidStream(t *testing.T) {
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
REDACTED

func TestDashboardModelStatsRequestTypePriority(t *testing.T) {
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?request_type=sync&stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.modelRequestType)
	require.Equal(t, int16(service.RequestTypeSync), *repo.modelRequestType)
	require.Nil(t, repo.modelStream)
REDACTED

func TestDashboardModelStatsInvalidRequestType(t *testing.T) {
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?request_type=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
REDACTED

func TestDashboardModelStatsInvalidStream(t *testing.T) {
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
REDACTED

func TestDashboardModelStatsInvalidModelSource(t *testing.T) {
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?model_source=invalid", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
REDACTED

func TestDashboardModelStatsValidModelSource(t *testing.T) {
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?model_source=upstream", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
REDACTED

func TestDashboardModelAuditFilterPropagatesToTrendModelAndGroupQueries(t *testing.T) {
	resetDashboardReadCachesForTest()
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	for _, path := range []string{
		"/admin/dashboard/trend?upstream_model_mismatch=true",
		"/admin/dashboard/models?upstream_model_mismatch=true",
		"/admin/dashboard/groups?upstream_model_mismatch=true",
REDACTED {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, path)
REDACTED

	require.NotNil(t, repo.trendMismatch)
	require.True(t, *repo.trendMismatch)
	require.NotNil(t, repo.modelMismatch)
	require.True(t, *repo.modelMismatch)
	require.NotNil(t, repo.groupMismatch)
	require.True(t, *repo.groupMismatch)
REDACTED

func TestDashboardModelAuditFilterRejectsInvalidBoolean(t *testing.T) {
	repo := &dashboardUsageRepoCapture{REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	for _, path := range []string{
		"/admin/dashboard/trend?upstream_model_mismatch=invalid",
		"/admin/dashboard/models?upstream_model_mismatch=invalid",
		"/admin/dashboard/groups?upstream_model_mismatch=invalid",
REDACTED {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code, path)
REDACTED
REDACTED

func TestDashboardUsersRankingLimitAndCache(t *testing.T) {
	dashboardUsersRankingCache = newSnapshotCache(5 * time.Minute)
	repo := &dashboardUsageRepoCapture{
		ranking: []usagestats.UserSpendingRankingItem{
			{UserID: 7, Email: "rank@example.com", ActualCost: 10.5, Requests: 3, Tokens: 300REDACTED,
	REDACTED,
		rankingTotal: 88.8,
REDACTED
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-ranking?limit=100&start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 50, repo.rankingLimit)
	require.Contains(t, rec.Body.String(), "\"total_actual_cost\":88.8")
	require.Contains(t, rec.Body.String(), "\"total_requests\":44")
	require.Contains(t, rec.Body.String(), "\"total_tokens\":1234")
	require.Equal(t, "miss", rec.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-ranking?limit=100&start_date=2025-01-01&end_date=2025-01-02", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
REDACTED
