package plugin

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	pluginsdkroot "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// Header names exposed to plugins. They are also documented in
// plugin-sdk/README.md. New plugin authors should rely on these constants via
// the SDK's RequestMetadata helper rather than parsing them by hand.
const (
	HeaderPluginUserID    = "X-Plugin-User-ID"
	HeaderPluginUserRole  = "X-Plugin-User-Role"
	HeaderPluginName      = "X-Plugin-Name"
	HeaderPluginRequestID = "X-Plugin-Request-ID"
	HeaderPluginAPIKeyID  = "X-Plugin-API-Key-ID"
	HeaderPluginClientIP  = "X-Plugin-Client-IP"
	// HeaderTraceparent matches pluginsdkroot.MetadataTraceparentKey so the
	// HTTP header (case-insensitive) and the gRPC metadata key (lower-case)
	// share the same wire identifier; downstream handlers reading either
	// surface end up with the same value.
	HeaderTraceparent = pluginsdkroot.MetadataTraceparentKey
)

// PluginRouter 是核心 HTTP 入口的包装层。
// 请求进来后先尝试匹配插件路由表;若命中则按鉴权要求执行中间件,然后反向代理到插件进程;
// 若未命中则交给原 Gin 引擎处理。
type PluginRouter struct {
	routeTable  atomic.Pointer[RouteTable]
	coreHandler http.Handler
	jwtAuth     gin.HandlerFunc
	adminAuth   gin.HandlerFunc
	apiKeyAuth  gin.HandlerFunc

	// proxies 缓存反向代理实例,避免每次请求重新构造。
	proxies sync.Map // map[string]*httputil.ReverseProxy
}

// NewPluginRouter 构造插件路由器,coreHandler 通常是 *gin.Engine。
// 三个鉴权中间件在请求被代理到插件前执行,与正式路由享有完全一致的鉴权语义。
func NewPluginRouter(coreHandler http.Handler, jwtAuth, adminAuth, apiKeyAuth gin.HandlerFunc) *PluginRouter {
	r := &PluginRouter{
		coreHandler: coreHandler,
		jwtAuth:     jwtAuth,
		adminAuth:   adminAuth,
		apiKeyAuth:  apiKeyAuth,
	}
	r.routeTable.Store(NewRouteTable())
	return r
}

// SwapRouteTable 原子地替换路由表,供 manager 在插件加载/卸载时调用。
func (r *PluginRouter) SwapRouteTable(table *RouteTable) {
	if table == nil {
		table = NewRouteTable()
	}
	r.routeTable.Store(table)
}

// CurrentTable 返回当前路由表快照,主要供测试与状态接口使用。
func (r *PluginRouter) CurrentTable() *RouteTable {
	return r.routeTable.Load()
}

// ServeHTTP 实现 http.Handler 接口。
func (r *PluginRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	table := r.routeTable.Load()
	entry, ok := table.Match(req.Method, req.URL.Path)
	if !ok {
		r.coreHandler.ServeHTTP(w, req)
		return
	}

	// trace 注入必须在鉴权之前: 鉴权 middleware 拿到的 req.Context() 会带有
	// trace_id, 这样鉴权失败路径的结构化日志(401/403)也能附上一致的 trace,
	// 跨进程链路不会因为提前返回而断裂。injectRequestContext 里仍会幂等地再
	// 调一次 ensureTraceparentOnRequest, 不会重写已有的合法 traceparent。
	req = ensureTraceparentOnRequest(req)

	// 借用 Gin 的鉴权中间件:先在临时 gin.Engine 上执行鉴权,
	// 鉴权通过后从 gin.Context 中读取用户信息再转发。
	authCtx, ok := r.runAuthMiddleware(w, req, entry.AuthType)
	if !ok {
		// 鉴权失败,中间件已写入响应。
		return
	}

	r.proxyTo(w, req, authCtx, entry)
}

// runAuthMiddleware 通过临时 gin.Engine 执行匹配 authType 的鉴权中间件。
// 返回的 *gin.Context 在通过时携带鉴权信息(无鉴权时返回 nil ctx + ok=true)。
// ok=false 表示鉴权失败,响应已经被中间件写出,调用方应直接返回。
func (r *PluginRouter) runAuthMiddleware(w http.ResponseWriter, req *http.Request, authType string) (*gin.Context, bool) {
	if authType == "" || authType == AuthTypeNone {
		return nil, true
	}

	var handler gin.HandlerFunc
	switch authType {
	case AuthTypeAdmin:
		handler = r.adminAuth
	case AuthTypeUser:
		handler = r.jwtAuth
	case AuthTypeAPIKey:
		handler = r.apiKeyAuth
	default:
		// 未知鉴权类型按禁止处理,防止意外暴露插件接口。
		http.Error(w, "unknown auth type", http.StatusForbidden)
		return nil, false
	}

	if handler == nil {
		// 中间件未注入(测试或配置错误),保守地返回 503。
		http.Error(w, "auth middleware not configured", http.StatusServiceUnavailable)
		return nil, false
	}

	engine := gin.New()
	engine.Use(handler)
	var captured *gin.Context
	engine.Any("/*any", func(c *gin.Context) {
		captured = c
	})
	engine.ServeHTTP(w, req)
	if captured == nil {
		// handler 已 abort,响应已写出。
		return nil, false
	}
	return captured, true
}

