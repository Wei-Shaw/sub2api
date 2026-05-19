//go:build unit

package middleware

import (
	"context"
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

func TestSimpleModeBypassesQuotaCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limit := 1.0
	group := &service.Group{
		ID:               42,
		Name:             "sub",
		Status:           service.StatusActive,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeSubscription,
		DailyLimitUSD:    &limit,
REDACTED
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
REDACTED
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
REDACTED
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
		REDACTED
			clone := *apiKey
			return &clone, nil
	REDACTED,
REDACTED

	t.Run("standard_mode_needs_maintenance_does_not_block_request", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeStandardREDACTED
		cfg.SubscriptionMaintenance.WorkerCount = 1
		cfg.SubscriptionMaintenance.QueueSize = 1

		apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)

		past := time.Now().Add(-48 * time.Hour)
		sub := &service.UserSubscription{
			ID:               55,
			UserID:           user.ID,
			GroupID:          group.ID,
			Status:           service.SubscriptionStatusActive,
			ExpiresAt:        time.Now().Add(24 * time.Hour),
			DailyWindowStart: &past,
			DailyUsageUSD:    0,
	REDACTED
		maintenanceCalled := make(chan struct{REDACTED, 1)
		subscriptionRepo := &stubUserSubscriptionRepo{
			getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
				clone := *sub
				return &clone, nil
		REDACTED,
			updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil REDACTED,
			activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil REDACTED,
			resetDaily: func(ctx context.Context, id int64, start time.Time) error {
				maintenanceCalled <- struct{REDACTED{REDACTED
				return nil
		REDACTED,
			resetWeekly:  func(ctx context.Context, id int64, start time.Time) error { return nil REDACTED,
			resetMonthly: func(ctx context.Context, id int64, start time.Time) error { return nil REDACTED,
	REDACTED
		subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, cfg)
		t.Cleanup(subscriptionService.Stop)

		router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("x-api-key", apiKey.Key)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		select {
		case <-maintenanceCalled:
			// ok
		case <-time.After(time.Second):
			t.Fatalf("expected maintenance to be scheduled")
	REDACTED
REDACTED)

	t.Run("simple_mode_bypasses_quota_check", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
		apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
		subscriptionService := service.NewSubscriptionService(nil, &stubUserSubscriptionRepo{REDACTED, nil, nil, cfg)
		router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("x-api-key", apiKey.Key)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
REDACTED)

	t.Run("simple_mode_accepts_lowercase_bearer", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
		apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
		subscriptionService := service.NewSubscriptionService(nil, &stubUserSubscriptionRepo{REDACTED, nil, nil, cfg)
		router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Authorization", "bearer "+apiKey.Key)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
REDACTED)

	t.Run("standard_mode_enforces_quota_check", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeStandardREDACTED
		apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)

		now := time.Now()
		sub := &service.UserSubscription{
			ID:               55,
			UserID:           user.ID,
			GroupID:          group.ID,
			Status:           service.SubscriptionStatusActive,
			ExpiresAt:        now.Add(24 * time.Hour),
			DailyWindowStart: &now,
			DailyUsageUSD:    10,
	REDACTED
		subscriptionRepo := &stubUserSubscriptionRepo{
			getActive: func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
				if userID != sub.UserID || groupID != sub.GroupID {
					return nil, service.ErrSubscriptionNotFound
			REDACTED
				clone := *sub
				return &clone, nil
		REDACTED,
			updateStatus:   func(ctx context.Context, subscriptionID int64, status string) error { return nil REDACTED,
			activateWindow: func(ctx context.Context, id int64, start time.Time) error { return nil REDACTED,
			resetDaily:     func(ctx context.Context, id int64, start time.Time) error { return nil REDACTED,
			resetWeekly:    func(ctx context.Context, id int64, start time.Time) error { return nil REDACTED,
			resetMonthly:   func(ctx context.Context, id int64, start time.Time) error { return nil REDACTED,
	REDACTED
		subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, cfg)
		router := newAuthTestRouter(apiKeyService, subscriptionService, cfg)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("x-api-key", apiKey.Key)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusTooManyRequests, w.Code)
		require.Contains(t, w.Body.String(), "USAGE_LIMIT_EXCEEDED")
REDACTED)
REDACTED

func TestAPIKeyAuthSetsGroupContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:       101,
		Name:     "g1",
		Status:   service.StatusActive,
		Platform: service.PlatformAnthropic,
		Hydrated: true,
REDACTED
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
REDACTED
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
REDACTED
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
		REDACTED
			clone := *apiKey
			return &clone, nil
	REDACTED,
REDACTED

	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.GET("/t", func(c *gin.Context) {
		groupFromCtx, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group)
		if !ok || groupFromCtx == nil || groupFromCtx.ID != group.ID {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": falseREDACTED)
			return
	REDACTED
		c.JSON(http.StatusOK, gin.H{"ok": trueREDACTED)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
REDACTED

func TestAPIKeyAuthOverwritesInvalidContextGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:       101,
		Name:     "g1",
		Status:   service.StatusActive,
		Platform: service.PlatformAnthropic,
		Hydrated: true,
REDACTED
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
REDACTED
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
REDACTED
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
		REDACTED
			clone := *apiKey
			return &clone, nil
	REDACTED,
REDACTED

	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))

	invalidGroup := &service.Group{
		ID:       group.ID,
		Platform: group.Platform,
		Status:   group.Status,
REDACTED
	router.GET("/t", func(c *gin.Context) {
		groupFromCtx, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group)
		if !ok || groupFromCtx == nil || groupFromCtx.ID != group.ID || !groupFromCtx.Hydrated || groupFromCtx == invalidGroup {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": falseREDACTED)
			return
	REDACTED
		c.JSON(http.StatusOK, gin.H{"ok": trueREDACTED)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, invalidGroup))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
REDACTED

func TestAPIKeyAuthIPRestrictionDoesNotTrustSpoofedForwardHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
REDACTED
	apiKey := &service.APIKey{
		ID:          100,
		UserID:      user.ID,
		Key:         "test-key",
		Status:      service.StatusActive,
		User:        user,
		IPWhitelist: []string{"1.2.3.4"REDACTED,
REDACTED

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
		REDACTED
			clone := *apiKey
			return &clone, nil
	REDACTED,
REDACTED

	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	var markedBusinessLimited bool
	var businessLimitedReason string
	router.Use(func(c *gin.Context) {
		c.Next()
		markedBusinessLimited = service.HasOpsClientBusinessLimited(c)
		if v, ok := c.Get(service.OpsClientBusinessLimitedReasonKey); ok {
			businessLimitedReason, _ = v.(string)
	REDACTED
REDACTED)
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": trueREDACTED)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("x-api-key", apiKey.Key)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "ACCESS_DENIED")
	require.True(t, markedBusinessLimited)
	require.Equal(t, service.OpsClientBusinessLimitedReasonIPRestriction, businessLimitedReason)
REDACTED

func TestAPIKeyAuthTouchesLastUsedOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
REDACTED
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "touch-ok",
		Status: service.StatusActive,
		User:   user,
REDACTED

	var touchedID int64
	var touchedAt time.Time
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
		REDACTED
			clone := *apiKey
			return &clone, nil
	REDACTED,
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchedID = id
			touchedAt = usedAt
			return nil
	REDACTED,
REDACTED

	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, apiKey.ID, touchedID)
	require.False(t, touchedAt.IsZero(), "expected touch timestamp")
REDACTED

func TestAPIKeyAuthTouchLastUsedFailureDoesNotBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          8,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
REDACTED
	apiKey := &service.APIKey{
		ID:     101,
		UserID: user.ID,
		Key:    "touch-fail",
		Status: service.StatusActive,
		User:   user,
REDACTED

	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
		REDACTED
			clone := *apiKey
			return &clone, nil
	REDACTED,
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchCalls++
			return errors.New("db unavailable")
	REDACTED,
REDACTED

	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "touch failure should not block request")
	require.Equal(t, 1, touchCalls)
REDACTED

func TestAPIKeyAuthTouchesLastUsedInStandardMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          9,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
REDACTED
	apiKey := &service.APIKey{
		ID:     102,
		UserID: user.ID,
		Key:    "touch-standard",
		Status: service.StatusActive,
		User:   user,
REDACTED

	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
		REDACTED
			clone := *apiKey
			return &clone, nil
	REDACTED,
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchCalls++
			return nil
	REDACTED,
REDACTED

	cfg := &config.Config{RunMode: config.RunModeStandardREDACTED
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, nil, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, touchCalls)
REDACTED

func newAuthTestRouter(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) *gin.Engine {
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, subscriptionService, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": trueREDACTED)
REDACTED)
	return router
REDACTED

type stubApiKeyRepo struct {
	getByKey       func(ctx context.Context, key string) (*service.APIKey, error)
	updateLastUsed func(ctx context.Context, id int64, usedAt time.Time) error
REDACTED

func (r *stubApiKeyRepo) Create(ctx context.Context, key *service.APIKey) error {
	return errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	return "", 0, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	if r.getByKey != nil {
		return r.getByKey(ctx, key)
REDACTED
	return nil, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	return r.GetByKey(ctx, key)
REDACTED

func (r *stubApiKeyRepo) Update(ctx context.Context, key *service.APIKey) error {
	return errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, _ service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED

func (r *stubApiKeyRepo) ExistsByKey(ctx context.Context, key string) (bool, error) {
	return false, errors.New("not implemented")
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

func (r *stubApiKeyRepo) UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
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
	if r.updateLastUsed != nil {
		return r.updateLastUsed(ctx, id, usedAt)
REDACTED
	return nil
REDACTED

func (r *stubApiKeyRepo) IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error {
	return nil
REDACTED
func (r *stubApiKeyRepo) ResetRateLimitWindows(ctx context.Context, id int64) error {
	return nil
REDACTED
func (r *stubApiKeyRepo) GetRateLimitData(ctx context.Context, id int64) (*service.APIKeyRateLimitData, error) {
	return nil, nil
REDACTED

type stubUserSubscriptionRepo struct {
	getActive      func(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error)
	updateStatus   func(ctx context.Context, subscriptionID int64, status string) error
	activateWindow func(ctx context.Context, id int64, start time.Time) error
	resetDaily     func(ctx context.Context, id int64, start time.Time) error
	resetWeekly    func(ctx context.Context, id int64, start time.Time) error
	resetMonthly   func(ctx context.Context, id int64, start time.Time) error
REDACTED

func (r *stubUserSubscriptionRepo) Create(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	if r.getActive != nil {
		return r.getActive(ctx, userID, groupID)
REDACTED
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) Update(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	return nil, errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return false, errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	if r.updateStatus != nil {
		return r.updateStatus(ctx, subscriptionID, status)
REDACTED
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) ActivateWindows(ctx context.Context, id int64, start time.Time) error {
	if r.activateWindow != nil {
		return r.activateWindow(ctx, id, start)
REDACTED
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) ResetDailyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	if r.resetDaily != nil {
		return r.resetDaily(ctx, id, newWindowStart)
REDACTED
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) ResetWeeklyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	if r.resetWeekly != nil {
		return r.resetWeekly(ctx, id, newWindowStart)
REDACTED
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) ResetMonthlyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	if r.resetMonthly != nil {
		return r.resetMonthly(ctx, id, newWindowStart)
REDACTED
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	return errors.New("not implemented")
REDACTED

func (r *stubUserSubscriptionRepo) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	return 0, errors.New("not implemented")
REDACTED
