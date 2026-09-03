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

type contractDeps struct {
	now         time.Time
	router      http.Handler
	cfg         *config.Config
	apiKeyRepo  *stubApiKeyRepo
	groupRepo   *stubGroupRepo
	userSubRepo *stubUserSubscriptionRepo
	usageRepo   *stubUsageLogRepo
	settingRepo *stubSettingRepo
	redeemRepo  *stubRedeemCodeRepo
}

func newContractDeps(t *testing.T) *contractDeps {
	t.Helper()

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
			},
		},
	}

	apiKeyRepo := newStubApiKeyRepo(now)
	apiKeyCache := stubApiKeyCache{}
	groupRepo := &stubGroupRepo{}
	userSubRepo := &stubUserSubscriptionRepo{}
	accountRepo := stubAccountRepo{}
	proxyRepo := stubProxyRepo{}
	redeemRepo := &stubRedeemCodeRepo{}

	cfg := &config.Config{
		Default: config.DefaultConfig{
			APIKeyPrefix: "sk-",
		},
		RunMode: config.RunModeStandard,
	}

	userService := service.NewUserService(userRepo, nil, nil, nil)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, userRepo, groupRepo, userSubRepo, nil, apiKeyCache, cfg)

	usageRepo := newStubUsageLogRepo()
	usageService := service.NewUsageService(usageRepo, userRepo, nil, nil)

	subscriptionService := service.NewSubscriptionService(groupRepo, userSubRepo, nil, nil, cfg)
	subscriptionHandler := handler.NewSubscriptionHandler(subscriptionService)

	redeemService := service.NewRedeemService(redeemRepo, userRepo, subscriptionService, nil, nil, nil, nil, nil)
	redeemHandler := handler.NewRedeemHandler(redeemService)

	settingRepo := newStubSettingRepo()
	settingService := service.NewSettingService(settingRepo, cfg)

	adminService := service.NewAdminService(userRepo, groupRepo, &accountRepo, proxyRepo, apiKeyRepo, redeemRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	authHandler := handler.NewAuthHandler(cfg, nil, userService, settingService, nil, redeemService, nil, nil)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyService)
	usageHandler := handler.NewUsageHandler(usageService, apiKeyService, nil, nil)
	adminSettingHandler := adminhandler.NewSettingHandler(settingService, nil, nil, nil, nil, nil, nil)
	adminAccountHandler := adminhandler.NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	jwtAuth := func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:      1,
			Concurrency: 5,
		})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	}
	adminAuth := func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:      1,
			Concurrency: 5,
		})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	}

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
		cfg:         cfg,
		apiKeyRepo:  apiKeyRepo,
		groupRepo:   groupRepo,
		userSubRepo: userSubRepo,
		usageRepo:   usageRepo,
		settingRepo: settingRepo,
		redeemRepo:  redeemRepo,
	}
}

func doRequest(t *testing.T, router http.Handler, method, path, body string, headers map[string]string) (int, string) {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	respBody, err := io.ReadAll(w.Result().Body)
	require.NoError(t, err)

	return w.Result().StatusCode, string(respBody)
}

func ptr[T any](v T) *T { return &v }

type stubUserRepo struct {
	users map[int64]*service.User
}

func (r *stubUserRepo) Create(ctx context.Context, user *service.User) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) CreateWithEmailAliasGuard(ctx context.Context, user *service.User) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, service.ErrUserNotFound
	}
	clone := *user
	return &clone, nil
}

func (r *stubUserRepo) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	for _, user := range r.users {
		if user.Email == email {
			clone := *user
			return &clone, nil
		}
	}
	return nil, service.ErrUserNotFound
}

func (r *stubUserRepo) GetFirstAdmin(ctx context.Context) (*service.User, error) {
	for _, user := range r.users {
		if user.Role == service.RoleAdmin && user.Status == service.StatusActive {
			clone := *user
			return &clone, nil
		}
	}
	return nil, service.ErrUserNotFound
}

