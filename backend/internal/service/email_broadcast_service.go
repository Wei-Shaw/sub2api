package service

import (
	"context"
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"github.com/microcosm-cc/bluemonday"
	"go.uber.org/zap"
)

// EmailBroadcastService 提供管理员批量发送公告邮件的能力。
//
// 设计要点:
//   - HTTP 入口通过 Send() 立刻创建一条 broadcast 记录后异步发送，避免长时间占用请求 goroutine。
//   - 发送过程对单封邮件失败保持容错，整批的成功 / 失败计数会增量回写到 DB。
//   - HTML 正文使用 bluemonday UGCPolicy 做 sanitize，去除潜在 XSS 风险；纯文本正文会被
//     HTML-escape 并替换换行后再生成最终邮件，保证两种格式的最终 MIME 都是合法的 HTML 邮件。
type EmailBroadcastService struct {
	repo                 EmailBroadcastRepository
	userRepo             UserRepository
	emailService         *EmailService
	settingRepo          SettingRepository
	htmlSanitizer        *bluemonday.Policy
	sendIntervalPerEmail time.Duration

	mu      sync.Mutex
	running map[int64]struct{}
}

// EmailBroadcastSendInput 发送一次广播邮件所需的参数集合 (供 handler 调用)。
type EmailBroadcastSendInput struct {
	Subject          string
	Body             string
	BodyFormat       string
	RecipientsMode   string
	RecipientUserIDs []int64
	CreatedBy        *int64
}

// NewEmailBroadcastService 创建广播邮件服务。
func NewEmailBroadcastService(
	repo EmailBroadcastRepository,
	userRepo UserRepository,
	emailService *EmailService,
	settingRepo SettingRepository,
) *EmailBroadcastService {
	policy := bluemonday.UGCPolicy()
	// 允许 <a target="_blank"> 等公告里常用的属性。
	policy.AllowAttrs("target").OnElements("a")
	policy.RequireNoReferrerOnLinks(true)
	policy.RequireNoFollowOnLinks(true)
	policy.AllowStandardURLs()

	return &EmailBroadcastService{
		repo:                 repo,
		userRepo:             userRepo,
		emailService:         emailService,
		settingRepo:          settingRepo,
		htmlSanitizer:        policy,
		sendIntervalPerEmail: 200 * time.Millisecond,
		running:              make(map[int64]struct{}),
	}
}

// Send 校验输入、解析收件人、立刻持久化 broadcast 记录，然后异步触发批量发送。
// 返回值是已创建的 broadcast (status=pending)。
func (s *EmailBroadcastService) Send(ctx context.Context, input EmailBroadcastSendInput) (*EmailBroadcast, error) {
	subject := strings.TrimSpace(input.Subject)
	body := strings.TrimSpace(input.Body)
	bodyFormat := strings.ToLower(strings.TrimSpace(input.BodyFormat))
	mode := strings.ToLower(strings.TrimSpace(input.RecipientsMode))

	if subject == "" {
		return nil, ErrEmailBroadcastSubjectRequired
	}
	if len([]rune(subject)) > domain.EmailBroadcastSubjectMaxLen {
		return nil, ErrEmailBroadcastSubjectTooLong
	}
	if body == "" {
		return nil, ErrEmailBroadcastBodyRequired
	}
	if len(body) > domain.EmailBroadcastBodyMaxLen {
		return nil, ErrEmailBroadcastBodyTooLong
	}
	switch bodyFormat {
	case EmailBroadcastBodyFormatHTML, EmailBroadcastBodyFormatText:
	case "":
		bodyFormat = EmailBroadcastBodyFormatHTML
	default:
		return nil, ErrEmailBroadcastInvalidBodyFormat
	}
	switch mode {
	case EmailBroadcastRecipientsModeAll, EmailBroadcastRecipientsModeSelected:
	case "":
		mode = EmailBroadcastRecipientsModeSelected
	default:
		return nil, ErrEmailBroadcastInvalidMode
	}

	dedupedIDs := dedupePositiveInt64s(input.RecipientUserIDs)
	if mode == EmailBroadcastRecipientsModeSelected {
		if len(dedupedIDs) == 0 {
			return nil, ErrEmailBroadcastNoRecipients
		}
		if len(dedupedIDs) > domain.EmailBroadcastMaxSelectedRecipients {
			return nil, ErrEmailBroadcastTooManyRecipients
		}
	} else {
		// "all" 模式忽略用户传入的 IDs，避免歧义。
		dedupedIDs = nil
	}

	// 提前校验 SMTP 配置存在 — 没配的话直接返回，不写脏数据。
	smtpConfig, err := s.emailService.GetSMTPConfig(ctx)
	if err != nil {
		return nil, ErrEmailBroadcastEmailNotConfigured
	}
	if smtpConfig == nil || strings.TrimSpace(smtpConfig.Host) == "" {
		return nil, ErrEmailBroadcastEmailNotConfigured
	}

	broadcast := &EmailBroadcast{
		Subject:          subject,
		Body:             body,
		BodyFormat:       bodyFormat,
		RecipientsMode:   mode,
		RecipientUserIDs: dedupedIDs,
		Status:           EmailBroadcastStatusPending,
		CreatedBy:        input.CreatedBy,
	}
	if err := s.repo.Create(ctx, broadcast); err != nil {
		return nil, err
	}

	go s.runBroadcast(broadcast.ID)

	return broadcast, nil
}

