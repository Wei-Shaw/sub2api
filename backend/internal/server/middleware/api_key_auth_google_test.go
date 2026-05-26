package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeAPIKeyRepo struct {
	getByKey       func(ctx context.Context, key string) (*service.APIKey, error)
	updateLastUsed func(ctx context.Context, id int64, usedAt time.Time) error
}

type fakeGoogleSubscriptionRepo struct {
	getActive      func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error)
	updateStatus   func(ctx context.Context, subscriptionID int64, status string) error
	activateWindow func(ctx context.Context, id int64, start time.Time) error
	resetDaily     func(ctx context.Context, id int64, start time.Time) error
	resetWeekly    func(ctx context.Context, id int64, start time.Time) error
	resetMonthly   func(ctx context.Context, id int64, start time.Time) error
}

func (f fakeAPIKeyRepo) Create(ctx context.Context, key *service.APIKey) error {
	return errors.New("not implemented")
}
func (f fakeAPIKeyRepo) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	return nil, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	return "", 0, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	if f.getByKey == nil {
		return nil, errors.New("unexpected call")
	}
	return f.getByKey(ctx, key)
}
func (f fakeAPIKeyRepo) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	return f.GetByKey(ctx, key)
}
func (f fakeAPIKeyRepo) Update(ctx context.Context, key *service.APIKey) error {
	return errors.New("not implemented")
}
func (f fakeAPIKeyRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}
func (f fakeAPIKeyRepo) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, _ service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	return nil, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	return 0, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) ExistsByKey(ctx context.Context, key string) (bool, error) {
	return false, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	return nil, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error) {
	return 0, errors.New("not implemented")
}
func (f fakeAPIKeyRepo) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	if f.updateLastUsed != nil {
		return f.updateLastUsed(ctx, id, usedAt)
	}
	return nil
}
func (f fakeAPIKeyRepo) IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error {
	return nil
}
func (f fakeAPIKeyRepo) ResetRateLimitWindows(ctx context.Context, id int64) error {
	return nil
}
func (f fakeAPIKeyRepo) GetRateLimitData(ctx context.Context, id int64) (*service.APIKeyRateLimitData, error) {
	return &service.APIKeyRateLimitData{}, nil
}
func (f fakeAPIKeyRepo) UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f fakeGoogleSubscriptionRepo) Create(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	if f.getActive != nil {
		return f.getActive(ctx, userID, groupID)
	}
	return nil, errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) Update(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return false, errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	if f.updateStatus != nil {
		return f.updateStatus(ctx, subscriptionID, status)
	}
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) ActivateWindows(ctx context.Context, id int64, start time.Time) error {
	if f.activateWindow != nil {
		return f.activateWindow(ctx, id, start)
	}
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) ResetDailyUsage(ctx context.Context, id int64, start time.Time) error {
	if f.resetDaily != nil {
		return f.resetDaily(ctx, id, start)
	}
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) ResetWeeklyUsage(ctx context.Context, id int64, start time.Time) error {
	if f.resetWeekly != nil {
		return f.resetWeekly(ctx, id, start)
	}
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) ResetMonthlyUsage(ctx context.Context, id int64, start time.Time) error {
	if f.resetMonthly != nil {
		return f.resetMonthly(ctx, id, start)
	}
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	return errors.New("not implemented")
}
func (f fakeGoogleSubscriptionRepo) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	return 0, errors.New("not implemented")
}

type googleErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func newTestAPIKeyService(repo service.APIKeyRepository) *service.APIKeyService {
	return service.NewAPIKeyService(
		repo,
		nil, // userRepo (unused in GetByKey)
		nil, // groupRepo
		nil, // userSubRepo
		nil, // userGroupRateRepo
		nil, // cache
		&config.Config{},
	)
}

func TestApiKeyAuthWithSubscriptionGoogle_MissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			return nil, errors.New("should not be called")
		},
	})
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{}))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Error.Code)
	require.Equal(t, "API key is required", resp.Error.Message)
	require.Equal(t, "UNAUTHENTICATED", resp.Error.Status)
}

func TestApiKeyAuthWithSubscriptionGoogle_QueryApiKeyRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			return nil, errors.New("should not be called")
		},
	})
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{}))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test?api_key=legacy", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusBadRequest, resp.Error.Code)
	require.Equal(t, "Query parameter api_key is deprecated. Use Authorization header or key instead.", resp.Error.Message)
	require.Equal(t, "INVALID_ARGUMENT", resp.Error.Status)
}

func TestApiKeyAuthWithSubscriptionGoogleSetsGroupContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:       99,
		Name:     "g1",
		Status:   service.StatusActive,
		Platform: service.PlatformGemini,
		Hydrated: true,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
	}
	apiKey.GroupID = &group.ID

	apiKeyService := service.NewAPIKeyService(
		fakeAPIKeyRepo{
			getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
				if key != apiKey.Key {
					return nil, service.ErrAPIKeyNotFound
				}
				clone := *apiKey
				return &clone, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		&config.Config{RunMode: config.RunModeSimple},
	)

	cfg := &config.Config{RunMode: config.RunModeSimple}
	r := gin.New()
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg))
	r.GET("/v1beta/test", func(c *gin.Context) {
		groupFromCtx, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group)
		if !ok || groupFromCtx == nil || groupFromCtx.ID != group.ID {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestApiKeyAuthWithSubscriptionGoogle_QueryKeyAllowedOnV1Beta(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			return &service.APIKey{
				ID:     1,
				Key:    key,
				Status: service.StatusActive,
				User: &service.User{
					ID:     123,
					Status: service.StatusActive,
				},
			}, nil
		},
	})
	cfg := &config.Config{RunMode: config.RunModeSimple}
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test?key=valid", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestApiKeyAuthWithSubscriptionGoogle_InvalidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			return nil, service.ErrAPIKeyNotFound
		},
	})
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{}))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Error.Code)
	require.Equal(t, "Invalid API key", resp.Error.Message)
	require.Equal(t, "UNAUTHENTICATED", resp.Error.Status)
}