func (r *stubUserRepo) Update(ctx context.Context, user *service.User, fields service.UserUpdateFields) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) GetUserAvatar(ctx context.Context, userID int64) (*service.UserAvatar, error) {
	return nil, nil
}

func (r *stubUserRepo) UpsertUserAvatar(ctx context.Context, userID int64, input service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUserRepo) DeleteUserAvatar(ctx context.Context, userID int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUserRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) DeductBalance(ctx context.Context, id int64, amount float64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) AdjustBalance(ctx context.Context, id int64, delta float64) (service.BalanceChange, error) {
	return service.BalanceChange{}, errors.New("not implemented")
}

func (r *stubUserRepo) SetBalance(ctx context.Context, id int64, value float64) (service.BalanceChange, error) {
	return service.BalanceChange{}, errors.New("not implemented")
}

func (r *stubUserRepo) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) BatchSetConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (r *stubUserRepo) BatchAddConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (r *stubUserRepo) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	return 0, nil
}

func (r *stubUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *stubUserRepo) ExistsByEmailAlias(ctx context.Context, email string) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *stubUserRepo) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubUserRepo) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) ListUserAuthIdentities(ctx context.Context, userID int64) ([]service.UserAuthIdentityRecord, error) {
	return nil, nil
}

func (r *stubUserRepo) UnbindUserAuthProvider(context.Context, int64, string) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}

func (r *stubUserRepo) GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error) {
	return nil, nil
}

func (r *stubUserRepo) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	return nil
}

func (r *stubUserRepo) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) EnableTotp(ctx context.Context, userID int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) DisableTotp(ctx context.Context, userID int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.User, error) {
	panic("unexpected GetByIDIncludeDeleted call")
}

type stubApiKeyCache struct{}

func (stubApiKeyCache) GetCreateAttemptCount(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

func (stubApiKeyCache) IncrementCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
}

func (stubApiKeyCache) DeleteCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
}

func (stubApiKeyCache) IncrementDailyUsage(ctx context.Context, apiKey string) error {
	return nil
}

func (stubApiKeyCache) SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error {
	return nil
}

func (stubApiKeyCache) GetAuthCache(ctx context.Context, key string) (*service.APIKeyAuthCacheEntry, error) {
	return nil, nil
}

func (stubApiKeyCache) SetAuthCache(ctx context.Context, key string, entry *service.APIKeyAuthCacheEntry, ttl time.Duration) error {
	return nil
}

func (stubApiKeyCache) DeleteAuthCache(ctx context.Context, key string) error {
	return nil
}

func (stubApiKeyCache) PublishAuthCacheInvalidation(ctx context.Context, cacheKey string) error {
	return nil
}

func (stubApiKeyCache) SubscribeAuthCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error {
	return nil
}

type stubGroupRepo struct {
	active []service.Group
}

func (r *stubGroupRepo) SetActive(groups []service.Group) {
	r.active = append([]service.Group(nil), groups...)
}

func (stubGroupRepo) Create(ctx context.Context, group *service.Group) error {
	return errors.New("not implemented")
}

func (stubGroupRepo) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	return nil, service.ErrGroupNotFound
}

func (stubGroupRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return nil, service.ErrGroupNotFound
}

func (stubGroupRepo) Update(ctx context.Context, group *service.Group) error {
	return errors.New("not implemented")
}

func (stubGroupRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (stubGroupRepo) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, errors.New("not implemented")
}

func (stubGroupRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubGroupRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubGroupRepo) ListActive(ctx context.Context) ([]service.Group, error) {
	return append([]service.Group(nil), r.active...), nil
}

func (r *stubGroupRepo) ListActiveByPlatform(ctx context.Context, platform string) ([]service.Group, error) {
	out := make([]service.Group, 0, len(r.active))
	for i := range r.active {
		g := r.active[i]
		if g.Platform == platform {
			out = append(out, g)
		}
	}
	return out, nil
}

func (stubGroupRepo) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, errors.New("not implemented")
}

func (stubGroupRepo) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	return 0, 0, errors.New("not implemented")
}

