package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type groupsStatusRouteRepoStub struct {
	service.GroupRepository
}

func (s *groupsStatusRouteRepoStub) ListWithFilters(
	_ context.Context,
	params pagination.PaginationParams,
	_, _, _ string,
	_ *bool,
) ([]service.Group, *pagination.PaginationResult, error) {
	return []service.Group{{
			ID:                 1,
			Name:               "public",
			Platform:           service.PlatformAnthropic,
			Status:             service.StatusActive,
			AccountCount:       1,
			ActiveAccountCount: 1,
		}}, &pagination.PaginationResult{
			Page: params.Page, PageSize: params.PageSize, Pages: 1, Total: 1,
		}, nil
}

func TestRegisterGroupsStatusRoutes_AnonymousGETOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	groupService := service.NewGroupService(&groupsStatusRouteRepoStub{}, nil)
	RegisterGroupsStatusRoutes(
		router.Group("/api/v1"),
		&handler.Handlers{GroupsStatus: handler.NewGroupsStatusHandler(groupService)},
		nil,
	)

	routes := router.Routes()
	require.Len(t, routes, 1)
	require.Equal(t, http.MethodGet, routes[0].Method)
	require.Equal(t, "/api/v1/groups-status", routes[0].Path)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/groups-status", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":0`)
	require.Contains(t, recorder.Body.String(), `"name":"public"`)

	request = httptest.NewRequest(http.MethodPost, "/api/v1/groups-status", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
