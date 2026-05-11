//go:build unit

package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserHandlerCreateForwardsSelectedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/users",
		bytes.NewBufferString(`{"email":"marketer@example.com","password":"safe-password","role":"marketing"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Create(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, adminSvc.createdUsers, 1)
	require.Equal(t, service.RoleMarketing, adminSvc.createdUsers[0].Role)
}

func TestUserHandlerUpdateForwardsSelectedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/admin/users/7",
		bytes.NewBufferString(`{"role":"marketing"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Update(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{7}, adminSvc.updatedUserIDs)
	require.Len(t, adminSvc.updatedUsers, 1)
	require.Equal(t, service.RoleMarketing, adminSvc.updatedUsers[0].Role)
}

func TestUserHandlerUpdateStatusAdminBypassesMarketingScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/42", bytes.NewBufferString(`{"status":"active"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Update(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{42}, adminSvc.updatedUserIDs)
	require.Len(t, adminSvc.updatedUsers, 1)
	require.Equal(t, service.StatusActive, adminSvc.updatedUsers[0].Status)
	require.Zero(t, adminSvc.lastListUsers.calls, "admin must not be scoped through marketing affiliate filters")
}

func TestUserHandlerUpdateStatusMarketingRequiresAffiliateScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Set(string(middleware.ContextKeyUserRole), service.RoleMarketing)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/42", bytes.NewBufferString(`{"status":"active"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Update(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Empty(t, adminSvc.updatedUserIDs)
	require.Equal(t, 1, adminSvc.lastListUsers.calls)
	require.NotNil(t, adminSvc.lastListUsers.filters.AffiliateInviterID)
	require.Equal(t, int64(7), *adminSvc.lastListUsers.filters.AffiliateInviterID)
	require.NotNil(t, adminSvc.lastListUsers.filters.UserID)
	require.Equal(t, int64(42), *adminSvc.lastListUsers.filters.UserID)
}

func TestUserHandlerUpdateMarketingAllowsAffiliateScopedPendingActivation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	adminSvc.users = []service.User{{ID: 42, Email: "customer@example.com", Status: service.StatusPendingActivation}}
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Set(string(middleware.ContextKeyUserRole), service.RoleMarketing)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/42", bytes.NewBufferString(`{"status":"active"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Update(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{42}, adminSvc.updatedUserIDs)
	require.Len(t, adminSvc.updatedUsers, 1)
	require.Equal(t, service.StatusActive, adminSvc.updatedUsers[0].Status)
	require.Equal(t, 1, adminSvc.lastListUsers.calls)
	require.NotNil(t, adminSvc.lastListUsers.filters.AffiliateInviterID)
	require.Equal(t, int64(7), *adminSvc.lastListUsers.filters.AffiliateInviterID)
	require.NotNil(t, adminSvc.lastListUsers.filters.UserID)
	require.Equal(t, int64(42), *adminSvc.lastListUsers.filters.UserID)
	require.Empty(t, adminSvc.lastListUsers.filters.Status)
}

func TestUserHandlerUpdateMarketingAllowsAffiliateScopedActiveUserBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	adminSvc.users = []service.User{{ID: 42, Email: "customer@example.com", Status: service.StatusActive}}
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Set(string(middleware.ContextKeyUserRole), service.RoleMarketing)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/42", bytes.NewBufferString(`{"status":"blocked"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Update(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{42}, adminSvc.updatedUserIDs)
	require.Len(t, adminSvc.updatedUsers, 1)
	require.Equal(t, service.StatusBlocked, adminSvc.updatedUsers[0].Status)
	require.Equal(t, 1, adminSvc.lastListUsers.calls)
	require.Empty(t, adminSvc.lastListUsers.filters.Status)
}

func TestUserHandlerUpdateMarketingAllowsAffiliateScopedProfilePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	adminSvc.users = []service.User{{ID: 42, Email: "customer@example.com", Status: service.StatusActive}}
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Set(string(middleware.ContextKeyUserRole), service.RoleMarketing)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
	c.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/admin/users/42",
		bytes.NewBufferString(`{"email":"customer+edited@example.com","password":"safe-password","username":"edited","notes":"managed by marketing","balance":12.5,"concurrency":3,"rpm_limit":60,"status":"blocked"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Update(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{42}, adminSvc.updatedUserIDs)
	require.Len(t, adminSvc.updatedUsers, 1)
	input := adminSvc.updatedUsers[0]
	require.Equal(t, "customer+edited@example.com", input.Email)
	require.Equal(t, "safe-password", input.Password)
	require.NotNil(t, input.Username)
	require.Equal(t, "edited", *input.Username)
	require.NotNil(t, input.Notes)
	require.Equal(t, "managed by marketing", *input.Notes)
	require.NotNil(t, input.Balance)
	require.Equal(t, 12.5, *input.Balance)
	require.NotNil(t, input.Concurrency)
	require.Equal(t, 3, *input.Concurrency)
	require.NotNil(t, input.RPMLimit)
	require.Equal(t, 60, *input.RPMLimit)
	require.Equal(t, service.StatusBlocked, input.Status)
	require.Empty(t, input.Role)
	require.Equal(t, 1, adminSvc.lastListUsers.calls)
	require.Empty(t, adminSvc.lastListUsers.filters.Status)
}

func TestUserHandlerUpdateMarketingRejectsRolePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Set(string(middleware.ContextKeyUserRole), service.RoleMarketing)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/42", bytes.NewBufferString(`{"status":"active","role":"admin"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Update(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Empty(t, adminSvc.updatedUserIDs)
	require.Zero(t, adminSvc.lastListUsers.calls, "invalid marketing payload should be rejected before affiliate scope lookup")
}

func TestUserHandlerMarketingUpdateBalanceAllowsAffiliateScopedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	marketingUserID := int64(91)
	adminSvc := newStubAdminService()
	adminSvc.users = []service.User{{ID: 7, Email: "scoped@example.com", Status: service.StatusActive}}
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/users/7/balance",
		bytes.NewBufferString(`{"balance":25,"operation":"add","notes":"affiliate deposit"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUserRole), service.RoleMarketing)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: marketingUserID})

	handler.UpdateBalance(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, adminSvc.updatedBalanceCalls, 1)
	require.Equal(t, stubBalanceUpdateCall{UserID: 7, Balance: 25, Operation: "add", Notes: "affiliate deposit"}, adminSvc.updatedBalanceCalls[0])
	require.NotNil(t, adminSvc.lastListUsers.filters.AffiliateInviterID)
	require.Equal(t, marketingUserID, *adminSvc.lastListUsers.filters.AffiliateInviterID)
	require.NotNil(t, adminSvc.lastListUsers.filters.UserID)
	require.Equal(t, int64(7), *adminSvc.lastListUsers.filters.UserID)
}

func TestUserHandlerMarketingUpdateBalanceRejectsUserOutsideAffiliateScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	adminSvc.users = nil
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/users/7/balance",
		bytes.NewBufferString(`{"balance":25,"operation":"add"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUserRole), service.RoleMarketing)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 91})

	handler.UpdateBalance(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Empty(t, adminSvc.updatedBalanceCalls)
}
