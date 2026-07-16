// Package repository — support_ticket_notification_repo.go
//
// 铃铛面板通知记录（support_ticket_notification）的 Repository 实现。
//
// 设计要点：
//   - Insert：一次一条；service 层会为多 recipient 逐条 Insert（工单 ticket_created / user_replied
//     发给所有 admin 的场景）。批量写入用简单 for 循环即可，量级预期在 O(admin 数量) 级别，
//     不需要 bulk insert 优化。
//   - ListByRecipient：SQL 层用 (recipient_user_id, is_read, created_at DESC) 复合索引，
//     OnlyUnread=true 时下推 is_read=false 条件；排序 created_at DESC, tie-break id DESC。
//   - CountUnreadByRecipient：COUNT WHERE recipient=? AND is_read=false，同索引前缀命中。
//   - MarkOneRead：先按 (id, recipient) 定位，找不到时返回 ErrSupportTicketNotificationNotFound（防泄漏）；
//     命中后 UPDATE is_read=true / read_at=?；已读时幂等 no-op 避免覆盖 read_at。
//   - MarkAllRead：一条 UPDATE 把 recipient 名下所有 is_read=false 置为 true，返回受影响行数。
package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/supportticketnotification"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type supportTicketNotificationRepository struct {
	client *dbent.Client
}

// NewSupportTicketNotificationRepository 构造铃铛面板通知 Repository。
func NewSupportTicketNotificationRepository(client *dbent.Client) service.SupportTicketNotificationRepository {
	return &supportTicketNotificationRepository{client: client}
}

