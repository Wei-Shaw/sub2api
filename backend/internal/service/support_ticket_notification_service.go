// Package service — support_ticket_notification_service.go
//
// SupportTicketNotificationService 编排"工单事件 → 铃铛通知记录 + 邮件"的多路投递。
//
// 该 service 独立于 SupportTicketService，原因：
//  1. SupportTicketService 已是 700+ 行的状态机 / 业务逻辑集中地，再叠加"通知分发"
//     会让边界模糊；
//  2. 邮件与通知记录的失败都不应影响工单创建/回复主流程（"log warn + swallow" 策略），
//     独立 service 可以让 caller 侧用 defer / goroutine 灵活调度；
//  3. 通知分发需要一次性拿到"管理员群体"（从 ticket_notify_emails 白名单或全体 role=admin），
//     这个需求与 SettingService / UserService 都相关，独立后依赖注入更清晰。
//
// 副作用可靠性等级（从主流程角度看）：
//   - 通知记录 Insert 失败：log warn，主流程继续（用户/管理员会因红点/铃铛缺一条条目，
//     但可通过下次 polling 从数据库真实状态修正）；
//   - 邮件 Send 失败：log warn，主流程继续（NotificationEmailService.Send 内部已经处理
//     队列/幂等/退订，此处只需要传参）。
//
// 调用点（详见 SupportTicketService 主流程勾包）：
//   - CreateTicket 成功后 → NotifyTicketCreated（fan-out 至所有管理员收件人）；
//   - AppendUserReply 成功后 → NotifyUserReplied（fan-out 至所有管理员收件人）；
//   - AppendAdminReply 成功后 → NotifyAdminReplied（recipient = ticket owner）。
package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/inbox"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// SupportTicketNotificationSettingsReader 只读取本 service 需要的两个 setting：
//   - GetSupportTicketRuntime().TicketNotifyEmails —— admin 邮件收件白名单（可空）；
//   - GetFrontendURL —— 用于拼装 portal_url（用户 / admin 工单详情链接）。
//
// 用两个 interface（组合）避免耦合到具体的 *SettingService。
type SupportTicketNotificationSettingsReader interface {
	GetSupportTicketRuntime(ctx context.Context) SupportTicketRuntime
	GetFrontendURL(ctx context.Context) string
}

// SupportTicketNotificationUserLookup 是获取"管理员群体 + 单个用户"信息的抽象。
//
// 拆出接口而非直接依赖 UserRepository，是为了：
//   - 测试可注入轻量 fake，避免复用整个 UserRepository mock；
//   - 未来若管理员群体来源发生变化（如引入"通知白名单角色"），只改这一个接口。
type SupportTicketNotificationUserLookup interface {
	// GetByID 用于取工单 owner 的邮箱 + 用户名（作为管理员回复的收件人）。
	GetByID(ctx context.Context, id int64) (*User, error)
	// ListAdmins 返回所有 role=admin 且 status=active 的用户。
	// 内部实现通常走 UserRepository.ListWithFilters(Role: admin) 并取满 PageSize；
	// admin 数量预期 < 数十，一页即可覆盖。
	ListAdmins(ctx context.Context) ([]User, error)
}

// SupportTicketNotificationEmailSender 是"发工单邮件"的抽象。
//
// 使用现有 *NotificationEmailService.Send，这里定义 interface 只为可测。
type SupportTicketNotificationEmailSender interface {
	Send(ctx context.Context, input NotificationEmailSendInput) error
}

// SupportTicketNotificationService 编排铃铛通知记录 + 邮件的多路投递。
type SupportTicketNotificationService struct {
	notifRepo SupportTicketNotificationRepository
	settings  SupportTicketNotificationSettingsReader
	users     SupportTicketNotificationUserLookup
	emailer   SupportTicketNotificationEmailSender

	// inboxPub 是通用信箱（general-inbox）发布出口，可空。新建/回复事件在写
	// support_ticket_notification 的同时也发布到 inbox，前端从 /inbox/* 读取。
	// nil 表示未装配（inbox 模块未接线），此时 inbox 发布整体跳过。
	inboxPub inbox.Publisher
}

// NewSupportTicketNotificationService 构造通知服务。
//
// emailer 允许为 nil：单测场景下（工单主流程测试）可以选择不注入 emailer，
// 此时邮件分发被跳过（仍写通知记录）。生产路由必须注入非 nil。
func NewSupportTicketNotificationService(
	notifRepo SupportTicketNotificationRepository,
	settings SupportTicketNotificationSettingsReader,
	users SupportTicketNotificationUserLookup,
	emailer SupportTicketNotificationEmailSender,
) *SupportTicketNotificationService {
	return &SupportTicketNotificationService{
		notifRepo: notifRepo,
		settings:  settings,
		users:     users,
		emailer:   emailer,
	}
}

