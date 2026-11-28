package service

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	ticketEmailContentMaxRunes = 2000
	ticketAdminNotifyPageSize  = 200
)

func (s *TicketService) notifyTicketCreated(ticket *Ticket, content string) {
	if s == nil || ticket == nil {
		return
	}
	go s.dispatchTicketEmails(func(ctx context.Context) {
		vars := s.ticketCommonVariables(ctx, ticket, true)
		vars["ticket_content"] = truncateTicketEmailContent(content)
		s.sendToAdmins(ctx, NotificationEmailSendInput{
			Event:      NotificationEmailEventTicketCreated,
			SourceType: "support_ticket",
			SourceID:   strconv.FormatInt(ticket.ID, 10),
			Variables:  vars,
		}, nil)
	})
}

func (s *TicketService) notifyTicketReplied(ticket *Ticket, msg *TicketMessage, fromAdmin bool) {
	if s == nil || ticket == nil || msg == nil {
		return
	}
	go s.dispatchTicketEmails(func(ctx context.Context) {
		vars := s.ticketCommonVariables(ctx, ticket, fromAdmin)
		vars["ticket_content"] = truncateTicketEmailContent(msg.Content)
		input := NotificationEmailSendInput{
			Event:      NotificationEmailEventTicketReplied,
			SourceType: "support_ticket_message",
			SourceID:   strconv.FormatInt(msg.ID, 10),
			Variables:  vars,
		}
		if fromAdmin {
			vars["replier_name"] = firstNonEmpty(msg.UserName, msg.UserEmail, "Admin")
			s.sendToTicketUser(ctx, ticket, input, func(locale string, v map[string]string) {
				v["replier_role"] = ticketActorLabel(locale, true)
			})
			return
		}
		vars["replier_name"] = firstNonEmpty(ticket.UserName, ticket.UserEmail, "User")
		s.sendToAdmins(ctx, input, func(locale string, v map[string]string) {
			v["replier_role"] = ticketActorLabel(locale, false)
		})
	})
}

func (s *TicketService) notifyTicketClosed(ticket *Ticket, closedByAdmin bool) {
	if s == nil || ticket == nil {
		return
	}
	go s.dispatchTicketEmails(func(ctx context.Context) {
		vars := s.ticketCommonVariables(ctx, ticket, closedByAdmin)
		input := NotificationEmailSendInput{
			Event:      NotificationEmailEventTicketClosed,
			SourceType: "support_ticket",
			SourceID:   strconv.FormatInt(ticket.ID, 10) + ":closed",
			Variables:  vars,
		}
		if closedByAdmin {
			s.sendToTicketUser(ctx, ticket, input, func(locale string, v map[string]string) {
				v["closed_by"] = ticketActorLabel(locale, true)
			})
			return
		}
		s.sendToAdmins(ctx, input, func(locale string, v map[string]string) {
			v["closed_by"] = ticketActorLabel(locale, false)
		})
	})
}

func (s *TicketService) dispatchTicketEmails(fn func(ctx context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in ticket email notification", "recover", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), emailSendTimeout)
	defer cancel()
	fn(ctx)
}

func (s *TicketService) ticketCommonVariables(ctx context.Context, ticket *Ticket, forAdminLink bool) map[string]string {
	return map[string]string{
		"ticket_id":      strconv.FormatInt(ticket.ID, 10),
		"ticket_subject": ticket.Subject,
		"user_email":     ticket.UserEmail,
		"user_name":      firstNonEmpty(ticket.UserName, ticket.UserEmail),
		"ticket_url":     s.ticketURL(ctx, forAdminLink),
	}
}

func (s *TicketService) ticketURL(ctx context.Context, forAdmin bool) string {
	base := strings.TrimRight(s.frontendURL(ctx), "/")
	path := "/tickets"
	if forAdmin {
		path = "/admin/tickets"
	}
	if base == "" {
		return path
	}
	return base + path
}

func (s *TicketService) frontendURL(ctx context.Context) string {
	if s == nil || s.settingRepo == nil {
		return ""
	}
	val, err := s.settingRepo.GetValue(ctx, SettingKeyFrontendURL)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(val)
}

type ticketEmailVarEnricher func(locale string, vars map[string]string)

func (s *TicketService) sendToTicketUser(ctx context.Context, ticket *Ticket, input NotificationEmailSendInput, enrich ticketEmailVarEnricher) {
	if s == nil || s.notificationEmailService == nil || ticket == nil {
		return
	}
	email := strings.TrimSpace(ticket.UserEmail)
	if email == "" {
		return
	}
	input.RecipientEmail = email
	input.RecipientName = firstNonEmpty(ticket.UserName, email)
	input.UserID = ticket.UserID
	input.Variables = cloneTicketEmailVars(input.Variables)
	locale := s.notificationEmailService.ResolveRecipientLocale(ctx, ticket.UserID, email)
	input.Locale = locale
	if enrich != nil {
		enrich(locale, input.Variables)
	}
	if err := s.notificationEmailService.Send(ctx, input); err != nil {
		slog.Warn("ticket notification email failed",
			"event", input.Event,
			"ticket_id", ticket.ID,
			"recipient_role", "user",
			"err", err.Error(),
		)
	}
}

func (s *TicketService) sendToAdmins(ctx context.Context, input NotificationEmailSendInput, enrich ticketEmailVarEnricher) {
	if s == nil || s.notificationEmailService == nil {
		return
	}
	for _, admin := range s.listActiveAdmins(ctx) {
		email := strings.TrimSpace(admin.Email)
		if email == "" {
			continue
		}
		adminInput := input
		adminInput.RecipientEmail = email
		adminInput.RecipientName = firstNonEmpty(admin.Username, email)
		adminInput.UserID = admin.ID
		adminInput.Variables = cloneTicketEmailVars(input.Variables)
		locale := s.notificationEmailService.ResolveRecipientLocale(ctx, admin.ID, email)
		adminInput.Locale = locale
		if enrich != nil {
			enrich(locale, adminInput.Variables)
		}
		if err := s.notificationEmailService.Send(ctx, adminInput); err != nil {
			slog.Warn("ticket notification email failed",
				"event", input.Event,
				"source_id", input.SourceID,
				"recipient_role", "admin",
				"err", err.Error(),
			)
		}
	}
}

func cloneTicketEmailVars(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func ticketActorLabel(locale string, isAdmin bool) string {
	if normalizeNotificationLocale(locale) == notificationEmailLocaleChinese {
		if isAdmin {
			return "管理员"
		}
		return "用户"
	}
	if isAdmin {
		return "admin"
	}
	return "user"
}

func (s *TicketService) listActiveAdmins(ctx context.Context) []User {
	if s == nil || s.userRepo == nil {
		return nil
	}
	noSubs := false
	users, _, err := s.userRepo.ListWithFilters(ctx,
		pagination.PaginationParams{Page: 1, PageSize: ticketAdminNotifyPageSize},
		UserListFilters{
			Role:                 RoleAdmin,
			Status:               StatusActive,
			IncludeSubscriptions: &noSubs,
		},
	)
	if err != nil {
		slog.Warn("list admin users for ticket email failed", "err", err.Error())
		return nil
	}
	return users
}

func truncateTicketEmailContent(content string) string {
	trimmed := strings.TrimSpace(content)
	if utf8.RuneCountInString(trimmed) <= ticketEmailContentMaxRunes {
		return trimmed
	}
	runes := []rune(trimmed)
	return string(runes[:ticketEmailContentMaxRunes]) + "…"
}
