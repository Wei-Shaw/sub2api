package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SessionBindingContext 全局中间件：将请求的可信客户端 IP 与 User-Agent 注入
// request context，供 token 签发路径（登录 / 刷新 / OAuth 回调）读取并写入会话绑定。
// 必须使用 GetTrustedClientIP（走 trusted_proxies 链），不可信头会导致绑定被伪造绕过。
func SessionBindingContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		binding := &service.SessionBinding{
			IP:        ip.GetTrustedClientIP(c),
			UserAgent: c.Request.UserAgent(),
	REDACTED
		c.Request = c.Request.WithContext(service.WithSessionBinding(c.Request.Context(), binding))
		c.Next()
REDACTED
REDACTED

// currentSessionBindingHash 计算当前请求的会话指纹哈希。
func currentSessionBindingHash(c *gin.Context) string {
	binding := &service.SessionBinding{
		IP:        ip.GetTrustedClientIP(c),
		UserAgent: c.Request.UserAgent(),
REDACTED
	return binding.Hash()
REDACTED

// enforceSessionBinding 校验 access token 的会话指纹（IP/UA 绑定）。
// 指纹不匹配时：撤销该会话家族的所有 refresh token、写入审计安全事件、返回 401。
// 返回 false 表示请求已被中断。
//
// 兼容性：claims.BindingHash 为空（功能上线前签发的旧 token）时放行，
// 该会话在下一次 refresh 轮转时会自动获得绑定。
func enforceSessionBinding(
	c *gin.Context,
	authService *service.AuthService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
	claims *service.JWTClaims,
) bool {
	if settingService == nil || !settingService.IsSessionBindingEnabled(c.Request.Context()) {
		return true
REDACTED
	if claims == nil || claims.BindingHash == "" {
		return true
REDACTED
	current := currentSessionBindingHash(c)
	if current == "" || current == claims.BindingHash {
		return true
REDACTED

	if authService != nil {
		_ = authService.RevokeSessionFamily(c.Request.Context(), claims.SessionID)
REDACTED
	if auditService != nil {
		uid := claims.UserID
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
	REDACTED
		auditService.Record(&service.AuditLog{
			ActorUserID: &uid,
			ActorEmail:  claims.Email,
			ActorRole:   claims.Role,
			AuthMethod:  service.AuditAuthMethodJWT,
			Action:      service.AuditActionSessionBindingMismatch,
			Method:      c.Request.Method,
			Path:        path,
			ClientIP:    ip.GetTrustedClientIP(c),
			UserAgent:   c.Request.UserAgent(),
			StatusCode:  401,
	REDACTED)
REDACTED
	AbortWithError(c, 401, "SESSION_BINDING_MISMATCH", "Session network fingerprint changed, please login again")
	return false
REDACTED
