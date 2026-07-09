//go:build unit

package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pluginkit"

	"github.com/stretchr/testify/require"
)

// TestProxyStreamSSE 反代流式（phase-4 决策 1 的核心卖点）：
// 插件的 SSE 响应经 unix socket 反代完整到达，Content-Type 保留。
func TestProxyStreamSSE(t *testing.T) {
	f := newSupervisorFixture(t)
	const id = pluginkit.ID("ext.sse")
	f.installFakePlugin(t, id, fakePluginConfig{Mode: "ok"})
	f.setEnabled(t, id, true)

	srv := httptest.NewServer(dispatchEngine(f.s))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/plugins/ext.sse/api/stream")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	body := make([]byte, 0, 128)
	buf := make([]byte, 64)
	for {
		n, err := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if err != nil {
			break
		}
	}
	for seq := 1; seq <= 3; seq++ {
		require.Contains(t, string(body), fmt.Sprintf(`data: {"seq":%d}`, seq))
	}
}

// TestProxyStripsInboundCredentials 断言反代把用户凭据（Authorization/Cookie/
// Proxy-Authorization）在转发给插件进程前剥离，非凭据的自定义头照常透传。
// 插件的身份来自宿主注入的 token，绝不应看到调用者的 JWT/Cookie。
func TestProxyStripsInboundCredentials(t *testing.T) {
	f := newSupervisorFixture(t)
	const id = pluginkit.ID("ext.echo")
	f.installFakePlugin(t, id, fakePluginConfig{Mode: "ok"})
	f.setEnabled(t, id, true)

	srv := httptest.NewServer(dispatchEngine(f.s))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/plugins/ext.echo/api/echo-headers", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer user-jwt-secret")
	req.Header.Set("Cookie", "session=abc123")
	req.Header.Set("Proxy-Authorization", "Basic zzz")
	req.Header.Set("X-Api-Key", "admin-api-key-secret")
	req.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin, jwt.admin-jwt-secret")
	req.Header.Set("X-Custom", "keep-me")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Empty(t, got["authorization"], "Authorization must be stripped before reaching plugin")
	require.Empty(t, got["cookie"], "Cookie must be stripped before reaching plugin")
	require.Empty(t, got["proxy_authorization"], "Proxy-Authorization must be stripped")
	require.Empty(t, got["x_api_key"], "X-Api-Key (admin credential) must be stripped")
	require.Equal(t, "sub2api-admin", got["sec_websocket_protocol"],
		"jwt.* subprotocol entries must be removed while other subprotocols pass through")
	require.Equal(t, "keep-me", got["x_custom"], "non-credential headers must pass through")
}

// TestProxyDispatchPathTraversalRejected user 面经 ".." 归一化蹦到 /admin
// 前缀的请求在宿主侧收口拒绝（与未知插件同一个 404），不透传给插件进程
// router 的归一化行为兜底。
func TestProxyDispatchPathTraversalRejected(t *testing.T) {
	f := newSupervisorFixture(t)
	const id = pluginkit.ID("ext.trav")
	f.installFakePlugin(t, id, fakePluginConfig{Mode: "ok"})
	f.setEnabled(t, id, true)
	engine := dispatchEngine(f.s)

	// 对照组：admin 面直达 /admin/info 正常。
	require.Equal(t, http.StatusOK, doDispatch(engine, "/api/v1/admin/plugins/ext.trav/api/info").Code)

	// user 面经 ".." 穿越到 /admin/info 必须 404。
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.trav/api/../admin/info").Code)
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.trav/api/x/../../admin/info").Code)
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.trav/api/..").Code)
}

// TestStripWebSocketJWT 覆盖子协议凭据摘除的边界：多值头、仅凭据条目、无该头。
func TestStripWebSocketJWT(t *testing.T) {
	h := http.Header{}
	h.Add("Sec-WebSocket-Protocol", "graphql-ws, jwt.aaa")
	h.Add("Sec-WebSocket-Protocol", "jwt.bbb, custom-proto")
	stripWebSocketJWT(h)
	require.Equal(t, "graphql-ws, custom-proto", h.Get("Sec-WebSocket-Protocol"))

	h = http.Header{}
	h.Set("Sec-WebSocket-Protocol", "jwt.only-credential")
	stripWebSocketJWT(h)
	_, present := h["Sec-Websocket-Protocol"]
	require.False(t, present, "header must be dropped entirely when only jwt.* entries remain")

	h = http.Header{}
	stripWebSocketJWT(h) // 无该头时为 no-op，不新增空头
	_, present = h["Sec-Websocket-Protocol"]
	require.False(t, present)
}

// TestAllowlistedHostEnvExcludesSecrets 断言子进程 env 白名单放行中性系统
// 变量、屏蔽宿主机密（DB/JWT/Redis 凭据）。
func TestAllowlistedHostEnvExcludesSecrets(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("TZ", "UTC")
	t.Setenv("LC_ALL", "C")
	t.Setenv("DATABASE_URL", "postgres://secret")
	t.Setenv("JWT_SECRET_KEY", "topsecret")
	t.Setenv("REDIS_PASSWORD", "hunter2")

	joined := "\n" + strings.Join(allowlistedHostEnv(), "\n") + "\n"
	require.Contains(t, joined, "\nPATH=/usr/bin\n")
	require.Contains(t, joined, "\nTZ=UTC\n")
	require.Contains(t, joined, "\nLC_ALL=C\n")
	require.NotContains(t, joined, "DATABASE_URL")
	require.NotContains(t, joined, "JWT_SECRET_KEY")
	require.NotContains(t, joined, "REDIS_PASSWORD")
}

// TestProxyDispatchGate 分发门控：未安装 / 已装未启用 / 纯前端无进程
// 一律同一个 404（防探测）。
func TestProxyDispatchGate(t *testing.T) {
	f := newSupervisorFixture(t)
	engine := dispatchEngine(f.s)

	// 未安装
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.ghost/api/info").Code)
	require.False(t, f.s.Handles("ext.ghost"))

	// 已安装未启用
	const id = pluginkit.ID("ext.gated")
	f.installFakePlugin(t, id, fakePluginConfig{Mode: "ok"})
	require.True(t, f.s.Handles(id))
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.gated/api/info").Code)

	// 纯前端插件：enabled 但无后端进程 → API 面 404
	webInst := &Installation{
		ID:      "ext.webonly",
		Version: "1.0.0",
		Manifest: &Manifest{
			ID: "ext.webonly", Name: "Web Only", Version: "1.0.0", Protocol: ProtocolHTTP1,
			Frontend: &FrontendSpec{Entry: "webapp/plugin.js"},
		},
		InstallPath: t.TempDir(),
	}
	require.NoError(t, f.installs.Upsert(context.Background(), webInst))
	f.s.NotifyInstalled(webInst)
	f.setEnabled(t, "ext.webonly", true)
	st, _ := f.s.StatusOf("ext.webonly")
	require.Equal(t, pluginkit.StateRunning, st.State, "纯前端插件 enabled 即 running")
	require.Equal(t, http.StatusNotFound, doDispatch(engine, "/api/v1/plugins/ext.webonly/api/info").Code)
}
