package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type leaderboardRouteUsageRepoStub struct {
	service.UsageLogRepository
	called bool
	limit  int
}

func (s *leaderboardRouteUsageRepoStub) GetUserSpendingRanking(
	ctx context.Context,
	startTime, endTime time.Time,
	limit int,
) (*usagestats.UserSpendingRankingResponse, error) {
	s.called = true
	s.limit = limit
	return &usagestats.UserSpendingRankingResponse{
		Ranking: []usagestats.UserSpendingRankingItem{
			{
				UserID:     1,
				Email:      "168123456@qq.com",
				ActualCost: 1.25,
				Requests:   3,
				Tokens:     456,
			},
		},
		TotalActualCost: 1.25,
		TotalRequests:   3,
		TotalTokens:     456,
	}, nil
}

func TestRegisterUserRoutesTodayLeaderboard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &leaderboardRouteUsageRepoStub{}
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	handlers := &handler.Handlers{
		Usage: handler.NewUsageHandler(usageSvc, nil, nil, nil, nil),
	}

	router := gin.New()
	v1 := router.Group("/api/v1")
	jwtAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	RegisterUserRoutes(v1, handlers, jwtAuth, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/leaderboard/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.called)
	require.Equal(t, 10, repo.limit)
	require.Contains(t, rec.Body.String(), "168*****456@qq.com")
	require.NotContains(t, rec.Body.String(), "168123456@qq.com")
}
