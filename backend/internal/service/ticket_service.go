package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	TicketStatusPending    = "pending"
	TicketStatusProcessing = "processing"
	TicketStatusClosed     = "closed"
	TicketSenderUser       = "user"
	TicketSenderAdmin      = "admin"
	TicketNotificationGap  = 30 * time.Minute
)

var (
	ErrTicketNotFound       = errors.New("ticket not found")
	ErrTicketClosed         = errors.New("ticket is closed")
	ErrTicketSystemDisabled = errors.New("ticket system is disabled")
	ErrTicketInvalidTitle   = errors.New("ticket title is required and must be at most 200 characters")
	ErrTicketInvalidContent = errors.New("ticket content is required")
	ErrTicketInvalidImages  = errors.New("ticket images are invalid")
	ErrTicketNotClosed      = errors.New("ticket must be closed before deletion")
)

type Ticket struct {
	ID          int64           `json:"id"`
	UserID      int64           `json:"user_id"`
	UserEmail   string          `json:"user_email,omitempty"`
	UserName    string          `json:"user_name,omitempty"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ClosedAt    *time.Time      `json:"closed_at,omitempty"`
	ClosedBy    *int64          `json:"closed_by,omitempty"`
	Messages    []TicketMessage `json:"messages,omitempty"`
}

type TicketMessage struct {
	ID         int64     `json:"id"`
	TicketID   int64     `json:"ticket_id"`
	SenderType string    `json:"sender_type"`
	SenderID   int64     `json:"sender_id"`
	Content    string    `json:"content"`
	Images     []string  `json:"images"`
	CreatedAt  time.Time `json:"created_at"`
}

type TicketListResult struct {
	Items    []Ticket
	Total    int64
	Page     int
	PageSize int
}

type CreateTicketInput struct {
	UserID      int64
	Title       string
	Description string
	Images      []string
}

type AddTicketMessageInput struct {
	TicketID   int64
	SenderID   int64
	SenderType string
	Content    string
	Images     []string
}

type TicketRepository interface {
	Create(ctx context.Context, input CreateTicketInput) (*Ticket, *TicketMessage, error)
	GetByID(ctx context.Context, id int64) (*Ticket, error)
	ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]Ticket, int64, error)
	List(ctx context.Context, params pagination.PaginationParams) ([]Ticket, int64, error)
	AddMessage(ctx context.Context, input AddTicketMessageInput) (*TicketMessage, error)
	Close(ctx context.Context, ticketID, actorID int64) error
	Delete(ctx context.Context, ticketID int64) error
	LastMessageBySender(ctx context.Context, ticketID int64, senderType string) (*TicketMessage, error)
}

type TicketService struct {
	repo         TicketRepository
	userRepo     UserRepository
	settingRepo  SettingRepository
	notification *NotificationEmailService
}

func NewTicketService(repo TicketRepository, userRepo UserRepository, settingRepo SettingRepository, notification *NotificationEmailService) *TicketService {
	return &TicketService{repo: repo, userRepo: userRepo, settingRepo: settingRepo, notification: notification}
}

func (s *TicketService) enabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return true
	}
	v, err := s.settingRepo.GetValue(ctx, SettingKeyTicketSystemEnabled)
	return err != nil || v == "true"
}

func validateTicketInput(title, content string, images []string) error {
	if title = strings.TrimSpace(title); title == "" || len([]rune(title)) > 200 {
		return ErrTicketInvalidTitle
	}
	if strings.TrimSpace(content) == "" {
		return ErrTicketInvalidContent
	}
	return validateTicketImages(images)
}

func validateTicketImages(images []string) error {
	if len(images) > 9 {
		return ErrTicketInvalidImages
	}
	for _, image := range images {
		image = strings.TrimSpace(image)
		// Ticket attachments are submitted as data URLs by the web client. Keep
		// each attachment bounded while allowing normal phone screenshots.
		if image == "" || len(image) > 5*1024*1024 {
			return ErrTicketInvalidImages
		}
	}
	return nil
}

func (s *TicketService) Create(ctx context.Context, input CreateTicketInput) (*Ticket, error) {
	if !s.enabled(ctx) {
		return nil, ErrTicketSystemDisabled
	}
	if err := validateTicketInput(input.Title, input.Description, input.Images); err != nil {
		return nil, err
	}
	ticket, message, err := s.repo.Create(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("create ticket: %w", err)
	}
	s.notifyAdmins(ctx, ticket, message)
	return ticket, nil
}

func (s *TicketService) Get(ctx context.Context, id int64, userID int64, admin bool) (*Ticket, error) {
	if !admin && !s.enabled(ctx) {
		return nil, ErrTicketSystemDisabled
	}
	ticket, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !admin && ticket.UserID != userID {
		return nil, ErrTicketNotFound
	}
	return ticket, nil
}

func (s *TicketService) List(ctx context.Context, userID int64, admin bool, params pagination.PaginationParams) (*TicketListResult, error) {
	if !admin && !s.enabled(ctx) {
		return nil, ErrTicketSystemDisabled
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	var items []Ticket
	var total int64
	var err error
	if admin {
		items, total, err = s.repo.List(ctx, params)
	} else {
		items, total, err = s.repo.ListByUser(ctx, userID, params)
	}
	if err != nil {
		return nil, err
	}
	return &TicketListResult{Items: items, Total: total, Page: params.Page, PageSize: params.PageSize}, nil
}

func (s *TicketService) AddMessage(ctx context.Context, input AddTicketMessageInput, userID int64, admin bool) (*TicketMessage, error) {
	if !s.enabled(ctx) {
		return nil, ErrTicketSystemDisabled
	}
	ticket, err := s.Get(ctx, input.TicketID, userID, admin)
	if err != nil {
		return nil, err
	}
	if ticket.Status == TicketStatusClosed {
		return nil, ErrTicketClosed
	}
	if input.SenderType != TicketSenderUser && input.SenderType != TicketSenderAdmin {
		return nil, errors.New("invalid ticket sender")
	}
	if input.SenderType == TicketSenderUser && admin || input.SenderType == TicketSenderAdmin && !admin {
		return nil, errors.New("invalid ticket sender")
	}
	var previousUserMessage *TicketMessage
	var previousAdminMessage *TicketMessage
	if input.SenderType == TicketSenderUser {
		previousUserMessage, err = s.repo.LastMessageBySender(ctx, ticket.ID, TicketSenderUser)
		if err != nil {
			return nil, fmt.Errorf("get previous ticket message: %w", err)
		}
	} else {
		previousAdminMessage, err = s.repo.LastMessageBySender(ctx, ticket.ID, TicketSenderAdmin)
		if err != nil {
			return nil, fmt.Errorf("get previous admin message: %w", err)
		}
	}
	if strings.TrimSpace(input.Content) == "" && len(input.Images) == 0 {
		return nil, ErrTicketInvalidContent
	}
	if err := validateTicketImages(input.Images); err != nil {
		return nil, err
	}
	message, err := s.repo.AddMessage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("add ticket message: %w", err)
	}
	if input.SenderType == TicketSenderAdmin {
		if ticket.Status == TicketStatusPending {
			ticket.Status = TicketStatusProcessing
		}
		if previousAdminMessage == nil || message.CreatedAt.Sub(previousAdminMessage.CreatedAt) >= TicketNotificationGap {
			s.notifyUser(ctx, ticket, message)
		}
	} else {
		s.notifyAdminsIfDelayed(ctx, ticket, message, previousUserMessage)
	}
	return message, nil
}

func (s *TicketService) Close(ctx context.Context, id, actorID int64, admin bool) error {
	ticket, err := s.Get(ctx, id, actorID, admin)
	if err != nil {
		return err
	}
	if ticket.Status == TicketStatusClosed {
		return nil
	}
	return s.repo.Close(ctx, ticket.ID, actorID)
}

func (s *TicketService) Delete(ctx context.Context, id int64, admin bool) error {
	if !admin {
		return ErrTicketNotFound
	}
	ticket, err := s.Get(ctx, id, 0, true)
	if err != nil {
		return err
	}
	if ticket.Status != TicketStatusClosed {
		return ErrTicketNotClosed
	}
	return s.repo.Delete(ctx, id)
}

func (s *TicketService) notifyAdmins(ctx context.Context, ticket *Ticket, message *TicketMessage) {
	if s.notification == nil || s.settingRepo == nil {
		return
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyTicketRecipientEmails)
	if err != nil {
		return
	}
	var recipients []string
	if json.Unmarshal([]byte(raw), &recipients) != nil {
		recipients = strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	}
	for _, recipient := range recipients {
		s.sendNotification(ctx, NotificationEmailSendInput{Event: NotificationEmailEventTicketAdmin, RecipientEmail: strings.TrimSpace(recipient), SourceType: "ticket_message", SourceID: fmt.Sprint(message.ID), Variables: ticketVariables(ticket, message)})
	}
}

func (s *TicketService) notifyAdminsIfDelayed(ctx context.Context, ticket *Ticket, message, last *TicketMessage) {
	if last == nil || message.CreatedAt.Sub(last.CreatedAt) >= TicketNotificationGap || message.CreatedAt.Sub(ticket.CreatedAt) >= TicketNotificationGap {
		s.notifyAdmins(ctx, ticket, message)
	}
}

func (s *TicketService) notifyUser(ctx context.Context, ticket *Ticket, message *TicketMessage) {
	if s.notification == nil || s.userRepo == nil {
		return
	}
	user, err := s.userRepo.GetByID(ctx, ticket.UserID)
	if err != nil || user == nil {
		return
	}
	s.sendNotification(ctx, NotificationEmailSendInput{Event: NotificationEmailEventTicketUser, RecipientEmail: user.Email, RecipientName: user.Username, UserID: user.ID, SourceType: "ticket_message", SourceID: fmt.Sprint(message.ID), Variables: ticketVariables(ticket, message)})
}

func (s *TicketService) sendNotification(ctx context.Context, input NotificationEmailSendInput) {
	if err := s.notification.Send(ctx, input); err != nil {
		slog.Warn("ticket notification suppressed", "event", input.Event, "err", err)
	}
}

func ticketVariables(ticket *Ticket, message *TicketMessage) map[string]string {
	images, _ := json.Marshal(message.Images)
	return map[string]string{"ticket_id": fmt.Sprint(ticket.ID), "ticket_title": ticket.Title, "ticket_description": ticket.Description, "ticket_status": ticket.Status, "message_content": message.Content, "message_images": string(images), "user_name": ticket.UserName, "user_email": ticket.UserEmail, "created_at": message.CreatedAt.Format(time.RFC3339)}
}
