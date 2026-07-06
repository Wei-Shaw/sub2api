//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 目标：严格验证“antigravity 账号通过 /v1/messages 提供 Claude 服务时”，
// 当账号 credentials.intercept_warmup_requests=true 且请求为 Warmup 时，
// 后端会在转发上游前直接拦截并返回 mock 响应（不依赖上游）。

type fakeSchedulerCache struct {
	accounts []*service.Account
REDACTED

func (f *fakeSchedulerCache) GetSnapshot(_ context.Context, _ service.SchedulerBucket) ([]*service.Account, bool, error) {
	return f.accounts, true, nil
REDACTED
func (f *fakeSchedulerCache) SetSnapshot(_ context.Context, _ service.SchedulerBucket, _ []service.Account) error {
	return nil
REDACTED
func (f *fakeSchedulerCache) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range f.accounts {
		if account != nil && account.ID == id {
			return account, nil
	REDACTED
REDACTED
	return nil, nil
REDACTED
func (f *fakeSchedulerCache) SetAccount(_ context.Context, _ *service.Account) error { return nil REDACTED
func (f *fakeSchedulerCache) DeleteAccount(_ context.Context, _ int64) error         { return nil REDACTED
func (f *fakeSchedulerCache) UpdateLastUsed(_ context.Context, _ map[int64]time.Time) error {
	return nil
REDACTED
func (f *fakeSchedulerCache) TryLockBucket(_ context.Context, _ service.SchedulerBucket, _ time.Duration) (bool, error) {
	return true, nil
REDACTED
func (f *fakeSchedulerCache) UnlockBucket(_ context.Context, _ service.SchedulerBucket) error {
	return nil
REDACTED
func (f *fakeSchedulerCache) ListBuckets(_ context.Context) ([]service.SchedulerBucket, error) {
	return nil, nil
REDACTED
func (f *fakeSchedulerCache) GetOutboxWatermark(_ context.Context) (int64, error) { return 0, nil REDACTED
func (f *fakeSchedulerCache) SetOutboxWatermark(_ context.Context, _ int64) error { return nil REDACTED

type fakeGroupRepo struct {
	group *service.Group
REDACTED

func (f *fakeGroupRepo) Create(context.Context, *service.Group) error { return nil REDACTED
func (f *fakeGroupRepo) GetByID(context.Context, int64) (*service.Group, error) {
	return f.group, nil
REDACTED
func (f *fakeGroupRepo) GetByIDLite(context.Context, int64) (*service.Group, error) {
	return f.group, nil
REDACTED
func (f *fakeGroupRepo) Update(context.Context, *service.Group) error          { return nil REDACTED
func (f *fakeGroupRepo) Delete(context.Context, int64) error                   { return nil REDACTED
func (f *fakeGroupRepo) DeleteCascade(context.Context, int64) ([]int64, error) { return nil, nil REDACTED
func (f *fakeGroupRepo) List(context.Context, pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (f *fakeGroupRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (f *fakeGroupRepo) ListActive(context.Context) ([]service.Group, error) { return nil, nil REDACTED
func (f *fakeGroupRepo) ListActiveByPlatform(context.Context, string) ([]service.Group, error) {
	return nil, nil
REDACTED
func (f *fakeGroupRepo) ExistsByName(context.Context, string) (bool, error) { return false, nil REDACTED
func (f *fakeGroupRepo) GetAccountCount(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
REDACTED
func (f *fakeGroupRepo) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
REDACTED
func (f *fakeGroupRepo) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	return nil, nil
REDACTED
func (f *fakeGroupRepo) BindAccountsToGroup(context.Context, int64, []int64) error { return nil REDACTED
func (f *fakeGroupRepo) UpdateSortOrders(context.Context, []service.GroupSortOrderUpdate) error {
	return nil
REDACTED

type fakeConcurrencyCache struct{REDACTED

func (f *fakeConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
REDACTED
func (f *fakeConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error { return nil REDACTED
func (f *fakeConcurrencyCache) GetAccountConcurrency(context.Context, int64) (int, error) {
	return 0, nil
REDACTED
func (f *fakeConcurrencyCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
REDACTED
func (f *fakeConcurrencyCache) DecrementAccountWaitCount(context.Context, int64) error { return nil REDACTED
func (f *fakeConcurrencyCache) GetAccountWaitingCount(context.Context, int64) (int, error) {
	return 0, nil
REDACTED
func (f *fakeConcurrencyCache) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
REDACTED
func (f *fakeConcurrencyCache) ReleaseUserSlot(context.Context, int64, string) error   { return nil REDACTED
func (f *fakeConcurrencyCache) GetUserConcurrency(context.Context, int64) (int, error) { return 0, nil REDACTED
func (f *fakeConcurrencyCache) IncrementWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
REDACTED
func (f *fakeConcurrencyCache) DecrementWaitCount(context.Context, int64) error { return nil REDACTED
func (f *fakeConcurrencyCache) GetAccountsLoadBatch(context.Context, []service.AccountWithConcurrency) (map[int64]*service.AccountLoadInfo, error) {
	return map[int64]*service.AccountLoadInfo{REDACTED, nil
REDACTED
func (f *fakeConcurrencyCache) GetUsersLoadBatch(context.Context, []service.UserWithConcurrency) (map[int64]*service.UserLoadInfo, error) {
	return map[int64]*service.UserLoadInfo{REDACTED, nil
REDACTED
func (f *fakeConcurrencyCache) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = 0
REDACTED
	return result, nil
REDACTED
func (f *fakeConcurrencyCache) CleanupExpiredAccountSlots(context.Context, int64) error { return nil REDACTED
func (f *fakeConcurrencyCache) CleanupExpiredAccountSlotKeys(context.Context) error     { return nil REDACTED
func (f *fakeConcurrencyCache) CleanupStaleProcessSlots(context.Context, string) error  { return nil REDACTED

func newTestGatewayHandler(t *testing.T, group *service.Group, accounts []*service.Account) (*GatewayHandler, func()) {
REDACTED

	schedulerCache := &fakeSchedulerCache{accounts: accountsREDACTED
	schedulerSnapshot := service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil)

	gwSvc := service.NewGatewayService(
		nil, // accountRepo (not used: scheduler snapshot hit)
		&fakeGroupRepo{group: groupREDACTED,
		nil, // usageLogRepo
		nil, // usageBillingRepo
		nil, // userRepo
		nil, // userSubRepo
		nil, // userGroupRateRepo
		nil, // cache (disable sticky)
		nil, // cfg
		schedulerSnapshot,
		nil, // concurrencyService (disable load-aware; tryAcquire always acquired)
		nil, // billingService
		nil, // rateLimitService
		nil, // billingCacheService
		nil, // identityService
		nil, // httpUpstream
		nil, // deferredService
		nil, // claudeTokenProvider
		nil, // sessionLimitCache
		nil, // rpmCache
		nil, // digestStore
		nil, // settingService
		nil, // tlsFPProfileService
		nil, // channelService
		nil, // resolver
		nil, // balanceNotifyService
		nil, // userPlatformQuotaRepo
	)

	// RunModeSimple：跳过计费检查，避免引入 repo/cache 依赖。
	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)

	concurrencySvc := service.NewConcurrencyService(&fakeConcurrencyCache{REDACTED)
	concurrencyHelper := NewConcurrencyHelper(concurrencySvc, SSEPingFormatClaude, 0)

	h := &GatewayHandler{
		gatewayService:      gwSvc,
		billingCacheService: billingCacheSvc,
		concurrencyHelper:   concurrencyHelper,
		// 这些字段对本测试不敏感，保持较小即可
		maxAccountSwitches:       1,
		maxAccountSwitchesGemini: 1,
REDACTED

	cleanup := func() {
		billingCacheSvc.Stop()
REDACTED
	return h, cleanup
REDACTED

func TestGatewayHandlerMessages_InterceptWarmup_AntigravityAccount_MixedSchedulingV1(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(2001)
	accountID := int64(1001)

	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic, // /v1/messages（Claude兼容）入口
		Status:   service.StatusActive,
REDACTED

	account := &service.Account{
		ID:       accountID,
		Name:     "ag-1",
		Platform: service.PlatformAntigravity,
		Type:     service.AccountTypeOAuth,
REDACTED
			"access_token":              "tok_xxx",
			"intercept_warmup_requests": true,
	REDACTED,
		Extra: map[string]any{
			"mixed_scheduling": true, // 关键：允许被 anthropic 分组混合调度选中
	REDACTED,
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupIDREDACTEDREDACTED,
REDACTED

	h, cleanup := newTestGatewayHandler(t, group, []*service.Account{accountREDACTED)
	defer cleanup()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"max_tokens": 256,
		"messages": [{"role":"user","content":[{"type":"text","text":"Warmup"REDACTED]REDACTED]
REDACTED`)
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req

	apiKey := &service.APIKey{
		ID:      3001,
		UserID:  4001,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:          4001,
			Concurrency: 10,
			Balance:     100,
	REDACTED,
		Group: group,
REDACTED

	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10REDACTED)

	h.Messages(c)

	require.Equal(t, 200, rec.Code)

	// 断言：确实选中了 antigravity 账号（不是纯函数测试，而是从 Handler 里验证调度结果）
	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.Equal(t, accountID, selected)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "msg_mock_warmup", resp["id"])
	require.Equal(t, "claude-sonnet-4-5", resp["model"])

	content, ok := resp["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "New Conversation", first["text"])
REDACTED

func TestGatewayHandlerMessages_InterceptWarmup_AntigravityAccount_ForcePlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(2002)
	accountID := int64(1002)

	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAntigravity,
		Status:   service.StatusActive,
REDACTED

	account := &service.Account{
		ID:       accountID,
		Name:     "ag-2",
		Platform: service.PlatformAntigravity,
		Type:     service.AccountTypeOAuth,
REDACTED
			"access_token":              "tok_xxx",
			"intercept_warmup_requests": true,
	REDACTED,
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupIDREDACTEDREDACTED,
REDACTED

	h, cleanup := newTestGatewayHandler(t, group, []*service.Account{accountREDACTED)
	defer cleanup()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"max_tokens": 256,
		"messages": [{"role":"user","content":[{"type":"text","text":"Warmup"REDACTED]REDACTED]
REDACTED`)
	req := httptest.NewRequest("POST", "/antigravity/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// 模拟 routes/gateway.go 里的 ForcePlatform 中间件效果：
	// - 写入 request.Context（Service读取）
	// - 写入 gin.Context（Handler快速读取）
	ctx := context.WithValue(req.Context(), ctxkey.Group, group)
	ctx = context.WithValue(ctx, ctxkey.ForcePlatform, service.PlatformAntigravity)
	req = req.WithContext(ctx)
	c.Request = req
	c.Set(string(middleware.ContextKeyForcePlatform), service.PlatformAntigravity)

	apiKey := &service.APIKey{
		ID:      3002,
		UserID:  4002,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:          4002,
			Concurrency: 10,
			Balance:     100,
	REDACTED,
		Group: group,
REDACTED

	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10REDACTED)

	h.Messages(c)

	require.Equal(t, 200, rec.Code)

	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.Equal(t, accountID, selected)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "msg_mock_warmup", resp["id"])
	require.Equal(t, "claude-sonnet-4-5", resp["model"])
REDACTED
