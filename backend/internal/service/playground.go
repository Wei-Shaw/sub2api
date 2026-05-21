package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrPlaygroundRecordNotFound = infraerrors.NotFound("PLAYGROUND_RECORD_NOT_FOUND", "playground record not found")

const (
	PlaygroundChatMessageStatusSuccess = "success"
	PlaygroundChatMessageStatusError   = "error"

	PlaygroundImageTaskStatusPending = "pending"
	PlaygroundImageTaskStatusRunning = "running"
	PlaygroundImageTaskStatusSuccess = "success"
	PlaygroundImageTaskStatusError   = "error"
)

type PlaygroundChatSession struct {
	ID            int64
	UserID        int64
	Title         string
	Model         string
	APIKeyID      *int64
	SystemPrompt  string
	UseContext    bool
	Metadata      map[string]any
	LastMessageAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PlaygroundChatMessage struct {
	ID          int64
	SessionID   int64
	UserID      int64
	APIKeyID    *int64
	Role        string
	Model       string
	Content     string
	ContentJSON map[string]any
	Images      []map[string]any
	Usage       map[string]any
	Status      string
	Error       string
	DurationMS  *int
	Metadata    map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type PlaygroundImageTask struct {
	ID              int64
	UserID          int64
	APIKeyID        *int64
	Model           string
	Prompt          string
	Quality         string
	Size            string
	N               int
	Endpoint        string
	Status          string
	Request         map[string]any
	ReferenceImages []map[string]any
	ResultImages    []map[string]any
	Response        map[string]any
	Error           string
	Usage           map[string]any
	Cost            float64
	DurationMS      *int
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type PlaygroundChatSessionListFilters struct{}
type PlaygroundImageTaskListFilters struct {
	Status string
}

type CreatePlaygroundChatSessionRequest struct {
	Title        string
	Model        string
	APIKeyID     *int64
	SystemPrompt string
	UseContext   bool
	Metadata     map[string]any
}

type UpdatePlaygroundChatSessionRequest struct {
	Title        *string
	Model        *string
	APIKeyID     *int64
	SystemPrompt *string
	UseContext   *bool
	Metadata     map[string]any
}

type CreatePlaygroundChatMessageRequest struct {
	APIKeyID     *int64
	Role         string
	Model        string
	Content      string
	ContentJSON  map[string]any
	Images       []map[string]any
	Usage        map[string]any
	Status       string
	Error        string
	DurationMS   *int
	Metadata     map[string]any
	TouchSession bool
}

type CreatePlaygroundImageTaskRequest struct {
	APIKeyID        *int64
	Model           string
	Prompt          string
	Quality         string
	Size            string
	N               int
	Endpoint        string
	Status          string
	Request         map[string]any
	ReferenceImages []map[string]any
	ResultImages    []map[string]any
	Response        map[string]any
	Error           string
	Usage           map[string]any
	Cost            float64
	DurationMS      *int
	Metadata        map[string]any
}

type UpdatePlaygroundImageTaskRequest struct {
	Status       *string
	ResultImages []map[string]any
	Response     map[string]any
	Error        *string
	Usage        map[string]any
	Cost         *float64
	DurationMS   *int
	Metadata     map[string]any
}

type PlaygroundRepository interface {
	ListChatSessions(ctx context.Context, userID int64, page, pageSize int, filters PlaygroundChatSessionListFilters) ([]PlaygroundChatSession, int64, error)
	CreateChatSession(ctx context.Context, userID int64, req CreatePlaygroundChatSessionRequest) (*PlaygroundChatSession, error)
	GetChatSession(ctx context.Context, userID, sessionID int64) (*PlaygroundChatSession, error)
	UpdateChatSession(ctx context.Context, userID, sessionID int64, req UpdatePlaygroundChatSessionRequest) (*PlaygroundChatSession, error)
	DeleteChatSession(ctx context.Context, userID, sessionID int64) error
	ListChatMessages(ctx context.Context, userID, sessionID int64, page, pageSize int) ([]PlaygroundChatMessage, int64, error)
	CreateChatMessage(ctx context.Context, userID, sessionID int64, req CreatePlaygroundChatMessageRequest) (*PlaygroundChatMessage, error)

	ListImageTasks(ctx context.Context, userID int64, page, pageSize int, filters PlaygroundImageTaskListFilters) ([]PlaygroundImageTask, int64, error)
	CreateImageTask(ctx context.Context, userID int64, req CreatePlaygroundImageTaskRequest) (*PlaygroundImageTask, error)
	GetImageTask(ctx context.Context, userID, taskID int64) (*PlaygroundImageTask, error)
	UpdateImageTask(ctx context.Context, userID, taskID int64, req UpdatePlaygroundImageTaskRequest) (*PlaygroundImageTask, error)
	DeleteImageTask(ctx context.Context, userID, taskID int64) error
}
