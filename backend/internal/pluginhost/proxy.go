package pluginhost

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"

	"github.com/gin-gonic/gin"
)

// 插件进程侧的鉴权面前缀，与 pluginkit.Manager 私有子路由器的
// /*path 重写语义严格对齐（phase-2 契约）：
//
//	/api/v1/admin/plugins/:id/api/*path → <socket>/admin/*path
//	/api/v1/plugins/:id/api/*path       → <socket>/user/*path
const (
	proxyAdminPathPrefix = "/admin"
	proxyUserPathPrefix  = "/user"
)

// proxyResponseHeaderTimeout 是等待插件写出响应头的上限（首字节超时）：
// 插件 accept 后不响应时释放宿主请求 goroutine，避免被慢/卡死插件无限占用。
// 只约束响应头，SSE/长流式在头写出后不受影响。
const proxyResponseHeaderTimeout = 60 * time.Second

// strippedInboundHeaders 是转发给插件进程前必须删除的入站请求头：
// 插件的身份来自宿主注入的 token，绝不应看到调用者的凭据。
// X-Api-Key 是管理员一等凭据（admin_auth 中间件与 JWT 等价），同级剥离。
var strippedInboundHeaders = []string{
	"Authorization",
	"Cookie",
	"Proxy-Authorization",
	"X-Api-Key",
}

// stripWebSocketJWT 从 Sec-WebSocket-Protocol 中剔除 jwt.<token> 条目：
// 管理员 WS 握手经该头携带 JWT（admin_auth 中间件契约），但整头剥离会破坏
// 插件自身的 WS 子协议协商，故只摘除凭据条目、保留其余子协议。
func stripWebSocketJWT(h http.Header) {
	const headerName = "Sec-Websocket-Protocol"
	values := h.Values(headerName)
	if len(values) == 0 {
		return
	}
	kept := make([]string, 0, len(values))
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			p := strings.TrimSpace(part)
			if p == "" || strings.HasPrefix(p, "jwt.") {
				continue
			}
			kept = append(kept, p)
		}
	}
	h.Del(headerName)
	if len(kept) > 0 {
		h.Set(headerName, strings.Join(kept, ", "))
	}
}

// proxyConfig 是构造反代所需的宿主侧配置。
type proxyConfig struct {
	// headerFilter 对插件响应施加与核心网关一致的响应头白名单过滤
	// （phase-4 风险对策 3：防插件泄内部头）；nil 走默认白名单。
	headerFilter *responseheaders.CompiledHeaderFilter
	logger       *slog.Logger
}

// Dispatch 是外部插件的数据面分发器入口：把宿主侧请求经 unix socket
// 反代到插件进程。门控 enabled && healthy；未安装、未启用、进程未就绪
// 一律同一个 404（与 Manager.Dispatch 的防探测语义一致）。
func (s *Supervisor) Dispatch(side pluginkit.Side, id string, c *gin.Context) {
	var prefix string
	switch side {
	case pluginkit.SideAdmin:
		prefix = proxyAdminPathPrefix
	case pluginkit.SideUser:
		prefix = proxyUserPathPrefix
	default:
		response.NotFound(c, "plugin not found")
		return
	}

	pid := pluginkit.ID(id)
	e := s.entryOf(pid)
	if e == nil || !s.states.Enabled(pid) {
		response.NotFound(c, "plugin not found")
		return
	}
	e.mu.Lock()
	proc := e.proc
	e.mu.Unlock()
	if proc == nil || !proc.healthy.Load() {
		response.NotFound(c, "plugin not found")
		return
	}

	// gin 的 *path 参数自带前导 /；路由无尾段时为空。归一化后逃出鉴权面
	// 前缀的路径（".." 穿越）与未知插件同一个 404：/user 与 /admin 的权限
	// 断言必须在宿主收口，不得依赖插件进程 router 的归一化行为。
	target, ok := pluginkit.SanitizeDispatchPath(prefix, c.Param("path"))
	if !ok {
		response.NotFound(c, "plugin not found")
		return
	}
	req := c.Request
	origPath, origRawPath := req.URL.Path, req.URL.RawPath
	req.URL.Path = target
	req.URL.RawPath = ""
	// 转发后还原，宿主侧后置中间件（日志/指标）看到的仍是原始路径。
	defer func() { req.URL.Path, req.URL.RawPath = origPath, origRawPath }()
	proc.proxy.ServeHTTP(c.Writer, req)
}

// newSocketReverseProxy 构造经 unix socket 拨号的反向代理：
//   - FlushInterval -1：对所有响应立即冲刷，SSE/流式天然可用（phase-4 决策 1）；
//   - ModifyResponse：复用核心网关的响应头白名单过滤；
//   - ErrorHandler：socket 拨号/中途失败统一 502（连接期失败才会走到，
//     已开始写响应体的流中断由 HTTP 库断连兜底）。
func newSocketReverseProxy(id pluginkit.ID, socketPath string, cfg proxyConfig) http.Handler {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		ResponseHeaderTimeout: proxyResponseHeaderTimeout,
	}
	return &httputil.ReverseProxy{
		Transport:     transport,
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			// URL.Path 已由 Dispatch 重写为插件相对路径；Host 仅为占位
			//（实际经 unix socket 拨号，不解析主机名）。
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "plugin"
			pr.SetXForwarded()
			// 剥离入站用户凭据：插件经宿主注入的 token 建立身份，绝不应看到
			// 调用者的 JWT/Cookie。宿主鉴权已在分发前完成，转发这些头只会
			// 把终端用户/管理员凭据无谓地扩散进插件进程。
			for _, h := range strippedInboundHeaders {
				pr.Out.Header.Del(h)
			}
			stripWebSocketJWT(pr.Out.Header)
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header = responseheaders.FilterHeaders(resp.Header, cfg.headerFilter)
			// 插件响应经宿主域名对外：Location 会以宿主身份把浏览器带去任意域
			//（跳转钓鱼），WWW-Authenticate 会在宿主域弹认证窗口（钓凭据）。
			// 默认白名单本就不含二者，此处显式剥离防配置放宽后回归。
			resp.Header.Del("Location")
			resp.Header.Del("WWW-Authenticate")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			cfg.logger.Error("plugin_proxy_error", "plugin", string(id), "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"code":502,"message":"plugin upstream unavailable"}`))
		},
	}
}
