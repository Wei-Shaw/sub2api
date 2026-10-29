package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type ticketRepository struct {
	db *sql.DB
}

func NewTicketRepository(db *sql.DB) service.TicketRepository {
	return &ticketRepository{db: db}
}

func (r *ticketRepository) Create(ctx context.Context, userID int64, subject, content string) (*service.Ticket, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var ticket service.Ticket
	err = tx.QueryRowContext(ctx, `
		INSERT INTO support_tickets (user_id, subject, status, last_replied_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, subject, status, last_replied_by, created_at, updated_at, closed_at
	`, userID, subject, service.TicketStatusOpen, service.TicketReplierUser).Scan(
		&ticket.ID, &ticket.UserID, &ticket.Subject, &ticket.Status, &ticket.LastRepliedBy,
		&ticket.CreatedAt, &ticket.UpdatedAt, &ticket.ClosedAt,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO support_ticket_messages (ticket_id, user_id, is_admin, content)
		VALUES ($1, $2, FALSE, $3)
	`, ticket.ID, userID, content)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ticket.MessageCount = 1
	return &ticket, nil
}

func (r *ticketRepository) GetByID(ctx context.Context, id int64) (*service.Ticket, error) {
	var ticket service.Ticket
	var userEmail, userName sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT t.id, t.user_id, COALESCE(u.email, ''), COALESCE(NULLIF(u.username, ''), u.email, ''),
		       t.subject, t.status, t.last_replied_by, t.created_at, t.updated_at, t.closed_at,
		       COALESCE((SELECT COUNT(*) FROM support_ticket_messages m WHERE m.ticket_id = t.id), 0)
		FROM support_tickets t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE t.id = $1
	`, id).Scan(
		&ticket.ID, &ticket.UserID, &userEmail, &userName,
		&ticket.Subject, &ticket.Status, &ticket.LastRepliedBy,
		&ticket.CreatedAt, &ticket.UpdatedAt, &ticket.ClosedAt, &ticket.MessageCount,
	)
	if err == sql.ErrNoRows {
		return nil, service.ErrTicketNotFound
	}
	if err != nil {
		return nil, err
	}
	ticket.UserEmail = userEmail.String
	ticket.UserName = userName.String
	return &ticket, nil
}

func (r *ticketRepository) List(ctx context.Context, params pagination.PaginationParams, filters service.TicketListFilters) ([]service.Ticket, *pagination.PaginationResult, error) {
	where := []string{"1=1"}
	args := make([]any, 0, 4)
	argN := 1
	if filters.UserID > 0 {
		where = append(where, fmt.Sprintf("t.user_id = $%d", argN))
		args = append(args, filters.UserID)
		argN++
	}
	if status := strings.TrimSpace(filters.Status); status != "" {
		where = append(where, fmt.Sprintf("t.status = $%d", argN))
		args = append(args, status)
		argN++
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		where = append(where, fmt.Sprintf("(t.subject ILIKE $%d OR COALESCE(u.email, '') ILIKE $%d OR COALESCE(u.username, '') ILIKE $%d)", argN, argN, argN))
		args = append(args, "%"+search+"%")
		argN++
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	countQuery := `
		SELECT COUNT(*)
		FROM support_tickets t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE ` + whereSQL
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, nil, err
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortCol := "t.updated_at"
	switch sortBy {
	case "created_at":
		sortCol = "t.created_at"
	case "status":
		sortCol = "t.status"
	case "id":
		sortCol = "t.id"
	case "subject":
		sortCol = "t.subject"
	}
	sortOrder := "DESC"
	if strings.EqualFold(params.SortOrder, "asc") {
		sortOrder = "ASC"
	}

	listArgs := append(append([]any{}, args...), pageSize, offset)
	listQuery := fmt.Sprintf(`
		SELECT t.id, t.user_id, COALESCE(u.email, ''), COALESCE(NULLIF(u.username, ''), u.email, ''),
		       t.subject, t.status, t.last_replied_by, t.created_at, t.updated_at, t.closed_at,
		       COALESCE((SELECT COUNT(*) FROM support_ticket_messages m WHERE m.ticket_id = t.id), 0)
		FROM support_tickets t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE %s
		ORDER BY %s %s, t.id DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, sortCol, sortOrder, argN, argN+1)

	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.Ticket, 0, pageSize)
	for rows.Next() {
		var ticket service.Ticket
		if err := rows.Scan(
			&ticket.ID, &ticket.UserID, &ticket.UserEmail, &ticket.UserName,
			&ticket.Subject, &ticket.Status, &ticket.LastRepliedBy,
			&ticket.CreatedAt, &ticket.UpdatedAt, &ticket.ClosedAt, &ticket.MessageCount,
		); err != nil {
			return nil, nil, err
		}
		out = append(out, ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, &pagination.PaginationResult{Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *ticketRepository) ListMessages(ctx context.Context, ticketID int64) ([]service.TicketMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.ticket_id, m.user_id, COALESCE(u.email, ''), COALESCE(NULLIF(u.username, ''), u.email, ''),
		       m.is_admin, m.content, m.created_at
		FROM support_ticket_messages m
		LEFT JOIN users u ON u.id = m.user_id
		WHERE m.ticket_id = $1
		ORDER BY m.created_at ASC, m.id ASC
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.TicketMessage, 0)
	for rows.Next() {
		var msg service.TicketMessage
		if err := rows.Scan(
			&msg.ID, &msg.TicketID, &msg.UserID, &msg.UserEmail, &msg.UserName,
			&msg.IsAdmin, &msg.Content, &msg.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (r *ticketRepository) AddMessage(ctx context.Context, ticketID, userID int64, isAdmin bool, content, nextStatus, lastRepliedBy string) (*service.TicketMessage, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var msg service.TicketMessage
	err = tx.QueryRowContext(ctx, `
		INSERT INTO support_ticket_messages (ticket_id, user_id, is_admin, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, ticket_id, user_id, is_admin, content, created_at
	`, ticketID, userID, isAdmin, content).Scan(
		&msg.ID, &msg.TicketID, &msg.UserID, &msg.IsAdmin, &msg.Content, &msg.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE support_tickets
		SET status = $2,
		    last_replied_by = $3,
		    updated_at = NOW(),
		    closed_at = NULL
		WHERE id = $1 AND status <> $4
	`, ticketID, nextStatus, lastRepliedBy, service.TicketStatusClosed)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, service.ErrTicketClosed
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *ticketRepository) UpdateStatus(ctx context.Context, ticketID int64, status string, closedAt *time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE support_tickets
		SET status = $2,
		    closed_at = $3,
		    updated_at = NOW()
		WHERE id = $1
	`, ticketID, status, closedAt)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrTicketNotFound
	}
	return nil
}
