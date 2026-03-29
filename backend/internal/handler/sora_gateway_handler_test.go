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
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 编译期接口断言
var _ service.SoraClient = (*stubSoraClient)(nil)
var _ service.AccountRepository = (*stubAccountRepo)(nil)
var _ service.GroupRepository = (*stubGroupRepo)(nil)
var _ service.UsageLogRepository = (*stubUsageLogRepo)(nil)

type stubSoraClient struct {
	imageURLs []string
}

func (s *stubSoraClient) Enabled() bool { return true }
func (s *stubSoraClient) UploadImage(ctx context.Context, account *service.Account, data []byte, filename string) (string, error) {
	return "upload", nil
}
func (s *stubSoraClient) CreateImageTask(ctx context.Context, account *service.Account, req service.SoraImageRequest) (string, error) {
	return "task-image", nil
}
func (s *stubSoraClient) CreateVideoTask(ctx context.Context, account *service.Account, req service.SoraVideoRequest) (string, error) {
	return "task-video", nil
}
func (s *stubSoraClient) CreateStoryboardTask(ctx context.Context, account *service.Account, req service.SoraStoryboardRequest) (string, error) {
	return "task-video", nil
}
func (s *stubSoraClient) UploadCharacterVideo(ctx context.Context, account *service.Account, data []byte) (string, error) {
	return "cameo-1", nil
}
func (s *stubSoraClient) GetCameoStatus(ctx context.Context, account *service.Account, cameoID string) (*service.SoraCameoStatus, error) {
	return &service.SoraCameoStatus{
		Status:          "finalized",
		StatusMessage:   "Completed",
		DisplayNameHint: "Character",
		UsernameHint:    "user.character",
		ProfileAssetURL: "https://example.com/avatar.webp",
	}, nil
}
func (s *stubSoraClient) DownloadCharacterImage(ctx context.Context, account *service.Account, imageURL string) ([]byte, error) {
	return []byte("avatar"), nil
}
func (s *stubSoraClient) UploadCharacterImage(ctx context.Context, account *service.Account, data []byte) (string, error) {
	return "asset-pointer", nil
}
func (s *stubSoraClient) FinalizeCharacter(ctx context.Context, account *service.Account, req service.SoraCharacterFinalizeRequest) (string, error) {
	return "character-1", nil
}
func (s *stubSoraClient) SetCharacterPublic(ctx context.Context, account *service.Account, cameoID string) error {
	return nil
}
func (s *stubSoraClient) DeleteCharacter(ctx context.Context, account *service.Account, characterID string) error {
	return nil
}
func (s *stubSoraClient) PostVideoForWatermarkFree(ctx context.Context, account *service.Account, generationID string) (string, error) {
	return "s_post", nil
}
func (s *stubSoraClient) DeletePost(ctx context.Context, account *service.Account, postID string) error {
	return nil
}
func (s *stubSoraClient) GetWatermarkFreeURLCustom(ctx context.Context, account *service.Account, parseURL, parseToken, postID string) (string, error) {
	return "https://example.com/no-watermark.mp4", nil
}
func (s *stubSoraClient) EnhancePrompt(ctx context.Context, account *service.Account, prompt, expansionLevel string, durationS int) (string, error) {
	return "enhanced prompt", nil
}
func (s *stubSoraClient) GetImageTask(ctx context.Context, account *service.Account, taskID string) (*service.SoraImageTaskStatus, error) {
	return &service.SoraImageTaskStatus{ID: taskID, Status: "completed", URLs: s.imageURLs}, nil
}
func (s *stubSoraClient) GetVideoTask(ctx context.Context, account *service.Account, taskID string) (*service.SoraVideoTaskStatus, error) {
	return &service.SoraVideoTaskStatus{ID: taskID, Status: "completed", URLs: s.imageURLs}, nil
}

type stubAccountRepo struct {
	accounts map[int64]*service.Account
}