// AttachInbox 注入通用信箱发布出口（general-inbox）。
//
// 采用 setter 而非构造函数入参：避免改动 NewSupportTicketNotificationService 的签名
// 而波及大量既有单测调用点；wire 侧通过 ProvideSupportTicketNotificationService 在
// 构造后调用本方法完成装配。pub 为 nil 时，inbox 发布整体跳过。
func (s *SupportTicketNotificationService) AttachInbox(pub inbox.Publisher) {
	s.inboxPub = pub
}

// ProvideSupportTicketNotificationService 是 wire provider：在 New 基础上一次性把
// 通用信箱发布出口装配好，避免手动调用 AttachInbox 的时序问题。
func ProvideSupportTicketNotificationService(
	notifRepo SupportTicketNotificationRepository,
	settings SupportTicketNotificationSettingsReader,
	users SupportTicketNotificationUserLookup,
	emailer SupportTicketNotificationEmailSender,
	inboxPub inbox.Publisher,
) *SupportTicketNotificationService {
	svc := NewSupportTicketNotificationService(notifRepo, settings, users, emailer)
	svc.AttachInbox(inboxPub)
	return svc
}

// SupportTicketEventContext 是通知分发所需的上下文快照。
//
// 调用方（SupportTicketService 主流程）在通知触发点即时构造该结构，只携带主流程
// 已经拿到的字段；Actor 信息通过 ActorUserID 惰性 lookup（notifier 内部走
// SupportTicketNotificationUserLookup.GetByID），避免 SupportTicketService 再持有
// UserRepository 依赖。
//
// 各字段语义：
//   - Ticket：工单快照（Title / UserID / ID 等，事件当时值）；
//   - ActorUserID：触发者用户 ID（工单创建者 / 回复作者）；0 表示未知（不查询）；
//   - Excerpt：事件正文摘要，调用方 SHOULD 已按 rune 级预截断；notifier 会再走
//     domain.Truncate* 兜底一次，因此 caller 不需要精确控制长度；
//   - ReplyID：仅对回复事件有值，用于 email ReminderKey 去重；
//   - IsAdminReply：仅对 admin_replied 事件为 true（内部使用，用于选择模板变量分支）。
type SupportTicketEventContext struct {
	Ticket       SupportTicket
	ActorUserID  int64
	Excerpt      string
	ReplyID      int64
	IsAdminReply bool
}

// resolveActor 从 ActorUserID 惰性 lookup 触发者用户；失败或 ID=0 返回 nil，
// 模板中 actor_name 会退化为 "unknown"。
func (s *SupportTicketNotificationService) resolveActor(ctx context.Context, actorID int64) *User {
	if actorID == 0 {
		return nil
	}
	u, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		slog.Warn("support_ticket_notification: resolve actor failed; falling back to unknown",
			"actor_user_id", actorID, "err", err)
		return nil
	}
	return u
}

