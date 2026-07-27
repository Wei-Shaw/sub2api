package domain

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ErrAuditLogNotFound 审计日志不存在。
var ErrAuditLogNotFound = infraerrors.NotFound("AUDIT_LOG_NOT_FOUND", "audit log not found")

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
