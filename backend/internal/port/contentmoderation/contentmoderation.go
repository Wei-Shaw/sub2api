// Package contentmoderation contains the port interface for the
// content-moderation bounded context's persistence contract: the audit-log
// store for moderation decisions. The Redis hash-cache port already lives in
// internal/port/cache; this package only owns the SQL repository contract.
// DTO/value types live in internal/domain.
package contentmoderation

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ContentModerationRepository persists moderation audit logs.
type ContentModerationRepository interface {
	CreateLog(ctx context.Context, log *domain.ContentModerationLog) error
	ListLogs(ctx context.Context, filter domain.ContentModerationLogFilter) ([]domain.ContentModerationLog, *pagination.PaginationResult, error)
	// CountFlaggedByUserSince 统计窗口内计入封号的违规次数（排除 hash_block；
	// excludeCyberPolicy 为 true 时额外排除 cyber_policy 行）。
	CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time, excludeCyberPolicy bool) (int, error)
	CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*domain.ContentModerationCleanupResult, error)
	// UpdateLogEmailSent 回写邮件发送结果（F7：CreateLog 先行后补 EmailSent）。
	UpdateLogEmailSent(ctx context.Context, id int64, sent bool) error
}