func (r *stubAccountRepo) Create(ctx context.Context, account *service.Account) error { return nil }
func (r *stubAccountRepo) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	if acc, ok := r.accounts[id]; ok {
		return acc, nil
	}
	return nil, service.ErrAccountNotFound
}
func (r *stubAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	var result []*service.Account
	for _, id := range ids {
		if acc, ok := r.accounts[id]; ok {
			result = append(result, acc)
		}
	}
	return result, nil
}
func (r *stubAccountRepo) ExistsByID(ctx context.Context, id int64) (bool, error) {
	_, ok := r.accounts[id]
	return ok, nil
}
func (r *stubAccountRepo) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	return nil, nil
}
func (r *stubAccountRepo) FindByExtraField(ctx context.Context, key string, value any) ([]service.Account, error) {
	return nil, nil
}
func (r *stubAccountRepo) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}
func (r *stubAccountRepo) Update(ctx context.Context, account *service.Account) error { return nil }
func (r *stubAccountRepo) Delete(ctx context.Context, id int64) error                 { return nil }
func (r *stubAccountRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubAccountRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	return nil, nil
}
func (r *stubAccountRepo) ListActive(ctx context.Context) ([]service.Account, error) { return nil, nil }
func (r *stubAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.listSchedulableByPlatform(platform), nil
}
func (r *stubAccountRepo) UpdateLastUsed(ctx context.Context, id int64) error { return nil }
func (r *stubAccountRepo) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
}
func (r *stubAccountRepo) SetError(ctx context.Context, id int64, errorMsg string) error { return nil }
func (r *stubAccountRepo) ClearError(ctx context.Context, id int64) error                { return nil }
func (r *stubAccountRepo) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return nil
}
func (r *stubAccountRepo) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}
func (r *stubAccountRepo) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return nil
}
func (r *stubAccountRepo) ListSchedulable(ctx context.Context) ([]service.Account, error) {
	return r.listSchedulable(), nil
}
func (r *stubAccountRepo) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	return r.listSchedulable(), nil
}
func (r *stubAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.listSchedulableByPlatform(platform), nil
}
func (r *stubAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	return r.listSchedulableByPlatform(platform), nil
}
func (r *stubAccountRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	var result []service.Account
	for _, acc := range r.accounts {
		for _, platform := range platforms {
			if acc.Platform == platform && acc.IsSchedulable() {
				result = append(result, *acc)
				break
			}
		}
	}
	return result, nil
}
func (r *stubAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	return r.ListSchedulableByPlatforms(ctx, platforms)
}
func (r *stubAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}
func (r *stubAccountRepo) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	return r.ListSchedulableByPlatforms(ctx, platforms)
}
func (r *stubAccountRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return nil
}
func (r *stubAccountRepo) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time) error {
	return nil
}
func (r *stubAccountRepo) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return nil
}
func (r *stubAccountRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return nil
}
func (r *stubAccountRepo) ClearTempUnschedulable(ctx context.Context, id int64) error { return nil }
func (r *stubAccountRepo) ClearRateLimit(ctx context.Context, id int64) error         { return nil }
func (r *stubAccountRepo) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	return nil
}
func (r *stubAccountRepo) ClearModelRateLimits(ctx context.Context, id int64) error { return nil }
func (r *stubAccountRepo) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return nil
}
func (r *stubAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return nil
}
func (r *stubAccountRepo) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	return 0, nil
}

func (r *stubAccountRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return nil
}

func (r *stubAccountRepo) ResetQuotaUsed(ctx context.Context, id int64) error {
	return nil
}

func (r *stubAccountRepo) listSchedulable() []service.Account {
	var result []service.Account
	for _, acc := range r.accounts {
		if acc.IsSchedulable() {
			result = append(result, *acc)
		}
	}
	return result
}

func (r *stubAccountRepo) listSchedulableByPlatform(platform string) []service.Account {
	var result []service.Account
	for _, acc := range r.accounts {
		if acc.Platform == platform && acc.IsSchedulable() {
			result = append(result, *acc)
		}
	}
	return result
}

type stubGroupRepo struct {
	group *service.Group
}

