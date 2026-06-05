package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userUsageRepoCapture struct {
	service.UsageLogRepository
	listParams  pagination.PaginationParams
	listFilters usagestats.UsageLogFilters
REDACTED

func (s *userUsageRepoCapture) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	s.listParams = params
	s.listFilters = filters
	return []service.UsageLog{REDACTED, &pagination.PaginationResult{
		Total:    0,
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    0,
REDACTED, nil
REDACTED

func newUserUsageRequestTypeTestRouter(repo *userUsageRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageSvc, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42REDACTED)
		c.Next()
REDACTED)
	router.GET("/usage", handler.List)
	return router
REDACTED

func TestUserUsageListRequestTypePriority(t *testing.T) {
	repo := &userUsageRepoCapture{REDACTED
	router := newUserUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage?request_type=ws_v2&stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), repo.listFilters.UserID)
	require.NotNil(t, repo.listFilters.RequestType)
	require.Equal(t, int16(service.RequestTypeWSV2), *repo.listFilters.RequestType)
	require.Nil(t, repo.listFilters.Stream)
REDACTED

func TestUserUsageListInvalidRequestType(t *testing.T) {
	repo := &userUsageRepoCapture{REDACTED
	router := newUserUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage?request_type=invalid", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
REDACTED

func TestUserUsageListInvalidStream(t *testing.T) {
	repo := &userUsageRepoCapture{REDACTED
	router := newUserUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage?stream=invalid", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
REDACTED
