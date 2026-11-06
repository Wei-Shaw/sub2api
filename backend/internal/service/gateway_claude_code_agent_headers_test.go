//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Claude Code 子 agent 请求用 x-claude-code-agent-id / x-claude-code-parent-agent-id
// 标识自己。按会话串行的上游靠它们把子 agent 拆成独立会话并行执行；白名单
// 漏掉它们时，所有子 agent 都会排在主会话后面。
func TestAllowedHeaders_IncludeClaudeCodeAgentHeaders(t *testing.T) {
	for _, key := range []string{
		"x-claude-code-session-id",
		"x-claude-code-agent-id",
		"x-claude-code-parent-agent-id",
	} {
		require.True(t, allowedHeaders[key], "allowedHeaders must forward %s", key)
	}
}

func TestBuildUpstreamRequestAnthropicAPIKeyPassthrough_ForwardsClaudeCodeAgentHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("X-Claude-Code-Session-Id", "6f1c2a4e-1b7d-4c3a-9e2f-0a1b2c3d4e5f")
	c.Request.Header.Set("X-Claude-Code-Agent-Id", "agent-child")
	c.Request.Header.Set("X-Claude-Code-Parent-Agent-Id", "agent-parent")

	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, newAnthropicAPIKeyPassthroughAccountForBetaTest(),
		[]byte(`{"model":"claude-haiku-4-5","messages":[]}`), "token",
	)
	require.NoError(t, err)
	require.Equal(t, "6f1c2a4e-1b7d-4c3a-9e2f-0a1b2c3d4e5f", getHeaderRaw(req.Header, "x-claude-code-session-id"))
	require.Equal(t, "agent-child", getHeaderRaw(req.Header, "x-claude-code-agent-id"))
	require.Equal(t, "agent-parent", getHeaderRaw(req.Header, "x-claude-code-parent-agent-id"))
}

func TestBuildUpstreamRequestAnthropicAPIKeyPassthrough_MainThreadHasNoAgentHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("X-Claude-Code-Session-Id", "6f1c2a4e-1b7d-4c3a-9e2f-0a1b2c3d4e5f")

	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, newAnthropicAPIKeyPassthroughAccountForBetaTest(),
		[]byte(`{"model":"claude-haiku-4-5","messages":[]}`), "token",
	)
	require.NoError(t, err)
	require.Empty(t, getHeaderRaw(req.Header, "x-claude-code-agent-id"))
	require.Empty(t, getHeaderRaw(req.Header, "x-claude-code-parent-agent-id"))
}
