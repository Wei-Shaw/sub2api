package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type groupsStatusRepoStub struct {
	service.GroupRepository
	groups []service.Group
	err    error
}

func (s *groupsStatusRepoStub) ListWithFilters(
	_ context.Context,
	params pagination.PaginationParams,
	_, _, _ string,
	_ *bool,
) ([]service.Group, *pagination.PaginationResult, error) {
	if s.err != nil {
		return nil, nil, s.err
	}
	return s.groups, &pagination.PaginationResult{
		Page: params.Page, PageSize: params.PageSize, Pages: 1, Total: int64(len(s.groups)),
	}, nil
}

func executeGroupsStatusRequest(t *testing.T, repo *groupsStatusRepoStub) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/groups-status", nil)
	NewGroupsStatusHandler(service.NewGroupService(repo, nil)).Get(c)
	return w
}

func TestResolveGroupAvailability_AllStates(t *testing.T) {
	tests := []struct {
		name  string
		group *service.Group
		want  string
	}{
		{name: "healthy", group: &service.Group{Status: service.StatusActive, ActiveAccountCount: 3}, want: groupAvailabilityAvailable},
		{name: "partially limited", group: &service.Group{Status: service.StatusActive, ActiveAccountCount: 2, RateLimitedAccountCount: 1}, want: groupAvailabilityDegraded},
		{name: "fully limited", group: &service.Group{Status: service.StatusActive, RateLimitedAccountCount: 2}, want: groupAvailabilityRateLimited},
		{name: "no available accounts", group: &service.Group{Status: service.StatusActive, AccountCount: 2}, want: groupAvailabilityUnavailable},
		{name: "disabled group", group: &service.Group{Status: service.StatusDisabled, ActiveAccountCount: 2}, want: groupAvailabilityUnavailable},
		{name: "nil", group: nil, want: groupAvailabilityUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, resolveGroupAvailability(tt.group))
		})
	}
}

func TestGroupsStatusHandler_ReturnsAllAvailabilityStatesAndDTOFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &groupsStatusRepoStub{groups: []service.Group{
		{
			ID: 1, Name: "healthy", Description: "public healthy group", Platform: service.PlatformAnthropic,
			SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 0.8,
			PeakRateEnabled: true, PeakStart: "09:00", PeakEnd: "12:00", PeakRateMultiplier: 1.5,
			Status: service.StatusActive, AccountCount: 4, ActiveAccountCount: 3,
		},
		{
			ID: 2, Name: "degraded", Platform: service.PlatformOpenAI,
			SubscriptionType: service.SubscriptionTypeSubscription, RateMultiplier: 1.2,
			Status: service.StatusActive, AccountCount: 5, ActiveAccountCount: 2, RateLimitedAccountCount: 2,
		},
		{
			ID: 3, Name: "fully-limited", Platform: service.PlatformGemini,
			SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 1,
			Status: service.StatusActive, AccountCount: 2, RateLimitedAccountCount: 2,
		},
		{
			ID: 4, Name: "disabled", Platform: service.PlatformGrok,
			SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 1.25,
			Status: service.StatusDisabled, AccountCount: 2, ActiveAccountCount: 2,
		},
		{
			ID: 5, Name: "exclusive-secret", Platform: service.PlatformGemini,
			IsExclusive: true, Status: service.StatusActive, AccountCount: 99, ActiveAccountCount: 99,
		},
	}}
	w := executeGroupsStatusRequest(t, repo)

	require.Equal(t, http.StatusOK, w.Code)
	var envelope struct {
		Code int                  `json:"code"`
		Data groupsStatusResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	require.Equal(t, []groupsStatusGroup{
		{
			ID: 1, Name: "healthy", Description: "public healthy group", Platform: service.PlatformAnthropic,
			SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 0.8,
			PeakRateEnabled: true, PeakStart: "09:00", PeakEnd: "12:00", PeakRateMultiplier: 1.5,
			AccountCount: 4, AvailableAccountCount: 3, RateLimitedAccountCount: 0,
			Status: service.StatusActive, Availability: groupAvailabilityAvailable, Available: true,
		},
		{
			ID: 2, Name: "degraded", Platform: service.PlatformOpenAI,
			SubscriptionType: service.SubscriptionTypeSubscription, RateMultiplier: 1.2,
			AccountCount: 5, AvailableAccountCount: 2, RateLimitedAccountCount: 2,
			Status: service.StatusActive, Availability: groupAvailabilityDegraded, Available: true,
		},
		{
			ID: 3, Name: "fully-limited", Platform: service.PlatformGemini,
			SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 1,
			AccountCount: 2, AvailableAccountCount: 0, RateLimitedAccountCount: 2,
			Status: service.StatusActive, Availability: groupAvailabilityRateLimited, Available: false,
		},
		{
			ID: 4, Name: "disabled", Platform: service.PlatformGrok,
			SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 1.25,
			AccountCount: 2, AvailableAccountCount: 2, RateLimitedAccountCount: 0,
			Status: service.StatusDisabled, Availability: groupAvailabilityUnavailable, Available: false,
		},
	}, envelope.Data.Groups)
	require.Equal(t, groupsStatusSummary{
		GroupCount:              4,
		AvailableGroupCount:     2,
		AccountCount:            13,
		AvailableAccountCount:   7,
		RateLimitedAccountCount: 4,
	}, envelope.Data.Summary)

	var raw response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	encoded := w.Body.String()
	require.NotContains(t, encoded, "exclusive-secret")
	require.NotContains(t, encoded, "is_exclusive")
}

func TestGroupsStatusHandler_EmptyResultUsesArrayAndZeroSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := executeGroupsStatusRequest(t, &groupsStatusRepoStub{groups: []service.Group{}})

	require.Equal(t, http.StatusOK, w.Code)
	var envelope struct {
		Code int                  `json:"code"`
		Data groupsStatusResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	require.NotNil(t, envelope.Data.Groups)
	require.Empty(t, envelope.Data.Groups)
	require.Equal(t, groupsStatusSummary{}, envelope.Data.Summary)
	require.Contains(t, w.Body.String(), `"groups":[]`)
}

func TestGroupsStatusHandler_ListPublicErrorDoesNotLeakInternalDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const internalDetail = "database unavailable at postgres://secret-host/groups"
	w := executeGroupsStatusRequest(t, &groupsStatusRepoStub{err: errors.New(internalDetail)})

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var envelope response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, http.StatusInternalServerError, envelope.Code)
	require.Equal(t, "internal error", envelope.Message)
	require.Empty(t, envelope.Reason)
	require.NotContains(t, w.Body.String(), internalDetail)
	require.NotContains(t, w.Body.String(), "list public groups")
}
