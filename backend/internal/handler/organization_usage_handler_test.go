//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type organizationUsageRepositoryStub struct {
	service.OrganizationRepository
	rows       []service.OrganizationUsageRow
	lastFilter service.OrganizationUsageFilter
}

func (s *organizationUsageRepositoryStub) ListUsage(_ context.Context, _ int64, filter service.OrganizationUsageFilter) ([]service.OrganizationUsageRow, int64, error) {
	s.lastFilter = filter
	return s.rows, int64(len(s.rows)), nil
}

type organizationVideoTaskRepositoryStub struct {
	service.AsyncVideoTaskRepository
	task *service.AsyncVideoTask
}

func (s *organizationVideoTaskRepositoryStub) GetByID(context.Context, int64) (*service.AsyncVideoTask, error) {
	return s.task, nil
}

func TestOrganizationUsageTimeFilterParsesListFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/organization/usage?group_id=7&billing_type=1&billing_mode=video", nil)

	filter := organizationUsageTimeFilter(ctx)

	require.NotNil(t, filter.GroupID)
	require.Equal(t, int64(7), *filter.GroupID)
	require.NotNil(t, filter.BillingType)
	require.Equal(t, int8(1), *filter.BillingType)
	require.Equal(t, "video", filter.BillingMode)
}

func TestOrganizationUsageListPassesBillingFiltersAndPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &organizationUsageRepositoryStub{}
	handler := NewOrganizationHandler(service.NewOrganizationService(repository, nil, &config.Config{}), nil, nil, nil, nil, nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/organization/usage?page=3&page_size=50&group_id=7&billing_type=1&billing_mode=video", nil)
	ctx.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 99})

	handler.Usage(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 3, repository.lastFilter.Page)
	require.Equal(t, 50, repository.lastFilter.PageSize)
	require.NotNil(t, repository.lastFilter.GroupID)
	require.Equal(t, int64(7), *repository.lastFilter.GroupID)
	require.NotNil(t, repository.lastFilter.BillingType)
	require.Equal(t, int8(1), *repository.lastFilter.BillingType)
	require.Equal(t, "video", repository.lastFilter.BillingMode)
}

func TestOrganizationUsageVideoTaskUsesScopedUsageRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	usageID := int64(88)
	taskID := int64(17)
	memberID := int64(42)
	repository := &organizationUsageRepositoryStub{rows: []service.OrganizationUsageRow{{
		ID:           usageID,
		MemberUserID: memberID,
		BillingMode:  string(service.BillingModeVideo),
		TaskID:       &taskID,
	}}}
	videoRepository := &organizationVideoTaskRepositoryStub{task: &service.AsyncVideoTask{
		ID:             taskID,
		UserID:         memberID,
		RequestedModel: "bytedance/seedance-2.5/text-to-video",
		Status:         service.AsyncVideoStatusSucceeded,
		FinalCost:      1.25,
		VideoURLs:      []string{"https://cdn.example.test/video.mp4"},
	}}
	handler := NewOrganizationHandler(
		service.NewOrganizationService(repository, nil, &config.Config{}),
		nil, nil, nil, nil,
		service.NewAsyncVideoService(videoRepository, nil, nil),
	)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/organization/usage/88/video-task", nil)
	ctx.Params = gin.Params{{Key: "usage_id", Value: "88"}}
	ctx.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 99})

	handler.UsageVideoTask(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NotNil(t, repository.lastFilter.UsageID)
	require.Equal(t, usageID, *repository.lastFilter.UsageID)
	require.Contains(t, recorder.Body.String(), `"final_cost":1.25`)
	require.Contains(t, recorder.Body.String(), `"https://cdn.example.test/video.mp4"`)
}