func (stubGroupRepo) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (stubGroupRepo) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return errors.New("not implemented")
}

func (stubGroupRepo) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, errors.New("not implemented")
}

func (stubGroupRepo) UpdateSortOrders(ctx context.Context, updates []service.GroupSortOrderUpdate) error {
	return nil
}

func (stubGroupRepo) FindByDuplicateOperationID(ctx context.Context, operationID string) (*service.Group, error) {
	return nil, nil
}

func (stubGroupRepo) CreateFromSource(ctx context.Context, group *service.Group, sourceGroupID int64) error {
	return errors.New("not implemented")
}

type stubAccountRepo struct {
	bulkUpdateIDs []int64
}

func (s *stubAccountRepo) Create(ctx context.Context, account *service.Account) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) CreateWithAccountGroups(ctx context.Context, account *service.Account, groups []service.AccountGroup) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	return nil, service.ErrAccountNotFound
}

func (s *stubAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ExistsByID(ctx context.Context, id int64) (bool, error) {
	return false, errors.New("not implemented")
}

func (s *stubAccountRepo) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) FindByExtraField(ctx context.Context, key string, value any) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) Update(ctx context.Context, account *service.Account) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) UpdateWithAccountBillingSettings(
	ctx context.Context,
	account *service.Account,
	probeEnabled *bool,
	rateSyncEnabled *bool,
	rateMultiplier *float64,
) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]service.Account, error) {
	return nil, nil
}

func (s *stubAccountRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListActive(ctx context.Context) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListOAuthRefreshCandidates(ctx context.Context) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) UpdateLastUsed(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetError(ctx context.Context, id int64, errorMsg string) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearError(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	return 0, errors.New("not implemented")
}

func (s *stubAccountRepo) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ListShadowsByParent(ctx context.Context, parentID int64) ([]*service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulable(ctx context.Context) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListModelAvailabilityCandidates(ctx context.Context, groupID *int64, platforms []string, includeGrouped bool) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearTempUnschedulable(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearRateLimit(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearModelRateLimits(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ResetQuotaUsedAndClearRateLimitCooldown(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	s.bulkUpdateIDs = append([]int64{}, ids...)
	return int64(len(ids)), nil
}

func (s *stubAccountRepo) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) RevertProxyFallback(ctx context.Context, accountID int64) error {
	return nil
}

type stubProxyRepo struct{}

func (stubProxyRepo) Create(ctx context.Context, proxy *service.Proxy) error {
	return errors.New("not implemented")
}

func (stubProxyRepo) GetByID(ctx context.Context, id int64) (*service.Proxy, error) {
	return nil, service.ErrProxyNotFound
}

func (stubProxyRepo) ListByIDs(ctx context.Context, ids []int64) ([]service.Proxy, error) {
	return nil, errors.New("not implemented")
}

func (stubProxyRepo) Update(ctx context.Context, proxy *service.Proxy) error {
	return errors.New("not implemented")
}

func (stubProxyRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (stubProxyRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Proxy, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubProxyRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.Proxy, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubProxyRepo) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubProxyRepo) ListActive(ctx context.Context) ([]service.Proxy, error) {
	return nil, errors.New("not implemented")
}

func (stubProxyRepo) ListActiveWithAccountCount(ctx context.Context) ([]service.ProxyWithAccountCount, error) {
	return nil, errors.New("not implemented")
}

func (stubProxyRepo) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return false, errors.New("not implemented")
}

func (stubProxyRepo) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (stubProxyRepo) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]service.ProxyAccountSummary, error) {
	return nil, errors.New("not implemented")
}

func (stubProxyRepo) SweepExpiredProxies(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}

func (stubProxyRepo) ListAllForFallback(ctx context.Context) ([]service.Proxy, error) {
	return nil, nil
}

func (stubProxyRepo) CountExpired(ctx context.Context) (int64, error) {
	return 0, nil
}

func (stubProxyRepo) CountExpiringSoon(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}

type stubRedeemCodeRepo struct {
	byUser map[int64][]service.RedeemCode
}

func (r *stubRedeemCodeRepo) SetByUser(userID int64, codes []service.RedeemCode) {
	if r.byUser == nil {
		r.byUser = make(map[int64][]service.RedeemCode)
	}
	r.byUser[userID] = append([]service.RedeemCode(nil), codes...)
}

func (stubRedeemCodeRepo) Create(ctx context.Context, code *service.RedeemCode) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) CreateBatch(ctx context.Context, codes []service.RedeemCode) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) GetByID(ctx context.Context, id int64) (*service.RedeemCode, error) {
	return nil, service.ErrRedeemCodeNotFound
}

