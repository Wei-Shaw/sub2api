package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/inbox"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// 本文件把 inbox 模块所需的"应用侧"依赖（依赖 service / middleware）以 wire provider
// 形式提供。这些 provider 放在 server 包，避免 inbox 反向依赖 service 形成 import 环。

// attrCacheTTL 用户属性缓存时长，缓解广播 fan-out 时的重复查询。
const attrCacheTTL = 30 * time.Second

// ProvideInboxConfig 提供信箱服务参数（零值回退默认：30 天保留、分页等）。
func ProvideInboxConfig() inbox.Config {
	return inbox.Config{}
}

// ProvideInboxMetrics 提供信箱可观测性打点：进程内原子计数器实现（零依赖、开销可
// 忽略）。累计发布/重试/catchup/ack/WS 等事件，可通过 Snapshot 读取，后续接
// Prometheus 导出器时直接读取快照即可，无需改业务打点。
func ProvideInboxMetrics() inbox.Metrics {
	return inbox.NewCountingMetrics()
}

// ProvideInboxUserIDFunc 提供从 gin 上下文取当前用户 id 的适配器。
func ProvideInboxUserIDFunc() inbox.UserIDFunc {
	return func(c *gin.Context) (int64, bool) {
		subject, ok := middleware2.GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			return 0, false
		}
		return subject.UserID, true
	}
}

// ProvideInboxOriginChecker 提供 WS 握手的 Origin 校验。
//
// 这里恒放行（不做 Origin 白名单）：inbox WS 用 query ?token= / Sec-WebSocket-Protocol 里的
// JWT 显式鉴权，而非浏览器自动携带的 cookie，跨站脚本无法窃取该 token，因此不存在跨站
// WebSocket 劫持(CSWSH)风险，Origin 校验并无额外收益，真正的准入由 WSAuthenticator 完成。
//
// 注意：必须返回非 nil 的恒 true 校验器，不能返回 nil —— gorilla 在 CheckOrigin==nil 时会
// 退化为"严格同源"校验，导致跨域前端（不同域/端口 + 反向代理，Origin≠Host）握手被判 403。
func ProvideInboxOriginChecker() inbox.OriginChecker {
	return func(*http.Request) bool { return true }
}

// ProvideInboxAttributeProvider 用 UserService 构造用户属性提供者（供广播 targeting
// 求值），外层包一层短 TTL 缓存。
func ProvideInboxAttributeProvider(userService *service.UserService) inbox.AttributeProvider {
	base := inbox.AttributeProviderFunc(func(ctx context.Context, userID int64) (map[string]any, error) {
		u, err := userService.GetByID(ctx, userID)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"id":     u.ID,
			"role":   u.Role,
			"status": u.Status,
		}, nil
	})
	return inbox.NewCachingAttributeProvider(base, attrCacheTTL)
}

// ProvideInboxWSAuthenticator 用 Auth/User 服务构造 WS 握手鉴权器。
func ProvideInboxWSAuthenticator(authService *service.AuthService, userService *service.UserService) inbox.WSAuthenticator {
	return &inboxWSAuth{auth: authService, users: userService}
}

// inboxWSAuth 实现 inbox.WSAuthenticator：从 WS 握手请求解析并校验 JWT。
type inboxWSAuth struct {
	auth  *service.AuthService
	users *service.UserService
}

// Authenticate 从 query ?token= 或 Sec-WebSocket-Protocol 提取 JWT 并校验，返回 userID。
func (a *inboxWSAuth) Authenticate(r *http.Request) (int64, error) {
	token := extractWSToken(r)
	if token == "" {
		return 0, errors.New("missing token")
	}
	claims, err := a.auth.ValidateToken(token)
	if err != nil {
		return 0, err
	}
	if claims.UserID <= 0 {
		return 0, errors.New("invalid token subject")
	}
	u, err := a.users.GetByID(r.Context(), claims.UserID)
	if err != nil {
		return 0, err
	}
	if !u.IsActive() {
		return 0, errors.New("user inactive")
	}
	// token 版本校验：密码变更等会递增 TokenVersion 使旧 token 失效。
	if u.TokenVersion != claims.TokenVersion {
		return 0, errors.New("token version mismatch")
	}
	return claims.UserID, nil
}

// extractWSToken 从握手请求提取 token：优先 query ?token=，其次 Sec-WebSocket-Protocol
// 中形如 "access_token,<jwt>" 的第二段。
func extractWSToken(r *http.Request) string {
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	proto := r.Header.Get("Sec-WebSocket-Protocol")
	if proto == "" {
		return ""
	}
	parts := splitAndTrim(proto)
	if len(parts) == 2 && parts[0] == "access_token" {
		return parts[1]
	}
	return ""
}

// splitAndTrim 按逗号切分并去除空白。
func splitAndTrim(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			seg := s[start:i]
			// trim spaces
			for len(seg) > 0 && (seg[0] == ' ' || seg[0] == '\t') {
				seg = seg[1:]
			}
			for len(seg) > 0 && (seg[len(seg)-1] == ' ' || seg[len(seg)-1] == '\t') {
				seg = seg[:len(seg)-1]
			}
			if seg != "" {
				out = append(out, seg)
			}
			start = i + 1
		}
	}
	return out
}
