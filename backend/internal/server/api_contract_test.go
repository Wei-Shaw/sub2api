//go:build unit

package server_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		setup      func(t *testing.T, deps *contractDeps)
		method     string
		path       string
		body       string
		headers    map[string]string
		wantStatus int
		wantJSON   string
REDACTED{
		{
			name:       "GET /api/v1/auth/me",
			method:     http.MethodGet,
			path:       "/api/v1/auth/me",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"id": 1,
					"email": "alice@example.com",
					"username": "alice",
					"role": "user",
					"balance": 12.5,
					"concurrency": 5,
					"status": "active",
					"allowed_groups": null,
					"created_at": "2025-01-02T03:04:05Z",
					"updated_at": "2025-01-02T03:04:05Z",
					"run_mode": "standard"
			REDACTED
		REDACTED`,
	REDACTED,
		{
			name:   "POST /api/v1/keys",
			method: http.MethodPost,
			path:   "/api/v1/keys",
			body:   `{"name":"Key One","custom_key":"sk_custom_1234567890"REDACTED`,
			headers: map[string]string{
				"Content-Type": "application/json",
		REDACTED,
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"id": 100,
					"user_id": 1,
					"key": "sk_custom_1234567890",
					"name": "Key One",
					"group_id": null,
					"status": "active",
					"ip_whitelist": null,
					"ip_blacklist": null,
					"last_used_at": null,
					"quota": 0,
					"quota_used": 0,
					"expires_at": null,
					"created_at": "2025-01-02T03:04:05Z",
					"updated_at": "2025-01-02T03:04:05Z"
			REDACTED
		REDACTED`,
	REDACTED,
		{
			name: "GET /api/v1/keys (paginated)",
			setup: func(t *testing.T, deps *contractDeps) {
			REDACTED
				deps.apiKeyRepo.MustSeed(&service.APIKey{
					ID:        100,
					UserID:    1,
					Key:       "sk_custom_1234567890",
					Name:      "Key One",
					Status:    service.StatusActive,
					CreatedAt: deps.now,
					UpdatedAt: deps.now,
			REDACTED)
		REDACTED,
			method:     http.MethodGet,
			path:       "/api/v1/keys?page=1&page_size=10",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"items": [
						{
							"id": 100,
							"user_id": 1,
							"key": "sk_custom_1234567890",
							"name": "Key One",
							"group_id": null,
							"status": "active",
							"ip_whitelist": null,
							"ip_blacklist": null,
							"last_used_at": null,
							"quota": 0,
							"quota_used": 0,
							"expires_at": null,
							"created_at": "2025-01-02T03:04:05Z",
							"updated_at": "2025-01-02T03:04:05Z"
					REDACTED
					],
					"total": 1,
					"page": 1,
					"page_size": 10,
					"pages": 1
			REDACTED
		REDACTED`,
	REDACTED,
		{
			name: "GET /api/v1/groups/available",
			setup: func(t *testing.T, deps *contractDeps) {
			REDACTED
				// 普通用户可见的分组列表不应包含内部字段（如 model_routing/account_count）。
				deps.groupRepo.SetActive([]service.Group{
					{
						ID:                  10,
						Name:                "Group One",
						Description:         "desc",
						Platform:            service.PlatformAnthropic,
						RateMultiplier:      1.5,
						IsExclusive:         false,
						Status:              service.StatusActive,
						SubscriptionType:    service.SubscriptionTypeStandard,
						ModelRoutingEnabled: true,
						ModelRouting: map[string][]int64{
							"claude-3-*": []int64{101, 102REDACTED,
					REDACTED,
						AccountCount: 2,
						CreatedAt:    deps.now,
						UpdatedAt:    deps.now,
				REDACTED,
			REDACTED)
				deps.userSubRepo.SetActiveByUserID(1, nil)
		REDACTED,
			method:     http.MethodGet,
			path:       "/api/v1/groups/available",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": [
					{
						"id": 10,
						"name": "Group One",
						"description": "desc",
						"platform": "anthropic",
						"rate_multiplier": 1.5,
						"is_exclusive": false,
						"status": "active",
						"subscription_type": "standard",
						"daily_limit_usd": null,
						"weekly_limit_usd": null,
						"monthly_limit_usd": null,
						"image_price_1k": null,
						"image_price_2k": null,
						"image_price_4k": null,
							"sora_image_price_360": null,
							"sora_image_price_540": null,
							"sora_storage_quota_bytes": 0,
							"sora_video_price_per_request": null,
							"sora_video_price_per_request_hd": null,
							"claude_code_only": false,
						"fallback_group_id": null,
						"fallback_group_id_on_invalid_request": null,
						"created_at": "2025-01-02T03:04:05Z",
						"updated_at": "2025-01-02T03:04:05Z"
				REDACTED
				]
		REDACTED`,
	REDACTED,
		{
			name: "GET /api/v1/subscriptions",
			setup: func(t *testing.T, deps *contractDeps) {
			REDACTED
				// 普通用户订阅接口不应包含 assigned_* / notes 等管理员字段。
				deps.userSubRepo.SetByUserID(1, []service.UserSubscription{
					{
						ID:              501,
						UserID:          1,
						GroupID:         10,
						StartsAt:        deps.now,
						ExpiresAt:       time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC), // 使用未来日期避免 normalizeSubscriptionStatus 标记为过期
						Status:          service.SubscriptionStatusActive,
						DailyUsageUSD:   1.23,
						WeeklyUsageUSD:  2.34,
						MonthlyUsageUSD: 3.45,
						AssignedBy:      ptr(int64(999)),
						AssignedAt:      deps.now,
						Notes:           "admin-note",
						CreatedAt:       deps.now,
						UpdatedAt:       deps.now,
				REDACTED,
			REDACTED)
		REDACTED,
			method:     http.MethodGet,
			path:       "/api/v1/subscriptions",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": [
					{
						"id": 501,
						"user_id": 1,
						"group_id": 10,
						"starts_at": "2025-01-02T03:04:05Z",
						"expires_at": "2099-01-02T03:04:05Z",
						"status": "active",
						"daily_window_start": null,
						"weekly_window_start": null,
						"monthly_window_start": null,
						"daily_usage_usd": 1.23,
						"weekly_usage_usd": 2.34,
						"monthly_usage_usd": 3.45,
						"created_at": "2025-01-02T03:04:05Z",
						"updated_at": "2025-01-02T03:04:05Z"
				REDACTED
				]
		REDACTED`,
	REDACTED,
		{
			name: "GET /api/v1/redeem/history",
			setup: func(t *testing.T, deps *contractDeps) {
			REDACTED
				// 普通用户兑换历史不应包含 notes 等内部字段。
				deps.redeemRepo.SetByUser(1, []service.RedeemCode{
					{
						ID:        900,
						Code:      "CODE-123",
						Type:      service.RedeemTypeBalance,
						Value:     1.25,
						Status:    service.StatusUsed,
						UsedBy:    ptr(int64(1)),
						UsedAt:    ptr(deps.now),
						Notes:     "internal-note",
						CreatedAt: deps.now,
				REDACTED,
			REDACTED)
		REDACTED,
			method:     http.MethodGet,
			path:       "/api/v1/redeem/history",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": [
					{
						"id": 900,
						"code": "CODE-123",
						"type": "balance",
						"value": 1.25,
						"status": "used",
						"used_by": 1,
						"used_at": "2025-01-02T03:04:05Z",
						"created_at": "2025-01-02T03:04:05Z",
						"group_id": null,
						"validity_days": 0
				REDACTED
				]
		REDACTED`,
	REDACTED,
		{
			name: "GET /api/v1/usage/stats",
			setup: func(t *testing.T, deps *contractDeps) {
			REDACTED
				deps.usageRepo.SetUserLogs(1, []service.UsageLog{
					{
						ID:                  1,
						UserID:              1,
						APIKeyID:            100,
						AccountID:           200,
						Model:               "claude-3",
						InputTokens:         10,
						OutputTokens:        20,
						CacheCreationTokens: 1,
						CacheReadTokens:     2,
						TotalCost:           0.5,
						ActualCost:          0.5,
						DurationMs:          ptr(100),
						CreatedAt:           deps.now,
				REDACTED,
					{
						ID:           2,
						UserID:       1,
						APIKeyID:     100,
						AccountID:    200,
						Model:        "claude-3",
						InputTokens:  5,
						OutputTokens: 15,
						TotalCost:    0.25,
						ActualCost:   0.25,
						DurationMs:   ptr(300),
						CreatedAt:    deps.now,
				REDACTED,
			REDACTED)
		REDACTED,
			method:     http.MethodGet,
			path:       "/api/v1/usage/stats?start_date=2025-01-01&end_date=2025-01-02",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"total_requests": 2,
					"total_input_tokens": 15,
					"total_output_tokens": 35,
					"total_cache_tokens": 3,
					"total_tokens": 53,
					"total_cost": 0.75,
					"total_actual_cost": 0.75,
					"average_duration_ms": 200
			REDACTED
		REDACTED`,
	REDACTED,
		{
			name: "GET /api/v1/usage (paginated)",
			setup: func(t *testing.T, deps *contractDeps) {
			REDACTED
				deps.usageRepo.SetUserLogs(1, []service.UsageLog{
					{
						ID:                    1,
						UserID:                1,
						APIKeyID:              100,
						AccountID:             200,
						AccountRateMultiplier: ptr(0.5),
						RequestID:             "req_123",
						Model:                 "claude-3",
						InputTokens:           10,
						OutputTokens:          20,
						CacheCreationTokens:   1,
						CacheReadTokens:       2,
						TotalCost:             0.5,
						ActualCost:            0.5,
						RateMultiplier:        1,
						BillingType:           service.BillingTypeBalance,
						Stream:                true,
						DurationMs:            ptr(100),
						FirstTokenMs:          ptr(50),
						CreatedAt:             deps.now,
				REDACTED,
			REDACTED)
		REDACTED,
			method:     http.MethodGet,
			path:       "/api/v1/usage?page=1&page_size=10",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"items": [
						{
							"id": 1,
							"user_id": 1,
							"api_key_id": 100,
							"account_id": 200,
								"request_id": "req_123",
								"model": "claude-3",
								"request_type": "stream",
								"openai_ws_mode": false,
								"group_id": null,
								"subscription_id": null,
							"input_tokens": 10,
							"output_tokens": 20,
							"cache_creation_tokens": 1,
							"cache_read_tokens": 2,
							"cache_creation_5m_tokens": 0,
							"cache_creation_1h_tokens": 0,
							"input_cost": 0,
							"output_cost": 0,
							"cache_creation_cost": 0,
							"cache_read_cost": 0,
						"total_cost": 0.5,
						"actual_cost": 0.5,
						"rate_multiplier": 1,
						"billing_type": 0,
							"stream": true,
							"duration_ms": 100,
							"first_token_ms": 50,
							"image_count": 0,
							"image_size": null,
							"media_type": null,
							"cache_ttl_overridden": false,
							"created_at": "2025-01-02T03:04:05Z",
							"user_agent": null
					REDACTED
					],
					"total": 1,
					"page": 1,
					"page_size": 10,
					"pages": 1
			REDACTED
		REDACTED`,
	REDACTED,
		{
			name: "GET /api/v1/admin/settings",
			setup: func(t *testing.T, deps *contractDeps) {
			REDACTED
				deps.settingRepo.SetAll(map[string]string{
					service.SettingKeyRegistrationEnabled: "true",
					service.SettingKeyEmailVerifyEnabled:  "false",
					service.SettingKeyPromoCodeEnabled:    "true",

					service.SettingKeySMTPHost:     "smtp.example.com",
					service.SettingKeySMTPPort:     "587",
					service.SettingKeySMTPUsername: "user",
					service.SettingKeySMTPPassword: "secret",
					service.SettingKeySMTPFrom:     "no-reply@example.com",
					service.SettingKeySMTPFromName: "Sub2API",
					service.SettingKeySMTPUseTLS:   "true",

					service.SettingKeyTurnstileEnabled:   "true",
					service.SettingKeyTurnstileSiteKey:   "site-key",
					service.SettingKeyTurnstileSecretKey: "secret-key",

					service.SettingKeySiteName:     "Sub2API",
					service.SettingKeySiteLogo:     "",
					service.SettingKeySiteSubtitle: "Subtitle",
					service.SettingKeyAPIBaseURL:   "https://api.example.com",
					service.SettingKeyContactInfo:  "support",
					service.SettingKeyDocURL:       "https://docs.example.com",

					service.SettingKeyDefaultConcurrency: "5",
					service.SettingKeyDefaultBalance:     "1.25",

					service.SettingKeyOpsMonitoringEnabled:         "false",
					service.SettingKeyOpsRealtimeMonitoringEnabled: "true",
					service.SettingKeyOpsQueryModeDefault:          "auto",
					service.SettingKeyOpsMetricsIntervalSeconds:    "60",
			REDACTED)
		REDACTED,
			method:     http.MethodGet,
			path:       "/api/v1/admin/settings",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"registration_enabled": true,
					"email_verify_enabled": false,
					"promo_code_enabled": true,
					"password_reset_enabled": false,
					"totp_enabled": false,
					"totp_encryption_key_configured": false,
					"smtp_host": "smtp.example.com",
					"smtp_port": 587,
					"smtp_username": "user",
					"smtp_password_configured": true,
					"smtp_from_email": "no-reply@example.com",
					"smtp_from_name": "Sub2API",
					"smtp_use_tls": true,
					"turnstile_enabled": true,
					"turnstile_site_key": "site-key",
					"turnstile_secret_key_configured": true,
					"linuxdo_connect_enabled": false,
						"linuxdo_connect_client_id": "",
						"linuxdo_connect_client_secret_configured": false,
						"linuxdo_connect_redirect_url": "",
						"ops_monitoring_enabled": false,
						"ops_realtime_monitoring_enabled": true,
						"ops_query_mode_default": "auto",
						"ops_metrics_interval_seconds": 60,
						"site_name": "Sub2API",
						"site_logo": "",
						"site_subtitle": "Subtitle",
						"api_base_url": "https://api.example.com",
					"contact_info": "support",
					"doc_url": "https://docs.example.com",
					"default_concurrency": 5,
					"default_balance": 1.25,
					"enable_model_fallback": false,
					"fallback_model_anthropic": "claude-3-5-sonnet-20241022",
					"fallback_model_antigravity": "gemini-2.5-pro",
					"fallback_model_gemini": "gemini-2.5-pro",
						"fallback_model_openai": "gpt-4o",
						"enable_identity_patch": true,
						"identity_patch_prompt": "",
						"sora_client_enabled": false,
						"invitation_code_enabled": false,
						"home_content": "",
					"hide_ccs_import_button": false,
					"purchase_subscription_enabled": false,
					"purchase_subscription_url": ""
			REDACTED
		REDACTED`,
	REDACTED,
		{
			name:   "POST /api/v1/admin/accounts/bulk-update",
			method: http.MethodPost,
			path:   "/api/v1/admin/accounts/bulk-update",
			body:   `{"account_ids":[101,102],"schedulable":falseREDACTED`,
			headers: map[string]string{
				"Content-Type": "application/json",
		REDACTED,
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"success": 2,
					"failed": 0,
					"success_ids": [101, 102],
					"failed_ids": [],
					"results": [
						{"account_id": 101, "success": trueREDACTED,
						{"account_id": 102, "success": trueREDACTED
					]
			REDACTED
		REDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newContractDeps(t)
			if tt.setup != nil {
				tt.setup(t, deps)
		REDACTED

			status, body := doRequest(t, deps.router, tt.method, tt.path, tt.body, tt.headers)
			require.Equal(t, tt.wantStatus, status)
			require.JSONEq(t, tt.wantJSON, body)
	REDACTED)
REDACTED
REDACTED

type contractDeps struct {
	now         time.Time
	router      http.Handler
	apiKeyRepo  *stubApiKeyRepo
	groupRepo   *stubGroupRepo
	userSubRepo *stubUserSubscriptionRepo
	usageRepo   *stubUsageLogRepo
	settingRepo *stubSettingRepo
	redeemRepo  *stubRedeemCodeRepo
REDACTED

func newContractDeps(t *testing.T) *contractDeps {
REDACTED

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	userRepo := &stubUserRepo{
		users: map[int64]*service.User{
			1: {
				ID:            1,
				Email:         "alice@example.com",
				Username:      "alice",
				Notes:         "hello",
				Role:          service.RoleUser,
				Balance:       12.5,
				Concurrency:   5,
				Status:        service.StatusActive,
				AllowedGroups: nil,
				CreatedAt:     now,
				UpdatedAt:     now,
		REDACTED,
	REDACTED,
REDACTED

	apiKeyRepo := newStubApiKeyRepo(now)
	apiKeyCache := stubApiKeyCache{REDACTED
	groupRepo := &stubGroupRepo{REDACTED
	userSubRepo := &stubUserSubscriptionRepo{REDACTED
	accountRepo := stubAccountRepo{REDACTED
	proxyRepo := stubProxyRepo{REDACTED
	redeemRepo := &stubRedeemCodeRepo{REDACTED

	cfg := &config.Config{
		Default: config.DefaultConfig{
			APIKeyPrefix: "sk-",
	REDACTED,
		RunMode: config.RunModeStandard,
REDACTED

	userService := service.NewUserService(userRepo, nil, nil)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, userRepo, groupRepo, userSubRepo, nil, apiKeyCache, cfg)

	usageRepo := newStubUsageLogRepo()
	usageService := service.NewUsageService(usageRepo, userRepo, nil, nil)

	subscriptionService := service.NewSubscriptionService(groupRepo, userSubRepo, nil, nil, cfg)
	subscriptionHandler := handler.NewSubscriptionHandler(subscriptionService)

	redeemService := service.NewRedeemService(redeemRepo, userRepo, subscriptionService, nil, nil, nil, nil)
	redeemHandler := handler.NewRedeemHandler(redeemService)

	settingRepo := newStubSettingRepo()
	settingService := service.NewSettingService(settingRepo, cfg)

	adminService := service.NewAdminService(userRepo, groupRepo, &accountRepo, nil, proxyRepo, apiKeyRepo, redeemRepo, nil, nil, nil, nil, nil, nil)
	authHandler := handler.NewAuthHandler(cfg, nil, userService, settingService, nil, redeemService, nil)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyService)
	usageHandler := handler.NewUsageHandler(usageService, apiKeyService)
	adminSettingHandler := adminhandler.NewSettingHandler(settingService, nil, nil, nil, nil)
	adminAccountHandler := adminhandler.NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	jwtAuth := func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:      1,
			Concurrency: 5,
	REDACTED)
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
REDACTED
	adminAuth := func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:      1,
			Concurrency: 5,
	REDACTED)
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
REDACTED

	r := gin.New()

	v1 := r.Group("/api/v1")

	v1Auth := v1.Group("")
	v1Auth.Use(jwtAuth)
	v1Auth.GET("/auth/me", authHandler.GetCurrentUser)

	v1Keys := v1.Group("")
	v1Keys.Use(jwtAuth)
	v1Keys.GET("/keys", apiKeyHandler.List)
	v1Keys.POST("/keys", apiKeyHandler.Create)
	v1Keys.GET("/groups/available", apiKeyHandler.GetAvailableGroups)

	v1Usage := v1.Group("")
	v1Usage.Use(jwtAuth)
	v1Usage.GET("/usage", usageHandler.List)
	v1Usage.GET("/usage/stats", usageHandler.Stats)

	v1Subs := v1.Group("")
	v1Subs.Use(jwtAuth)
	v1Subs.GET("/subscriptions", subscriptionHandler.List)

	v1Redeem := v1.Group("")
	v1Redeem.Use(jwtAuth)
	v1Redeem.GET("/redeem/history", redeemHandler.GetHistory)

	v1Admin := v1.Group("/admin")
	v1Admin.Use(adminAuth)
	v1Admin.GET("/settings", adminSettingHandler.GetSettings)
	v1Admin.POST("/accounts/bulk-update", adminAccountHandler.BulkUpdate)

	return &contractDeps{
		now:         now,
		router:      r,
		apiKeyRepo:  apiKeyRepo,
		groupRepo:   groupRepo,
		userSubRepo: userSubRepo,
		usageRepo:   usageRepo,
		settingRepo: settingRepo,
		redeemRepo:  redeemRepo,
REDACTED
REDACTED

func doRequest(t *testing.T, router http.Handler, method, path, body string, headers map[string]string) (int, string) {
REDACTED

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for k, v := range headers {
		req.Header.Set(k, v)
REDACTED

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	respBody, err := io.ReadAll(w.Result().Body)
REDACTED

	return w.Result().StatusCode, string(respBody)
REDACTED

func ptr[T any](v T) *T { return &v REDACTED

type stubUserRepo struct {
	users map[int64]*service.User
REDACTED

func (r *stubUserRepo) Create(ctx context.Context, user *service.User) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, service.ErrUserNotFound
REDACTED
	clone := *user
	return &clone, nil
REDACTED

func (r *stubUserRepo) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	for _, user := range r.users {
		if user.Email == email {
			clone := *user
			return &clone, nil
	REDACTED
REDACTED
	return nil, service.ErrUserNotFound
REDACTED

func (r *stubUserRepo) GetFirstAdmin(ctx context.Context) (*service.User, error) {
	for _, user := range r.users {
		if user.Role == service.RoleAdmin && user.Status == service.StatusActive {
			clone := *user
			return &clone, nil
	REDACTED
REDACTED
	return nil, service.ErrUserNotFound
REDACTED

func (r *stubUserRepo) Update(ctx context.Context, user *service.User) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubUserRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserRepo) DeductBalance(ctx context.Context, id int64, amount float64) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserRepo) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return false, errors.New("not implemented")
REDACTED

func (r *stubUserRepo) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

func (r *stubUserRepo) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserRepo) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserRepo) EnableTotp(ctx context.Context, userID int64) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserRepo) DisableTotp(ctx context.Context, userID int64) error {
	return errors.New("not implemented")
REDACTED

type stubApiKeyCache struct{REDACTED

func (stubApiKeyCache) GetCreateAttemptCount(ctx context.Context, userID int64) (int, error) {
	return 0, nil
REDACTED

func (stubApiKeyCache) IncrementCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
REDACTED

func (stubApiKeyCache) DeleteCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
REDACTED

func (stubApiKeyCache) IncrementDailyUsage(ctx context.Context, apiKey string) error {
	return nil
REDACTED

func (stubApiKeyCache) SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error {
	return nil
REDACTED

func (stubApiKeyCache) GetAuthCache(ctx context.Context, key string) (*service.APIKeyAuthCacheEntry, error) {
	return nil, nil
REDACTED

func (stubApiKeyCache) SetAuthCache(ctx context.Context, key string, entry *service.APIKeyAuthCacheEntry, ttl time.Duration) error {
	return nil
REDACTED

func (stubApiKeyCache) DeleteAuthCache(ctx context.Context, key string) error {
	return nil
REDACTED

func (stubApiKeyCache) PublishAuthCacheInvalidation(ctx context.Context, cacheKey string) error {
	return nil
REDACTED

func (stubApiKeyCache) SubscribeAuthCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error {
	return nil
REDACTED

type stubGroupRepo struct {
	active []service.Group
REDACTED

func (r *stubGroupRepo) SetActive(groups []service.Group) {
	r.active = append([]service.Group(nil), groups...)
REDACTED

func (stubGroupRepo) Create(ctx context.Context, group *service.Group) error {
	return errors.New("not implemented")
REDACTED

func (stubGroupRepo) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	return nil, service.ErrGroupNotFound
REDACTED

func (stubGroupRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return nil, service.ErrGroupNotFound
REDACTED

func (stubGroupRepo) Update(ctx context.Context, group *service.Group) error {
	return errors.New("not implemented")
REDACTED

func (stubGroupRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (stubGroupRepo) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, errors.New("not implemented")
REDACTED

func (stubGroupRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (stubGroupRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubGroupRepo) ListActive(ctx context.Context) ([]service.Group, error) {
	return append([]service.Group(nil), r.active...), nil
REDACTED

func (r *stubGroupRepo) ListActiveByPlatform(ctx context.Context, platform string) ([]service.Group, error) {
	out := make([]service.Group, 0, len(r.active))
	for i := range r.active {
		g := r.active[i]
		if g.Platform == platform {
			out = append(out, g)
	REDACTED
REDACTED
	return out, nil
REDACTED

func (stubGroupRepo) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, errors.New("not implemented")
REDACTED

func (stubGroupRepo) GetAccountCount(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

func (stubGroupRepo) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

func (stubGroupRepo) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return errors.New("not implemented")
REDACTED

func (stubGroupRepo) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, errors.New("not implemented")
REDACTED

func (stubGroupRepo) UpdateSortOrders(ctx context.Context, updates []service.GroupSortOrderUpdate) error {
	return nil
REDACTED

type stubAccountRepo struct {
	bulkUpdateIDs []int64
REDACTED

func (s *stubAccountRepo) Create(ctx context.Context, account *service.Account) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	return nil, service.ErrAccountNotFound
REDACTED

func (s *stubAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ExistsByID(ctx context.Context, id int64) (bool, error) {
	return false, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) FindByExtraField(ctx context.Context, key string, value any) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) Update(ctx context.Context, account *service.Account) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListActive(ctx context.Context) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) UpdateLastUsed(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) SetError(ctx context.Context, id int64, errorMsg string) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ClearError(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListSchedulable(ctx context.Context) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ClearTempUnschedulable(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ClearRateLimit(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) ClearModelRateLimits(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return errors.New("not implemented")
REDACTED

func (s *stubAccountRepo) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	s.bulkUpdateIDs = append([]int64{REDACTED, ids...)
	return int64(len(ids)), nil
REDACTED

func (s *stubAccountRepo) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	return nil, errors.New("not implemented")
REDACTED

type stubProxyRepo struct{REDACTED

func (stubProxyRepo) Create(ctx context.Context, proxy *service.Proxy) error {
	return errors.New("not implemented")
REDACTED

func (stubProxyRepo) GetByID(ctx context.Context, id int64) (*service.Proxy, error) {
	return nil, service.ErrProxyNotFound
REDACTED

func (stubProxyRepo) ListByIDs(ctx context.Context, ids []int64) ([]service.Proxy, error) {
	return nil, errors.New("not implemented")
REDACTED

func (stubProxyRepo) Update(ctx context.Context, proxy *service.Proxy) error {
	return errors.New("not implemented")
REDACTED

func (stubProxyRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (stubProxyRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Proxy, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (stubProxyRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.Proxy, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (stubProxyRepo) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (stubProxyRepo) ListActive(ctx context.Context) ([]service.Proxy, error) {
	return nil, errors.New("not implemented")
REDACTED

func (stubProxyRepo) ListActiveWithAccountCount(ctx context.Context) ([]service.ProxyWithAccountCount, error) {
	return nil, errors.New("not implemented")
REDACTED

func (stubProxyRepo) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return false, errors.New("not implemented")
REDACTED

func (stubProxyRepo) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

func (stubProxyRepo) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]service.ProxyAccountSummary, error) {
	return nil, errors.New("not implemented")
REDACTED

type stubRedeemCodeRepo struct {
	byUser map[int64][]service.RedeemCode
REDACTED

func (r *stubRedeemCodeRepo) SetByUser(userID int64, codes []service.RedeemCode) {
	if r.byUser == nil {
		r.byUser = make(map[int64][]service.RedeemCode)
REDACTED
	r.byUser[userID] = append([]service.RedeemCode(nil), codes...)
REDACTED

func (stubRedeemCodeRepo) Create(ctx context.Context, code *service.RedeemCode) error {
	return errors.New("not implemented")
REDACTED

func (stubRedeemCodeRepo) CreateBatch(ctx context.Context, codes []service.RedeemCode) error {
	return errors.New("not implemented")
REDACTED

func (stubRedeemCodeRepo) GetByID(ctx context.Context, id int64) (*service.RedeemCode, error) {
	return nil, service.ErrRedeemCodeNotFound
REDACTED

func (stubRedeemCodeRepo) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	return nil, service.ErrRedeemCodeNotFound
REDACTED

func (stubRedeemCodeRepo) Update(ctx context.Context, code *service.RedeemCode) error {
	return errors.New("not implemented")
REDACTED

func (stubRedeemCodeRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (stubRedeemCodeRepo) Use(ctx context.Context, id, userID int64) error {
	return errors.New("not implemented")
REDACTED

func (stubRedeemCodeRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (stubRedeemCodeRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubRedeemCodeRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	if r.byUser == nil {
		return nil, nil
REDACTED
	codes := r.byUser[userID]
	if limit > 0 && len(codes) > limit {
		codes = codes[:limit]
REDACTED
	return append([]service.RedeemCode(nil), codes...), nil
REDACTED

func (stubRedeemCodeRepo) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (stubRedeemCodeRepo) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	return 0, errors.New("not implemented")
REDACTED

type stubUserSubscriptionRepo struct {
	byUser       map[int64][]service.UserSubscription
	activeByUser map[int64][]service.UserSubscription
REDACTED

func (r *stubUserSubscriptionRepo) SetByUserID(userID int64, subs []service.UserSubscription) {
	if r.byUser == nil {
		r.byUser = make(map[int64][]service.UserSubscription)
REDACTED
	r.byUser[userID] = append([]service.UserSubscription(nil), subs...)
REDACTED

func (r *stubUserSubscriptionRepo) SetActiveByUserID(userID int64, subs []service.UserSubscription) {
	if r.activeByUser == nil {
		r.activeByUser = make(map[int64][]service.UserSubscription)
REDACTED
	r.activeByUser[userID] = append([]service.UserSubscription(nil), subs...)
REDACTED

func (stubUserSubscriptionRepo) Create(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) Update(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED
func (r *stubUserSubscriptionRepo) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	if r.byUser == nil {
		return nil, nil
REDACTED
	return append([]service.UserSubscription(nil), r.byUser[userID]...), nil
REDACTED
func (r *stubUserSubscriptionRepo) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	if r.activeByUser == nil {
		return nil, nil
REDACTED
	return append([]service.UserSubscription(nil), r.activeByUser[userID]...), nil
REDACTED
func (stubUserSubscriptionRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return false, errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) ActivateWindows(ctx context.Context, id int64, start time.Time) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) ResetDailyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) ResetWeeklyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) ResetMonthlyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	return errors.New("not implemented")
REDACTED
func (stubUserSubscriptionRepo) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

type stubApiKeyRepo struct {
	now time.Time

	nextID int64
	byID   map[int64]*service.APIKey
	byKey  map[string]*service.APIKey
REDACTED

func newStubApiKeyRepo(now time.Time) *stubApiKeyRepo {
	return &stubApiKeyRepo{
		now:    now,
		nextID: 100,
		byID:   make(map[int64]*service.APIKey),
		byKey:  make(map[string]*service.APIKey),
REDACTED
REDACTED

func (r *stubApiKeyRepo) MustSeed(key *service.APIKey) {
	if key == nil {
		return
REDACTED
	clone := *key
	r.byID[clone.ID] = &clone
	r.byKey[clone.Key] = &clone
REDACTED

func (r *stubApiKeyRepo) Create(ctx context.Context, key *service.APIKey) error {
	if key == nil {
		return errors.New("nil key")
REDACTED
	if key.ID == 0 {
		key.ID = r.nextID
		r.nextID++
REDACTED
	if key.CreatedAt.IsZero() {
		key.CreatedAt = r.now
REDACTED
	if key.UpdatedAt.IsZero() {
		key.UpdatedAt = r.now
REDACTED
	clone := *key
	r.byID[clone.ID] = &clone
	r.byKey[clone.Key] = &clone
	return nil
REDACTED

func (r *stubApiKeyRepo) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	key, ok := r.byID[id]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
REDACTED
	clone := *key
	return &clone, nil
REDACTED

func (r *stubApiKeyRepo) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	key, ok := r.byID[id]
	if !ok {
		return "", 0, service.ErrAPIKeyNotFound
REDACTED
	return key.Key, key.UserID, nil
REDACTED

func (r *stubApiKeyRepo) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	found, ok := r.byKey[key]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
REDACTED
	clone := *found
	return &clone, nil
REDACTED

func (r *stubApiKeyRepo) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	return r.GetByKey(ctx, key)
REDACTED

func (r *stubApiKeyRepo) Update(ctx context.Context, key *service.APIKey) error {
	if key == nil {
		return errors.New("nil key")
REDACTED
	if _, ok := r.byID[key.ID]; !ok {
		return service.ErrAPIKeyNotFound
REDACTED
	if key.UpdatedAt.IsZero() {
		key.UpdatedAt = r.now
REDACTED
	clone := *key
	r.byID[clone.ID] = &clone
	r.byKey[clone.Key] = &clone
	return nil
REDACTED

func (r *stubApiKeyRepo) Delete(ctx context.Context, id int64) error {
	key, ok := r.byID[id]
	if !ok {
		return service.ErrAPIKeyNotFound
REDACTED
	delete(r.byID, id)
	delete(r.byKey, key.Key)
	return nil
REDACTED

func (r *stubApiKeyRepo) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	ids := make([]int64, 0, len(r.byID))
	for id := range r.byID {
		if r.byID[id].UserID == userID {
			ids = append(ids, id)
	REDACTED
REDACTED
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] REDACTED)

	start := params.Offset()
	if start > len(ids) {
		start = len(ids)
REDACTED
	end := start + params.Limit()
	if end > len(ids) {
		end = len(ids)
REDACTED

	out := make([]service.APIKey, 0, end-start)
	for _, id := range ids[start:end] {
		clone := *r.byID[id]
		out = append(out, clone)
REDACTED

	total := int64(len(ids))
	pageSize := params.Limit()
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		pages = 1
REDACTED
	return out, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: pageSize,
		Pages:    pages,
REDACTED, nil
REDACTED

func (r *stubApiKeyRepo) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	if len(apiKeyIDs) == 0 {
		return []int64{REDACTED, nil
REDACTED
	seen := make(map[int64]struct{REDACTED, len(apiKeyIDs))
	out := make([]int64, 0, len(apiKeyIDs))
	for _, id := range apiKeyIDs {
		if _, ok := seen[id]; ok {
			continue
	REDACTED
		seen[id] = struct{REDACTED{REDACTED
		key, ok := r.byID[id]
		if ok && key.UserID == userID {
			out = append(out, id)
	REDACTED
REDACTED
	return out, nil
REDACTED

func (r *stubApiKeyRepo) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	var count int64
	for _, key := range r.byID {
		if key.UserID == userID {
			count++
	REDACTED
REDACTED
	return count, nil
REDACTED

func (r *stubApiKeyRepo) ExistsByKey(ctx context.Context, key string) (bool, error) {
	_, ok := r.byKey[key]
	return ok, nil
REDACTED

func (r *stubApiKeyRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error) {
	return 0, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	key, ok := r.byID[id]
	if !ok {
		return service.ErrAPIKeyNotFound
REDACTED
	ts := usedAt
	key.LastUsedAt = &ts
	key.UpdatedAt = usedAt
	clone := *key
	r.byID[id] = &clone
	r.byKey[clone.Key] = &clone
	return nil
REDACTED

type stubUsageLogRepo struct {
	userLogs map[int64][]service.UsageLog
REDACTED

func newStubUsageLogRepo() *stubUsageLogRepo {
	return &stubUsageLogRepo{userLogs: make(map[int64][]service.UsageLog)REDACTED
REDACTED

func (r *stubUsageLogRepo) SetUserLogs(userID int64, logs []service.UsageLog) {
	r.userLogs[userID] = logs
REDACTED

func (r *stubUsageLogRepo) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	return false, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetByID(ctx context.Context, id int64) (*service.UsageLog, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	logs := r.userLogs[userID]
	total := int64(len(logs))
	out := paginateLogs(logs, params)
	return out, paginationResult(total, params), nil
REDACTED

func (r *stubUsageLogRepo) ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	logs := r.userLogs[userID]
	return logs, paginationResult(int64(len(logs)), pagination.PaginationParams{Page: 1, PageSize: 100REDACTED), nil
REDACTED

func (r *stubUsageLogRepo) ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.ModelStat, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	logs := r.userLogs[userID]
	if len(logs) == 0 {
		return &usagestats.UsageStats{REDACTED, nil
REDACTED

	var totalRequests int64
	var totalInputTokens int64
	var totalOutputTokens int64
	var totalCacheTokens int64
	var totalCost float64
	var totalActualCost float64
	var totalDuration int64
	var durationCount int64

	for _, log := range logs {
		totalRequests++
		totalInputTokens += int64(log.InputTokens)
		totalOutputTokens += int64(log.OutputTokens)
		totalCacheTokens += int64(log.CacheCreationTokens + log.CacheReadTokens)
		totalCost += log.TotalCost
		totalActualCost += log.ActualCost
		if log.DurationMs != nil {
			totalDuration += int64(*log.DurationMs)
			durationCount++
	REDACTED
REDACTED

	var avgDuration float64
	if durationCount > 0 {
		avgDuration = float64(totalDuration) / float64(durationCount)
REDACTED

	return &usagestats.UsageStats{
		TotalRequests:     totalRequests,
		TotalInputTokens:  totalInputTokens,
		TotalOutputTokens: totalOutputTokens,
		TotalCacheTokens:  totalCacheTokens,
		TotalTokens:       totalInputTokens + totalOutputTokens + totalCacheTokens,
		TotalCost:         totalCost,
		TotalActualCost:   totalActualCost,
		AverageDurationMs: avgDuration,
REDACTED, nil
REDACTED

func (r *stubUsageLogRepo) GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) ([]map[string]any, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchUserUsageStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*usagestats.UserDashboardStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	logs := r.userLogs[filters.UserID]

	// Apply filters
	var filtered []service.UsageLog
	for _, log := range logs {
		// Apply APIKeyID filter
		if filters.APIKeyID > 0 && log.APIKeyID != filters.APIKeyID {
			continue
	REDACTED
		// Apply Model filter
		if filters.Model != "" && log.Model != filters.Model {
			continue
	REDACTED
		// Apply Stream filter
		if filters.Stream != nil && log.Stream != *filters.Stream {
			continue
	REDACTED
		// Apply BillingType filter
		if filters.BillingType != nil && log.BillingType != *filters.BillingType {
			continue
	REDACTED
		// Apply time range filters
		if filters.StartTime != nil && log.CreatedAt.Before(*filters.StartTime) {
			continue
	REDACTED
		if filters.EndTime != nil && log.CreatedAt.After(*filters.EndTime) {
			continue
	REDACTED
		filtered = append(filtered, log)
REDACTED

	total := int64(len(filtered))
	out := paginateLogs(filtered, params)
	return out, paginationResult(total, params), nil
REDACTED

func (r *stubUsageLogRepo) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUsageLogRepo) GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
REDACTED

type stubSettingRepo struct {
	all map[string]string
REDACTED

func newStubSettingRepo() *stubSettingRepo {
	return &stubSettingRepo{all: make(map[string]string)REDACTED
REDACTED

func (r *stubSettingRepo) SetAll(values map[string]string) {
	r.all = make(map[string]string, len(values))
	for k, v := range values {
		r.all[k] = v
REDACTED
REDACTED

func (r *stubSettingRepo) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, ok := r.all[key]
	if !ok {
		return nil, service.ErrSettingNotFound
REDACTED
	return &service.Setting{Key: key, Value: valueREDACTED, nil
REDACTED

func (r *stubSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := r.all[key]
	if !ok {
		return "", service.ErrSettingNotFound
REDACTED
	return value, nil
REDACTED

func (r *stubSettingRepo) Set(ctx context.Context, key, value string) error {
	r.all[key] = value
	return nil
REDACTED

func (r *stubSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r.all[key]
REDACTED
	return out, nil
REDACTED

func (r *stubSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.all[k] = v
REDACTED
	return nil
REDACTED

func (r *stubSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.all))
	for k, v := range r.all {
		out[k] = v
REDACTED
	return out, nil
REDACTED

func (r *stubSettingRepo) Delete(ctx context.Context, key string) error {
	delete(r.all, key)
	return nil
REDACTED

func paginateLogs(logs []service.UsageLog, params pagination.PaginationParams) []service.UsageLog {
	start := params.Offset()
	if start > len(logs) {
		start = len(logs)
REDACTED
	end := start + params.Limit()
	if end > len(logs) {
		end = len(logs)
REDACTED
	out := make([]service.UsageLog, 0, end-start)
	out = append(out, logs[start:end]...)
	return out
REDACTED

func paginationResult(total int64, params pagination.PaginationParams) *pagination.PaginationResult {
	pageSize := params.Limit()
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		pages = 1
REDACTED
	return &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: pageSize,
		Pages:    pages,
REDACTED
REDACTED

// Ensure compile-time interface compliance.
var (
	_ service.UserRepository             = (*stubUserRepo)(nil)
	_ service.APIKeyRepository           = (*stubApiKeyRepo)(nil)
	_ service.APIKeyCache                = (*stubApiKeyCache)(nil)
	_ service.GroupRepository            = (*stubGroupRepo)(nil)
	_ service.UserSubscriptionRepository = (*stubUserSubscriptionRepo)(nil)
	_ service.UsageLogRepository         = (*stubUsageLogRepo)(nil)
	_ service.SettingRepository          = (*stubSettingRepo)(nil)
)