func (stubRedeemCodeRepo) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	return nil, service.ErrRedeemCodeNotFound
}

func (stubRedeemCodeRepo) Update(ctx context.Context, code *service.RedeemCode) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) BatchUpdate(ctx context.Context, ids []int64, fields service.RedeemCodeBatchUpdateFields) (int64, error) {
	return int64(len(ids)), nil
}

func (stubRedeemCodeRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) Use(ctx context.Context, id, userID int64) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubRedeemCodeRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubRedeemCodeRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	if r.byUser == nil {
		return nil, nil
	}
	codes := r.byUser[userID]
	if limit > 0 && len(codes) > limit {
		codes = codes[:limit]
	}
	return append([]service.RedeemCode(nil), codes...), nil
}

func (stubRedeemCodeRepo) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubRedeemCodeRepo) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	return 0, errors.New("not implemented")
}

type stubUserSubscriptionRepo struct {
	byUser       map[int64][]service.UserSubscription
	activeByUser map[int64][]service.UserSubscription
}

func (r *stubUserSubscriptionRepo) SetByUserID(userID int64, subs []service.UserSubscription) {
	if r.byUser == nil {
		r.byUser = make(map[int64][]service.UserSubscription)
	}
	r.byUser[userID] = append([]service.UserSubscription(nil), subs...)
}

func (r *stubUserSubscriptionRepo) SetActiveByUserID(userID int64, subs []service.UserSubscription) {
	if r.activeByUser == nil {
		r.activeByUser = make(map[int64][]service.UserSubscription)
	}
	r.activeByUser[userID] = append([]service.UserSubscription(nil), subs...)
}

func (stubUserSubscriptionRepo) Create(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) GetByIDForUpdate(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) Update(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) Restore(ctx context.Context, subscriptionID int64, restoredStatus string) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (r *stubUserSubscriptionRepo) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	if r.byUser == nil {
		return nil, nil
	}
	return append([]service.UserSubscription(nil), r.byUser[userID]...), nil
}
func (r *stubUserSubscriptionRepo) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	if r.activeByUser == nil {
		return nil, nil
	}
	return append([]service.UserSubscription(nil), r.activeByUser[userID]...), nil
}
func (stubUserSubscriptionRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return false, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ExistsActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return false, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ActivateWindows(ctx context.Context, id int64, dailyStart, periodicStart time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ResetUsageWindows(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	return 0, errors.New("not implemented")
}

type stubApiKeyRepo struct {
	now time.Time

	nextID int64
	byID   map[int64]*service.APIKey
	byKey  map[string]*service.APIKey
}

func newStubApiKeyRepo(now time.Time) *stubApiKeyRepo {
	return &stubApiKeyRepo{
		now:    now,
		nextID: 100,
		byID:   make(map[int64]*service.APIKey),
		byKey:  make(map[string]*service.APIKey),
	}
}

func (r *stubApiKeyRepo) MustSeed(key *service.APIKey) {
	if key == nil {
		return
	}
	clone := *key
	r.byID[clone.ID] = &clone
	r.byKey[clone.Key] = &clone
}

func (r *stubApiKeyRepo) Create(ctx context.Context, key *service.APIKey) error {
	if key == nil {
		return errors.New("nil key")
	}
	if key.ID == 0 {
		key.ID = r.nextID
		r.nextID++
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = r.now
	}
	if key.UpdatedAt.IsZero() {
		key.UpdatedAt = r.now
	}
	clone := *key
	r.byID[clone.ID] = &clone
	r.byKey[clone.Key] = &clone
	return nil
}

func (r *stubApiKeyRepo) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	key, ok := r.byID[id]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func (r *stubApiKeyRepo) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	key, ok := r.byID[id]
	if !ok {
		return "", 0, service.ErrAPIKeyNotFound
	}
	return key.Key, key.UserID, nil
}

func (r *stubApiKeyRepo) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	found, ok := r.byKey[key]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *found
	return &clone, nil
}

