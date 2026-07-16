// Package repository — support_chat_log_repo.go
//
// 客服浮窗对话审计（add-support-chat-transcript-log）Repository 实现。设计要点：
//
//   - UpsertConversationAndAppend 在单事务内幂等归并会话 + 追加消息。会话 upsert 用
//     ent 的 sql/upsert（ON CONFLICT(session_id)）：turn_count 走原子 `+1` SQL 表达式，
//     last_status/last_at 覆盖为本轮值，user_id 用 COALESCE(旧,新) 保留首次登录身份。
//     调用方（handler）已保证同一 session 串行发问，并发 upsert 概率极低；即便发生，
//     ON CONFLICT 也保证不重复建会话。
//
//   - content 落库前截断到 SupportChatLogContentMaxLen（与工单 chat_context 上限对齐），
//     DB 列不加约束，截断在本层做，避免 handler 各分支重复截断逻辑。
//
//   - ListConversations 不返回消息正文（列表视图只读会话头）；q 关键词走 message content
//     子查询 ILIKE（ent ContainsFold，参数化，防注入）。
package repository

import (
	"context"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/supportchatconversation"
	"github.com/Wei-Shaw/sub2api/ent/supportchatmessage"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type supportChatLogRepository struct {
	client *dbent.Client
}

// NewSupportChatLogRepository 构造对话审计 Repository（接受运行期 *ent.Client，
// 事务由 clientFromContext 透明切换）。
func NewSupportChatLogRepository(client *dbent.Client) service.SupportChatLogRepository {
	return &supportChatLogRepository{client: client}
}

// truncateChatLogContent 把 content 截断到 SupportChatLogContentMaxLen 个 rune。
func truncateChatLogContent(s string) string {
	r := []rune(s)
	if len(r) <= service.SupportChatLogContentMaxLen {
		return s
	}
	return string(r[:service.SupportChatLogContentMaxLen])
}

// UpsertConversationAndAppend 幂等归并会话 + 追加本轮 user/assistant 消息，全程单事务。
func (r *supportChatLogRepository) UpsertConversationAndAppend(ctx context.Context, turn service.SupportChatTurn) error {
	exec := func(ctx context.Context) error {
		client := clientFromContext(ctx, r.client)

		// 1. upsert 会话头。turn_count 原子 +1；last_status/last_at 覆盖；user_id COALESCE 保留旧值。
		upsert := client.SupportChatConversation.Create().
			SetSessionID(turn.SessionID).
			SetClientIP(turn.ClientIP).
			SetTurnCount(1).
			SetLastStatus(turn.Status).
			SetFirstAt(turn.At).
			SetLastAt(turn.At)
		if turn.UserID != nil {
			upsert = upsert.SetUserID(*turn.UserID)
		}
		if err := upsert.
			OnConflictColumns(supportchatconversation.FieldSessionID).
			Update(func(u *dbent.SupportChatConversationUpsert) {
				u.AddTurnCount(1)
				u.SetLastStatus(turn.Status)
				u.SetLastAt(turn.At)
				u.UpdateUpdatedAt()
				// user_id 不在 conflict 分支更新：同一 session_id 的用户身份稳定
				// （首轮 INSERT 已定），保留首次写入值即可，无需 COALESCE。
			}).
			Exec(ctx); err != nil {
			return err
		}

		// 2. 取回会话 id（session_id 唯一）。
		conv, err := client.SupportChatConversation.Query().
			Where(supportchatconversation.SessionIDEQ(turn.SessionID)).
			Only(ctx)
		if err != nil {
			return err
		}

		// 3. append user 行（若有内容）。
		if strings.TrimSpace(turn.UserContent) != "" {
			if _, err := client.SupportChatMessage.Create().
				SetConversationID(conv.ID).
				SetRole(service.ChatLogRoleUser).
				SetContent(truncateChatLogContent(turn.UserContent)).
				SetCreatedAt(turn.At).
				Save(ctx); err != nil {
				return err
			}
		}

		// 4. append assistant 行（回包，可能为空/部分文本）。
		amsg := client.SupportChatMessage.Create().
			SetConversationID(conv.ID).
			SetRole(service.ChatLogRoleAssistant).
			SetContent(truncateChatLogContent(turn.AssistantContent)).
			SetStatus(turn.Status).
			SetCreatedAt(turn.At)
		if turn.ErrorMessage != "" {
			amsg = amsg.SetErrorMessage(turn.ErrorMessage)
		}
		if turn.Model != "" {
			amsg = amsg.SetModel(turn.Model)
		}
		if turn.LatencyMS != nil {
			amsg = amsg.SetLatencyMs(*turn.LatencyMS)
		}
		if _, err := amsg.Save(ctx); err != nil {
			return err
		}
		return nil
	}

	// 无 entClient 事务时直接顺序执行（clientFromContext 会 fallback 到默认 client）。
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := exec(txCtx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// ListConversations 返回 admin 分页列表（不含消息正文），按 last_at DESC、id DESC 排序。
func (r *supportChatLogRepository) ListConversations(
	ctx context.Context,
	filters service.SupportChatLogListFilters,
	params pagination.PaginationParams,
) ([]service.SupportChatConversation, *pagination.PaginationResult, error) {
	q := r.client.SupportChatConversation.Query()

	if s := strings.TrimSpace(filters.Status); s != "" {
		q = q.Where(supportchatconversation.LastStatusEQ(s))
	}
	if filters.UserID != nil {
		q = q.Where(supportchatconversation.UserIDEQ(*filters.UserID))
	}
	if ip := strings.TrimSpace(filters.ClientIP); ip != "" {
		q = q.Where(supportchatconversation.ClientIPEQ(ip))
	}
	if filters.From != nil {
		q = q.Where(supportchatconversation.LastAtGTE(*filters.From))
	}
	if filters.To != nil {
		q = q.Where(supportchatconversation.LastAtLTE(*filters.To))
	}
	if kw := strings.TrimSpace(filters.Search); kw != "" {
		// 命中任一消息 content ILIKE %kw% 的会话。HasMessagesWith 生成 EXISTS 子查询，
		// ContainsFold 生成参数化 ILIKE，无注入风险。
		q = q.Where(supportchatconversation.HasMessagesWith(
			supportchatmessage.ContentContainsFold(kw),
		))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	items, err := q.
		Order(dbent.Desc(supportchatconversation.FieldLastAt), dbent.Desc(supportchatconversation.FieldID)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([]service.SupportChatConversation, 0, len(items))
	for _, m := range items {
		out = append(out, *supportChatConversationEntityToService(m))
	}
	if err := r.attachUserEmails(ctx, out); err != nil {
		return nil, nil, err
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

// attachUserEmails 批量回填 email：给定的每条会话若 UserID 非空则一次性查 users 表补 UserEmail。
// 穿透软删除以便 admin 视图能显示已删用户身份（与 usage_log 列表口径一致）。
func (r *supportChatLogRepository) attachUserEmails(ctx context.Context, convs []service.SupportChatConversation) error {
	if len(convs) == 0 {
		return nil
	}
	idSet := make(map[int64]struct{}, len(convs))
	for _, c := range convs {
		if c.UserID != nil {
			idSet[*c.UserID] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	users, err := r.client.User.Query().
		Where(dbuser.IDIn(ids...)).
		Select(dbuser.FieldID, dbuser.FieldEmail).
		All(mixins.SkipSoftDelete(ctx))
	if err != nil {
		return err
	}
	emails := make(map[int64]string, len(users))
	for _, u := range users {
		emails[u.ID] = u.Email
	}
	for i := range convs {
		if convs[i].UserID == nil {
			continue
		}
		if email, ok := emails[*convs[i].UserID]; ok {
			e := email
			convs[i].UserEmail = &e
		}
	}
	return nil
}

// GetConversationWithMessages 返回会话头 + 按 created_at ASC 的全部消息。
func (r *supportChatLogRepository) GetConversationWithMessages(ctx context.Context, id int64) (*service.SupportChatConversationDetail, error) {
	conv, err := r.client.SupportChatConversation.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSupportChatConversationNotFound, nil)
	}

	msgs, err := r.client.SupportChatMessage.Query().
		Where(supportchatmessage.ConversationIDEQ(id)).
		Order(dbent.Asc(supportchatmessage.FieldCreatedAt), dbent.Asc(supportchatmessage.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	detail := &service.SupportChatConversationDetail{
		Conversation: *supportChatConversationEntityToService(conv),
		Messages:     make([]service.SupportChatMessage, 0, len(msgs)),
	}
	// 单条会话也走同一路径回填 email；参数化查询，成本可忽略。
	tmp := []service.SupportChatConversation{detail.Conversation}
	if err := r.attachUserEmails(ctx, tmp); err != nil {
		return nil, err
	}
	detail.Conversation = tmp[0]
	for _, m := range msgs {
		detail.Messages = append(detail.Messages, *supportChatMessageEntityToService(m))
	}
	return detail, nil
}

func supportChatConversationEntityToService(m *dbent.SupportChatConversation) *service.SupportChatConversation {
	if m == nil {
		return nil
	}
	return &service.SupportChatConversation{
		ID:         m.ID,
		SessionID:  m.SessionID,
		UserID:     m.UserID,
		ClientIP:   m.ClientIP,
		TurnCount:  m.TurnCount,
		LastStatus: m.LastStatus,
		FirstAt:    m.FirstAt,
		LastAt:     m.LastAt,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

func supportChatMessageEntityToService(m *dbent.SupportChatMessage) *service.SupportChatMessage {
	if m == nil {
		return nil
	}
	return &service.SupportChatMessage{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		Role:           m.Role,
		Content:        m.Content,
		Status:         m.Status,
		ErrorMessage:   m.ErrorMessage,
		Model:          m.Model,
		LatencyMS:      m.LatencyMs,
		CreatedAt:      m.CreatedAt,
	}
}