// List 查询 broadcast 历史。
func (s *EmailBroadcastService) List(ctx context.Context, params EmailBroadcastListParams) (*EmailBroadcastListResult, error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 || params.PageSize > 100 {
		params.PageSize = 20
	}
	return s.repo.List(ctx, params)
}

// Get 取单条 broadcast 详情。
func (s *EmailBroadcastService) Get(ctx context.Context, id int64) (*EmailBroadcast, error) {
	if id <= 0 {
		return nil, ErrEmailBroadcastNotFound
	}
	return s.repo.GetByID(ctx, id)
}

// Delete 物理删除一条历史 broadcast。
// 拒绝删除正在发送中的记录,避免与后台 worker 的状态回写竞争。
// Delete hard-deletes a broadcast record. Refuses to delete records that are
// still being dispatched to avoid racing the background worker's status writes.
func (s *EmailBroadcastService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrEmailBroadcastNotFound
	}
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrEmailBroadcastNotFound
	}
	if existing.Status == EmailBroadcastStatusPending || existing.Status == EmailBroadcastStatusSending {
		return ErrEmailBroadcastDeleteInFlight
	}
	if s.isRunning(id) {
		return ErrEmailBroadcastDeleteInFlight
	}
	return s.repo.Delete(ctx, id)
}

// runBroadcast 是后台 goroutine 入口，对单次 broadcast 执行解析收件人 + 实际 SMTP 投递 + 状态回写。
// 整个过程使用独立的 context.Background，避免 HTTP 请求结束导致中断。
func (s *EmailBroadcastService) runBroadcast(id int64) {
	if !s.markRunning(id) {
		return
	}
	defer s.unmarkRunning(id)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			logger.L().Error("email_broadcast.panic", zap.Int64("broadcast_id", id), zap.Any("panic", r))
			now := time.Now()
			msg := fmt.Sprintf("panic: %v", r)
			_ = s.repo.UpdateStatus(ctx, id, EmailBroadcastStatusUpdate{
				Status:       ptrStr(EmailBroadcastStatusFailed),
				ErrorMessage: &msg,
				FinishedAt:   &now,
			})
		}
	}()

	broadcast, err := s.repo.GetByID(ctx, id)
	if err != nil || broadcast == nil {
		logger.L().Error("email_broadcast.load_failed", zap.Int64("broadcast_id", id), zap.Error(err))
		return
	}

	smtpConfig, err := s.emailService.GetSMTPConfig(ctx)
	if err != nil || smtpConfig == nil {
		now := time.Now()
		msg := "SMTP config unavailable"
		_ = s.repo.UpdateStatus(ctx, id, EmailBroadcastStatusUpdate{
			Status:       ptrStr(EmailBroadcastStatusFailed),
			ErrorMessage: &msg,
			FinishedAt:   &now,
		})
		return
	}

	emails, err := s.resolveRecipientEmails(ctx, broadcast)
	if err != nil {
		now := time.Now()
		msg := err.Error()
		_ = s.repo.UpdateStatus(ctx, id, EmailBroadcastStatusUpdate{
			Status:       ptrStr(EmailBroadcastStatusFailed),
			ErrorMessage: &msg,
			FinishedAt:   &now,
		})
		return
	}
	if len(emails) == 0 {
		now := time.Now()
		msg := "no valid recipients found"
		zero := 0
		_ = s.repo.UpdateStatus(ctx, id, EmailBroadcastStatusUpdate{
			Status:       ptrStr(EmailBroadcastStatusFailed),
			TotalCount:   &zero,
			ErrorMessage: &msg,
			FinishedAt:   &now,
		})
		return
	}

	senderName := s.resolveSenderName(ctx)
	htmlBody := s.composeHTMLBody(broadcast.Subject, broadcast.Body, broadcast.BodyFormat, senderName)

	started := time.Now()
	total := len(emails)
	zero := 0
	_ = s.repo.UpdateStatus(ctx, id, EmailBroadcastStatusUpdate{
		Status:       ptrStr(EmailBroadcastStatusSending),
		TotalCount:   &total,
		SuccessCount: &zero,
		FailedCount:  &zero,
		StartedAt:    &started,
	})

	success, failed := 0, 0
	for idx, addr := range emails {
		if err := s.emailService.SendEmailWithConfigAndContentType(
			smtpConfig,
			addr,
			broadcast.Subject,
			htmlBody,
			"text/html; charset=UTF-8",
		); err != nil {
			failed++
			logger.L().Warn("email_broadcast.send_failed",
				zap.Int64("broadcast_id", id),
				zap.String("recipient", addr),
				zap.Error(err))
		} else {
			success++
		}

		// Throttle to avoid SMTP rate limits; skip after last message.
		if idx < total-1 && s.sendIntervalPerEmail > 0 {
			select {
			case <-ctx.Done():
				failed += total - idx - 1
				goto done
			case <-time.After(s.sendIntervalPerEmail):
			}
		}
	}

