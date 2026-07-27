package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logredact"
	"github.com/Wei-Shaw/sub2api/internal/port/audit"
)

// Audit-log BC types/errors live in domain; re-exported here for existing call sites.
type AuditLog = domain.AuditLog
type AuditLogFilter = domain.AuditLogFilter
type AuditLogList = domain.AuditLogList
type AuditLogRepository = audit.AuditLogRepository

var ErrAuditLogNotFound = domain.ErrAuditLogNotFound

// 审计日志相关常量。
const (
	// AuditAuthMethodJWT / AuditAuthMethodAdminAPIKey 与 auth 中间件写入的 auth_method 对齐。
	AuditAuthMethodJWT         = "jwt"
	AuditAuthMethodAdminAPIKey = "admin_api_key"

	// auditRequestBodyMaxBytes 请求体脱敏后入库的最大长度（字节），超出截断。
	auditRequestBodyMaxBytes = 16 * 1024
	// AuditRequestBodyCaptureLimit 请求体参与脱敏解析的原始大小上限（字节）。
	// 审计中间件按此上限截断读取，超出的请求体仅记录占位符不解析。
	AuditRequestBodyCaptureLimit = 256 * 1024
)

// 内置审计动作名（认证/安全事件与特殊操作使用固定值，普通请求由路由自动推导）。
const (
	AuditActionLogin                  = "auth.login"
	AuditActionLogin2FA               = "auth.login.2fa"
	AuditActionRegister               = "auth.register"
	AuditActionTokenRefresh           = "auth.token.refresh"
	AuditActionSessionBindingMismatch = "auth.session_binding.mismatch"
	AuditActionStepUpVerify           = "auth.step_up.verify"
	AuditActionAuditLogClear          = "admin.audit_log.clear"
)

// auditNormalizeBodyKey 归一化键名：小写并去除分隔符，
// 使 private_key / privateKey / privatekey / api-v3-key 等写法共享同一判定，
// 避免子串清单假设 snake_case 而漏掉支付渠道等无分隔符风格的密钥字段。
func auditNormalizeBodyKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range strings.ToLower(strings.TrimSpace(key)) {
		switch r {
		case '_', '-', '.', ' ':
			continue
		default:
			_, _ = b.WriteRune(r)
		}
	}
	return b.String()
}

// auditBodySensitiveExactKeys 请求体脱敏的精确匹配键（归一化后）。
// 除内置清单外，程序化并入两份权威敏感表以防清单漂移：
//   - SensitiveCredentialKeys：账号 credentials 的敏感子键（session_key / service_account_json 等）
//   - providerSensitiveConfigFields：支付渠道密钥字段（pkey / privatekey / apiv3key 等）
var auditBodySensitiveExactKeys = func() map[string]struct{} {
	builtin := []string{
		"code", "codes", "pin", "cvv",
		"authorization", "cookie", "x-api-key",
		"key",
		// 字符串值内嵌完整凭证的字段：
		// proxy_key 为 protocol|host|port|username|password 拼接，
		// custom_key 为用户自设的平台 API Key 明文。
		"proxy_key", "custom_key",
	}
	set := make(map[string]struct{}, len(builtin)+len(SensitiveCredentialKeys)+16)
	for _, k := range builtin {
		set[auditNormalizeBodyKey(k)] = struct{}{}
	}
	for _, k := range SensitiveCredentialKeys {
		set[auditNormalizeBodyKey(k)] = struct{}{}
	}
	for _, fields := range providerSensitiveConfigFields {
		for k := range fields {
			set[auditNormalizeBodyKey(k)] = struct{}{}
		}
	}
	return set
}()

// auditBodySensitiveSubstrings 请求体脱敏的包含匹配子串（对归一化后的键名比对）。
// 命中任一子串即整体擦除该键的值（例如 new_password / secret_access_key / temp_token）。
var auditBodySensitiveSubstrings = []string{
	"password", "passwd", "secret", "token",
	"apikey", "accesskey", "privatekey",
	"otp", "credentialvalue",
	"sessionkey", "serviceaccount",
}

func isAuditSensitiveBodyKey(key string) bool {
	k := auditNormalizeBodyKey(key)
	if _, ok := auditBodySensitiveExactKeys[k]; ok {
		return true
	}
	for _, sub := range auditBodySensitiveSubstrings {
		if strings.Contains(k, sub) {
			return true
		}
	}
	return false
}

const auditRedactedPlaceholder = "***"

// RedactAuditBody 对请求体做审计入库前的脱敏：
//   - JSON：递归擦除敏感键的值（保留结构，base_url 等非敏感字段可见以便追责）
//   - 非 JSON：返回占位说明
//   - 超长：截断并附截断标记
func RedactAuditBody(raw []byte, contentType string) string {
	if len(raw) == 0 {
		return ""
	}
	if len(raw) > AuditRequestBodyCaptureLimit {
		// raw 可能已被中间件按上限截断，实际请求体只会更大，不报具体字节数。
		return "<body omitted: exceeds " + strconv.Itoa(AuditRequestBodyCaptureLimit) + " bytes>"
	}
	ct := strings.ToLower(contentType)
	if !strings.Contains(ct, "json") || !json.Valid(raw) {
		// 表单等非 JSON 内容走文本兜底脱敏后仍可能含敏感信息，直接不入库。
		return "<non-json body omitted: " + strconv.Itoa(len(raw)) + " bytes, content-type=" + strings.TrimSpace(contentType) + ">"
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "<unparsable body omitted>"
	}
	redacted := redactAuditValue(value, 0)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return "<redacted>"
	}
	out := string(encoded)
	if len(out) > auditRequestBodyMaxBytes {
		out = out[:auditRequestBodyMaxBytes] + "...<truncated>"
	}
	return out
}

const auditRedactMaxDepth = 24

func redactAuditValue(value any, depth int) any {
	if depth > auditRedactMaxDepth {
		return "<depth limit exceeded>"
	}
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			if isAuditSensitiveBodyKey(k) {
				out[k] = auditRedactedPlaceholder
				continue
			}
			out[k] = redactAuditValue(item, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactAuditValue(item, depth+1)
		}
		return out
	default:
		return value
	}
}

// MaskAuditCredential 对请求头中的凭证做首尾保留掩码：
// 保留前 6 位与后 4 位，中间以 **** 表示；过短的凭证整体掩码。
func MaskAuditCredential(credential string) string {
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return ""
	}
	if len(credential) <= 14 {
		return "****"
	}
	return credential[:6] + "****" + credential[len(credential)-4:]
}

// RedactAuditQuery 对 URL query 做轻量脱敏后返回。
func RedactAuditQuery(rawQuery string) string {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return ""
	}
	return logredact.RedactText(rawQuery, "api_key", "apikey", "token", "secret", "key")
}
