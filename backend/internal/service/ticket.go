package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	TicketStatusOpen         = "open"
	TicketStatusWaitingAdmin = "waiting_admin"
	TicketStatusAnswered     = "answered"
	TicketStatusClosed       = "closed"

	TicketReplierUser  = "user"
	TicketReplierAdmin = "admin"

	ticketSubjectMaxLen = 200
	ticketContentMaxLen = 10000
)

var (
	ErrTicketNotFound = infraerrors.NotFound("TICKET_NOT_FOUND", "ticket not found")
	ErrTicketClosed   = infraerrors.BadRequest("TICKET_CLOSED", "ticket is closed")
	ErrTicketForbidden = infraerrors.Forbidden("TICKET_FORBIDDEN", "you do not have access to this ticket")
)

type Ticket struct {
	ID            int64      `json:"id"`
	UserID        int64      `json:"user_id"`
	UserEmail     string     `json:"user_email,omitempty"`
	UserName      string     `json:"user_name,omitempty"`
	Subject       string     `json:"subject"`
	Status        string     `json:"status"`
	LastRepliedBy string     `json:"last_replied_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
	MessageCount  int        `json:"message_count,omitempty"`
	Messages      []TicketMessage `json:"messages,omitempty"`
}

type TicketMessage struct {
	ID        int64     `json:"id"`
	TicketID  int64     `json:"ticket_id"`
	UserID    int64     `json:"user_id"`
	UserEmail string    `json:"user_email,omitempty"`
	UserName  string    `json:"user_name,omitempty"`
	IsAdmin   bool      `json:"is_admin"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type TicketListFilters struct {
	Status string
	Search string
	UserID int64 // 0 = all users (admin)
}

type CreateTicketInput struct {
	Subject string
	Content string
}

type ReplyTicketInput struct {
	Content string
}

type TicketRepository interface {
	Create(ctx context.Context, userID int64, subject, content string) (*Ticket, error)
	GetByID(ctx context.Context, id int64) (*Ticket, error)
	List(ctx context.Context, params pagination.PaginationParams, filters TicketListFilters) ([]Ticket, *pagination.PaginationResult, error)
	ListMessages(ctx context.Context, ticketID int64) ([]TicketMessage, error)
	AddMessage(ctx context.Context, ticketID, userID int64, isAdmin bool, content, nextStatus, lastRepliedBy string) (*TicketMessage, error)
	UpdateStatus(ctx context.Context, ticketID int64, status string, closedAt *time.Time) error
}

type TicketService struct {
	repo TicketRepository
}

func NewTicketService(repo TicketRepository) *TicketService {
	return &TicketService{repo: repo}
}

func (s *TicketService) Create(ctx context.Context, userID int64, input *CreateTicketInput) (*Ticket, error) {
	subject, err := normalizeTicketSubject(input.Subject)
	if err != nil {
		return nil, err
	}
	content, err := normalizeTicketContent(input.Content)
	if err != nil {
		return nil, err
	}
	ticket, err := s.repo.Create(ctx, userID, subject, content)
	if err != nil {
		return nil, err
	}
	messages, err := s.repo.ListMessages(ctx, ticket.ID)
	if err == nil {
		ticket.Messages = messages
		ticket.MessageCount = len(messages)
	}
	return ticket, nil
}

func (s *TicketService) ListForUser(ctx context.Context, userID int64, params pagination.PaginationParams, status, search string) ([]Ticket, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params, TicketListFilters{Status: status, Search: search, UserID: userID})
}

func (s *TicketService) ListForAdmin(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Ticket, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params, TicketListFilters{Status: status, Search: search})
}

