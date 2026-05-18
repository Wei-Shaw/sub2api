package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newOpsHandlerTestService(t *testing.T, repo service.OpsRepository) *service.OpsService {
	t.Helper()
	return service.NewOpsService(
		repo,
		newTestSettingRepo(),
		&config.Config{Ops: config.OpsConfig{Enabled: true}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func TestOpsHandler_ListRequestErrors_ScheduledAccountVisibilityRespectsAdminAndGroupSetting(t *testing.T) {
	groupRepo := &opsHandlerGroupVisibilityStub{
		groups: map[int64]*service.Group{
			11: {ID: 11, ExposeScheduledAccountInLogs: false},
			12: {ID: 12, ExposeScheduledAccountInLogs: true},
		},
	}
	repo := &serviceOpsRepoStubForHandler{
		listErrorLogs: &service.OpsErrorLogList{
			Errors: []*service.OpsErrorLog{
				{
					ID:                   1,
					GroupID:              int64Ptr(11),
					ScheduledAccountID:   int64Ptr(88),
					ScheduledAccountName: "scheduled-a",
				},
				{
					ID:                   2,
					GroupID:              int64Ptr(12),
					ScheduledAccountID:   int64Ptr(89),
					ScheduledAccountName: "scheduled-b",
				},
			},
			Total:    2,
			Page:     1,
			PageSize: 20,
		},
	}
	h := NewOpsHandler(newOpsHandlerTestService(t, repo), groupRepo)

	adminBody := performOpsHandlerRequest(t, h.ListRequestErrors, service.RoleAdmin, "/request-errors")
	require.Equal(t, int64(88), gjson.GetBytes(adminBody, "data.items.0.scheduled_account_id").Int())
	require.Equal(t, "scheduled-a", gjson.GetBytes(adminBody, "data.items.0.scheduled_account_name").String())
	require.Equal(t, int64(89), gjson.GetBytes(adminBody, "data.items.1.scheduled_account_id").Int())
	require.Equal(t, "scheduled-b", gjson.GetBytes(adminBody, "data.items.1.scheduled_account_name").String())

	userBody := performOpsHandlerRequest(t, h.ListRequestErrors, service.RoleUser, "/request-errors")
	require.False(t, gjson.GetBytes(userBody, "data.items.0.scheduled_account_id").Exists())
	require.Equal(t, "", gjson.GetBytes(userBody, "data.items.0.scheduled_account_name").String())
	require.Equal(t, int64(89), gjson.GetBytes(userBody, "data.items.1.scheduled_account_id").Int())
	require.Equal(t, "scheduled-b", gjson.GetBytes(userBody, "data.items.1.scheduled_account_name").String())
}

func TestOpsHandler_GetErrorLogByID_ScheduledAccountVisibilityRespectsAdminAndGroupSetting(t *testing.T) {
	groupRepo := &opsHandlerGroupVisibilityStub{
		groups: map[int64]*service.Group{
			11: {ID: 11, ExposeScheduledAccountInLogs: false},
			12: {ID: 12, ExposeScheduledAccountInLogs: true},
		},
	}
	adminRepo := &serviceOpsRepoStubForHandler{
		errorDetail: &service.OpsErrorLogDetail{
			OpsErrorLog: service.OpsErrorLog{
				ID:                   2,
				GroupID:              int64Ptr(11),
				ScheduledAccountID:   int64Ptr(99),
				ScheduledAccountName: "scheduled-b",
			},
		},
	}
	adminHandler := NewOpsHandler(newOpsHandlerTestService(t, adminRepo), groupRepo)

	adminBody := performOpsHandlerRequest(t, adminHandler.GetErrorLogByID, service.RoleAdmin, "/errors/2")
	require.Equal(t, int64(99), gjson.GetBytes(adminBody, "data.scheduled_account_id").Int())
	require.Equal(t, "scheduled-b", gjson.GetBytes(adminBody, "data.scheduled_account_name").String())

	userBody := performOpsHandlerRequest(t, adminHandler.GetErrorLogByID, service.RoleUser, "/errors/2")
	require.False(t, gjson.GetBytes(userBody, "data.scheduled_account_id").Exists())
	require.Equal(t, "", gjson.GetBytes(userBody, "data.scheduled_account_name").String())

	userVisibleRepo := &serviceOpsRepoStubForHandler{
		errorDetail: &service.OpsErrorLogDetail{
			OpsErrorLog: service.OpsErrorLog{
				ID:                   2,
				GroupID:              int64Ptr(12),
				ScheduledAccountID:   int64Ptr(100),
				ScheduledAccountName: "scheduled-c",
			},
		},
	}
	userVisibleHandler := NewOpsHandler(newOpsHandlerTestService(t, userVisibleRepo), groupRepo)
	userVisibleBody := performOpsHandlerRequest(t, userVisibleHandler.GetErrorLogByID, service.RoleUser, "/errors/2")
	require.Equal(t, int64(100), gjson.GetBytes(userVisibleBody, "data.scheduled_account_id").Int())
	require.Equal(t, "scheduled-c", gjson.GetBytes(userVisibleBody, "data.scheduled_account_name").String())
}

func TestOpsHandler_ListRequestDetails_ScheduledAccountVisibilityRespectsAdminAndGroupSetting(t *testing.T) {
	groupRepo := &opsHandlerGroupVisibilityStub{
		groups: map[int64]*service.Group{
			11: {ID: 11, ExposeScheduledAccountInLogs: false},
			12: {ID: 12, ExposeScheduledAccountInLogs: true},
		},
	}
	repo := &serviceOpsRepoStubForHandler{
		requestDetails: []*service.OpsRequestDetail{
			{
				RequestID:            "req-1",
				GroupID:              int64Ptr(11),
				ScheduledAccountID:   int64Ptr(55),
				ScheduledAccountName: "scheduled-c",
			},
			{
				RequestID:            "req-2",
				GroupID:              int64Ptr(12),
				ScheduledAccountID:   int64Ptr(56),
				ScheduledAccountName: "scheduled-d",
			},
		},
	}
	h := NewOpsHandler(newOpsHandlerTestService(t, repo), groupRepo)

	adminBody := performOpsHandlerRequest(t, h.ListRequestDetails, service.RoleAdmin, "/requests")
	require.Equal(t, int64(55), gjson.GetBytes(adminBody, "data.items.0.scheduled_account_id").Int())
	require.Equal(t, "scheduled-c", gjson.GetBytes(adminBody, "data.items.0.scheduled_account_name").String())
	require.Equal(t, int64(56), gjson.GetBytes(adminBody, "data.items.1.scheduled_account_id").Int())
	require.Equal(t, "scheduled-d", gjson.GetBytes(adminBody, "data.items.1.scheduled_account_name").String())

	userBody := performOpsHandlerRequest(t, h.ListRequestDetails, service.RoleUser, "/requests")
	require.False(t, gjson.GetBytes(userBody, "data.items.0.scheduled_account_id").Exists())
	require.Equal(t, "", gjson.GetBytes(userBody, "data.items.0.scheduled_account_name").String())
	require.Equal(t, int64(56), gjson.GetBytes(userBody, "data.items.1.scheduled_account_id").Int())
	require.Equal(t, "scheduled-d", gjson.GetBytes(userBody, "data.items.1.scheduled_account_name").String())
}

type opsHandlerGroupVisibilityStub struct {
	groups map[int64]*service.Group
}

func (s *opsHandlerGroupVisibilityStub) GetByIDLite(_ context.Context, id int64) (*service.Group, error) {
	if s == nil {
		return nil, service.ErrGroupNotFound
	}
	group, ok := s.groups[id]
	if !ok || group == nil {
		return nil, service.ErrGroupNotFound
	}
	cloned := *group
	return &cloned, nil
}

type serviceOpsRepoStubForHandler struct {
	service.OpsRepository
	listErrorLogs  *service.OpsErrorLogList
	errorDetail    *service.OpsErrorLogDetail
	requestDetails []*service.OpsRequestDetail
}

func (s *serviceOpsRepoStubForHandler) ListErrorLogs(ctx context.Context, filter *service.OpsErrorLogFilter) (*service.OpsErrorLogList, error) {
	if s.listErrorLogs != nil {
		return cloneOpsErrorLogList(s.listErrorLogs), nil
	}
	return &service.OpsErrorLogList{Errors: []*service.OpsErrorLog{}, Page: 1, PageSize: 20}, nil
}

func (s *serviceOpsRepoStubForHandler) GetErrorLogByID(ctx context.Context, id int64) (*service.OpsErrorLogDetail, error) {
	if s.errorDetail != nil {
		return cloneOpsErrorLogDetail(s.errorDetail), nil
	}
	return &service.OpsErrorLogDetail{}, nil
}

func (s *serviceOpsRepoStubForHandler) ListRequestDetails(ctx context.Context, filter *service.OpsRequestDetailFilter) ([]*service.OpsRequestDetail, int64, error) {
	if s.requestDetails != nil {
		return cloneOpsRequestDetails(s.requestDetails), int64(len(s.requestDetails)), nil
	}
	return []*service.OpsRequestDetail{}, 0, nil
}

func performOpsHandlerRequest(t *testing.T, handler gin.HandlerFunc, role string, path string) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
		c.Set(string(middleware.ContextKeyUserRole), role)
		c.Next()
	})
	routePath := path
	if path == "/errors/2" {
		routePath = "/errors/:id"
	}
	r.GET(routePath, handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return w.Body.Bytes()
}

func int64Ptr(v int64) *int64 { return &v }

func cloneOpsErrorLogList(src *service.OpsErrorLogList) *service.OpsErrorLogList {
	if src == nil {
		return nil
	}
	out := *src
	out.Errors = make([]*service.OpsErrorLog, 0, len(src.Errors))
	for _, item := range src.Errors {
		if item == nil {
			out.Errors = append(out.Errors, nil)
			continue
		}
		cloned := *item
		out.Errors = append(out.Errors, &cloned)
	}
	return &out
}

func cloneOpsErrorLogDetail(src *service.OpsErrorLogDetail) *service.OpsErrorLogDetail {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func cloneOpsRequestDetails(src []*service.OpsRequestDetail) []*service.OpsRequestDetail {
	out := make([]*service.OpsRequestDetail, 0, len(src))
	for _, item := range src {
		if item == nil {
			out = append(out, nil)
			continue
		}
		cloned := *item
		out = append(out, &cloned)
	}
	return out
}