func (r *stubGroupRepo) Create(ctx context.Context, group *service.Group) error { return nil }
func (r *stubGroupRepo) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	return r.group, nil
}
func (r *stubGroupRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.group, nil
}
func (r *stubGroupRepo) Update(ctx context.Context, group *service.Group) error { return nil }
func (r *stubGroupRepo) Delete(ctx context.Context, id int64) error             { return nil }
func (r *stubGroupRepo) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, nil
}
func (r *stubGroupRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubGroupRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubGroupRepo) ListActive(ctx context.Context) ([]service.Group, error) { return nil, nil }
func (r *stubGroupRepo) ListActiveByPlatform(ctx context.Context, platform string) ([]service.Group, error) {
	return nil, nil
}
func (r *stubGroupRepo) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
}
func (r *stubGroupRepo) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	return 0, 0, nil
}
func (r *stubGroupRepo) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}
func (r *stubGroupRepo) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, nil
}
func (r *stubGroupRepo) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return nil
}
func (r *stubGroupRepo) UpdateSortOrders(ctx context.Context, updates []service.GroupSortOrderUpdate) error {
	return nil
}

type stubUsageLogRepo struct{}

func (s *stubUsageLogRepo) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	return true, nil
}
func (s *stubUsageLogRepo) GetByID(ctx context.Context, id int64) (*service.UsageLog, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) Delete(ctx context.Context, id int64) error { return nil }
func (s *stubUsageLogRepo) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubUsageLogRepo) ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubUsageLogRepo) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubUsageLogRepo) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubUsageLogRepo) ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubUsageLogRepo) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubUsageLogRepo) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubUsageLogRepo) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.ModelStat, error) {
	return nil, nil
}

func (s *stubUsageLogRepo) GetEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.EndpointStat, error) {
	return []usagestats.EndpointStat{}, nil
}