done:
	finished := time.Now()
	finalStatus := EmailBroadcastStatusCompleted
	if success == 0 && failed > 0 {
		finalStatus = EmailBroadcastStatusFailed
	}
	_ = s.repo.UpdateStatus(ctx, id, EmailBroadcastStatusUpdate{
		Status:       ptrStr(finalStatus),
		SuccessCount: &success,
		FailedCount:  &failed,
		FinishedAt:   &finished,
	})

	logger.L().Info("email_broadcast.completed",
		zap.Int64("broadcast_id", id),
		zap.Int("total", total),
		zap.Int("success", success),
		zap.Int("failed", failed),
		zap.String("status", finalStatus))
}

// resolveRecipientEmails 把 broadcast 描述的"全部用户 / 指定 IDs"展开为收件人邮箱列表。
func (s *EmailBroadcastService) resolveRecipientEmails(ctx context.Context, b *EmailBroadcast) ([]string, error) {
	emails := make([]string, 0)
	seen := make(map[string]struct{})

	collect := func(email string) {
		email = strings.TrimSpace(email)
		if email == "" {
			return
		}
		key := strings.ToLower(email)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		emails = append(emails, email)
	}

	switch b.RecipientsMode {
	case EmailBroadcastRecipientsModeAll:
		const pageSize = 500
		page := 1
		for {
			users, pageResult, err := s.userRepo.List(ctx, paginationParams(page, pageSize))
			if err != nil {
				return nil, fmt.Errorf("list users: %w", err)
			}
			for i := range users {
				collect(users[i].Email)
			}
			if pageResult == nil || len(users) < pageSize {
				break
			}
			// 防御性：避免无限循环
			if int64(page*pageSize) >= pageResult.Total {
				break
			}
			page++
		}
	case EmailBroadcastRecipientsModeSelected:
		for _, id := range b.RecipientUserIDs {
			user, err := s.userRepo.GetByID(ctx, id)
			if err != nil {
				logger.L().Warn("email_broadcast.user_lookup_failed",
					zap.Int64("broadcast_id", b.ID),
					zap.Int64("user_id", id),
					zap.Error(err))
				continue
			}
			if user != nil {
				collect(user.Email)
			}
		}
	default:
		return nil, fmt.Errorf("unknown recipients mode: %s", b.RecipientsMode)
	}

	return emails, nil
}

// PreviewHTML 生成与最终投递一致的预览 HTML。
// 不需要落库，主要给管理后台编辑器实时预览使用。
//
// PreviewHTML returns the exact HTML that would be sent for the given
// subject + body + format. It is intentionally side-effect free so it can be
// driven by the admin composer for live previewing.
func (s *EmailBroadcastService) PreviewHTML(ctx context.Context, subject, body, format string) string {
	senderName := s.resolveSenderName(ctx)
	return s.composeHTMLBody(subject, body, format, senderName)
}

// composeHTMLBody 根据 broadcast 的 body_format 生成最终的 HTML 邮件正文。
// 纯文本会被 HTML-escape、保留段落与换行；HTML 会经过 bluemonday sanitize。
// 两种格式最终都使用统一的卡片式邮件模板，附 senderName 头部和系统签名页脚，
// 与 sub2api 现有其他模板风格保持一致。
func (s *EmailBroadcastService) composeHTMLBody(subject, body, format, senderName string) string {
	var inner string
	switch format {
	case EmailBroadcastBodyFormatText:
		inner = renderPlainTextAsHTML(body)
	case EmailBroadcastBodyFormatHTML:
		inner = s.htmlSanitizer.Sanitize(body)
	default:
		inner = html.EscapeString(body)
	}
	return wrapBroadcastHTMLShell(subject, senderName, inner)
}

