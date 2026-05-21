package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type PlaygroundService struct {
	repo          PlaygroundRepository
	apiKeyService *APIKeyService
}

func NewPlaygroundService(repo PlaygroundRepository, apiKeyService *APIKeyService) *PlaygroundService {
	return &PlaygroundService{repo: repo, apiKeyService: apiKeyService}
}

func (s *PlaygroundService) ListChatSessions(ctx context.Context, userID int64, params pagination.PaginationParams, filters PlaygroundChatSessionListFilters) ([]PlaygroundChatSession, *pagination.PaginationResult, error) {
	page, pageSize := normalizePlaygroundPagination(params.Page, params.PageSize)
	items, total, err := s.repo.ListChatSessions(ctx, userID, page, pageSize, filters)
	if err != nil {
		return nil, nil, err
	}
	return items, playgroundPaginationResult(total, page, pageSize), nil
}

func (s *PlaygroundService) CreateChatSession(ctx context.Context, userID int64, req CreatePlaygroundChatSessionRequest) (*PlaygroundChatSession, error) {
	if err := s.ensureAPIKeyOwnership(ctx, userID, req.APIKeyID); err != nil {
		return nil, err
	}
	req.Title = defaultPlaygroundTitle(req.Title)
	req.Model = strings.TrimSpace(req.Model)
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	return s.repo.CreateChatSession(ctx, userID, req)
}

func (s *PlaygroundService) GetChatSession(ctx context.Context, userID, sessionID int64) (*PlaygroundChatSession, error) {
	return s.repo.GetChatSession(ctx, userID, sessionID)
}

func (s *PlaygroundService) UpdateChatSession(ctx context.Context, userID, sessionID int64, req UpdatePlaygroundChatSessionRequest) (*PlaygroundChatSession, error) {
	if err := s.ensureAPIKeyOwnership(ctx, userID, req.APIKeyID); err != nil {
		return nil, err
	}
	return s.repo.UpdateChatSession(ctx, userID, sessionID, req)
}

func (s *PlaygroundService) DeleteChatSession(ctx context.Context, userID, sessionID int64) error {
	return s.repo.DeleteChatSession(ctx, userID, sessionID)
}

func (s *PlaygroundService) ListChatMessages(ctx context.Context, userID, sessionID int64, params pagination.PaginationParams) ([]PlaygroundChatMessage, *pagination.PaginationResult, error) {
	page, pageSize := normalizePlaygroundPagination(params.Page, params.PageSize)
	items, total, err := s.repo.ListChatMessages(ctx, userID, sessionID, page, pageSize)
	if err != nil {
		return nil, nil, err
	}
	return items, playgroundPaginationResult(total, page, pageSize), nil
}

func (s *PlaygroundService) CreateChatMessage(ctx context.Context, userID, sessionID int64, req CreatePlaygroundChatMessageRequest) (*PlaygroundChatMessage, error) {
	if err := s.ensureAPIKeyOwnership(ctx, userID, req.APIKeyID); err != nil {
		return nil, err
	}
	req.Role = strings.TrimSpace(req.Role)
	if req.Role == "" {
		return nil, fmt.Errorf("role is required")
	}
	if req.Status == "" {
		req.Status = PlaygroundChatMessageStatusSuccess
	}
	if req.ContentJSON == nil {
		req.ContentJSON = map[string]any{}
	}
	if req.Images == nil {
		req.Images = []map[string]any{}
	}
	if req.Usage == nil {
		req.Usage = map[string]any{}
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	return s.repo.CreateChatMessage(ctx, userID, sessionID, req)
}

func (s *PlaygroundService) ListImageTasks(ctx context.Context, userID int64, params pagination.PaginationParams, filters PlaygroundImageTaskListFilters) ([]PlaygroundImageTask, *pagination.PaginationResult, error) {
	page, pageSize := normalizePlaygroundPagination(params.Page, params.PageSize)
	filters.Status = strings.TrimSpace(filters.Status)
	items, total, err := s.repo.ListImageTasks(ctx, userID, page, pageSize, filters)
	if err != nil {
		return nil, nil, err
	}
	return items, playgroundPaginationResult(total, page, pageSize), nil
}

func (s *PlaygroundService) CreateImageTask(ctx context.Context, userID int64, req CreatePlaygroundImageTaskRequest) (*PlaygroundImageTask, error) {
	if err := s.ensureAPIKeyOwnership(ctx, userID, req.APIKeyID); err != nil {
		return nil, err
	}
	req.Model = strings.TrimSpace(req.Model)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Status == "" {
		req.Status = PlaygroundImageTaskStatusPending
	}
	if req.Endpoint == "" {
		req.Endpoint = "/v1/images/generations"
	}
	if req.N <= 0 {
		req.N = 1
	}
	if req.Request == nil {
		req.Request = map[string]any{}
	}
	if req.ReferenceImages == nil {
		req.ReferenceImages = []map[string]any{}
	}
	if req.ResultImages == nil {
		req.ResultImages = []map[string]any{}
	}
	if req.Response == nil {
		req.Response = map[string]any{}
	}
	if req.Usage == nil {
		req.Usage = map[string]any{}
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	return s.repo.CreateImageTask(ctx, userID, req)
}

func (s *PlaygroundService) GetImageTask(ctx context.Context, userID, taskID int64) (*PlaygroundImageTask, error) {
	return s.repo.GetImageTask(ctx, userID, taskID)
}

func (s *PlaygroundService) UpdateImageTask(ctx context.Context, userID, taskID int64, req UpdatePlaygroundImageTaskRequest) (*PlaygroundImageTask, error) {
	return s.repo.UpdateImageTask(ctx, userID, taskID, req)
}

func (s *PlaygroundService) DeleteImageTask(ctx context.Context, userID, taskID int64) error {
	return s.repo.DeleteImageTask(ctx, userID, taskID)
}

func (s *PlaygroundService) ensureAPIKeyOwnership(ctx context.Context, userID int64, apiKeyID *int64) error {
	if apiKeyID == nil || *apiKeyID == 0 {
		return nil
	}
	if s.apiKeyService == nil {
		return fmt.Errorf("api key service is unavailable")
	}
	key, err := s.apiKeyService.GetByID(ctx, *apiKeyID)
	if err != nil {
		return err
	}
	if key.UserID != userID {
		return ErrInsufficientPerms
	}
	return nil
}

func defaultPlaygroundTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "新会话"
	}
	if len([]rune(title)) > 200 {
		return string([]rune(title)[:200])
	}
	return title
}

func playgroundPaginationResult(total int64, page, pageSize int) *pagination.PaginationResult {
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		pages = 1
	}
	return &pagination.PaginationResult{Total: total, Page: page, PageSize: pageSize, Pages: pages}
}

func normalizePlaygroundPagination(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
