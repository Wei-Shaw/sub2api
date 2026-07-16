package middleware

import (
	"bytes"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AuditLogMiddleware 管理面操作审计中间件类型（用于 wire 注入区分）。
type AuditLogMiddleware gin.HandlerFunc

// 审计相关 gin context 覆写键：handler / 认证中间件可通过这些键补充审计信息。
const (
	auditCtxKeyAction     = "audit_action"
	auditCtxKeyActorID    = "audit_actor_id"
	auditCtxKeyActorEmail = "audit_actor_email"
	auditCtxKeySkip       = "audit_skip"
	// ContextKeyAuthEmail 认证中间件写入的用户邮箱（审计用）。
	ContextKeyAuthEmail = "auth_email"
	// ContextKeySessionID 认证中间件写入的会话 ID（refresh token family）。
	ContextKeySessionID = "session_id"
)

// SetAuditAction 允许 handler / 中间件为当前请求指定审计动作名（覆盖自动推导）。
func SetAuditAction(c *gin.Context, action string) {
	c.Set(auditCtxKeyAction, action)
}

// SetAuditActor 允许 handler 在认证上下文缺失时（如登录接口）补充操作者身份。
func SetAuditActor(c *gin.Context, userID int64, email string) {
	if userID > 0 {
		c.Set(auditCtxKeyActorID, userID)
	}
	if email != "" {
		c.Set(auditCtxKeyActorEmail, email)
	}
}

// SkipAudit 跳过当前请求的审计记录。
func SkipAudit(c *gin.Context) {
	c.Set(auditCtxKeySkip, true)
}

// auditSensitiveReads 需要审计的敏感 GET 读取（method+FullPath → 动作名）。
var auditSensitiveReads = map[string]string{
	"GET /api/v1/admin/accounts/data":             "admin.accounts.export",
	"GET /api/v1/admin/proxies/data":              "admin.proxies.export",
	"GET /api/v1/admin/redeem-codes/export":       "admin.redeem_codes.export",
	"GET /api/v1/admin/backups/:id/download-url":  "admin.backups.download",
	"GET /api/v1/admin/settings/admin-api-key":    "admin.admin_api_key.read",
	"GET /api/v1/admin/users/:id/api-keys":        "admin.users.api_keys.read",
	"GET /api/v1/admin/groups/:id/api-keys":       "admin.groups.api_keys.read",
	"GET /api/v1/admin/backups/s3-config":         "admin.backups.s3_config.read",
	"GET /api/v1/admin/data-management/s3/config": "admin.data_management.s3_config.read",
}

// auditActionOverrides 变更类请求的动作名精确映射（未命中时自动推导）。
var auditActionOverrides = map[string]string{
	"POST /api/v1/auth/login":                              service.AuditActionLogin,
	"POST /api/v1/auth/login/2fa":                          service.AuditActionLogin2FA,
	"POST /api/v1/auth/register":                           service.AuditActionRegister,
	"POST /api/v1/auth/refresh":                            service.AuditActionTokenRefresh,
	"POST /api/v1/user/totp/step-up":                       service.AuditActionStepUpVerify,
	"POST /api/v1/admin/audit-logs/clear":                  service.AuditActionAuditLogClear,
	"POST /api/v1/admin/accounts/data":                     "admin.accounts.import",
	"POST /api/v1/admin/backups":                           "admin.backups.create",
	"POST /api/v1/admin/backups/:id/restore":               "admin.backups.restore",
	"DELETE /api/v1/admin/backups/:id":                     "admin.backups.delete",
	"PUT /api/v1/admin/backups/s3-config":                  "admin.backups.s3_config.update",
	"POST /api/v1/admin/settings/admin-api-key/regenerate": "admin.admin_api_key.regenerate",
	"DELETE /api/v1/admin/settings/admin-api-key":          "admin.admin_api_key.delete",
}

// auditBodyOmittedRoutes 请求体几乎整体由凭证构成的路由（如整块粘贴 auth JSON 的导入接口）。
// 这类 body 的凭证内嵌在普通字符串值里，键级脱敏无法覆盖，整体不入库。
var auditBodyOmittedRoutes = map[string]struct{}{
	"POST /api/v1/admin/accounts/import/codex-session": {},
}

// NewAuditLogMiddleware 创建审计中间件。
// 记录范围：变更类请求（POST/PUT/PATCH/DELETE）+ 白名单内的敏感 GET 读取。
// 挂载位置：admin / user / admin-payment 组挂在各自认证中间件之后（只审计已认证请求，
// 未过认证的 401/403 不入库）；auth 组（登录/注册/刷新）无前置认证，天然记录失败尝试。
func NewAuditLogMiddleware(auditService *service.AuditLogService) AuditLogMiddleware {
	return AuditLogMiddleware(func(c *gin.Context) {
		routeKey := c.Request.Method + " " + c.FullPath()

		record := false
		action := ""
		switch c.Request.Method {
		case "POST", "PUT", "PATCH", "DELETE":
			record = true
			if v, ok := auditActionOverrides[routeKey]; ok {
				action = v
			}
		case "GET":
			if v, ok := auditSensitiveReads[routeKey]; ok {
				record = true
				action = v
			}
		}
		if !record {
			c.Next()
			return
		}

		// 捕获请求体（读出后回填，避免影响后续 ShouldBindJSON）。
		// 只读取脱敏解析上限内的字节，超出部分与已读部分拼接回填，
		// 避免大体积导入请求被完整复制进内存两次。
		var bodyRedacted string
		if _, omit := auditBodyOmittedRoutes[routeKey]; omit {
			bodyRedacted = "<credential-bearing body omitted>"
		} else if c.Request.Body != nil && c.Request.Method != "GET" {
			orig := c.Request.Body
			raw, err := io.ReadAll(io.LimitReader(orig, service.AuditRequestBodyCaptureLimit+1))
			if err == nil {
				c.Request.Body = &restoredBody{
					Reader: io.MultiReader(bytes.NewReader(raw), orig),
					closer: orig,
				}
				bodyRedacted = service.RedactAuditBody(raw, c.GetHeader("Content-Type"))
			}
		}

		start := time.Now()
		c.Next()

		if c.GetBool(auditCtxKeySkip) {
			return
		}

		status := c.Writer.Status()
		// token 刷新成功属于高频常规操作，只记录失败（潜在攻击信号）。
		if routeKey == "POST /api/v1/auth/refresh" && status < 400 {
			return
		}

		entry := &service.AuditLog{
			CreatedAt:   time.Now().UTC(),
			Action:      action,
			Method:      c.Request.Method,
			Path:        c.FullPath(),
			ClientIP:    ip.GetTrustedClientIP(c),
			UserAgent:   c.Request.UserAgent(),
			RequestBody: bodyRedacted,
			StatusCode:  status,
			LatencyMs:   time.Since(start).Milliseconds(),
		}
		if entry.Path == "" {
			entry.Path = c.Request.URL.Path
		}
		if entry.Action == "" {
			entry.Action = deriveAuditAction(c.Request.Method, entry.Path)
		}
		if v, ok := c.Get(auditCtxKeyAction); ok {
			if s, ok := v.(string); ok && s != "" {
				entry.Action = s
			}
		}
		if requestID, ok := c.Request.Context().Value(ctxkey.RequestID).(string); ok {
			entry.RequestID = requestID
		}

		// 操作者身份：优先取认证中间件写入的上下文，其次取 handler 覆写（登录等场景）。
		if subject, ok := GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
			uid := subject.UserID
			entry.ActorUserID = &uid
		}
		if role, ok := GetUserRoleFromContext(c); ok {
			entry.ActorRole = role
		}
		entry.ActorEmail = c.GetString(ContextKeyAuthEmail)
		entry.AuthMethod = c.GetString("auth_method")
		if entry.AuthMethod == "" && entry.ActorUserID != nil {
			entry.AuthMethod = service.AuditAuthMethodJWT
		}
		if v, ok := c.Get(auditCtxKeyActorID); ok {
			if id, ok := v.(int64); ok && id > 0 {
				entry.ActorUserID = &id
			}
		}
		if v, ok := c.Get(auditCtxKeyActorEmail); ok {
			if s, ok := v.(string); ok && s != "" {
				entry.ActorEmail = s
			}
		}

		// 请求头凭证掩码（仅保留首尾）。
		entry.CredentialMasked = MaskedRequestCredential(c)

		extra := map[string]any{}
		if len(c.Params) > 0 {
			params := make(map[string]string, len(c.Params))
			for _, p := range c.Params {
				params[p.Key] = p.Value
			}
			extra["params"] = params
		}
		if q := service.RedactAuditQuery(c.Request.URL.RawQuery); q != "" {
			extra["query"] = q
		}
		if len(extra) > 0 {
			entry.Extra = extra
		}

		auditService.Record(entry)
	})
}