func (s *stubUsageLogRepo) GetUpstreamEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.EndpointStat, error) {
	return []usagestats.EndpointStat{}, nil
}
func (s *stubUsageLogRepo) GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.GroupStat, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetUserBreakdownStats(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int) ([]usagestats.UserBreakdownItem, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetAllGroupUsageSummary(ctx context.Context, todayStart time.Time) ([]usagestats.GroupUsageSummary, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetUserSpendingRanking(ctx context.Context, startTime, endTime time.Time, limit int) (*usagestats.UserSpendingRankingResponse, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchUserUsageStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*usagestats.UserDashboardStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubUsageLogRepo) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (s *stubUsageLogRepo) GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) ([]map[string]any, error) {
	return nil, nil
}

func TestSoraGatewayHandler_ChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		RunMode: config.RunModeSimple,
		Gateway: config.GatewayConfig{
			SoraStreamMode:     "force",
			MaxAccountSwitches: 1,
			Scheduling: config.GatewaySchedulingConfig{
				LoadBatchEnabled: false,
			},
		},
		Concurrency: config.ConcurrencyConfig{PingInterval: 0},
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL:             "https://sora.test",
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
			},
		},
	}

	account := &service.Account{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 1}
	accountRepo := &stubAccountRepo{accounts: map[int64]*service.Account{account.ID: account}}
	group := &service.Group{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Hydrated: true}
	groupRepo := &stubGroupRepo{group: group}

	usageLogRepo := &stubUsageLogRepo{}
	deferredService := service.NewDeferredService(accountRepo, nil, 0)
	billingService := service.NewBillingService(cfg, nil)
	concurrencyService := service.NewConcurrencyService(testutil.StubConcurrencyCache{})
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, cfg)
	t.Cleanup(func() {
		billingCacheService.Stop()
	})

	gatewayService := service.NewGatewayService(
		accountRepo,
		groupRepo,
		usageLogRepo,
		nil,
		nil,
		nil,
		nil,
		testutil.StubGatewayCache{},
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
		testutil.StubSessionLimitCache{},
		nil, // rpmCache
		nil, // digestStore
		nil, // settingService
		nil, // tlsFPProfileService
		nil, // channelService
	)

	soraClient := &stubSoraClient{imageURLs: []string{"https://example.com/a.png"}}
	soraGatewayService := service.NewSoraGatewayService(soraClient, nil, nil, cfg)

	handler := NewSoraGatewayHandler(gatewayService, soraGatewayService, concurrencyService, billingCacheService, nil, cfg)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := `{"model":"gpt-image","messages":[{"role":"user","content":"hello"}]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/sora/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	apiKey := &service.APIKey{
		ID:      1,
		UserID:  1,
		Status:  service.StatusActive,
		GroupID: &group.ID,
		User:    &service.User{ID: 1, Concurrency: 1, Status: service.StatusActive},
		Group:   group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: apiKey.User.Concurrency})

	handler.ChatCompletions(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp["media_url"])
}

// TestSoraHandler_StreamForcing 验证 sora handler 的 stream 强制逻辑
func TestSoraHandler_StreamForcing(t *testing.T) {
	// 测试 1：stream=false 时 sjson 强制修改为 true
	body := []byte(`{"model":"sora","messages":[{"role":"user","content":"test"}],"stream":false}`)
	clientStream := gjson.GetBytes(body, "stream").Bool()
	require.False(t, clientStream)
	newBody, err := sjson.SetBytes(body, "stream", true)
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(newBody, "stream").Bool())

	// 测试 2：stream=true 时不修改
	body2 := []byte(`{"model":"sora","messages":[{"role":"user","content":"test"}],"stream":true}`)
	require.True(t, gjson.GetBytes(body2, "stream").Bool())

	// 测试 3：无 stream 字段时 gjson 返回 false（零值）
	body3 := []byte(`{"model":"sora","messages":[{"role":"user","content":"test"}]}`)
	require.False(t, gjson.GetBytes(body3, "stream").Bool())
}

// TestSoraHandler_ValidationExtraction 验证 sora handler 中 gjson 字段校验逻辑
func TestSoraHandler_ValidationExtraction(t *testing.T) {
	// model 缺失
	body := []byte(`{"messages":[{"role":"user","content":"test"}]}`)
	modelResult := gjson.GetBytes(body, "model")
	require.True(t, !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "")

	// model 为数字 → 类型不是 gjson.String，应被拒绝
	body1b := []byte(`{"model":123,"messages":[{"role":"user","content":"test"}]}`)
	modelResult1b := gjson.GetBytes(body1b, "model")
	require.True(t, modelResult1b.Exists())
	require.NotEqual(t, gjson.String, modelResult1b.Type)

	// messages 缺失
	body2 := []byte(`{"model":"sora"}`)
	require.False(t, gjson.GetBytes(body2, "messages").IsArray())

	// messages 不是 JSON 数组（字符串）
	body3 := []byte(`{"model":"sora","messages":"not array"}`)
	require.False(t, gjson.GetBytes(body3, "messages").IsArray())

	// messages 是对象而非数组 → IsArray 返回 false
	body4 := []byte(`{"model":"sora","messages":{}}`)
	require.False(t, gjson.GetBytes(body4, "messages").IsArray())

	// messages 是空数组 → IsArray 为 true 但 len==0，应被拒绝
	body5 := []byte(`{"model":"sora","messages":[]}`)
	msgsResult := gjson.GetBytes(body5, "messages")
	require.True(t, msgsResult.IsArray())
	require.Equal(t, 0, len(msgsResult.Array()))

	// 非法 JSON 被 gjson.ValidBytes 拦截
	require.False(t, gjson.ValidBytes([]byte(`{invalid`)))
}

// TestGenerateOpenAISessionHash_WithBody 验证 generateOpenAISessionHash 的 body/header 解析逻辑
func TestGenerateOpenAISessionHash_WithBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 从 body 提取 prompt_cache_key
	body := []byte(`{"model":"sora","prompt_cache_key":"session-abc"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)

	hash := generateOpenAISessionHash(c, body)
	require.NotEmpty(t, hash)

	// 无 prompt_cache_key 且无 header → 空 hash
	body2 := []byte(`{"model":"sora"}`)
	hash2 := generateOpenAISessionHash(c, body2)
	require.Empty(t, hash2)

	// header 优先于 body
	c.Request.Header.Set("session_id", "from-header")
	hash3 := generateOpenAISessionHash(c, body)
	require.NotEmpty(t, hash3)
	require.NotEqual(t, hash, hash3) // 不同来源应产生不同 hash
}