// proxyTo 把请求反向代理到插件进程。
// 在转发前会清除 hop-by-hop header,并补充 X-Plugin-* / traceparent 头便于
// 插件识别调用者并串联跨进程日志/追踪。
func (r *PluginRouter) proxyTo(w http.ResponseWriter, req *http.Request, authCtx *gin.Context, entry *RouteEntry) {
	target, err := url.Parse(entry.ProxyURL)
	if err != nil {
		http.Error(w, "invalid plugin upstream", http.StatusBadGateway)
		return
	}

	proxy := r.getOrCreateProxy(entry.ProxyURL, target)
	req = injectRequestContext(req, authCtx, entry.PluginName)

	// 移除 hop-by-hop header,避免误传给插件 HTTP server。
	stripHopByHopHeaders(req.Header)

	proxy.ServeHTTP(w, req)
}

// ensureTraceparentOnRequest 是 trace 注入的单一入口, 必须在所有鉴权 / 业务
// middleware 之前调用一次。它具有 idempotent 语义: 第二次进入时 header 已合法、
// ctx 已携带相同值, 不会重新生成或覆盖。
//
// 两层效果:
//   - req.Header[HeaderTraceparent] 是插件经 RequestMetadata.Traceparent()
//     读取的 wire-level header (HTTP 路径)。
//   - req.Context() 经 pluginsdkroot.WithTraceparent 增强, 让 host 进程内的
//     handler / 鉴权 middleware / 嵌套 gRPC 客户端都能通过 TraceparentFromContext
//     拿到同一 id, 与 gRPC server interceptor 行为对齐。
//
// 把 trace 注入与 plugin 头注入解耦的原因: trace 与鉴权是两个独立横切关注点,
// 鉴权失败路径(401/403)上的日志也应该带 trace_id, 因此 trace 必须在鉴权之前
// 完成注入(见 ServeHTTP)。
func ensureTraceparentOnRequest(req *http.Request) *http.Request {
	tp := req.Header.Get(HeaderTraceparent)
	if !pluginsdkroot.IsValidTraceparent(tp) {
		tp = pluginsdkroot.NewTraceparent()
		if tp != "" {
			req.Header.Set(HeaderTraceparent, tp)
		}
	}
	if tp == "" {
		return req
	}
	// 已带相同 traceparent 的 ctx 重复 WithValue 等价于 no-op (相同 key 同值),
	// 但仍会构造一个新 ctx; 用 TraceparentFromContext 早返回避免无谓分配。
	if existing, ok := pluginsdkroot.TraceparentFromContext(req.Context()); ok && existing == tp {
		return req
	}
	return req.WithContext(pluginsdkroot.WithTraceparent(req.Context(), tp))
}

// injectRequestContext 将 plugin 关心的上下文头写入 req:
//   - X-Plugin-User-ID / X-Plugin-User-Role: 来自鉴权中间件捕获的 gin.Context
//   - X-Plugin-API-Key-ID: 仅 APIKey 鉴权路径有,只暴露 ID 不暴露 raw key
//   - X-Plugin-Client-IP: gin.Context.ClientIP() (已尊重 trust proxy)
//   - X-Plugin-Name: 插件名 (用于 plugin 多实例区分)
//   - X-Plugin-Request-ID: 优先透传上游, 否则生成 UUID v4
//   - traceparent: 委托给 ensureTraceparentOnRequest, 保持 idempotent (鉴权前
//     ServeHTTP 已注入过, 这里只是兜底)
//
// 注意: 不透传任何敏感原始凭据(JWT body、raw API key)。
//
// Returns the (possibly ctx-augmented) request the caller should use for
// onward handling.
func injectRequestContext(req *http.Request, authCtx *gin.Context, pluginName string) *http.Request {
	// 安全前置: 在重新写入受信任的 X-Plugin-* header 之前, 必须无条件清除任何
	// 客户端伪造的同名 header。这覆盖 AuthTypeNone 路径 (公开 / webhook 端点);
	// 否则攻击者可以通过 `X-Plugin-User-ID: 1` / `X-Plugin-User-Role: admin`
	// 绕过插件内部基于 RequestMetadata 的鉴权判断, 因为这些 header 设计上是
	// host 单向写入的 (见 docs/plugin-architecture/V4-CURATE.md)。
	//
	// HeaderPluginName 由本函数无条件重写, 但仍一并 Del 以保持"鉴权类
	// X-Plugin-* 全部由 host 控制"的不变量。HeaderPluginRequestID 故意保留
	// 透传语义 (前端 / gateway 已生成的 request id 可关联跨进程日志), 它不
	// 承载权限信息, 攻击者伪造只影响自身请求的日志聚合。
	stripInboundAuthPluginHeaders(req.Header)

	if authCtx != nil {
		if subject, ok := middleware.GetAuthSubjectFromContext(authCtx); ok && subject.UserID > 0 {
			req.Header.Set(HeaderPluginUserID, strconv.FormatInt(subject.UserID, 10))
		}
		if role, ok := middleware.GetUserRoleFromContext(authCtx); ok && role != "" {
			req.Header.Set(HeaderPluginUserRole, role)
		}
		if apiKey, ok := middleware.GetAPIKeyFromContext(authCtx); ok && apiKey != nil && apiKey.ID > 0 {
			req.Header.Set(HeaderPluginAPIKeyID, strconv.FormatInt(apiKey.ID, 10))
		}
		if ip := authCtx.ClientIP(); ip != "" {
			req.Header.Set(HeaderPluginClientIP, ip)
		}
	}
	req.Header.Set(HeaderPluginName, pluginName)

	// request id：上游 (前端 / gateway) 已带就透传，否则在 proxy 边界生成。
	if req.Header.Get(HeaderPluginRequestID) == "" {
		req.Header.Set(HeaderPluginRequestID, uuid.NewString())
	}

	// trace: ServeHTTP 已在鉴权前调过 ensureTraceparentOnRequest; 这里幂等再调
	// 一次, 兼容直接复用 injectRequestContext 的单元测试用例。
	return ensureTraceparentOnRequest(req)
}