// restoredBody 把审计中间件按上限读出的前缀与未读完的原始 body 拼接回填，
// 保证 handler 读到完整请求体；Close 委托给原始 body。
type restoredBody struct {
	io.Reader
	closer io.Closer
}

func (b *restoredBody) Close() error { return b.closer.Close() }

// MaskedRequestCredential 提取请求头中的凭证并做首尾掩码。
func MaskedRequestCredential(c *gin.Context) string {
	if apiKey := strings.TrimSpace(c.GetHeader("x-api-key")); apiKey != "" {
		return "x-api-key " + service.MaskAuditCredential(apiKey)
	}
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 {
		return parts[0] + " " + service.MaskAuditCredential(strings.TrimSpace(parts[1]))
	}
	return service.MaskAuditCredential(authHeader)
}

// deriveAuditAction 由 method + 路由模板自动推导动作名，
// 例：PUT /api/v1/admin/accounts/:id → admin.accounts.update
func deriveAuditAction(method, fullPath string) string {
	path := strings.TrimPrefix(fullPath, "/api/v1/")
	path = strings.Trim(path, "/")
	segs := strings.Split(path, "/")
	parts := make([]string, 0, len(segs))
	for _, seg := range segs {
		if seg == "" || strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "*") {
			continue
		}
		parts = append(parts, strings.ReplaceAll(seg, "-", "_"))
	}
	verb := ""
	switch method {
	case "POST":
		verb = "create"
	case "PUT", "PATCH":
		verb = "update"
	case "DELETE":
		verb = "delete"
	case "GET":
		verb = "read"
	default:
		verb = strings.ToLower(method)
	}
	if len(parts) == 0 {
		return verb
	}
	return strings.Join(parts, ".") + "." + verb
}