func TestApiKeyAuthWithSubscriptionGoogle_RepoError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			return nil, errors.New("db down")
		},
	})
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{}))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("Authorization", "Bearer any")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Error.Code)
	require.Equal(t, "Failed to validate API key", resp.Error.Message)
	require.Equal(t, "INTERNAL", resp.Error.Status)
}

func TestApiKeyAuthWithSubscriptionGoogle_DisabledKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			return &service.APIKey{
				ID:     1,
				Key:    key,
				Status: service.StatusDisabled,
				User: &service.User{
					ID:     123,
					Status: service.StatusActive,
				},
			}, nil
		},
	})
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{}))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("Authorization", "Bearer disabled")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Error.Code)
	require.Equal(t, "API key is disabled", resp.Error.Message)
	require.Equal(t, "UNAUTHENTICATED", resp.Error.Status)
}

func TestApiKeyAuthWithSubscriptionGoogle_InsufficientBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			return &service.APIKey{
				ID:     1,
				Key:    key,
				Status: service.StatusActive,
				User: &service.User{
					ID:      123,
					Status:  service.StatusActive,
					Balance: 0,
				},
			}, nil
		},
	})
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{}))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("Authorization", "Bearer ok")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusForbidden, resp.Error.Code)
	require.Equal(t, "Insufficient account balance", resp.Error.Message)
	require.Equal(t, "PERMISSION_DENIED", resp.Error.Status)
}

func TestApiKeyAuthWithSubscriptionGoogle_TouchesLastUsedOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          11,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     201,
		UserID: user.ID,
		Key:    "google-touch-ok",
		Status: service.StatusActive,
		User:   user,
	}

	var touchedID int64
	var touchedAt time.Time
	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchedID = id
			touchedAt = usedAt
			return nil
		},
	})
	cfg := &config.Config{RunMode: config.RunModeSimple}
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("x-goog-api-key", apiKey.Key)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, apiKey.ID, touchedID)
	require.False(t, touchedAt.IsZero())
}

func TestApiKeyAuthWithSubscriptionGoogle_TouchFailureDoesNotBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          12,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     202,
		UserID: user.ID,
		Key:    "google-touch-fail",
		Status: service.StatusActive,
		User:   user,
	}

	touchCalls := 0
	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchCalls++
			return errors.New("write failed")
		},
	})
	cfg := &config.Config{RunMode: config.RunModeSimple}
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("x-goog-api-key", apiKey.Key)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, touchCalls)
}

func TestApiKeyAuthWithSubscriptionGoogle_TouchesLastUsedInStandardMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          13,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     203,
		UserID: user.ID,
		Key:    "google-touch-standard",
		Status: service.StatusActive,
		User:   user,
	}

	touchCalls := 0
	r := gin.New()
	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchCalls++
			return nil
		},
	})
	cfg := &config.Config{RunMode: config.RunModeStandard}
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey.Key)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, touchCalls)
}

func TestApiKeyAuthWithSubscriptionGoogle_SubscriptionLimitExceededReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limit := 1.0
	group := &service.Group{
		ID:               77,
		Name:             "gemini-sub",
		Status:           service.StatusActive,
		Platform:         service.PlatformGemini,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeSubscription,
		DailyLimitUSD:    &limit,
	}
	user := &service.User{
		ID:          999,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     501,
		UserID: user.ID,
		Key:    "google-sub-limit",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
	}
	apiKey.GroupID = &group.ID

	apiKeyService := newTestAPIKeyService(fakeAPIKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	})

	now := time.Now()
	sub := &service.UserSubscription{
		ID:               601,
		UserID:           user.ID,
		GroupID:          group.ID,
		Status:           service.SubscriptionStatusActive,
		ExpiresAt:        now.Add(24 * time.Hour),
		DailyWindowStart: &now,
		DailyUsageUSD:    10,
	}
	subscriptionService := service.NewSubscriptionService(nil, fakeGoogleSubscriptionRepo{
		getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
			if userID != user.ID || groupID != group.ID {
				return nil, service.ErrSubscriptionNotFound
			}
			clone := *sub
			return &clone, nil
		},
		updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil },
		activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetDaily:     func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetWeekly:    func(ctx context.Context, id int64, start time.Time) error { return nil },
		resetMonthly:   func(ctx context.Context, id int64, start time.Time) error { return nil },
	}, nil, nil, &config.Config{RunMode: config.RunModeStandard})

	r := gin.New()
	r.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, &config.Config{RunMode: config.RunModeStandard}))
	r.GET("/v1beta/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
	req.Header.Set("x-goog-api-key", apiKey.Key)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	var resp googleErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusTooManyRequests, resp.Error.Code)
	require.Equal(t, "RESOURCE_EXHAUSTED", resp.Error.Status)
	require.Contains(t, resp.Error.Message, "daily usage limit exceeded")
}
