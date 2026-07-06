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

type leaderboardUsageRepoStub struct {
	service.UsageLogRepository
	called    bool
	startTime time.Time
	endTime   time.Time
	limit     int
	ranking   *usagestats.UserSpendingRankingResponse
}

func (s *leaderboardUsageRepoStub) GetUserSpendingRanking(
	ctx context.Context,
	startTime, endTime time.Time,
	limit int,
) (*usagestats.UserSpendingRankingResponse, error) {
	s.called = true
	s.startTime = startTime
	s.endTime = endTime
	s.limit = limit
	if s.ranking != nil {
		return s.ranking, nil
	}
	return &usagestats.UserSpendingRankingResponse{}, nil
}

func newLeaderboardTestRouter(repo *leaderboardUsageRepoStub, userID int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageSvc, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID})
		c.Next()
	})
	router.GET("/usage/leaderboard/today", handler.TodayLeaderboard)
	return router
}

type todayLeaderboardHandlerResponse struct {
	Code int `json:"code"`
	Data struct {
		Items []todayUsageLeaderboardItem `json:"items"`
	} `json:"data"`
}

func TestTodayLeaderboardMasksEmailsAndLimitsToTopTen(t *testing.T) {
	rows := make([]usagestats.UserSpendingRankingItem, 0, 12)
	for i := 0; i < 12; i++ {
		rows = append(rows, usagestats.UserSpendingRankingItem{
			UserID:     int64(i + 1),
			Email:      "168123456@qq.com",
			ActualCost: float64(12 - i),
			Requests:   int64(20 - i),
			Tokens:     int64(1000 - i),
		})
	}
	repo := &leaderboardUsageRepoStub{
		ranking: &usagestats.UserSpendingRankingResponse{
			Ranking:         rows,
			TotalActualCost: 78,
			TotalRequests:   120,
			TotalTokens:     9000,
		},
	}
	router := newLeaderboardTestRouter(repo, 42)

	req := httptest.NewRequest(http.MethodGet, "/usage/leaderboard/today?timezone=Asia%2FShanghai", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.called)
	require.Equal(t, 10, repo.limit)
	require.Equal(t, 24*time.Hour, repo.endTime.Sub(repo.startTime))

	var got todayLeaderboardHandlerResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Data.Items, 10)
	require.Equal(t, 1, got.Data.Items[0].Rank)
	require.Equal(t, "168*****456@qq.com", got.Data.Items[0].MaskedEmail)
	require.Equal(t, float64(12), got.Data.Items[0].ActualCost)
	require.NotContains(t, rec.Body.String(), "168123456@qq.com")
}

func TestMaskUsageLeaderboardEmailShortLocalPart(t *testing.T) {
	require.Equal(t, "a*****b@example.com", maskUsageLeaderboardEmail("ab@example.com"))
	require.Equal(t, "u*****@example.com", maskUsageLeaderboardEmail("u@example.com"))
	require.Equal(t, "alp*****pha@example.com", maskUsageLeaderboardEmail("alphaalpha@example.com"))
	require.Equal(t, "hidden", maskUsageLeaderboardEmail(""))
}
