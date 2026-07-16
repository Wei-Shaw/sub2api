package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

// ErrAuditLogNotFound 审计日志不存在。
var ErrAuditLogNotFound = infraerrors.NotFound("AUDIT_LOG_NOT_FOUND", "audit log not found")

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

// AuditLog 一条管理面操作审计记录。
type AuditLog struct {
	ID               int64          `json:"id"`
	CreatedAt        time.Time      `json:"created_at"`
	ActorUserID      *int64         `json:"actor_user_id,omitempty"`
	ActorEmail       string         `json:"actor_email"`
	ActorRole        string         `json:"actor_role"`
	AuthMethod       string         `json:"auth_method"`
	CredentialMasked string         `json:"credential_masked"`
	Action           string         `json:"action"`
	Method           string         `json:"method"`
	Path             string         `json:"path"`
	RequestID        string         `json:"request_id"`
	ClientIP         string         `json:"client_ip"`
	UserAgent        string         `json:"user_agent"`
	RequestBody      string         `json:"request_body,omitempty"`
	StatusCode       int            `json:"status_code"`
	LatencyMs        int64          `json:"latency_ms"`
	Extra            map[string]any `json:"extra,omitempty"`
}

// AuditLogFilter 审计日志列表查询条件。
type AuditLogFilter struct {
	Page     int
	PageSize int

	StartTime   *time.Time
	EndTime     *time.Time
	ActorUserID *int64
	ActorEmail  string
	AuthMethod  string
	Action      string
	Method      string
	ClientIP    string
	// Success: nil 全部；true 仅 2xx/3xx；false 仅 >=400。
	Success *bool
	// Query 对 path / action / actor_email 做模糊匹配。
	Query string
}

// AuditLogList 分页结果。
type AuditLogList struct {
	Logs     []*AuditLog
	Total    int
	Page     int
	PageSize int
}

// AuditLogRepository 审计日志持久化端口。
// 注意：接口刻意不提供单条删除能力——审计日志只允许追加与全量清空。
type AuditLogRepository interface {
	BatchInsert(ctx context.Context, logs []*AuditLog) (int64, error)
	// Insert 同步写入单条（用于清空留痕等必须落库的记录）。
	Insert(ctx context.Context, log *AuditLog) error
	List(ctx context.Context, filter *AuditLogFilter) (*AuditLogList, error)
	GetByID(ctx context.Context, id int64) (*AuditLog, error)
	Count(ctx context.Context) (int64, error)
	// TruncateAll 全量清空（TRUNCATE），返回前需调用方自行 Count 记录行数。
	TruncateAll(ctx context.Context) error
	// DeleteBefore 按保留期批量删除，返回本批删除行数（幂等，可多实例并发）。
	DeleteBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error)
}

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
