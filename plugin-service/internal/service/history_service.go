package service

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/repository"
)

var ErrHistoryForbidden = errors.New("history record is not accessible")

type HistoryService struct {
	repo HistoryRepository
}

type HistoryRepository interface {
	Create(ctx context.Context, principal model.CurrentPrincipal, prompt string, request map[string]any) (*model.HistoryRecord, error)
	Update(ctx context.Context, record *model.HistoryRecord) error
	Get(ctx context.Context, id string) (*model.HistoryRecord, bool, error)
	Delete(ctx context.Context, id string) error
	ListAll(ctx context.Context, query model.HistoryQuery) ([]model.HistoryRecord, error)
	ListByUser(ctx context.Context, userID int64, query model.HistoryQuery) ([]model.HistoryRecord, error)
	ListConversations(ctx context.Context, userID *int64, query model.CursorQuery) ([]model.ConversationSummary, error)
	ListConversationMessages(ctx context.Context, userID *int64, conversationID string, query model.CursorQuery) ([]model.HistoryRecord, error)
}

func (s *HistoryService) ListConversations(ctx context.Context, principal model.CurrentPrincipal, query model.CursorQuery) ([]model.ConversationSummary, error) {
	if principal.IsAdmin() {
		return s.repo.ListConversations(ctx, nil, query)
	}
	return s.repo.ListConversations(ctx, &principal.UserID, query)
}

func (s *HistoryService) ListConversationMessages(ctx context.Context, principal model.CurrentPrincipal, conversationID string, query model.CursorQuery) ([]model.HistoryRecord, error) {
	if principal.IsAdmin() {
		return s.repo.ListConversationMessages(ctx, nil, conversationID, query)
	}
	return s.repo.ListConversationMessages(ctx, &principal.UserID, conversationID, query)
}

func NewHistoryService(repo HistoryRepository) *HistoryService {
	return &HistoryService{repo: repo}
}

func (s *HistoryService) Create(ctx context.Context, principal model.CurrentPrincipal, prompt string, request map[string]any) (*model.HistoryRecord, error) {
	return s.repo.Create(ctx, principal, prompt, request)
}

func (s *HistoryService) Update(ctx context.Context, record *model.HistoryRecord) error {
	return s.repo.Update(ctx, record)
}

func (s *HistoryService) List(ctx context.Context, principal model.CurrentPrincipal, query model.HistoryQuery) ([]model.HistoryRecord, error) {
	if principal.IsAdmin() {
		return s.repo.ListAll(ctx, query)
	}
	return s.repo.ListByUser(ctx, principal.UserID, query)
}

func (s *HistoryService) Get(ctx context.Context, principal model.CurrentPrincipal, id string) (*model.HistoryRecord, error) {
	record, ok, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, repository.ErrNotFound
	}
	if !principal.IsAdmin() && record.UserID != principal.UserID {
		return nil, ErrHistoryForbidden
	}
	return record, nil
}

func (s *HistoryService) Delete(ctx context.Context, principal model.CurrentPrincipal, id string) error {
	record, err := s.Get(ctx, principal, id)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, record.ID)
}