func (r *PluginRouter) getOrCreateProxy(key string, target *url.URL) *httputil.ReverseProxy {
	if v, ok := r.proxies.Load(key); ok {
		rp, _ := v.(*httputil.ReverseProxy)
		return rp
	}
	// 用 Rewrite 替代已 deprecated 的 Director (Go 1.20+ 推荐写法,1.26 起 Director 标记弃用)。
	//
	// 等价语义对照 (旧 Director 路径 -> 新 Rewrite 路径):
	//   - NewSingleHostReverseProxy 默认 Director 路由到 target.Scheme/Host + 拼接 BasePath
	//     -> pr.SetURL(target) 完成同样的路由动作。
	//   - 原代码额外的 req.Host = target.Host (覆盖 SingleHost "不重写 Host" 的默认行为)
	//     -> pr.SetURL(target) 默认就重写 Out.Host 为 target.Host (见 SetURL godoc),
	//        因此无需再显式赋值; 语义完全一致。
	//   - Director 模式下 ReverseProxy 自动追加 X-Forwarded-For/Host/Proto
	//     -> Rewrite 模式默认会移除入站 X-Forwarded-* 头, 需先把 inbound XFF 拷给 outbound
	//        再调 SetXForwarded, 等价于"追加 client IP 到现有 XFF"行为
	//        (Go 官方文档推荐写法, 见 SetXForwarded godoc)。
	//
	// SSRF 风险: target 来自 plugin manifest 注册时 inst.HTTPAddr, 该地址在
	// manager_spawn.go 调用 validateLoopbackAddr 强制校验为 loopback (127.0.0.1/::1/localhost),
	// 因此这里的 target 不可能指向任意外网/内网主机。
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Header["X-Forwarded-For"] = pr.In.Header["X-Forwarded-For"]
			pr.SetXForwarded()
		},
	}
	actual, _ := r.proxies.LoadOrStore(key, proxy)
	rp, _ := actual.(*httputil.ReverseProxy)
	return rp
}

// hop-by-hop 头列表来自 RFC 7230 §6.1。
var hopByHopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// authPluginHeaderNames 列出 host 单向写入、绝不允许客户端伪造的鉴权类
// X-Plugin-* header。HeaderPluginRequestID 不在列表中: 它是日志关联标识,
// 故意保留前端 / 网关透传语义 (见 injectRequestContext 的注释)。
var authPluginHeaderNames = []string{
	HeaderPluginUserID,
	HeaderPluginUserRole,
	HeaderPluginAPIKeyID,
	HeaderPluginClientIP,
	HeaderPluginName,
}

// stripInboundAuthPluginHeaders 清除任何客户端伪造的鉴权类 X-Plugin-* header。
// 必须在 host 重新写入受信任值之前调用, 包括 AuthTypeNone 路径。
func stripInboundAuthPluginHeaders(h http.Header) {
	for _, name := range authPluginHeaderNames {
		h.Del(name)
	}
}

func stripHopByHopHeaders(h http.Header) {
	if connection := h.Get("Connection"); connection != "" {
		// Connection 头中列出的字段也属于 hop-by-hop。
		for _, f := range strings.Split(connection, ",") {
			if name := strings.TrimSpace(f); name != "" {
				h.Del(name)
			}
		}
	}
	for _, name := range hopByHopHeaders {
		h.Del(name)
	}
}