func (s *TicketService) GetForUser(ctx context.Context, userID, ticketID int64) (*Ticket, error) {
	ticket, err := s.getWithMessages(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.UserID != userID {
		return nil, ErrTicketForbidden
	}
	return ticket, nil
}

func (s *TicketService) GetForAdmin(ctx context.Context, ticketID int64) (*Ticket, error) {
	return s.getWithMessages(ctx, ticketID)
}

func (s *TicketService) ReplyAsUser(ctx context.Context, userID, ticketID int64, input *ReplyTicketInput) (*Ticket, error) {
	ticket, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.UserID != userID {
		return nil, ErrTicketForbidden
	}
	if ticket.Status == TicketStatusClosed {
		return nil, ErrTicketClosed
	}
	content, err := normalizeTicketContent(input.Content)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.AddMessage(ctx, ticketID, userID, false, content, TicketStatusWaitingAdmin, TicketReplierUser); err != nil {
		return nil, err
	}
	return s.getWithMessages(ctx, ticketID)
}

func (s *TicketService) ReplyAsAdmin(ctx context.Context, adminUserID, ticketID int64, input *ReplyTicketInput) (*Ticket, error) {
	ticket, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.Status == TicketStatusClosed {
		return nil, ErrTicketClosed
	}
	content, err := normalizeTicketContent(input.Content)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.AddMessage(ctx, ticketID, adminUserID, true, content, TicketStatusAnswered, TicketReplierAdmin); err != nil {
		return nil, err
	}
	return s.getWithMessages(ctx, ticketID)
}

func (s *TicketService) CloseAsUser(ctx context.Context, userID, ticketID int64) (*Ticket, error) {
	ticket, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.UserID != userID {
		return nil, ErrTicketForbidden
	}
	if ticket.Status == TicketStatusClosed {
		return ticket, nil
	}
	now := time.Now().UTC()
	if err := s.repo.UpdateStatus(ctx, ticketID, TicketStatusClosed, &now); err != nil {
		return nil, err
	}
	return s.getWithMessages(ctx, ticketID)
}

func (s *TicketService) CloseAsAdmin(ctx context.Context, ticketID int64) (*Ticket, error) {
	ticket, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.Status == TicketStatusClosed {
		return s.getWithMessages(ctx, ticketID)
	}
	now := time.Now().UTC()
	if err := s.repo.UpdateStatus(ctx, ticketID, TicketStatusClosed, &now); err != nil {
		return nil, err
	}
	return s.getWithMessages(ctx, ticketID)
}

func (s *TicketService) ReopenAsAdmin(ctx context.Context, ticketID int64) (*Ticket, error) {
	ticket, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	next := TicketStatusWaitingAdmin
	if ticket.LastRepliedBy == TicketReplierAdmin {
		next = TicketStatusAnswered
	} else if ticket.LastRepliedBy == TicketReplierUser && ticket.Status == TicketStatusClosed {
		next = TicketStatusWaitingAdmin
	} else if ticket.MessageCount <= 1 {
		next = TicketStatusOpen
	}
	if err := s.repo.UpdateStatus(ctx, ticketID, next, nil); err != nil {
		return nil, err
	}
	return s.getWithMessages(ctx, ticketID)
}

func (s *TicketService) getWithMessages(ctx context.Context, ticketID int64) (*Ticket, error) {
	ticket, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	messages, err := s.repo.ListMessages(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	ticket.Messages = messages
	ticket.MessageCount = len(messages)
	return ticket, nil
}

func normalizeTicketSubject(raw string) (string, error) {
	subject := strings.TrimSpace(raw)
	if subject == "" {
		return "", infraerrors.BadRequest("TICKET_SUBJECT_REQUIRED", "subject is required")
	}
	if utf8.RuneCountInString(subject) > ticketSubjectMaxLen {
		return "", infraerrors.BadRequest("TICKET_SUBJECT_TOO_LONG", "subject is too long")
	}
	return subject, nil
}

func normalizeTicketContent(raw string) (string, error) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return "", infraerrors.BadRequest("TICKET_CONTENT_REQUIRED", "content is required")
	}
	if utf8.RuneCountInString(content) > ticketContentMaxLen {
		return "", infraerrors.BadRequest("TICKET_CONTENT_TOO_LONG", "content is too long")
	}
	return content, nil
}
