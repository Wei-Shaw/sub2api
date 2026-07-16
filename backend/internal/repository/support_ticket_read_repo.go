// Package repository — support_ticket_read_repo.go
//
// 工单读游标（support_ticket_reads）的 Repository 实现。
//
// 该 repo 有两类操作：
//
//  1. 写入（MarkTicketRead）— 采用 ent upsert；(ticket_id, user_id) 唯一冲突时更新
//     last_read_at + updated_at。该动作由工单详情端点与 admin 回复端点作为副作用触发，
//     即使失败也不应阻塞主流程（handler 会捕获错误并降级为 warn 日志）。
//
//  2. 聚合（CountUnreadForUser / CountUnreadForAdmin）— 未读工单聚合走原生 SQL LEFT JOIN，
//     语义清晰且能一次查询命中：
//
//     用户视角未读工单：
//     SELECT COUNT(DISTINCT t.id)
//     FROM support_tickets t
//     JOIN support_ticket_replies r ON r.ticket_id = t.id AND r.is_admin = true
//     LEFT JOIN support_ticket_reads rd
//     ON rd.ticket_id = t.id AND rd.user_id = t.user_id
//     WHERE t.user_id = $1
//     AND r.created_at > COALESCE(rd.last_read_at, '1970-01-01'::timestamptz)
//
//     管理员视角未读工单：任何管理员看到的未读集合都相同（"新工单" + "有用户回复"）。
//     同一表结构支持逐 admin 独立游标，语义上每个 admin 各自看到自己没读过的工单：
//     SELECT COUNT(DISTINCT t.id)
//     FROM support_tickets t
//     LEFT JOIN support_ticket_reads rd
//     ON rd.ticket_id = t.id AND rd.user_id = $1
//     LEFT JOIN LATERAL (
//     SELECT MAX(rp.created_at) AS last_user_reply_at
//     FROM support_ticket_replies rp
//     WHERE rp.ticket_id = t.id AND rp.is_admin = false
//     ) ur ON TRUE
//     WHERE t.created_at            > COALESCE(rd.last_read_at, '1970-01-01'::timestamptz)
//     OR ur.last_user_reply_at   > COALESCE(rd.last_read_at, '1970-01-01'::timestamptz)
//
//     用 LATERAL 子查询取每工单最新用户回复时间，避免 self-join 造成的重复行。
//     所有查询都参数化，无 SQL 注入风险。
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/supportticketread"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type supportTicketReadRepository struct {
	client *dbent.Client
	db     *sql.DB
}

// NewSupportTicketReadRepository 构造读游标 Repository。
//
// 两个依赖：
//   - *dbent.Client 用于 upsert（走 ent 的 OnConflictColumns API 更简洁）；
//   - *sql.DB 用于聚合查询（原生 SQL 更容易表达 COALESCE / LATERAL / LEFT JOIN 组合）。
func NewSupportTicketReadRepository(client *dbent.Client, db *sql.DB) service.SupportTicketReadRepository {
	return &supportTicketReadRepository{client: client, db: db}
}

// MarkTicketRead 把 (ticketID, userID) 的 last_read_at 置为 readAt（upsert）。
//
// 使用 ent 生成的 OnConflictColumns + Update 语法，冲突时把 last_read_at / updated_at
// 更新为新值。ent schema 已在 UpdateDefaultUpdatedAt 中挂钩 time.Now，无需手工 SetUpdatedAt。
//
// 幂等：多次调用不会失败；readAt 单调递增由调用方（详情端点）保证。
func (r *supportTicketReadRepository) MarkTicketRead(
	ctx context.Context,
	ticketID, userID int64,
	readAt time.Time,
) error {
	client := clientFromContext(ctx, r.client)
	return client.SupportTicketRead.Create().
		SetTicketID(ticketID).
		SetUserID(userID).
		SetLastReadAt(readAt).
		OnConflictColumns(supportticketread.FieldTicketID, supportticketread.FieldUserID).
		Update(func(u *dbent.SupportTicketReadUpsert) {
			// 用新写入的 last_read_at 覆盖旧值；updated_at 由 ent 生成的 UpdateNewValues 处理
			// —— 但我们只想更新这两列，因此显式列出。
			u.UpdateLastReadAt()
			u.UpdateUpdatedAt()
		}).
		Exec(ctx)
}

// CountUnreadForUser 返回该用户 owner 的工单中"存在 admin 回复晚于 last_read_at"的工单条数。
//
// 参见文件头 SQL 注释。走 support_ticket_replies (ticket_id, is_admin, created_at) 索引 +
// support_ticket_reads.uq(ticket_id,user_id) 唯一索引，聚合复杂度 O(admin_reply_count)。
func (r *supportTicketReadRepository) CountUnreadForUser(ctx context.Context, userID int64) (int64, error) {
	const q = `
SELECT COUNT(DISTINCT t.id)
  FROM support_tickets t
  JOIN support_ticket_replies r ON r.ticket_id = t.id AND r.is_admin = TRUE
  LEFT JOIN support_ticket_reads rd
         ON rd.ticket_id = t.id AND rd.user_id = t.user_id
 WHERE t.user_id = $1
   AND r.created_at > COALESCE(rd.last_read_at, TIMESTAMP 'epoch')
`
	var count int64
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("support_ticket_read_repo: count unread for user %d: %w", userID, err)
	}
	return count, nil
}

// CountUnreadForAdmin 返回该 admin 视角下未读工单条数：
// 工单 created_at 晚于 last_read_at（新工单未查看）
// OR 最新用户回复时间晚于 last_read_at（已看过后来又有用户回复）。
//
// 用 LATERAL 子查询取每工单最新用户回复时间，避免 self-join 造成的重复行。
func (r *supportTicketReadRepository) CountUnreadForAdmin(ctx context.Context, adminID int64) (int64, error) {
	const q = `
SELECT COUNT(*)
  FROM support_tickets t
  LEFT JOIN support_ticket_reads rd
         ON rd.ticket_id = t.id AND rd.user_id = $1
  LEFT JOIN LATERAL (
      SELECT MAX(rp.created_at) AS last_user_reply_at
        FROM support_ticket_replies rp
       WHERE rp.ticket_id = t.id AND rp.is_admin = FALSE
  ) ur ON TRUE
 WHERE t.created_at          > COALESCE(rd.last_read_at, TIMESTAMP 'epoch')
    OR ur.last_user_reply_at > COALESCE(rd.last_read_at, TIMESTAMP 'epoch')
`
	var count int64
	if err := r.db.QueryRowContext(ctx, q, adminID).Scan(&count); err != nil {
		return 0, fmt.Errorf("support_ticket_read_repo: count unread for admin %d: %w", adminID, err)
	}
	return count, nil
}