func (r *stubApiKeyRepo) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	return r.GetByKey(ctx, key)
}

func (r *stubApiKeyRepo) Update(ctx context.Context, key *service.APIKey, _ service.APIKeyUpdateFields) error {
	if key == nil {
		return errors.New("nil key")
	}
	if _, ok := r.byID[key.ID]; !ok {
		return service.ErrAPIKeyNotFound
	}
	if key.UpdatedAt.IsZero() {
		key.UpdatedAt = r.now
	}
	clone := *key
	r.byID[clone.ID] = &clone
	r.byKey[clone.Key] = &clone
	return nil
}

func (r *stubApiKeyRepo) Delete(ctx context.Context, id int64) error {
	key, ok := r.byID[id]
	if !ok {
		return service.ErrAPIKeyNotFound
	}
	delete(r.byID, id)
	delete(r.byKey, key.Key)
	return nil
}

func (r *stubApiKeyRepo) DeleteWithAudit(ctx context.Context, id int64) error {
	return r.Delete(ctx, id)
}

func (r *stubApiKeyRepo) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, _ service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	ids := make([]int64, 0, len(r.byID))
	for id := range r.byID {
		if r.byID[id].UserID == userID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })

	start := params.Offset()
	if start > len(ids) {
		start = len(ids)
	}
	end := start + params.Limit()
	if end > len(ids) {
		end = len(ids)
	}

	out := make([]service.APIKey, 0, end-start)
	for _, id := range ids[start:end] {
		clone := *r.byID[id]
		out = append(out, clone)
	}

	total := int64(len(ids))
	pageSize := params.Limit()
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		pages = 1
	}
	return out, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: pageSize,
		Pages:    pages,
	}, nil
}

func (r *stubApiKeyRepo) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	if len(apiKeyIDs) == 0 {
		return []int64{}, nil
	}
	seen := make(map[int64]struct{}, len(apiKeyIDs))
	out := make([]int64, 0, len(apiKeyIDs))
	for _, id := range apiKeyIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		key, ok := r.byID[id]
		if ok && key.UserID == userID {
			out = append(out, id)
		}
	}
	return out, nil
}

func (r *stubApiKeyRepo) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	var count int64
	for _, key := range r.byID {
		if key.UserID == userID {
			count++
		}
	}
	return count, nil
}

func (r *stubApiKeyRepo) ExistsByKey(ctx context.Context, key string) (bool, error) {
	_, ok := r.byKey[key]
	return ok, nil
}

func (r *stubApiKeyRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
	var updated int64
	for id, key := range r.byID {
		if key.UserID != userID || key.GroupID == nil || *key.GroupID != oldGroupID {
			continue
		}
		clone := *key
		gid := newGroupID
		clone.GroupID = &gid
		r.byID[id] = &clone
		r.byKey[clone.Key] = &clone
		updated++
	}
	return updated, nil
}

func (r *stubApiKeyRepo) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	key, ok := r.byID[id]
	if !ok {
		return service.ErrAPIKeyNotFound
	}
	ts := usedAt
	key.LastUsedAt = &ts
	key.UpdatedAt = usedAt
	clone := *key
	r.byID[id] = &clone
	r.byKey[clone.Key] = &clone
	return nil
}

func (r *stubApiKeyRepo) IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error {
	return nil
}
func (r *stubApiKeyRepo) ResetRateLimitWindows(ctx context.Context, id int64) error {
	return nil
}
func (r *stubApiKeyRepo) GetRateLimitData(ctx context.Context, id int64) (*service.APIKeyRateLimitData, error) {
	return nil, nil
}

