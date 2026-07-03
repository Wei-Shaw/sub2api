package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupRequestBindingRejectsXAIPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("create", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", strings.NewReader(`{"name":"grok","platform":"xai"}`))
		c.Request.Header.Set("Content-Type", "application/json")

		var req CreateGroupRequest
		require.Error(t, c.ShouldBindJSON(&req))
	})

	t.Run("update", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/1", strings.NewReader(`{"platform":"xai"}`))
		c.Request.Header.Set("Content-Type", "application/json")

		var req UpdateGroupRequest
		require.Error(t, c.ShouldBindJSON(&req))
	})
}
