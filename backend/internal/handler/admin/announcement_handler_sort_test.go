package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type announcementRepoCapture struct {
	service.AnnouncementRepository
	listParams pagination.PaginationParams
}

func (r *announcementRepoCapture) List(ctx context.Context, params pagination.PaginationParams, filters service.AnnouncementListFilters) ([]service.Announcement, *pagination.PaginationResult, error) {
	r.listParams = params
	return []service.Announcement{}, &pagination.PaginationResult{
		Total:    0,
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    0,
	}, nil
}

func (r *announcementRepoCapture) GetByID(ctx context.Context, id int64) (*service.Announcement, error) {
	return &service.Announcement{
		ID:        id,
		Title:     "announcement",
		Content:   "content",
		Status:    service.AnnouncementStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

type announcementUserRepoCapture struct {
	service.UserRepository
	listParams pagination.PaginationParams
}

func (r *announcementUserRepoCapture) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	r.listParams = params
	return []service.User{}, &pagination.PaginationResult{
		Total:    0,
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    0,
	}, nil
}

type announcementReadRepoCapture struct {
	service.AnnouncementReadRepository
}

func (r *announcementReadRepoCapture) GetReadMapByUsers(ctx context.Context, announcementID int64, userIDs []int64) (map[int64]time.Time, error) {
	return map[int64]time.Time{}, nil
}

type announcementUserSubRepoCapture struct {
	service.UserSubscriptionRepository
}

func newAnnouncementSortTestRouter(announcementRepo *announcementRepoCapture, userRepo *announcementUserRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewAnnouncementService(
		announcementRepo,
		&announcementReadRepoCapture{},
		userRepo,
		&announcementUserSubRepoCapture{},
	)
	handler := NewAnnouncementHandler(svc)
	router := gin.New()
	router.GET("/admin/announcements", handler.List)
	router.GET("/admin/announcements/:id/read-status", handler.ListReadStatus)
	return router
}

func TestAdminAnnouncementListSortParams(t *testing.T) {
	announcementRepo := &announcementRepoCapture{}
	userRepo := &announcementUserRepoCapture{}
	router := newAnnouncementSortTestRouter(announcementRepo, userRepo)

	req := httptest.NewRequest(http.MethodGet, "/admin/announcements?sort_by=title&sort_order=ASC", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "title", announcementRepo.listParams.SortBy)
	require.Equal(t, "ASC", announcementRepo.listParams.SortOrder)
}

func TestAdminAnnouncementListSortDefaults(t *testing.T) {
	announcementRepo := &announcementRepoCapture{}
	userRepo := &announcementUserRepoCapture{}
	router := newAnnouncementSortTestRouter(announcementRepo, userRepo)

	req := httptest.NewRequest(http.MethodGet, "/admin/announcements", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "created_at", announcementRepo.listParams.SortBy)
	require.Equal(t, "desc", announcementRepo.listParams.SortOrder)
}

func TestAdminAnnouncementReadStatusSortParams(t *testing.T) {
	announcementRepo := &announcementRepoCapture{}
	userRepo := &announcementUserRepoCapture{}
	router := newAnnouncementSortTestRouter(announcementRepo, userRepo)

	req := httptest.NewRequest(http.MethodGet, "/admin/announcements/1/read-status?sort_by=balance&sort_order=DESC", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "balance", userRepo.listParams.SortBy)
	require.Equal(t, "DESC", userRepo.listParams.SortOrder)
}

func TestAdminAnnouncementReadStatusSortDefaults(t *testing.T) {
	announcementRepo := &announcementRepoCapture{}
	userRepo := &announcementUserRepoCapture{}
	router := newAnnouncementSortTestRouter(announcementRepo, userRepo)

	req := httptest.NewRequest(http.MethodGet, "/admin/announcements/1/read-status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "email", userRepo.listParams.SortBy)
	require.Equal(t, "asc", userRepo.listParams.SortOrder)
}
