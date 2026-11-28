//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type ticketEmailUserRepoStub struct {
	userRepoStub
	admins []User
}

func (s *ticketEmailUserRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	out := make([]User, 0, len(s.admins))
	for _, admin := range s.admins {
		if filters.Role != "" && admin.Role != filters.Role {
			continue
		}
		if filters.Status != "" && admin.Status != filters.Status {
			continue
		}
		out = append(out, admin)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: 1, PageSize: params.PageSize}, nil
}

func TestTruncateTicketEmailContent(t *testing.T) {
	require.Equal(t, "hello", truncateTicketEmailContent("  hello  "))
	long := strings.Repeat("字", ticketEmailContentMaxRunes+10)
	got := truncateTicketEmailContent(long)
	require.True(t, strings.HasSuffix(got, "…"))
	require.Equal(t, ticketEmailContentMaxRunes+1, len([]rune(got)))
}

func TestTicketActorLabel(t *testing.T) {
	require.Equal(t, "admin", ticketActorLabel("en", true))
	require.Equal(t, "user", ticketActorLabel("en", false))
	require.Equal(t, "管理员", ticketActorLabel("zh", true))
	require.Equal(t, "用户", ticketActorLabel("zh-CN", false))
}

func TestTicketServiceSendsCreatedAndClosedEmails(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	smtpServer := startNotificationEmailTestSMTPServer(t)
	require.NoError(t, repo.SetMultiple(ctx, smtpServer.settings()))
	require.NoError(t, repo.Set(ctx, SettingKeyFrontendURL, "https://app.example.com"))

	emailService := NewEmailService(repo, nil)
	notificationService := NewNotificationEmailService(repo, emailService)
	userRepo := &ticketEmailUserRepoStub{
		admins: []User{
			{ID: 9, Email: "admin@example.com", Username: "Admin", Role: RoleAdmin, Status: StatusActive},
		},
	}
	svc := ProvideTicketService(nil, userRepo, repo, notificationService)

	ticket := &Ticket{
		ID:        7,
		UserID:    3,
		UserEmail: "user@example.com",
		UserName:  "Alice",
		Subject:   "Need help",
	}

	svc.sendToAdmins(ctx, NotificationEmailSendInput{
		Event:      NotificationEmailEventTicketCreated,
		SourceType: "support_ticket",
		SourceID:   "7",
		Variables: map[string]string{
			"ticket_id":      "7",
			"ticket_subject": "Need help",
			"ticket_content": "Something broke",
			"ticket_url":     "https://app.example.com/admin/tickets",
			"user_email":     "user@example.com",
			"user_name":      "Alice",
		},
	}, nil)
	require.Eventually(t, func() bool {
		return smtpServer.messageCount() >= 1
	}, 2*time.Second, 20*time.Millisecond)
	body := smtpServer.lastMessageBody(t)
	require.Contains(t, body, "Need help")
	require.Contains(t, body, "Something broke")

	beforeClose := smtpServer.messageCount()
	svc.sendToTicketUser(ctx, ticket, NotificationEmailSendInput{
		Event:      NotificationEmailEventTicketClosed,
		SourceType: "support_ticket",
		SourceID:   "7:closed",
		Variables: map[string]string{
			"ticket_id":      "7",
			"ticket_subject": "Need help",
			"ticket_url":     "https://app.example.com/tickets",
			"user_email":     "user@example.com",
			"user_name":      "Alice",
		},
	}, func(locale string, vars map[string]string) {
		vars["closed_by"] = ticketActorLabel(locale, true)
	})
	require.Eventually(t, func() bool {
		return smtpServer.messageCount() > beforeClose
	}, 2*time.Second, 20*time.Millisecond)
	closedBody := smtpServer.lastMessageBody(t)
	require.Contains(t, closedBody, "closed")
}
