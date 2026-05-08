package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	ImageTaskStatusPending   = "pending"
	ImageTaskStatusRunning   = "running"
	ImageTaskStatusSucceeded = "succeeded"
	ImageTaskStatusFailed    = "failed"
	ImageTaskStatusExpired   = "expired"

	defaultImageTaskTTL = time.Hour
)

var (
	ErrImageTaskNotFound = errors.New("image task not found")
	ErrImageTaskExpired  = errors.New("image task expired")
)

type ImageTask struct {
	TaskID       string
	UserID       int64
	APIKeyID     int64
	Status       string
	Endpoint     string
	Model        string
	Prompt       string
	FilePath     string
	MimeType     string
	ByteSize     int64
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ExpiresAt    time.Time
}

type ImageTaskRepository interface {
	Create(ctx context.Context, task *ImageTask) error
	GetByTaskID(ctx context.Context, taskID string) (*ImageTask, error)
	MarkRunning(ctx context.Context, taskID string) error
	MarkSucceeded(ctx context.Context, taskID, filePath, mimeType string, byteSize int64, expiresAt time.Time) error
	MarkFailed(ctx context.Context, taskID, message string, expiresAt time.Time) error
	DeleteExpired(ctx context.Context, now time.Time) ([]ImageTask, error)
	MarkStaleRunningFailed(ctx context.Context, message string) error
}

type ImageTaskService struct {
	repo       ImageTaskRepository
	storageDir string
	ttl        time.Duration
	now        func() time.Time
	startOnce  sync.Once
}

func NewImageTaskService(repo ImageTaskRepository) *ImageTaskService {
	return NewImageTaskServiceWithOptions(repo, "", 0)
}

func NewImageTaskServiceWithOptions(repo ImageTaskRepository, storageDir string, ttl time.Duration) *ImageTaskService {
	if ttl <= 0 {
		ttl = defaultImageTaskTTL
	}
	if strings.TrimSpace(storageDir) == "" {
		storageDir = filepath.Join(os.TempDir(), "sub2api-image-tasks")
	}
	return &ImageTaskService{
		repo:       repo,
		storageDir: storageDir,
		ttl:        ttl,
		now:        time.Now,
	}
}

func (s *ImageTaskService) Start(timingWheel *TimingWheelService) {
	if s == nil || timingWheel == nil {
		return
	}
	s.startOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.MarkStaleRunningFailed(ctx)
		_, _ = s.CleanupExpired(ctx)
		cancel()
		timingWheel.ScheduleRecurring("image_task_cleanup", 5*time.Minute, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, _ = s.CleanupExpired(ctx)
		})
	})
}

func (s *ImageTaskService) CreatePending(ctx context.Context, userID, apiKeyID int64, endpoint, model, prompt string) (*ImageTask, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("image task service is unavailable")
	}
	now := s.now()
	task := &ImageTask{
		TaskID:    newImageTaskID(),
		UserID:    userID,
		APIKeyID:  apiKeyID,
		Status:    ImageTaskStatusPending,
		Endpoint:  strings.TrimSpace(endpoint),
		Model:     strings.TrimSpace(model),
		Prompt:    strings.TrimSpace(prompt),
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}
	if err := s.repo.Create(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ImageTaskService) GetForUser(ctx context.Context, taskID string, userID int64) (*ImageTask, error) {
	task, err := s.repo.GetByTaskID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	if task == nil || task.UserID != userID {
		return nil, ErrImageTaskNotFound
	}
	if s.now().After(task.ExpiresAt) {
		task.Status = ImageTaskStatusExpired
		return task, nil
	}
	return task, nil
}

func (s *ImageTaskService) GetForAPIKey(ctx context.Context, taskID string, userID, apiKeyID int64) (*ImageTask, error) {
	task, err := s.GetForUser(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	if task.APIKeyID != apiKeyID {
		return nil, ErrImageTaskNotFound
	}
	return task, nil
}

func (s *ImageTaskService) MarkRunning(ctx context.Context, taskID string) error {
	return s.repo.MarkRunning(ctx, taskID)
}

func (s *ImageTaskService) SaveResult(ctx context.Context, taskID string, data []byte, mimeType string) (*ImageTask, error) {
	if len(data) == 0 {
		return nil, errors.New("image result is empty")
	}
	task, err := s.repo.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.storageDir, 0o755); err != nil {
		return nil, fmt.Errorf("create image task storage: %w", err)
	}
	mimeType = normalizeImageTaskMimeType(mimeType)
	filePath := filepath.Join(s.storageDir, taskID+imageTaskExtension(mimeType))
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return nil, fmt.Errorf("write image task file: %w", err)
	}
	expiresAt := s.now().Add(s.ttl)
	if err := s.repo.MarkSucceeded(ctx, taskID, filePath, mimeType, int64(len(data)), expiresAt); err != nil {
		_ = os.Remove(filePath)
		return nil, err
	}
	task.Status = ImageTaskStatusSucceeded
	task.FilePath = filePath
	task.MimeType = mimeType
	task.ByteSize = int64(len(data))
	task.ExpiresAt = expiresAt
	return task, nil
}

func (s *ImageTaskService) MarkFailed(ctx context.Context, taskID string, err error) error {
	message := "image generation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	return s.repo.MarkFailed(ctx, taskID, message, s.now().Add(s.ttl))
}

func (s *ImageTaskService) CleanupExpired(ctx context.Context) (int, error) {
	tasks, err := s.repo.DeleteExpired(ctx, s.now())
	if err != nil {
		return 0, err
	}
	for _, task := range tasks {
		if strings.TrimSpace(task.FilePath) != "" {
			_ = os.Remove(task.FilePath)
		}
	}
	return len(tasks), nil
}

func (s *ImageTaskService) MarkStaleRunningFailed(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.MarkStaleRunningFailed(ctx, "server restarted before task completed")
}

func newImageTaskID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("img_%d", time.Now().UnixNano())
	}
	return "img_" + hex.EncodeToString(b[:])
}

func normalizeImageTaskMimeType(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp":
		return mimeType
	default:
		return "image/png"
	}
}

func imageTaskExtension(mimeType string) string {
	switch normalizeImageTaskMimeType(mimeType) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
