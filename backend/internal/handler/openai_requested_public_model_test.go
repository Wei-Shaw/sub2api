package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func newRequestedPublicModelTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

// 返回值必须是同步后的 context。WebSocket 入口持有自己的 ctx 变量并用它选号，
// 只更新 c.Request 而不回写那个变量，公开别名就进不了调度看到的 context——
// 配了渠道映射的分组会出现"规则写了却从不命中"。
func TestRememberRequestedPublicModel_ReturnsSyncedContext(t *testing.T) {
	c := newRequestedPublicModelTestContext(t)

	ctx := rememberRequestedPublicModel(c, "public-alias")

	got, ok := service.RequestedPublicModelFromContext(ctx)
	require.True(t, ok, "返回的 context 必须带上公开别名")
	require.Equal(t, "public-alias", got)

	fromRequest, ok := service.RequestedPublicModelFromContext(c.Request.Context())
	require.True(t, ok, "请求 context 同样要更新")
	require.Equal(t, "public-alias", fromRequest)
}

// composite 中间件先记录过时不覆盖：那已经是解析前的公开名。
func TestRememberRequestedPublicModel_KeepsExistingRecord(t *testing.T) {
	c := newRequestedPublicModelTestContext(t)
	c.Request = c.Request.WithContext(
		service.WithRequestedPublicModel(c.Request.Context(), "composite-alias"),
	)

	ctx := rememberRequestedPublicModel(c, "channel-mapped-model")

	got, ok := service.RequestedPublicModelFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "composite-alias", got)
}

// 空模型名不应写入，否则会把查表回落到调度参数的行为改成"匹配空串"。
func TestRememberRequestedPublicModel_IgnoresBlankModel(t *testing.T) {
	c := newRequestedPublicModelTestContext(t)

	ctx := rememberRequestedPublicModel(c, "   ")

	_, ok := service.RequestedPublicModelFromContext(ctx)
	require.False(t, ok)
}