// NotifyTicketCreated 对"用户新建工单"事件做多路投递：
//   - 为每位管理员写入一条 support_ticket_notification 记录；
//   - 对配置的收件人白名单 / 或全体管理员发送 support_ticket.new_ticket 邮件。
//
// 收件人策略详见 resolveAdminRecipients。
//
// 该方法内部所有失败都被 swallow 到 warn 日志：调用方不应因通知失败回滚工单主流程。
func (s *SupportTicketNotificationService) NotifyTicketCreated(ctx context.Context, evt SupportTicketEventContext) {
	admins, extraEmails, err := s.resolveAdminRecipients(ctx)
	if err != nil {
		slog.Warn("support_ticket_notification: resolve admin recipients failed",
			"ticket_id", evt.Ticket.ID, "err", err)
		return
	}
	if len(admins) == 0 && len(extraEmails) == 0 {
		slog.Warn("support_ticket_notification: no admin recipients configured; skipping fanout",
			"ticket_id", evt.Ticket.ID)
		return
	}

	title := domain.TruncateSupportTicketNotificationTitleSnapshot(evt.Ticket.Title)
	excerpt := domain.TruncateSupportTicketNotificationExcerpt(evt.Excerpt)
	actor := s.resolveActor(ctx, evt.ActorUserID)

	// 1. 站内通知：仅写入 role=admin 用户的通知记录；额外的 email-only 白名单不写记录（因为
	//    这些 email 可能匹配不到系统内的用户，写记录会破坏 FK）。
	for i := range admins {
		if admins[i].ID == 0 {
			continue
		}
		n := &SupportTicketNotification{
			RecipientUserID: admins[i].ID,
			TicketID:        evt.Ticket.ID,
			EventType:       domain.SupportTicketNotificationEventTicketCreated,
			TitleSnapshot:   title,
			Excerpt:         excerpt,
			ActorUserID:     actorIDOrNil(actor),
		}
		if err := s.notifRepo.Insert(ctx, n); err != nil {
			slog.Warn("support_ticket_notification: insert failed",
				"ticket_id", evt.Ticket.ID, "recipient_user_id", admins[i].ID, "err", err)
		}
	}

	// 2. 邮件：admin 用户邮箱 + email-only 白名单 一起投递。
	portalURL := s.buildAdminPortalURL(ctx, evt.Ticket.ID)
	for _, rec := range s.mergeAdminEmailRecipients(admins, extraEmails) {
		s.sendMail(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventSupportTicketNewTicket,
			RecipientEmail: rec.email,
			RecipientName:  rec.name,
			UserID:         rec.userID, // 0 表示未匹配到系统用户
			SourceType:     "support_ticket",
			SourceID:       strconv.FormatInt(evt.Ticket.ID, 10),
			ReminderKey:    fmt.Sprintf("%s|%d", domain.SupportTicketNotificationEventTicketCreated, evt.Ticket.ID),
			Variables: map[string]string{
				"ticket_id":  strconv.FormatInt(evt.Ticket.ID, 10),
				"title":      evt.Ticket.Title,
				"excerpt":    evt.Excerpt,
				"actor_name": actorDisplayName(actor),
				"portal_url": portalURL,
			},
		})
	}

	// 通用信箱双写：向全体管理员广播一条工单新建事件。
	s.publishInboxToAdmins(ctx, evt, domain.SupportTicketNotificationEventTicketCreated,
		actorDisplayName(actor), s.buildAdminPortalURL(ctx, evt.Ticket.ID), evt.Ticket.ID)
}

// NotifyUserReplied 对"用户回复工单"事件做多路投递：通知所有管理员。
// 与 NotifyTicketCreated 语义一致，仅事件类型与 ReminderKey 不同。
func (s *SupportTicketNotificationService) NotifyUserReplied(ctx context.Context, evt SupportTicketEventContext) {
	admins, extraEmails, err := s.resolveAdminRecipients(ctx)
	if err != nil {
		slog.Warn("support_ticket_notification: resolve admin recipients failed",
			"ticket_id", evt.Ticket.ID, "err", err)
		return
	}
	if len(admins) == 0 && len(extraEmails) == 0 {
		return
	}

	title := domain.TruncateSupportTicketNotificationTitleSnapshot(evt.Ticket.Title)
	excerpt := domain.TruncateSupportTicketNotificationExcerpt(evt.Excerpt)
	actor := s.resolveActor(ctx, evt.ActorUserID)

	for i := range admins {
		if admins[i].ID == 0 {
			continue
		}
		n := &SupportTicketNotification{
			RecipientUserID: admins[i].ID,
			TicketID:        evt.Ticket.ID,
			EventType:       domain.SupportTicketNotificationEventUserReplied,
			TitleSnapshot:   title,
			Excerpt:         excerpt,
			ActorUserID:     actorIDOrNil(actor),
		}
		if err := s.notifRepo.Insert(ctx, n); err != nil {
			slog.Warn("support_ticket_notification: insert failed",
				"ticket_id", evt.Ticket.ID, "recipient_user_id", admins[i].ID, "err", err)
		}
	}

	portalURL := s.buildAdminPortalURL(ctx, evt.Ticket.ID)
	replyKindLabel := s.replyKindLabel(false)
	for _, rec := range s.mergeAdminEmailRecipients(admins, extraEmails) {
		s.sendMail(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventSupportTicketNewReply,
			RecipientEmail: rec.email,
			RecipientName:  rec.name,
			UserID:         rec.userID,
			SourceType:     "support_ticket",
			SourceID:       strconv.FormatInt(evt.Ticket.ID, 10),
			ReminderKey:    fmt.Sprintf("%s|%d", domain.SupportTicketNotificationEventUserReplied, evt.ReplyID),
			Variables: map[string]string{
				"ticket_id":        strconv.FormatInt(evt.Ticket.ID, 10),
				"title":            evt.Ticket.Title,
				"excerpt":          evt.Excerpt,
				"actor_name":       actorDisplayName(actor),
				"reply_kind_label": replyKindLabel,
				"portal_url":       portalURL,
			},
		})
	}

	// 通用信箱双写：向全体管理员广播一条用户回复事件（dedup 维度按 reply_id）。
	s.publishInboxToAdmins(ctx, evt, domain.SupportTicketNotificationEventUserReplied,
		actorDisplayName(actor), s.buildAdminPortalURL(ctx, evt.Ticket.ID), evt.ReplyID)
}