type stubUsageLogRepo struct {
	userLogs map[int64][]service.UsageLog
}

func newStubUsageLogRepo() *stubUsageLogRepo {
	return &stubUsageLogRepo{userLogs: make(map[int64][]service.UsageLog)}
}

func (r *stubUsageLogRepo) SetUserLogs(userID int64, logs []service.UsageLog) {
	r.userLogs[userID] = logs
}

func (r *stubUsageLogRepo) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetByID(ctx context.Context, id int64) (*service.UsageLog, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	logs := r.userLogs[userID]
	total := int64(len(logs))
	out := paginateLogs(logs, params)
	return out, paginationResult(total, params), nil
}

func (r *stubUsageLogRepo) ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	logs := r.userLogs[userID]
	return logs, paginationResult(int64(len(logs)), pagination.PaginationParams{Page: 1, PageSize: 100}), nil
}

func (r *stubUsageLogRepo) ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.ModelStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.EndpointStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUpstreamEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.EndpointStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.GroupStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserBreakdownStats(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int) ([]usagestats.UserBreakdownItem, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserSpendingRanking(ctx context.Context, startTime, endTime time.Time, limit int) (*usagestats.UserSpendingRankingResponse, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	logs := r.userLogs[userID]
	if len(logs) == 0 {
		return &usagestats.UsageStats{}, nil
	}

	var totalRequests int64
	var totalInputTokens int64
	var totalOutputTokens int64
	var totalCacheTokens int64
	var totalCacheCreationTokens int64
	var totalCacheReadTokens int64
	var totalCost float64
	var totalActualCost float64
	var totalDuration int64
	var durationCount int64

	for _, log := range logs {
		totalRequests++
		totalInputTokens += int64(log.InputTokens)
		totalOutputTokens += int64(log.OutputTokens)
		totalCacheTokens += int64(log.CacheCreationTokens + log.CacheReadTokens)
		totalCacheCreationTokens += int64(log.CacheCreationTokens)
		totalCacheReadTokens += int64(log.CacheReadTokens)
		totalCost += log.TotalCost
		totalActualCost += log.ActualCost
		if log.DurationMs != nil {
			totalDuration += int64(*log.DurationMs)
			durationCount++
		}
	}

	var avgDuration float64
	if durationCount > 0 {
		avgDuration = float64(totalDuration) / float64(durationCount)
	}

	return &usagestats.UsageStats{
		TotalRequests:            totalRequests,
		TotalInputTokens:         totalInputTokens,
		TotalOutputTokens:        totalOutputTokens,
		TotalCacheTokens:         totalCacheTokens,
		TotalCacheCreationTokens: totalCacheCreationTokens,
		TotalCacheReadTokens:     totalCacheReadTokens,
		TotalTokens:              totalInputTokens + totalOutputTokens + totalCacheTokens,
		TotalCost:                totalCost,
		TotalActualCost:          totalActualCost,
		AverageDurationMs:        avgDuration,
	}, nil
}

func (r *stubUsageLogRepo) GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) ([]map[string]any, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchUserUsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*usagestats.UserDashboardStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	logs := r.userLogs[filters.UserID]

	// Apply filters
	var filtered []service.UsageLog
	for _, log := range logs {
		// Apply APIKeyID filter
		if filters.APIKeyID > 0 && log.APIKeyID != filters.APIKeyID {
			continue
		}
		// Apply Model filter
		if filters.Model != "" && stubUsageLogFilterModel(log, filters.ModelFilterSource) != filters.Model {
			continue
		}
		// Apply Stream filter
		if filters.Stream != nil && log.Stream != *filters.Stream {
			continue
		}
		// Apply BillingType filter
		if filters.BillingType != nil && log.BillingType != *filters.BillingType {
			continue
		}
		// Apply time range filters
		if filters.StartTime != nil && log.CreatedAt.Before(*filters.StartTime) {
			continue
		}
		if filters.EndTime != nil && log.CreatedAt.After(*filters.EndTime) {
			continue
		}
		filtered = append(filtered, log)
	}

	total := int64(len(filtered))
	out := paginateLogs(filtered, params)
	return out, paginationResult(total, params), nil
}