func TestSoraHandleStreamingAwareError_JSONEscaping(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		message string
	}{
		{
			name:    "包含双引号",
			errType: "upstream_error",
			message: `upstream returned "invalid" payload`,
		},
		{
			name:    "包含换行和制表符",
			errType: "rate_limit_error",
			message: "line1\nline2\ttab",
		},
		{
			name:    "包含反斜杠",
			errType: "upstream_error",
			message: `path C:\Users\test\file.txt not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			h := &SoraGatewayHandler{}
			h.handleStreamingAwareError(c, http.StatusBadGateway, tt.errType, tt.message, true)

			body := w.Body.String()
			require.True(t, strings.HasPrefix(body, "event: error\n"), "应以 SSE error 事件开头")
			require.True(t, strings.HasSuffix(body, "\n\n"), "应以 SSE 结束分隔符结尾")

			lines := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n")
			require.Len(t, lines, 2, "SSE 错误事件应包含 event 行和 data 行")
			require.Equal(t, "event: error", lines[0])
			require.True(t, strings.HasPrefix(lines[1], "data: "), "第二行应为 data 前缀")

			jsonStr := strings.TrimPrefix(lines[1], "data: ")
			var parsed map[string]any
			require.NoError(t, json.Unmarshal([]byte(jsonStr), &parsed), "data 行必须是合法 JSON")

			errorObj, ok := parsed["error"].(map[string]any)
			require.True(t, ok, "JSON 中应包含 error 对象")
			require.Equal(t, tt.errType, errorObj["type"])
			require.Equal(t, tt.message, errorObj["message"])
		})
	}
}

func TestSoraHandleFailoverExhausted_StreamPassesUpstreamMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &SoraGatewayHandler{}
	resp := []byte(`{"error":{"message":"invalid \"prompt\"\nline2","code":"bad_request"}}`)
	h.handleFailoverExhausted(c, http.StatusBadGateway, nil, resp, true)

	body := w.Body.String()
	require.True(t, strings.HasPrefix(body, "event: error\n"))
	require.True(t, strings.HasSuffix(body, "\n\n"))

	lines := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n")
	require.Len(t, lines, 2)
	jsonStr := strings.TrimPrefix(lines[1], "data: ")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &parsed))

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "upstream_error", errorObj["type"])
	require.Equal(t, "invalid \"prompt\"\nline2", errorObj["message"])
}

func TestSoraHandleFailoverExhausted_CloudflareChallengeIncludesRay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	headers := http.Header{}
	headers.Set("cf-ray", "9d01b0e9ecc35829-SEA")
	body := []byte(`<!DOCTYPE html><html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt={};</script></body></html>`)

	h := &SoraGatewayHandler{}
	h.handleFailoverExhausted(c, http.StatusForbidden, headers, body, true)

	lines := strings.Split(strings.TrimSuffix(w.Body.String(), "\n\n"), "\n")
	require.Len(t, lines, 2)
	jsonStr := strings.TrimPrefix(lines[1], "data: ")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &parsed))

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "upstream_error", errorObj["type"])
	msg, _ := errorObj["message"].(string)
	require.Contains(t, msg, "Cloudflare challenge")
	require.Contains(t, msg, "cf-ray: 9d01b0e9ecc35829-SEA")
}

func TestSoraHandleFailoverExhausted_CfShield429MappedToRateLimitError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	headers := http.Header{}
	headers.Set("cf-ray", "9d03b68c086027a1-SEA")
	body := []byte(`{"error":{"code":"cf_shield_429","message":"shield blocked"}}`)

	h := &SoraGatewayHandler{}
	h.handleFailoverExhausted(c, http.StatusTooManyRequests, headers, body, true)

	lines := strings.Split(strings.TrimSuffix(w.Body.String(), "\n\n"), "\n")
	require.Len(t, lines, 2)
	jsonStr := strings.TrimPrefix(lines[1], "data: ")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &parsed))

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "rate_limit_error", errorObj["type"])
	msg, _ := errorObj["message"].(string)
	require.Contains(t, msg, "Cloudflare shield")
	require.Contains(t, msg, "cf-ray: 9d03b68c086027a1-SEA")
}

func TestExtractSoraFailoverHeaderInsights(t *testing.T) {
	headers := http.Header{}
	headers.Set("cf-mitigated", "challenge")
	headers.Set("content-type", "text/html")
	body := []byte(`<script>window._cf_chl_opt={cRay: '9cff2d62d83bb98d'};</script>`)

	rayID, mitigated, contentType := extractSoraFailoverHeaderInsights(headers, body)
	require.Equal(t, "9cff2d62d83bb98d", rayID)
	require.Equal(t, "challenge", mitigated)
	require.Equal(t, "text/html", contentType)
}