// NotifyAdminReplied 对"管理员回复工单"事件做单点投递：通知工单 owner。
// 仅当 owner 存在（未被删除）时发邮件；通知记录始终写入（FK CASCADE 会兜底清理）。
func (s *SupportTicketNotificationService) NotifyAdminReplied(ctx context.Context, evt SupportTicketEventContext) {
	owner, err := s.users.GetByID(ctx, evt.Ticket.UserID)
	if err != nil {
		slog.Warn("support_ticket_notification: load ticket owner failed",
			"ticket_id", evt.Ticket.ID, "owner_id", evt.Ticket.UserID, "err", err)
		// 通知记录仍尝试写入（FK 仍在）：即使 owner 被删，级联清理会兜底。
	}

	title := domain.TruncateSupportTicketNotificationTitleSnapshot(evt.Ticket.Title)
	excerpt := domain.TruncateSupportTicketNotificationExcerpt(evt.Excerpt)
	actor := s.resolveActor(ctx, evt.ActorUserID)

	n := &SupportTicketNotification{
		RecipientUserID: evt.Ticket.UserID,
		TicketID:        evt.Ticket.ID,
		EventType:       domain.SupportTicketNotificationEventAdminReplied,
		TitleSnapshot:   title,
		Excerpt:         excerpt,
		ActorUserID:     actorIDOrNil(actor),
	}
	if err := s.notifRepo.Insert(ctx, n); err != nil {
		slog.Warn("support_ticket_notification: insert failed",
			"ticket_id", evt.Ticket.ID, "recipient_user_id", evt.Ticket.UserID, "err", err)
	}

	// 通用信箱双写：单播一条 admin 回复事件给工单 owner（dedup 维度按 reply_id）。
	s.publishInboxDirect(ctx, evt.Ticket.UserID, evt, domain.SupportTicketNotificationEventAdminReplied,
		actorDisplayName(actor), s.buildUserPortalURL(ctx, evt.Ticket.ID), evt.ReplyID)

	if owner == nil || strings.TrimSpace(owner.Email) == "" {
		return
	}
	portalURL := s.buildUserPortalURL(ctx, evt.Ticket.ID)
	replyKindLabel := s.replyKindLabel(true)
	s.sendMail(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventSupportTicketNewReply,
		RecipientEmail: owner.Email,
		RecipientName:  displayNameForUser(owner),
		UserID:         owner.ID,
		SourceType:     "support_ticket",
		SourceID:       strconv.FormatInt(evt.Ticket.ID, 10),
		ReminderKey:    fmt.Sprintf("%s|%d", domain.SupportTicketNotificationEventAdminReplied, evt.ReplyID),
		Variables: map[string]string{
			"ticket_id":        strconv.FormatInt(evt.Ticket.ID, 10),
			"title":            evt.Ticket.Title,
			"excerpt":          evt.Excerpt,
			"actor_name":       actorDisplayName(actor),
			"reply_kind_label": replyKindLabel,
			"portal_url":       portalURL,
		},
	})
}

