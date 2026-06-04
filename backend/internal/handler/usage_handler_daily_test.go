package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dailyUsageRepoStub struct {
	service.UsageLogRepository
	trend []usagestats.TrendDataPoint

	called      bool
	startTime   time.Time
	endTime     time.Time
	granularity string
	userID      int64
	apiKeyID    int64
REDACTED

func (s *dailyUsageRepoStub) GetUsageTrendWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.TrendDataPoint, error) {
	s.called = true
	s.startTime = startTime
	s.endTime = endTime
	s.granularity = granularity
	s.userID = userID
	s.apiKeyID = apiKeyID
	return s.trend, nil
REDACTED

type dailyUsageAPIKeyRepoStub struct {
	service.APIKeyRepository
	keys map[int64]*service.APIKey
REDACTED

func (s *dailyUsageAPIKeyRepoStub) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	key, ok := s.keys[id]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
REDACTED
	clone := *key
	return &clone, nil
REDACTED

func newDailyUsageTestRouter(usageRepo *dailyUsageRepoStub, apiKeyRepo *dailyUsageAPIKeyRepoStub, userID int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	usageSvc := service.NewUsageService(usageRepo, nil, nil, nil)
	apiKeySvc := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, nil)
	handler := NewUsageHandler(usageSvc, apiKeySvc, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userIDREDACTED)
		c.Next()
REDACTED)
	router.GET("/user/api-keys/:id/usage/daily", handler.GetMyAPIKeyDailyUsage)
	return router
REDACTED

type dailyUsageHandlerResponse struct {
	Code int `json:"code"`
	Data struct {
		Items []usagestats.APIKeyDailyUsagePoint `json:"items"`
		Days  int                                `json:"days"`
REDACTED `json:"data"`
REDACTED

func TestGetMyAPIKeyDailyUsageRejectsCrossUserAccess(t *testing.T) {
	usageRepo := &dailyUsageRepoStub{REDACTED
	apiKeyRepo := &dailyUsageAPIKeyRepoStub{
		keys: map[int64]*service.APIKey{
			7: {ID: 7, UserID: 99, Status: service.StatusAPIKeyActiveREDACTED,
	REDACTED,
REDACTED
	router := newDailyUsageTestRouter(usageRepo, apiKeyRepo, 42)

	req := httptest.NewRequest(http.MethodGet, "/user/api-keys/7/usage/daily?days=30", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, usageRepo.called)
REDACTED

func TestGetMyAPIKeyDailyUsageRejectsInvalidDays(t *testing.T) {
	for _, path := range []string{
		"/user/api-keys/7/usage/daily?days=0",
		"/user/api-keys/7/usage/daily?days=91",
REDACTED {
		t.Run(path, func(t *testing.T) {
			usageRepo := &dailyUsageRepoStub{REDACTED
			apiKeyRepo := &dailyUsageAPIKeyRepoStub{
				keys: map[int64]*service.APIKey{
					7: {ID: 7, UserID: 42, Status: service.StatusAPIKeyActiveREDACTED,
			REDACTED,
		REDACTED
			router := newDailyUsageTestRouter(usageRepo, apiKeyRepo, 42)

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.False(t, usageRepo.called)
	REDACTED)
REDACTED
REDACTED

func TestGetMyAPIKeyDailyUsageReturnsEmptyData(t *testing.T) {
	usageRepo := &dailyUsageRepoStub{trend: []usagestats.TrendDataPoint{REDACTEDREDACTED
	apiKeyRepo := &dailyUsageAPIKeyRepoStub{
		keys: map[int64]*service.APIKey{
			7: {ID: 7, UserID: 42, Status: service.StatusAPIKeyActiveREDACTED,
	REDACTED,
REDACTED
	router := newDailyUsageTestRouter(usageRepo, apiKeyRepo, 42)

	req := httptest.NewRequest(http.MethodGet, "/user/api-keys/7/usage/daily", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got dailyUsageHandlerResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, 30, got.Data.Days)
	require.Empty(t, got.Data.Items)
REDACTED

func TestGetMyAPIKeyDailyUsageAggregatesByDayForOwnedKey(t *testing.T) {
	usageRepo := &dailyUsageRepoStub{
		trend: []usagestats.TrendDataPoint{
			{
				Date:                "2026-05-19",
				Requests:            3,
				InputTokens:         10,
				OutputTokens:        20,
				CacheCreationTokens: 4,
				CacheReadTokens:     6,
				TotalTokens:         40,
				Cost:                0.5,
				ActualCost:          0.4,
		REDACTED,
	REDACTED,
REDACTED
	apiKeyRepo := &dailyUsageAPIKeyRepoStub{
		keys: map[int64]*service.APIKey{
			7: {ID: 7, UserID: 42, Status: service.StatusAPIKeyActiveREDACTED,
	REDACTED,
REDACTED
	router := newDailyUsageTestRouter(usageRepo, apiKeyRepo, 42)

	req := httptest.NewRequest(http.MethodGet, "/user/api-keys/7/usage/daily?days=7", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, usageRepo.called)
	require.Equal(t, "day", usageRepo.granularity)
	require.Equal(t, int64(42), usageRepo.userID)
	require.Equal(t, int64(7), usageRepo.apiKeyID)
	require.True(t, usageRepo.startTime.Before(usageRepo.endTime))

	var got dailyUsageHandlerResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, 7, got.Data.Days)
	require.Len(t, got.Data.Items, 1)
	require.Equal(t, usagestats.APIKeyDailyUsagePoint{
		Date:             "2026-05-19",
		Requests:         3,
		InputTokens:      10,
		OutputTokens:     20,
		CacheReadTokens:  6,
		CacheWriteTokens: 4,
		TotalTokens:      40,
		Cost:             0.5,
		ActualCost:       0.4,
REDACTED, got.Data.Items[0])
REDACTED