func stubUsageLogFilterModel(log service.UsageLog, source string) string {
	if source == usagestats.ModelSourceRequested && log.RequestedModel != "" {
		return log.RequestedModel
	}
	return log.Model
}

func (r *stubUsageLogRepo) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	logs, _, err := r.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 100000}, filters)
	if err != nil {
		return nil, err
	}

	var totalRequests int64
	var totalInputTokens int64
	var totalOutputTokens int64
	var totalCacheTokens int64
	var totalCacheCreationTokens int64
	var totalCacheReadTokens int64
	var totalCost float64
	var totalActualCost float64
	var totalDuration int64
	var durationCount int64

	for _, log := range logs {
		totalRequests++
		totalInputTokens += int64(log.InputTokens)
		totalOutputTokens += int64(log.OutputTokens)
		totalCacheTokens += int64(log.CacheCreationTokens + log.CacheReadTokens)
		totalCacheCreationTokens += int64(log.CacheCreationTokens)
		totalCacheReadTokens += int64(log.CacheReadTokens)
		totalCost += log.TotalCost
		totalActualCost += log.ActualCost
		if log.DurationMs != nil {
			totalDuration += int64(*log.DurationMs)
			durationCount++
		}
	}

	var avgDuration float64
	if durationCount > 0 {
		avgDuration = float64(totalDuration) / float64(durationCount)
	}

	return &usagestats.UsageStats{
		TotalRequests:            totalRequests,
		TotalInputTokens:         totalInputTokens,
		TotalOutputTokens:        totalOutputTokens,
		TotalCacheTokens:         totalCacheTokens,
		TotalCacheCreationTokens: totalCacheCreationTokens,
		TotalCacheReadTokens:     totalCacheReadTokens,
		TotalTokens:              totalInputTokens + totalOutputTokens + totalCacheTokens,
		TotalCost:                totalCost,
		TotalActualCost:          totalActualCost,
		AverageDurationMs:        avgDuration,
		Endpoints:                []usagestats.EndpointStat{},
	}, nil
}
func (r *stubUsageLogRepo) GetAllGroupUsageSummary(ctx context.Context, todayStart time.Time) ([]usagestats.GroupUsageSummary, error) {
	return nil, errors.New("not implemented")
}

type stubSettingRepo struct {
	all map[string]string
}

func newStubSettingRepo() *stubSettingRepo {
	return &stubSettingRepo{all: make(map[string]string)}
}

func (r *stubSettingRepo) SetAll(values map[string]string) {
	r.all = make(map[string]string, len(values))
	for k, v := range values {
		r.all[k] = v
	}
}

func (r *stubSettingRepo) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, ok := r.all[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (r *stubSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := r.all[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *stubSettingRepo) Set(ctx context.Context, key, value string) error {
	r.all[key] = value
	return nil
}

func (r *stubSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r.all[key]
	}
	return out, nil
}

func (r *stubSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.all[k] = v
	}
	return nil
}

func (r *stubSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.all))
	for k, v := range r.all {
		out[k] = v
	}
	return out, nil
}

func (r *stubSettingRepo) Delete(ctx context.Context, key string) error {
	delete(r.all, key)
	return nil
}

func paginateLogs(logs []service.UsageLog, params pagination.PaginationParams) []service.UsageLog {
	start := params.Offset()
	if start > len(logs) {
		start = len(logs)
	}
	end := start + params.Limit()
	if end > len(logs) {
		end = len(logs)
	}
	out := make([]service.UsageLog, 0, end-start)
	out = append(out, logs[start:end]...)
	return out
}

func paginationResult(total int64, params pagination.PaginationParams) *pagination.PaginationResult {
	pageSize := params.Limit()
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		pages = 1
	}
	return &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: pageSize,
		Pages:    pages,
	}
}

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
