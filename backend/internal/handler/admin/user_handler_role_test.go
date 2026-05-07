//go:build unit

package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

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
