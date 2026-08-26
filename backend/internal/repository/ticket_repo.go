package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type ticketRepository struct{ db *sql.DB }

func NewTicketRepository(db *sql.DB) service.TicketRepository { return &ticketRepository{db: db} }

func ticketImages(images []string) (string, error) {
	if images == nil {
		images = []string{}
	}
	b, err := json.Marshal(images)
	return string(b), err
}

func scanTicket(row interface{ Scan(...any) error }) (*service.Ticket, error) {
	var t service.Ticket
	var closedAt sql.NullTime
	var closedBy sql.NullInt64
	if err := row.Scan(&t.ID, &t.UserID, &t.UserEmail, &t.UserName, &t.Title, &t.Description, &t.Status, &t.CreatedAt, &t.UpdatedAt, &closedAt, &closedBy); err != nil {
		return nil, err
	}
	if closedAt.Valid {
		t.ClosedAt = &closedAt.Time
	}
	if closedBy.Valid {
		t.ClosedBy = &closedBy.Int64
	}
	t.Messages = []service.TicketMessage{}
	return &t, nil
}

const ticketColumns = `t.id, t.user_id, u.email, COALESCE(u.username, ''), t.title, t.description, t.status, t.created_at, t.updated_at, t.closed_at, t.closed_by`

func (r *ticketRepository) Create(ctx context.Context, input service.CreateTicketInput) (*service.Ticket, *service.TicketMessage, error) {
	images, err := ticketImages(input.Images)
	if err != nil {
		return nil, nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	var ticketID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO tickets (user_id, title, description) VALUES ($1,$2,$3) RETURNING id`, input.UserID, input.Title, input.Description).Scan(&ticketID); err != nil {
		return nil, nil, err
	}
	var messageID int64
	var createdAt time.Time
	if err = tx.QueryRowContext(ctx, `INSERT INTO ticket_messages (ticket_id, sender_type, sender_id, content, images) VALUES ($1,'user',$2,$3,$4) RETURNING id, created_at`, ticketID, input.UserID, input.Description, images).Scan(&messageID, &createdAt); err != nil {
		return nil, nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}
	ticket, err := r.GetByID(ctx, ticketID)
	if err != nil {
		return nil, nil, err
	}
	return ticket, &service.TicketMessage{ID: messageID, TicketID: ticketID, SenderType: service.TicketSenderUser, SenderID: input.UserID, Content: input.Description, Images: input.Images, CreatedAt: createdAt}, nil
}

func (r *ticketRepository) GetByID(ctx context.Context, id int64) (*service.Ticket, error) {
	t, err := scanTicket(r.db.QueryRowContext(ctx, `SELECT `+ticketColumns+` FROM tickets t JOIN users u ON u.id=t.user_id WHERE t.id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrTicketNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, ticket_id, sender_type, sender_id, content, images, created_at FROM ticket_messages WHERE ticket_id=$1 ORDER BY created_at ASC, id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m service.TicketMessage
		var raw string
		if err := rows.Scan(&m.ID, &m.TicketID, &m.SenderType, &m.SenderID, &m.Content, &raw, &m.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &m.Images)
		t.Messages = append(t.Messages, m)
	}
	return t, rows.Err()
}

func (r *ticketRepository) ListByUser(ctx context.Context, userID int64) ([]service.Ticket, error) {
	return r.list(ctx, `WHERE t.user_id=$1`, userID)
}
func (r *ticketRepository) List(ctx context.Context) ([]service.Ticket, error) {
	return r.list(ctx, ``, nil)
}
func (r *ticketRepository) list(ctx context.Context, where string, arg any) ([]service.Ticket, error) {
	query := `SELECT ` + ticketColumns + ` FROM tickets t JOIN users u ON u.id=t.user_id ` + where + ` ORDER BY t.updated_at DESC, t.id DESC`
	var rows *sql.Rows
	var err error
	if arg == nil {
		rows, err = r.db.QueryContext(ctx, query)
	} else {
		rows, err = r.db.QueryContext(ctx, query, arg)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r *ticketRepository) AddMessage(ctx context.Context, input service.AddTicketMessageInput) (*service.TicketMessage, error) {
	images, err := ticketImages(input.Images)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id int64
	var createdAt time.Time
	if err = tx.QueryRowContext(ctx, `INSERT INTO ticket_messages (ticket_id,sender_type,sender_id,content,images) VALUES ($1,$2,$3,$4,$5) RETURNING id,created_at`, input.TicketID, input.SenderType, input.SenderID, input.Content, images).Scan(&id, &createdAt); err != nil {
		return nil, err
	}
	if input.SenderType == service.TicketSenderAdmin {
		if _, err = tx.ExecContext(ctx, `UPDATE tickets SET status=CASE WHEN status='pending' THEN 'processing' ELSE status END, updated_at=NOW() WHERE id=$1`, input.TicketID); err != nil {
			return nil, err
		}
	} else {
		if _, err = tx.ExecContext(ctx, `UPDATE tickets SET updated_at=NOW() WHERE id=$1`, input.TicketID); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &service.TicketMessage{ID: id, TicketID: input.TicketID, SenderType: input.SenderType, SenderID: input.SenderID, Content: input.Content, Images: input.Images, CreatedAt: createdAt}, nil
}

func (r *ticketRepository) Close(ctx context.Context, ticketID, actorID int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE tickets SET status='closed', closed_at=NOW(), closed_by=$2, updated_at=NOW() WHERE id=$1 AND status<>'closed'`, ticketID, actorID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists int
		if err := r.db.QueryRowContext(ctx, `SELECT 1 FROM tickets WHERE id=$1`, ticketID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return service.ErrTicketNotFound
		}
	}
	return nil
}

func (r *ticketRepository) LastMessageBySender(ctx context.Context, ticketID int64, senderType string) (*service.TicketMessage, error) {
	var m service.TicketMessage
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT id,ticket_id,sender_type,sender_id,content,images,created_at FROM ticket_messages WHERE ticket_id=$1 AND sender_type=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, ticketID, senderType).Scan(&m.ID, &m.TicketID, &m.SenderType, &m.SenderID, &m.Content, &raw, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get last ticket message: %w", err)
	}
	_ = json.Unmarshal([]byte(raw), &m.Images)
	return &m, nil
}