// Insert 写入一条铃铛通知记录。
//
// 入参约束：excerpt / title_snapshot 应已经过 domain.TruncateSupportTicketNotification*
// 截断到列长度以内；event_type 应使用 domain 常量之一（本层不做二次校验，
// schema 的 EventTypeValidator + service 层的分发口子已经收敛）。
func (r *supportTicketNotificationRepository) Insert(ctx context.Context, n *service.SupportTicketNotification) error {
	client := clientFromContext(ctx, r.client)
	builder := client.SupportTicketNotification.Create().
		SetRecipientUserID(n.RecipientUserID).
		SetTicketID(n.TicketID).
		SetEventType(n.EventType).
		SetTitleSnapshot(n.TitleSnapshot)

	// excerpt 为可空列。空串等价于 NULL（前端展示上无差别），这里不写入即可。
	if n.Excerpt != "" {
		builder.SetExcerpt(n.Excerpt)
	}
	if n.ActorUserID != nil {
		builder.SetActorUserID(*n.ActorUserID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	n.ID = created.ID
	n.CreatedAt = created.CreatedAt
	n.IsRead = created.IsRead
	// ReadAt 保持 nil（新写入的记录 is_read=false，read_at 列为 NULL）。
	return nil
}

// ListByRecipient 返回分页后的通知列表，按 created_at DESC / id DESC 排序。
// OnlyUnread=true 时下推 is_read=false 过滤到 SQL 层。
func (r *supportTicketNotificationRepository) ListByRecipient(
	ctx context.Context,
	params service.SupportTicketNotificationListParams,
) ([]service.SupportTicketNotification, *pagination.PaginationResult, error) {
	q := r.client.SupportTicketNotification.Query().
		Where(supportticketnotification.RecipientUserIDEQ(params.RecipientUserID))

	if params.OnlyUnread {
		q = q.Where(supportticketnotification.IsReadEQ(false))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	items, err := q.
		Order(
			dbent.Desc(supportticketnotification.FieldCreatedAt),
			dbent.Desc(supportticketnotification.FieldID),
		).
		Offset(params.Params.Offset()).
		Limit(params.Params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([]service.SupportTicketNotification, len(items))
	for i, m := range items {
		out[i] = supportTicketNotificationEntityToService(m)
	}
	return out, paginationResultFromTotal(int64(total), params.Params), nil
}

// CountUnreadByRecipient 返回 recipient 视角下 is_read=false 的通知条数。
// 走 (recipient_user_id, is_read, created_at) 复合索引前缀。
func (r *supportTicketNotificationRepository) CountUnreadByRecipient(
	ctx context.Context,
	recipientUserID int64,
) (int64, error) {
	count, err := r.client.SupportTicketNotification.Query().
		Where(
			supportticketnotification.RecipientUserIDEQ(recipientUserID),
			supportticketnotification.IsReadEQ(false),
		).
		Count(ctx)
	if err != nil {
		return 0, err
	}
	return int64(count), nil
}

// MarkOneRead 把 (id, recipientUserID) 匹配的通知置为已读。
//
// 语义细节：
//   - (id, recipient) 不匹配 → 返回 ErrSupportTicketNotificationNotFound
//     （无论 id 不存在，还是 id 存在但 recipient ≠ caller，都返回同一个错误，
//     避免泄漏"该通知是否存在"，spec 5.3）。
//   - 已经 is_read=true 的重复调用：不写入（避免 read_at 被覆盖到最后一次点击时刻），
//     直接返回 nil，保留最初读取时刻用于审计。
func (r *supportTicketNotificationRepository) MarkOneRead(
	ctx context.Context,
	id int64,
	recipientUserID int64,
	readAt time.Time,
) error {
	client := clientFromContext(ctx, r.client)

	notif, err := client.SupportTicketNotification.Query().
		Where(
			supportticketnotification.IDEQ(id),
			supportticketnotification.RecipientUserIDEQ(recipientUserID),
		).
		Only(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSupportTicketNotificationNotFound, nil)
	}

	// 幂等 no-op：保留首次已读时间戳。
	if notif.IsRead {
		return nil
	}

	return client.SupportTicketNotification.UpdateOneID(id).
		SetIsRead(true).
		SetReadAt(readAt).
		Exec(ctx)
}

// MarkAllRead 把 recipient 名下所有 is_read=false 通知一次性置为 true / read_at=readAt。
// 返回受影响行数。
func (r *supportTicketNotificationRepository) MarkAllRead(
	ctx context.Context,
	recipientUserID int64,
	readAt time.Time,
) (int64, error) {
	client := clientFromContext(ctx, r.client)
	affected, err := client.SupportTicketNotification.Update().
		Where(
			supportticketnotification.RecipientUserIDEQ(recipientUserID),
			supportticketnotification.IsReadEQ(false),
		).
		SetIsRead(true).
		SetReadAt(readAt).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	return int64(affected), nil
}

// supportTicketNotificationEntityToService 把 ent entity 拷贝到 service 领域模型。
//
// Excerpt / ActorUserID / ReadAt 是可空列（*string / *int64 / *time.Time）；
// 领域模型选择用值 + 指针混合以匹配 handler 序列化时的期待：
//   - Excerpt：ent 层是 *string（Optional().Nillable()）；领域模型用 string，
//     NULL 对齐到空串以让 handler 端简单直接（前端也不区分 null/空串）。
//   - ActorUserID / ReadAt：保留指针语义，因为它们对 UI 展示有语义区别
//     （未知触发者 vs 未读通知）。
func supportTicketNotificationEntityToService(m *dbent.SupportTicketNotification) service.SupportTicketNotification {
	n := service.SupportTicketNotification{
		ID:              m.ID,
		RecipientUserID: m.RecipientUserID,
		TicketID:        m.TicketID,
		EventType:       m.EventType,
		TitleSnapshot:   m.TitleSnapshot,
		ActorUserID:     m.ActorUserID,
		IsRead:          m.IsRead,
		CreatedAt:       m.CreatedAt,
		ReadAt:          m.ReadAt,
	}
	if m.Excerpt != nil {
		n.Excerpt = *m.Excerpt
	}
	return n
}