// resolveSenderName 读取邮件落款使用的"发件人名称":
//   1. 优先使用 SMTP 配置里的 \"发件人名称\"(smtp_from_name)。这与收件人收件箱中看到的
//      发件人显示名一致(SendEmailWithConfig 在 From 头里就用这个字段),邮件正文头
//      与签名也跟着保持一致。
//   2. 回退到站点名(site_name)。
//   3. 仍然为空时回退到 \"Sub2API\"。
//
// resolveSenderName chooses the display name used in the email header banner and
// system footer. It prefers the SMTP From-Name (the same name the inbox shows as
// the sender), falling back to site_name and finally to "Sub2API".
func (s *EmailBroadcastService) resolveSenderName(ctx context.Context) string {
	if s.settingRepo == nil {
		return "Sub2API"
	}
	for _, key := range []string{SettingKeySMTPFromName, SettingKeySiteName} {
		name, err := s.settingRepo.GetValue(ctx, key)
		if err != nil {
			continue
		}
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			return trimmed
		}
	}
	return "Sub2API"
}

// renderPlainTextAsHTML 把纯文本转成对应的 HTML 片段:
//   - HTML-escape 防 XSS
//   - 连续两个换行 -> 段落分隔 (<p>)
//   - 单个换行    -> 行内换行 (<br>)
// renderPlainTextAsHTML converts a plain-text body into HTML by escaping it,
// splitting on blank lines into paragraphs and turning single line breaks into <br>.
func renderPlainTextAsHTML(body string) string {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	paragraphs := strings.Split(normalized, "\n\n")
	parts := make([]string, 0, len(paragraphs))
	for _, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		escaped := html.EscapeString(trimmed)
		escaped = strings.ReplaceAll(escaped, "\n", "<br>")
		parts = append(parts, "<p style=\"margin:0 0 16px;\">"+escaped+"</p>")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

const broadcastHTMLTemplate = `<!DOCTYPE html>
<html lang="zh">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
</head>
<body style="margin:0;padding:0;background-color:#f4f6f8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;color:#1f2937;">
    <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="background-color:#f4f6f8;padding:24px 12px;">
        <tr>
            <td align="center">
                <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="600" style="max-width:600px;background-color:#ffffff;border-radius:12px;overflow:hidden;box-shadow:0 4px 16px rgba(15,23,42,0.06);">
                    <tr>
                        <td style="background:linear-gradient(135deg,#2563eb 0%%,#1d4ed8 100%%);padding:28px 32px;color:#ffffff;">
                            <div style="font-size:13px;letter-spacing:0.06em;text-transform:uppercase;opacity:0.85;">%s</div>
                            <h1 style="margin:6px 0 0;font-size:22px;line-height:1.35;font-weight:600;">%s</h1>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:32px;font-size:15px;line-height:1.7;color:#1f2937;">
%s
                        </td>
                    </tr>
                    <tr>
                        <td style="background-color:#f8fafc;padding:18px 32px;border-top:1px solid #eef2f7;color:#94a3b8;font-size:12px;line-height:1.5;text-align:center;">
                            %s
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`

const broadcastFooterText = `此邮件由 %s 系统发送。<br>This email was sent by %s.`

// wrapBroadcastHTMLShell 给正文 + 主题 + 发件人名称(brand)组合成最终邮件 HTML。
func wrapBroadcastHTMLShell(subject, senderName, inner string) string {
	escapedSubject := html.EscapeString(strings.TrimSpace(subject))
	if escapedSubject == "" {
		escapedSubject = "Announcement"
	}
	escapedBrand := html.EscapeString(strings.TrimSpace(senderName))
	if escapedBrand == "" {
		escapedBrand = "Sub2API"
	}
	if strings.TrimSpace(inner) == "" {
		inner = "<p style=\"margin:0;\"></p>"
	}
	footer := fmt.Sprintf(broadcastFooterText, escapedBrand, escapedBrand)
	return fmt.Sprintf(broadcastHTMLTemplate, escapedSubject, escapedBrand, escapedSubject, inner, footer)
}

// markRunning 防止同一个 broadcast 被并发触发两次。
func (s *EmailBroadcastService) markRunning(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.running[id]; ok {
		return false
	}
	s.running[id] = struct{}{}
	return true
}

func (s *EmailBroadcastService) unmarkRunning(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, id)
}

// isRunning 报告指定 broadcast 当前是否在后台 worker 中执行。
func (s *EmailBroadcastService) isRunning(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.running[id]
	return ok
}

func ptrStr(s string) *string { return &s }

func paginationParams(page, pageSize int) pagination.PaginationParams {
	return pagination.PaginationParams{Page: page, PageSize: pageSize}
}

func dedupePositiveInt64s(in []int64) []int64 {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