// resolveAdminRecipients 是收件人策略核心（Design 决策 γ）：
//   - 若 settings.ticket_notify_emails 非空 → 用其作为邮件收件白名单；
//     同时尝试根据 email 匹配系统用户，匹配到的用户既写通知记录也发邮件，
//     匹配不到的 email 只发邮件（extraEmails）；
//   - 若白名单为空 → 兜底为所有 role=admin 且 status=active 的用户；
//     admin 用户既写通知记录也发邮件（extraEmails 为空）。
//
// 返回值：
//   - admins：需要写入 support_ticket_notification 的用户集合（同时也是邮件收件人）；
//   - extraEmails：仅发送邮件的额外邮箱（配置白名单里 match 不到用户的邮箱）；
//   - err：settings 读取失败时非 nil，通知分发直接跳过。
func (s *SupportTicketNotificationService) resolveAdminRecipients(ctx context.Context) (
	admins []User,
	extraEmails []string,
	err error,
) {
	rt := s.settings.GetSupportTicketRuntime(ctx)
	whitelist := normalizeEmailList(rt.TicketNotifyEmails)

	if len(whitelist) == 0 {
		// 兜底：全体 role=admin 用户。
		list, err := s.users.ListAdmins(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list admins: %w", err)
		}
		return list, nil, nil
	}

	// 白名单模式：一次拉出全体 admin，构造 email → user 的索引，然后 O(len(whitelist)) 匹配。
	// 白名单预期 <10、admin 也 <10；额外一次 admin 列表查询是可接受的常量开销。
	// 若 ListAdmins 失败，白名单邮箱全部退化为 email-only 投递（extraEmails），仍保证邮件送出。
	adminList, listErr := s.users.ListAdmins(ctx)
	if listErr != nil {
		slog.Warn("support_ticket_notification: list admins for whitelist match failed; degrading to email-only",
			"err", listErr)
		adminList = nil
	}
	adminByEmail := make(map[string]*User, len(adminList))
	for i := range adminList {
		key := strings.ToLower(strings.TrimSpace(adminList[i].Email))
		if key != "" {
			adminByEmail[key] = &adminList[i]
		}
	}

	admins = make([]User, 0, len(whitelist))
	extraEmails = make([]string, 0)
	seen := make(map[int64]struct{}, len(whitelist))
	for _, email := range whitelist {
		if u, ok := adminByEmail[email]; ok {
			if _, dup := seen[u.ID]; !dup {
				admins = append(admins, *u)
				seen[u.ID] = struct{}{}
			}
			continue
		}
		extraEmails = append(extraEmails, email)
	}
	return admins, extraEmails, nil
}

// normalizeEmailList 把 settings 里存的 email 列表做 trim + lowercase + 去空 + 去重。
// 不做严格 RFC 校验（由 settings update 层负责）。
func normalizeEmailList(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(list))
	out := make([]string, 0, len(list))
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		s = strings.ToLower(s)
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// adminEmailRecipient 是"发一封邮件"所需的最小信息 tuple。
type adminEmailRecipient struct {
	userID int64
	email  string
	name   string
}

// mergeAdminEmailRecipients 把 admins（含 userID + name）与 extraEmails（仅 email）
// 合并成统一的邮件收件人列表。空 email 跳过。
func (s *SupportTicketNotificationService) mergeAdminEmailRecipients(admins []User, extraEmails []string) []adminEmailRecipient {
	out := make([]adminEmailRecipient, 0, len(admins)+len(extraEmails))
	for i := range admins {
		email := strings.TrimSpace(admins[i].Email)
		if email == "" {
			continue
		}
		out = append(out, adminEmailRecipient{
			userID: admins[i].ID,
			email:  email,
			name:   displayNameForUser(&admins[i]),
		})
	}
	for _, email := range extraEmails {
		if strings.TrimSpace(email) == "" {
			continue
		}
		out = append(out, adminEmailRecipient{
			userID: 0,
			email:  email,
			name:   email,
		})
	}
	return out
}

// buildAdminPortalURL / buildUserPortalURL 分别构造 admin / 用户视角的工单详情 URL。
// 若 FrontendURL 未配置，返回相对路径（邮件里点了 UI 会失效但不影响内容展示）。
func (s *SupportTicketNotificationService) buildAdminPortalURL(ctx context.Context, ticketID int64) string {
	base := strings.TrimRight(strings.TrimSpace(s.settings.GetFrontendURL(ctx)), "/")
	// admin 端工单详情走列表页的 query 深链（?open=<id>）打开抽屉，
	// 前端未注册 /admin/support/tickets/:id 路径式路由，直接拼 /:id 会 404。
	if base == "" {
		return fmt.Sprintf("/admin/support/tickets?open=%d", ticketID)
	}
	return fmt.Sprintf("%s/admin/support/tickets?open=%d", base, ticketID)
}

func (s *SupportTicketNotificationService) buildUserPortalURL(ctx context.Context, ticketID int64) string {
	base := strings.TrimRight(strings.TrimSpace(s.settings.GetFrontendURL(ctx)), "/")
	if base == "" {
		return fmt.Sprintf("/support/tickets/%d", ticketID)
	}
	return fmt.Sprintf("%s/support/tickets/%d", base, ticketID)
}

