package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	AuditCaptureMaxBytes = 64 * 1024
)

type AuditLog struct {
	ID                int64     `json:"id"`
	RequestID         string    `json:"request_id"`
	SessionID         string    `json:"session_id"`
	RequestCount      int       `json:"request_count"`
	UserID            *int64    `json:"user_id,omitempty"`
	UserEmail         string    `json:"user_email"`
	APIKeyID          *int64    `json:"api_key_id,omitempty"`
	APIKeyName        string    `json:"api_key_name"`
	GroupID           *int64    `json:"group_id,omitempty"`
	GroupName         string    `json:"group_name"`
	Platform          string    `json:"platform"`
	Endpoint          string    `json:"endpoint"`
	Method            string    `json:"method"`
	Model             string    `json:"model"`
	StatusCode        int       `json:"status_code"`
	RequestBody       string    `json:"request_body"`
	ResponseBody      string    `json:"response_body"`
	RequestTruncated  bool      `json:"request_truncated"`
	ResponseTruncated bool      `json:"response_truncated"`
	DurationMS        int       `json:"duration_ms"`
	IPAddress         string    `json:"ip_address"`
	UserAgent         string    `json:"user_agent"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type AuditLogFilter struct {
	Pagination pagination.PaginationParams
	Search     string
	Platform   string
	Model      string
	Endpoint   string
	From       *time.Time
	To         *time.Time
}

type AuditRepository interface {
	Create(ctx context.Context, log *AuditLog) error
	List(ctx context.Context, filter AuditLogFilter) ([]AuditLog, *pagination.PaginationResult, error)
}

type AuditService struct {
	repo AuditRepository
}

func NewAuditService(repo AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) Create(ctx context.Context, log *AuditLog) error {
	if s == nil || s.repo == nil || log == nil {
		return nil
	}
	return s.repo.Create(ctx, log)
}

func (s *AuditService) List(ctx context.Context, filter AuditLogFilter) ([]AuditLog, *pagination.PaginationResult, error) {
	if filter.Pagination.Page <= 0 {
		filter.Pagination.Page = 1
	}
	if filter.Pagination.PageSize <= 0 {
		filter.Pagination.PageSize = 20
	}
	if filter.Pagination.PageSize > 100 {
		filter.Pagination.PageSize = 100
	}
	return s.repo.List(ctx, filter)
}
