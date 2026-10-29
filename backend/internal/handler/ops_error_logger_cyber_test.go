package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// cyber mark 存在时，中间件必须跳过自身落库（由 recordCyberPolicyIfMarked 统一落 403）。
func TestOpsErrorLoggerMiddlewareSkipsCyber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	service.MarkOpsCyberPolicy(c, service.CyberPolicyMark{Code: "cyber_policy", Message: "blocked", UpstreamStatus: http.StatusOK})

	require.NotNil(t, service.GetOpsCyberPolicy(c), "前置：mark 已设置")
	require.True(t, shouldSkipOpsErrorLogForCyber(c), "cyber mark 命中应跳过中间件落库")
}
