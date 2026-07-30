//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type provisionAdminStub struct {
	stubAdminService
	user *service.User
	err  error
}

func (s *provisionAdminStub) GetUser(context.Context, int64) (*service.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.user != nil {
		return s.user, nil
	}
	return &service.User{ID: 1, Role: service.RoleUser, Status: service.StatusActive}, nil
}

type provisionerStub struct {
	result *service.ProvisionResult
	err    error
	calls  []int64
}

func (p *provisionerStub) ProvisionPrivatePlatformGroups(_ context.Context, userID int64) (*service.ProvisionResult, error) {
	p.calls = append(p.calls, userID)
	if p.err != nil {
		return nil, p.err
	}
	if p.result != nil {
		return p.result, nil
	}
	return &service.ProvisionResult{
		UserID:          userID,
		CreatedGroupIDs: []int64{101},
		EnsuredGroupIDs: []int64{101, 102},
	}, nil
}
func (p *provisionerStub) RevokePrivatePlatformGroups(context.Context, int64) (*service.RevokeResult, error) {
	return &service.RevokeResult{}, nil
}
func (p *provisionerStub) SyncPrivateSubscriptionExpiresAt(context.Context) (*service.SyncResult, error) {
	return &service.SyncResult{}, nil
}
func (p *provisionerStub) AfterCommit(context.Context, *service.ProvisionResult)     {}
func (p *provisionerStub) AfterRevokeCommit(context.Context, *service.RevokeResult) {}
func (p *provisionerStub) EnsurePrivateGroupForPlatform(ctx context.Context, userID int64, platform string) (*service.Group, *service.ProvisionResult, error) {
	return nil, &service.ProvisionResult{}, nil
}


func TestProvisionPrivateGroups_RejectsUnsupportedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := &provisionAdminStub{user: &service.User{ID: 9, Role: "guest"}}
	prov := &provisionerStub{}
	h := NewUserHandler(adminSvc, nil, nil, nil, nil, nil, nil)
	h.SetPrivateGroupDeps(prov, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/9/provision-private-groups", nil)

	h.ProvisionPrivateGroups(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Empty(t, prov.calls)
}

func TestProvisionPrivateGroups_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := &provisionAdminStub{user: &service.User{ID: 3, Role: service.RoleUser}}
	prov := &provisionerStub{}
	h := NewUserHandler(adminSvc, nil, nil, nil, nil, nil, nil)
	h.SetPrivateGroupDeps(prov, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "3"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/3/provision-private-groups", nil)

	h.ProvisionPrivateGroups(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []int64{3}, prov.calls)
}

func TestProvisionPrivateGroups_SuccessForAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := &provisionAdminStub{user: &service.User{ID: 7, Role: service.RoleAdmin}}
	prov := &provisionerStub{}
	h := NewUserHandler(adminSvc, nil, nil, nil, nil, nil, nil)
	h.SetPrivateGroupDeps(prov, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/provision-private-groups", nil)

	h.ProvisionPrivateGroups(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []int64{7}, prov.calls)
}
