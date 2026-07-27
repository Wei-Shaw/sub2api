// Package audit contains the port interface for the audit-log bounded
// context: append-only management-plane audit records backed by raw SQL.
// The contract references only domain types so the repository layer can
// implement it without importing internal/service. The service package keeps
// a type alias to the interface so existing call sites and test stubs continue
// to satisfy the contract.
package audit

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// AuditLogRepository 审计日志持久化端口。
// 注意：接口刻意不提供单条删除能力——审计日志只允许追加与全量清空。
type AuditLogRepository interface {
	BatchInsert(ctx context.Context, logs []*domain.AuditLog) (int64, error)
	// Insert 同步写入单条（用于清空留痕等必须落库的记录）。
	Insert(ctx context.Context, log *domain.AuditLog) error
	List(ctx context.Context, filter *domain.AuditLogFilter) (*domain.AuditLogList, error)
	GetByID(ctx context.Context, id int64) (*domain.AuditLog, error)
	Count(ctx context.Context) (int64, error)
	// TruncateAll 全量清空（TRUNCATE），返回前需调用方自行 Count 记录行数。
	TruncateAll(ctx context.Context) error
	// DeleteBefore 按保留期批量删除，返回本批删除行数（幂等，可多实例并发）。
	DeleteBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error)
}