// replyKindLabel 根据是否 admin 回复返回本地无关的英文短语；
// service 层与邮件主题里都直接嵌入该字符串（收件人可能是不同 locale，
// 精确 i18n 由邮件模板层的 subject/HTML 走整套 template 时不好覆盖 —— 这里选用
// 中英双语拼接的短语，让两种语言用户都能读懂；后续 i18n 优化可以拆分模板）。
func (s *SupportTicketNotificationService) replyKindLabel(isAdmin bool) string {
	if isAdmin {
		return "客服回复 / support reply"
	}
	return "用户回复 / user reply"
}

// sendMail 是 emailer 的容错包装：nil 时 no-op，err 时降级为 warn 日志。
func (s *SupportTicketNotificationService) sendMail(ctx context.Context, input NotificationEmailSendInput) {
	if s.emailer == nil {
		return
	}
	if err := s.emailer.Send(ctx, input); err != nil {
		slog.Warn("support_ticket_notification: send email failed",
			"event", input.Event,
			"source_id", input.SourceID,
			"err", err)
	}
}

// actorIDOrNil 把 *User 转成 *int64（用于 ActorUserID 字段）。
func actorIDOrNil(u *User) *int64 {
	if u == nil {
		return nil
	}
	id := u.ID
	return &id
}

// actorDisplayName 返回 actor 的展示名（模板 actor_name 变量）。
// 优先 Username，其次 Email，都空时返回 "unknown"。
func actorDisplayName(u *User) string {
	if u == nil {
		return "unknown"
	}
	return displayNameForUser(u)
}

// displayNameForUser 与 actorDisplayName 语义一致，但输入是非 nil 指针。
// 拆出是为了收件人展示（admins / owner 时调用方能确保 non-nil）。
func displayNameForUser(u *User) string {
	if u == nil {
		return "unknown"
	}
	if name := strings.TrimSpace(u.Username); name != "" {
		return name
	}
	if email := strings.TrimSpace(u.Email); email != "" {
		return email
	}
	return "unknown"
}

// ================================================================
// Listing helpers（handler 层直接透传参数使用）
// ================================================================

// ListNotifications 是 handler GET /notifications 端点的直连业务口子；
// 分页 + OnlyUnread 直接透传到 Repository。
func (s *SupportTicketNotificationService) ListNotifications(
	ctx context.Context,
	recipientUserID int64,
	onlyUnread bool,
	params pagination.PaginationParams,
) ([]SupportTicketNotification, *pagination.PaginationResult, error) {
	if recipientUserID == 0 {
		return nil, nil, ErrSupportTicketNotificationRecipientRequired
	}
	return s.notifRepo.ListByRecipient(ctx, SupportTicketNotificationListParams{
		RecipientUserID: recipientUserID,
		OnlyUnread:      onlyUnread,
		Params:          params,
	})
}

// CountUnread 是 handler GET /notifications/unread-count 端点的口子。
// 直接返回 recipient 名下 is_read=false 的通知条数（不区分事件类型）。
func (s *SupportTicketNotificationService) CountUnread(ctx context.Context, recipientUserID int64) (int64, error) {
	if recipientUserID == 0 {
		return 0, ErrSupportTicketNotificationRecipientRequired
	}
	return s.notifRepo.CountUnreadByRecipient(ctx, recipientUserID)
}

// MarkOneRead 是 handler POST /notifications/:id/read 端点的口子。
// 权限校验完全落在 Repository 层的 (id, recipient) 二元定位：不属于 caller 返回 NotFound。
func (s *SupportTicketNotificationService) MarkOneRead(ctx context.Context, id int64, recipientUserID int64) error {
	if recipientUserID == 0 {
		return ErrSupportTicketNotificationRecipientRequired
	}
	return s.notifRepo.MarkOneRead(ctx, id, recipientUserID, nowUTC())
}

// MarkAllRead 是 handler POST /notifications/read-all 端点的口子。
// 返回受影响行数供 handler 决定日志级别。
func (s *SupportTicketNotificationService) MarkAllRead(ctx context.Context, recipientUserID int64) (int64, error) {
	if recipientUserID == 0 {
		return 0, ErrSupportTicketNotificationRecipientRequired
	}
	return s.notifRepo.MarkAllRead(ctx, recipientUserID, nowUTC())
}

// nowUTC 是包内共享的 time.Now 包装（不做 mock hook，若单测需要 freeze 时间，
// 可从 SupportTicketNotificationService 上引出 now 函数字段替换；这里保持简洁，
// 因为读值不参与逻辑分支，用一个包级变量即可）。
var nowUTC = time.Now
