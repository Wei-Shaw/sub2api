package service

import (
	"context"
	"testing"
	"time"
)

type ticketRepoStub struct {
	created         *Ticket
	message         *TicketMessage
	listCalls       int
	listByUserCalls int
}

func (r *ticketRepoStub) Create(_ context.Context, input CreateTicketInput) (*Ticket, *TicketMessage, error) {
	now := time.Now().UTC()
	r.created = &Ticket{ID: 1, UserID: input.UserID, Title: input.Title, Description: input.Description, Status: TicketStatusPending, CreatedAt: now, UpdatedAt: now}
	r.message = &TicketMessage{ID: 1, TicketID: 1, SenderType: TicketSenderUser, SenderID: input.UserID, Content: input.Description, Images: input.Images, CreatedAt: now}
	return r.created, r.message, nil
}
func (r *ticketRepoStub) GetByID(context.Context, int64) (*Ticket, error) { return r.created, nil }
func (r *ticketRepoStub) ListByUser(context.Context, int64) ([]Ticket, error) {
	r.listByUserCalls++
	return nil, nil
}
func (r *ticketRepoStub) List(context.Context) ([]Ticket, error) {
	r.listCalls++
	return []Ticket{{ID: 1, UserID: 7}, {ID: 2, UserID: 8}}, nil
}
func (r *ticketRepoStub) AddMessage(context.Context, AddTicketMessageInput) (*TicketMessage, error) {
	return r.message, nil
}
func (r *ticketRepoStub) Close(context.Context, int64, int64) error { return nil }
func (r *ticketRepoStub) LastMessageBySender(context.Context, int64, string) (*TicketMessage, error) {
	return nil, nil
}

type ticketSettingStub struct{ values map[string]string }

func (s *ticketSettingStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (s *ticketSettingStub) GetValue(_ context.Context, key string) (string, error) {
	v, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}
func (s *ticketSettingStub) Set(context.Context, string, string) error { return nil }
func (s *ticketSettingStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return s.values, nil
}
func (s *ticketSettingStub) SetMultiple(context.Context, map[string]string) error { return nil }
func (s *ticketSettingStub) GetAll(context.Context) (map[string]string, error)    { return s.values, nil }
func (s *ticketSettingStub) Delete(context.Context, string) error                 { return nil }

func TestTicketServiceCreatePersistsPendingTicketAndInitialMessage(t *testing.T) {
	repo := &ticketRepoStub{}
	settings := &ticketSettingStub{values: map[string]string{SettingKeyTicketSystemEnabled: "true"}}
	svc := NewTicketService(repo, nil, settings, nil)
	ticket, err := svc.Create(context.Background(), CreateTicketInput{UserID: 7, Title: "API error", Description: "Please investigate", Images: []string{"https://example.com/a.png"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if ticket.Status != TicketStatusPending || repo.message.SenderType != TicketSenderUser {
		t.Fatalf("unexpected initial ticket state: %#v / %#v", ticket, repo.message)
	}
}

func TestTicketServiceRejectsWhenDisabled(t *testing.T) {
	svc := NewTicketService(&ticketRepoStub{}, nil, &ticketSettingStub{values: map[string]string{SettingKeyTicketSystemEnabled: "false"}}, nil)
	_, err := svc.Create(context.Background(), CreateTicketInput{UserID: 7, Title: "API error", Description: "Please investigate"})
	if err != ErrTicketSystemDisabled {
		t.Fatalf("Create() error = %v, want disabled", err)
	}
}

func TestTicketServiceAdminListReturnsTicketsFromAllUsers(t *testing.T) {
	repo := &ticketRepoStub{}
	svc := NewTicketService(repo, nil, &ticketSettingStub{values: map[string]string{SettingKeyTicketSystemEnabled: "true"}}, nil)
	items, err := svc.List(context.Background(), 1, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 || repo.listCalls != 1 || repo.listByUserCalls != 0 {
		t.Fatalf("admin list used the wrong scope: items=%d list=%d listByUser=%d", len(items), repo.listCalls, repo.listByUserCalls)
	}
}
