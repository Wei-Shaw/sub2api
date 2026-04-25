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

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
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
// 在转发前会清除 hop-by-hop header,并补充 X-Plugin-User-* 头便于插件识别调用者。
func (r *PluginRouter) proxyTo(w http.ResponseWriter, req *http.Request, authCtx *gin.Context, entry *RouteEntry) {
	target, err := url.Parse(entry.ProxyURL)
	if err != nil {
		http.Error(w, "invalid plugin upstream", http.StatusBadGateway)
		return
	}

	proxy := r.getOrCreateProxy(entry.ProxyURL, target)

	// 注入用户上下文头,使插件不必再次解析 JWT/APIKey。
	if authCtx != nil {
		if subject, ok := middleware.GetAuthSubjectFromContext(authCtx); ok && subject.UserID > 0 {
			req.Header.Set("X-Plugin-User-ID", strconv.FormatInt(subject.UserID, 10))
		}
		if role, ok := middleware.GetUserRoleFromContext(authCtx); ok && role != "" {
			req.Header.Set("X-Plugin-User-Role", role)
		}
	}
	req.Header.Set("X-Plugin-Name", entry.PluginName)

	// 移除 hop-by-hop header,避免误传给插件 HTTP server。
	stripHopByHopHeaders(req.Header)

	proxy.ServeHTTP(w, req)
}

func (r *PluginRouter) getOrCreateProxy(key string, target *url.URL) *httputil.ReverseProxy {
	if v, ok := r.proxies.Load(key); ok {
		return v.(*httputil.ReverseProxy)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// 覆写 Director,设置 req.Host 为目标 host,确保上游 server 正确处理。
	original := proxy.Director
	proxy.Director = func(req *http.Request) {
		original(req)
		req.Host = target.Host
	}
	actual, _ := r.proxies.LoadOrStore(key, proxy)
	return actual.(*httputil.ReverseProxy)
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
