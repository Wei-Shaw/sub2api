package dto

import "github.com/Wei-Shaw/sub2api/internal/service"

type TicketMessage struct {
	ID        int64  `json:"id"`
	TicketID  int64  `json:"ticket_id"`
	UserID    int64  `json:"user_id"`
	UserEmail string `json:"user_email,omitempty"`
	UserName  string `json:"user_name,omitempty"`
	IsAdmin   bool   `json:"is_admin"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

type Ticket struct {
	ID            int64           `json:"id"`
	UserID        int64           `json:"user_id"`
	UserEmail     string          `json:"user_email,omitempty"`
	UserName      string          `json:"user_name,omitempty"`
	Subject       string          `json:"subject"`
	Status        string          `json:"status"`
	LastRepliedBy string          `json:"last_replied_by"`
	CreatedAt     int64           `json:"created_at"`
	UpdatedAt     int64           `json:"updated_at"`
	ClosedAt      *int64          `json:"closed_at,omitempty"`
	MessageCount  int             `json:"message_count"`
	Messages      []TicketMessage `json:"messages,omitempty"`
}

func TicketFromService(t *service.Ticket) *Ticket {
	if t == nil {
		return nil
	}
	out := &Ticket{
		ID:            t.ID,
		UserID:        t.UserID,
		UserEmail:     t.UserEmail,
		UserName:      t.UserName,
		Subject:       t.Subject,
		Status:        t.Status,
		LastRepliedBy: t.LastRepliedBy,
		CreatedAt:     t.CreatedAt.Unix(),
		UpdatedAt:     t.UpdatedAt.Unix(),
		MessageCount:  t.MessageCount,
	}
	if t.ClosedAt != nil {
		v := t.ClosedAt.Unix()
		out.ClosedAt = &v
	}
	if len(t.Messages) > 0 {
		out.Messages = make([]TicketMessage, 0, len(t.Messages))
		for i := range t.Messages {
			out.Messages = append(out.Messages, TicketMessageFromService(&t.Messages[i]))
		}
	}
	return out
}

func TicketMessageFromService(m *service.TicketMessage) TicketMessage {
	if m == nil {
		return TicketMessage{}
	}
	return TicketMessage{
		ID:        m.ID,
		TicketID:  m.TicketID,
		UserID:    m.UserID,
		UserEmail: m.UserEmail,
		UserName:  m.UserName,
		IsAdmin:   m.IsAdmin,
		Content:   m.Content,
		CreatedAt: m.CreatedAt.Unix(),
	}
}

func TicketsFromService(items []service.Ticket) []Ticket {
	out := make([]Ticket, 0, len(items))
	for i := range items {
		out = append(out, *TicketFromService(&items[i]))
	}
	return out
}
