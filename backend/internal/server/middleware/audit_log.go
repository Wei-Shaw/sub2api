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
REDACTED

// SetAuditActor 允许 handler 在认证上下文缺失时（如登录接口）补充操作者身份。
func SetAuditActor(c *gin.Context, userID int64, email string) {
	if userID > 0 {
		c.Set(auditCtxKeyActorID, userID)
REDACTED
	if email != "" {
		c.Set(auditCtxKeyActorEmail, email)
REDACTED
REDACTED

// SkipAudit 跳过当前请求的审计记录。
func SkipAudit(c *gin.Context) {
	c.Set(auditCtxKeySkip, true)
REDACTED

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
REDACTED

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
REDACTED

// NewAuditLogMiddleware 创建审计中间件。
// 记录范围：变更类请求（POST/PUT/PATCH/DELETE）+ 白名单内的敏感 GET 读取。
// 该中间件应挂在认证中间件之前，以便同时记录认证失败的尝试。
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
		REDACTED
		case "GET":
			if v, ok := auditSensitiveReads[routeKey]; ok {
				record = true
				action = v
		REDACTED
	REDACTED
		if !record {
			c.Next()
			return
	REDACTED

		// 捕获请求体（读出后回填，避免影响后续 ShouldBindJSON）。
		var bodyRedacted string
		if c.Request.Body != nil && c.Request.Method != "GET" {
			raw, err := io.ReadAll(c.Request.Body)
			if err == nil {
				c.Request.Body = io.NopCloser(bytes.NewReader(raw))
				bodyRedacted = service.RedactAuditBody(raw, c.GetHeader("Content-Type"))
		REDACTED
	REDACTED

		start := time.Now()
		c.Next()

		if c.GetBool(auditCtxKeySkip) {
			return
	REDACTED

		status := c.Writer.Status()
		// token 刷新成功属于高频常规操作，只记录失败（潜在攻击信号）。
		if routeKey == "POST /api/v1/auth/refresh" && status < 400 {
			return
	REDACTED

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
	REDACTED
		if entry.Path == "" {
			entry.Path = c.Request.URL.Path
	REDACTED
		if entry.Action == "" {
			entry.Action = deriveAuditAction(c.Request.Method, entry.Path)
	REDACTED
		if v, ok := c.Get(auditCtxKeyAction); ok {
			if s, ok := v.(string); ok && s != "" {
				entry.Action = s
		REDACTED
	REDACTED
		if requestID, ok := c.Request.Context().Value(ctxkey.RequestID).(string); ok {
			entry.RequestID = requestID
	REDACTED

		// 操作者身份：优先取认证中间件写入的上下文，其次取 handler 覆写（登录等场景）。
		if subject, ok := GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
			uid := subject.UserID
			entry.ActorUserID = &uid
	REDACTED
		if role, ok := GetUserRoleFromContext(c); ok {
			entry.ActorRole = role
	REDACTED
		entry.ActorEmail = c.GetString(ContextKeyAuthEmail)
		entry.AuthMethod = c.GetString("auth_method")
		if entry.AuthMethod == "" && entry.ActorUserID != nil {
			entry.AuthMethod = service.AuditAuthMethodJWT
	REDACTED
		if v, ok := c.Get(auditCtxKeyActorID); ok {
			if id, ok := v.(int64); ok && id > 0 {
				entry.ActorUserID = &id
		REDACTED
	REDACTED
		if v, ok := c.Get(auditCtxKeyActorEmail); ok {
			if s, ok := v.(string); ok && s != "" {
				entry.ActorEmail = s
		REDACTED
	REDACTED

		// 请求头凭证掩码（仅保留首尾）。
		entry.CredentialMasked = MaskedRequestCredential(c)

		extra := map[string]any{REDACTED
		if len(c.Params) > 0 {
			params := make(map[string]string, len(c.Params))
			for _, p := range c.Params {
				params[p.Key] = p.Value
		REDACTED
			extra["params"] = params
	REDACTED
		if q := service.RedactAuditQuery(c.Request.URL.RawQuery); q != "" {
			extra["query"] = q
	REDACTED
		if len(extra) > 0 {
			entry.Extra = extra
	REDACTED

		auditService.Record(entry)
REDACTED)
REDACTED

// MaskedRequestCredential 提取请求头中的凭证并做首尾掩码。
func MaskedRequestCredential(c *gin.Context) string {
	if apiKey := strings.TrimSpace(c.GetHeader("x-api-key")); apiKey != "" {
		return "x-api-key " + service.MaskAuditCredential(apiKey)
REDACTED
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		return ""
REDACTED
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 {
		return parts[0] + " " + service.MaskAuditCredential(strings.TrimSpace(parts[1]))
REDACTED
	return service.MaskAuditCredential(authHeader)
REDACTED

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
	REDACTED
		parts = append(parts, strings.ReplaceAll(seg, "-", "_"))
REDACTED
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
REDACTED
	if len(parts) == 0 {
		return verb
REDACTED
	return strings.Join(parts, ".") + "." + verb
REDACTED
