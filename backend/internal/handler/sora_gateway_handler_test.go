//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 编译期接口断言
var _ service.SoraClient = (*stubSoraClient)(nil)
var _ service.AccountRepository = (*stubAccountRepo)(nil)
var _ service.GroupRepository = (*stubGroupRepo)(nil)
var _ service.UsageLogRepository = (*stubUsageLogRepo)(nil)

type stubSoraClient struct {
	imageURLs []string
REDACTED

func (s *stubSoraClient) Enabled() bool { return true REDACTED
func (s *stubSoraClient) UploadImage(ctx context.Context, account *service.Account, data []byte, filename string) (string, error) {
	return "upload", nil
REDACTED
func (s *stubSoraClient) CreateImageTask(ctx context.Context, account *service.Account, req service.SoraImageRequest) (string, error) {
	return "task-image", nil
REDACTED
func (s *stubSoraClient) CreateVideoTask(ctx context.Context, account *service.Account, req service.SoraVideoRequest) (string, error) {
	return "task-video", nil
REDACTED
func (s *stubSoraClient) GetImageTask(ctx context.Context, account *service.Account, taskID string) (*service.SoraImageTaskStatus, error) {
	return &service.SoraImageTaskStatus{ID: taskID, Status: "completed", URLs: s.imageURLsREDACTED, nil
REDACTED
func (s *stubSoraClient) GetVideoTask(ctx context.Context, account *service.Account, taskID string) (*service.SoraVideoTaskStatus, error) {
	return &service.SoraVideoTaskStatus{ID: taskID, Status: "completed", URLs: s.imageURLsREDACTED, nil
REDACTED

type stubAccountRepo struct {
	accounts map[int64]*service.Account
REDACTED

func (r *stubAccountRepo) Create(ctx context.Context, account *service.Account) error { return nil REDACTED
func (r *stubAccountRepo) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	if acc, ok := r.accounts[id]; ok {
		return acc, nil
REDACTED
	return nil, service.ErrAccountNotFound
REDACTED
func (r *stubAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	var result []*service.Account
	for _, id := range ids {
		if acc, ok := r.accounts[id]; ok {
			result = append(result, acc)
	REDACTED
REDACTED
	return result, nil
REDACTED
func (r *stubAccountRepo) ExistsByID(ctx context.Context, id int64) (bool, error) {
	_, ok := r.accounts[id]
	return ok, nil
REDACTED
func (r *stubAccountRepo) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	return nil, nil
REDACTED
func (r *stubAccountRepo) FindByExtraField(ctx context.Context, key string, value any) ([]service.Account, error) {
	return nil, nil
REDACTED
func (r *stubAccountRepo) Update(ctx context.Context, account *service.Account) error { return nil REDACTED
func (r *stubAccountRepo) Delete(ctx context.Context, id int64) error                 { return nil REDACTED
func (r *stubAccountRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (r *stubAccountRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (r *stubAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	return nil, nil
REDACTED
func (r *stubAccountRepo) ListActive(ctx context.Context) ([]service.Account, error) { return nil, nil REDACTED
func (r *stubAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.listSchedulableByPlatform(platform), nil
REDACTED
func (r *stubAccountRepo) UpdateLastUsed(ctx context.Context, id int64) error { return nil REDACTED
func (r *stubAccountRepo) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
REDACTED
func (r *stubAccountRepo) SetError(ctx context.Context, id int64, errorMsg string) error { return nil REDACTED
func (r *stubAccountRepo) ClearError(ctx context.Context, id int64) error                { return nil REDACTED
func (r *stubAccountRepo) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return nil
REDACTED
func (r *stubAccountRepo) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
REDACTED
func (r *stubAccountRepo) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return nil
REDACTED
func (r *stubAccountRepo) ListSchedulable(ctx context.Context) ([]service.Account, error) {
	return r.listSchedulable(), nil
REDACTED
func (r *stubAccountRepo) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	return r.listSchedulable(), nil
REDACTED
func (r *stubAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.listSchedulableByPlatform(platform), nil
REDACTED
func (r *stubAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	return r.listSchedulableByPlatform(platform), nil
REDACTED
func (r *stubAccountRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	var result []service.Account
	for _, acc := range r.accounts {
		for _, platform := range platforms {
			if acc.Platform == platform && acc.IsSchedulable() {
				result = append(result, *acc)
				break
		REDACTED
	REDACTED
REDACTED
	return result, nil
REDACTED
func (r *stubAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	return r.ListSchedulableByPlatforms(ctx, platforms)
REDACTED
func (r *stubAccountRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return nil
REDACTED
func (r *stubAccountRepo) SetAntigravityQuotaScopeLimit(ctx context.Context, id int64, scope service.AntigravityQuotaScope, resetAt time.Time) error {
	return nil
REDACTED
func (r *stubAccountRepo) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time) error {
	return nil
REDACTED
func (r *stubAccountRepo) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return nil
REDACTED
func (r *stubAccountRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return nil
REDACTED
func (r *stubAccountRepo) ClearTempUnschedulable(ctx context.Context, id int64) error { return nil REDACTED
func (r *stubAccountRepo) ClearRateLimit(ctx context.Context, id int64) error         { return nil REDACTED
func (r *stubAccountRepo) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	return nil
REDACTED
func (r *stubAccountRepo) ClearModelRateLimits(ctx context.Context, id int64) error { return nil REDACTED
func (r *stubAccountRepo) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return nil
REDACTED
func (r *stubAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return nil
REDACTED
func (r *stubAccountRepo) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	return 0, nil
REDACTED

func (r *stubAccountRepo) listSchedulable() []service.Account {
	var result []service.Account
	for _, acc := range r.accounts {
		if acc.IsSchedulable() {
			result = append(result, *acc)
	REDACTED
REDACTED
	return result
REDACTED

func (r *stubAccountRepo) listSchedulableByPlatform(platform string) []service.Account {
	var result []service.Account
	for _, acc := range r.accounts {
		if acc.Platform == platform && acc.IsSchedulable() {
			result = append(result, *acc)
	REDACTED
REDACTED
	return result
REDACTED

type stubGroupRepo struct {
	group *service.Group
REDACTED

func (r *stubGroupRepo) Create(ctx context.Context, group *service.Group) error { return nil REDACTED
func (r *stubGroupRepo) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	return r.group, nil
REDACTED
func (r *stubGroupRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.group, nil
REDACTED
func (r *stubGroupRepo) Update(ctx context.Context, group *service.Group) error { return nil REDACTED
func (r *stubGroupRepo) Delete(ctx context.Context, id int64) error             { return nil REDACTED
func (r *stubGroupRepo) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, nil
REDACTED
func (r *stubGroupRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (r *stubGroupRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (r *stubGroupRepo) ListActive(ctx context.Context) ([]service.Group, error) { return nil, nil REDACTED
func (r *stubGroupRepo) ListActiveByPlatform(ctx context.Context, platform string) ([]service.Group, error) {
	return nil, nil
REDACTED
func (r *stubGroupRepo) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
REDACTED
func (r *stubGroupRepo) GetAccountCount(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
REDACTED
func (r *stubGroupRepo) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
REDACTED
func (r *stubGroupRepo) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, nil
REDACTED
func (r *stubGroupRepo) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return nil
REDACTED

type stubUsageLogRepo struct{REDACTED

func (s *stubUsageLogRepo) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	return true, nil
REDACTED
func (s *stubUsageLogRepo) GetByID(ctx context.Context, id int64) (*service.UsageLog, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) Delete(ctx context.Context, id int64) error { return nil REDACTED
func (s *stubUsageLogRepo) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (s *stubUsageLogRepo) ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (s *stubUsageLogRepo) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (s *stubUsageLogRepo) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (s *stubUsageLogRepo) ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (s *stubUsageLogRepo) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (s *stubUsageLogRepo) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (s *stubUsageLogRepo) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, stream *bool, billingType *int8) ([]usagestats.ModelStat, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchUserUsageStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*usagestats.UserDashboardStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (s *stubUsageLogRepo) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
REDACTED
func (s *stubUsageLogRepo) GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) ([]map[string]any, error) {
	return nil, nil
REDACTED

func TestSoraGatewayHandler_ChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		RunMode: config.RunModeSimple,
		Gateway: config.GatewayConfig{
			SoraStreamMode:     "force",
			MaxAccountSwitches: 1,
			Scheduling: config.GatewaySchedulingConfig{
				LoadBatchEnabled: false,
		REDACTED,
	REDACTED,
		Concurrency: config.ConcurrencyConfig{PingInterval: 0REDACTED,
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL:             "https://sora.test",
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
		REDACTED,
	REDACTED,
REDACTED

	account := &service.Account{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED
	accountRepo := &stubAccountRepo{accounts: map[int64]*service.Account{account.ID: accountREDACTEDREDACTED
	group := &service.Group{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Hydrated: trueREDACTED
	groupRepo := &stubGroupRepo{group: groupREDACTED

	usageLogRepo := &stubUsageLogRepo{REDACTED
	deferredService := service.NewDeferredService(accountRepo, nil, 0)
	billingService := service.NewBillingService(cfg, nil)
	concurrencyService := service.NewConcurrencyService(testutil.StubConcurrencyCache{REDACTED)
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, cfg)
	t.Cleanup(func() {
		billingCacheService.Stop()
REDACTED)

	gatewayService := service.NewGatewayService(
		accountRepo,
		groupRepo,
		usageLogRepo,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		concurrencyService,
		billingService,
		nil,
		billingCacheService,
		nil,
		nil,
		deferredService,
		nil,
		nil,
	)

	soraClient := &stubSoraClient{imageURLs: []string{"https://example.com/a.png"REDACTEDREDACTED
	soraGatewayService := service.NewSoraGatewayService(soraClient, nil, nil, cfg)

	handler := NewSoraGatewayHandler(gatewayService, soraGatewayService, concurrencyService, billingCacheService, cfg)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := `{"model":"gpt-image","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`
	c.Request = httptest.NewRequest(http.MethodPost, "/sora/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	apiKey := &service.APIKey{
		ID:      1,
		UserID:  1,
		Status:  service.StatusActive,
		GroupID: &group.ID,
		User:    &service.User{ID: 1, Concurrency: 1, Status: service.StatusActiveREDACTED,
		Group:   group,
REDACTED
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: apiKey.User.ConcurrencyREDACTED)

	handler.ChatCompletions(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp["media_url"])
REDACTED
