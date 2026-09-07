//go:build unit

package service

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestAccountTestService_TestAccountConnection_KiroDispatchesToKiroGatewayService
// 是真实账号联调发现的回归：TestAccountConnection 的平台路由此前完全没有
// Kiro 分支，任何 Kiro 账号点"测试连接"都会落进通用的
// testClaudeAccountConnection，报"No API key available"（Kiro 账号创建时
// Type 统一填的是 apikey，不管实际 auth_method），就算 Type 恰好是 oauth，
// 也会把 Kiro 的 access_token 发去 Anthropic 的真实 API——两条路都是错的。
// 这里验证 PlatformKiro 账号被正确路由到 kiroGatewayService.TestConnection，
// 用的是真实的转发/翻译链路，而不是另一套简化探测。
func TestAccountTestService_TestAccountConnection_KiroDispatchesToKiroGatewayService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"pong"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, frames
	})

	account := kiroTestOAuthAccount(600)
	// 复刻真实场景踩到的坑：账号创建时 Type 被前端统一填成了 apikey，
	// 不管实际 auth_method（idc/builder_id/social）是什么——正确的路由必须
	// 靠 Platform 判断，不能依赖 Type。
	account.Type = AccountTypeAPIKey

	repo := &mockAccountRepoForGemini{
		accountsByID: map[int64]*Account{account.ID: account},
	}
	kiroSvc := &KiroGatewayService{}
	kiroSvc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	svc := &AccountTestService{
		accountRepo:        repo,
		kiroGatewayService: kiroSvc,
	}

	rec, c := kiroTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeDefault)
	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))

	body := rec.Body.String()
	require.Contains(t, body, `"type":"test_start"`)
	require.Contains(t, body, "pong")
	require.Contains(t, body, `"success":true`)
}

func TestAccountTestService_TestAccountConnection_KiroSurfacesUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv, _ := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusBadRequest, []byte(`{"message":"malformed schema"}`)
	})

	account := kiroTestOAuthAccount(601)
	repo := &mockAccountRepoForGemini{
		accountsByID: map[int64]*Account{account.ID: account},
	}
	kiroSvc := &KiroGatewayService{}
	kiroSvc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	svc := &AccountTestService{
		accountRepo:        repo,
		kiroGatewayService: kiroSvc,
	}

	_, c := kiroTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeDefault)
	require.Error(t, err, "上游拒绝时必须报错，不能悄悄返回成功")
}
