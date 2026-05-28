package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type leaderboardRepoStub struct {
	service.UsageLogRepository
	items []usagestats.DailyTokenLeaderboardSourceItem

	called    bool
	startTime time.Time
	endTime   time.Time
	limit     int
}

func (s *leaderboardRepoStub) GetDailyTokenLeaderboard(
	ctx context.Context,
	startTime, endTime time.Time,
	limit int,
) ([]usagestats.DailyTokenLeaderboardSourceItem, error) {
	s.called = true
	s.startTime = startTime
	s.endTime = endTime
	s.limit = limit
	return s.items, nil
}

type leaderboardSettingRepoStub struct {
	service.SettingRepository
	leaderboardEnabled string
}

func (s *leaderboardSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if key == service.SettingKeyDailyTokenLeaderboardEnabled {
		return s.leaderboardEnabled, nil
	}
	return "", service.ErrSettingNotFound
}

func newLeaderboardTestRouter(repo *leaderboardRepoStub, authenticated bool) *gin.Engine {
	return newLeaderboardTestRouterWithFeature(repo, authenticated, true)
}

func newLeaderboardTestRouterWithFeature(repo *leaderboardRepoStub, authenticated bool, enabled bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	settingSvc := service.NewSettingService(&leaderboardSettingRepoStub{
		leaderboardEnabled: strconv.FormatBool(enabled),
	}, nil)
	handler := NewUsageHandler(usageSvc, nil, settingSvc)
	router := gin.New()
	if authenticated {
		router.Use(func(c *gin.Context) {
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
			c.Next()
		})
	}
	router.GET("/usage/leaderboard/daily", handler.DailyTokenLeaderboard)
	return router
}

type dailyTokenLeaderboardHandlerResponse struct {
	Code int `json:"code"`
	Data struct {
		Items []usagestats.DailyTokenLeaderboardItem `json:"items"`
		Limit int                                    `json:"limit"`
	} `json:"data"`
}

func TestDailyTokenLeaderboardRequiresAuthenticatedUser(t *testing.T) {
	repo := &leaderboardRepoStub{}
	router := newLeaderboardTestRouter(repo, false)

	req := httptest.NewRequest(http.MethodGet, "/usage/leaderboard/daily", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, repo.called)
}

func TestDailyTokenLeaderboardReturnsNotFoundWhenFeatureDisabled(t *testing.T) {
	repo := &leaderboardRepoStub{}
	router := newLeaderboardTestRouterWithFeature(repo, true, false)

	req := httptest.NewRequest(http.MethodGet, "/usage/leaderboard/daily", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.False(t, repo.called)
}

func TestDailyTokenLeaderboardReturnsMaskedTopFiveWithoutRawIdentity(t *testing.T) {
	repo := &leaderboardRepoStub{
		items: []usagestats.DailyTokenLeaderboardSourceItem{
			{UserID: 1, Username: "alice", Email: "alice@example.com", TotalTokens: 3000},
			{UserID: 2, Username: "", Email: "bo@example.com", TotalTokens: 2000},
			{UserID: 3, Username: "", Email: "", TotalTokens: 1000},
		},
	}
	router := newLeaderboardTestRouter(repo, true)

	req := httptest.NewRequest(http.MethodGet, "/usage/leaderboard/daily", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.called)
	require.Equal(t, 5, repo.limit)
	require.True(t, repo.startTime.Before(repo.endTime))

	body := rec.Body.String()
	require.NotContains(t, body, "alice@example.com")
	require.NotContains(t, body, `"user_id"`)

	var got dailyTokenLeaderboardHandlerResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, 5, got.Data.Limit)
	require.Equal(t, []usagestats.DailyTokenLeaderboardItem{
		{Rank: 1, DisplayName: "ali***", TotalTokens: 3000},
		{Rank: 2, DisplayName: "bo***", TotalTokens: 2000},
		{Rank: 3, DisplayName: "用户***", TotalTokens: 1000},
	}, got.Data.Items)
}

func TestDailyTokenLeaderboardUsesEnglishFallbackForEnglishLocale(t *testing.T) {
	repo := &leaderboardRepoStub{
		items: []usagestats.DailyTokenLeaderboardSourceItem{
			{UserID: 1, Username: "", Email: "", TotalTokens: 1000},
		},
	}
	router := newLeaderboardTestRouter(repo, true)

	req := httptest.NewRequest(http.MethodGet, "/usage/leaderboard/daily", nil)
	req.Header.Set("Accept-Language", "en")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got dailyTokenLeaderboardHandlerResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []usagestats.DailyTokenLeaderboardItem{
		{Rank: 1, DisplayName: "User***", TotalTokens: 1000},
	}, got.Data.Items)
}

func TestDailyTokenLeaderboardReturnsEmptyItems(t *testing.T) {
	repo := &leaderboardRepoStub{items: []usagestats.DailyTokenLeaderboardSourceItem{}}
	router := newLeaderboardTestRouter(repo, true)

	req := httptest.NewRequest(http.MethodGet, "/usage/leaderboard/daily", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got dailyTokenLeaderboardHandlerResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Empty(t, got.Data.Items)
	require.False(t, strings.Contains(rec.Body.String(), "email"))
}
